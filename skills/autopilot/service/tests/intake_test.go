package tests

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slack-go/slack/slackevents"
	"github.com/stretchr/testify/suite"

	"github.com/sadensmol/autopilot/internal/config"
	controllerslack "github.com/sadensmol/autopilot/internal/controller/slack"
	"github.com/sadensmol/autopilot/internal/domain"
	"github.com/sadensmol/autopilot/internal/gate/profile"
	"github.com/sadensmol/autopilot/internal/linear"
	"github.com/sadensmol/autopilot/internal/repository"
	"github.com/sadensmol/autopilot/internal/slackpost"
	"github.com/sadensmol/autopilot/internal/usecase"
)

const (
	ownerID   = "UOWNER"
	botUserID = "UBOT"
	channelID = "CWIDGETS"
)

type startedRun struct {
	Run     domain.Run
	Product config.Project
}

// runStarter stands in for the run start of S7.
type runStarter struct {
	mu      sync.Mutex
	started []startedRun
	err     error
}

func (r *runStarter) Start(_ context.Context, run domain.Run, product config.Project) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.started = append(r.started, startedRun{Run: run, Product: product})
	return r.err
}

func (r *runStarter) Started() []startedRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]startedRun(nil), r.started...)
}

type IntakeTestSuite struct {
	Suite
	linear        *FakeLinear
	slack         *FakeSlack
	starter       *runStarter
	runs          *repository.Run
	product       config.Project
	repoRoot      string
	handler       *controllerslack.Handler
	intake        *usecase.Intake
	created       []map[string]any
	failRunCreate bool
}

// failingRuns makes the run store fail on Create, as a full disk would.
type failingRuns struct {
	*repository.Run
	fail *bool
}

func (f failingRuns) Create(ctx context.Context, run domain.Run, now time.Time) (domain.Run, error) {
	if *f.fail {
		return domain.Run{}, errors.New("disk full")
	}
	return f.Run.Create(ctx, run, now)
}

func (s *IntakeTestSuite) runStore() usecase.IRunStore {
	return failingRuns{Run: s.runs, fail: &s.failRunCreate}
}

func TestIntakeTestSuite(t *testing.T) {
	suite.Run(t, new(IntakeTestSuite))
}

func (s *IntakeTestSuite) SetupTest() {
	s.Suite.SetupTest()
	s.linear = NewFakeLinear(s.T())
	s.slack = NewFakeSlack(s.T())
	s.starter = &runStarter{}
	s.runs = repository.NewRun(s.DB)
	s.created = nil
	s.repoRoot = s.T().TempDir()
	s.product = config.Project{
		SlackChannel:  "widgets",
		LinearTeam:    "ACM",
		LinearProject: "widgets",
		Repos:         []config.Repo{{Name: "api", DefaultBranch: "main"}, {Name: "web", DefaultBranch: "main"}},
	}
	s.useTools("go", "uvx", "npx", "npm", "node")
	s.addRepo("api", "go.mod")
	s.addRepo("web", "go.mod")
	s.onLinear(nil)
	s.handler = s.newHandler()
}

// useTools puts only these fake tools on PATH, so the profile check does not depend on the machine.
func (s *IntakeTestSuite) useTools(names ...string) {
	dir := s.T().TempDir()
	for _, n := range names {
		s.Require().NoError(os.WriteFile(filepath.Join(dir, n), []byte("#!/bin/sh\nexit 0\n"), 0o755))
	}
	s.T().Setenv("PATH", dir)
}

// addRepo makes a repo folder that counts as a git checkout: it has a .git folder.
func (s *IntakeTestSuite) addRepo(name string, files ...string) {
	dir := filepath.Join(s.repoRoot, name)
	s.Require().NoError(os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	for _, f := range files {
		s.Require().NoError(os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644))
	}
}

func (s *IntakeTestSuite) registry() config.Registry {
	var b config.Registry
	b.Linear.Team = "ACM"
	b.Linear.DefaultLabel = "autopilot"
	b.Linear.Statuses = map[string]string{"todo": "Todo"}
	return b
}

