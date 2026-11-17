package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/sadensmol/autopilot/internal/agterm"
	"github.com/sadensmol/autopilot/internal/config"
	controllerslack "github.com/sadensmol/autopilot/internal/controller/slack"
	"github.com/sadensmol/autopilot/internal/domain"
	"github.com/sadensmol/autopilot/internal/linear"
	"github.com/sadensmol/autopilot/internal/repository"
	"github.com/sadensmol/autopilot/internal/slackpost"
)

const (
	// askWaitLimit keeps one tool call of an agent under the 10 minute limit of its harness.
	askWaitLimit    = 8 * time.Minute
	askPollInterval = 2 * time.Second
	blockedPhase    = "blocked"
	statusBlocked   = "blocked"
	statusActive    = "active"
	dialogTitleHead = "Question from"
)

// IAskLinear is the part of the Linear client that asks use.
type IAskLinear interface {
	IssueByKey(ctx context.Context, key string) (linear.Issue, error)
	TeamByKey(ctx context.Context, key string) (linear.Team, error)
	WorkflowStates(ctx context.Context, teamID string) ([]linear.State, error)
	SetState(ctx context.Context, issueID, stateID string) error
}

// IAskSlack is the part of the Slack poster that asks use.
type IAskSlack interface {
	Mention(ctx context.Context, channelID, threadTS, userID, text string) error
	React(ctx context.Context, channelID, ts, name string) error
}

// IAskRuns is the part of the run store that asks use.
type IAskRuns interface {
	FindByKey(ctx context.Context, key domain.IssueKey) (domain.Run, error)
	FindByRequest(ctx context.Context, channelID, requestTS string) (domain.Run, error)
}

// IAskStore is the part of the ask store that asks use.
type IAskStore interface {
	Create(ctx context.Context, d repository.CreateAskData) (domain.Ask, error)
	Find(ctx context.Context, id domain.AskID) (domain.Ask, error)
	FindOpen(ctx context.Context, key domain.IssueKey) (domain.Ask, error)
	SetDialog(ctx context.Context, id domain.AskID, dialogID string) error
	Answer(ctx context.Context, id domain.AskID, answer domain.AskAnswer, now time.Time) error
	Cancel(ctx context.Context, id domain.AskID, now time.Time) error
}

// IAskSessions is the part of the agterm client that asks use.
type IAskSessions interface {
	OpenAsk(ctx context.Context, d agterm.AskOpenData) (string, error)
	AskResult(ctx context.Context, dialogID string) (agterm.AskResult, error)
	CancelAsk(ctx context.Context, dialogID string) error
	SetStatus(ctx context.Context, sessionID, state string, blink bool) error
}

type AskData struct {
	OwnerID  string
	Registry config.Registry
	Linear   IAskLinear
	Slack    IAskSlack
	Runs     IAskRuns
	Asks     IAskStore
	Sessions IAskSessions
	Now      func() time.Time
	// Sleep pauses between two polls; it returns the context's error when the context ends.
	Sleep func(ctx context.Context, d time.Duration) error
}

// Ask puts a worker's question to the owner in Slack and in agterm; the first answer wins.
// A run has one open ask; later asks queue and open in order when the open one closes.
// An agterm answer is seen only while a caller waits on the ask, because agterm emits no event for it.
type Ask struct {
	d AskData
	// locks holds one mutex per run: opening, closing and presenting asks of a run never overlap,
	// so the ask the store opens behind a closed one is shown before anyone can answer it.
	locks sync.Map
}

