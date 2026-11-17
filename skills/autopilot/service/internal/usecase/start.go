package usecase

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/sadensmol/autopilot/internal/agterm"
	"github.com/sadensmol/autopilot/internal/config"
	"github.com/sadensmol/autopilot/internal/domain"
	"github.com/sadensmol/autopilot/internal/git"
	"github.com/sadensmol/autopilot/internal/linear"
	"github.com/sadensmol/autopilot/internal/slackpost"
)

// ICheckLinear runs the Linear start checks.
type ICheckLinear interface {
	CheckStart(ctx context.Context, check linear.StartCheck) error
}

// IChannels lists the Slack channels the bot can see.
type IChannels interface {
	Channels(ctx context.Context) ([]slackpost.Channel, error)
}

// LinearStartCheck is the Linear part of the start checks: the organisation, and in linear.team (when set) and in
// the team of every project the statuses and the default label, then the Linear project of every project that
// names one.
func LinearStartCheck(registry config.Registry) linear.StartCheck {
	check := linear.StartCheck{
		Workspace:    registry.Linear.Workspace,
		Statuses:     registry.Linear.Statuses,
		DefaultLabel: registry.Linear.DefaultLabel,
	}
	if registry.Linear.Team != "" {
		check.TeamKeys = append(check.TeamKeys, registry.Linear.Team)
	}
	for _, p := range registry.Projects {
		check.TeamKeys = append(check.TeamKeys, p.LinearTeam)
		if p.LinearProject != "" {
			check.Projects = append(check.Projects, linear.ProjectRef{TeamKey: p.LinearTeam, Name: p.LinearProject})
		}
	}
	return check
}

// StartChecks runs the start checks of the service and returns the served projects by Slack channel id.
// It fails with the name that does not match: the Linear organisation, a status, a team, a project or a channel.
func StartChecks(ctx context.Context, lin ICheckLinear, slack IChannels, registry config.Registry) (map[string]config.Project, error) {
	if err := lin.CheckStart(ctx, LinearStartCheck(registry)); err != nil {
		return nil, fmt.Errorf("linear start check: %w", err)
	}
	channels, err := projectChannels(ctx, slack, registry.Projects)
	if err != nil {
		return nil, fmt.Errorf("slack start check: %w", err)
	}
	served := make(map[string]config.Project, len(registry.Projects))
	for n, p := range registry.Projects {
		if err := channels[n].Err; err != nil {
			return nil, fmt.Errorf("slack start check: %w: create the channel or invite the bot to it", err)
		}
		served[channels[n].ID] = p
	}
	return served, nil
}

const (
	runFolder       = "run"
	issueFile       = "issue.md"
	maxSessionTitle = 50
	waitNote        = "the run session is not ready yet: waiting for the Mac to wake"
	// controlSession is the name of the control room session; run sessions sit right after it, at the top.
	controlSession = "auto main"
	neverNote       = "the run session never started; check the Mac and restart the run"
)

// IStartLinear is the part of the Linear client that the run start uses.
type IStartLinear interface {
	IssueByKey(ctx context.Context, key string) (linear.Issue, error)
	Comment(ctx context.Context, issueID, body string) error
}

// IStartSlack is the part of the Slack poster that the run start uses.
type IStartSlack interface {
	IReactSlack
	Thread(ctx context.Context, channelID, threadTS, text string) error
}

// IStartRuns is the part of the run store that the run start uses.
type IStartRuns interface {
	SetSession(ctx context.Context, key domain.IssueKey, sessionID, worktreeBase string, now time.Time) error
	End(ctx context.Context, key domain.IssueKey, now time.Time) error
}

// IStartRequests is the part of the intake record that the run start uses.
type IStartRequests interface {
	Claim(ctx context.Context, channelID, requestTS string, now time.Time) (domain.IntakeRequest, error)
	SetState(ctx context.Context, channelID, requestTS string, state domain.IntakeState, now time.Time) error
}

