package usecase

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/rs/zerolog/log"

	"github.com/sadensmol/autopilot/internal/config"
	controllerslack "github.com/sadensmol/autopilot/internal/controller/slack"
	"github.com/sadensmol/autopilot/internal/domain"
	"github.com/sadensmol/autopilot/internal/gate/profile"
	"github.com/sadensmol/autopilot/internal/linear"
	"github.com/sadensmol/autopilot/internal/slackpost"
)

const (
	maxTitleRunes   = 80
	fallbackTitle   = "Request from Slack"
	issueFailedText = "could not create the issue"
	startPhaseKey   = "todo"
)

var (
	mentionPattern = regexp.MustCompile(`<@([A-Z0-9]+)(?:\|[^>]*)?>`)
	groupPattern   = regexp.MustCompile(`<!(?:subteam\^|here\b|channel\b|everyone\b)`)
	specialPattern = regexp.MustCompile(`<!([a-z]+)(?:\|[^>]*)?>`)
	linkPattern    = regexp.MustCompile(`<([^<>|]+)(?:\|([^<>]*))?>`)
	spacePattern   = regexp.MustCompile(`\s+`)
	unescaper      = strings.NewReplacer("&lt;", "<", "&gt;", ">", "&amp;", "&")
	bracketEscaper = strings.NewReplacer("<", `\<`, ">", `\>`)
)

// ILinear is the part of the Linear client that intake uses.
type ILinear interface {
	Viewer(ctx context.Context) (linear.Viewer, error)
	TeamByKey(ctx context.Context, key string) (linear.Team, error)
	WorkflowStates(ctx context.Context, teamID string) ([]linear.State, error)
	LabelByName(ctx context.Context, team linear.Team, name string) (linear.Label, error)
	ProjectByName(ctx context.Context, team linear.Team, name string) (linear.Project, error)
	CreateIssue(ctx context.Context, d linear.CreateIssueData) (linear.Issue, error)
	Comment(ctx context.Context, issueID, body string) error
}

// ISlack is the part of the Slack poster that intake uses.
type ISlack interface {
	IReactSlack
	Permalink(ctx context.Context, channelID, ts string) (string, error)
	Thread(ctx context.Context, channelID, threadTS, text string) error
	Card(ctx context.Context, channelID, threadTS string, c slackpost.Card) (string, error)
}

// IRunStore is the part of the run store that intake uses.
type IRunStore interface {
	Create(ctx context.Context, run domain.Run, now time.Time) (domain.Run, error)
	FindByRequest(ctx context.Context, channelID, requestTS string) (domain.Run, error)
	SetCardTS(ctx context.Context, key domain.IssueKey, cardTS string, now time.Time) error
	End(ctx context.Context, key domain.IssueKey, now time.Time) error
}

// IRequestStore is the durable record of the messages intake took.
type IRequestStore interface {
	Claim(ctx context.Context, channelID, requestTS string, now time.Time) (domain.IntakeRequest, error)
	SetIssue(ctx context.Context, channelID, requestTS string, issue domain.IntakeIssue, now time.Time) error
	SetState(ctx context.Context, channelID, requestTS string, state domain.IntakeState, now time.Time) error
}

// IRunStarter starts a stored run: worktrees and the run session.
type IRunStarter interface {
	Start(ctx context.Context, run domain.Run, product config.Project) error
}

// UsableFunc is the cheap profile check of one repo folder.
type UsableFunc func(dir string, overrides map[string]string) (profile.Profile, error)

// IThreadReplies takes the owner's replies in a run's thread.
type IThreadReplies interface {
	ThreadReply(ctx context.Context, m controllerslack.Message) error
}

type IntakeData struct {
	// BotUserID is the Slack user id of the app; a mention of it does not stop a request.
	BotUserID       string
	Registry        config.Registry
	ChannelProducts map[string]config.Project
	RepoPath        func(repo string) string
	Linear          ILinear
	Slack           ISlack
	Runs            IRunStore
	Requests        IRequestStore
	Starter         IRunStarter
	Replies         IThreadReplies
	Usable          UsableFunc
	Now             func() time.Time
}

// Intake turns an owner's top-level message into an issue, a card and a run.
type Intake struct {
	d IntakeData

	mu      sync.Mutex
	claimed map[string]bool
}

