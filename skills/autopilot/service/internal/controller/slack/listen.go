package controllerslack

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

// ISocket is the part of the Socket Mode client that Listen uses.
type ISocket interface {
	RunContext(ctx context.Context) error
	AckCtx(ctx context.Context, envelopeID string, payload any) error
}

// Listen runs the socket and hands every event to h until ctx ends; for the real client pass client.Events.
func Listen(ctx context.Context, socket ISocket, events <-chan socketmode.Event, h *Handler) error {
	runErr := make(chan error, 1)
	go func() {
		runErr <- socket.RunContext(ctx)
	}()
	for {
		select {
		case <-ctx.Done():
			<-runErr
			return nil
		case err := <-runErr:
			if ctx.Err() != nil {
				return nil
			}
			if err == nil {
				return errors.New("slack connection ended")
			}
			return fmt.Errorf("slack connection ended: %w", err)
		case evt := <-events:
			dispatch(ctx, socket, h, evt)
		}
	}
}

func dispatch(ctx context.Context, socket ISocket, h *Handler, evt socketmode.Event) {
	switch evt.Type {
	case socketmode.EventTypeConnected:
		log.Ctx(ctx).Info().Msg("connected to Slack")
	case socketmode.EventTypeConnectionError, socketmode.EventTypeInvalidAuth, socketmode.EventTypeIncomingError,
		socketmode.EventTypeErrorBadMessage, socketmode.EventTypeErrorWriteFailed:
		log.Ctx(ctx).Warn().Msgf("slack socket %s", evt.Type)
	case socketmode.EventTypeEventsAPI:
		if evt.Request == nil {
			return
		}
		ack(ctx, socket, evt.Request.EnvelopeID)
		apiEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
		if !ok {
			log.Ctx(ctx).Warn().Msgf("unexpected slack event data %T", evt.Data)
			return
		}
		if inner, ok := apiEvent.InnerEvent.Data.(*slackevents.MessageEvent); ok {
			h.Message(ctx, inner)
		}
	case socketmode.EventTypeInteractive:
		cb, ok := evt.Data.(slack.InteractionCallback)
		if !ok || evt.Request == nil {
			log.Ctx(ctx).Warn().Msgf("unexpected interaction data %T", evt.Data)
			return
		}
		ack(ctx, socket, evt.Request.EnvelopeID)
		if cb.Type == slack.InteractionTypeBlockActions {
			h.BlockActions(ctx, cb)
		}
	}
}

func ack(ctx context.Context, socket ISocket, envelopeID string) {
	if err := socket.AckCtx(ctx, envelopeID, nil); err != nil {
		log.Ctx(ctx).Error().Err(err).Msgf("cannot ack slack envelope %s", envelopeID)
	}
}
