package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/slack-go/slack"
	"github.com/stretchr/testify/suite"

	controllerslack "github.com/sadensmol/autopilot/internal/controller/slack"
	"github.com/sadensmol/autopilot/internal/domain"
	"github.com/sadensmol/autopilot/internal/slackpost"
	"github.com/sadensmol/autopilot/internal/usecase"
)

const cancelCardTS = "1700002000.000500"

type CancelTestSuite struct {
	statusBase
}

func TestCancelTestSuite(t *testing.T) {
	suite.Run(t, new(CancelTestSuite))
}

func (s *CancelTestSuite) SetupTest() {
	s.statusBase.SetupTest()
	s.linear.On("Comment", func(map[string]any) LinearReply {
		return LinearReply{Data: map[string]any{"commentCreate": map[string]any{"success": true}}}
	})
	s.Require().NoError(s.runs.SetCardTS(s.Ctx, statusRunKey, cancelCardTS, s.Clock.Now()))
	s.Require().NoError(os.MkdirAll(filepath.Join(s.base, "run"), 0o755))
	s.Require().NoError(os.WriteFile(filepath.Join(s.base, "run", "engine.json"),
		[]byte(`{"key":"ACM-9","parent":"sess-1","waves":[{"status":"running","parts":[{"session_id":"part-1"}]}]}`), 0o644))
}

// clickAs sends a Cancel click through the real Slack handler and waits for its work.
func (s *CancelTestSuite) clickAs(user, channel string, canceller *usecase.Canceller) {
	h := controllerslack.NewHandler(s.Ctx, controllerslack.HandlerData{
		OwnerID: statusOwnerID, ChannelIDs: []string{statusChannelID, "C2"}, Canceller: canceller,
	})
	cb := slack.InteractionCallback{Type: slack.InteractionTypeBlockActions}
	cb.User.ID = user
	cb.Channel.ID = channel
	cb.Message.Timestamp = cancelCardTS
	cb.ActionCallback.BlockActions = []*slack.BlockAction{{ActionID: slackpost.CancelAction, Value: string(statusRunKey)}}
	h.BlockActions(s.Ctx, cb)
	h.Wait()
}

func (s *CancelTestSuite) commentBodies() []string {
	var out []string
	for _, r := range s.linear.Requests() {
		if r.Operation == "Comment" {
			out = append(out, r.Variables["body"].(string))
		}
	}
	return out
}

func (s *CancelTestSuite) TestOwnerCancelSetsCanceledCommentsClosesSessionsAndUpdatesTheCard() {
	ask := s.newAsk()
	_, err := ask.Open(s.Ctx, statusRunKey, "Which flag?", nil)
	s.Require().NoError(err)

	s.clickAs(statusOwnerID, statusChannelID, s.newCanceller(ask))

	s.Equal("st-Canceled", s.stateChanges()[len(s.stateChanges())-1])
	s.Require().Len(s.commentBodies(), 1)
	s.Contains(s.commentBodies()[0], "Canceled")
	log := s.agtermLogAt()
	s.Contains(log, "session\tclose\t--socket\t/tmp/ap-test.sock\t--target\tpart-1")
	s.Contains(log, "session\tclose\t--socket\t/tmp/ap-test.sock\t--target\tsess-1")
	updates := s.slack.Updates()
	s.Require().Len(updates, 1)
	s.Equal(cancelCardTS, updates[0].TS)
	s.Contains(updates[0].Text, "canceled")
	s.NotContains(updates[0].Blocks, slackpost.CancelAction, "a canceled card has no Cancel button")
	run, err := s.runs.FindByKey(s.Ctx, statusRunKey)
	s.Require().NoError(err)
	s.False(run.EndedAt.IsZero())
	s.Equal(domain.Phase("canceled"), run.Phase)
	_, err = s.asks.FindOpen(s.Ctx, statusRunKey)
	s.ErrorIs(err, domain.ErrNotFound, "cancel closes the open asks")
	s.Equal("🚫 ACM-9 canceled.", s.texts()[len(s.texts())-1])
}

func (s *CancelTestSuite) TestWorktreeStaysAfterCancel() {
	s.clickAs(statusOwnerID, statusChannelID, s.newCanceller(s.newAsk()))

	s.FileExists(filepath.Join(s.base, "run", "engine.json"))
}

func (s *CancelTestSuite) TestClickByAnotherUserDoesNothing() {
	s.clickAs("U-OTHER", statusChannelID, s.newCanceller(s.newAsk()))

	s.assertUntouched()
}

func (s *CancelTestSuite) TestClickInAnotherServedChannelDoesNothing() {
	s.clickAs(statusOwnerID, "C2", s.newCanceller(s.newAsk()))

	s.assertUntouched()
}

func (s *CancelTestSuite) TestSecondCancelChangesNothing() {
	c := s.newCanceller(s.newAsk())
	s.clickAs(statusOwnerID, statusChannelID, c)
	posts, changes := len(s.slack.Posts()), len(s.stateChanges())

	s.clickAs(statusOwnerID, statusChannelID, c)

	s.Len(s.slack.Posts(), posts)
	s.Len(s.stateChanges(), changes)
	s.Len(s.commentBodies(), 1)
}

func (s *CancelTestSuite) TestOwnerVerbCancelsWithoutAChannel() {
	s.Require().NoError(s.newCanceller(s.newAsk()).CancelRun(s.Ctx, statusRunKey))

	run, err := s.runs.FindByKey(s.Ctx, statusRunKey)
	s.Require().NoError(err)
	s.False(run.EndedAt.IsZero())
}

func (s *CancelTestSuite) assertUntouched() {
	s.Empty(s.stateChanges())
	s.Empty(s.commentBodies())
	s.Empty(s.slack.Posts())
	s.Empty(s.slack.Updates())
	s.NotContains(s.agtermLogAt(), "session\tclose")
	run, err := s.runs.FindByKey(s.Ctx, statusRunKey)
	s.Require().NoError(err)
	s.True(run.EndedAt.IsZero())
	s.Equal(domain.Phase("implementing"), run.Phase)
}

func (s *CancelTestSuite) agtermLogAt() string {
	data, err := os.ReadFile(filepath.Join(os.Getenv("FAKE_AGTERM_DIR"), "argv.log"))
	if os.IsNotExist(err) {
		return ""
	}
	s.Require().NoError(err)
	return strings.TrimSpace(string(data))
}

func (s *CancelTestSuite) TestCancel_ProductNotInTheRegistryIsAClearLinearStatusError() {
	const key = domain.IssueKey("ACM-10")
	_, err := s.runs.Create(s.Ctx, domain.Run{
		IssueKey: key, Product: "retired", ChannelID: statusChannelID, RequestTS: "1700000999.000001", ThreadTS: "1700000999.000001", Phase: "implementing",
	}, s.Clock.Now())
	s.Require().NoError(err)

	err = s.newCanceller(s.newAsk()).CancelRun(s.Ctx, key)

	s.Require().Error(err)
	s.Contains(err.Error(), "product retired of run ACM-10 is not in the workspace registry")
	s.Empty(s.stateChanges())
}

func (s *CancelTestSuite) TestCancel_RemovesTheWorkingReaction() {
	s.clickAs(statusOwnerID, statusChannelID, s.newCanceller(s.newAsk()))

	s.Equal([]SlackReaction{{Method: "remove", Name: slackpost.ReactionWorking, Channel: statusChannelID, TS: statusThreadTS}}, s.slack.Reactions())
}
