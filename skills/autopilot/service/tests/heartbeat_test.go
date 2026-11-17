package tests

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/sadensmol/autopilot/internal/agterm"
	"github.com/sadensmol/autopilot/internal/linear"
	"github.com/sadensmol/autopilot/internal/repository"
	"github.com/sadensmol/autopilot/internal/usecase"
)

type HeartbeatTestSuite struct {
	statusBase
}

func TestHeartbeatTestSuite(t *testing.T) {
	suite.Run(t, new(HeartbeatTestSuite))
}

func (s *HeartbeatTestSuite) newHeartbeat(ask *usecase.Ask) *usecase.Heartbeat {
	return usecase.NewHeartbeat(usecase.HeartbeatData{
		Registry: testRegistry(),
		Quiet:    30 * time.Minute,
		Tick:     time.Minute,
		Linear:   linear.NewClient(s.linear.URL(), "k", nil),
		Runs:     s.runs,
		Asks:     s.asks,
		Guard:    ask,
		Sessions: agterm.New(s.fakeAgterm(), "/tmp/ap-test.sock"),
		Now:      s.Clock.Now,
		Sleep: func(ctx context.Context, d time.Duration) error {
			s.Clock.Advance(d)
			return ctx.Err()
		},
	})
}

func (s *HeartbeatTestSuite) TestEveryTickShowsThePhaseUnderTheRunSession() {
	hb := s.newHeartbeat(s.newAsk())

	s.Require().NoError(hb.Tick(s.Ctx))

	data, err := os.ReadFile(filepath.Join(os.Getenv("FAKE_AGTERM_DIR"), "argv.log"))
	s.Require().NoError(err)
	s.Contains(strings.Split(strings.TrimSpace(string(data)), "\n"),
		"session\tinfo\t--socket\t/tmp/ap-test.sock\t--target\tsess-1\t--hide-after\t180\tphase: implementing")
}

func (s *HeartbeatTestSuite) TestOneStatusCheckAfterThirtyQuietMinutesAndNoThreadPost() {
	s.current = "Done"
	hb := s.newHeartbeat(s.newAsk())

	s.Clock.Advance(29 * time.Minute)
	s.Require().NoError(hb.Tick(s.Ctx))
	s.Empty(s.stateChanges())

	s.Clock.Advance(time.Minute)
	s.Require().NoError(hb.Tick(s.Ctx))
	s.Require().NoError(hb.Tick(s.Ctx))

	s.Equal([]string{"st-In Progress"}, s.stateChanges())
	s.Empty(s.texts())
}

func (s *HeartbeatTestSuite) TestAnyPostResetsTheTimer() {
	s.current = "Done"
	ask := s.newAsk()
	hb := s.newHeartbeat(ask)
	status := s.newStatus(ask, 3)
	s.Clock.Advance(25 * time.Minute)
	s.Require().NoError(status.Post(s.Ctx, statusRunKey, "progress"))
	s.Clock.Advance(25 * time.Minute)

	s.Require().NoError(hb.Tick(s.Ctx))

	s.Empty(s.stateChanges())

	s.Clock.Advance(5 * time.Minute)
	s.Require().NoError(hb.Tick(s.Ctx))
	s.Equal([]string{"st-In Progress"}, s.stateChanges())
	s.Equal([]string{"progress"}, s.texts())
}

func (s *HeartbeatTestSuite) TestDriftedLinearStatusIsReset() {
	s.current = "Done"
	hb := s.newHeartbeat(s.newAsk())
	s.Clock.Advance(31 * time.Minute)

	s.Require().NoError(hb.Tick(s.Ctx))

	s.Equal([]string{"st-In Progress"}, s.stateChanges())
}

func (s *HeartbeatTestSuite) TestMatchingLinearStatusIsLeftAlone() {
	hb := s.newHeartbeat(s.newAsk())
	s.Clock.Advance(31 * time.Minute)

	s.Require().NoError(hb.Tick(s.Ctx))

	s.Empty(s.stateChanges())
}

func (s *HeartbeatTestSuite) TestOpenAskExpectsBlocked() {
	s.current = "Blocked"
	_, err := s.asks.Create(s.Ctx, repository.CreateAskData{IssueKey: statusRunKey, Question: "q", Now: s.Clock.Now()})
	s.Require().NoError(err)
	hb := s.newHeartbeat(s.newAsk())
	s.Clock.Advance(31 * time.Minute)

	s.Require().NoError(hb.Tick(s.Ctx))

	s.Empty(s.stateChanges())
}

func (s *HeartbeatTestSuite) TestEndedRunGetsNoHeartbeat() {
	s.current = "Done"
	s.Require().NoError(s.runs.End(s.Ctx, statusRunKey, s.Clock.Now()))
	hb := s.newHeartbeat(s.newAsk())
	s.Clock.Advance(31 * time.Minute)

	s.Require().NoError(hb.Tick(s.Ctx))

	s.Empty(s.stateChanges())
	s.Empty(s.texts())
}

func (s *HeartbeatTestSuite) TestRunStopsWhenContextEnds() {
	hb := s.newHeartbeat(s.newAsk())
	ctx, cancel := context.WithCancel(s.Ctx)
	cancel()

	err := hb.Run(ctx)

	s.True(errors.Is(err, context.Canceled))
}
