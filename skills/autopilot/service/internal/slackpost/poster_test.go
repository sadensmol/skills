package slackpost_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/slack-go/slack"
	"github.com/stretchr/testify/require"

	"github.com/sadensmol/autopilot/internal/slackpost"
	"github.com/sadensmol/autopilot/tests"
)

func TestEscape(t *testing.T) {
	require.Equal(t, "a &amp; b &lt;c&gt;", slackpost.Escape("a & b <c>"))
}

func TestThread_EscapesText(t *testing.T) {
	fake := tests.NewFakeSlack(t)
	p := slackpost.NewPoster(fake.API())

	require.NoError(t, p.Thread(t.Context(), "C1", "1.1", "a <!channel> & <b>"))

	posts := fake.Posts()
	require.Len(t, posts, 1)
	require.Equal(t, "C1", posts[0].Channel)
	require.Equal(t, "1.1", posts[0].ThreadTS)
	require.Equal(t, "a &lt;!channel&gt; &amp; &lt;b&gt;", posts[0].Text)
}

func TestCard_EscapesAndLinks(t *testing.T) {
	fake := tests.NewFakeSlack(t)
	p := slackpost.NewPoster(fake.API())

	ts, err := p.Card(t.Context(), "C1", "1.1", slackpost.Card{
		IssueKey: "ACME-1", Title: "Fix <b> & more", URL: "https://linear.example/acme/ACME-1?a=1&b=2|x y",
	})

	require.NoError(t, err)
	posts := fake.Posts()
	require.Len(t, posts, 1)
	require.Equal(t, ts, posts[0].TS)
	require.Equal(t, "1.1", posts[0].ThreadTS)
	text := sectionText(t, posts[0].Blocks)
	require.Equal(t, "*<https://linear.example/acme/ACME-1?a=1&amp;b=2%7Cx%20y|ACME-1>* Fix &lt;b&gt; &amp; more", text)
	require.Contains(t, posts[0].Blocks, `"action_id":"cancel-run"`)
	require.Contains(t, posts[0].Blocks, `"value":"ACME-1"`)
}

func TestCard_NoLinkForNonHTTP(t *testing.T) {
	fake := tests.NewFakeSlack(t)
	p := slackpost.NewPoster(fake.API())

	_, err := p.Card(t.Context(), "C1", "", slackpost.Card{IssueKey: "ACME-1", Title: "t", URL: "javascript:alert(1)"})

	require.NoError(t, err)
	require.NotContains(t, fake.Posts()[0].Blocks, "javascript")
	require.NotContains(t, fake.Posts()[0].Blocks, "<javascript")
}

func TestUpdateCard_DropsCancelButton(t *testing.T) {
	fake := tests.NewFakeSlack(t)
	p := slackpost.NewPoster(fake.API())

	err := p.UpdateCard(t.Context(), "C1", "9.9", slackpost.Card{
		IssueKey: "ACME-1", Title: "t", URL: "https://linear.example/ACME-1", State: "Canceled <by owner>",
	})

	require.NoError(t, err)
	ups := fake.Updates()
	require.Len(t, ups, 1)
	require.Equal(t, "9.9", ups[0].TS)
	require.NotContains(t, ups[0].Blocks, "cancel-run")
	require.Equal(t, "*<https://linear.example/ACME-1|ACME-1>* t\n_Canceled &lt;by owner&gt;_", sectionText(t, ups[0].Blocks))
}

func TestNotPosted(t *testing.T) {
	require.True(t, slackpost.NotPosted(slack.SlackErrorResponse{Err: "channel_not_found"}))
	require.True(t, slackpost.NotPosted(&slack.RateLimitedError{}))
	require.False(t, slackpost.NotPosted(errors.New("connection reset")))
	require.Equal(t, "not_in_channel", slackpost.ErrorCode(slack.SlackErrorResponse{Err: "not_in_channel"}))
	require.Empty(t, slackpost.ErrorCode(errors.New("x")))
}

