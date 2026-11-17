package tests

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	controllerslack "github.com/sadensmol/autopilot/internal/controller/slack"
	"github.com/sadensmol/autopilot/internal/domain"
	"github.com/sadensmol/autopilot/internal/gh"
	"github.com/sadensmol/autopilot/internal/slackpost"
	"github.com/sadensmol/autopilot/internal/usecase"
)

const (
	prCardTS   = "1700002100.000100"
	prCardText = "✅ PR ready: " + prURL
)

type PRMergeTestSuite struct {
	statusBase
	ghDir string
}

func TestPRMergeTestSuite(t *testing.T) {
	suite.Run(t, new(PRMergeTestSuite))
}

func (s *PRMergeTestSuite) SetupTest() {
	s.statusBase.SetupTest()
	s.ghDir = s.T().TempDir()
	s.T().Setenv("FAKE_GH_DIR", s.ghDir)
	s.writeGH("repo.json", `{"allow_merge_commit":true,"allow_squash_merge":true,"allow_rebase_merge":true}`)
	s.Require().NoError(s.runs.SetPR(s.Ctx, statusRunKey, prRepo, prURL, s.Clock.Now()))
	s.Require().NoError(s.runs.SetPRCard(s.Ctx, statusRunKey, prRepo, domain.PRCard{TS: prCardTS, Text: prCardText}, s.Clock.Now()))
}

func (s *PRMergeTestSuite) writeGH(name, body string) {
	s.Require().NoError(os.WriteFile(filepath.Join(s.ghDir, name), []byte(body), 0o644))
}

func (s *PRMergeTestSuite) ghLog() string {
	data, err := os.ReadFile(filepath.Join(s.ghDir, "argv.log"))
	if errors.Is(err, os.ErrNotExist) {
		return ""
	}
	s.Require().NoError(err)
	return string(data)
}

func (s *PRMergeTestSuite) merge(url string) error {
	bin, err := filepath.Abs(filepath.Join("testdata", "fake-gh.sh"))
	s.Require().NoError(err)
	m := usecase.NewPRMerge(usecase.PRMergeData{
		GH:      gh.New(bin, time.Second),
		OwnerID: statusOwnerID,
		Slack:   slackpost.NewPoster(s.slack.API()),
		Runs:    s.runs,
		Now:     s.Clock.Now,
	})
	return m.Merge(s.Ctx, controllerslack.MergeClick{IssueKey: string(statusRunKey), PRURL: url, ChannelID: statusChannelID, MessageTS: prCardTS})
}

// requireCard checks a redraw of the clicked card: its text with the owner's mention, and either the state line
// or the Review and Merge buttons.
func (s *PRMergeTestSuite) requireCard(update SlackPost, state string) {
	s.Equal(prCardTS, update.TS)
	s.Equal("<@"+statusOwnerID+"> "+prCardText, update.Text)
	if state == "" {
		s.Contains(update.Blocks, `"action_id":"merge-pr"`)
		s.Contains(update.Blocks, `"url":"`+prURL+`/files"`)
		s.NotContains(update.Blocks, `"type":"context"`)
		return
	}
	s.NotContains(update.Blocks, `"action_id":"merge-pr"`)
	s.NotContains(update.Blocks, `"url":"`+prURL+`/files"`)
	s.Contains(update.Blocks, `"text":"`+state+`"`)
}

func (s *PRMergeTestSuite) TestMerge_ClickShowsMergingInPlaceOfTheButtons() {
	s.Require().NoError(s.merge(prURL))

	updates := s.slack.Updates()
	s.Require().Len(updates, 1)
	s.requireCard(updates[0], "Merging…")
}

func (s *PRMergeTestSuite) TestMerge_UsesTheFirstAllowedMethod() {
	s.writeGH("repo.json", `{"allow_merge_commit":false,"allow_squash_merge":true,"allow_rebase_merge":true}`)

	s.Require().NoError(s.merge(prURL))

	s.Contains(s.ghLog(), "api\trepos/acme/acme-api\n")
	s.Contains(s.ghLog(), "pr\tmerge\t"+prURL+"\t--squash\t--delete-branch\n")
	s.Empty(s.slack.Posts())
}

func (s *PRMergeTestSuite) TestMerge_UnknownPRIsRefused() {
	s.Require().NoError(s.merge("https://github.com/acme/acme-api/pull/8"))

	s.NotContains(s.ghLog(), "pr\tmerge")
	s.Empty(s.slack.Posts())
	s.Empty(s.slack.Updates())
}

func (s *PRMergeTestSuite) TestMerge_GHFailurePostsTheError() {
	s.writeGH("merge-fails", "Pull request acme/acme-api#7 is not mergeable")

	s.Require().NoError(s.merge(prURL))

	posts := s.slack.Posts()
	s.Require().Len(posts, 1)
	s.Equal(statusThreadTS, posts[0].ThreadTS)
	s.Contains(posts[0].Text, "could not merge "+prURL+": ")
	s.Contains(posts[0].Text, "is not mergeable")
	updates := s.slack.Updates()
	s.Require().Len(updates, 2)
	s.requireCard(updates[0], "Merging…")
	s.requireCard(updates[1], "")
	run, err := s.runs.FindByKey(s.Ctx, statusRunKey)
	s.Require().NoError(err)
	s.True(run.EndedAt.IsZero())
}
