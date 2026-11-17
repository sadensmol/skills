package tests

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/sadensmol/autopilot/internal/agterm"
	"github.com/sadensmol/autopilot/internal/config"
	"github.com/sadensmol/autopilot/internal/controller/socket"
	"github.com/sadensmol/autopilot/internal/domain"
	"github.com/sadensmol/autopilot/internal/gh"
	"github.com/sadensmol/autopilot/internal/git"
	"github.com/sadensmol/autopilot/internal/linear"
	"github.com/sadensmol/autopilot/internal/repository"
	"github.com/sadensmol/autopilot/internal/slackpost"
	"github.com/sadensmol/autopilot/internal/usecase"
)

const (
	prURL     = "https://github.com/acme/acme-api/pull/7"
	prRepo    = "acme-api"
	pointer   = "PR feedback on " + prURL + ": run the follow-up loop"
	typeVerb  = "session\ttype"
	closeVerb = "session\tclose"

	prBranch     = "acm-9-health"
	mergedView   = `"headRefName":"` + prBranch + `","mergeCommit":{"oid":"abc1234def5678"},"reviews":[],"comments":[],"statusCheckRollup":[]`
	branchDelete = "api\t-X\tDELETE\trepos/acme/acme-api/git/refs/heads/" + prBranch + "\n"

	prCreated     = "2026-11-14T04:30:00Z"
	laterThanOpen = "2026-11-14T05:10:00Z"
	muchLater     = "2026-11-14T05:20:00Z"
	beforeOpen    = "2026-11-14T04:00:00Z"

	treeActive    = `{"ok":true,"result":{"windows":[{"workspaces":[{"name":"acme","sessions":[{"id":"sess-1","name":"x","realized":true,"status":"active"}]}]}]}}`
	treeIdle      = `{"ok":true,"result":{"windows":[{"workspaces":[{"name":"acme","sessions":[{"id":"sess-1","name":"x","realized":true,"status":"completed"}]}]}]}}`
	treeUnrealize = `{"ok":true,"result":{"windows":[{"workspaces":[{"name":"acme","sessions":[{"id":"sess-1","name":"x","realized":false}]}]}]}}`
)

type PRWatchTestSuite struct {
	statusBase
	ghDir    string
	agterm   string
	repoRoot string
	registry config.Registry
	verdicts *repository.Verdict
}

func TestPRWatchTestSuite(t *testing.T) {
	suite.Run(t, new(PRWatchTestSuite))
}

func (s *PRWatchTestSuite) SetupTest() {
	s.statusBase.SetupTest()
	s.ghDir = s.T().TempDir()
	s.agterm = os.Getenv("FAKE_AGTERM_DIR")
	s.repoRoot = s.T().TempDir()
	s.registry = testRegistry()
	s.verdicts = repository.NewVerdict(s.DB)
	s.writeAgterm("tree.json", treeIdle)
	s.view("OPEN", `"reviews":[],"comments":[],"statusCheckRollup":[]`)
}

func (s *PRWatchTestSuite) writeAgterm(name, body string) {
	s.Require().NoError(os.WriteFile(filepath.Join(s.agterm, name), []byte(body), 0o644))
}

func (s *PRWatchTestSuite) writeGH(name, body string) {
	s.Require().NoError(os.WriteFile(filepath.Join(s.ghDir, name), []byte(body), 0o644))
}

func (s *PRWatchTestSuite) view(state, extra string) {
	s.writeGH("view-7.json", `{"state":"`+state+`","headRefOid":"abc","createdAt":"`+prCreated+`",`+extra+`}`)
}

func (s *PRWatchTestSuite) fakeBin(name string) string {
	bin, err := filepath.Abs(filepath.Join("testdata", name))
	s.Require().NoError(err)
	return bin
}

func (s *PRWatchTestSuite) newWatch(ask *usecase.Ask) *usecase.PRWatch {
	return s.newWatchWithTimeout(ask, time.Second)
}