// lockRun takes the run's mutex and returns the unlock function.
func (a *Ask) lockRun(key domain.IssueKey) func() {
	m, _ := a.locks.LoadOrStore(key, &sync.Mutex{})
	mu := m.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// LockRun takes the run's mutex and returns the unlock function. Status moves take it too,
// so a phase change never overlaps an ask opening or closing on the same run.
func (a *Ask) LockRun(key domain.IssueKey) func() {
	return a.lockRun(key)
}

func NewAsk(d AskData) *Ask {
	return &Ask{d: d}
}

// Open stores the ask. When the run has no open ask, it shows the ask at once and blocks the run.
func (a *Ask) Open(ctx context.Context, key domain.IssueKey, question string, options []domain.AskOption) (domain.Ask, error) {
	run, err := a.d.Runs.FindByKey(ctx, key)
	if err != nil {
		return domain.Ask{}, fmt.Errorf("cannot find run %s: %w", key, err)
	}
	if !run.EndedAt.IsZero() {
		return domain.Ask{}, fmt.Errorf("run %s has ended", key)
	}
	defer a.lockRun(key)()
	ask, err := a.d.Asks.Create(ctx, repository.CreateAskData{IssueKey: key, Question: question, Options: options, Now: a.d.Now()})
	if err != nil {
		return domain.Ask{}, err
	}
	if ask.Status != domain.AskOpen {
		return ask, nil
	}
	if err := a.present(ctx, run, &ask); err != nil {
		if cancelErr := a.d.Asks.Cancel(ctx, ask.ID, a.d.Now()); cancelErr != nil {
			log.Error().Ctx(ctx).Err(cancelErr).Msgf("cannot cancel ask %d after its dialog failed", ask.ID)
		}
		return domain.Ask{}, fmt.Errorf("cannot show ask %d: %w", ask.ID, err)
	}
	a.setLinearStatus(ctx, run, blockedPhase)
	return ask, nil
}

// present opens the dialog, marks the session blocked and posts the question in the thread.
func (a *Ask) present(ctx context.Context, run domain.Run, ask *domain.Ask) error {
	choices := make([]agterm.AskChoice, 0, len(ask.Options))
	for _, o := range ask.Options {
		choices = append(choices, agterm.AskChoice{ID: o.ID, Label: o.Label})
	}
	dialogID, err := a.d.Sessions.OpenAsk(ctx, agterm.AskOpenData{
		Target:  run.SessionID,
		Title:   fmt.Sprintf("%s %s", dialogTitleHead, run.IssueKey),
		Message: strings.Join(strings.Fields(ask.Question), " "),
		Choices: choices,
	})
	if err != nil {
		return err
	}
	if err := a.d.Asks.SetDialog(ctx, ask.ID, dialogID); err != nil {
		return err
	}
	ask.DialogID = dialogID
	// the dialog is up and the answer can come from it, so a failed marker or post is logged, not returned
	if err := a.d.Sessions.SetStatus(ctx, run.SessionID, statusBlocked, true); err != nil {
		log.Error().Ctx(ctx).Err(err).Msgf("cannot mark session of run %s blocked", run.IssueKey)
	}
	if err := a.d.Slack.Mention(ctx, run.ChannelID, run.ThreadTS, a.d.OwnerID, questionText(run.IssueKey, *ask)); err != nil {
		log.Error().Ctx(ctx).Err(err).Msgf("cannot post the question of run %s", run.IssueKey)
	}
	return nil
}

func questionText(key domain.IssueKey, ask domain.Ask) string {
	var b strings.Builder
	fmt.Fprintf(&b, "❓ %s needs you: %s", key, ask.Question)
	for _, o := range ask.Options {
		fmt.Fprintf(&b, "\n• %s", o.Label)
	}
	b.WriteString("\nReply in this thread.")
	return b.String()
}

// Wait blocks until the ask closes or the limit passes. It returns the answered ask; domain.ErrAskDismissed
// when the owner closed the dialog; domain.ErrAskPending after eight minutes with the ask still open.
func (a *Ask) Wait(ctx context.Context, id domain.AskID) (domain.Ask, error) {
	deadline := a.d.Now().Add(askWaitLimit)
	for {
		ask, err := a.d.Asks.Find(ctx, id)
		if err != nil {
			return domain.Ask{}, err
		}
		switch ask.Status {
		case domain.AskAnswered:
			return ask, nil
		case domain.AskCancelled:
			return domain.Ask{}, fmt.Errorf("ask %d: %w", id, domain.ErrAskDismissed)
		case domain.AskOpen:
			closed, err := a.pollDialog(ctx, ask)
			if err != nil {
				return domain.Ask{}, err
			}
			if closed {
				// read the ask again: the other side may have closed it first
				continue
			}
		}
		if !a.d.Now().Before(deadline) {
			return domain.Ask{}, fmt.Errorf("ask %d: %w", id, domain.ErrAskPending)
		}
		if err := a.d.Sleep(ctx, askPollInterval); err != nil {
			return domain.Ask{}, err
		}
	}
}

// pollDialog reads the dialog of an open ask and closes the ask when the owner answered or dismissed it there.
func (a *Ask) pollDialog(ctx context.Context, ask domain.Ask) (bool, error) {
	if ask.DialogID == "" {
		return false, nil
	}
	result, err := a.d.Sessions.AskResult(ctx, ask.DialogID)
	if err != nil {
		return false, fmt.Errorf("cannot read the dialog of ask %d: %w", ask.ID, err)
	}
	var d closeData
	switch result.State {
	case agterm.AskAnswered:
		d = closeData{answer: result.Answer, by: domain.AnsweredByAgterm}
	case agterm.AskDismissed:
		d = closeData{dismissed: true}
	default:
		return false, nil
	}
	defer a.lockRun(ask.IssueKey)()
	// the ask may have closed, or been presented again, while the dialog was read
	current, err := a.d.Asks.Find(ctx, ask.ID)
	if err != nil {
		return false, err
	}
	if current.Status != domain.AskOpen {
		return true, nil
	}
	return true, a.closeLocked(ctx, current, d)
}

// ThreadReply takes a reply in a run's thread. The owner's reply in the thread of an active run gets the seen
// reaction; only while an ask is open does it answer the ask.
func (a *Ask) ThreadReply(ctx context.Context, m controllerslack.Message) error {
	text := strings.TrimSpace(m.Text)
	if m.UserID != a.d.OwnerID || text == "" {
		return nil
	}
	run, err := a.d.Runs.FindByRequest(ctx, m.ChannelID, m.ThreadTS)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot find the run of thread %s: %w", m.ThreadTS, err)
	}
	if !run.EndedAt.IsZero() {
		return nil
	}
	// the reaction only tells the owner the reply was read, so a failure is logged
	if err := a.d.Slack.React(ctx, m.ChannelID, m.TS, slackpost.ReactionSeen); err != nil {
		log.Warn().Ctx(ctx).Err(err).Msgf("cannot mark thread reply %s of run %s as seen", m.TS, run.IssueKey)
	}
	defer a.lockRun(run.IssueKey)()
	ask, err := a.d.Asks.FindOpen(ctx, run.IssueKey)
	if errors.Is(err, domain.ErrNotFound) {
		log.Debug().Ctx(ctx).Msgf("thread reply %s of run %s answers nothing: no open ask", m.TS, run.IssueKey)
		return nil
	}
	if err != nil {
		return err
	}
	return a.closeLocked(ctx, ask, closeData{answer: optionIDFor(ask.Options, text), by: domain.AnsweredBySlack})
}

