package tests_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/sadensmol/autopilot/internal/domain"
	"github.com/sadensmol/autopilot/internal/repository"
	"github.com/sadensmol/autopilot/tests"
)

type AskRepositoryTestSuite struct {
	tests.Suite
	asks *repository.Ask
}

func TestAskRepositoryTestSuite(t *testing.T) {
	suite.Run(t, &AskRepositoryTestSuite{})
}

func (s *AskRepositoryTestSuite) SetupTest() {
	s.Suite.SetupTest()
	s.asks = repository.NewAsk(s.DB)
	runs := repository.NewRun(s.DB)
	for i, key := range []domain.IssueKey{"ACM-1", "ACM-2"} {
		ts := []string{"1.1", "2.2"}[i]
		_, err := runs.Create(s.Ctx, domain.Run{
			IssueKey: key, Product: "widgets", ChannelID: "C1", RequestTS: ts, ThreadTS: ts, Phase: domain.Phase("planning"),
		}, s.Clock.Now())
		s.Require().NoError(err)
	}
}

func (s *AskRepositoryTestSuite) ask(key domain.IssueKey, question string) domain.Ask {
	ask, err := s.asks.Create(s.Ctx, repository.CreateAskData{
		IssueKey: key, Question: question, Options: []domain.AskOption{{ID: "a", Label: "Yes"}, {ID: "b", Label: "No"}}, Now: s.Clock.Now(),
	})
	s.Require().NoError(err)
	return ask
}

func (s *AskRepositoryTestSuite) TestFirstAskIsOpenLaterOnesQueue() {
	first := s.ask("ACM-1", "first?")
	second := s.ask("ACM-1", "second?")
	third := s.ask("ACM-1", "third?")
	other := s.ask("ACM-2", "other run?")

	s.Equal(domain.AskOpen, first.Status)
	s.Equal(domain.AskQueued, second.Status)
	s.Equal(domain.AskQueued, third.Status)
	s.Equal(domain.AskOpen, other.Status)
	s.Equal([]domain.AskOption{{ID: "a", Label: "Yes"}, {ID: "b", Label: "No"}}, first.Options)

	open, err := s.asks.FindOpen(s.Ctx, "ACM-1")
	s.Require().NoError(err)
	s.Equal(first.ID, open.ID)
}

func (s *AskRepositoryTestSuite) TestAnswerPromotesNextInOrder() {
	first := s.ask("ACM-1", "first?")
	second := s.ask("ACM-1", "second?")
	third := s.ask("ACM-1", "third?")

	s.Require().NoError(s.asks.Answer(s.Ctx, first.ID, domain.AskAnswer{Text: "a", By: domain.AnsweredBySlack}, s.Clock.Now()))

	answered, err := s.asks.Find(s.Ctx, first.ID)
	s.Require().NoError(err)
	s.Equal(domain.AskAnswered, answered.Status)
	s.Equal("a", answered.Answer)
	s.Equal(domain.AnsweredBySlack, answered.AnsweredBy)

	open, err := s.asks.FindOpen(s.Ctx, "ACM-1")
	s.Require().NoError(err)
	s.Equal(second.ID, open.ID)

	s.Require().NoError(s.asks.Cancel(s.Ctx, second.ID, s.Clock.Now()))
	open, err = s.asks.FindOpen(s.Ctx, "ACM-1")
	s.Require().NoError(err)
	s.Equal(third.ID, open.ID)
}

func (s *AskRepositoryTestSuite) TestAnswerOfNonOpenAskIsRefused() {
	first := s.ask("ACM-1", "first?")
	second := s.ask("ACM-1", "second?")

	err := s.asks.Answer(s.Ctx, second.ID, domain.AskAnswer{Text: "a", By: domain.AnsweredByAgterm}, s.Clock.Now())
	s.True(errors.Is(err, domain.ErrNotOpen))

	s.Require().NoError(s.asks.Answer(s.Ctx, first.ID, domain.AskAnswer{Text: "a", By: domain.AnsweredByAgterm}, s.Clock.Now()))
	err = s.asks.Answer(s.Ctx, first.ID, domain.AskAnswer{Text: "b", By: domain.AnsweredByAgterm}, s.Clock.Now())
	s.True(errors.Is(err, domain.ErrNotOpen))
}

func (s *AskRepositoryTestSuite) TestNoOpenAskIsNotFound() {
	_, err := s.asks.FindOpen(s.Ctx, "ACM-1")
	s.True(errors.Is(err, domain.ErrNotFound))
	_, err = s.asks.Find(s.Ctx, 99)
	s.True(errors.Is(err, domain.ErrNotFound))
}

func (s *AskRepositoryTestSuite) TestAtMostOneOpenAskPerRunInTheDatabase() {
	s.ask("ACM-1", "first?")

	_, err := s.DB.ExecContext(s.Ctx, `
INSERT INTO ask (issue_key, question, options, status, created_at)
VALUES ('ACM-1', 'forced', '[]', 'open', '2026-11-14T05:00:00Z')`)
	s.Error(err)
}

func (s *AskRepositoryTestSuite) TestAskOfUnknownRunIsRefused() {
	_, err := s.asks.Create(s.Ctx, repository.CreateAskData{IssueKey: "ACM-9", Question: "q", Now: s.Clock.Now()})
	s.Error(err)
}

func (s *AskRepositoryTestSuite) TestConcurrentAnswersOfOneAskSucceedOnce() {
	const racers = 16
	first := s.ask("ACM-1", "first?")
	s.ask("ACM-1", "second?")

	start := make(chan struct{})
	errs := make(chan error, racers)
	for range racers {
		go func() {
			<-start
			errs <- s.asks.Answer(s.Ctx, first.ID, domain.AskAnswer{Text: "a", By: domain.AnsweredBySlack}, s.Clock.Now())
		}()
	}
	close(start)

	succeeded, notOpen := 0, 0
	for range racers {
		err := <-errs
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, domain.ErrNotOpen):
			notOpen++
		default:
			s.Failf("unexpected error", "%v", err)
		}
	}
	s.Equal(1, succeeded)
	s.Equal(racers-1, notOpen)

	open, err := s.asks.FindOpen(s.Ctx, "ACM-1")
	s.Require().NoError(err)
	s.Equal("second?", open.Question)
}
