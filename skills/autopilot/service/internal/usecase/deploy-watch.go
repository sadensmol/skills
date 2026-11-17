package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/sadensmol/autopilot/internal/config"
	"github.com/sadensmol/autopilot/internal/domain"
	"github.com/sadensmol/autopilot/internal/gh"
	"github.com/sadensmol/autopilot/internal/slackpost"
)

const (
	// deployCheckTimeout bounds one try of a repo's deploy check.
	deployCheckTimeout = 60 * time.Second
	jobCompleted       = "completed"
	jobSuccess         = "success"
	jobSkipped         = "skipped"
)

// IDeployRuns is the part of the run store that the deploy watch uses.
type IDeployRuns interface {
	PendingDeploys(ctx context.Context) ([]domain.PendingDeploy, error)
	SetDeployState(ctx context.Context, key domain.IssueKey, repo string, state domain.DeployState, now time.Time) error
	SetDeployRetried(ctx context.Context, key domain.IssueKey, repo string, now time.Time) error
	Repos(ctx context.Context, key domain.IssueKey) ([]domain.RunRepo, error)
	SetPhase(ctx context.Context, key domain.IssueKey, phase domain.Phase, now time.Time) error
}

// IDeployGit reads the CI jobs of a commit and the default branch, and re-runs failed jobs.
type IDeployGit interface {
	JobOnCommit(ctx context.Context, repo, sha, job string) (gh.Job, bool, error)
	RerunFailed(ctx context.Context, repo string, runID int64) error
	BranchHead(ctx context.Context, repo, branch string) (string, error)
	Contains(ctx context.Context, repo, sha, ancestor string) (bool, error)
}

// IDeployCheck runs the deploy check of a repo; nil means the commit is live.
type IDeployCheck interface {
	Run(ctx context.Context, command string) error
}

// IDeploySessions types the fault of a stuck deploy into the control room session.
type IDeploySessions interface {
	SessionByName(ctx context.Context, workspace, name string) (string, error)
	Type(ctx context.Context, sessionID, text string) error
}

type DeployWatchData struct {
	Registry config.Registry
	// Interval is the pause between two polls of the pending deploys.
	Interval time.Duration
	Linear   IMirrorLinear
	Slack    IPRWatchSlack
	Runs     IDeployRuns
	GH       IDeployGit
	Checks   IDeployCheck
	// AgtermWorkspace holds the control room session, which gets the fault of a stuck deploy.
	AgtermWorkspace string
	Sessions        IDeploySessions
	Now             func() time.Time
	// Sleep pauses between two polls; it returns the context's error when the context ends.
	Sleep func(ctx context.Context, d time.Duration) error
}

// DeployWatch waits until the merge of every repo with an automatic deploy is live: its deploy job passed on the
// merge commit and its deploy check exits 0. When every repo of a run is live, the run is done. A failed or skipped
// deploy job gets one rerun of the failed jobs of its CI run. A second failure, or no live merge within the repo's
// timeout, makes the run stuck and hands the fault to the control room; the watch then waits for the fix on the
// default branch.
type DeployWatch struct {
	d DeployWatchData
}

func NewDeployWatch(d DeployWatchData) *DeployWatch {
	return &DeployWatch{d: d}
}

// Run polls every interval until the context ends; it returns the context's error then.
func (w *DeployWatch) Run(ctx context.Context) error {
	for {
		if err := w.Tick(ctx); err != nil && ctx.Err() == nil {
			log.Error().Ctx(ctx).Err(err).Msg("deploy watch failed")
		}
		if err := w.d.Sleep(ctx, w.d.Interval); err != nil {
			return err
		}
	}
}

