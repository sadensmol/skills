package tests

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/sadensmol/autopilot/internal/agterm"
	"github.com/sadensmol/autopilot/internal/config"
	controllerslack "github.com/sadensmol/autopilot/internal/controller/slack"
	"github.com/sadensmol/autopilot/internal/controller/socket"
	"github.com/sadensmol/autopilot/internal/domain"
	"github.com/sadensmol/autopilot/internal/linear"
	"github.com/sadensmol/autopilot/internal/repository"
	"github.com/sadensmol/autopilot/internal/slackpost"
	"github.com/sadensmol/autopilot/internal/usecase"
)

const (
	askOwnerID   = "U-OWNER"
	askChannelID = "C1"
	askThreadTS  = "1700001000.001000"
	askRunKey    = domain.IssueKey("ACM-7")
)

type AskTestSuite struct {
	Suite
	fake   string
	slack  *FakeSlack
	linear *FakeLinear
	runs   *repository.Run
	asks   *repository.Ask
	uc     *usecase.Ask
	// onSleep runs after the fake clock moved; tests use it to make things happen while a caller waits.
	onSleep func(polls int)
	polls   int
}

func TestAskTestSuite(t *testing.T) {
	suite.Run(t, new(AskTestSuite))
}

func (s *AskTestSuite) SetupTest() {
	s.Suite.SetupTest()
	s.fake = s.T().TempDir()
	s.T().Setenv("FAKE_AGTERM_DIR", s.fake)
	s.onSleep = nil
	s.polls = 0
	s.slack = NewFakeSlack(s.T())
	s.linear = NewFakeLinear(s.T())
	s.linear.On("IssueByKey", func(map[string]any) LinearReply {
		return LinearReply{Data: map[string]any{"issue": map[string]any{
			"id": "iss-1", "identifier": "ACM-7", "title": "Add a health endpoint", "url": "https://linear.example/ACM-7",
			"branchName": "kim/acm-7-add-a-health-endpoint", "state": map[string]any{"name": "In Progress"},
		}}}
	})
	s.linear.On("TeamByKey", func(map[string]any) LinearReply {
		return LinearReply{Data: map[string]any{"teams": map[string]any{"nodes": []map[string]any{{"id": "team-1", "key": "ACM", "name": "Acme"}}}}}
	})
	s.linear.On("WorkflowStates", func(map[string]any) LinearReply {
		return LinearReply{Data: map[string]any{"workflowStates": map[string]any{
			"nodes": []map[string]any{
				{"id": "st-progress", "name": "In Progress", "type": "started"},
				{"id": "st-blocked", "name": "Blocked", "type": "started"},
			},
			"pageInfo": map[string]any{"hasNextPage": false},
		}}}
	})
	s.linear.On("SetState", func(map[string]any) LinearReply {
		return LinearReply{Data: map[string]any{"issueUpdate": map[string]any{"success": true}}}
	})
	s.runs = repository.NewRun(s.DB)
	s.asks = repository.NewAsk(s.DB)
	_, err := s.runs.Create(s.Ctx, domain.Run{
		IssueKey: askRunKey, Product: "widgets", ChannelID: askChannelID, RequestTS: askThreadTS, ThreadTS: askThreadTS, Phase: "implementing",
	}, s.Clock.Now())
	s.Require().NoError(err)
	s.Require().NoError(s.runs.SetSession(s.Ctx, askRunKey, "sess-1", "/tmp/base", s.Clock.Now()))
	s.uc = s.newAsk()
}

func (s *AskTestSuite) newAsk() *usecase.Ask {
	fakeBin, err := filepath.Abs(filepath.Join("testdata", "fake-agtermctl.sh"))
	s.Require().NoError(err)
	var b config.Registry
	b.Linear.Statuses = map[string]string{"implementing": "In Progress", "blocked": "Blocked"}
	b.Projects = []config.Project{{SlackChannel: "widgets", LinearTeam: "ACM"}}
	return usecase.NewAsk(usecase.AskData{
		OwnerID:  askOwnerID,
		Registry: b,
		Linear:   linear.NewClient(s.linear.URL(), "k", nil),
		Slack:    slackpost.NewPoster(s.slack.API()),
		Runs:     s.runs,
		Asks:     s.asks,
		Sessions: agterm.New(fakeBin, "/tmp/ap-test.sock"),
		Now:      s.Clock.Now,
		Sleep: func(_ context.Context, d time.Duration) error {
			s.Clock.Advance(d)
			s.polls++
			if s.onSleep != nil {
				s.onSleep(s.polls)
			}
			return nil
		},
	})
}