func (s *PRWatchTestSuite) newWatchWithTimeout(ask *usecase.Ask, timeout time.Duration) *usecase.PRWatch {
	s.T().Setenv("FAKE_GH_DIR", s.ghDir)
	return usecase.NewPRWatch(usecase.PRWatchData{
		Registry:  s.registry,
		Interval:  5 * time.Minute,
		Linear:    linear.NewClient(s.linear.URL(), "k", nil),
		OwnerID:   statusOwnerID,
		Slack:     slackpost.NewPoster(s.slack.API()),
		Runs:      s.runs,
		Asks:      ask,
		Sessions:  agterm.New(s.fakeBin("fake-agtermctl.sh"), "/tmp/ap-test.sock"),
		GH:        gh.New(s.fakeBin("fake-gh.sh"), timeout),
		Worktrees: git.New(),
		RepoPath:  func(repo string) string { return filepath.Join(s.repoRoot, repo) },
		Guard:     ask,
		Now:       s.Clock.Now,
	})
}

func (s *PRWatchTestSuite) newOpen(ask *usecase.Ask) *usecase.PROpen {
	return s.newOpenWith(ask, testRegistry())
}

func (s *PRWatchTestSuite) newOpenWith(ask *usecase.Ask, registry config.Registry) *usecase.PROpen {
	s.T().Setenv("FAKE_GH_DIR", s.ghDir)
	return usecase.NewPROpen(usecase.PROpenData{
		Registry: registry,
		GH:       gh.New(s.fakeBin("fake-gh.sh"), time.Second),
		OwnerID:  statusOwnerID,
		Slack:    slackpost.NewPoster(s.slack.API()),
		Runs:     s.runs,
		Verdicts: s.verdicts,
		Guard:    ask,
		Now:      s.Clock.Now,
	})
}

func (s *PRWatchTestSuite) openPR(ask *usecase.Ask) {
	s.Require().NoError(s.newOpen(ask).Open(s.Ctx, statusRunKey, prURL))
}

func (s *PRWatchTestSuite) agtermLog() string {
	data, err := os.ReadFile(filepath.Join(s.agterm, "argv.log"))
	if errors.Is(err, os.ErrNotExist) {
		return ""
	}
	s.Require().NoError(err)
	return string(data)
}

func (s *PRWatchTestSuite) lastPost() string {
	posts := s.slack.Posts()
	s.Require().NotEmpty(posts)
	return posts[len(posts)-1].Text
}

func (s *PRWatchTestSuite) TestReviewCommentTypesOnePointerLineWhenSessionIsNotActive() {
	ask := s.newAsk()
	s.openPR(ask)
	watch := s.newWatch(ask)
	s.view("OPEN", `"reviews":[{"state":"COMMENTED","submittedAt":"`+laterThanOpen+`"}],"comments":[],"statusCheckRollup":[]`)

	s.Require().NoError(watch.Tick(s.Ctx))
	s.Require().NoError(watch.Tick(s.Ctx))

	s.Equal(1, strings.Count(s.agtermLog(), typeVerb), "the same feedback is pointed at once")
	s.Contains(s.agtermLog(), "--target\tsess-1\t"+pointer+"\n")
	s.NotContains(s.agtermLog(), closeVerb)
	run, err := s.runs.FindByKey(s.Ctx, statusRunKey)
	s.Require().NoError(err)
	s.True(run.EndedAt.IsZero())
}

func (s *PRWatchTestSuite) TestNewerFeedbackPointsAgain() {
	ask := s.newAsk()
	s.openPR(ask)
	watch := s.newWatch(ask)
	s.view("OPEN", `"reviews":[{"state":"COMMENTED","submittedAt":"`+laterThanOpen+`"}],"comments":[],"statusCheckRollup":[]`)
	s.Require().NoError(watch.Tick(s.Ctx))

	s.view("OPEN", `"reviews":[{"state":"COMMENTED","submittedAt":"`+laterThanOpen+`"}],"comments":[{"createdAt":"`+muchLater+`"}],"statusCheckRollup":[]`)
	s.Require().NoError(watch.Tick(s.Ctx))

	s.Equal(2, strings.Count(s.agtermLog(), typeVerb))
}

func (s *PRWatchTestSuite) TestActiveSessionIsNotTypedIntoAndTheFeedbackWaits() {
	ask := s.newAsk()
	s.openPR(ask)
	watch := s.newWatch(ask)
	s.view("OPEN", `"reviews":[],"comments":[{"createdAt":"`+laterThanOpen+`"}],"statusCheckRollup":[]`)
	s.writeAgterm("tree.json", treeActive)

	s.Require().NoError(watch.Tick(s.Ctx))
	s.NotContains(s.agtermLog(), typeVerb)

	s.writeAgterm("tree.json", treeIdle)
	s.Require().NoError(watch.Tick(s.Ctx))
	s.Equal(1, strings.Count(s.agtermLog(), typeVerb))
}

