package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/socketmode"

	"github.com/sadensmol/autopilot/internal/agterm"
	controllerslack "github.com/sadensmol/autopilot/internal/controller/slack"
	"github.com/sadensmol/autopilot/internal/controller/socket"
	"github.com/sadensmol/autopilot/internal/gate"
	"github.com/sadensmol/autopilot/internal/gate/profile"
	"github.com/sadensmol/autopilot/internal/gh"
	"github.com/sadensmol/autopilot/internal/git"
	"github.com/sadensmol/autopilot/internal/linear"
	"github.com/sadensmol/autopilot/internal/repository"
	"github.com/sadensmol/autopilot/internal/shell"
	"github.com/sadensmol/autopilot/internal/slackpost"
	"github.com/sadensmol/autopilot/internal/usecase"
)

// serve loads the configuration, runs the start checks and listens to Slack until ctx ends.
// It refuses to start, naming the failing key, name or file.
func serve(ctx context.Context, args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	settingsDir := fs.String("settings", "", "settings folder (.workspace/autopilot)")
	socketFlag := fs.String("agterm-socket", "", "agterm control socket (default: AGTERM_SOCKET of the environment)")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if *settingsDir == "" {
		fmt.Fprintln(stderr, "usage: autopilot serve --settings <dir>")
		return exitUsage
	}
	agtermSocket := *socketFlag
	if agtermSocket == "" {
		agtermSocket = os.Getenv("AGTERM_SOCKET")
	}
	if agtermSocket == "" {
		fmt.Fprintln(stderr, "autopilot serve: the agterm socket is missing: pass --agterm-socket or set AGTERM_SOCKET")
		return exitUsage
	}
	scrubAgtermEnv()
	if err := runServe(ctx, *settingsDir, agtermSocket); err != nil {
		fmt.Fprintf(stderr, "autopilot serve: %v\n", err)
		if errors.Is(err, socket.ErrAlreadyRunning) {
			return exitAlreadyRunning
		}
		return exitError
	}
	return exitOK
}

