package usecase

import (
	"context"

	"github.com/rs/zerolog/log"

	"github.com/sadensmol/autopilot/internal/domain"
	"github.com/sadensmol/autopilot/internal/slackpost"
)

const (
	prCardMerging = "Merging…"
	prCardMerged  = "Merged."
	prCardClosed  = "Closed without a merge."
)

// IPRCardSlack redraws a posted PR card.
type IPRCardSlack interface {
	UpdatePRCard(ctx context.Context, channelID, ts string, c slackpost.PRCard) error
}

// prCards redraws the posted PR card of a repo. The card is only a view, so a failure is logged, never returned.
type prCards struct {
	slack   IPRCardSlack
	ownerID string
}

// redraw draws the card at repo.PRCard.TS again with its stored text: the state line in place of the Review and
// Merge buttons, or the buttons back when state is empty. A card posted before its text was stored is left as it is.
func (c prCards) redraw(ctx context.Context, run domain.Run, repo domain.RunRepo, state string) {
	if repo.PRCard.TS == "" || repo.PRCard.Text == "" {
		return
	}
	card := slackpost.PRCard{IssueKey: string(run.IssueKey), PRURL: repo.PRURL, UserID: c.ownerID, Text: repo.PRCard.Text, State: state}
	if err := c.slack.UpdatePRCard(ctx, run.ChannelID, repo.PRCard.TS, card); err != nil {
		log.Warn().Ctx(ctx).Err(err).Msgf("cannot redraw the PR card of %s in run %s", repo.PRURL, run.IssueKey)
	}
}