func (s *PRWatchTestSuite) TestUnrealizedSessionIsNotTypedInto() {
	ask := s.newAsk()
	s.openPR(ask)
	watch := s.newWatch(ask)
	s.view("OPEN", `"reviews":[],"comments":[{"createdAt":"`+laterThanOpen+`"}],"statusCheckRollup":[]`)
	s.writeAgterm("tree.json", treeUnrealize)

	s.Require().NoError(watch.Tick(s.Ctx))

	s.NotContains(s.agtermLog(), typeVerb)
}

func (s *PRWatchTestSuite) TestInlineReviewCommentFromTheApiTypesPointer() {
	ask := s.newAsk()
	s.openPR(ask)
	watch := s.newWatch(ask)
	s.view("OPEN", `"reviews":[],"comments":[],"statusCheckRollup":[]`)
	s.writeGH("comments-7.json", `[{"id":1,"created_at":"`+laterThanOpen+`"}]`)

	s.Require().NoError(watch.Tick(s.Ctx))

	s.Equal(1, strings.Count(s.agtermLog(), typeVerb))
	s.Contains(s.ghLog(), "api\t--paginate\trepos/acme/acme-api/pulls/7/comments")
}

func (s *PRWatchTestSuite) ghLog() string {
	data, err := os.ReadFile(filepath.Join(s.ghDir, "argv.log"))
	s.Require().NoError(err)
	return string(data)
}

func (s *PRWatchTestSuite) TestFailedCheckTypesPointerButPassingAndOldOnesDoNot() {
	ask := s.newAsk()
	s.openPR(ask)
	watch := s.newWatch(ask)
	s.view("OPEN", `"reviews":[{"state":"APPROVED","submittedAt":"`+laterThanOpen+`"}],"comments":[],"statusCheckRollup":[`+
		`{"conclusion":"SUCCESS","completedAt":"`+laterThanOpen+`"},{"conclusion":"FAILURE","completedAt":"`+beforeOpen+`"}]`)
	s.Require().NoError(watch.Tick(s.Ctx))
	s.NotContains(s.agtermLog(), typeVerb)

	s.view("OPEN", `"reviews":[],"comments":[],"statusCheckRollup":[{"conclusion":"FAILURE","completedAt":"`+laterThanOpen+`"}]`)
	s.Require().NoError(watch.Tick(s.Ctx))
	s.Equal(1, strings.Count(s.agtermLog(), typeVerb))
}

func (s *PRWatchTestSuite) TestMergedMovesIssueToDoneNotesThreadClosesSessionsAndEndsRun() {
	ask := s.newAsk()
	s.openPR(ask)
	opened, err := ask.Open(s.Ctx, statusRunKey, "Which flag?", nil)
	s.Require().NoError(err)
	s.Require().NotZero(opened.ID)
	s.Require().NoError(os.MkdirAll(filepath.Join(s.base, "run"), 0o755))
	s.Require().NoError(os.WriteFile(filepath.Join(s.base, "run", "engine.json"),
		[]byte(`{"key":"ACM-9","parent":"sess-1","waves":[{"status":"complete","parts":[{"session_id":"part-1"},{"session_id":""}]}]}`), 0o644))
	watch := s.newWatch(ask)
	s.view("MERGED", `"reviews":[],"comments":[],"statusCheckRollup":[]`)
	before := len(s.slack.Posts())

	s.Require().NoError(watch.Tick(s.Ctx))

	s.Equal([]string{"st-Blocked", "st-Done"}, s.stateChanges(), "the open ask blocked the issue, the merge sets it done")
	s.Equal([]string{"✅ ACM-9 done. Deploy it by hand: run make release."}, s.texts()[before:], "the merge posts only the done line")
	s.Contains(s.slack.Reactions(), SlackReaction{Method: "add", Name: slackpost.ReactionDone, Channel: statusChannelID, TS: statusThreadTS})
	log := s.agtermLog()
	s.Contains(log, closeVerb+"\t--socket\t/tmp/ap-test.sock\t--target\tpart-1")
	s.Contains(log, closeVerb+"\t--socket\t/tmp/ap-test.sock\t--target\tsess-1")
	run, err := s.runs.FindByKey(s.Ctx, statusRunKey)
	s.Require().NoError(err)
	s.False(run.EndedAt.IsZero())
	s.Equal(domain.Phase("done"), run.Phase)
	_, err = s.asks.FindOpen(s.Ctx, statusRunKey)
	s.True(errors.Is(err, domain.ErrNotFound), "the ask of an ended run is closed")

	posts := len(s.slack.Posts())
	s.Require().NoError(watch.Tick(s.Ctx))
	s.Len(s.slack.Posts(), posts, "an ended run is not watched again")
}

