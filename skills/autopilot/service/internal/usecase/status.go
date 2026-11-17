package usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/sadensmol/autopilot/internal/config"
	"github.com/sadensmol/autopilot/internal/domain"
)

const (
	donePhase     = "done"
	canceledPhase = "canceled"
)

// IStatusSlack is the part of the Slack poster that status moves use.
type IStatusSlack interface {
	Thread(ctx context.Context, channelID, threadTS, text string) error
}

// IStatusRuns is the part of the run store that status moves use.
type IStatusRuns interface {
	FindByKey(ctx context.Context, key domain.IssueKey) (domain.Run, error)
	SetPhase(ctx context.Context, key domain.IssueKey, phase domain.Phase, now time.Time) error
	SetLastPostAt(ctx context.Context, key domain.IssueKey, at time.Time) error
	IncLoop(ctx context.Context, key domain.IssueKey, loop domain.Loop, now time.Time) (int, error)
	Loops(ctx context.Context, key domain.IssueKey) (map[domain.Loop]int, error)
	ResetLoops(ctx context.Context, key domain.IssueKey, loops ...domain.Loop) error
}

// IOpenAsks tells whether a run waits on an ask.
type IOpenAsks interface {
	FindOpen(ctx context.Context, key domain.IssueKey) (domain.Ask, error)
}

// IStatusSessions types into the run session of a run.
type IStatusSessions interface {
	Type(ctx context.Context, sessionID, text string) error
}

// IRunGuard serialises the work on one run. The ask usecase holds the lock of a run while it
// opens or closes an ask; a status move takes the same lock, so both agree on who owns the Linear status.
type IRunGuard interface {
	LockRun(key domain.IssueKey) func()
}

type StatusData struct {
	Registry config.Registry
	// LoopCap is the count of gate or review loops a run may pass; the next loop blocks the run.
	LoopCap int
	Linear  IMirrorLinear
	Slack   IStatusSlack
	Runs    IStatusRuns
	Asks    IOpenAsks
	Guard   IRunGuard
	// Sessions wakes the run session on a resume.
	Sessions IStatusSessions
	Now      func() time.Time
}

// Status mirrors the phase of a run to Linear; the thread hears only of a blocked run.
type Status struct {
	d StatusData
}

func NewStatus(d StatusData) *Status {
	return &Status{d: d}
}

// Move records the phase, counts the loop it closes and sets the Linear status. Only a move to blocked posts a
// thread line. The same phase twice changes nothing. While an ask is open the phase is recorded but Linear stays
// blocked: the ask owns that status and restores the status of the new phase when it closes.
// A blocked run, and the phases done and canceled, cannot be set through status.
func (s *Status) Move(ctx context.Context, key domain.IssueKey, phase string) error {
	if _, ok := s.d.Registry.Linear.Statuses[phase]; !ok {
		return fmt.Errorf("unknown phase %q: use one of %s", phase, strings.Join(s.phaseNames(), ", "))
	}
	if phase == donePhase || phase == canceledPhase {
		return fmt.Errorf("phase %q is not set by status: a merged PR ends a run as done and a cancel ends it as canceled", phase)
	}
	defer s.d.Guard.LockRun(key)()
	run, err := s.d.Runs.FindByKey(ctx, key)
	if err != nil {
		return fmt.Errorf("cannot find run %s: %w", key, err)
	}
	if !run.EndedAt.IsZero() {
		return fmt.Errorf("run %s has ended", key)
	}
	if run.Phase == blockedPhase {
		return fmt.Errorf("run %s is blocked: its owner must resume it, so status cannot move it to %s", key, phase)
	}
	askOpen, err := s.askOpen(ctx, key)
	if err != nil {
		return err
	}
	if askOpen && phase == blockedPhase {
		return fmt.Errorf("run %s waits on an open ask: it cannot move to %s", key, blockedPhase)
	}
	if string(run.Phase) == phase {
		return nil
	}
	if phase == blockedPhase {
		// the thread line goes first: a failed post leaves the run unchanged, so a retry of the same move works
		line := fmt.Sprintf("⛔ %s is stuck. Resume it from the control room when it is fixed.", key)
		if err := s.postLine(ctx, run, line); err != nil {
			return err
		}
	}
	if err := s.d.Runs.SetPhase(ctx, key, domain.Phase(phase), s.d.Now()); err != nil {
		return fmt.Errorf("cannot set phase of run %s: %w", key, err)
	}
	loop, capped := closedLoop(string(run.Phase), phase)
	count := 0
	if loop != "" {
		if count, err = s.d.Runs.IncLoop(ctx, key, loop, s.d.Now()); err != nil {
			return fmt.Errorf("cannot count loop: %w", err)
		}
	}
	if capped && count > s.d.LoopCap {
		return s.block(ctx, run, loop, count, askOpen)
	}
	if !askOpen {
		s.mirror().setOrLog(ctx, run, phase)
	}
	return nil
}