func (s *AskTestSuite) open(question string, options ...domain.AskOption) domain.Ask {
	ask, err := s.uc.Open(s.Ctx, askRunKey, question, options)
	s.Require().NoError(err)
	return ask
}

func (s *AskTestSuite) writeFake(name, content string) {
	s.Require().NoError(os.WriteFile(filepath.Join(s.fake, name), []byte(content), 0o644))
}

func (s *AskTestSuite) agtermCalls(sub string) [][]string {
	data, err := os.ReadFile(filepath.Join(s.fake, "argv.log"))
	if os.IsNotExist(err) {
		return nil
	}
	s.Require().NoError(err)
	var out [][]string
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		fields := strings.Split(line, "\t")
		if strings.HasPrefix(strings.Join(fields, " "), sub) {
			out = append(out, fields)
		}
	}
	return out
}

// stateChanges lists the status ids the fake Linear was asked to set, in order.
func (s *AskTestSuite) stateChanges() []string {
	var out []string
	for _, r := range s.linear.Requests() {
		if r.Operation == "SetState" {
			out = append(out, r.Variables["stateId"].(string))
		}
	}
	return out
}

func (s *AskTestSuite) threadTexts() []string {
	var out []string
	for _, p := range s.slack.Posts() {
		s.Equal(askThreadTS, p.ThreadTS)
		out = append(out, p.Text)
	}
	return out
}

func (s *AskTestSuite) ownerReply(text string) controllerslack.Message {
	return controllerslack.Message{ChannelID: askChannelID, UserID: askOwnerID, Text: text, TS: "1700001500.000100", ThreadTS: askThreadTS}
}

func (s *AskTestSuite) answerInAgterm(dialogID, body string) {
	s.writeFake("result-"+dialogID+".json", `{"ok":true,"result":`+body+`}`)
}

func (s *AskTestSuite) TestOpenPostsQuestionOpensDialogAndBlocks() {
	ask := s.open("Which database?", domain.AskOption{ID: "pg", Label: "Postgres"}, domain.AskOption{ID: "lite", Label: "SQLite"})

	s.Equal(domain.AskOpen, ask.Status)
	opens := s.agtermCalls("ask open")
	s.Require().Len(opens, 1)
	s.Contains(opens[0], "--no-block")
	s.Contains(opens[0], "sess-1")
	s.Contains(opens[0], "terminal")
	s.Contains(opens[0], "pg=Postgres")
	s.Contains(opens[0], "lite=SQLite")
	s.Require().Len(s.agtermCalls("session status"), 1)
	s.Contains(s.agtermCalls("session status")[0], "blocked")
	s.Contains(s.agtermCalls("session status")[0], "--blink")
	s.Equal([]string{"<@" + askOwnerID + "> ❓ ACM-7 needs you: Which database?\n• Postgres\n• SQLite\nReply in this thread."}, s.threadTexts())
	s.Equal([]string{"st-blocked"}, s.stateChanges())
	stored, err := s.asks.Find(s.Ctx, ask.ID)
	s.Require().NoError(err)
	s.Equal("dlg-1", stored.DialogID)
}

func (s *AskTestSuite) TestQuestionWithoutOptionsGetsATextField() {
	s.open("What name?")

	opens := s.agtermCalls("ask open")
	s.Require().Len(opens, 1)
	s.Contains(opens[0], "--input")
	s.NotContains(opens[0], "--choice")
}

func (s *AskTestSuite) TestSlackReplyFirstAnswersCancelsDialogAndRestoresStatus() {
	ask := s.open("Which database?", domain.AskOption{ID: "pg", Label: "Postgres"})
	s.onSleep = func(int) {
		s.Require().NoError(s.uc.ThreadReply(s.Ctx, s.ownerReply("pg")))
	}

	got, err := s.uc.Wait(s.Ctx, ask.ID)

	s.Require().NoError(err)
	s.Equal(domain.AskAnswered, got.Status)
	s.Equal("pg", got.Answer)
	s.Equal(domain.AnsweredBySlack, got.AnsweredBy)
	s.Equal([][]string{{"ask", "cancel", "dlg-1"}}, trimTail(s.agtermCalls("ask cancel"), 3))
	s.Equal([]string{"st-blocked", "st-progress"}, s.stateChanges())
	s.Contains(strings.Join(s.agtermCalls("session status")[len(s.agtermCalls("session status"))-1], " "), "active")
}

