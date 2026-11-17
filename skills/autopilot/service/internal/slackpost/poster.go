package slackpost

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/slack-go/slack"
)

const (
	CancelAction = "cancel-run"
	MergeAction  = "merge-pr"

	ReactionWorking = "hourglass_flowing_sand"
	ReactionDone    = "white_check_mark"
	ReactionSeen    = "eyes"
	ReactionStuck   = "x"

	cancelButtonText = "Cancel"
	reviewAction     = "review-pr"
	reviewButtonText = "Review"
	mergeButtonText  = "Merge"
	channelPageSize  = 200
)

var (
	escaper    = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	urlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "|", "%7C", " ", "%20", "\t", "%09", "\n", "%0A", "\r", "%0D")
)

// Escape makes text safe for Slack mrkdwn: it cannot form a link, mention or channel ping.
func Escape(s string) string {
	return escaper.Replace(s)
}

// Card is the message that shows one run: the issue, and a Cancel button while the run is active.
type Card struct {
	IssueKey string
	Title    string
	URL      string
	// State, when set, replaces the Cancel button with this line.
	State string
}

type Poster struct {
	api *slack.Client
}

func NewPoster(api *slack.Client) *Poster {
	return &Poster{api: api}
}

// Thread posts one line of text; an empty threadTS posts to the channel itself.
func (p *Poster) Thread(ctx context.Context, channelID, threadTS, text string) error {
	opts := []slack.MsgOption{slack.MsgOptionText(Escape(text), false), slack.MsgOptionDisableLinkUnfurl(), slack.MsgOptionDisableMediaUnfurl()}
	if threadTS != "" {
		opts = append(opts, slack.MsgOptionTS(threadTS))
	}
	if _, _, err := p.api.PostMessageContext(ctx, channelID, opts...); err != nil {
		return fmt.Errorf("cannot post text to slack channel %s: %w", channelID, err)
	}
	return nil
}

// Mention posts one line of text in a thread that starts with an @mention of the user.
func (p *Poster) Mention(ctx context.Context, channelID, threadTS, userID, text string) error {
	line := fmt.Sprintf("<@%s> %s", userID, Escape(text))
	opts := []slack.MsgOption{slack.MsgOptionText(line, false), slack.MsgOptionDisableLinkUnfurl(), slack.MsgOptionDisableMediaUnfurl(), slack.MsgOptionTS(threadTS)}
	if _, _, err := p.api.PostMessageContext(ctx, channelID, opts...); err != nil {
		return fmt.Errorf("cannot post mention to slack channel %s: %w", channelID, err)
	}
	return nil
}

// PRCard is the post of an open pull request: the text, a Review link to the PR's files and a Merge button.
type PRCard struct {
	IssueKey string
	PRURL    string
	// UserID, when set, is mentioned at the start of the text.
	UserID string
	Text   string
	// State, when set, replaces the Review and Merge buttons with this small line.
	State string
}

// PRCard posts the PR card in a thread and returns its ts.
func (p *Poster) PRCard(ctx context.Context, channelID, threadTS string, c PRCard) (string, error) {
	opts := []slack.MsgOption{
		slack.MsgOptionText(prCardLine(c), false),
		slack.MsgOptionBlocks(prCardBlocks(c)...),
		slack.MsgOptionDisableLinkUnfurl(),
		slack.MsgOptionDisableMediaUnfurl(),
		slack.MsgOptionTS(threadTS),
	}
	_, ts, err := p.api.PostMessageContext(ctx, channelID, opts...)
	if err != nil {
		return "", fmt.Errorf("cannot post the PR card of %s to slack: %w", c.IssueKey, err)
	}
	return ts, nil
}

// UpdatePRCard redraws a posted PR card, for example with a State and no buttons.
func (p *Poster) UpdatePRCard(ctx context.Context, channelID, ts string, c PRCard) error {
	if _, _, _, err := p.api.UpdateMessageContext(ctx, channelID, ts,
		slack.MsgOptionText(prCardLine(c), false),
		slack.MsgOptionBlocks(prCardBlocks(c)...),
	); err != nil {
		return fmt.Errorf("cannot update the PR card of %s in slack: %w", c.IssueKey, err)
	}
	return nil
}

func prCardLine(c PRCard) string {
	line := Escape(c.Text)
	if c.UserID != "" {
		line = fmt.Sprintf("<@%s> %s", c.UserID, line)
	}
	return line
}

func prCardBlocks(c PRCard) []slack.Block {
	section := slack.NewSectionBlock(slack.NewTextBlockObject(slack.MarkdownType, prCardLine(c), false, false), nil, nil)
	if c.State != "" {
		return []slack.Block{section, slack.NewContextBlock("pr-state", slack.NewTextBlockObject(slack.PlainTextType, c.State, false, false))}
	}
	actions := slack.NewActionBlock("pr-actions",
		slack.NewButtonBlockElement(reviewAction, c.IssueKey,
			slack.NewTextBlockObject(slack.PlainTextType, reviewButtonText, false, false)).WithURL(fmt.Sprintf("%s/files", c.PRURL)),
		slack.NewButtonBlockElement(MergeAction, fmt.Sprintf("%s %s", c.IssueKey, c.PRURL),
			slack.NewTextBlockObject(slack.PlainTextType, mergeButtonText, false, false)).WithStyle(slack.StylePrimary))
	return []slack.Block{section, actions}
}