func runServe(ctx context.Context, settingsDir, agtermSocket string) error {
	settings, registry, err := loadConfig(settingsDir)
	if err != nil {
		return err
	}
	// the socket's exclusive lock is the one-serve-per-folder guard; it comes first, so a second serve
	// stops before it opens the store or starts a loop
	mux := socket.NewMux()
	server, err := socket.NewServer(filepath.Join(settings.WorkspaceFolder, ".workspace", "autopilot", socket.FileName), mux)
	if err != nil {
		return err
	}
	serving := false
	defer func() {
		if serving {
			return
		}
		if err := server.Close(); err != nil {
			log.Error().Err(err).Msg("cannot close the socket of a serve that never served")
		}
	}()
	db, err := repository.Open(ctx, filepath.Join(settingsDir, repository.DBFileName))
	if err != nil {
		return err
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Error().Err(err).Msg("cannot close the store")
		}
	}()

	mux.Register(socket.VerbPing, socket.PingHandler)
	mux.Register(socket.VerbRuns, socket.NewRunsHandler(repository.NewRun(db)))
	api := slack.New(settings.SlackBotToken, slack.OptionAppLevelToken(settings.SlackAppToken))
	poster := slackpost.NewPoster(api)
	linearClient := linear.NewClient(linear.DefaultEndpoint, settings.LinearAPIKey, nil)
	agtermClient := agterm.New("agtermctl", agtermSocket)
	asks := usecase.NewAsk(usecase.AskData{
		OwnerID:  settings.Owner.SlackUserID,
		Registry: registry,
		Linear:   linearClient,
		Slack:    poster,
		Runs:     repository.NewRun(db),
		Asks:     repository.NewAsk(db),
		Sessions: agtermClient,
		Now:      timeNow,
		Sleep:    sleepContext,
	})
	mux.Register(socket.VerbAsk, socket.NewAskHandler(asks))
	mux.Register(socket.VerbAskWait, socket.NewAskWaitHandler(asks))
	status := usecase.NewStatus(usecase.StatusData{
		Registry: registry,
		LoopCap:  settings.LoopCap,
		Linear:   linearClient,
		Slack:    poster,
		Runs:     repository.NewRun(db),
		Asks:     repository.NewAsk(db),
		Guard:    asks,
		Sessions: agtermClient,
		Now:      timeNow,
	})
	mux.Register(socket.VerbStatus, socket.NewStatusHandler(status))
	mux.Register(socket.VerbResume, socket.NewResumeHandler(status))
	mux.Register(socket.VerbPost, socket.NewPostHandler(status))
	gates := usecase.NewGate(usecase.GateData{
		Registry: registry,
		Runs:     repository.NewRun(db),
		Verdicts: repository.NewVerdict(db),
		Checker:  gate.New(gate.Data{Detect: gate.DetectProfile, Timeout: time.Duration(settings.GateTimeoutMinutes) * time.Minute}),
		Now:      timeNow,
	})
	mux.Register(socket.VerbRed, socket.NewRedHandler(gates))
	mux.Register(socket.VerbGate, socket.NewGateHandler(gates))
	ghClient := gh.New("gh", ghTimeout)
	prOpen := usecase.NewPROpen(usecase.PROpenData{
		Registry: registry,
		GH:       ghClient,
		OwnerID:  settings.Owner.SlackUserID,
		Slack:    poster,
		Runs:     repository.NewRun(db),
		Verdicts: repository.NewVerdict(db),
		Guard:    asks,
		Now:      timeNow,
	})
	mux.Register(socket.VerbPR, socket.NewPRHandler(prOpen))
	mux.Register(socket.VerbManual, socket.NewManualHandler(prOpen))
	heartbeat := usecase.NewHeartbeat(usecase.HeartbeatData{
		Registry: registry,
		Quiet:    time.Duration(settings.HeartbeatMinutes) * time.Minute,
		Tick:     heartbeatTick,
		Linear:   linearClient,
		Runs:     repository.NewRun(db),
		Asks:     repository.NewAsk(db),
		Guard:    asks,
		Sessions: agtermClient,
		Now:      timeNow,
		Sleep:    sleepContext,
	})
	heartbeatCtx, stopHeartbeat := context.WithCancel(ctx)
	heartbeatDone := make(chan error, 1)
	go func() { heartbeatDone <- heartbeat.Run(heartbeatCtx) }()
	defer func() {
		// the loop ends with ctx; a serve that fails earlier must still stop it before the store closes
		stopHeartbeat()
		if err := <-heartbeatDone; err != nil && !errors.Is(err, context.Canceled) {
			log.Error().Err(err).Msg("heartbeat stopped with an error")
		}
	}()

	prWatch := usecase.NewPRWatch(usecase.PRWatchData{
		Registry:  registry,
		Interval:  time.Duration(settings.PRPollMinutes) * time.Minute,
		Linear:    linearClient,
		OwnerID:   settings.Owner.SlackUserID,
		Slack:     poster,
		Runs:      repository.NewRun(db),
		Asks:      asks,
		Sessions:  agtermClient,
		GH:        ghClient,
		Worktrees: git.New(),
		RepoPath:  settings.RepoPath,
		Guard:     asks,
		Now:       timeNow,
		Sleep:     sleepContext,
	})
	canceller := usecase.NewCanceller(usecase.CancellerData{
		Registry: registry,
		Linear:   linearClient,
		Slack:    poster,
		Runs:     repository.NewRun(db),
		Asks:     asks,
		Sessions: agtermClient,
		Guard:    asks,
		Now:      timeNow,
	})
	mux.Register(socket.VerbComment, socket.NewCommentHandler(usecase.NewCommenter(usecase.CommenterData{
		Linear: linearClient,
		Runs:   repository.NewRun(db),
	})))
	mux.Register(socket.VerbCancel, socket.NewCancelHandler(canceller))
	prMerge := usecase.NewPRMerge(usecase.PRMergeData{
		GH:      ghClient,
		OwnerID: settings.Owner.SlackUserID,
		Slack:   poster,
		Runs:    repository.NewRun(db),
		Now:     timeNow,
	})
	mux.Register(socket.VerbFix, socket.NewFixHandler(usecase.NewFixMerge(usecase.FixMergeData{
		Registry: registry,
		GH:       ghClient,
		Slack:    poster,
		Runs:     repository.NewRun(db),
		Now:      timeNow,
		Sleep:    sleepContext,
	})))
	prWatchCtx, stopPRWatch := context.WithCancel(ctx)
	prWatchDone := make(chan error, 1)
	go func() { prWatchDone <- prWatch.Run(prWatchCtx) }()
	defer func() {
		// the loop ends with ctx; a serve that fails earlier must still stop it before the store closes
		stopPRWatch()
		if err := <-prWatchDone; err != nil && !errors.Is(err, context.Canceled) {
			log.Error().Err(err).Msg("PR watch stopped with an error")
		}
	}()
	deployWatch := usecase.NewDeployWatch(usecase.DeployWatchData{
		Registry:        registry,
		Interval:        time.Duration(settings.PRPollMinutes) * time.Minute,
		Linear:          linearClient,
		Slack:           poster,
		Runs:            repository.NewRun(db),
		GH:              ghClient,
		Checks:          shell.New(),
		AgtermWorkspace: registry.AgtermWorkspace,
		Sessions:        agtermClient,
		Now:             timeNow,
		Sleep:           sleepContext,
	})
	deployWatchCtx, stopDeployWatch := context.WithCancel(ctx)
	deployWatchDone := make(chan error, 1)
	go func() { deployWatchDone <- deployWatch.Run(deployWatchCtx) }()
	defer func() {
		// the loop ends with ctx; a serve that fails earlier must still stop it before the store closes
		stopDeployWatch()
		if err := <-deployWatchDone; err != nil && !errors.Is(err, context.Canceled) {
			log.Error().Err(err).Msg("deploy watch stopped with an error")
		}
	}()

	serving = true
	socketCtx, stopSocket := context.WithCancel(ctx)
	socketDone := make(chan error, 1)
	go func() { socketDone <- server.Serve(socketCtx) }()
	defer func() {
		// Serve ends when ctx ends; a run that stops earlier must still close and unlink the socket
		stopSocket()
		if err := <-socketDone; err != nil {
			log.Error().Err(err).Msg("socket server stopped with an error")
		}
	}()

	served, err := usecase.StartChecks(ctx, linearClient, poster, registry)
	if err != nil {
		return err
	}
	auth, err := api.AuthTestContext(ctx)
	if err != nil {
		return fmt.Errorf("slack auth check: %w", err)
	}
	if key := workspaceKey(auth.URL); key != strings.ToLower(registry.Slack.Workspace) {
		return fmt.Errorf("slack start check: bot workspace %q, configured slack.workspace %q", key, registry.Slack.Workspace)
	}

	starter := usecase.NewRunStarter(usecase.RunStarterData{
		WorkspaceFolder:  settings.WorkspaceFolder,
		RepoPath:         settings.RepoPath,
		AgtermWorkspace:  registry.AgtermWorkspace,
		Linear:           linearClient,
		Slack:            poster,
		Runs:             repository.NewRun(db),
		Requests:         repository.NewIntakeRequest(db),
		Worktrees:        git.New(),
		Sessions:         agtermClient,
		Now:              timeNow,
		Background:       ctx,
		PollInterval:     startPollInterval,
		RealizedAttempts: realizedAttempts,
		DialogAttempts:   dialogAttempts,
	})
	intake := usecase.NewIntake(usecase.IntakeData{
		BotUserID:       auth.UserID,
		Registry:        registry,
		ChannelProducts: served,
		RepoPath:        settings.RepoPath,
		Linear:          linearClient,
		Slack:           poster,
		Runs:            repository.NewRun(db),
		Requests:        repository.NewIntakeRequest(db),
		Starter:         starter,
		Replies:         asks,
		Usable:          profile.Usable,
		Now:             timeNow,
	})
	channelIDs := make([]string, 0, len(served))
	for id := range served {
		channelIDs = append(channelIDs, id)
	}
	handler := controllerslack.NewHandler(ctx, controllerslack.HandlerData{
		OwnerID:    settings.Owner.SlackUserID,
		ChannelIDs: channelIDs,
		Intake:     intake,
		Canceller:  canceller,
		Merger:     prMerge,
	})
	socket := socketmode.New(api)
	log.Info().Msgf("serving %d channel(s)", len(channelIDs))
	err = controllerslack.Listen(ctx, socket, socket.Events, handler)
	handler.Wait()
	starter.Wait()
	return err
}

const (
	startPollInterval = 5 * time.Second
	heartbeatTick     = time.Minute
	ghTimeout         = time.Minute
	realizedAttempts  = 720 // one hour for a sleeping Mac
	dialogAttempts    = 12  // one minute after the session is realized
)

// scrubAgtermEnv removes every AGTERM_* variable, so the service never reports to the session that started it.
func scrubAgtermEnv() {
	for _, kv := range os.Environ() {
		if name, _, _ := strings.Cut(kv, "="); strings.HasPrefix(name, "AGTERM_") {
			os.Unsetenv(name)
		}
	}
}

// workspaceKey is the first label of the workspace URL, such as acme for https://acme.slack.com/.
func workspaceKey(workspaceURL string) string {
	u, err := url.Parse(workspaceURL)
	if err != nil {
		return ""
	}
	key, _, _ := strings.Cut(u.Hostname(), ".")
	return strings.ToLower(key)
}