func NewIntake(d IntakeData) *Intake {
	return &Intake{d: d, claimed: map[string]bool{}}
}

// ThreadReply hands an owner's thread reply to the asks; only a reply to an open ask means anything.
func (i *Intake) ThreadReply(ctx context.Context, m controllerslack.Message) error {
	return i.d.Replies.ThreadReply(ctx, m)
}

// NewRequest handles one top-level message of the owner. A message it does not take as a request is dropped without error.
// The message is claimed in the store before the issue exists, and every later step records its result on that row,
// so a redelivery, even after a restart, resumes the work or stops; it never creates a second issue.
// Before the issue exists it checks the Linear project and the repo folders of the product; a missing part gets
// one thread reply that names it, and no issue.
func (i *Intake) NewRequest(ctx context.Context, m controllerslack.Message) error {
	product, ok := i.d.ChannelProducts[m.ChannelID]
	if !ok || m.ThreadTS != "" || mentionsSomeoneElse(m.Text, i.d.BotUserID) {
		return nil
	}
	key := m.ChannelID + "|" + m.TS
	if !i.begin(key) {
		return nil
	}
	defer i.end(key)
	req, err := i.d.Requests.Claim(ctx, m.ChannelID, m.TS, i.d.Now())
	if err != nil {
		return err
	}
	if req.State.Terminal() {
		return nil
	}
	if req.Issue.Key == "" {
		report := InspectProduct(ctx, i.d.Linear, i.d.RepoPath, product)
		if err := report.Err(); err != nil {
			return i.productMissing(ctx, m, err)
		}
		issue, err := i.createIssue(ctx, m, report)
		if err != nil {
			i.tell(ctx, m, issueFailedText)
			return err
		}
		req.Issue = domain.IntakeIssue{ID: issue.ID, Key: issue.Key, Title: issue.Title, URL: issue.URL}
		if err := i.d.Requests.SetIssue(ctx, m.ChannelID, m.TS, req.Issue, i.d.Now()); err != nil {
			return i.failAfterIssue(ctx, m, req.Issue, "could not record the request", err)
		}
	}
	return i.openRun(ctx, m, product, req.Issue)
}

// begin marks a message as being worked on in this process; it is false when another delivery of it is.
func (i *Intake) begin(key string) bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.claimed[key] {
		return false
	}
	i.claimed[key] = true
	return true
}

func (i *Intake) end(key string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	delete(i.claimed, key)
}

// productMissing ends a request whose product lacks a part, so a redelivery stays quiet. That is a handled
// refusal, told in the thread, so it returns nil like refuse. A failed check leaves the request open for a
// redelivery to try again.
func (i *Intake) productMissing(ctx context.Context, m controllerslack.Message, cause error) error {
	var missing *ProductError
	if !errors.As(cause, &missing) {
		i.tell(ctx, m, issueFailedText)
		return fmt.Errorf("cannot check the product of request %s: %w", m.TS, cause)
	}
	if err := i.setState(ctx, m, domain.IntakeRefused); err != nil {
		return err
	}
	i.tell(ctx, m, "no issue was created: "+missing.Reason)
	return nil
}

func (i *Intake) createIssue(ctx context.Context, m controllerslack.Message, product ProductReport) (linear.Issue, error) {
	permalink, err := i.d.Slack.Permalink(ctx, m.ChannelID, m.TS)
	if err != nil {
		return linear.Issue{}, err
	}
	viewer, err := i.d.Linear.Viewer(ctx)
	if err != nil {
		return linear.Issue{}, fmt.Errorf("cannot read the linear api key user: %w", err)
	}
	stateID, err := i.todoStateID(ctx, product.Team)
	if err != nil {
		return linear.Issue{}, err
	}
	label, err := i.d.Linear.LabelByName(ctx, product.Team, i.d.Registry.Linear.DefaultLabel)
	if err != nil {
		return linear.Issue{}, fmt.Errorf("cannot find the default label: %w", err)
	}
	issue, err := i.d.Linear.CreateIssue(ctx, linear.CreateIssueData{
		TeamID:      product.Team.ID,
		Title:       titleOf(m.Text),
		Description: descriptionOf(m.Text, permalink),
		AssigneeID:  viewer.ID,
		LabelIDs:    []string{label.ID},
		StateID:     stateID,
		ProjectID:   product.ProjectID,
	})
	if err != nil {
		return linear.Issue{}, fmt.Errorf("cannot create the issue: %w", err)
	}
	return issue, nil
}

