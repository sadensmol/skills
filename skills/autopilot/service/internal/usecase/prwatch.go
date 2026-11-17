package usecase

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/sadensmol/autopilot/internal/agterm"
	"github.com/sadensmol/autopilot/internal/config"
	"github.com/sadensmol/autopilot/internal/domain"
	"github.com/sadensmol/autopilot/internal/engine"
	"github.com/sadensmol/autopilot/internal/gh"
	"github.com/sadensmol/autopilot/internal/slackpost"
)

const (
	prOpen                 = "open"
	prMerged               = "merged"
	prClosed               = "closed"
	reviewChangesRequested = "CHANGES_REQUESTED"
	reviewCommented        = "COMMENTED"
)

// IPRWatchRuns is the part of the run store that the PR watch uses.
type IPRWatchRuns interface {
	ListActive(ctx context.Context) ([]domain.Run, error)
	FindByKey(ctx context.Context, key domain.IssueKey) (domain.Run, error)
	Repos(ctx context.Context, key domain.IssueKey) ([]domain.RunRepo, error)
	SetPRWatch(ctx context.Context, key domain.IssueKey, repo string, w domain.PRWatch, now time.Time) error
	SetMerge(ctx context.Context, key domain.IssueKey, repo string, m domain.Merge, now time.Time) error
	SetPhase(ctx context.Context, key domain.IssueKey, phase domain.Phase, now time.Time) error
	SetLastPostAt(ctx context.Context, key domain.IssueKey, at time.Time) error
	End(ctx context.Context, key domain.IssueKey, now time.Time) error
}

// IReactSlack sets and clears the reactions on the request message of a run.
type IReactSlack interface {
	React(ctx context.Context, channelID, ts, name string) error
	Unreact(ctx context.Context, channelID, ts, name string) error
}

// IPRWatchSlack is the part of the Slack poster that the PR watch uses.
type IPRWatchSlack interface {
	IStatusSlack
	IReactSlack
	IPRCardSlack
}

// IPRGit reads pull requests and deletes the branch of a merged one.
type IPRGit interface {
	View(ctx context.Context, prURL string) (gh.PullRequest, error)
	ReviewComments(ctx context.Context, ref gh.PRRef) ([]gh.ReviewComment, error)
	DeleteBranch(ctx context.Context, ref gh.PRRef, branch string) error
}

// IPRSessions is the part of the agterm client that the PR watch uses.
type IPRSessions interface {
	State(ctx context.Context, sessionID string) (agterm.SessionState, error)
	Type(ctx context.Context, sessionID, text string) error
	Close(ctx context.Context, sessionID string) error
}

// IPRAsks closes the asks of a run that ends.
type IPRAsks interface {
	CloseAllForRun(ctx context.Context, key domain.IssueKey) error
}

type PRWatchData struct {
	Registry config.Registry
	// Interval is the pause between two polls of the open PRs.
	Interval time.Duration
	Linear   IMirrorLinear
	// OwnerID is mentioned on the PR card, as when it was posted.
	OwnerID  string
	Slack    IPRWatchSlack
	Runs     IPRWatchRuns
	Asks     IPRAsks
	Sessions IPRSessions
	GH       IPRGit
	// Worktrees and RepoPath remove the worktree and the local branch of a merged repo.
	Worktrees IWorktrees
	RepoPath  func(repo string) string
	Guard     IRunGuard
	Now       func() time.Time
	// Sleep pauses between two polls; it returns the context's error when the context ends.
	Sleep func(ctx context.Context, d time.Duration) error
}

// PRWatch polls the PR of every active run. New feedback becomes one pointer line typed into the idle
// run session; a merged or closed PR ends the run. It is the one finish path of a merge, from Slack or from GitHub.
type PRWatch struct {
	d PRWatchData
}

func NewPRWatch(d PRWatchData) *PRWatch {
	return &PRWatch{d: d}
}

// Run polls every interval until the context ends; it returns the context's error then.
func (w *PRWatch) Run(ctx context.Context) error {
	for {
		if err := w.Tick(ctx); err != nil && ctx.Err() == nil {
			log.Error().Ctx(ctx).Err(err).Msg("PR watch failed")
		}
		if err := w.d.Sleep(ctx, w.d.Interval); err != nil {
			return err
		}
	}
}