func (s *AskTestSuite) TestAgtermFirstAnswersAndPostsNothing() {
	ask := s.open("Which database?", domain.AskOption{ID: "pg", Label: "Postgres"})
	s.onSleep = func(int) {
		s.answerInAgterm("dlg-1", `{"result":"answered","id":"ok","label":"OK","ids":["pg"]}`)
	}

	got, err := s.uc.Wait(s.Ctx, ask.ID)

	s.Require().NoError(err)
	s.Equal("pg", got.Answer)
	s.Equal(domain.AnsweredByAgterm, got.AnsweredBy)
	s.Empty(s.agtermCalls("ask cancel"))
	s.Len(s.threadTexts(), 1, "only the question is in the thread")
	s.Equal([]string{"st-blocked", "st-progress"}, s.stateChanges())
}

func (s *AskTestSuite) TestAgtermTextFieldAnswer() {
	ask := s.open("What name?")
	s.onSleep = func(int) {
		s.answerInAgterm("dlg-1", `{"result":"answered","id":"ok","label":"OK","text":"Sky"}`)
	}

	got, err := s.uc.Wait(s.Ctx, ask.ID)

	s.Require().NoError(err)
	s.Equal("Sky", got.Answer)
}

func (s *AskTestSuite) TestNonOwnerAndOtherThreadRepliesAreIgnored() {
	ask := s.open("Which database?")

	stranger := s.ownerReply("pg")
	stranger.UserID = "U-STRANGER"
	s.Require().NoError(s.uc.ThreadReply(s.Ctx, stranger))
	elsewhere := s.ownerReply("pg")
	elsewhere.ThreadTS = "1700009999.000000"
	s.Require().NoError(s.uc.ThreadReply(s.Ctx, elsewhere))
	s.Require().NoError(s.uc.ThreadReply(s.Ctx, s.ownerReply("   ")))

	stored, err := s.asks.Find(s.Ctx, ask.ID)
	s.Require().NoError(err)
	s.Equal(domain.AskOpen, stored.Status)
	s.Equal([]string{"st-blocked"}, s.stateChanges())
	s.Empty(s.agtermCalls("ask cancel"))
	s.Empty(s.slack.Reactions())
}

func (s *AskTestSuite) TestOwnerReplyWithoutOpenAskGetsOnlyTheSeenReaction() {
	s.Require().NoError(s.uc.ThreadReply(s.Ctx, s.ownerReply("hello")))

	s.Empty(s.stateChanges())
	s.Empty(s.slack.Posts())
	s.Equal([]SlackReaction{{Method: "add", Name: slackpost.ReactionSeen, Channel: askChannelID, TS: "1700001500.000100"}}, s.slack.Reactions())
}

func (s *AskTestSuite) TestOwnerReplyThatAnswersTheAskGetsTheSeenReaction() {
	ask := s.open("Which database?")

	s.Require().NoError(s.uc.ThreadReply(s.Ctx, s.ownerReply("pg")))

	stored, err := s.asks.Find(s.Ctx, ask.ID)
	s.Require().NoError(err)
	s.Equal(domain.AskAnswered, stored.Status)
	s.Equal([]SlackReaction{{Method: "add", Name: slackpost.ReactionSeen, Channel: askChannelID, TS: "1700001500.000100"}}, s.slack.Reactions())
}

func (s *AskTestSuite) TestNonOwnerReplyGetsNoReaction() {
	stranger := s.ownerReply("hello")
	stranger.UserID = "U-STRANGER"

	s.Require().NoError(s.uc.ThreadReply(s.Ctx, stranger))

	s.Empty(s.slack.Reactions())
}

func (s *AskTestSuite) TestWaitEndsAfterEightMinutesThenWaitByIDReturnsLaterAnswer() {
	ask := s.open("Which database?")
	started := s.Clock.Now()

	_, err := s.uc.Wait(s.Ctx, ask.ID)

	s.ErrorIs(err, domain.ErrAskPending)
	s.Equal(8*time.Minute, s.Clock.Now().Sub(started))
	stored, findErr := s.asks.Find(s.Ctx, ask.ID)
	s.Require().NoError(findErr)
	s.Equal(domain.AskOpen, stored.Status)

	s.answerInAgterm("dlg-1", `{"result":"answered","id":"ok","text":"late"}`)
	got, err := s.uc.Wait(s.Ctx, ask.ID)
	s.Require().NoError(err)
	s.Equal("late", got.Answer)

	again, err := s.uc.Wait(s.Ctx, ask.ID)
	s.Require().NoError(err)
	s.Equal("late", again.Answer)
}