// React adds a reaction to a message; a reaction that is there already counts as added.
func (p *Poster) React(ctx context.Context, channelID, ts, name string) error {
	err := p.api.AddReactionContext(ctx, name, slack.NewRefToMessage(channelID, ts))
	if err != nil && ErrorCode(err) != "already_reacted" {
		return fmt.Errorf("cannot add reaction %s to %s in %s: %w", name, ts, channelID, err)
	}
	return nil
}

// Unreact removes a reaction of the bot from a message; a reaction that is not there counts as removed.
func (p *Poster) Unreact(ctx context.Context, channelID, ts, name string) error {
	err := p.api.RemoveReactionContext(ctx, name, slack.NewRefToMessage(channelID, ts))
	if err != nil && ErrorCode(err) != "no_reaction" {
		return fmt.Errorf("cannot remove reaction %s from %s in %s: %w", name, ts, channelID, err)
	}
	return nil
}

// Permalink returns the link of a message.
func (p *Poster) Permalink(ctx context.Context, channelID, ts string) (string, error) {
	link, err := p.api.GetPermalinkContext(ctx, &slack.PermalinkParameters{Channel: channelID, Ts: ts})
	if err != nil {
		return "", fmt.Errorf("cannot get slack permalink of %s in %s: %w", ts, channelID, err)
	}
	return link, nil
}

// Card posts the run card and returns its ts; an empty threadTS posts to the channel itself.
func (p *Poster) Card(ctx context.Context, channelID, threadTS string, c Card) (string, error) {
	opts := []slack.MsgOption{
		slack.MsgOptionText(Escape(cardText(c)), false),
		slack.MsgOptionBlocks(cardBlocks(c)...),
		slack.MsgOptionDisableLinkUnfurl(),
		slack.MsgOptionDisableMediaUnfurl(),
	}
	if threadTS != "" {
		opts = append(opts, slack.MsgOptionTS(threadTS))
	}
	_, ts, err := p.api.PostMessageContext(ctx, channelID, opts...)
	if err != nil {
		return "", fmt.Errorf("cannot post card of %s to slack: %w", c.IssueKey, err)
	}
	return ts, nil
}

// UpdateCard redraws a posted card, for example with its final State and no Cancel button.
func (p *Poster) UpdateCard(ctx context.Context, channelID, ts string, c Card) error {
	if _, _, _, err := p.api.UpdateMessageContext(ctx, channelID, ts,
		slack.MsgOptionText(Escape(cardText(c)), false),
		slack.MsgOptionBlocks(cardBlocks(c)...),
	); err != nil {
		return fmt.Errorf("cannot update card of %s in slack: %w", c.IssueKey, err)
	}
	return nil
}

// Channel is one Slack channel the bot can see.
type Channel struct {
	ID, Name string
	IsMember bool
}

// Channels lists the public channels and the private channels the bot can see, archived ones left out.
func (p *Poster) Channels(ctx context.Context) ([]Channel, error) {
	var out []Channel
	cursor := ""
	for {
		chans, next, err := p.api.GetConversationsContext(ctx, &slack.GetConversationsParameters{
			Types:           []string{"public_channel", "private_channel"},
			ExcludeArchived: true,
			Limit:           channelPageSize,
			Cursor:          cursor,
		})
		if err != nil {
			return nil, fmt.Errorf("cannot list slack channels: %w", err)
		}
		for _, c := range chans {
			out = append(out, Channel{ID: c.ID, Name: c.Name, IsMember: c.IsMember})
		}
		if next == "" {
			return out, nil
		}
		cursor = next
	}
}

func cardText(c Card) string {
	text := fmt.Sprintf("%s %s", c.IssueKey, c.Title)
	if c.State != "" {
		text = fmt.Sprintf("%s (%s)", text, c.State)
	}
	return text
}

func cardBlocks(c Card) []slack.Block {
	// the key is text, not a link, unless the url is http(s): Slack would run any other scheme
	head := Escape(c.IssueKey)
	if strings.HasPrefix(c.URL, "http://") || strings.HasPrefix(c.URL, "https://") {
		head = fmt.Sprintf("<%s|%s>", urlEscaper.Replace(c.URL), head)
	}
	text := fmt.Sprintf("*%s* %s", head, Escape(c.Title))
	if c.State != "" {
		text = fmt.Sprintf("%s\n_%s_", text, Escape(c.State))
	}
	section := slack.NewSectionBlock(slack.NewTextBlockObject(slack.MarkdownType, text, false, false), nil, nil)
	blocks := []slack.Block{section}
	if c.State == "" {
		blocks = append(blocks, slack.NewActionBlock("card-actions",
			slack.NewButtonBlockElement(CancelAction, c.IssueKey,
				slack.NewTextBlockObject(slack.PlainTextType, cancelButtonText, false, false))))
	}
	return blocks
}

// NotPosted reports whether slack answered a post with a clear rejection, so the message is surely not in the channel.
// A transport error or a timeout is not one: slack may have posted the message before the answer got lost.
func NotPosted(err error) bool {
	var apiErr slack.SlackErrorResponse
	var rateErr *slack.RateLimitedError
	return errors.As(err, &apiErr) || errors.As(err, &rateErr)
}

// ErrorCode returns the code of a call that slack rejected, such as not_in_channel; it is empty for any other error.
func ErrorCode(err error) string {
	var apiErr slack.SlackErrorResponse
	if errors.As(err, &apiErr) {
		return apiErr.Err
	}
	return ""
}