// onLinear installs the happy replies; extra replaces or adds handlers by operation.
func (s *IntakeTestSuite) onLinear(extra map[string]func(map[string]any) LinearReply) {
	s.linear.On("Viewer", func(map[string]any) LinearReply {
		return LinearReply{Data: map[string]any{"viewer": map[string]any{"id": "u-key-owner", "name": "Key Owner", "organization": map[string]any{"urlKey": "acme"}}}}
	})
	s.linear.On("TeamByKey", func(map[string]any) LinearReply {
		return LinearReply{Data: map[string]any{"teams": map[string]any{"nodes": []map[string]any{{"id": "team-1", "key": "ACM", "name": "Acme"}}}}}
	})
	s.linear.On("WorkflowStates", func(map[string]any) LinearReply {
		return LinearReply{Data: map[string]any{"workflowStates": map[string]any{
			"nodes":    []map[string]any{{"id": "st-backlog", "name": "Backlog", "type": "backlog"}, {"id": "st-todo", "name": "Todo", "type": "unstarted"}},
			"pageInfo": map[string]any{"hasNextPage": false},
		}}}
	})
	s.linear.On("LabelByName", func(map[string]any) LinearReply {
		return LinearReply{Data: map[string]any{"issueLabels": map[string]any{"nodes": []map[string]any{{"id": "lb-1", "name": "autopilot", "team": map[string]any{"id": "team-1"}}}}}}
	})
	s.linear.On("ProjectByName", func(vars map[string]any) LinearReply {
		nodes := []map[string]any{}
		if vars["name"] == "widgets" {
			nodes = append(nodes, map[string]any{"id": "pr-widgets", "name": "widgets", "teams": map[string]any{"nodes": []map[string]any{{"id": "team-1"}}}})
		}
		return LinearReply{Data: map[string]any{"projects": map[string]any{"nodes": nodes}}}
	})
	s.linear.On("CreateIssue", func(vars map[string]any) LinearReply {
		s.created = append(s.created, vars["input"].(map[string]any))
		return LinearReply{Data: map[string]any{"issueCreate": map[string]any{"success": true, "issue": map[string]any{
			"id": "iss-1", "identifier": "ACM-7", "title": "t", "url": "https://linear.example/acme/issue/ACM-7", "branchName": "me/acm-7-t", "state": map[string]any{"name": "Todo"},
		}}}}
	})
	s.linear.On("Comment", func(map[string]any) LinearReply {
		return LinearReply{Data: map[string]any{"commentCreate": map[string]any{"success": true}}}
	})
	for op, fn := range extra {
		s.linear.On(op, fn)
	}
}

// noReplies stands in for the asks, which these tests do not use.
type noReplies struct{}

func (noReplies) ThreadReply(context.Context, controllerslack.Message) error { return nil }

func (s *IntakeTestSuite) newHandler() *controllerslack.Handler {
	client := linear.NewClient(s.linear.URL(), "fake-key", nil)
	s.intake = usecase.NewIntake(usecase.IntakeData{
		BotUserID:       botUserID,
		Registry:        s.registry(),
		ChannelProducts: map[string]config.Project{channelID: s.product},
		RepoPath:        func(repo string) string { return filepath.Join(s.repoRoot, repo) },
		Linear:          client,
		Slack:           slackpost.NewPoster(s.slack.API()),
		Runs:            s.runStore(),
		Requests:        repository.NewIntakeRequest(s.DB),
		Starter:         s.starter,
		Replies:         noReplies{},
		Usable:          profile.Usable,
		Now:             s.Clock.Now,
	})
	return controllerslack.NewHandler(s.Ctx, controllerslack.HandlerData{OwnerID: ownerID, ChannelIDs: []string{channelID}, Intake: s.intake})
}

func (s *IntakeTestSuite) send(e slackevents.MessageEvent) {
	s.handler.Message(s.Ctx, &e)
	s.handler.Wait()
}

func ownerMessage(ts, text string) slackevents.MessageEvent {
	return slackevents.MessageEvent{User: ownerID, Channel: channelID, Text: text, TimeStamp: ts}
}

func (s *IntakeTestSuite) countOps(op string) int {
	n := 0
	for _, r := range s.linear.Requests() {
		if r.Operation == op {
			n++
		}
	}
	return n
}