func (s *PRWatchTestSuite) TestMergedDoesNotSetDoneWhenLinearIsAlreadyDone() {
	ask := s.newAsk()
	s.openPR(ask)
	watch := s.newWatch(ask)
	s.current = "Done"
	s.view("MERGED", `"reviews":[],"comments":[],"statusCheckRollup":[]`)

	s.Require().NoError(watch.Tick(s.Ctx))

	s.Empty(s.stateChanges())
	run, err := s.runs.FindByKey(s.Ctx, statusRunKey)
	s.Require().NoError(err)
	s.False(run.EndedAt.IsZero())
}

func (s *PRWatchTestSuite) TestClosedEndsRunWithANoteAndLeavesLinearAlone() {
	ask := s.newAsk()
	s.openPR(ask)
	watch := s.newWatch(ask)
	s.view("CLOSED", `"reviews":[],"comments":[],"statusCheckRollup":[]`)

	s.Require().NoError(watch.Tick(s.Ctx))

	s.Empty(s.stateChanges())
	s.Equal("🚫 PR closed without a merge: "+prURL, s.lastPost())
	s.Contains(s.agtermLog(), closeVerb)
	run, err := s.runs.FindByKey(s.Ctx, statusRunKey)
	s.Require().NoError(err)
	s.False(run.EndedAt.IsZero())
	s.Equal(domain.Phase("implementing"), run.Phase)
}

func (s *PRWatchTestSuite) TestFailedGHCallLeavesTheRunWatched() {
	ask := s.newAsk()
	s.openPR(ask)
	watch := s.newWatch(ask)
	s.writeGH("view-fails", "")

	s.Error(watch.Tick(s.Ctx))

	run, err := s.runs.FindByKey(s.Ctx, statusRunKey)
	s.Require().NoError(err)
	s.True(run.EndedAt.IsZero())
}

func (s *PRWatchTestSuite) TestPROpenPostsTheManualChecksWithAMention() {
	ask := s.newAsk()
	s.Require().NoError(s.verdicts.Save(s.Ctx, domain.Verdict{
		IssueKey: statusRunKey, Repo: prRepo, SHA: "abc",
		Checks:      []domain.CheckResult{{Name: "tests", Status: domain.CheckPass}},
		Manual:      []string{"AC-3: open the page on a phone"},
		RedReProofs: []string{"re-proof at def changed tests/health_test.go"},
	}, s.Clock.Now()))

	s.openPR(ask)

	s.Equal("<@"+statusOwnerID+"> ✅ PR ready: "+prURL+"\nCheck by hand before you merge:\n• AC-3: open the page on a phone", s.lastPost())
	repos, err := s.runs.Repos(s.Ctx, statusRunKey)
	s.Require().NoError(err)
	s.Require().Len(repos, 1)
	s.Equal(prURL, repos[0].PRURL)
	s.Equal("open", repos[0].Watch.State)
	s.False(repos[0].Watch.CheckedAt.IsZero())
}

func (s *PRWatchTestSuite) TestPROpenWithoutManualChecksIsOnlyTheLinkWithAMentionAndTheButtons() {
	s.Require().NoError(s.verdicts.Save(s.Ctx, domain.Verdict{
		IssueKey: statusRunKey, Repo: prRepo, SHA: "abc",
		Checks:      []domain.CheckResult{{Name: "tests", Status: domain.CheckPass}},
		RedReProofs: []string{"re-proof at def changed tests/health_test.go"},
	}, s.Clock.Now()))

	s.openPR(s.newAsk())

	post := s.slack.Posts()[len(s.slack.Posts())-1]
	s.Equal("<@"+statusOwnerID+"> ✅ PR ready: "+prURL, post.Text)
	s.Contains(post.Blocks, `"action_id":"review-pr"`)
	s.Contains(post.Blocks, `"action_id":"`+slackpost.MergeAction+`"`)
}

