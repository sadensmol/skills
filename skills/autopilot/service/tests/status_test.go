package tests

import (
	"context"
	"errors"
	"path/filepath"
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
	statusOwnerID   = "U-OWNER"
	statusChannelID = "C1"
	statusThreadTS  = "1700002000.001000"
	statusRunKey    = domain.IssueKey("ACM-9")
	manualDeploy    = "run make release"
)

type statusBase struct {
	Suite
	slack  *FakeSlack
	linear *FakeLinear
	runs   *repository.Run
	asks   *repository.Ask
	// current is the status name the fake Linear reports for the issue.
	current string
	base    string
}

type StatusTestSuite struct {
	statusBase
}

func TestStatusTestSuite(t *testing.T) {
	suite.Run(t, new(StatusTestSuite))
}

func testRegistry() config.Registry {
	var b config.Registry
	b.Linear.Statuses = map[string]string{
		"todo": "Todo", "planning": "Planning", "implementing": "In Progress", "qa": "In QA", "review": "In Review",
		"pr": "PR Open", "blocked": "Blocked", "done": "Done", "canceled": "Canceled",
	}
	b.Projects = []config.Project{{SlackChannel: "widgets", LinearTeam: "ACM", Repos: []config.Repo{{
		Name: "acme-api", DefaultBranch: "main", Deploy: config.RepoDeploy{Manual: manualDeploy, Timeout: time.Hour},
	}}}}
	return b
}

func (s *statusBase) SetupTest() {
	s.Suite.SetupTest()
	s.base = s.T().TempDir()
	s.T().Setenv("FAKE_AGTERM_DIR", s.T().TempDir())
	s.current = "In Progress"
	s.slack = NewFakeSlack(s.T())
	s.linear = NewFakeLinear(s.T())
	s.linear.On("IssueByKey", func(map[string]any) LinearReply {
		return LinearReply{Data: map[string]any{"issue": map[string]any{
			"id": "iss-1", "identifier": "ACM-9", "title": "Add a health endpoint", "url": "https://linear.example/ACM-9",
			"branchName": "kim/acm-9", "state": map[string]any{"name": s.current},
		}}}
	})
	s.linear.On("TeamByKey", func(map[string]any) LinearReply {
		return LinearReply{Data: map[string]any{"teams": map[string]any{"nodes": []map[string]any{{"id": "team-1", "key": "ACM", "name": "Acme"}}}}}
	})
	nodes := []map[string]any{}
	for _, name := range []string{"Todo", "Planning", "In Progress", "In QA", "In Review", "PR Open", "Blocked", "Done", "Canceled"} {
		nodes = append(nodes, map[string]any{"id": "st-" + name, "name": name, "type": "started"})
	}
	s.linear.On("WorkflowStates", func(map[string]any) LinearReply {
		return LinearReply{Data: map[string]any{"workflowStates": map[string]any{"nodes": nodes, "pageInfo": map[string]any{"hasNextPage": false}}}}
	})
	s.linear.On("SetState", func(map[string]any) LinearReply {
		return LinearReply{Data: map[string]any{"issueUpdate": map[string]any{"success": true}}}
	})
	s.runs = repository.NewRun(s.DB)
	s.asks = repository.NewAsk(s.DB)
	s.createRun(statusRunKey, "implementing")
}

func (s *statusBase) createRun(key domain.IssueKey, phase domain.Phase) {
	_, err := s.runs.Create(s.Ctx, domain.Run{
		IssueKey: key, Product: "widgets", ChannelID: statusChannelID, RequestTS: statusThreadTS, ThreadTS: statusThreadTS, Phase: phase,
	}, s.Clock.Now())
	s.Require().NoError(err)
	s.Require().NoError(s.runs.SetSession(s.Ctx, key, "sess-1", s.base, s.Clock.Now()))
}

func (s *statusBase) newAsk() *usecase.Ask {
	fakeBin, err := filepath.Abs(filepath.Join("testdata", "fake-agtermctl.sh"))
	s.Require().NoError(err)
	return usecase.NewAsk(usecase.AskData{
		OwnerID:  statusOwnerID,
		Sessions: agterm.New(fakeBin, "/tmp/ap-test.sock"),
		Registry: testRegistry(),
		Linear:   linear.NewClient(s.linear.URL(), "k", nil),
		Slack:    slackpost.NewPoster(s.slack.API()),
		Runs:     s.runs,
		Asks:     s.asks,
		Now:      s.Clock.Now,
	})
}