// IWorktrees makes and removes the git worktrees of a run.
type IWorktrees interface {
	AddWorktree(ctx context.Context, d git.AddWorktreeData) (git.Added, error)
	RemoveWorktree(ctx context.Context, repo, dir, branch string, deleteBranch bool) error
}

// ISessions drives the agterm sessions.
type ISessions interface {
	NewSession(ctx context.Context, d agterm.NewSessionData) (string, error)
	SessionByName(ctx context.Context, workspace, name string) (string, error)
	MoveToTop(ctx context.Context, sessionID string) error
	SetContext(ctx context.Context, sessionID, text string) error
	Realized(ctx context.Context, sessionID string) (bool, error)
	Text(ctx context.Context, sessionID string) (string, error)
	Type(ctx context.Context, sessionID, text string) error
	Close(ctx context.Context, sessionID string) error
}

type RunStarterData struct {
	WorkspaceFolder string
	RepoPath        func(repo string) string
	AgtermWorkspace string
	Linear          IStartLinear
	Slack           IStartSlack
	Runs            IStartRuns
	Requests        IStartRequests
	Worktrees       IWorktrees
	Sessions        ISessions
	Now             func() time.Time
	// Background is the service context: the watch of a started session stops when it ends.
	Background context.Context
	// PollInterval is the pause between two polls of a session.
	PollInterval time.Duration
	// RealizedAttempts bounds the wait for a session to get a terminal.
	RealizedAttempts int
	// DialogAttempts bounds the watch for the start dialogs after the session is realized.
	DialogAttempts int
}

// RunStarter makes a run's worktrees and starts its session.
type RunStarter struct {
	d  RunStarterData
	wg sync.WaitGroup
}

func NewRunStarter(d RunStarterData) *RunStarter {
	return &RunStarter{d: d}
}

// Wait blocks until every background watch has ended; the service calls it after its context ends.
func (s *RunStarter) Wait() {
	s.wg.Wait()
}

type madeWorktree struct {
	repo, dir, branch string
	createdBranch     bool
}

// watch is what the background goroutine of a started session needs.
type watch struct {
	run       domain.Run
	sessionID string
}

// startState is what a start made so far; a failure undoes it.
type startState struct {
	worktrees []madeWorktree
	sessionID string
}

// Start makes one worktree per repo on the issue's branch, then the run session. On a failure it undoes what it made,
// tells the thread, comments on the issue, ends the run and marks the intake row failed.
func (s *RunStarter) Start(ctx context.Context, run domain.Run, product config.Project) error {
	var made startState
	err := s.launch(ctx, run, product, &made)
	if err == nil {
		return nil
	}
	s.undo(ctx, made)
	s.report(ctx, run, err)
	return err
}

