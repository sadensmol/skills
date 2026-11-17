package usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/rs/zerolog/log"

	controllerslack "github.com/sadensmol/autopilot/internal/controller/slack"
	"github.com/sadensmol/autopilot/internal/domain"
	"github.com/sadensmol/autopilot/internal/gh"
	"github.com/sadensmol/autopilot/internal/slackpost"
)

// IPRMergeGit merges pull requests.
type IPRMergeGit interface {
	MergeMethod(ctx context.Context, ref gh.PRRef) (string, error)
	Merge(ctx context.Context, prURL, method string) error
}

// IPRMergeRuns is the part of the run store that a merge uses.
type IPRMergeRuns interface {
	FindByKey(ctx context.Context, key domain.IssueKey) (domain.Run, error)
	Repos(ctx context.Context, key domain.IssueKey) ([]domain.RunRepo, error)
	SetLastPostAt(ctx context.Context, key domain.IssueKey, at time.Time) error
}

// IPRMergeSlack is the part of the Slack poster that a merge uses.
type IPRMergeSlack interface {
	IStatusSlack
	IPRCardSlack
}

type PRMergeData struct {
	GH IPRMergeGit
	// OwnerID is mentioned on the PR card, as when it was posted.
	OwnerID string
	Slack   IPRMergeSlack
	Runs    IPRMergeRuns
	Now     func() time.Time
}

// PRMerge merges a run's PR on the owner's click. It only merges: the PR watch sees the merge and finishes the run,
// as it does for a merge on GitHub.
type PRMerge struct {
	d PRMergeData
}

func NewPRMerge(d PRMergeData) *PRMerge {
	return &PRMerge{d: d}
}

// Merge takes a click on the Merge button of a PR card. The click counts only when the run of the issue key is
// active, lives in the channel of the card and has a PR with that URL; any other click changes nothing.
// A counted click shows Merging… on the card in place of its buttons. A failed merge brings the buttons back and is
// told in the thread.
func (m *PRMerge) Merge(ctx context.Context, click controllerslack.MergeClick) error {
	run, err := m.d.Runs.FindByKey(ctx, domain.IssueKey(click.IssueKey))
	if errors.Is(err, domain.ErrNotFound) {
		log.Info().Ctx(ctx).Msgf("ignored a merge of unknown run %s", click.IssueKey)
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot find run %s: %w", click.IssueKey, err)
	}
	if !run.EndedAt.IsZero() || run.ChannelID != click.ChannelID {
		log.Info().Ctx(ctx).Msgf("ignored a merge of run %s from channel %s", click.IssueKey, click.ChannelID)
		return nil
	}
	repos, err := m.d.Runs.Repos(ctx, run.IssueKey)
	if err != nil {
		return fmt.Errorf("cannot read the repos of run %s: %w", run.IssueKey, err)
	}
	i := slices.IndexFunc(repos, func(r domain.RunRepo) bool { return r.PRURL == click.PRURL })
	if i < 0 {
		log.Info().Ctx(ctx).Msgf("ignored a merge of %s: not a PR of run %s", click.PRURL, run.IssueKey)
		return nil
	}
	repo := repos[i]
	repo.PRCard.TS = click.MessageTS
	cards := prCards{slack: m.d.Slack, ownerID: m.d.OwnerID}
	cards.redraw(ctx, run, repo, prCardMerging)
	ref, err := gh.ParsePRURL(click.PRURL)
	if err != nil {
		return fmt.Errorf("cannot merge the PR of run %s: %w", run.IssueKey, err)
	}
	method, err := m.d.GH.MergeMethod(ctx, ref)
	if err == nil {
		err = m.d.GH.Merge(ctx, click.PRURL, method)
	}
	if err == nil {
		return nil
	}
	cards.redraw(ctx, run, repo, "")
	log.Warn().Ctx(ctx).Err(err).Msgf("cannot merge %s of run %s", click.PRURL, run.IssueKey)
	if err := m.d.Slack.Thread(ctx, run.ChannelID, run.ThreadTS, slackpost.Escape(fmt.Sprintf("could not merge %s: %s", click.PRURL, err))); err != nil {
		return fmt.Errorf("cannot post to the thread of run %s: %w", run.IssueKey, err)
	}
	if err := m.d.Runs.SetLastPostAt(ctx, run.IssueKey, m.d.Now()); err != nil {
		return fmt.Errorf("cannot store the post time of run %s: %w", run.IssueKey, err)
	}
	return nil
}