// Tick checks every pending deploy once. A failing repo does not stop the others.
func (w *DeployWatch) Tick(ctx context.Context) error {
	pending, err := w.d.Runs.PendingDeploys(ctx)
	if err != nil {
		return fmt.Errorf("cannot list pending deploys: %w", err)
	}
	var firstErr error
	for _, p := range pending {
		if err := w.check(ctx, p); err != nil {
			log.Error().Ctx(ctx).Err(err).Msgf("deploy watch of %s in run %s failed", p.Repo.Repo, p.Run.IssueKey)
			if firstErr == nil {
				firstErr = err
			}
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return firstErr
}

// deployCommit is one commit of a merged repo whose deploy the watch reads: the repo in the registry, its
// owner/name on GitHub and the commit.
type deployCommit struct {
	repo config.Repo
	slug string
	sha  string
}

// deployStep is where the deploy of one commit stands: pending, deployed, failed or skipped. Job is the deploy job
// when CI has one for the commit.
type deployStep struct {
	state domain.DeployState
	job   gh.Job
}

// deployFault is why a deploy is stuck: the reason of the stuck line, what fails for the control room, and the
// deploy job when CI has one.
type deployFault struct {
	reason string
	fails  string
	job    gh.Job
}

// check moves one pending deploy on. A deploy that is not live yet stays pending, quietly. The first failed or
// skipped deploy job re-runs the failed jobs of its CI run, quietly. A live or stuck deploy is posted to the thread
// before its state is stored, so a failed post is tried again. A faulted deploy waits for the fix, quietly.
func (w *DeployWatch) check(ctx context.Context, p domain.PendingDeploy) error {
	project, err := runProject(w.d.Registry, p.Run)
	if err != nil {
		return err
	}
	repo, ok := project.Repo(p.Repo.Repo)
	if !ok {
		return fmt.Errorf("repo %s of run %s is not in the workspace registry", p.Repo.Repo, p.Run.IssueKey)
	}
	ref, err := gh.ParsePRURL(p.Repo.PRURL)
	if err != nil {
		return fmt.Errorf("cannot watch the deploy of %s in run %s: %w", p.Repo.Repo, p.Run.IssueKey, err)
	}
	c := deployCommit{repo: repo, slug: fmt.Sprintf("%s/%s", ref.Owner, ref.Repo), sha: p.Repo.Merge.SHA}
	if p.Repo.Merge.DeployState == domain.DeployFaulted {
		return w.waitFix(ctx, p, project, c)
	}
	step, err := w.progress(ctx, p, c)
	if err != nil {
		return err
	}
	switch step.state {
	case domain.DeployDeployed:
		return w.live(ctx, p, project)
	case domain.DeploySkipped, domain.DeployFailed:
		if p.Repo.Merge.RetriedAt.IsZero() {
			err := w.rerun(ctx, p, c.slug, step.job)
			if err == nil {
				return nil
			}
			log.Warn().Ctx(ctx).Err(err).Msgf("the deploy of %s in run %s is stuck without a rerun", p.Repo.Repo, p.Run.IssueKey)
		}
		fault := deployFault{reason: fmt.Sprintf("failed: %s", step.job.URL), fails: fmt.Sprintf("failed: %s", step.job.URL), job: step.job}
		if step.state == domain.DeploySkipped {
			fault.reason = "failed: job was skipped"
		}
		return w.stuck(ctx, p, repo, fault)
	}
	if w.d.Now().Sub(p.Repo.Merge.WaitsSince()) <= repo.Deploy.Timeout {
		return nil
	}
	late := fmt.Sprintf("not live after %d minutes", int(repo.Deploy.Timeout.Minutes()))
	return w.stuck(ctx, p, repo, deployFault{reason: late, fails: late, job: step.job})
}

// progress tells where the deploy of a commit stands. The deploy check runs only after the deploy job passed.
func (w *DeployWatch) progress(ctx context.Context, p domain.PendingDeploy, c deployCommit) (deployStep, error) {
	step := deployStep{state: domain.DeployPending}
	published := true
	if c.repo.Deploy.Job != "" {
		job, found, err := w.d.GH.JobOnCommit(ctx, c.slug, c.sha, c.repo.Deploy.Job)
		if err != nil {
			return deployStep{}, fmt.Errorf("cannot read the deploy job of %s in run %s: %w", p.Repo.Repo, p.Run.IssueKey, err)
		}
		step.job = job
		switch {
		case found && job.Status == jobCompleted && job.Conclusion == jobSkipped:
			step.state = domain.DeploySkipped
			return step, nil
		case found && job.Status == jobCompleted && job.Conclusion != jobSuccess:
			step.state = domain.DeployFailed
			return step, nil
		}
		published = found && job.Status == jobCompleted
	}
	if published && w.liveByCheck(ctx, p, c) {
		step.state = domain.DeployDeployed
	}
	return step, nil
}

// liveByCheck runs the deploy check with the commit in place of {sha} and {short_sha}; no check counts as live. A
// check that fails is not live yet: the timeout decides when the run is stuck.
func (w *DeployWatch) liveByCheck(ctx context.Context, p domain.PendingDeploy, c deployCommit) bool {
	if c.repo.Deploy.Check == "" {
		return true
	}
	command := strings.NewReplacer("{sha}", c.sha, "{short_sha}", domain.ShortSHA(c.sha)).Replace(c.repo.Deploy.Check)
	ctx, cancel := context.WithTimeout(ctx, deployCheckTimeout)
	defer cancel()
	if err := w.d.Checks.Run(ctx, command); err != nil {
		log.Debug().Ctx(ctx).Err(err).Msgf("%s of run %s is not live yet at %s", p.Repo.Repo, p.Run.IssueKey, domain.ShortSHA(c.sha))
		return false
	}
	return true
}

// rerun re-runs the failed jobs of the CI run that holds the deploy job, once per merge; the timeout starts again.
func (w *DeployWatch) rerun(ctx context.Context, p domain.PendingDeploy, slug string, job gh.Job) error {
	if err := w.d.GH.RerunFailed(ctx, slug, job.Run.ID); err != nil {
		return fmt.Errorf("cannot re-run the failed jobs of CI run %d of %s: %w", job.Run.ID, slug, err)
	}
	if err := w.d.Runs.SetDeployRetried(ctx, p.Run.IssueKey, p.Repo.Repo, w.d.Now()); err != nil {
		return fmt.Errorf("cannot store the deploy retry of %s in run %s: %w", p.Repo.Repo, p.Run.IssueKey, err)
	}
	log.Info().Ctx(ctx).Msgf("re-ran the failed jobs of CI run %d for the deploy of %s in run %s", job.Run.ID, p.Repo.Repo, p.Run.IssueKey)
	return nil
}

// waitFix watches a faulted deploy until the fix of the control room is live: the newest commit of the default
// branch that contains the merge commit passes the deploy job and the deploy check. It posts nothing before that.
func (w *DeployWatch) waitFix(ctx context.Context, p domain.PendingDeploy, project config.Project, c deployCommit) error {
	head, err := w.d.GH.BranchHead(ctx, c.slug, c.repo.DefaultBranch)
	if err != nil {
		return fmt.Errorf("cannot read the head of %s of %s in run %s: %w", c.repo.DefaultBranch, p.Repo.Repo, p.Run.IssueKey, err)
	}
	if head != c.sha {
		contains, err := w.d.GH.Contains(ctx, c.slug, head, c.sha)
		if err != nil {
			return fmt.Errorf("cannot compare the head of %s with the merge of %s in run %s: %w", c.repo.DefaultBranch, p.Repo.Repo, p.Run.IssueKey, err)
		}
		if contains {
			c.sha = head
		}
	}
	step, err := w.progress(ctx, p, c)
	if err != nil {
		return err
	}
	if step.state == domain.DeployDeployed {
		return w.live(ctx, p, project)
	}
	return nil
}

// live stores a live repo. When it is the last repo of its run to go live, the run is done first: Linear done, the
// live line, the phase done, then the done reaction in place of the working one.
func (w *DeployWatch) live(ctx context.Context, p domain.PendingDeploy, project config.Project) error {
	run := p.Run
	repos, err := w.d.Runs.Repos(ctx, run.IssueKey)
	if err != nil {
		return fmt.Errorf("cannot read the repos of run %s: %w", run.IssueKey, err)
	}
	var lives []string
	for _, r := range repos {
		if r.Repo == p.Repo.Repo {
			r.Merge.DeployState = domain.DeployDeployed
		}
		switch r.Merge.DeployState {
		case "":
		case domain.DeployDeployed:
			lives = append(lives, fmt.Sprintf("%s %s", r.Repo, r.Merge.ShortSHA()))
		default:
			return w.store(ctx, p, domain.DeployDeployed)
		}
	}
	if err := (linearMirror{linear: w.d.Linear, registry: w.d.Registry}).setDone(ctx, run); err != nil {
		return err
	}
	where := strings.Join(lives, ", ")
	if len(lives) == 1 {
		where = p.Repo.Merge.ShortSHA()
	}
	line := fmt.Sprintf("✅ %s is live (%s). Ready to test.", run.IssueKey, where)
	if manual := handDeploys(project, repos); manual != "" {
		line = fmt.Sprintf("%s Deploy by hand: %s.", line, manual)
	}
	if err := w.d.Slack.Thread(ctx, run.ChannelID, run.ThreadTS, line); err != nil {
		return fmt.Errorf("cannot post to the thread of run %s: %w", run.IssueKey, err)
	}
	if err := w.d.Runs.SetPhase(ctx, run.IssueKey, donePhase, w.d.Now()); err != nil {
		return fmt.Errorf("cannot set phase of run %s: %w", run.IssueKey, err)
	}
	if err := w.store(ctx, p, domain.DeployDeployed); err != nil {
		return err
	}
	clearWorking(ctx, w.d.Slack, run)
	if err := w.d.Slack.React(ctx, run.ChannelID, run.RequestTS, slackpost.ReactionDone); err != nil {
		log.Warn().Ctx(ctx).Err(err).Msgf("cannot add the done reaction of run %s", run.IssueKey)
	}
	return nil
}

// stuck posts the stuck line of a deploy, stores it as faulted, puts the stuck reaction in place of the working one
// and hands the fault to the control room. Linear keeps its status: the control room fixes the default branch, and
// the watch finishes the run when the fix is live.
func (w *DeployWatch) stuck(ctx context.Context, p domain.PendingDeploy, repo config.Repo, fault deployFault) error {
	run := p.Run
	line := fmt.Sprintf("⛔ %s is stuck: deploy of %s %s.", run.IssueKey, p.Repo.Repo, fault.reason)
	if err := w.d.Slack.Thread(ctx, run.ChannelID, run.ThreadTS, line); err != nil {
		return fmt.Errorf("cannot post to the thread of run %s: %w", run.IssueKey, err)
	}
	if err := w.store(ctx, p, domain.DeployFaulted); err != nil {
		return err
	}
	clearWorking(ctx, w.d.Slack, run)
	if err := w.d.Slack.React(ctx, run.ChannelID, run.RequestTS, slackpost.ReactionStuck); err != nil {
		log.Warn().Ctx(ctx).Err(err).Msgf("cannot add the stuck reaction of run %s", run.IssueKey)
	}
	w.handFault(ctx, p, repo, fault)
	return nil
}

// handFault types the fault line into the control room session, then Enter. The deploy is stored as faulted
// already, so a failure is only logged.
func (w *DeployWatch) handFault(ctx context.Context, p domain.PendingDeploy, repo config.Repo, fault deployFault) {
	proof := "the deploy check did not pass"
	if fault.job.Run.URL != "" {
		failed := "none"
		if len(fault.job.Run.Failed) > 0 {
			failed = strings.Join(fault.job.Run.Failed, ", ")
		}
		proof = fmt.Sprintf("%s (failed jobs: %s)", fault.job.Run.URL, failed)
	}
	line := fmt.Sprintf("autopilot fault %s: deploy of %s %s; proof: %s; fix: find why CI on %s fails at %s, fix it "+
		"with a PR, then run autopilot merge-fix %s <PR url>; the deploy check then passes",
		p.Run.IssueKey, p.Repo.Repo, fault.fails, proof, repo.DefaultBranch, p.Repo.Merge.ShortSHA(), p.Run.IssueKey)
	control, err := w.d.Sessions.SessionByName(ctx, w.d.AgtermWorkspace, controlSession)
	if err != nil {
		log.Error().Ctx(ctx).Err(err).Msgf("cannot hand the deploy fault of run %s to the control room", p.Run.IssueKey)
		return
	}
	for _, keys := range []string{line, "\r"} {
		if err := w.d.Sessions.Type(ctx, control, keys); err != nil {
			log.Error().Ctx(ctx).Err(err).Msgf("cannot type the deploy fault of run %s into the control room", p.Run.IssueKey)
			return
		}
	}
}

func (w *DeployWatch) store(ctx context.Context, p domain.PendingDeploy, state domain.DeployState) error {
	if err := w.d.Runs.SetDeployState(ctx, p.Run.IssueKey, p.Repo.Repo, state, w.d.Now()); err != nil {
		return fmt.Errorf("cannot store the deploy state of %s in run %s: %w", p.Repo.Repo, p.Run.IssueKey, err)
	}
	return nil
}

// handDeploys says how a person deploys the merged repos that the registry marks manual: the one text alone, or
// each text after its repo name. It is empty when no merged repo is manual.
func handDeploys(project config.Project, repos []domain.RunRepo) string {
	var texts []string
	var only string
	for _, r := range repos {
		repo, ok := project.Repo(r.Repo)
		if r.Watch.State != prMerged || !ok || repo.Deploy.Manual == "" {
			continue
		}
		only = strings.TrimRight(repo.Deploy.Manual, ". ")
		texts = append(texts, fmt.Sprintf("%s: %s", r.Repo, only))
	}
	if len(texts) == 1 {
		return only
	}
	return strings.Join(texts, "; ")
}