func (s *RunStarter) launch(ctx context.Context, run domain.Run, product config.Project, made *startState) error {
	issue, err := s.d.Linear.IssueByKey(ctx, string(run.IssueKey))
	if err != nil {
		return fmt.Errorf("cannot read issue %s: %w", run.IssueKey, err)
	}
	if issue.Branch == "" {
		return fmt.Errorf("issue %s has no branch name", run.IssueKey)
	}
	base := filepath.Join(s.d.WorkspaceFolder, domain.RunsFolder, string(run.IssueKey))
	for _, repo := range product.Repos {
		w := madeWorktree{repo: s.d.RepoPath(repo.Name), dir: filepath.Join(base, repo.Name), branch: issue.Branch}
		added, err := s.d.Worktrees.AddWorktree(ctx, git.AddWorktreeData{
			Repo: w.repo, Dir: w.dir, Branch: w.branch, DefaultBranch: repo.DefaultBranch,
		})
		if err != nil {
			return fmt.Errorf("cannot make the worktree of %s: %w", repo.Name, err)
		}
		if added.Worktree {
			w.createdBranch = added.Branch
			made.worktrees = append(made.worktrees, w)
		}
	}
	if err := os.MkdirAll(filepath.Join(base, runFolder), 0o755); err != nil {
		return fmt.Errorf("cannot make the run folder: %w", err)
	}
	if err := writeIssueFile(filepath.Join(base, runFolder, issueFile), issue); err != nil {
		return fmt.Errorf("cannot write %s: %w", issueFile, err)
	}
	control, err := s.d.Sessions.SessionByName(ctx, s.d.AgtermWorkspace, controlSession)
	if err != nil {
		return fmt.Errorf("cannot find the control room session: %w", err)
	}
	if err := s.d.Sessions.MoveToTop(ctx, control); err != nil {
		log.Warn().Ctx(ctx).Err(err).Msgf("cannot move session %s to the top of workspace %s", control, s.d.AgtermWorkspace)
	}
	made.sessionID, err = s.d.Sessions.NewSession(ctx, agterm.NewSessionData{
		After:   control,
		Cwd:     base,
		Name:    fmt.Sprintf("auto %s · %s", run.IssueKey, shortTitle(issue.Title)),
		Command: fmt.Sprintf("claude --dangerously-skip-permissions '/autopilot run %s'", run.IssueKey),
	})
	if err != nil {
		return fmt.Errorf("cannot create the run session: %w", err)
	}
	if err := s.d.Runs.SetSession(ctx, run.IssueKey, made.sessionID, base, s.d.Now()); err != nil {
		return fmt.Errorf("cannot store the run session: %w", err)
	}
	if err := s.d.Sessions.SetContext(ctx, made.sessionID, issue.URL); err != nil {
		return fmt.Errorf("cannot set the session context: %w", err)
	}
	s.wg.Add(1)
	go s.watchSession(watch{run: run, sessionID: made.sessionID})
	return nil
}

// watchSession waits for the session to get a terminal, then answers the start dialogs.
// It runs after Start returned, so a sleeping Mac never blocks intake.
func (s *RunStarter) watchSession(w watch) {
	defer s.wg.Done()
	ctx := s.d.Background
	realized, err := s.waitRealized(ctx, w)
	if err == nil && realized {
		err = s.answerDialog(ctx, w.sessionID)
	}
	if err == nil || ctx.Err() != nil {
		return
	}
	if errors.Is(err, agterm.ErrNoSession) {
		s.report(ctx, w.run, errors.New("the run session exited before it started work"))
		return
	}
	log.Error().Ctx(ctx).Err(err).Msgf("cannot watch the run session of %s", w.run.IssueKey)
	s.say(ctx, w.run, fmt.Sprintf("the run session of %s could not be watched: %s", w.run.IssueKey, slackpost.Escape(err.Error())))
}

func (s *RunStarter) say(ctx context.Context, run domain.Run, text string) {
	if err := s.d.Slack.Thread(context.WithoutCancel(ctx), run.ChannelID, run.ThreadTS, text); err != nil {
		log.Warn().Ctx(ctx).Err(err).Msgf("cannot post to the thread of %s", run.IssueKey)
	}
}

// waitRealized polls until the session has a terminal; it posts the wait note once when it has not,
// and a final line when the session never gets one.
func (s *RunStarter) waitRealized(ctx context.Context, w watch) (bool, error) {
	noted := false
	for range s.d.RealizedAttempts {
		ok, err := s.d.Sessions.Realized(ctx, w.sessionID)
		if err != nil {
			return false, fmt.Errorf("cannot check the run session: %w", err)
		}
		if ok {
			return true, nil
		}
		if !noted {
			noted = true
			s.say(ctx, w.run, waitNote)
		}
		if err := sleep(ctx, s.d.PollInterval); err != nil {
			return false, err
		}
	}
	s.say(ctx, w.run, neverNote)
	return false, nil
}