func (s *PRWatchTestSuite) TestPROpenWithoutAVerdictStillMentionsTheOwner() {
	s.openPR(s.newAsk())

	s.Equal("<@"+statusOwnerID+"> ✅ PR ready: "+prURL, s.lastPost())
}

func (s *PRWatchTestSuite) TestPROpenProductNotInTheRegistryIsAClearError() {
	err := s.newOpenWith(s.newAsk(), config.Registry{}).Open(s.Ctx, statusRunKey, prURL)

	s.Require().Error(err)
	s.Contains(err.Error(), fmt.Sprintf("product widgets of run %s is not in the workspace registry", statusRunKey))
	s.Empty(s.slack.Posts())
}

func (s *PRWatchTestSuite) TestPROpenRefusesUrlsThatAreNotAPlainPullRequestOfTheProduct() {
	for name, raw := range map[string]string{
		"not a pull request": "https://github.com/acme/acme-api",
		"http":               "http://github.com/acme/acme-api/pull/7",
		"other host":         "https://git.example/acme/acme-api/pull/7",
		"host with port":     "https://github.com:8443/acme/acme-api/pull/7",
		"user info":          "https://u@github.com/acme/acme-api/pull/7",
		"foreign repo":       "https://github.com/acme/other-repo/pull/7",
		"encoded slash":      "https://github.com/acme/acme-api%2Fx/pull/7",
		"encoded question":   "https://github.com/acme/acme-api/pull/7%3Fx",
		"query":              "https://github.com/acme/acme-api/pull/7?x=1",
		"empty query":        "https://github.com/acme/acme-api/pull/7?",
		"fragment":           "https://github.com/acme/acme-api/pull/7#issue",
		"space":              "https://github.com/acme/acme-api/pull/7 --web",
		"extra path":         "https://github.com/acme/acme-api/pull/7/files",
		"signed number":      "https://github.com/acme/acme-api/pull/+7",
	} {
		s.Run(name, func() {
			err := s.newOpen(s.newAsk()).Open(s.Ctx, statusRunKey, raw)
			s.Error(err)
			s.Empty(s.slack.Posts())
			repos, repoErr := s.runs.Repos(s.Ctx, statusRunKey)
			s.Require().NoError(repoErr)
			s.Empty(repos)
		})
	}
}

func (s *PRWatchTestSuite) TestOnlyTheCanonicalUrlIsStoredTypedAndPosted() {
	ask := s.newAsk()
	s.Require().NoError(s.newOpen(ask).Open(s.Ctx, statusRunKey, "https://GitHub.com/acme/acme-api/pull/7/"))

	repos, err := s.runs.Repos(s.Ctx, statusRunKey)
	s.Require().NoError(err)
	s.Require().Len(repos, 1)
	s.Equal(prURL, repos[0].PRURL)
	s.Contains(s.lastPost(), prURL)
	s.view("OPEN", `"reviews":[],"comments":[{"createdAt":"`+laterThanOpen+`"}],"statusCheckRollup":[]`)
	s.Require().NoError(s.newWatch(ask).Tick(s.Ctx))
	s.Contains(s.agtermLog(), "--target\tsess-1\t"+pointer+"\n")
	s.Contains(s.ghLog(), "pr\tview\t"+prURL+"\t")
}

func (s *PRWatchTestSuite) TestACheckThatFailedBeforeThePRWasRegisteredStillPoints() {
	ask := s.newAsk()
	s.view("OPEN", `"reviews":[],"comments":[],"statusCheckRollup":[{"conclusion":"FAILURE","completedAt":"2026-11-14T04:45:00Z"}]`)
	s.openPR(ask)

	s.Require().NoError(s.newWatch(ask).Tick(s.Ctx))

	s.Equal(1, strings.Count(s.agtermLog(), typeVerb))
}