func (s *IntakeTestSuite) TestIntake_OwnerRequest_CreatesIssueCardAndRun() {
	s.send(ownerMessage("1700000100.000100", "Add a health endpoint to the API\nso the load balancer can probe it"))

	s.Require().Len(s.created, 1)
	in := s.created[0]
	s.Equal("team-1", in["teamId"])
	s.Equal("Add a health endpoint to the API", in["title"])
	s.Equal("st-todo", in["stateId"])
	s.Equal("u-key-owner", in["assigneeId"])
	s.Equal([]any{"lb-1"}, in["labelIds"])
	s.Equal("pr-widgets", in["projectId"])
	desc := in["description"].(string)
	s.Contains(desc, "so the load balancer can probe it")
	s.Contains(desc, "https://slack.example/archives/CWIDGETS/p1700000100000100")

	posts := s.slack.Posts()
	s.Require().Len(posts, 1)
	s.Equal(channelID, posts[0].Channel)
	s.Equal("1700000100.000100", posts[0].ThreadTS)
	s.Contains(posts[0].Blocks, `"action_id":"cancel-run"`)
	s.Contains(posts[0].Blocks, `"value":"ACM-7"`)

	run, err := s.runs.FindByKey(s.Ctx, "ACM-7")
	s.Require().NoError(err)
	s.Equal("widgets", run.Product)
	s.Equal(channelID, run.ChannelID)
	s.Equal("1700000100.000100", run.RequestTS)
	s.Equal("1700000100.000100", run.ThreadTS)
	s.Equal(posts[0].TS, run.CardTS)
	s.Equal(domain.Phase("todo"), run.Phase)

	started := s.starter.Started()
	s.Require().Len(started, 1)
	s.Equal(domain.IssueKey("ACM-7"), started[0].Run.IssueKey)
	s.Equal("widgets", started[0].Product.SlackChannel)
	s.Equal(0, s.countOps("Comment"))
}

func (s *IntakeTestSuite) TestIntake_NoLinearProject_IssueHasNoProject() {
	s.product.LinearProject = ""
	s.handler = s.newHandler()

	s.send(ownerMessage("1700000155.000155", "Add a health endpoint"))

	s.Require().Len(s.created, 1)
	s.NotContains(s.created[0], "projectId")
	s.Equal(0, s.countOps("ProjectByName"))
	s.Len(s.starter.Started(), 1)
}

func (s *IntakeTestSuite) TestIntake_LinearProjectMissing_NoIssueAndOneReply() {
	s.product.LinearProject = "gizmos"
	s.handler = s.newHandler()
	m := ownerMessage("1700000160.000160", "Add a health endpoint")

	s.send(m)
	s.send(m)

	s.Empty(s.created)
	s.Empty(s.starter.Started())
	s.Equal(0, s.CountRows("run"))
	posts := s.plainPosts()
	s.Require().Len(posts, 1, "a redelivery stays quiet")
	s.Equal("1700000160.000160", posts[0].ThreadTS)
	s.Contains(posts[0].Text, "no issue was created")
	s.Contains(posts[0].Text, "no Linear project `gizmos` in team ACM")
	s.Equal(domain.IntakeRefused, s.requestRow("1700000160.000160").State)
}

func (s *IntakeTestSuite) TestIntake_MissingPart_IsAHandledRefusalNotAnError() {
	s.product.LinearProject = "gizmos"
	s.handler = s.newHandler()

	err := s.intake.NewRequest(s.Ctx, controllerslack.Message{ChannelID: channelID, UserID: ownerID, Text: "Add a health endpoint", TS: "1700000165.000165"})

	s.Require().NoError(err)
	s.Len(s.plainPosts(), 1)
	s.Equal(domain.IntakeRefused, s.requestRow("1700000165.000165").State)
}

