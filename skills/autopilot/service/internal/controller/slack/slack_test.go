package controllerslack_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
	"github.com/stretchr/testify/require"

	controllerslack "github.com/sadensmol/autopilot/internal/controller/slack"
	"github.com/sadensmol/autopilot/internal/slackpost"
	"github.com/sadensmol/autopilot/tests"
)

const (
	owner   = "UOWNER"
	channel = "C1"
)

type recorder struct {
	mu      sync.Mutex
	tops    []controllerslack.Message
	replies []controllerslack.Message
	cancels []controllerslack.CancelClick
	merges  []controllerslack.MergeClick
}

func (r *recorder) NewRequest(_ context.Context, m controllerslack.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tops = append(r.tops, m)
	return nil
}

func (r *recorder) ThreadReply(_ context.Context, m controllerslack.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.replies = append(r.replies, m)
	return nil
}

func (r *recorder) Cancel(_ context.Context, c controllerslack.CancelClick) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cancels = append(r.cancels, c)
	return nil
}

func (r *recorder) Merge(_ context.Context, c controllerslack.MergeClick) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.merges = append(r.merges, c)
	return nil
}

func (r *recorder) mergeClicks() []controllerslack.MergeClick {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]controllerslack.MergeClick(nil), r.merges...)
}

func (r *recorder) counts() (int, int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.tops), len(r.replies), len(r.cancels)
}

func newHandler(rec *recorder) *controllerslack.Handler {
	return controllerslack.NewHandler(context.Background(), controllerslack.HandlerData{
		OwnerID: owner, ChannelIDs: []string{channel}, Intake: rec, Canceller: rec, Merger: rec,
	})
}

func msgEvent(m slackevents.MessageEvent) socketmode.Event {
	return socketmode.Event{
		Type:    socketmode.EventTypeEventsAPI,
		Request: &socketmode.Request{EnvelopeID: "env-m"},
		Data: slackevents.EventsAPIEvent{
			Type:       slackevents.CallbackEvent,
			InnerEvent: slackevents.EventsAPIInnerEvent{Type: "message", Data: &m},
		},
	}
}

func click(user, ch, value string) socketmode.Event {
	return clickOn(slackpost.CancelAction, user, ch, value)
}

func clickOn(actionID, user, ch, value string) socketmode.Event {
	cb := slack.InteractionCallback{Type: slack.InteractionTypeBlockActions}
	cb.User.ID = user
	cb.Channel.ID = ch
	cb.Message.Timestamp = "5.5"
	cb.ActionCallback.BlockActions = []*slack.BlockAction{{ActionID: actionID, Value: value}}
	return socketmode.Event{Type: socketmode.EventTypeInteractive, Request: &socketmode.Request{EnvelopeID: "env-c"}, Data: cb}
}

// run feeds events through Listen and waits until the handler finished all work.
func run(t *testing.T, h *controllerslack.Handler, evts ...socketmode.Event) *tests.FakeSocket {
	t.Helper()
	sock := tests.NewFakeSocket()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- controllerslack.Listen(ctx, sock, sock.Events, h) }()
	for _, e := range evts {
		sock.Events <- e
	}
	require.Eventually(t, func() bool { return len(sock.Events) == 0 }, time.Second, time.Millisecond)
	cancel()
	require.NoError(t, <-done)
	h.Wait()
	return sock
}

func TestListen_OwnerTopLevelMessageStartsIntake(t *testing.T) {
	rec := &recorder{}

	sock := run(t, newHandler(rec), msgEvent(slackevents.MessageEvent{User: owner, Channel: channel, Text: "add x", TimeStamp: "1.1"}))

	tops, replies, _ := rec.counts()
	require.Equal(t, 1, tops)
	require.Zero(t, replies)
	require.Equal(t, controllerslack.Message{ChannelID: channel, UserID: owner, Text: "add x", TS: "1.1"}, rec.tops[0])
	require.Equal(t, []string{"env-m"}, sock.Acks())
}

func TestListen_OwnerThreadReplyGoesToReplies(t *testing.T) {
	rec := &recorder{}

	run(t, newHandler(rec), msgEvent(slackevents.MessageEvent{User: owner, Channel: channel, Text: "yes", TimeStamp: "2.2", ThreadTimeStamp: "1.1"}))

	tops, replies, _ := rec.counts()
	require.Zero(t, tops)
	require.Equal(t, 1, replies)
	require.Equal(t, "1.1", rec.replies[0].ThreadTS)
}

func TestListen_IgnoredMessages(t *testing.T) {
	tests := []struct {
		name string
		m    slackevents.MessageEvent
	}{
		{name: "not the owner", m: slackevents.MessageEvent{User: "UOTHER", Channel: channel, TimeStamp: "1.1"}},
		{name: "other channel", m: slackevents.MessageEvent{User: owner, Channel: "C9", TimeStamp: "1.1"}},
		{name: "edit", m: slackevents.MessageEvent{User: owner, Channel: channel, TimeStamp: "1.1", SubType: "message_changed"}},
		{name: "bot message", m: slackevents.MessageEvent{User: owner, Channel: channel, TimeStamp: "1.1", BotID: "B1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recorder{}

			run(t, newHandler(rec), msgEvent(tt.m))

			tops, replies, cancels := rec.counts()
			require.Zero(t, tops+replies+cancels)
		})
	}
}

func TestListen_CancelClickGating(t *testing.T) {
	tests := []struct {
		name, user, channel string
		want                int
	}{
		{name: "owner in the product channel", user: owner, channel: channel, want: 1},
		{name: "someone else", user: "UOTHER", channel: channel},
		{name: "owner in another channel", user: owner, channel: "C9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recorder{}

			sock := run(t, newHandler(rec), click(tt.user, tt.channel, "ACME-1"))

			_, _, cancels := rec.counts()
			require.Equal(t, tt.want, cancels)
			require.Equal(t, []string{"env-c"}, sock.Acks())
			if tt.want == 1 {
				require.Equal(t, controllerslack.CancelClick{IssueKey: "ACME-1", ChannelID: channel, MessageTS: "5.5"}, rec.cancels[0])
			}
		})
	}
}

func TestListen_ConnectionDropReturnsError(t *testing.T) {
	sock := tests.NewFakeSocket()
	h := newHandler(&recorder{})
	done := make(chan error, 1)
	go func() { done <- controllerslack.Listen(t.Context(), sock, sock.Events, h) }()

	sock.Drop()

	require.ErrorContains(t, <-done, "slack connection ended")
}

func TestBlockActions_OwnerMergeClickSplitsTheValue(t *testing.T) {
	rec := &recorder{}

	run(t, newHandler(rec), clickOn(slackpost.MergeAction, owner, channel, "ACME-1 https://github.com/o/r/pull/7"))

	require.Equal(t, []controllerslack.MergeClick{{IssueKey: "ACME-1", PRURL: "https://github.com/o/r/pull/7", ChannelID: channel, MessageTS: "5.5"}}, rec.mergeClicks())
}

func TestBlockActions_MergeClickOfOtherUserIgnored(t *testing.T) {
	rec := &recorder{}

	sock := run(t, newHandler(rec), clickOn(slackpost.MergeAction, "UOTHER", channel, "ACME-1 https://github.com/o/r/pull/7"))

	require.Empty(t, rec.mergeClicks())
	require.Equal(t, []string{"env-c"}, sock.Acks())
}