func (s *statusBase) newCanceller(ask *usecase.Ask) *usecase.Canceller {
	fakeBin, err := filepath.Abs(filepath.Join("testdata", "fake-agtermctl.sh"))
	s.Require().NoError(err)
	return usecase.NewCanceller(usecase.CancellerData{
		Registry: testRegistry(),
		Linear:   linear.NewClient(s.linear.URL(), "k", nil),
		Slack:    slackpost.NewPoster(s.slack.API()),
		Runs:     s.runs,
		Asks:     ask,
		Sessions: agterm.New(fakeBin, "/tmp/ap-test.sock"),
		Guard:    ask,
		Now:      s.Clock.Now,
	})
}

func (s *statusBase) newStatus(ask *usecase.Ask, loopCap int) *usecase.Status {
	return usecase.NewStatus(usecase.StatusData{
		Registry: testRegistry(),
		LoopCap:  loopCap,
		Linear:   linear.NewClient(s.linear.URL(), "k", nil),
		Slack:    slackpost.NewPoster(s.slack.API()),
		Runs:     s.runs,
		Asks:     s.asks,
		Guard:    ask,
		Sessions: agterm.New(s.fakeAgterm(), "/tmp/ap-test.sock"),
		Now:      s.Clock.Now,
	})
}

func (s *statusBase) fakeAgterm() string {
	fakeBin, err := filepath.Abs(filepath.Join("testdata", "fake-agtermctl.sh"))
	s.Require().NoError(err)
	return fakeBin
}

func (s *statusBase) stateChanges() []string {
	var out []string
	for _, r := range s.linear.Requests() {
		if r.Operation == "SetState" {
			out = append(out, r.Variables["stateId"].(string))
		}
	}
	return out
}

func (s *statusBase) texts() []string {
	var out []string
	for _, p := range s.slack.Posts() {
		s.Equal(statusThreadTS, p.ThreadTS)
		out = append(out, p.Text)
	}
	return out
}

func (s *statusBase) phase() domain.Phase {
	run, err := s.runs.FindByKey(s.Ctx, statusRunKey)
	s.Require().NoError(err)
	return run.Phase
}

func (s *statusBase) move(uc *usecase.Status, phase string) {
	s.Require().NoError(uc.Move(s.Ctx, statusRunKey, phase))
}

func (s *StatusTestSuite) TestQAPhaseSetsStatusOnceAndPostsNothing() {
	uc := s.newStatus(s.newAsk(), 3)

	s.move(uc, "qa")
	s.move(uc, "qa")

	s.Equal([]string{"st-In QA"}, s.stateChanges())
	s.Empty(s.texts())
	s.Equal(domain.Phase("qa"), s.phase())
	run, err := s.runs.FindByKey(s.Ctx, statusRunKey)
	s.Require().NoError(err)
	s.Equal(domain.Phase("implementing"), run.PreviousPhase)
}

func (s *StatusTestSuite) TestUnknownPhaseIsRefused() {
	uc := s.newStatus(s.newAsk(), 3)

	err := uc.Move(s.Ctx, statusRunKey, "shipping")

	s.Require().Error(err)
	s.Contains(err.Error(), "shipping")
	s.Empty(s.stateChanges())
	s.Empty(s.texts())
	s.Equal(domain.Phase("implementing"), s.phase())
}

func (s *StatusTestSuite) TestPhaseMovesPostNothing() {
	s.Require().NoError(s.runs.SetPR(s.Ctx, statusRunKey, "api", "https://github.example/acme/api/pull/4", s.Clock.Now()))
	uc := s.newStatus(s.newAsk(), 3)

	for _, phase := range []string{"planning", "implementing", "qa", "review", "pr"} {
		s.move(uc, phase)
	}

	s.Equal([]string{"st-Planning", "st-In Progress", "st-In QA", "st-In Review", "st-PR Open"}, s.stateChanges())
	s.Empty(s.texts())
}

func (s *StatusTestSuite) TestMoveToImplementingPostsNothing() {
	_, err := s.runs.Create(s.Ctx, domain.Run{
		IssueKey: "ACM-10", Product: "widgets", ChannelID: statusChannelID, RequestTS: "1700003000.000100", ThreadTS: statusThreadTS, Phase: "planning",
	}, s.Clock.Now())
	s.Require().NoError(err)
	s.Require().NoError(s.runs.SetSession(s.Ctx, "ACM-10", "sess-2", s.base, s.Clock.Now()))
	uc := s.newStatus(s.newAsk(), 3)

	s.Require().NoError(uc.Move(s.Ctx, "ACM-10", "implementing"))

	s.Equal([]string{"st-In Progress"}, s.stateChanges())
	s.Empty(s.texts())
}