func (s *IntakeTestSuite) TestIntake_RepoMissing_NoIssueAndReplyNamesEveryMissingPart() {
	s.product.LinearProject = "gizmos"
	s.Require().NoError(os.RemoveAll(filepath.Join(s.repoRoot, "web")))
	s.Require().NoError(os.RemoveAll(filepath.Join(s.repoRoot, "api", ".git")))
	s.handler = s.newHandler()

	s.send(ownerMessage("1700000170.000170", "Add a health endpoint"))

	s.Empty(s.created)
	s.Empty(s.starter.Started())
	posts := s.plainPosts()
	s.Require().Len(posts, 1)
	s.Contains(posts[0].Text, "no Linear project `gizmos`")
	s.Contains(posts[0].Text, "repo folder for `api` is not a git checkout")
	s.Contains(posts[0].Text, "no repo folder for `web`")
	s.NotContains(posts[0].Text, s.repoRoot)
	s.Equal(domain.IntakeRefused, s.requestRow("1700000170.000170").State)
}

func (s *IntakeTestSuite) TestIntake_ProjectLookupFails_RetryWorksAfterRecovery() {
	fail := true
	s.onLinear(map[string]func(map[string]any) LinearReply{
		"ProjectByName": func(map[string]any) LinearReply {
			if fail {
				return LinearReply{Status: 503, Body: "down"}
			}
			return LinearReply{Data: map[string]any{"projects": map[string]any{"nodes": []map[string]any{
				{"id": "pr-widgets", "name": "widgets", "teams": map[string]any{"nodes": []map[string]any{{"id": "team-1"}}}},
			}}}}
		},
	})
	m := ownerMessage("1700000180.000180", "Add a health endpoint")

	s.send(m)

	s.Empty(s.created)
	posts := s.plainPosts()
	s.Require().Len(posts, 1)
	s.Contains(posts[0].Text, "could not create the issue")
	s.NotContains(posts[0].Text, "down")
	s.False(s.requestRow("1700000180.000180").State.Terminal())

	fail = false
	s.send(m)

	s.Require().Len(s.created, 1)
	s.Equal("pr-widgets", s.created[0]["projectId"])
	s.Len(s.starter.Started(), 1)
}

func (s *IntakeTestSuite) TestIntake_Ignored() {
	botMention := "<@" + botUserID + "> add a health endpoint"
	tests := map[string]slackevents.MessageEvent{
		"bot message":       {BotID: "B1", Channel: channelID, Text: "x", TimeStamp: "1.1"},
		"other user":        {User: "USOMEONE", Channel: channelID, Text: "x", TimeStamp: "1.2"},
		"thread reply":      {User: ownerID, Channel: channelID, Text: "x", TimeStamp: "1.4", ThreadTimeStamp: "1.3"},
		"edit":              {User: ownerID, Channel: channelID, Text: "x", TimeStamp: "1.5", SubType: "message_changed"},
		"other channel":     {User: ownerID, Channel: "COTHER", Text: "x", TimeStamp: "1.6"},
		"mentions a user":   {User: ownerID, Channel: channelID, Text: "ask <@USOMEONE> to add it", TimeStamp: "1.7"},
		"mentions linear":   {User: ownerID, Channel: channelID, Text: "<@ULINEAR|linear> create an issue", TimeStamp: "1.8"},
		"mentions a team":   {User: ownerID, Channel: channelID, Text: "<!subteam^SGROUP> please look", TimeStamp: "1.9"},
		"mentions here":     {User: ownerID, Channel: channelID, Text: "<!here> look at this", TimeStamp: "1.11"},
		"mentions channel":  {User: ownerID, Channel: channelID, Text: "<!channel|channel> look", TimeStamp: "1.12"},
		"mentions everyone": {User: ownerID, Channel: channelID, Text: "<!everyone> look", TimeStamp: "1.13"},
		"mentions both":     {User: ownerID, Channel: channelID, Text: botMention + " and <@USOMEONE>", TimeStamp: "1.10"},
	}
	for name, e := range tests {
		s.Run(name, func() {
			s.send(e)
			s.Empty(s.created)
			s.Empty(s.slack.Posts())
			s.Empty(s.starter.Started())
			s.Equal(0, s.CountRows("run"))
		})
	}
}

func (s *IntakeTestSuite) TestIntake_MentionOfAutopilotStarts() {
	s.send(ownerMessage("1700000200.000200", "<@"+botUserID+"> add a health endpoint"))

	s.Require().Len(s.created, 1)
	s.Equal("add a health endpoint", s.created[0]["title"])
	s.Len(s.starter.Started(), 1)
}