// block stops a run whose loop passed the cap. The phase becomes blocked even while an ask is open:
// the ask keeps Linear blocked, and when it closes it restores the status of the phase, which is blocked.
func (s *Status) block(ctx context.Context, run domain.Run, loop domain.Loop, count int, askOpen bool) error {
	if err := s.d.Runs.SetPhase(ctx, run.IssueKey, blockedPhase, s.d.Now()); err != nil {
		return fmt.Errorf("cannot block run %s: %w", run.IssueKey, err)
	}
	if !askOpen {
		s.mirror().setOrLog(ctx, run, blockedPhase)
	}
	line := fmt.Sprintf("⛔ %s is stuck: the %s loop ran %d times, the cap is %d. Resume it from the control room when it is fixed.",
		run.IssueKey, loop, count, s.d.LoopCap)
	return s.postLine(ctx, run, line)
}

// closedLoop names the loop a move from one phase to another closes, and whether the loop has a cap.
func closedLoop(from, to string) (loop domain.Loop, capped bool) {
	switch {
	case from == "qa" && to == "implementing":
		return domain.LoopGate, true
	case from == "review" && (to == "implementing" || to == "qa"):
		return domain.LoopReview, true
	case from == "pr" && to == "implementing":
		return domain.LoopPR, false
	}
	return "", false
}

// Post sends a plain thread line of a worker and resets the heartbeat timer.
func (s *Status) Post(ctx context.Context, key domain.IssueKey, note string) error {
	defer s.d.Guard.LockRun(key)()
	run, err := s.d.Runs.FindByKey(ctx, key)
	if err != nil {
		return fmt.Errorf("cannot find run %s: %w", key, err)
	}
	if !run.EndedAt.IsZero() {
		return fmt.Errorf("run %s has ended", key)
	}
	return s.postLine(ctx, run, note)
}

func (s *Status) postLine(ctx context.Context, run domain.Run, text string) error {
	if err := s.d.Slack.Thread(ctx, run.ChannelID, run.ThreadTS, text); err != nil {
		return fmt.Errorf("cannot post to the thread of run %s: %w", run.IssueKey, err)
	}
	if err := s.d.Runs.SetLastPostAt(ctx, run.IssueKey, s.d.Now()); err != nil {
		return fmt.Errorf("cannot store the post time of run %s: %w", run.IssueKey, err)
	}
	return nil
}

func (s *Status) askOpen(ctx context.Context, key domain.IssueKey) (bool, error) {
	_, err := s.d.Asks.FindOpen(ctx, key)
	if errors.Is(err, domain.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("cannot read the asks of run %s: %w", key, err)
	}
	return true, nil
}

func (s *Status) phaseNames() []string {
	names := make([]string, 0, len(s.d.Registry.Linear.Statuses))
	for name := range s.d.Registry.Linear.Statuses {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func (s *Status) mirror() statusMirror {
	return statusMirror{linearMirror{linear: s.d.Linear, registry: s.d.Registry}}
}

// statusMirror sets a status where the phase is already stored, so a failed Linear call is logged and the next
// heartbeat repairs the status.
type statusMirror struct{ linearMirror }

func (m statusMirror) setOrLog(ctx context.Context, run domain.Run, phase string) {
	if err := m.set(ctx, run, phase); err != nil {
		log.Error().Ctx(ctx).Err(err).Msgf("cannot set the linear status of run %s for phase %s", run.IssueKey, phase)
	}
}