func (s *PRWatchTestSuite) TestAHungGHTimesOutAndTheOtherRunsAreStillWatched() {
	ask := s.newAsk()
	s.openPR(ask)
	const otherKey = domain.IssueKey("ACM-10")
	s.writeGH("view-8.json", `{"state":"OPEN","createdAt":"`+prCreated+`","reviews":[],"comments":[],"statusCheckRollup":[]}`)
	_, err := s.runs.Create(s.Ctx, domain.Run{
		IssueKey: otherKey, Product: "widgets", ChannelID: statusChannelID, RequestTS: "1700002001.000000", ThreadTS: "1700002001.000000", Phase: "pr",
	}, s.Clock.Now())
	s.Require().NoError(err)
	s.Require().NoError(s.runs.SetSession(s.Ctx, otherKey, "sess-2", s.base, s.Clock.Now()))
	s.Require().NoError(s.newOpen(ask).Open(s.Ctx, otherKey, "https://github.com/acme/acme-api/pull/8"))
	s.writeGH("hang-7", "")
	s.writeGH("view-8.json", `{"state":"MERGED","createdAt":"`+prCreated+`","reviews":[],"comments":[],"statusCheckRollup":[]}`)
	watch := s.newWatchWithTimeout(ask, 300*time.Millisecond)

	started := time.Now()
	err = watch.Tick(s.Ctx)

	s.Error(err)
	s.Less(time.Since(started), 10*time.Second)
	other, findErr := s.runs.FindByKey(s.Ctx, otherKey)
	s.Require().NoError(findErr)
	s.False(other.EndedAt.IsZero(), "the run after the hung one is watched")
	hung, findErr := s.runs.FindByKey(s.Ctx, statusRunKey)
	s.Require().NoError(findErr)
	s.True(hung.EndedAt.IsZero())
}

func (s *PRWatchTestSuite) TestInlineCommentsArePaginated() {
	ask := s.newAsk()
	s.openPR(ask)
	s.writeGH("comments-7.json", `[{"created_at":"`+beforeOpen+`"}][{"created_at":"`+laterThanOpen+`"}]`)

	s.Require().NoError(s.newWatch(ask).Tick(s.Ctx))

	s.Equal(1, strings.Count(s.agtermLog(), typeVerb), "a comment on the second page counts")
	s.Contains(s.ghLog(), "api\t--paginate\trepos/acme/acme-api/pulls/7/comments")
}

func (s *PRWatchTestSuite) TestManualVerbPostsTheChecklistWithAMention() {
	handler := socket.NewManualHandler(s.newOpen(s.newAsk()))

	resp := handler(s.Ctx, socket.Request{Verb: socket.VerbManual, Key: string(statusRunKey), Body: "- check the dark theme"})

	s.Equal(socket.CodeOK, resp.Code, resp.Error)
	text := s.lastPost()
	s.True(strings.HasPrefix(text, "<@"+statusOwnerID+"> "), text)
	s.Contains(text, "- check the dark theme")
}

func (s *PRWatchTestSuite) TestPRVerbRecordsThePR() {
	handler := socket.NewPRHandler(s.newOpen(s.newAsk()))

	resp := handler(s.Ctx, socket.Request{Verb: socket.VerbPR, Key: string(statusRunKey), URL: prURL})

	s.Equal(socket.CodeOK, resp.Code, resp.Error)
	repos, err := s.runs.Repos(s.Ctx, statusRunKey)
	s.Require().NoError(err)
	s.Require().Len(repos, 1)
	s.Equal(prURL, repos[0].PRURL)
}

func (s *PRWatchTestSuite) TestPRWatch_ClosedPRRemovesTheWorkingReaction() {
	ask := s.newAsk()
	s.openPR(ask)
	watch := s.newWatch(ask)
	s.view("CLOSED", `"reviews":[],"comments":[],"statusCheckRollup":[]`)

	s.Require().NoError(watch.Tick(s.Ctx))

	s.Equal([]SlackReaction{{Method: "remove", Name: slackpost.ReactionWorking, Channel: statusChannelID, TS: statusThreadTS}}, s.slack.Reactions())
}

func (s *PRWatchTestSuite) TestPROpen_PostsReviewAndMergeButtons() {
	s.openPR(s.newAsk())

	posts := s.slack.Posts()
	s.Require().NotEmpty(posts)
	blocks := posts[len(posts)-1].Blocks
	s.Contains(blocks, `"url":"`+prURL+`/files"`)
	s.Contains(blocks, `"action_id":"merge-pr"`)
	s.Contains(blocks, `"value":"`+string(statusRunKey)+` `+prURL+`"`)
}

// requireCardRedrawn checks that the only card update redrew the posted card with its text, no buttons and the
// state line.
func (s *PRWatchTestSuite) requireCardRedrawn(card SlackPost, state string) {
	updates := s.slack.Updates()
	s.Require().Len(updates, 1)
	s.Equal(card.TS, updates[0].TS)
	s.Equal(card.Text, updates[0].Text)
	s.NotContains(updates[0].Blocks, `"action_id":"merge-pr"`)
	s.NotContains(updates[0].Blocks, `"url":"`+prURL+`/files"`)
	s.Contains(updates[0].Blocks, `"type":"context"`)
	s.Contains(updates[0].Blocks, `"text":"`+state+`"`)
}