func (s *StatusTestSuite) TestBlockedPostsTheStuckLine() {
	uc := s.newStatus(s.newAsk(), 3)

	s.move(uc, "blocked")

	s.Equal(domain.Phase("blocked"), s.phase())
	s.Equal([]string{"st-Blocked"}, s.stateChanges())
	s.Equal([]string{"⛔ ACM-9 is stuck. Resume it from the control room when it is fixed."}, s.texts())
}

func (s *StatusTestSuite) TestLoopCounters() {
	uc := s.newStatus(s.newAsk(), 3)

	s.move(uc, "qa")
	s.move(uc, "implementing") // gate loop
	s.move(uc, "qa")
	s.move(uc, "review")
	s.move(uc, "qa") // review loop
	s.move(uc, "review")
	s.move(uc, "implementing") // review loop
	s.move(uc, "pr")
	s.move(uc, "implementing") // pr loop

	loops, err := s.runs.Loops(s.Ctx, statusRunKey)
	s.Require().NoError(err)
	s.Equal(map[domain.Loop]int{domain.LoopGate: 1, domain.LoopReview: 2, domain.LoopPR: 1}, loops)
}

func (s *StatusTestSuite) TestGateLoopPastCapBlocksTheRun() {
	uc := s.newStatus(s.newAsk(), 2)
	for i := 0; i < 2; i++ {
		s.move(uc, "qa")
		s.move(uc, "implementing")
	}
	s.move(uc, "qa")

	s.move(uc, "implementing") // third gate loop, cap is 2

	s.Equal(domain.Phase("blocked"), s.phase())
	s.Equal("st-Blocked", s.stateChanges()[len(s.stateChanges())-1])
	s.Equal([]string{"⛔ ACM-9 is stuck: the gate loop ran 3 times, the cap is 2. Resume it from the control room when it is fixed."}, s.texts())
}

func (s *StatusTestSuite) TestPRLoopHasNoCap() {
	uc := s.newStatus(s.newAsk(), 1)
	for i := 0; i < 3; i++ {
		s.move(uc, "pr")
		s.move(uc, "implementing")
	}

	s.Equal(domain.Phase("implementing"), s.phase())
}

func (s *StatusTestSuite) TestStatusDuringOpenAskKeepsLinearBlocked() {
	ask := s.newAsk()
	uc := s.newStatus(ask, 3)
	_, err := s.asks.Create(s.Ctx, repository.CreateAskData{IssueKey: statusRunKey, Question: "Which database?", Now: s.Clock.Now()})
	s.Require().NoError(err)

	s.move(uc, "qa")

	s.Equal(domain.Phase("qa"), s.phase())
	s.Empty(s.stateChanges())
	s.Empty(s.texts())
}

func (s *StatusTestSuite) TestBlockedPhaseIsRefusedDuringOpenAsk() {
	uc := s.newStatus(s.newAsk(), 3)
	_, err := s.asks.Create(s.Ctx, repository.CreateAskData{IssueKey: statusRunKey, Question: "q", Now: s.Clock.Now()})
	s.Require().NoError(err)

	err = uc.Move(s.Ctx, statusRunKey, "blocked")

	s.Require().Error(err)
	s.Equal(domain.Phase("implementing"), s.phase())
}

func (s *StatusTestSuite) TestPostSendsPlainLineAndResetsTimer() {
	uc := s.newStatus(s.newAsk(), 3)
	s.Clock.Advance(20 * time.Minute)

	s.Require().NoError(uc.Post(s.Ctx, statusRunKey, "tests are running"))

	s.Equal([]string{"tests are running"}, s.texts())
	run, err := s.runs.FindByKey(s.Ctx, statusRunKey)
	s.Require().NoError(err)
	s.Equal(s.Clock.Now(), run.LastPostAt)
}

func (s *StatusTestSuite) TestEndedRunIsRefused() {
	uc := s.newStatus(s.newAsk(), 3)
	s.Require().NoError(s.runs.End(s.Ctx, statusRunKey, s.Clock.Now()))

	s.Require().Error(uc.Move(s.Ctx, statusRunKey, "qa"))
	s.Require().Error(uc.Post(s.Ctx, statusRunKey, "hello"))
	s.Empty(s.texts())
}