func (s *AskTestSuite) TestSecondAskQueuesAndOpensWhenTheFirstCloses() {
	first := s.open("first?")
	second := s.open("second?")

	s.Equal(domain.AskQueued, second.Status)
	s.Len(s.agtermCalls("ask open"), 1)
	s.Equal([]string{"st-blocked"}, s.stateChanges())

	s.Require().NoError(s.uc.ThreadReply(s.Ctx, s.ownerReply("one")))

	s.Len(s.agtermCalls("ask open"), 2)
	s.Equal([]string{"st-blocked"}, s.stateChanges())
	s.Contains(strings.Join(s.threadTexts(), "\n"), "second?")
	opened, err := s.asks.Find(s.Ctx, second.ID)
	s.Require().NoError(err)
	s.Equal(domain.AskOpen, opened.Status)
	s.Equal("dlg-2", opened.DialogID)
	done, err := s.uc.Wait(s.Ctx, first.ID)
	s.Require().NoError(err)
	s.Equal("one", done.Answer)

	s.Require().NoError(s.uc.ThreadReply(s.Ctx, s.ownerReply("two")))
	s.Equal([]string{"st-blocked", "st-progress"}, s.stateChanges())
}

func (s *AskTestSuite) TestDismissedDialogEndsTheAsk() {
	ask := s.open("Which database?")
	s.onSleep = func(int) { s.answerInAgterm("dlg-1", `{"result":"escaped"}`) }

	_, err := s.uc.Wait(s.Ctx, ask.ID)

	s.ErrorIs(err, domain.ErrAskDismissed)
	s.Equal([]string{"st-blocked", "st-progress"}, s.stateChanges())
}

func (s *AskTestSuite) TestOpenForEndedRunFails() {
	s.Require().NoError(s.runs.End(s.Ctx, askRunKey, s.Clock.Now()))

	_, err := s.uc.Open(s.Ctx, askRunKey, "q?", nil)

	s.Error(err)
	s.Empty(s.agtermCalls("ask open"))
}

func (s *AskTestSuite) TestDialogFailureCancelsTheAsk() {
	s.writeFake("open-fails", "")

	_, err := s.uc.Open(s.Ctx, askRunKey, "q?", nil)

	s.Error(err)
	_, findErr := s.asks.FindOpen(s.Ctx, askRunKey)
	s.ErrorIs(findErr, domain.ErrNotFound)
	s.Empty(s.stateChanges())
}

func (s *AskTestSuite) TestDialogRefusalCarriesAgtermsReason() {
	s.writeFake("open-pending", "")

	_, err := s.uc.Open(s.Ctx, askRunKey, "q?", nil)

	s.ErrorContains(err, "ask already pending")
}

func (s *AskTestSuite) TestAskVerbReturnsTheAnswerOrPendingWithTheID() {
	handler := socket.NewAskHandler(s.uc)
	req := socket.Request{Verb: socket.VerbAsk, Key: string(askRunKey), Text: "Which database?", Options: []socket.Option{{ID: "pg", Label: "Postgres"}}}

	pending := handler(s.Ctx, req)
	s.Equal(socket.CodePending, pending.Code)
	s.Equal("1", pending.Output)

	s.answerInAgterm("dlg-1", `{"result":"answered","id":"ok","ids":["pg"]}`)
	waited := socket.NewAskWaitHandler(s.uc)(s.Ctx, socket.Request{Verb: socket.VerbAskWait, ID: 1})
	s.Equal(socket.CodeOK, waited.Code)
	s.Equal("pg", waited.Output)

	missing := socket.NewAskWaitHandler(s.uc)(s.Ctx, socket.Request{Verb: socket.VerbAskWait, ID: 99})
	s.Equal(socket.CodeError, missing.Code)
	s.Contains(missing.Error, "99")
}

func (s *AskTestSuite) TestIntakeThreadReplyGoesToTheAsk() {
	ask := s.open("Which database?")
	intake := usecase.NewIntake(usecase.IntakeData{Replies: s.uc})

	s.Require().NoError(intake.ThreadReply(s.Ctx, s.ownerReply("pg")))

	stored, err := s.asks.Find(s.Ctx, ask.ID)
	s.Require().NoError(err)
	s.Equal(domain.AskAnswered, stored.Status)
}

// inParallel runs the functions at once and waits for all of them.
func inParallel(fns ...func()) {
	var wg sync.WaitGroup
	start := make(chan struct{})
	for _, fn := range fns {
		wg.Go(func() {
			<-start
			fn()
		})
	}
	close(start)
	wg.Wait()
}

