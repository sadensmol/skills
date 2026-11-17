package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/sadensmol/autopilot/internal/domain"
	"github.com/sadensmol/autopilot/internal/usecase"
)

type ResumeTestSuite struct {
	statusBase
}

func TestResumeTestSuite(t *testing.T) {
	suite.Run(t, new(ResumeTestSuite))
}

// blockRun passes the gate loop cap of 1 so the run blocks; the phase before the block is implementing.
func (s *ResumeTestSuite) blockRun() {
	uc := s.newStatus(s.newAsk(), 1)
	for _, phase := range []string{"qa", "implementing", "qa", "implementing"} {
		s.Require().NoError(uc.Move(s.Ctx, statusRunKey, phase))
	}
	run, err := s.runs.FindByKey(s.Ctx, statusRunKey)
	s.Require().NoError(err)
	s.Require().Equal(domain.Phase("blocked"), run.Phase)
}

func (s *ResumeTestSuite) TestResumeRestoresThePreviousPhaseResetsLoopsAndPostsALine() {
	s.blockRun()
	uc := s.newStatus(s.newAsk(), 1)
	posts := len(s.slack.Posts())

	s.Require().NoError(uc.Resume(s.Ctx, statusRunKey))

	run, err := s.runs.FindByKey(s.Ctx, statusRunKey)
	s.Require().NoError(err)
	s.Equal(domain.Phase("implementing"), run.Phase)
	loops, err := s.runs.Loops(s.Ctx, statusRunKey)
	s.Require().NoError(err)
	s.Zero(loops[domain.LoopGate])
	s.Zero(loops[domain.LoopReview])
	s.Equal("st-In Progress", s.stateChanges()[len(s.stateChanges())-1])
	s.Require().Len(s.slack.Posts(), posts+1)
	s.Equal("▶️ ACM-9 resumed: back to implementing.", s.lastText())
	s.NoError(uc.Move(s.Ctx, statusRunKey, "qa"), "a resumed run moves again")
}

func (s *ResumeTestSuite) TestResumeWakesTheRunSessionByItsStoredID() {
	s.blockRun()

	s.Require().NoError(s.newStatus(s.newAsk(), 1).Resume(s.Ctx, statusRunKey))

	s.Contains(s.agtermLog(), "session\ttype\t--socket\t/tmp/ap-test.sock\t--target\tsess-1\tautopilot resumed ACM-9: continue the run")
}

func (s *ResumeTestSuite) TestResumeThatCannotWakeTheRunSessionNamesItAndStaysResumed() {
	s.blockRun()
	s.Require().NoError(os.WriteFile(filepath.Join(os.Getenv("FAKE_AGTERM_DIR"), "type-fails"), nil, 0o600))

	err := s.newStatus(s.newAsk(), 1).Resume(s.Ctx, statusRunKey)

	s.Require().Error(err)
	s.Contains(err.Error(), "resumed, but cannot wake its session sess-1")
	s.Equal(domain.Phase("implementing"), s.phase())
}

func (s *ResumeTestSuite) agtermLog() string {
	data, err := os.ReadFile(filepath.Join(os.Getenv("FAKE_AGTERM_DIR"), "argv.log"))
	s.Require().NoError(err)
	return string(data)
}

func (s *ResumeTestSuite) TestResumeOfARunThatIsNotBlockedIsRefused() {
	uc := s.newStatus(s.newAsk(), 1)

	err := uc.Resume(s.Ctx, statusRunKey)

	s.Require().Error(err)
	s.Contains(err.Error(), "not blocked")
	s.Empty(s.stateChanges())
}

func (s *ResumeTestSuite) TestResumeOfAnEndedRunIsRefused() {
	s.blockRun()
	s.Require().NoError(s.runs.End(s.Ctx, statusRunKey, s.Clock.Now()))

	err := s.newStatus(s.newAsk(), 1).Resume(s.Ctx, statusRunKey)

	s.Require().Error(err)
	s.Contains(err.Error(), "ended")
}

func (s *ResumeTestSuite) TestResumeWhileAnAskIsOpenLeavesLinearToTheAsk() {
	s.blockRun()
	ask := s.newAsk()
	_, err := ask.Open(s.Ctx, statusRunKey, "Which flag?", nil)
	s.Require().NoError(err)
	changes := len(s.stateChanges())

	s.Require().NoError(s.newStatus(ask, 1).Resume(s.Ctx, statusRunKey))

	s.Len(s.stateChanges(), changes)
	run, err := s.runs.FindByKey(s.Ctx, statusRunKey)
	s.Require().NoError(err)
	s.Equal(domain.Phase("implementing"), run.Phase)
}

func (s *ResumeTestSuite) lastText() string {
	posts := s.slack.Posts()
	return posts[len(posts)-1].Text
}

func (s *ResumeTestSuite) TestThirdResumeOfTheSameRunIsRefusedAndTheOwnerIsTold() {
	uc := s.newStatus(s.newAsk(), 1)
	for range 2 {
		s.blockAgain(uc)
		s.Require().NoError(uc.Resume(s.Ctx, statusRunKey))
	}
	s.blockAgain(uc)
	posts := len(s.slack.Posts())

	err := uc.Resume(s.Ctx, statusRunKey)

	s.Require().Error(err)
	s.Contains(err.Error(), "cancel")
	run, findErr := s.runs.FindByKey(s.Ctx, statusRunKey)
	s.Require().NoError(findErr)
	s.Equal(domain.Phase("blocked"), run.Phase)
	s.Len(s.slack.Posts(), posts)
}

// blockAgain passes the gate loop cap of 1 again; the run must be in implementing.
func (s *ResumeTestSuite) blockAgain(uc *usecase.Status) {
	phases := []string{"qa", "implementing", "qa", "implementing"}
	if s.phase() == "blocked" {
		return
	}
	if s.phase() != "implementing" {
		s.FailNow("run is not in implementing")
	}
	for _, phase := range phases {
		s.Require().NoError(uc.Move(s.Ctx, statusRunKey, phase))
	}
}
