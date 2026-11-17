package tests

import (
	"strings"
	"sync"
	"time"

	"github.com/sadensmol/autopilot/internal/domain"
)

// statefulLinear makes the fake Linear remember the status that SetState wrote, and slow every write down, so that
// an unlocked read-then-write has time to interleave with another writer.
func (s *PRWatchTestSuite) statefulLinear() {
	var mu sync.Mutex
	current := "In Progress"
	s.linear.On("IssueByKey", func(map[string]any) LinearReply {
		mu.Lock()
		defer mu.Unlock()
		return LinearReply{Data: map[string]any{"issue": map[string]any{
			"id": "iss-1", "identifier": "ACM-9", "title": "Add a health endpoint", "url": "https://linear.example/ACM-9",
			"branchName": "kim/acm-9", "state": map[string]any{"name": current},
		}}}
	})
	s.linear.On("SetState", func(vars map[string]any) LinearReply {
		time.Sleep(5 * time.Millisecond)
		mu.Lock()
		defer mu.Unlock()
		current = strings.TrimPrefix(vars["stateId"].(string), "st-")
		return LinearReply{Data: map[string]any{"issueUpdate": map[string]any{"success": true}}}
	})
	s.linear.On("Comment", func(map[string]any) LinearReply {
		time.Sleep(5 * time.Millisecond)
		return LinearReply{Data: map[string]any{"commentCreate": map[string]any{"success": true}}}
	})
}

func (s *PRWatchTestSuite) comments() int {
	n := 0
	for _, r := range s.linear.Requests() {
		if r.Operation == "Comment" {
			n++
		}
	}
	return n
}

func (s *PRWatchTestSuite) race(fns ...func()) {
	var wg sync.WaitGroup
	start := make(chan struct{})
	for _, fn := range fns {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			fn()
		}()
	}
	close(start)
	wg.Wait()
}

func (s *PRWatchTestSuite) TestCancelRacingTheMergeEndWritesOneTerminalLinearStatus() {
	for range 20 {
		s.SetupTest()
		s.statefulLinear()
		ask := s.newAsk()
		s.openPR(ask)
		watch := s.newWatch(ask)
		canceller := s.newCanceller(ask)
		s.view("MERGED", `"reviews":[],"comments":[],"statusCheckRollup":[]`)

		s.race(
			func() { s.NoError(watch.Tick(s.Ctx)) },
			func() { s.NoError(canceller.CancelRun(s.Ctx, statusRunKey)) },
		)

		changes := s.stateChanges()
		s.Require().Len(changes, 1, "one terminal status, got %v", changes)
		run, err := s.runs.FindByKey(s.Ctx, statusRunKey)
		s.Require().NoError(err)
		s.False(run.EndedAt.IsZero())
		if changes[0] == "st-Canceled" {
			s.Equal(domain.Phase("canceled"), run.Phase)
			s.Equal(1, s.comments())
		} else {
			s.Equal(domain.Phase("done"), run.Phase)
			s.Zero(s.comments(), "a cancel that lost the race writes nothing to Linear")
		}
	}
}

func (s *PRWatchTestSuite) TestTwoCancelsCommentAndSetTheStatusOnce() {
	for range 20 {
		s.SetupTest()
		s.statefulLinear()
		ask := s.newAsk()
		canceller := s.newCanceller(ask)

		s.race(
			func() { s.NoError(canceller.CancelRun(s.Ctx, statusRunKey)) },
			func() { s.NoError(canceller.CancelRun(s.Ctx, statusRunKey)) },
		)

		s.Equal([]string{"st-Canceled"}, s.stateChanges())
		s.Equal(1, s.comments())
	}
}
