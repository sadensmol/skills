package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/sadensmol/autopilot/internal/config"
	"github.com/sadensmol/autopilot/internal/domain"
	"github.com/sadensmol/autopilot/internal/gh"
	"github.com/sadensmol/autopilot/internal/slackpost"
)

// IPROpenRuns is the part of the run store that a PR registration uses.
type IPROpenRuns interface {
	FindByKey(ctx context.Context, key domain.IssueKey) (domain.Run, error)
	SetPR(ctx context.Context, key domain.IssueKey, repo, prURL string, now time.Time) error
	SetPRWatch(ctx context.Context, key domain.IssueKey, repo string, w domain.PRWatch, now time.Time) error
	SetPRCard(ctx context.Context, key domain.IssueKey, repo string, card domain.PRCard, now time.Time) error
	SetLastPostAt(ctx context.Context, key domain.IssueKey, at time.Time) error
}

// IPROpenVerdicts reads the gate verdicts.
type IPROpenVerdicts interface {
	Latest(ctx context.Context, key domain.IssueKey, repo string) (domain.Verdict, error)
}

// IPROpenGit reads the creation time of a pull request.
type IPROpenGit interface {
	View(ctx context.Context, prURL string) (gh.PullRequest, error)
}

// IPROpenSlack is the part of the Slack poster that a PR registration uses.
type IPROpenSlack interface {
	Mention(ctx context.Context, channelID, threadTS, userID, text string) error
	PRCard(ctx context.Context, channelID, threadTS string, c slackpost.PRCard) (string, error)
}

type PROpenData struct {
	// Registry names the repos a PR may belong to.
	Registry config.Registry
	GH       IPROpenGit
	OwnerID  string
	Slack    IPROpenSlack
	Runs     IPROpenRuns
	Verdicts IPROpenVerdicts
	Guard    IRunGuard
	Now      func() time.Time
}

// PROpen records the PR of a run and hands it to the owner with the manual checks of the latest verdict.
type PROpen struct {
	d PROpenData
}

func NewPROpen(d PROpenData) *PROpen {
	return &PROpen{d: d}
}

// Open stores the PR for watching and posts it to the thread with an @mention of the owner, its manual checks,
// a Review link and a Merge button. The URL must be a plain github.com pull request of a repo of the run's product; only its canonical
// form is stored. The watch counts feedback from the creation time of the PR on, so a check that failed before this call is not missed.
// The repo of the PR is the repo in its URL; it must be named like the product's repo in the registry.
func (p *PROpen) Open(ctx context.Context, key domain.IssueKey, rawURL string) error {
	ref, err := gh.ParsePRURL(rawURL)
	if err != nil {
		return err
	}
	prURL := ref.URL()
	defer p.d.Guard.LockRun(key)()
	run, err := p.activeRun(ctx, key)
	if err != nil {
		return err
	}
	project, err := runProject(p.d.Registry, run)
	if err != nil {
		return err
	}
	if !project.HasRepo(ref.Repo) {
		return fmt.Errorf("repo %s is not a repo of product %s", ref.Repo, run.Product)
	}
	pr, err := p.d.GH.View(ctx, prURL)
	if err != nil {
		return fmt.Errorf("cannot read the PR %s: %w", prURL, err)
	}
	now := p.d.Now()
	if err := p.d.Runs.SetPR(ctx, key, ref.Repo, prURL, now); err != nil {
		return err
	}
	watch := domain.PRWatch{State: prOpen, LastCommentID: pr.CreatedAt.UTC().Format(time.RFC3339), CheckedAt: now}
	if err := p.d.Runs.SetPRWatch(ctx, key, ref.Repo, watch, now); err != nil {
		return err
	}
	verdict, err := p.d.Verdicts.Latest(ctx, key, ref.Repo)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("cannot read the verdict of %s in run %s: %w", ref.Repo, key, err)
	}
	card := slackpost.PRCard{IssueKey: string(key), PRURL: prURL, UserID: p.d.OwnerID, Text: prOpenText(prURL, verdict)}
	ts, err := p.d.Slack.PRCard(ctx, run.ChannelID, run.ThreadTS, card)
	if err != nil {
		return fmt.Errorf("cannot post to the thread of run %s: %w", run.IssueKey, err)
	}
	// the card is posted: a second call would post it again, and without the stored card only its redraw is lost
	if err := p.d.Runs.SetPRCard(ctx, key, ref.Repo, domain.PRCard{TS: ts, Text: card.Text}, p.d.Now()); err != nil {
		log.Error().Ctx(ctx).Err(err).Msgf("cannot store the PR card of %s in run %s", prURL, key)
	}
	if err := p.d.Runs.SetLastPostAt(ctx, run.IssueKey, p.d.Now()); err != nil {
		return fmt.Errorf("cannot store the post time of run %s: %w", run.IssueKey, err)
	}
	return nil
}

// Manual posts a manual checklist to the thread with an @mention of the owner.
func (p *PROpen) Manual(ctx context.Context, key domain.IssueKey, checklist string) error {
	defer p.d.Guard.LockRun(key)()
	run, err := p.activeRun(ctx, key)
	if err != nil {
		return err
	}
	return p.mention(ctx, run, fmt.Sprintf("Manual checks before you merge:\n%s", checklist))
}

func (p *PROpen) activeRun(ctx context.Context, key domain.IssueKey) (domain.Run, error) {
	run, err := p.d.Runs.FindByKey(ctx, key)
	if err != nil {
		return domain.Run{}, fmt.Errorf("cannot find run %s: %w", key, err)
	}
	if !run.EndedAt.IsZero() {
		return domain.Run{}, fmt.Errorf("run %s has ended", key)
	}
	return run, nil
}

func (p *PROpen) mention(ctx context.Context, run domain.Run, text string) error {
	if err := p.d.Slack.Mention(ctx, run.ChannelID, run.ThreadTS, p.d.OwnerID, text); err != nil {
		return fmt.Errorf("cannot post to the thread of run %s: %w", run.IssueKey, err)
	}
	if err := p.d.Runs.SetLastPostAt(ctx, run.IssueKey, p.d.Now()); err != nil {
		return fmt.Errorf("cannot store the post time of run %s: %w", run.IssueKey, err)
	}
	return nil
}

func prOpenText(prURL string, verdict domain.Verdict) string {
	var b strings.Builder
	fmt.Fprintf(&b, "✅ PR ready: %s", prURL)
	if len(verdict.Manual) > 0 {
		b.WriteString("\nCheck by hand before you merge:")
		for _, m := range verdict.Manual {
			fmt.Fprintf(&b, "\n• %s", m)
		}
	}
	return b.String()
}