// optionIDFor gives the option id when the reply names an option by id or label; any other text stays as it is.
func optionIDFor(options []domain.AskOption, text string) string {
	for _, o := range options {
		if strings.EqualFold(text, o.ID) || strings.EqualFold(text, o.Label) {
			return o.ID
		}
	}
	return text
}

type closeData struct {
	answer    string
	by        domain.AnsweredBy
	dismissed bool
}

// closeLocked ends an open ask; the caller holds the run's lock. The store lets one closer win; a loser returns
// without error and changes nothing. The winner closes the dialog of a Slack answer, then shows the next queued
// ask or gives the run its status back. A run that has ended keeps its final status: the answer is stored and nothing else happens.
func (a *Ask) closeLocked(ctx context.Context, ask domain.Ask, d closeData) error {
	var err error
	if d.dismissed {
		err = a.d.Asks.Cancel(ctx, ask.ID, a.d.Now())
	} else {
		err = a.d.Asks.Answer(ctx, ask.ID, domain.AskAnswer{Text: d.answer, By: d.by}, a.d.Now())
	}
	if errors.Is(err, domain.ErrNotOpen) {
		return nil
	}
	if err != nil {
		return err
	}
	run, err := a.d.Runs.FindByKey(ctx, ask.IssueKey)
	if err != nil {
		return fmt.Errorf("cannot find run %s: %w", ask.IssueKey, err)
	}
	if !run.EndedAt.IsZero() {
		return nil
	}
	a.closeDialog(ctx, ask, d)
	return a.showNext(ctx, run)
}