func TestPost_RejectionMapsToNotPosted(t *testing.T) {
	fake := tests.NewFakeSlack(t)
	fake.RejectWith = "channel_not_found"
	p := slackpost.NewPoster(fake.API())

	err := p.Thread(t.Context(), "C1", "", "hi")

	require.Error(t, err)
	require.True(t, slackpost.NotPosted(err))
	require.Equal(t, "channel_not_found", slackpost.ErrorCode(err))
}

func TestChannels_PagesAndMembership(t *testing.T) {
	fake := tests.NewFakeSlack(t)
	fake.Channels = []tests.SlackChannel{{ID: "C1", Name: "general"}, {ID: "C2", Name: "acme-app", IsMember: true}, {ID: "C3", Name: "acme-web", IsMember: true}}
	p := slackpost.NewPoster(fake.API())

	got, err := p.Channels(t.Context())

	require.NoError(t, err)
	require.Equal(t, []slackpost.Channel{
		{ID: "C1", Name: "general"},
		{ID: "C2", Name: "acme-app", IsMember: true},
		{ID: "C3", Name: "acme-web", IsMember: true},
	}, got)
}

// sectionText decodes the posted blocks json and returns the text of the first section.
func sectionText(t *testing.T, raw string) string {
	t.Helper()
	var blocks []struct {
		Type string `json:"type"`
		Text struct {
			Text string `json:"text"`
		} `json:"text"`
	}
	require.NoError(t, json.Unmarshal([]byte(raw), &blocks))
	require.NotEmpty(t, blocks)
	require.Equal(t, "section", blocks[0].Type)
	return blocks[0].Text.Text
}

func TestPermalink(t *testing.T) {
	fake := tests.NewFakeSlack(t)
	p := slackpost.NewPoster(fake.API())

	link, err := p.Permalink(t.Context(), "C1", "1700000100.000100")

	require.NoError(t, err)
	require.Equal(t, "https://slack.example/archives/C1/p1700000100000100", link)
}

func TestReact_AlreadyReactedIsNoError(t *testing.T) {
	fake := tests.NewFakeSlack(t)
	p := slackpost.NewPoster(fake.API())
	require.NoError(t, p.React(t.Context(), "C1", "1.1", slackpost.ReactionWorking))

	err := p.React(t.Context(), "C1", "1.1", slackpost.ReactionWorking)

	require.NoError(t, err)
	require.Equal(t, []tests.SlackReaction{
		{Method: "add", Name: "hourglass_flowing_sand", Channel: "C1", TS: "1.1"},
		{Method: "add", Name: "hourglass_flowing_sand", Channel: "C1", TS: "1.1"},
	}, fake.Reactions())
}

func TestUnreact_NoReactionIsNoError(t *testing.T) {
	fake := tests.NewFakeSlack(t)
	p := slackpost.NewPoster(fake.API())

	err := p.Unreact(t.Context(), "C1", "1.1", slackpost.ReactionWorking)

	require.NoError(t, err)
	require.Equal(t, []tests.SlackReaction{{Method: "remove", Name: "hourglass_flowing_sand", Channel: "C1", TS: "1.1"}}, fake.Reactions())
}

func TestPRCard_HasReviewLinkAndMergeButton(t *testing.T) {
	fake := tests.NewFakeSlack(t)
	p := slackpost.NewPoster(fake.API())

	_, err := p.PRCard(t.Context(), "C1", "1.1", slackpost.PRCard{
		IssueKey: "ACM-7", PRURL: "https://github.com/o/r/pull/7", UserID: "U1", Text: "✅ PR open <x>",
	})

	require.NoError(t, err)
	posts := fake.Posts()
	require.Len(t, posts, 1)
	require.Equal(t, "1.1", posts[0].ThreadTS)
	require.Equal(t, "<@U1> ✅ PR open &lt;x&gt;", posts[0].Text)
	require.Equal(t, "<@U1> ✅ PR open &lt;x&gt;", sectionText(t, posts[0].Blocks))
	require.Contains(t, posts[0].Blocks, `"url":"https://github.com/o/r/pull/7/files"`)
	require.Contains(t, posts[0].Blocks, `"action_id":"merge-pr"`)
	require.Contains(t, posts[0].Blocks, `"value":"ACM-7 https://github.com/o/r/pull/7"`)
}