func (i *Intake) todoStateID(ctx context.Context, team linear.Team) (string, error) {
	name := i.d.Registry.Linear.Statuses[startPhaseKey]
	states, err := i.d.Linear.WorkflowStates(ctx, team.ID)
	if err != nil {
		return "", fmt.Errorf("cannot read statuses of team %s: %w", team.Key, err)
	}
	for _, s := range states {
		if s.Name == name {
			return s.ID, nil
		}
	}
	return "", fmt.Errorf("team %s has no status %q: %w", team.Key, name, domain.ErrNotFound)
}

// openRun checks the repos, stores the run, posts its card and starts it. The card carries Cancel, so it is
// posted only after the run exists. A failure from here on tells the thread, comments on the issue and stops.
func (i *Intake) openRun(ctx context.Context, m controllerslack.Message, product config.Project, issue domain.IntakeIssue) error {
	card := slackpost.Card{IssueKey: issue.Key, Title: issue.Title, URL: issue.URL}
	run, err := i.d.Runs.FindByRequest(ctx, m.ChannelID, m.TS)
	switch {
	case err == nil && run.CardTS != "":
		return i.setState(ctx, m, domain.IntakeDone)
	case err == nil:
		// an earlier attempt stored the run and stopped before the card
	case !errors.Is(err, domain.ErrNotFound):
		return i.failAfterIssue(ctx, m, issue, "could not look up the run", err)
	default:
		if reason := i.profileProblem(product); reason != "" {
			return i.refuse(ctx, m, issue, card, reason)
		}
		run, err = i.d.Runs.Create(ctx, domain.Run{
			IssueKey:  domain.IssueKey(issue.Key),
			Product:   product.SlackChannel,
			ChannelID: m.ChannelID,
			RequestTS: m.TS,
			ThreadTS:  m.TS,
			Phase:     domain.Phase(startPhaseKey),
		}, i.d.Now())
		if err != nil {
			return i.failAfterIssue(ctx, m, issue, "could not store the run", err)
		}
	}
	cardTS, err := i.d.Slack.Card(ctx, m.ChannelID, m.TS, card)
	if err == nil {
		err = i.d.Runs.SetCardTS(ctx, run.IssueKey, cardTS, i.d.Now())
	}
	if err != nil {
		// no card means no Cancel button: do not leave an active run nobody can see
		if endErr := i.d.Runs.End(ctx, run.IssueKey, i.d.Now()); endErr != nil {
			log.Ctx(ctx).Error().Err(endErr).Msgf("cannot end run %s after a failed card", run.IssueKey)
		}
		return i.failAfterIssue(ctx, m, issue, "could not post the card", err)
	}
	run.CardTS = cardTS
	if err := i.d.Slack.React(ctx, m.ChannelID, m.TS, slackpost.ReactionWorking); err != nil {
		log.Ctx(ctx).Warn().Err(err).Msgf("cannot add the working reaction to request %s of run %s", m.TS, run.IssueKey)
	}
	if err := i.setState(ctx, m, domain.IntakeDone); err != nil {
		return err
	}
	if err := i.d.Starter.Start(ctx, run, product); err != nil {
		return fmt.Errorf("cannot start run %s: %w", issue.Key, err)
	}
	return nil
}

// refuse posts a card with the reason and no Cancel button and comments on the issue, which stays in Todo.
func (i *Intake) refuse(ctx context.Context, m controllerslack.Message, issue domain.IntakeIssue, card slackpost.Card, reason string) error {
	if err := i.setState(ctx, m, domain.IntakeRefused); err != nil {
		return err
	}
	card.State = "no run started: " + reason
	if _, err := i.d.Slack.Card(ctx, m.ChannelID, m.TS, card); err != nil {
		return i.failAfterIssue(ctx, m, issue, "could not post the card", err)
	}
	if err := i.d.Linear.Comment(ctx, issue.ID, "autopilot did not start a run: "+reason); err != nil {
		return fmt.Errorf("cannot comment on %s: %w", issue.Key, err)
	}
	return nil
}