func (s *IntakeTestSuite) TestIntake_NoProfile() {
	s.Require().NoError(os.Remove(filepath.Join(s.repoRoot, "web", "go.mod")))

	s.send(ownerMessage("1700000300.000300", "Add a health endpoint"))

	s.Require().Len(s.created, 1, "the card needs the issue link")
	s.Equal("st-todo", s.created[0]["stateId"])
	s.Equal(1, s.countOps("Comment"))
	s.Equal(0, s.countOps("SetState"))
	posts := s.slack.Posts()
	s.Require().Len(posts, 1)
	s.Equal("1700000300.000300", posts[0].ThreadTS)
	s.Contains(posts[0].Text, "web")
	s.Contains(posts[0].Text, "no usable test profile")
	s.NotContains(posts[0].Text, "api:")
	s.NotContains(posts[0].Blocks, "cancel-run")
	s.Empty(s.starter.Started())
	s.Equal(0, s.CountRows("run"))
}

func (s *IntakeTestSuite) TestIntake_NoProfile_MissingRuntime() {
	s.useTools("uvx")

	s.send(ownerMessage("1700000400.000400", "Add a health endpoint"))

	s.Require().Len(s.slack.Posts(), 1)
	s.Contains(s.slack.Posts()[0].Text, `"go"`)
	s.Empty(s.starter.Started())
}

func (s *IntakeTestSuite) TestIntake_SameMessageTwice() {
	m := ownerMessage("1700000500.000500", "Add a health endpoint")

	s.send(m)
	s.send(m)

	s.Len(s.created, 1)
	s.Len(s.slack.Posts(), 1)
	s.Len(s.starter.Started(), 1)
	s.Equal(1, s.CountRows("run"))
}

func (s *IntakeTestSuite) TestIntake_SameMessageConcurrently() {
	m := ownerMessage("1700000600.000600", "Add a health endpoint")
	s.handler.Message(s.Ctx, &m)
	s.handler.Message(s.Ctx, &m)
	s.handler.Wait()

	s.Len(s.created, 1)
	s.Len(s.starter.Started(), 1)
}

func (s *IntakeTestSuite) TestIntake_SameMessageAfterRestart() {
	m := ownerMessage("1700000700.000700", "Add a health endpoint")
	s.send(m)
	s.handler = s.newHandler() // a new process: no memory, only the store

	s.send(m)

	s.Len(s.created, 1)
	s.Len(s.starter.Started(), 1)
}

func (s *IntakeTestSuite) TestIntake_LinearFails() {
	s.onLinear(map[string]func(map[string]any) LinearReply{
		"CreateIssue": func(map[string]any) LinearReply { return LinearReply{Status: 503, Body: "down"} },
	})

	s.send(ownerMessage("1700000800.000800", "Add a health endpoint"))

	posts := s.slack.Posts()
	s.Require().Len(posts, 1)
	s.Equal("1700000800.000800", posts[0].ThreadTS)
	s.Contains(posts[0].Text, "could not create the issue")
	s.NotContains(posts[0].Text, "down")
	s.Empty(s.starter.Started())
	s.Equal(0, s.CountRows("run"))
}

func (s *IntakeTestSuite) TestIntake_LinearFails_RetryWorksAfterRecovery() {
	fail := true
	s.onLinear(map[string]func(map[string]any) LinearReply{
		"Viewer": func(map[string]any) LinearReply {
			if fail {
				return LinearReply{Status: 503}
			}
			return LinearReply{Data: map[string]any{"viewer": map[string]any{"id": "u1", "name": "K", "organization": map[string]any{"urlKey": "acme"}}}}
		},
	})
	m := ownerMessage("1700000900.000900", "Add a health endpoint")
	s.send(m)
	fail = false

	s.send(m)

	s.Len(s.starter.Started(), 1)
}

func (s *IntakeTestSuite) TestIntake_StartFails_CardStaysAndErrorIsReturned() {
	s.starter.err = os.ErrPermission

	s.send(ownerMessage("1700001000.001000", "Add a health endpoint"))

	s.Equal(1, s.CountRows("run"))
	s.Len(s.slack.Posts(), 1)
}

