package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/sadensmol/autopilot/internal/config"
	controllerslack "github.com/sadensmol/autopilot/internal/controller/slack"
	"github.com/sadensmol/autopilot/internal/domain"
	"github.com/sadensmol/autopilot/internal/linear"
	"github.com/sadensmol/autopilot/internal/slackpost"
)

const canceledCardState = "canceled"

// ICancelLinear is the part of the Linear client that a cancel uses.
type ICancelLinear interface {
	IMirrorLinear
	Comment(ctx context.Context, issueID, body string) error
}

// ICancelSlack is the part of the Slack poster that a cancel uses.
type ICancelSlack interface {
	IStatusSlack
	IReactSlack
	UpdateCard(ctx context.Context, channelID, ts string, c slackpost.Card) error
}

// ICancelRuns is the part of the run store that a cancel uses.
type ICancelRuns interface {
	FindByKey(ctx context.Context, key domain.IssueKey) (domain.Run, error)
	SetPhase(ctx context.Context, key domain.IssueKey, phase domain.Phase, now time.Time) error
	SetLastPostAt(ctx context.Context, key domain.IssueKey, at time.Time) error
	End(ctx context.Context, key domain.IssueKey, now time.Time) error
}

type CancellerData struct {
	Registry config.Registry
	Linear   ICancelLinear
	Slack    ICancelSlack
	Runs     ICancelRuns
	Asks     IPRAsks
	Sessions ISessionCloser
	Guard    IRunGuard
	Now      func() time.Time
}

// Canceller ends a run on the owner's request. The worktree stays: a cancel never deletes work.
type Canceller struct {
	d CancellerData
}

func NewCanceller(d CancellerData) *Canceller {
	return &Canceller{d: d}
}

// Cancel takes a click on a card's Cancel button. The click counts only when the run of the issue key lives in
// the channel of the card; any other click changes nothing.
func (c *Canceller) Cancel(ctx context.Context, click controllerslack.CancelClick) error {
	run, err := c.d.Runs.FindByKey(ctx, domain.IssueKey(click.IssueKey))
	if errors.Is(err, domain.ErrNotFound) {
		log.Info().Ctx(ctx).Msgf("ignored a cancel of unknown run %s", click.IssueKey)
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot find run %s: %w", click.IssueKey, err)
	}
	if run.ChannelID != click.ChannelID {
		log.Info().Ctx(ctx).Msgf("ignored a cancel of run %s from channel %s", click.IssueKey, click.ChannelID)
		return nil
	}
	return c.cancel(ctx, run)
}

// CancelRun cancels a run on the owner's verb; the socket is private to the owner, so no channel is checked.
func (c *Canceller) CancelRun(ctx context.Context, key domain.IssueKey) error {
	run, err := c.d.Runs.FindByKey(ctx, key)
	if err != nil {
		return fmt.Errorf("cannot find run %s: %w", key, err)
	}
	return c.cancel(ctx, run)
}

// cancel closes the asks, then holds the run lock while it comments and sets Linear to canceled and ends the run,
// so no other writer (a PR merge, a status move, an ask) moves Linear in between. The sessions close and the card
// redraws after the lock is free. Every step can run again after a failure: the run ends before the courtesy steps.
func (c *Canceller) cancel(ctx context.Context, run domain.Run) error {
	if !run.EndedAt.IsZero() {
		return nil
	}
	// CloseAllForRun takes the run lock itself, so the lock below starts after it
	if err := c.d.Asks.CloseAllForRun(ctx, run.IssueKey); err != nil {
		return fmt.Errorf("cannot close the asks of run %s: %w", run.IssueKey, err)
	}
	issue, ended, err := c.endLocked(ctx, run)
	if err != nil || !ended {
		return err
	}
	closeRunSessions(ctx, c.d.Sessions, run)
	clearWorking(ctx, c.d.Slack, run)
	c.redrawCard(ctx, run, issue.Title, issue.URL)
	return nil
}

// endLocked does every write of the cancel under the run lock. It reports false when the run had ended already:
// a second cancel, or a PR merge that won, writes nothing.
func (c *Canceller) endLocked(ctx context.Context, run domain.Run) (linear.Issue, bool, error) {
	defer c.d.Guard.LockRun(run.IssueKey)()
	current, err := c.d.Runs.FindByKey(ctx, run.IssueKey)
	if err != nil {
		return linear.Issue{}, false, fmt.Errorf("cannot find run %s: %w", run.IssueKey, err)
	}
	if !current.EndedAt.IsZero() {
		return linear.Issue{}, false, nil
	}
	issue, err := c.d.Linear.IssueByKey(ctx, string(run.IssueKey))
	if err != nil {
		return linear.Issue{}, false, fmt.Errorf("cannot read issue %s: %w", run.IssueKey, err)
	}
	if issue.State != c.d.Registry.Linear.Statuses[canceledPhase] {
		if err := c.d.Linear.Comment(ctx, issue.ID, "Canceled by the owner from Slack. The worktree is kept."); err != nil {
			return linear.Issue{}, false, fmt.Errorf("cannot comment on issue %s: %w", run.IssueKey, err)
		}
		mirror := linearMirror{linear: c.d.Linear, registry: c.d.Registry}
		if err := mirror.set(ctx, run, canceledPhase); err != nil {
			return linear.Issue{}, false, fmt.Errorf("cannot set the linear status of run %s to canceled: %w", run.IssueKey, err)
		}
	}
	if err := c.d.Slack.Thread(ctx, run.ChannelID, run.ThreadTS, fmt.Sprintf("🚫 %s canceled.", run.IssueKey)); err != nil {
		return linear.Issue{}, false, fmt.Errorf("cannot post to the thread of run %s: %w", run.IssueKey, err)
	}
	if err := c.d.Runs.SetLastPostAt(ctx, run.IssueKey, c.d.Now()); err != nil {
		return linear.Issue{}, false, fmt.Errorf("cannot store the post time of run %s: %w", run.IssueKey, err)
	}
	if err := c.d.Runs.SetPhase(ctx, run.IssueKey, canceledPhase, c.d.Now()); err != nil {
		return linear.Issue{}, false, fmt.Errorf("cannot set phase of run %s: %w", run.IssueKey, err)
	}
	if err := c.d.Runs.End(ctx, run.IssueKey, c.d.Now()); err != nil {
		return linear.Issue{}, false, fmt.Errorf("cannot end run %s: %w", run.IssueKey, err)
	}
	return issue, true, nil
}

// redrawCard shows the final state on the card and drops its Cancel button. The run has ended, so a failure is logged.
func (c *Canceller) redrawCard(ctx context.Context, run domain.Run, title, url string) {
	if run.CardTS == "" {
		return
	}
	card := slackpost.Card{IssueKey: string(run.IssueKey), Title: title, URL: url, State: canceledCardState}
	if err := c.d.Slack.UpdateCard(ctx, run.ChannelID, run.CardTS, card); err != nil {
		log.Error().Ctx(ctx).Err(err).Msgf("cannot update the card of run %s", run.IssueKey)
	}
}