func (i *Intake) setState(ctx context.Context, m controllerslack.Message, state domain.IntakeState) error {
	if err := i.d.Requests.SetState(ctx, m.ChannelID, m.TS, state, i.d.Now()); err != nil {
		return fmt.Errorf("cannot mark request %s as %s: %w", m.TS, state, err)
	}
	return nil
}

// failAfterIssue ends the work on a message whose issue exists: one plain thread line (no Cancel card), a comment
// on the issue, which stays in Todo, and a terminal row so a redelivery does nothing. It returns cause, wrapped.
func (i *Intake) failAfterIssue(ctx context.Context, m controllerslack.Message, issue domain.IntakeIssue, what string, cause error) error {
	if err := i.d.Requests.SetState(ctx, m.ChannelID, m.TS, domain.IntakeFailed, i.d.Now()); err != nil {
		log.Ctx(ctx).Error().Err(err).Msgf("cannot mark request %s in %s as failed", m.TS, m.ChannelID)
	}
	i.tell(ctx, m, fmt.Sprintf("%s was created, but %s: no run started", issue.Key, what))
	if err := i.d.Linear.Comment(ctx, issue.ID, "autopilot did not start a run: "+what); err != nil {
		log.Ctx(ctx).Error().Err(err).Msgf("cannot comment on %s", issue.Key)
	}
	return fmt.Errorf("%s for %s: %w", what, issue.Key, cause)
}

// profileProblem names the first repo that has no usable profile and why; it is empty when all are usable.
func (i *Intake) profileProblem(product config.Project) string {
	for _, repo := range product.Repos {
		dir := i.d.RepoPath(repo.Name)
		if _, err := i.d.Usable(dir, repo.ProfileOverrides); err != nil {
			return fmt.Sprintf("repo %s: %s", repo.Name, strings.ReplaceAll(err.Error(), dir, repo.Name))
		}
	}
	return ""
}

// tell posts a short line in the request's thread. A failed post is only logged: the caller returns the real error.
func (i *Intake) tell(ctx context.Context, m controllerslack.Message, text string) {
	if err := i.d.Slack.Thread(ctx, m.ChannelID, m.TS, text); err != nil {
		log.Ctx(ctx).Error().Err(err).Msgf("cannot tell request %s in %s: %s", m.TS, m.ChannelID, text)
	}
}

// mentionsSomeoneElse is true when the text mentions a user or user group other than the app.
func mentionsSomeoneElse(text, botUserID string) bool {
	for _, found := range mentionPattern.FindAllStringSubmatch(text, -1) {
		if found[1] != botUserID {
			return true
		}
	}
	return groupPattern.MatchString(text)
}

// plainText drops the app mention and turns Slack markup into plain text.
func plainText(text string) string {
	text = mentionPattern.ReplaceAllString(text, "")
	text = specialPattern.ReplaceAllString(text, "@$1")
	text = linkPattern.ReplaceAllStringFunc(text, func(s string) string {
		parts := linkPattern.FindStringSubmatch(s)
		if parts[2] != "" {
			return parts[2]
		}
		return parts[1]
	})
	return unescaper.Replace(text)
}

func titleOf(text string) string {
	for _, line := range strings.Split(plainText(text), "\n") {
		line = strings.TrimSpace(spacePattern.ReplaceAllString(line, " "))
		if line == "" {
			continue
		}
		if utf8.RuneCountInString(line) > maxTitleRunes {
			line = string([]rune(line)[:maxTitleRunes])
			line = strings.TrimSpace(line) + "…"
		}
		return line
	}
	return fallbackTitle
}

// descriptionOf puts the permalink first and the message after it as a quote, with angle brackets escaped,
// so the owner's markup (an open code fence, raw tags) cannot hide the link or change the page.
func descriptionOf(text, permalink string) string {
	body := bracketEscaper.Replace(strings.TrimSpace(plainText(text)))
	lines := strings.Split(body, "\n")
	for n, line := range lines {
		lines[n] = strings.TrimRight("> "+line, " ")
	}
	return fmt.Sprintf("Slack: %s\n\n%s", permalink, strings.Join(lines, "\n"))
}
