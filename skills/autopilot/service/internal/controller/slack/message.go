package controllerslack

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/slack-go/slack/slackevents"
)

const workTimeout = 10 * time.Minute

// Message is one owner message in a served channel.
type Message struct {
	ChannelID string
	UserID    string
	Text      string
	TS        string
	// ThreadTS is empty for a top-level message.
	ThreadTS string
}

// IIntake takes the owner's messages: a top-level one is a new request, a thread reply may answer an ask.
type IIntake interface {
	NewRequest(ctx context.Context, m Message) error
	ThreadReply(ctx context.Context, m Message) error
}

// CancelClick is a click on a card's Cancel button.
type CancelClick struct {
	IssueKey  string
	ChannelID string
	MessageTS string
}

type ICanceller interface {
	Cancel(ctx context.Context, c CancelClick) error
}

// MergeClick is a click on the Merge button of a PR card.
type MergeClick struct {
	IssueKey  string
	PRURL     string
	ChannelID string
	MessageTS string
}

type IMerger interface {
	Merge(ctx context.Context, c MergeClick) error
}

type HandlerData struct {
	OwnerID    string
	ChannelIDs []string
	Intake     IIntake
	Canceller  ICanceller
	Merger     IMerger
}

// Handler gates events to the owner and the served channels, then runs the work after the ack.
type Handler struct {
	ownerID    string
	channelIDs []string
	intake     IIntake
	canceller  ICanceller
	merger     IMerger
	background context.Context
	pending    sync.WaitGroup
}

// NewHandler takes background as the parent of the work that outlives an ack.
func NewHandler(background context.Context, d HandlerData) *Handler {
	return &Handler{
		ownerID:    d.OwnerID,
		channelIDs: slices.Clone(d.ChannelIDs),
		intake:     d.Intake,
		canceller:  d.Canceller,
		merger:     d.Merger,
		background: background,
	}
}

// Wait blocks until all work started by events has finished.
func (h *Handler) Wait() {
	h.pending.Wait()
}

func (h *Handler) served(channelID string) bool {
	return slices.Contains(h.channelIDs, channelID)
}

// Message takes a message event; only a plain message of the owner in a served channel counts.
func (h *Handler) Message(ctx context.Context, e *slackevents.MessageEvent) {
	// a subtype marks an edit, a delete, a join or a bot post; none of them is a request
	if e.SubType != "" || e.BotID != "" || e.User != h.ownerID || !h.served(e.Channel) {
		return
	}
	m := Message{ChannelID: e.Channel, UserID: e.User, Text: e.Text, TS: e.TimeStamp}
	if e.ThreadTimeStamp != "" && e.ThreadTimeStamp != e.TimeStamp {
		m.ThreadTS = e.ThreadTimeStamp
	}
	h.pending.Go(func() {
		workCtx, cancel := context.WithTimeout(h.background, workTimeout)
		defer cancel()
		var err error
		if m.ThreadTS == "" {
			err = h.intake.NewRequest(workCtx, m)
		} else {
			err = h.intake.ThreadReply(workCtx, m)
		}
		if err != nil {
			log.Ctx(workCtx).Error().Err(err).Msgf("cannot handle message %s in %s", m.TS, m.ChannelID)
		}
	})
}
