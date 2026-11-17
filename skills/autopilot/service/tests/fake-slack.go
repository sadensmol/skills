package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/socketmode"
)

// SlackChannel is one channel the fake lists.
type SlackChannel struct {
	ID, Name string
	IsMember bool
}

// SlackPost is one chat.postMessage or chat.update call the fake received.
type SlackPost struct {
	Channel, ThreadTS, TS, Text, Blocks string
}

// SlackReaction is one reactions.add or reactions.remove call the fake received; Method is add or remove.
type SlackReaction struct {
	Method, Name, Channel, TS string
}

// FakeSlack is an httptest Slack web API; it never opens a socket.
type FakeSlack struct {
	Server *httptest.Server

	// Channels is what conversations.list returns, two per page.
	Channels []SlackChannel
	// RejectWith makes every chat call answer ok:false with this error code.
	RejectWith string
	// RejectCards makes chat.postMessage answer ok:false when the message carries blocks, as a card does.
	RejectCards bool

	mu        sync.Mutex
	posts     []SlackPost
	updates   []SlackPost
	reactions []SlackReaction
	// reacted holds the reactions on the messages, as channel|ts|name, so a repeat answers like slack does.
	reacted map[string]bool
	counter int
}

func NewFakeSlack(t *testing.T) *FakeSlack {
	f := &FakeSlack{reacted: map[string]bool{}}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.Server.Close)
	return f
}

// API returns a Slack client that talks to the fake.
func (f *FakeSlack) API() *slack.Client {
	return slack.New("fake-token", slack.OptionAPIURL(f.Server.URL+"/api/"))
}

func (f *FakeSlack) Posts() []SlackPost {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]SlackPost(nil), f.posts...)
}

func (f *FakeSlack) Updates() []SlackPost {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]SlackPost(nil), f.updates...)
}

func (f *FakeSlack) Reactions() []SlackReaction {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]SlackReaction(nil), f.reactions...)
}

func (f *FakeSlack) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_ = r.ParseForm()
	w.Header().Set("Content-Type", "application/json")
	method := strings.TrimPrefix(r.URL.Path, "/api/")
	if f.RejectWith != "" && strings.HasPrefix(method, "chat.") {
		fmt.Fprintf(w, `{"ok":false,"error":%q}`, f.RejectWith)
		return
	}
	if f.RejectCards && method == "chat.postMessage" && r.PostForm.Get("blocks") != "" {
		fmt.Fprint(w, `{"ok":false,"error":"internal_error"}`)
		return
	}
	switch method {
	case "chat.postMessage":
		f.counter++
		ts := fmt.Sprintf("1700000000.%06d", f.counter)
		f.posts = append(f.posts, postFrom(r.PostForm, ts))
		fmt.Fprintf(w, `{"ok":true,"channel":%q,"ts":%q}`, r.PostForm.Get("channel"), ts)
	case "chat.update":
		f.updates = append(f.updates, postFrom(r.PostForm, r.PostForm.Get("ts")))
		fmt.Fprintf(w, `{"ok":true,"channel":%q,"ts":%q}`, r.PostForm.Get("channel"), r.PostForm.Get("ts"))
	case "chat.getPermalink":
		ts := r.Form.Get("message_ts")
		fmt.Fprintf(w, `{"ok":true,"channel":%q,"permalink":%q}`, r.Form.Get("channel"),
			"https://slack.example/archives/"+r.Form.Get("channel")+"/p"+strings.ReplaceAll(ts, ".", ""))
	case "conversations.list":
		f.writeChannels(w, r.PostForm.Get("cursor"))
	case "reactions.add", "reactions.remove":
		f.react(w, strings.TrimPrefix(method, "reactions."), r.PostForm)
	default:
		fmt.Fprintf(w, `{"ok":false,"error":"unknown_method"}`)
	}
}

// react records the call and answers already_reacted or no_reaction where slack does.
func (f *FakeSlack) react(w http.ResponseWriter, method string, form url.Values) {
	rec := SlackReaction{Method: method, Name: form.Get("name"), Channel: form.Get("channel"), TS: form.Get("timestamp")}
	f.reactions = append(f.reactions, rec)
	key := fmt.Sprintf("%s|%s|%s", rec.Channel, rec.TS, rec.Name)
	switch {
	case method == "add" && f.reacted[key]:
		fmt.Fprint(w, `{"ok":false,"error":"already_reacted"}`)
	case method == "remove" && !f.reacted[key]:
		fmt.Fprint(w, `{"ok":false,"error":"no_reaction"}`)
	default:
		f.reacted[key] = method == "add"
		fmt.Fprint(w, `{"ok":true}`)
	}
}

func (f *FakeSlack) writeChannels(w http.ResponseWriter, cursor string) {
	const page = 2
	start := 0
	if cursor != "" {
		_, _ = fmt.Sscanf(cursor, "c%d", &start)
	}
	end := min(start+page, len(f.Channels))
	type ch struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		IsMember bool   `json:"is_member"`
	}
	out := struct {
		OK       bool `json:"ok"`
		Channels []ch `json:"channels"`
		Meta     struct {
			Next string `json:"next_cursor"`
		} `json:"response_metadata"`
	}{OK: true, Channels: []ch{}}
	for _, c := range f.Channels[start:end] {
		out.Channels = append(out.Channels, ch{ID: c.ID, Name: c.Name, IsMember: c.IsMember})
	}
	if end < len(f.Channels) {
		out.Meta.Next = fmt.Sprintf("c%d", end)
	}
	_ = json.NewEncoder(w).Encode(out)
}

func postFrom(form url.Values, ts string) SlackPost {
	return SlackPost{
		Channel:  form.Get("channel"),
		ThreadTS: form.Get("thread_ts"),
		TS:       ts,
		Text:     form.Get("text"),
		Blocks:   form.Get("blocks"),
	}
}

// FakeSocket stands in for the Socket Mode client: it feeds events and records acks.
type FakeSocket struct {
	Events chan socketmode.Event

	mu   sync.Mutex
	acks []string
	done chan struct{}
}

func NewFakeSocket() *FakeSocket {
	return &FakeSocket{Events: make(chan socketmode.Event, 16), done: make(chan struct{})}
}

// RunContext blocks like the real client until the context ends or Drop is called.
func (f *FakeSocket) RunContext(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-f.done:
		return fmt.Errorf("fake connection dropped")
	}
}

// Drop ends RunContext with an error.
func (f *FakeSocket) Drop() {
	close(f.done)
}

func (f *FakeSocket) AckCtx(_ context.Context, envelopeID string, _ any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.acks = append(f.acks, envelopeID)
	return nil
}

func (f *FakeSocket) Acks() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.acks...)
}