// answerDialog accepts each start dialog (folder trust, permission bypass) once when its exact text is on screen,
// until the prompt is ready; it types nothing otherwise.
func (s *RunStarter) answerDialog(ctx context.Context, sessionID string) error {
	answered := map[agterm.Dialog]bool{}
	for range s.d.DialogAttempts {
		text, err := s.d.Sessions.Text(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("cannot read the run session: %w", err)
		}
		if agterm.Ready(text) {
			return nil
		}
		if dialog, answer, ok := agterm.DialogAnswer(text); ok && !answered[dialog] {
			for _, keys := range answer {
				if err := s.d.Sessions.Type(ctx, sessionID, keys); err != nil {
					return fmt.Errorf("cannot answer the %s dialog: %w", dialog, err)
				}
			}
			answered[dialog] = true
		}
		if err := sleep(ctx, s.d.PollInterval); err != nil {
			return err
		}
	}
	return nil
}

// undo removes what a failed start made. Each failure is logged: nothing else can report it.
func (s *RunStarter) undo(ctx context.Context, made startState) {
	ctx = context.WithoutCancel(ctx)
	if made.sessionID != "" {
		if err := s.d.Sessions.Close(ctx, made.sessionID); err != nil {
			log.Error().Ctx(ctx).Err(err).Msgf("cannot close session %s after a failed start", made.sessionID)
		}
	}
	for _, w := range made.worktrees {
		if err := s.d.Worktrees.RemoveWorktree(ctx, w.repo, w.dir, w.branch, w.createdBranch); err != nil {
			log.Error().Ctx(ctx).Err(err).Msgf("cannot remove worktree %s after a failed start", w.dir)
		}
	}
}

// report tells the thread and the issue, ends the run and marks the intake row failed.
func (s *RunStarter) report(ctx context.Context, run domain.Run, cause error) {
	ctx = context.WithoutCancel(ctx)
	text := fmt.Sprintf("could not start the run for %s: %s", run.IssueKey, cause)
	if err := s.d.Slack.Thread(ctx, run.ChannelID, run.ThreadTS, slackpost.Escape(text)); err != nil {
		log.Error().Ctx(ctx).Err(err).Msgf("cannot tell the thread that %s did not start", run.IssueKey)
	}
	req, err := s.d.Requests.Claim(ctx, run.ChannelID, run.RequestTS, s.d.Now())
	switch {
	case err != nil:
		log.Error().Ctx(ctx).Err(err).Msgf("cannot read the intake row of %s", run.IssueKey)
	case req.Issue.ID == "":
		log.Error().Ctx(ctx).Err(errors.New("issue id is missing")).Msgf("cannot comment on %s", run.IssueKey)
	default:
		if err := s.d.Linear.Comment(ctx, req.Issue.ID, "autopilot could not start the run: "+cause.Error()); err != nil {
			log.Error().Ctx(ctx).Err(err).Msgf("cannot comment on %s", run.IssueKey)
		}
	}
	if err := s.d.Runs.End(ctx, run.IssueKey, s.d.Now()); err != nil {
		log.Error().Ctx(ctx).Err(err).Msgf("cannot end run %s", run.IssueKey)
	}
	clearWorking(ctx, s.d.Slack, run)
	if err := s.d.Requests.SetState(ctx, run.ChannelID, run.RequestTS, domain.IntakeFailed, s.d.Now()); err != nil {
		log.Error().Ctx(ctx).Err(err).Msgf("cannot mark the intake row of %s failed", run.IssueKey)
	}
}

func shortTitle(title string) string {
	r := []rune(strings.TrimSpace(title))
	if len(r) > maxSessionTitle {
		return string(r[:maxSessionTitle-1]) + "…"
	}
	return string(r)
}

func sleep(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// writeIssueFile writes the issue for the run session to read. The description goes in verbatim under its heading:
// it is data from Linear, never instructions. The write is atomic: a temp file in the same folder, then a rename.
func writeIssueFile(path string, issue linear.Issue) error {
	text := fmt.Sprintf("# %s: %s\n\nURL: %s\n\n## Description\n\n%s\n", issue.Key, issue.Title, issue.URL, issue.Description)
	tmp, err := os.CreateTemp(filepath.Dir(path), ".issue-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(text); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
