package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/sadensmol/autopilot/internal/config"
	"github.com/sadensmol/autopilot/internal/domain"
)

// IHeartbeatRuns is the part of the run store that the heartbeat uses.
type IHeartbeatRuns interface {
	ListActive(ctx context.Context) ([]domain.Run, error)
	SetLastPostAt(ctx context.Context, key domain.IssueKey, at time.Time) error
}

// IHeartbeatSessions shows the phase of a run under its run session's sidebar row.
type IHeartbeatSessions interface {
	SetInfo(ctx context.Context, sessionID, text string, hideAfter time.Duration) error
}

// phaseLineTicks is how many ticks the phase line outlives its last refresh, so a stopped service leaves no stale line.
const phaseLineTicks = 3

type HeartbeatData struct {
	Registry config.Registry
	// Quiet is the time without a post after which a run gets a heartbeat.
	Quiet time.Duration
	// Tick is the pause between two checks of the runs.
	Tick   time.Duration
	Linear IMirrorLinear
	Runs   IHeartbeatRuns
	Asks   IOpenAsks
	Guard  IRunGuard
	// Sessions shows the phase of each run under its run session.
	Sessions IHeartbeatSessions
	Now      func() time.Time
	// Sleep pauses between two checks; it returns the context's error when the context ends.
	Sleep func(ctx context.Context, d time.Duration) error
}

// Heartbeat repairs the Linear status of a run that was quiet for too long, when it no longer matches the run.
// It posts nothing to the thread.
type Heartbeat struct {
	d HeartbeatData
}

func NewHeartbeat(d HeartbeatData) *Heartbeat {
	return &Heartbeat{d: d}
}

// Run checks the runs every tick until the context ends; it returns the context's error then.
func (h *Heartbeat) Run(ctx context.Context) error {
	for {
		if err := h.Tick(ctx); err != nil && ctx.Err() == nil {
			log.Error().Ctx(ctx).Err(err).Msg("heartbeat check failed")
		}
		if err := h.d.Sleep(ctx, h.d.Tick); err != nil {
			return err
		}
	}
}

// Tick gives a heartbeat to every active run that has been quiet for the quiet time.
func (h *Heartbeat) Tick(ctx context.Context) error {
	runs, err := h.d.Runs.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("cannot list runs: %w", err)
	}
	var firstErr error
	for _, run := range runs {
		h.showPhase(ctx, run)
		if err := h.beat(ctx, run); err != nil {
			log.Error().Ctx(ctx).Err(err).Msgf("heartbeat of run %s failed", run.IssueKey)
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

// showPhase refreshes the phase line of a run's session. It is best effort: the line only informs the user.
func (h *Heartbeat) showPhase(ctx context.Context, run domain.Run) {
	if run.SessionID == "" {
		return
	}
	if err := h.d.Sessions.SetInfo(ctx, run.SessionID, fmt.Sprintf("phase: %s", run.Phase), phaseLineTicks*h.d.Tick); err != nil {
		log.Warn().Ctx(ctx).Err(err).Msgf("cannot show the phase of run %s", run.IssueKey)
	}
}

func (h *Heartbeat) beat(ctx context.Context, listed domain.Run) error {
	if !h.quiet(listed) {
		return nil
	}
	defer h.d.Guard.LockRun(listed.IssueKey)()
	// the lock was free only now: a post or a phase change may have happened since the list
	run, err := h.fresh(ctx, listed)
	if err != nil || run == nil || !h.quiet(*run) {
		return err
	}
	if err := h.d.Runs.SetLastPostAt(ctx, run.IssueKey, h.d.Now()); err != nil {
		return fmt.Errorf("cannot store the heartbeat time: %w", err)
	}
	return h.repairStatus(ctx, *run)
}

// fresh reads the run again; nil means it ended meanwhile.
func (h *Heartbeat) fresh(ctx context.Context, listed domain.Run) (*domain.Run, error) {
	runs, err := h.d.Runs.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot list runs: %w", err)
	}
	for _, r := range runs {
		if r.IssueKey == listed.IssueKey {
			return &r, nil
		}
	}
	return nil, nil
}

func (h *Heartbeat) quiet(run domain.Run) bool {
	last := run.LastPostAt
	if last.IsZero() {
		last = run.CreatedAt
	}
	return h.d.Now().Sub(last) >= h.d.Quiet
}

// repairStatus sets the Linear status back when automation or a person changed it.
// An open ask expects blocked; any other run expects the status of its phase.
func (h *Heartbeat) repairStatus(ctx context.Context, run domain.Run) error {
	expectedPhase := string(run.Phase)
	if _, err := h.d.Asks.FindOpen(ctx, run.IssueKey); err == nil {
		expectedPhase = blockedPhase
	} else if !errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("cannot read the asks of run %s: %w", run.IssueKey, err)
	}
	expected, ok := h.d.Registry.Linear.Statuses[expectedPhase]
	if !ok {
		return fmt.Errorf("no linear status is configured for phase %s", expectedPhase)
	}
	mirror := linearMirror{linear: h.d.Linear, registry: h.d.Registry}
	current, err := mirror.current(ctx, run)
	if err != nil {
		return err
	}
	if current == expected {
		return nil
	}
	log.Warn().Ctx(ctx).Msgf("linear status of run %s is %q, expected %q: setting it back", run.IssueKey, current, expected)
	return mirror.set(ctx, run, expectedPhase)
}