func (s *PRWatchTestSuite) TestPRWatch_MergeOnGitHubRedrawsTheCardAsMerged() {
	ask := s.newAsk()
	s.openPR(ask)
	card := s.slack.Posts()[len(s.slack.Posts())-1]
	s.view("MERGED", mergedView)

	s.Require().NoError(s.newWatch(ask).Tick(s.Ctx))

	s.requireCardRedrawn(card, "Merged.")
}

func (s *PRWatchTestSuite) TestPRWatch_ClosedPRRedrawsTheCardAsClosed() {
	ask := s.newAsk()
	s.openPR(ask)
	card := s.slack.Posts()[len(s.slack.Posts())-1]
	s.view("CLOSED", `"reviews":[],"comments":[],"statusCheckRollup":[]`)

	s.Require().NoError(s.newWatch(ask).Tick(s.Ctx))

	s.requireCardRedrawn(card, "Closed without a merge.")
}

func (s *PRWatchTestSuite) TestPRWatch_OpenPRKeepsTheCard() {
	ask := s.newAsk()
	s.openPR(ask)

	s.Require().NoError(s.newWatch(ask).Tick(s.Ctx))

	s.Empty(s.slack.Updates())
}

func (s *PRWatchTestSuite) git(dir string, args ...string) string {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	s.Require().NoError(err, string(out))
	return strings.TrimSpace(string(out))
}

// makeWorktree makes the acme-api checkout and the run's worktree of it on prBranch, as a run start does.
func (s *PRWatchTestSuite) makeWorktree() string {
	s.T().Setenv("GIT_AUTHOR_NAME", "t")
	s.T().Setenv("GIT_AUTHOR_EMAIL", "t@example.com")
	s.T().Setenv("GIT_COMMITTER_NAME", "t")
	s.T().Setenv("GIT_COMMITTER_EMAIL", "t@example.com")
	checkout := filepath.Join(s.repoRoot, prRepo)
	s.Require().NoError(os.MkdirAll(checkout, 0o755))
	s.git(checkout, "init", "-b", "main")
	s.git(checkout, "commit", "--allow-empty", "-m", "first")
	dir := filepath.Join(s.base, prRepo)
	s.git(checkout, "worktree", "add", "-b", prBranch, "--", dir)
	return dir
}

func (s *PRWatchTestSuite) TestPRWatch_MergePostsDoneAndTicksTheRequest() {
	ask := s.newAsk()
	s.openPR(ask)
	s.view("MERGED", mergedView)

	s.Require().NoError(s.newWatch(ask).Tick(s.Ctx))

	s.Equal("✅ ACM-9 done. Deploy it by hand: run make release.", s.lastPost())
	s.Equal(statusThreadTS, s.slack.Posts()[len(s.slack.Posts())-1].ThreadTS)
	s.Equal([]SlackReaction{
		{Method: "remove", Name: slackpost.ReactionWorking, Channel: statusChannelID, TS: statusThreadTS},
		{Method: "add", Name: slackpost.ReactionDone, Channel: statusChannelID, TS: statusThreadTS},
	}, s.slack.Reactions())
}

func (s *PRWatchTestSuite) TestPRWatch_MergeRemovesWorktreeAndBranches() {
	dir := s.makeWorktree()
	ask := s.newAsk()
	s.openPR(ask)
	s.view("MERGED", mergedView)

	s.Require().NoError(s.newWatch(ask).Tick(s.Ctx))

	s.NoDirExists(dir)
	s.Empty(s.git(filepath.Join(s.repoRoot, prRepo), "branch", "--list", prBranch))
	s.Contains(s.ghLog(), branchDelete)
	repos, err := s.runs.Repos(s.Ctx, statusRunKey)
	s.Require().NoError(err)
	s.Require().Len(repos, 1)
	s.Equal(domain.Merge{Branch: prBranch, SHA: "abc1234def5678", At: s.Clock.Now()}, repos[0].Merge)
}

