package usecase

import (
	"context"
	"fmt"

	"github.com/sadensmol/autopilot/internal/domain"
)

// maxResumes bounds the resumes of one run: a run that blocks again after that goes to the owner for a cancel.
const maxResumes = 2

// Resume takes a run out of the blocked phase on the owner's word: it restores the phase before the block,
// resets the gate and review loop counters, posts one thread line and wakes the run session. While an ask is
// open Linear stays with the ask, which restores the status of the phase when it closes.
func (s *Status) Resume(ctx context.Context, key domain.IssueKey) error {
	defer s.d.Guard.LockRun(key)()
	run, err := s.d.Runs.FindByKey(ctx, key)
	if err != nil {
		return fmt.Errorf("cannot find run %s: %w", key, err)
	}
	if !run.EndedAt.IsZero() {
		return fmt.Errorf("run %s has ended", key)
	}
	if run.Phase != blockedPhase {
		return fmt.Errorf("run %s is not blocked: it is in phase %s", key, run.Phase)
	}
	if run.PreviousPhase == "" || run.PreviousPhase == blockedPhase {
		return fmt.Errorf("run %s has no phase to go back to", key)
	}
	loops, err := s.d.Runs.Loops(ctx, key)
	if err != nil {
		return fmt.Errorf("cannot read the loops of run %s: %w", key, err)
	}
	if loops[domain.LoopResume] >= maxResumes {
		return fmt.Errorf("run %s was resumed %d times already: the owner must cancel it", key, maxResumes)
	}
	askOpen, err := s.askOpen(ctx, key)
	if err != nil {
		return err
	}
	// the thread line goes first: a failed post leaves the run blocked, so a retry works
	if err := s.postLine(ctx, run, fmt.Sprintf("▶️ %s resumed: back to %s.", key, run.PreviousPhase)); err != nil {
		return err
	}
	if err := s.d.Runs.ResetLoops(ctx, key, domain.LoopGate, domain.LoopReview); err != nil {
		return fmt.Errorf("cannot reset the loops of run %s: %w", key, err)
	}
	// the phase moves last: a failure before it leaves the run blocked, so a retry works
	if err := s.d.Runs.SetPhase(ctx, key, run.PreviousPhase, s.d.Now()); err != nil {
		return fmt.Errorf("cannot set phase of run %s: %w", key, err)
	}
	if _, err := s.d.Runs.IncLoop(ctx, key, domain.LoopResume, s.d.Now()); err != nil {
		return fmt.Errorf("cannot count the resume of run %s: %w", key, err)
	}
	if !askOpen {
		s.mirror().setOrLog(ctx, run, string(run.PreviousPhase))
	}
	// the wake goes to the stored run session id: part sessions share the run session's name prefix
	if err := s.d.Sessions.Type(ctx, run.SessionID, fmt.Sprintf("autopilot resumed %s: continue the run\n", key)); err != nil {
		return fmt.Errorf("run %s resumed, but cannot wake its session %s: %w", key, run.SessionID, err)
	}
	return nil
}