func (s *AskTestSuite) TestTwoRepliesAtOnceAnswerTheTwoAsksInOrder() {
	first := s.open("first?")
	second := s.open("second?")

	inParallel(
		func() { s.NoError(s.uc.ThreadReply(s.Ctx, s.ownerReply("a"))) },
		func() { s.NoError(s.uc.ThreadReply(s.Ctx, s.ownerReply("b"))) },
	)

	a, err := s.asks.Find(s.Ctx, first.ID)
	s.Require().NoError(err)
	b, err := s.asks.Find(s.Ctx, second.ID)
	s.Require().NoError(err)
	s.Equal(domain.AskAnswered, a.Status)
	s.Equal(domain.AskAnswered, b.Status)
	s.Equal("dlg-2", b.DialogID)
	s.Len(s.agtermCalls("ask open"), 2)
	s.Len(s.agtermCalls("ask cancel"), 2)
	s.Equal([]string{"st-blocked", "st-progress"}, s.stateChanges())
	statuses := s.agtermCalls("session status")
	s.Contains(statuses[len(statuses)-1], "active")
}

func (s *AskTestSuite) TestAgtermAndSlackAnswerAtOnceShowTheNextAskOnce() {
	first := s.open("first?")
	second := s.open("second?")
	s.answerInAgterm("dlg-1", `{"result":"answered","id":"ok","text":"from agterm"}`)

	inParallel(
		func() {
			_, err := s.uc.Wait(s.Ctx, first.ID)
			s.NoError(err)
		},
		func() { s.NoError(s.uc.ThreadReply(s.Ctx, s.ownerReply("from slack"))) },
	)

	a, err := s.asks.Find(s.Ctx, first.ID)
	s.Require().NoError(err)
	s.Equal(domain.AskAnswered, a.Status)
	b, err := s.asks.Find(s.Ctx, second.ID)
	s.Require().NoError(err)
	// the Slack reply may answer the second ask after the first closed; either way every dialog is shown once
	s.Len(s.agtermCalls("ask open"), 2)
	s.Equal("dlg-2", b.DialogID)
	if b.Status == domain.AskOpen {
		s.Equal([]string{"st-blocked"}, s.stateChanges())
	} else {
		s.Equal([]string{"st-blocked", "st-progress"}, s.stateChanges())
	}
}

func (s *AskTestSuite) TestReplyToEndedRunIsIgnored() {
	ask := s.open("Which database?")
	s.Require().NoError(s.runs.End(s.Ctx, askRunKey, s.Clock.Now()))

	s.Require().NoError(s.uc.ThreadReply(s.Ctx, s.ownerReply("pg")))

	stored, err := s.asks.Find(s.Ctx, ask.ID)
	s.Require().NoError(err)
	s.Equal(domain.AskOpen, stored.Status)
	s.Equal([]string{"st-blocked"}, s.stateChanges())
}

func (s *AskTestSuite) TestAgtermAnswerAfterRunEndedKeepsTheFinalStatus() {
	ask := s.open("Which database?")
	s.Require().NoError(s.runs.End(s.Ctx, askRunKey, s.Clock.Now()))
	s.answerInAgterm("dlg-1", `{"result":"answered","id":"ok","text":"pg"}`)

	got, err := s.uc.Wait(s.Ctx, ask.ID)

	s.Require().NoError(err)
	s.Equal("pg", got.Answer)
	s.Equal([]string{"st-blocked"}, s.stateChanges())
	s.Len(s.agtermCalls("session status"), 1)
	s.Len(s.threadTexts(), 1)
}

func (s *AskTestSuite) TestCloseAllForRunCancelsOpenAndQueuedAsks() {
	first := s.open("first?")
	second := s.open("second?")
	third := s.open("third?")

	s.Require().NoError(s.uc.CloseAllForRun(s.Ctx, askRunKey))

	for _, id := range []domain.AskID{first.ID, second.ID, third.ID} {
		stored, err := s.asks.Find(s.Ctx, id)
		s.Require().NoError(err)
		s.Equal(domain.AskCancelled, stored.Status)
	}
	s.Equal([][]string{{"ask", "cancel", "dlg-1"}}, trimTail(s.agtermCalls("ask cancel"), 3))
	s.Equal([]string{"st-blocked"}, s.stateChanges())
	_, err := s.uc.Wait(s.Ctx, second.ID)
	s.ErrorIs(err, domain.ErrAskDismissed)
}

func trimTail(calls [][]string, n int) [][]string {
	out := make([][]string, 0, len(calls))
	for _, c := range calls {
		out = append(out, c[:min(n, len(c))])
	}
	return out
}