func (s *IntakeTestSuite) restart() {
	s.handler = s.newHandler() // a new process: no memory, only the store
}

func (s *IntakeTestSuite) requestRow(ts string) domain.IntakeRequest {
	row, err := repository.NewIntakeRequest(s.DB).Claim(s.Ctx, channelID, ts, s.Clock.Now())
	s.Require().NoError(err)
	return row
}

// plainPosts are the posts without a Cancel button.
func (s *IntakeTestSuite) plainPosts() []SlackPost {
	var out []SlackPost
	for _, p := range s.slack.Posts() {
		if p.Blocks == "" {
			out = append(out, p)
		}
	}
	return out
}

func (s *IntakeTestSuite) noCancelCard() {
	for _, p := range s.slack.Posts() {
		s.NotContains(p.Blocks, "cancel-run")
	}
}

func (s *IntakeTestSuite) TestIntake_RowIsClaimedBeforeTheIssue() {
	var seen domain.IntakeRequest
	s.onLinear(map[string]func(map[string]any) LinearReply{
		"CreateIssue": func(map[string]any) LinearReply {
			seen = s.requestRow("1700001100.001100")
			return LinearReply{Status: 503}
		},
	})

	s.send(ownerMessage("1700001100.001100", "Add a health endpoint"))

	s.Equal(domain.IntakeClaimed, seen.State)
}

func (s *IntakeTestSuite) TestIntake_LinearFails_RestartRetries() {
	fail := true
	s.onLinear(map[string]func(map[string]any) LinearReply{
		"CreateIssue": func(map[string]any) LinearReply {
			if fail {
				return LinearReply{Status: 503}
			}
			s.created = append(s.created, map[string]any{})
			return LinearReply{Data: map[string]any{"issueCreate": map[string]any{"success": true, "issue": map[string]any{
				"id": "iss-1", "identifier": "ACM-7", "title": "t", "url": "https://linear.example/acme/issue/ACM-7", "state": map[string]any{"name": "Todo"},
			}}}}
		},
	})
	m := ownerMessage("1700001200.001200", "Add a health endpoint")
	s.send(m)
	fail = false
	s.restart()

	s.send(m)

	s.Len(s.created, 1)
	s.Len(s.starter.Started(), 1)
}

func (s *IntakeTestSuite) TestIntake_CardFails_TellsThreadAndRestartDoesNotDuplicate() {
	s.slack.RejectCards = true
	m := ownerMessage("1700001300.001300", "Add a health endpoint")

	s.send(m)

	s.Len(s.created, 1)
	plain := s.plainPosts()
	s.Require().Len(plain, 1)
	s.Equal(m.TimeStamp, plain[0].ThreadTS)
	s.Contains(plain[0].Text, "ACM-7")
	s.Contains(plain[0].Text, "could not post the card")
	s.Equal(1, s.countOps("Comment"))
	s.noCancelCard()
	s.Empty(s.starter.Started())
	runs, err := s.runs.ListActive(s.Ctx)
	s.Require().NoError(err)
	s.Empty(runs)

	s.slack.RejectCards = false
	s.restart()
	s.send(m)

	s.Len(s.created, 1)
	s.Empty(s.starter.Started())
	s.noCancelCard()
}

func (s *IntakeTestSuite) TestIntake_RunStoreFails_NoCancelCardAndRestartDoesNotDuplicate() {
	s.failRunCreate = true
	m := ownerMessage("1700001400.001400", "Add a health endpoint")

	s.send(m)

	s.Len(s.created, 1)
	plain := s.plainPosts()
	s.Require().Len(plain, 1)
	s.Contains(plain[0].Text, "could not store the run")
	s.Equal(1, s.countOps("Comment"))
	s.noCancelCard()
	s.Empty(s.starter.Started())
	s.Equal(0, s.CountRows("run"))

	s.failRunCreate = false
	s.restart()
	s.send(m)

	s.Len(s.created, 1)
	s.Empty(s.starter.Started())
	s.Equal(0, s.CountRows("run"))
}

