package controllerslack

import (
	"context"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/slack-go/slack"

	"github.com/sadensmol/autopilot/internal/slackpost"
)

// BlockActions takes a button click after the ack; only the owner in a served channel counts.
func (h *Handler) BlockActions(ctx context.Context, cb slack.InteractionCallback) {
	if cb.User.ID != h.ownerID || !h.served(cb.Channel.ID) {
		log.Ctx(ctx).Info().Msgf("ignored a click of user %s in %s", cb.User.ID, cb.Channel.ID)
		return
	}
	for _, action := range cb.ActionCallback.BlockActions {
		if action == nil || action.Value == "" {
			continue
		}
		switch action.ActionID {
		case slackpost.CancelAction:
			click := CancelClick{IssueKey: action.Value, ChannelID: cb.Channel.ID, MessageTS: cb.Message.Timestamp}
			h.pending.Go(func() {
				workCtx, cancel := context.WithTimeout(h.background, workTimeout)
				defer cancel()
				if err := h.canceller.Cancel(workCtx, click); err != nil {
					log.Ctx(workCtx).Error().Err(err).Msgf("cannot cancel %s", click.IssueKey)
				}
			})
		case slackpost.MergeAction:
			key, prURL, ok := strings.Cut(action.Value, " ")
			if !ok {
				continue
			}
			click := MergeClick{IssueKey: key, PRURL: prURL, ChannelID: cb.Channel.ID, MessageTS: cb.Message.Timestamp}
			h.pending.Go(func() {
				workCtx, cancel := context.WithTimeout(h.background, workTimeout)
				defer cancel()
				if err := h.merger.Merge(workCtx, click); err != nil {
					log.Ctx(workCtx).Error().Err(err).Msgf("cannot merge %s of %s", click.PRURL, click.IssueKey)
				}
			})
		}
	}
}