func (s *PRWatchTestSuite) TestPRWatch_MergeWithWorktreeFolderGoneStillDeletesTheLocalBranch() {
	dir := s.makeWorktree()
	s.Require().NoError(os.RemoveAll(dir))
	ask := s.newAsk()
	s.openPR(ask)
	s.view("MERGED", mergedView)

	s.Require().NoError(s.newWatch(ask).Tick(s.Ctx))

	s.Empty(s.git(filepath.Join(s.repoRoot, prRepo), "branch", "--list", prBranch))
	s.NotContains(s.git(filepath.Join(s.repoRoot, prRepo), "worktree", "list", "--porcelain"), dir)
}

func (s *PRWatchTestSuite) TestPRWatch_MergeWithRemoteBranchGoneIsQuiet() {
	for name, stderr := range map[string]string{
		"422": "gh: Reference does not exist (HTTP 422)",
		"404": "gh: Not Found (HTTP 404)",
	} {
		s.Run(name, func() {
			s.writeGH("delete-fails", stderr)
			client := gh.New(s.fakeBin("fake-gh.sh"), time.Second)

			err := client.DeleteBranch(s.Ctx, gh.PRRef{Owner: "acme", Repo: "acme-api", Number: 7}, prBranch)

			s.NoError(err)
		})
	}
	ask := s.newAsk()
	s.openPR(ask)
	s.view("MERGED", mergedView)

	s.Require().NoError(s.newWatch(ask).Tick(s.Ctx))

	s.Equal("✅ ACM-9 done. Deploy it by hand: run make release.", s.lastPost())
}

func (s *PRWatchTestSuite) TestPRWatch_ClosedPRKeepsTheBranch() {
	dir := s.makeWorktree()
	ask := s.newAsk()
	s.openPR(ask)
	s.view("CLOSED", `"headRefName":"`+prBranch+`","reviews":[],"comments":[],"statusCheckRollup":[]`)

	s.Require().NoError(s.newWatch(ask).Tick(s.Ctx))

	s.DirExists(dir)
	s.Equal(prBranch, s.git(filepath.Join(s.repoRoot, prRepo), "branch", "--list", prBranch, "--format=%(refname:short)"))
	s.NotContains(s.ghLog(), "DELETE")
}

func (s *PRWatchTestSuite) TestPRWatch_MergeOfAnAutomaticRepoWaitsForTheDeploy() {
	dir := s.makeWorktree()
	s.registry = deployRegistry(config.RepoDeploy{Job: deployJob, Timeout: time.Hour})
	ask := s.newAsk()
	s.openPR(ask)
	s.view("MERGED", mergedView)
	before := len(s.slack.Posts())

	s.Require().NoError(s.newWatch(ask).Tick(s.Ctx))

	s.Len(s.slack.Posts(), before, "no done line before the merge is live")
	s.Empty(s.slack.Reactions(), "the working reaction stays")
	s.Empty(s.stateChanges(), "Linear waits for the deploy")
	run, err := s.runs.FindByKey(s.Ctx, statusRunKey)
	s.Require().NoError(err)
	s.False(run.EndedAt.IsZero(), "the run leaves the PR watch")
	s.NotEqual(domain.Phase("done"), run.Phase)
	s.Contains(s.agtermLog(), closeVerb)
	s.NoDirExists(dir)
	s.Contains(s.ghLog(), branchDelete)
}

func (s *PRWatchTestSuite) TestPRWatch_MergeOfRepoWithDeployJobIsPending() {
	s.registry = deployRegistry(config.RepoDeploy{Job: deployJob, Timeout: time.Hour})
	ask := s.newAsk()
	s.openPR(ask)
	s.view("MERGED", mergedView)

	s.Require().NoError(s.newWatch(ask).Tick(s.Ctx))

	pending, err := s.runs.PendingDeploys(s.Ctx)
	s.Require().NoError(err)
	s.Require().Len(pending, 1)
	s.Equal(statusRunKey, pending[0].Run.IssueKey)
	s.Equal(prRepo, pending[0].Repo.Repo)
	s.Equal(domain.Merge{Branch: prBranch, SHA: "abc1234def5678", DeployState: domain.DeployPending, At: s.Clock.Now()}, pending[0].Repo.Merge)
}

func (s *PRWatchTestSuite) TestPRWatch_ManualRepoIsNeverPending() {
	ask := s.newAsk()
	s.openPR(ask)
	s.view("MERGED", mergedView)

	s.Require().NoError(s.newWatch(ask).Tick(s.Ctx))

	pending, err := s.runs.PendingDeploys(s.Ctx)
	s.Require().NoError(err)
	s.Empty(pending)
}