func (s *StatusTestSuite) TestSocketHandlers() {
	uc := s.newStatus(s.newAsk(), 3)

	moved := socket.NewStatusHandler(uc)(s.Ctx, socket.Request{Verb: socket.VerbStatus, Key: string(statusRunKey), Phase: "qa"})
	posted := socket.NewPostHandler(uc)(s.Ctx, socket.Request{Verb: socket.VerbPost, Key: string(statusRunKey), Text: "halfway"})
	refused := socket.NewStatusHandler(uc)(s.Ctx, socket.Request{Verb: socket.VerbStatus, Key: string(statusRunKey), Phase: "shipping"})

	s.Equal(socket.CodeOK, moved.Code)
	s.Equal(socket.CodeOK, posted.Code)
	s.Equal(socket.CodeError, refused.Code)
	s.Contains(refused.Error, "shipping")
	s.Equal([]string{"halfway"}, s.texts())
}

func (s *StatusTestSuite) TestCapHitDuringOpenAskBlocksWhenTheAskCloses() {
	ask := s.newAsk()
	uc := s.newStatus(ask, 1)
	s.move(uc, "qa")
	s.move(uc, "implementing") // gate loop 1, within the cap
	s.move(uc, "qa")
	_, err := s.asks.Create(s.Ctx, repository.CreateAskData{IssueKey: statusRunKey, Question: "Which database?", Now: s.Clock.Now()})
	s.Require().NoError(err)
	changesBefore := len(s.stateChanges())

	s.move(uc, "implementing") // gate loop 2 passes the cap while the ask is open

	s.Equal(domain.Phase("blocked"), s.phase())
	s.Len(s.stateChanges(), changesBefore, "the ask owns Linear while it is open")
	reply := controllerslack.Message{ChannelID: statusChannelID, UserID: statusOwnerID, Text: "pg", TS: "1700002500.000100", ThreadTS: statusThreadTS}
	s.Require().NoError(ask.ThreadReply(s.Ctx, reply))

	s.Equal(domain.Phase("blocked"), s.phase())
	changes := s.stateChanges()
	s.Require().Len(changes, changesBefore+1)
	s.Equal("st-Blocked", changes[len(changes)-1])
}

func (s *StatusTestSuite) TestBlockedRunRefusesStatusMoves() {
	uc := s.newStatus(s.newAsk(), 1)
	s.move(uc, "qa")
	s.move(uc, "implementing")
	s.move(uc, "qa")
	s.move(uc, "implementing")
	s.Require().Equal(domain.Phase("blocked"), s.phase())
	posts := len(s.texts())

	err := uc.Move(s.Ctx, statusRunKey, "implementing")

	s.Require().Error(err)
	s.Contains(err.Error(), "blocked")
	s.Equal(domain.Phase("blocked"), s.phase())
	s.Len(s.texts(), posts)
}

func (s *StatusTestSuite) TestTerminalPhasesAreRefused() {
	uc := s.newStatus(s.newAsk(), 3)

	for _, phase := range []string{"done", "canceled"} {
		err := uc.Move(s.Ctx, statusRunKey, phase)

		s.Require().Error(err)
		s.Contains(err.Error(), phase)
	}
	s.Equal(domain.Phase("implementing"), s.phase())
	s.Empty(s.texts())
	s.Empty(s.stateChanges())
}

// flakySlack fails its posts while fail is set.
type flakySlack struct {
	inner usecase.IStatusSlack
	fail  bool
}

func (f *flakySlack) Thread(ctx context.Context, channelID, threadTS, text string) error {
	if f.fail {
		return errors.New("slack is down")
	}
	return f.inner.Thread(ctx, channelID, threadTS, text)
}

func (s *StatusTestSuite) TestFailedStuckPostLeavesTheBlockRetryable() {
	slack := &flakySlack{inner: slackpost.NewPoster(s.slack.API()), fail: true}
	uc := usecase.NewStatus(usecase.StatusData{
		Registry: testRegistry(), LoopCap: 3, Linear: linear.NewClient(s.linear.URL(), "k", nil),
		Slack: slack, Runs: s.runs, Asks: s.asks, Guard: s.newAsk(), Now: s.Clock.Now,
	})
	s.Require().Error(uc.Move(s.Ctx, statusRunKey, "blocked"))

	s.Equal(domain.Phase("implementing"), s.phase())
	s.Empty(s.stateChanges())

	slack.fail = false
	s.move(uc, "blocked")

	s.Equal(domain.Phase("blocked"), s.phase())
	s.Equal([]string{"st-Blocked"}, s.stateChanges())
	s.Len(s.texts(), 1)
}