func (s *IntakeTestSuite) TestIntake_NoProfile_RestartDoesNotDuplicate() {
	s.Require().NoError(os.Remove(filepath.Join(s.repoRoot, "web", "go.mod")))
	m := ownerMessage("1700001500.001500", "Add a health endpoint")
	s.send(m)
	s.restart()

	s.send(m)

	s.Len(s.created, 1)
	s.Len(s.slack.Posts(), 1)
	s.Equal(1, s.countOps("Comment"))
}

func (s *IntakeTestSuite) TestIntake_ResumesAfterCrashWithIssue() {
	// the process died after the issue was recorded: the row holds the issue, no card, no run
	requests := repository.NewIntakeRequest(s.DB)
	ts := "1700001600.001600"
	_, err := requests.Claim(s.Ctx, channelID, ts, s.Clock.Now())
	s.Require().NoError(err)
	s.Require().NoError(requests.SetIssue(s.Ctx, channelID, ts, domain.IntakeIssue{ID: "iss-9", Key: "ACM-9", Title: "Earlier title", URL: "https://linear.example/acme/issue/ACM-9"}, s.Clock.Now()))

	s.send(ownerMessage(ts, "Add a health endpoint"))

	s.Empty(s.created, "the issue exists already")
	s.Equal(0, s.countOps("CreateIssue"))
	s.Require().Len(s.slack.Posts(), 1)
	s.Contains(s.slack.Posts()[0].Blocks, `"value":"ACM-9"`)
	run, err := s.runs.FindByKey(s.Ctx, "ACM-9")
	s.Require().NoError(err)
	s.Equal(s.slack.Posts()[0].TS, run.CardTS)
	s.Len(s.starter.Started(), 1)
}

func (s *IntakeTestSuite) TestIntake_ResumesAfterCrashWithRunAndNoCard() {
	ts := "1700001700.001700"
	s.Require().NoError(s.crashAfterRun(ts))

	s.send(ownerMessage(ts, "Add a health endpoint"))

	s.Equal(0, s.countOps("CreateIssue"))
	s.Require().Len(s.slack.Posts(), 1)
	run, err := s.runs.FindByKey(s.Ctx, "ACM-9")
	s.Require().NoError(err)
	s.Equal(s.slack.Posts()[0].TS, run.CardTS)
	s.Len(s.starter.Started(), 1)
}

func (s *IntakeTestSuite) crashAfterRun(ts string) error {
	requests := repository.NewIntakeRequest(s.DB)
	if _, err := requests.Claim(s.Ctx, channelID, ts, s.Clock.Now()); err != nil {
		return err
	}
	issue := domain.IntakeIssue{ID: "iss-9", Key: "ACM-9", Title: "Earlier title", URL: "https://linear.example/acme/issue/ACM-9"}
	if err := requests.SetIssue(s.Ctx, channelID, ts, issue, s.Clock.Now()); err != nil {
		return err
	}
	_, err := s.runs.Create(s.Ctx, domain.Run{IssueKey: "ACM-9", Product: "widgets", ChannelID: channelID, RequestTS: ts, ThreadTS: ts, Phase: "todo"}, s.Clock.Now())
	return err
}

func (s *IntakeTestSuite) TestIntake_DescriptionIsPlainMarkdown() {
	text := "<@" + botUserID + "> see <https://example.com/a?b=1|the doc> and <https://example.com/x> &lt;b&gt; <#C1|general>\n```\nunbalanced fence"

	s.send(ownerMessage("1700001800.001800", text))

	s.Require().Len(s.created, 1)
	desc := s.created[0]["description"].(string)
	s.True(strings.HasPrefix(desc, "Slack: https://slack.example/archives/CWIDGETS/p1700001800001800"), desc)
	s.Contains(desc, "the doc")
	s.NotContains(desc, "<https")
	s.NotContains(desc, "|the doc")
	s.Contains(desc, `\<b\>`)
	s.Contains(desc, "unbalanced fence")
}

func (s *IntakeTestSuite) TestIntake_StartedRunGetsTheWorkingReaction() {
	s.send(ownerMessage("1700000100.000100", "Add a health endpoint"))

	s.Equal([]SlackReaction{{Method: "add", Name: slackpost.ReactionWorking, Channel: channelID, TS: "1700000100.000100"}}, s.slack.Reactions())
}