// CloseAllForRun cancels every open and queued ask of a run and its dialogs, without a status change.
// The run is ending, so the caller sets the final status.
func (a *Ask) CloseAllForRun(ctx context.Context, key domain.IssueKey) error {
	defer a.lockRun(key)()
	for {
		ask, err := a.d.Asks.FindOpen(ctx, key)
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if ask.DialogID != "" {
			if err := a.d.Sessions.CancelAsk(ctx, ask.DialogID); err != nil {
				log.Error().Ctx(ctx).Err(err).Msgf("cannot cancel the dialog of ask %d", ask.ID)
			}
		}
		// cancelling an open ask opens the next queued one, so the loop ends when none is left
		if err := a.d.Asks.Cancel(ctx, ask.ID, a.d.Now()); err != nil && !errors.Is(err, domain.ErrNotOpen) {
			return fmt.Errorf("cannot cancel ask %d: %w", ask.ID, err)
		}
	}
}

// closeDialog cancels the dialog after a Slack answer. It is courtesy: the answer is already stored, so a failure
// is logged. An agterm answer posts nothing: the thread stays quiet.
func (a *Ask) closeDialog(ctx context.Context, ask domain.Ask, d closeData) {
	if d.by != domain.AnsweredBySlack || ask.DialogID == "" {
		return
	}
	if err := a.d.Sessions.CancelAsk(ctx, ask.DialogID); err != nil {
		log.Error().Ctx(ctx).Err(err).Msgf("cannot cancel the dialog of ask %d", ask.ID)
	}
}

// showNext shows the ask the store opened behind the closed one. With none left, the run is no longer blocked.
func (a *Ask) showNext(ctx context.Context, run domain.Run) error {
	next, err := a.d.Asks.FindOpen(ctx, run.IssueKey)
	if err == nil {
		if err := a.present(ctx, run, &next); err != nil {
			return fmt.Errorf("cannot show ask %d: %w", next.ID, err)
		}
		return nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	if err := a.d.Sessions.SetStatus(ctx, run.SessionID, statusActive, false); err != nil {
		log.Error().Ctx(ctx).Err(err).Msgf("cannot mark session of run %s active", run.IssueKey)
	}
	a.setLinearStatus(ctx, run, string(run.Phase))
	return nil
}

// setLinearStatus moves the issue to the Linear status of a phase. The status is a side effect of the ask:
// the answer is stored already, so a failure is logged and never fails the caller.
func (a *Ask) setLinearStatus(ctx context.Context, run domain.Run, phase string) {
	if err := a.moveIssue(ctx, run, phase); err != nil {
		log.Error().Ctx(ctx).Err(err).Msgf("cannot set the linear status of run %s for phase %s", run.IssueKey, phase)
	}
}

func (a *Ask) moveIssue(ctx context.Context, run domain.Run, phase string) error {
	return linearMirror{linear: a.d.Linear, registry: a.d.Registry}.set(ctx, run, phase)
}
