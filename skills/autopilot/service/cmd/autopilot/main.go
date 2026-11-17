package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/slack-go/slack"

	"github.com/sadensmol/autopilot/internal/config"
	"github.com/sadensmol/autopilot/internal/controller/cli"
	"github.com/sadensmol/autopilot/internal/linear"
	"github.com/sadensmol/autopilot/internal/slackpost"
	"github.com/sadensmol/autopilot/internal/usecase"
)

var version = "dev"

const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
	// exitAlreadyRunning is the exit code of a serve that found another serve on its folder.
	exitAlreadyRunning = 4
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: autopilot version | check-config | check-linear | check-products | serve | supervise | plan | wave | say | <verb>")
		return exitUsage
	}
	switch args[0] {
	case "version":
		fmt.Println(version)
		return exitOK
	case "check-config":
		return checkConfig(args[1:], os.Stdout, os.Stderr)
	case "check-linear":
		return checkLinear(args[1:], os.Stdout, os.Stderr)
	case "check-products":
		return checkProducts(args[1:], os.Stdout, os.Stderr)
	case "serve":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return serve(ctx, args[1:], os.Stderr)
	case "supervise":
		return supervise(args[1:], os.Stderr)
	default:
		if cli.IsEngineCommand(args[0]) {
			return cli.RunEngine(args, cli.Env{Getenv: os.Getenv, Getwd: os.Getwd}, os.Stdout, os.Stderr)
		}
		if cli.IsVerb(args[0]) {
			return cli.Run(args, cli.Env{Getenv: os.Getenv, Getwd: os.Getwd}, os.Stdout, os.Stderr)
		}
		fmt.Fprintf(os.Stderr, "autopilot: unknown verb %q\n", args[0])
		return exitUsage
	}
}

// checkConfig loads the settings and the workspace registry, prints OK or the error, and returns the exit code.
func checkConfig(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check-config", flag.ContinueOnError)
	fs.SetOutput(stderr)
	settingsDir := fs.String("settings", "", "settings folder (.workspace/autopilot)")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if *settingsDir == "" {
		fmt.Fprintln(stderr, "usage: autopilot check-config --settings <dir>")
		return exitUsage
	}
	if _, _, err := loadConfig(*settingsDir); err != nil {
		fmt.Fprintln(stderr, err)
		return exitError
	}
	fmt.Fprintln(stdout, "OK")
	return exitOK
}

// checkLinear runs the Linear start checks: the API key and its organisation, every team used with its statuses
// and default label, and the Linear project of every project.
func checkLinear(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check-linear", flag.ContinueOnError)
	fs.SetOutput(stderr)
	settingsDir := fs.String("settings", "", "settings folder (.workspace/autopilot)")
	endpoint := fs.String("endpoint", linear.DefaultEndpoint, "GraphQL endpoint (tests only)")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if *settingsDir == "" {
		fmt.Fprintln(stderr, "usage: autopilot check-linear --settings <dir>")
		return exitUsage
	}
	settings, registry, err := loadConfig(*settingsDir)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitError
	}
	client := linear.NewClient(*endpoint, settings.LinearAPIKey, nil)
	err = client.CheckStart(context.Background(), usecase.LinearStartCheck(registry))
	if err != nil {
		fmt.Fprintf(stderr, "linear: %v\n", err)
		return exitError
	}
	fmt.Fprintln(stdout, "OK")
	return exitOK
}

// checkProducts prints one line per project of the registry: its Slack channel, its Linear project and each repo
// folder, each ok or not. It fails when a line fails.
func checkProducts(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check-products", flag.ContinueOnError)
	fs.SetOutput(stderr)
	settingsDir := fs.String("settings", "", "settings folder (.workspace/autopilot)")
	endpoint := fs.String("endpoint", linear.DefaultEndpoint, "Linear GraphQL endpoint (tests only)")
	slackAPI := fs.String("slack-api", "", "Slack web API URL (tests only)")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if *settingsDir == "" {
		fmt.Fprintln(stderr, "usage: autopilot check-products --settings <dir>")
		return exitUsage
	}
	settings, registry, err := loadConfig(*settingsDir)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitError
	}
	var opts []slack.Option
	if *slackAPI != "" {
		opts = append(opts, slack.OptionAPIURL(*slackAPI))
	}
	reports, err := usecase.CheckProducts(context.Background(), usecase.CheckProductsData{
		Registry: registry,
		RepoPath: settings.RepoPath,
		Slack:    slackpost.NewPoster(slack.New(settings.SlackBotToken, opts...)),
		Linear:   linear.NewClient(*endpoint, settings.LinearAPIKey, nil),
	})
	if err != nil {
		fmt.Fprintf(stderr, "slack: %v\n", err)
		return exitError
	}
	code := exitOK
	for _, r := range reports {
		fmt.Fprintln(stdout, r.Line())
		if r.Err() != nil {
			code = exitError
		}
	}
	return code
}

// loadConfig loads the settings folder and the workspace registry of its workspace folder.
func loadConfig(settingsDir string) (config.Settings, config.Registry, error) {
	settings, err := config.LoadSettings(settingsDir)
	if err != nil {
		return config.Settings{}, config.Registry{}, fmt.Errorf("settings: %w", err)
	}
	registry, err := config.LoadRegistry(settings.WorkspaceFolder)
	if err != nil {
		return config.Settings{}, config.Registry{}, fmt.Errorf("registry: %w", err)
	}
	return settings, registry, nil
}
