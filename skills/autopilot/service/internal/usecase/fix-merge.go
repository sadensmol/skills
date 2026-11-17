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
)

const (
	fixChecksInterval = 30 * time.Second
	fixChecksLimit    = 30 * time.Minute
	prStateOpen       = "OPEN"
)

// IFixMergeGit reads a fix PR and merges it.
type IFixMergeGit interface {
	View(ctx context.Context, prURL string) (gh.PullRequest, error)
	MergeMethod(ctx context.Context, ref gh.PRRef) (string, error)
	Merge(ctx context.Context, prURL, method string) error
}

type FixMergeData struct {
	Registry config.Registry
	GH       IFixMergeGit
	Slack    IStatusSlack
	Runs     IPRMergeRuns
	Now      func() time.Time
	// Sleep pauses between two polls; it returns the context's error when the context ends.
	Sleep func(ctx context.Context, d time.Duration) error
}

// FixMerge merges the control room's fix of a broken default branch. The owner's Merge click on the run's PR
// approves every fix PR that makes that merge deploy, so the owner is never asked again.
type FixMerge struct {
	d FixMergeData
}

func NewFixMerge(d FixMergeData) *FixMerge {
	return &FixMerge{d: d}
}

// Merge waits for the checks of a fix PR and merges it when they are green. The PR must be open, target the default
// branch of a repo of the run whose deploy failed, and live in that repo on github.com. A failed check, a PR that
// is no longer open and checks that are not green within 30 minutes end the wait with an error. The deploy state
// stays as it is: the deploy watch follows the head of the default branch.
func (m *FixMerge) Merge(ctx context.Context, key domain.IssueKey, prURL string) error {
	ref, err := gh.ParsePRURL(prURL)
	if err != nil {
		return fmt.Errorf("cannot take the fix PR: %w", err)
	}
	run, err := m.d.Runs.FindByKey(ctx, key)
	if err != nil {
		return fmt.Errorf("cannot find run %s: %w", key, err)
	}
	branch, err := m.defaultBranch(ctx, run, ref)
	if err != nil {
		return err
	}
	if err := m.waitGreen(ctx, ref.URL(), branch); err != nil {
		return err
	}
	method, err := m.d.GH.MergeMethod(ctx, ref)
	if err != nil {
		return fmt.Errorf("cannot pick the merge method of %s: %w", ref.URL(), err)
	}
	if err := m.d.GH.Merge(ctx, ref.URL(), method); err != nil {
		return fmt.Errorf("cannot merge %s: %w", ref.URL(), err)
	}
	log.Info().Ctx(ctx).Msgf("merged the fix %s of the default branch %s for run %s", ref.URL(), branch, run.IssueKey)
	line := fmt.Sprintf("🔧 %s: merged a fix for the default branch: %s", run.IssueKey, ref.URL())
	if err := m.d.Slack.Thread(ctx, run.ChannelID, run.ThreadTS, line); err != nil {
		log.Error().Ctx(ctx).Err(err).Msgf("cannot post the merge of fix %s to the thread of run %s", ref.URL(), run.IssueKey)
		return nil
	}
	if err := m.d.Runs.SetLastPostAt(ctx, run.IssueKey, m.d.Now()); err != nil {
		log.Error().Ctx(ctx).Err(err).Msgf("cannot store the post time of run %s", run.IssueKey)
	}
	return nil
}

// defaultBranch finds the repo of the run that the fix PR belongs to: a repo whose deploy failed and whose PR lives
// in the same GitHub repo. It returns the default branch of that repo.
func (m *FixMerge) defaultBranch(ctx context.Context, run domain.Run, ref gh.PRRef) (string, error) {
	project, err := runProject(m.d.Registry, run)
	if err != nil {
		return "", err
	}
	repos, err := m.d.Runs.Repos(ctx, run.IssueKey)
	if err != nil {
		return "", fmt.Errorf("cannot read the repos of run %s: %w", run.IssueKey, err)
	}
	var failing []string
	for _, r := range repos {
		if !r.Merge.Failing() {
			continue
		}
		own, err := gh.ParsePRURL(r.PRURL)
		if err != nil {
			return "", fmt.Errorf("cannot read the PR of %s in run %s: %w", r.Repo, run.IssueKey, err)
		}
		failing = append(failing, fmt.Sprintf("%s/%s", own.Owner, own.Repo))
		if !strings.EqualFold(own.Owner, ref.Owner) || !strings.EqualFold(own.Repo, ref.Repo) {
			continue
		}
		repo, ok := project.Repo(r.Repo)
		if !ok {
			return "", fmt.Errorf("repo %s of run %s is not in the workspace registry", r.Repo, run.IssueKey)
		}
		return repo.DefaultBranch, nil
	}
	if len(failing) == 0 {
		return "", fmt.Errorf("run %s has no repo with a failed deploy", run.IssueKey)
	}
	return "", fmt.Errorf("%s is not a PR of a repo of run %s with a failed deploy (%s)", ref.URL(), run.IssueKey, strings.Join(failing, ", "))
}

// waitGreen polls the PR until every check passed. A PR with no checks yet waits for its CI to report.
func (m *FixMerge) waitGreen(ctx context.Context, prURL, branch string) error {
	deadline := m.d.Now().Add(fixChecksLimit)
	for {
		pr, err := m.d.GH.View(ctx, prURL)
		if err != nil {
			return fmt.Errorf("cannot read %s: %w", prURL, err)
		}
		if !strings.EqualFold(pr.State, prStateOpen) {
			return fmt.Errorf("%s is %s, not open", prURL, strings.ToLower(pr.State))
		}
		if pr.BaseRefName != branch {
			return fmt.Errorf("%s targets %s, not the default branch %s", prURL, pr.BaseRefName, branch)
		}
		var failed []string
		pending := len(pr.Checks) == 0
		for _, c := range pr.Checks {
			switch {
			case c.Pending():
				pending = true
			case !c.Passed():
				failed = append(failed, fmt.Sprintf("%s (%s)", c.Label(), c.Link()))
			}
		}
		if len(failed) > 0 {
			return fmt.Errorf("checks failed on %s: %s", prURL, strings.Join(failed, ", "))
		}
		if !pending {
			return nil
		}
		if !m.d.Now().Before(deadline) {
			return fmt.Errorf("the checks of %s are not green after %d minutes", prURL, int(fixChecksLimit.Minutes()))
		}
		if err := m.d.Sleep(ctx, fixChecksInterval); err != nil {
			return err
		}
	}
}