// Tick polls the PRs of every active run once. A failing run does not stop the others.
func (w *PRWatch) Tick(ctx context.Context) error {
	runs, err := w.d.Runs.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("cannot list runs: %w", err)
	}
	var firstErr error
	for _, run := range runs {
		if err := w.watchRun(ctx, run); err != nil {
			log.Error().Ctx(ctx).Err(err).Msgf("PR watch of run %s failed", run.IssueKey)
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

func (w *PRWatch) watchRun(ctx context.Context, run domain.Run) error {
	repos, err := w.d.Runs.Repos(ctx, run.IssueKey)
	if err != nil {
		return fmt.Errorf("cannot read the repos of run %s: %w", run.IssueKey, err)
	}
	watched, stillOpen, merged := 0, false, false
	for _, repo := range repos {
		if repo.PRURL == "" {
			continue
		}
		watched++
		state := repo.Watch.State
		if state != prMerged && state != prClosed {
			if state, err = w.pollPR(ctx, run, repo); err != nil {
				return err
			}
		}
		stillOpen = stillOpen || (state != prMerged && state != prClosed)
		merged = merged || state == prMerged
	}
	if watched == 0 || stillOpen {
		return nil
	}
	return w.end(ctx, run, merged)
}

// pollPR reads one PR and returns its state. A merge or close is stored once, and a close is noted in the thread;
// a merge stores the PR's branch and merge commit too, and a pending deploy when the repo names a deploy job.
// The done line of the run tells the thread about a merge.
func (w *PRWatch) pollPR(ctx context.Context, run domain.Run, repo domain.RunRepo) (string, error) {
	ref, err := gh.ParsePRURL(repo.PRURL)
	if err != nil {
		return "", fmt.Errorf("cannot watch the PR of %s in run %s: %w", repo.Repo, run.IssueKey, err)
	}
	repo.PRURL = ref.URL()
	pr, err := w.d.GH.View(ctx, repo.PRURL)
	if err != nil {
		return "", fmt.Errorf("cannot read the PR of %s in run %s: %w", repo.Repo, run.IssueKey, err)
	}
	state := strings.ToLower(pr.State)
	if state != prMerged && state != prClosed {
		return prOpen, w.followUp(ctx, run, repo, ref, pr)
	}
	if state == prClosed {
		note := fmt.Sprintf("🚫 PR closed without a merge: %s", repo.PRURL)
		if err := w.d.Slack.Thread(ctx, run.ChannelID, run.ThreadTS, note); err != nil {
			return "", fmt.Errorf("cannot post to the thread of run %s: %w", run.IssueKey, err)
		}
		if err := w.d.Runs.SetLastPostAt(ctx, run.IssueKey, w.d.Now()); err != nil {
			return "", fmt.Errorf("cannot store the post time of run %s: %w", run.IssueKey, err)
		}
	}
	if state == prMerged {
		project, err := runProject(w.d.Registry, run)
		if err != nil {
			return "", err
		}
		merge := domain.Merge{Branch: pr.HeadRefName, SHA: pr.MergeCommit.OID, At: w.d.Now()}
		if r, ok := project.Repo(repo.Repo); ok && r.Deploy.Manual == "" {
			merge.DeployState = domain.DeployPending
		}
		if err := w.d.Runs.SetMerge(ctx, run.IssueKey, repo.Repo, merge, w.d.Now()); err != nil {
			return "", fmt.Errorf("cannot store the merge of %s in run %s: %w", repo.Repo, run.IssueKey, err)
		}
	}
	watch := domain.PRWatch{State: state, LastCommentID: repo.Watch.LastCommentID, CheckedAt: w.d.Now()}
	if err := w.d.Runs.SetPRWatch(ctx, run.IssueKey, repo.Repo, watch, w.d.Now()); err != nil {
		return "", fmt.Errorf("cannot store the PR state of %s in run %s: %w", repo.Repo, run.IssueKey, err)
	}
	cardState := prCardMerged
	if state == prClosed {
		cardState = prCardClosed
	}
	prCards{slack: w.d.Slack, ownerID: w.d.OwnerID}.redraw(ctx, run, repo, cardState)
	return state, nil
}

// followUp types the pointer line when the PR has feedback newer than the last one pointed at. The line goes
// only to a live session that does not work: the feedback waits for the next poll otherwise.
func (w *PRWatch) followUp(ctx context.Context, run domain.Run, repo domain.RunRepo, ref gh.PRRef, pr gh.PullRequest) error {
	inline, err := w.d.GH.ReviewComments(ctx, ref)
	if err != nil {
		return fmt.Errorf("cannot read the review comments of %s in run %s: %w", repo.Repo, run.IssueKey, err)
	}
	seen, _ := time.Parse(time.RFC3339, repo.Watch.LastCommentID)
	newest := newestFeedback(pr, inline, seen)
	watch := domain.PRWatch{State: prOpen, LastCommentID: repo.Watch.LastCommentID, CheckedAt: w.d.Now()}
	if !newest.IsZero() {
		state, err := w.d.Sessions.State(ctx, run.SessionID)
		if err != nil {
			log.Warn().Ctx(ctx).Err(err).Msgf("cannot read the session of run %s: PR feedback waits", run.IssueKey)
			return nil
		}
		if !state.Realized || state.Status == statusActive {
			return nil
		}
		line := fmt.Sprintf("PR feedback on %s: run the follow-up loop\n", repo.PRURL)
		if err := w.d.Sessions.Type(ctx, run.SessionID, line); err != nil {
			return fmt.Errorf("cannot type the PR feedback pointer into the session of run %s: %w", run.IssueKey, err)
		}
		watch.LastCommentID = newest.UTC().Format(time.RFC3339)
	}
	if err := w.d.Runs.SetPRWatch(ctx, run.IssueKey, repo.Repo, watch, w.d.Now()); err != nil {
		return fmt.Errorf("cannot store the PR watch of %s in run %s: %w", repo.Repo, run.IssueKey, err)
	}
	return nil
}

// newestFeedback is the time of the newest review, comment or failed check after since; zero when there is none.
func newestFeedback(pr gh.PullRequest, inline []gh.ReviewComment, since time.Time) time.Time {
	var newest time.Time
	note := func(t time.Time) {
		if t.After(since) && t.After(newest) {
			newest = t
		}
	}
	for _, r := range pr.Reviews {
		if r.State == reviewChangesRequested || r.State == reviewCommented {
			note(r.SubmittedAt)
		}
	}
	for _, c := range pr.Comments {
		note(c.CreatedAt)
	}
	for _, c := range inline {
		note(c.CreatedAt)
	}
	for _, c := range pr.Checks {
		if c.Failed() {
			note(c.At())
		}
	}
	return newest
}

// end finishes a run whose PRs are all merged or closed. A merge whose repos a person deploys moves Linear to done
// unless it is done already; a merge with a pending deploy leaves Linear, the phase and the working reaction to the
// deploy watch. The Linear write and the end of the run happen under the run lock, after a recheck, so a cancel
// cannot flip the status in between. Every step can run again after a failure: the run ends last.
func (w *PRWatch) end(ctx context.Context, run domain.Run, merged bool) error {
	// CloseAllForRun takes the run lock itself, so the lock below starts after it
	if err := w.d.Asks.CloseAllForRun(ctx, run.IssueKey); err != nil {
		return fmt.Errorf("cannot close the asks of run %s: %w", run.IssueKey, err)
	}
	repos, err := w.d.Runs.Repos(ctx, run.IssueKey)
	if err != nil {
		return fmt.Errorf("cannot read the repos of run %s: %w", run.IssueKey, err)
	}
	deploying := false
	for _, repo := range repos {
		deploying = deploying || repo.Merge.DeployState.Watched()
	}
	ended, err := w.endLocked(ctx, run, merged && !deploying)
	if err != nil || !ended {
		return err
	}
	w.closeSessions(ctx, run)
	switch {
	case !merged:
		clearWorking(ctx, w.d.Slack, run)
	case deploying:
		w.removeBranches(ctx, run, repos)
	default:
		w.removeBranches(ctx, run, repos)
		w.finishByHand(ctx, run, repos)
	}
	return nil
}

// finishByHand tells the thread and the request that the run is done and how a person deploys it. The run has
// ended, so a failure is only logged.
func (w *PRWatch) finishByHand(ctx context.Context, run domain.Run, repos []domain.RunRepo) {
	line := fmt.Sprintf("✅ %s done.", run.IssueKey)
	project, err := runProject(w.d.Registry, run)
	if err != nil {
		log.Warn().Ctx(ctx).Err(err).Msgf("cannot read how run %s deploys", run.IssueKey)
	} else if manual := handDeploys(project, repos); manual != "" {
		line = fmt.Sprintf("✅ %s done. Deploy it by hand: %s.", run.IssueKey, manual)
	}
	if err := w.d.Slack.Thread(ctx, run.ChannelID, run.ThreadTS, line); err != nil {
		log.Warn().Ctx(ctx).Err(err).Msgf("cannot post the done line of run %s", run.IssueKey)
	}
	clearWorking(ctx, w.d.Slack, run)
	if err := w.d.Slack.React(ctx, run.ChannelID, run.RequestTS, slackpost.ReactionDone); err != nil {
		log.Warn().Ctx(ctx).Err(err).Msgf("cannot add the done reaction of run %s", run.IssueKey)
	}
}

// removeBranches removes the branch of every merged repo: the worktree, the local branch and the branch on GitHub.
// A closed PR keeps its branch. The run has ended, so every failure is only logged and the next repo still runs.
func (w *PRWatch) removeBranches(ctx context.Context, run domain.Run, repos []domain.RunRepo) {
	for _, repo := range repos {
		if repo.Watch.State != prMerged || repo.Merge.Branch == "" {
			continue
		}
		if run.WorktreeBase != "" {
			dir := filepath.Join(run.WorktreeBase, repo.Repo)
			if err := w.d.Worktrees.RemoveWorktree(ctx, w.d.RepoPath(repo.Repo), dir, repo.Merge.Branch, true); err != nil {
				log.Warn().Ctx(ctx).Err(err).Msgf("cannot remove the worktree of %s in run %s", repo.Repo, run.IssueKey)
			}
		}
		ref, err := gh.ParsePRURL(repo.PRURL)
		if err != nil {
			log.Warn().Ctx(ctx).Err(err).Msgf("cannot delete the branch of %s in run %s", repo.Repo, run.IssueKey)
			continue
		}
		if err := w.d.GH.DeleteBranch(ctx, ref, repo.Merge.Branch); err != nil {
			log.Warn().Ctx(ctx).Err(err).Msgf("cannot delete branch %s of %s in run %s", repo.Merge.Branch, repo.Repo, run.IssueKey)
		}
	}
}

// endLocked reports false when the run had ended already, for example by a cancel: nothing is written then.
// done moves Linear and the phase to done.
func (w *PRWatch) endLocked(ctx context.Context, run domain.Run, done bool) (bool, error) {
	defer w.d.Guard.LockRun(run.IssueKey)()
	current, err := w.d.Runs.FindByKey(ctx, run.IssueKey)
	if err != nil {
		return false, fmt.Errorf("cannot find run %s: %w", run.IssueKey, err)
	}
	if !current.EndedAt.IsZero() {
		return false, nil
	}
	if done {
		if err := (linearMirror{linear: w.d.Linear, registry: w.d.Registry}).setDone(ctx, run); err != nil {
			return false, err
		}
		if current.Phase != donePhase {
			if err := w.d.Runs.SetPhase(ctx, run.IssueKey, donePhase, w.d.Now()); err != nil {
				return false, fmt.Errorf("cannot set phase of run %s: %w", run.IssueKey, err)
			}
		}
	}
	if err := w.d.Runs.End(ctx, run.IssueKey, w.d.Now()); err != nil {
		return false, fmt.Errorf("cannot end run %s: %w", run.IssueKey, err)
	}
	return true, nil
}

// closeSessions closes the sessions of the run that ended.
func (w *PRWatch) closeSessions(ctx context.Context, run domain.Run) {
	closeRunSessions(ctx, w.d.Sessions, run)
}

// clearWorking takes the working reaction off the request of a run that ended. The run has ended, so a failure is
// only logged.
func clearWorking(ctx context.Context, slack IReactSlack, run domain.Run) {
	if err := slack.Unreact(ctx, run.ChannelID, run.RequestTS, slackpost.ReactionWorking); err != nil {
		log.Warn().Ctx(ctx).Err(err).Msgf("cannot remove the working reaction of run %s", run.IssueKey)
	}
}

// ISessionCloser closes agterm sessions.
type ISessionCloser interface {
	Close(ctx context.Context, sessionID string) error
}

// closeRunSessions closes the part sessions of the run, then the run session. The run has ended, so a session
// that is already gone only gets a warning.
func closeRunSessions(ctx context.Context, sessions ISessionCloser, run domain.Run) {
	var ids []string
	state, err := engine.LoadState(run.WorktreeBase)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		log.Warn().Ctx(ctx).Err(err).Msgf("cannot read the part sessions of run %s", run.IssueKey)
	}
	for _, wave := range state.Waves {
		for _, part := range wave.Parts {
			if part.SessionID != "" {
				ids = append(ids, part.SessionID)
			}
		}
	}
	if run.SessionID != "" {
		ids = append(ids, run.SessionID)
	}
	for _, id := range ids {
		if err := sessions.Close(ctx, id); err != nil {
			log.Warn().Ctx(ctx).Err(err).Msgf("cannot close session %s of run %s", id, run.IssueKey)
		}
	}
}
