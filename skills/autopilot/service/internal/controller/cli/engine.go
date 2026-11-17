package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/sadensmol/autopilot/internal/agterm"
	"github.com/sadensmol/autopilot/internal/engine"
	"github.com/sadensmol/autopilot/internal/gate/profile"
)

const (
	agtermSocketEnv         = "AGTERM_SOCKET"
	agtermSessionEnv        = "AGTERM_SESSION_ID"
	defaultAgtermBin        = "agtermctl"
	defaultWaveTimeoutMins  = 45
	engineUsage             = "usage: autopilot plan check <plan.json> --base <dir> | plan init <plan.json> --base <dir> --key <KEY> [--parent <session>] | wave spawn|wait|verify|close <base> <n> | wave read <base> <part> | say <base> <from> <to> <message...> | criteria check <criteria.json> --repo <dir> --base <dir>"
	engineWaveSlice         = 60 * time.Second
	engineRealizedAttempts  = 30
	engineDialogAttempts    = 30
	enginePollInterval      = time.Second
	engineParentSessionHint = "the parent session is missing: pass --parent or run inside the run session"
)

// IsEngineCommand reports whether name is a command that the engine runs in this process, not through the service.
func IsEngineCommand(name string) bool {
	return name == "plan" || name == "wave" || name == "say" || name == "criteria"
}

type engineCommand struct {
	valueFlags []string
	run        func(ctx context.Context, e engineRun) (engine.Report, error)
}

// engineRun is what a command gets: its positionals and flags, and a runner built on first use.
type engineRun struct {
	pos    []string
	flags  map[string][]string
	env    Env
	stdout io.Writer
	stderr io.Writer
}

func (e engineRun) flag(name string) string {
	if v := e.flags[name]; len(v) > 0 {
		return v[len(v)-1]
	}
	return ""
}

func (e engineRun) runner() (*engine.Runner, error) {
	socket := e.flag("agterm-socket")
	if socket == "" {
		socket = e.env.Getenv(agtermSocketEnv)
	}
	if socket == "" {
		return nil, fmt.Errorf("the agterm socket is missing: pass --agterm-socket or set %s", agtermSocketEnv)
	}
	bin := e.env.AgtermBin
	if bin == "" {
		bin = defaultAgtermBin
	}
	poll := e.env.PollInterval
	if poll == 0 {
		poll = enginePollInterval
	}
	return engine.NewRunner(engine.RunnerData{
		Sessions:         agterm.New(bin, socket),
		Now:              time.Now,
		PollInterval:     poll,
		WaitSlice:        engineWaveSlice,
		RealizedAttempts: engineRealizedAttempts,
		DialogAttempts:   engineDialogAttempts,
	}), nil
}

var errEngineUsage = errors.New("wrong arguments")

// RunEngine runs the plan, wave and say commands and returns the exit code.
func RunEngine(args []string, env Env, stdout, stderr io.Writer) int {
	if len(args) == 0 || !IsEngineCommand(args[0]) {
		fmt.Fprintln(stderr, engineUsage)
		return ExitUsage
	}
	name, rest := args[0], args[1:]
	if name != "say" {
		if len(rest) == 0 {
			fmt.Fprintln(stderr, engineUsage)
			return ExitUsage
		}
		name, rest = name+" "+rest[0], rest[1:]
	}
	cmd, ok := engineCommands[name]
	if !ok {
		fmt.Fprintf(stderr, "autopilot: unknown command %q\n%s\n", name, engineUsage)
		return ExitUsage
	}
	pos, flags, err := parse(verbSpec{valueFlags: append([]string{"agterm-socket"}, cmd.valueFlags...)}, rest)
	if err != nil {
		fmt.Fprintf(stderr, "autopilot %s: %v\n%s\n", name, err, engineUsage)
		return ExitUsage
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	report, err := cmd.run(ctx, engineRun{pos: pos, flags: flags, env: env, stdout: stdout, stderr: stderr})
	for _, line := range report.Lines {
		fmt.Fprintln(stdout, line)
	}
	switch {
	case errors.Is(err, errEngineUsage):
		fmt.Fprintf(stderr, "autopilot %s: %v\n%s\n", name, err, engineUsage)
		return ExitUsage
	case err != nil:
		fmt.Fprintf(stderr, "autopilot %s: %v\n", name, err)
		return ExitError
	case !report.OK:
		return ExitError
	}
	return ExitOK
}

var engineCommands = map[string]engineCommand{
	"plan check":     {valueFlags: []string{"base"}, run: planCheck},
	"criteria check": {valueFlags: []string{"repo", "base"}, run: criteriaCheck},
	"plan init":      {valueFlags: []string{"base", "key", "parent"}, run: planInit},
	"wave spawn":     {valueFlags: []string{"part"}, run: waveSpawn},
	"wave wait":      {valueFlags: []string{"timeout-minutes"}, run: waveWait},
	"wave verify":    {run: waveVerify},
	"wave close":     {run: waveClose},
	"wave read":      {run: waveRead},
	"say":            {run: say},
}

func planCheck(_ context.Context, e engineRun) (engine.Report, error) {
	if len(e.pos) != 1 {
		return engine.Report{}, errEngineUsage
	}
	p, err := engine.LoadPlan(e.pos[0])
	if err != nil {
		return engine.Report{}, err
	}
	base := e.flag("base")
	if base == "" {
		return engine.Report{}, fmt.Errorf("--base is required, the QA ownership rule reads it: %w", errEngineUsage)
	}
	var parts []engine.Part
	for _, w := range p.Waves {
		parts = append(parts, w.Parts...)
	}
	qaFiles, err := engine.QATestsFor(base, parts)
	if err != nil {
		return engine.Report{}, err
	}
	problems := engine.ValidatePlanQA(p, qaFiles)
	if len(problems) == 0 {
		return engine.Report{Lines: []string{"plan ok"}, OK: true}, nil
	}
	fmt.Fprintf(e.stderr, "PLAN PROBLEMS:\n  - %s\n", strings.Join(problems, "\n  - "))
	return engine.Report{}, nil
}

func criteriaCheck(_ context.Context, e engineRun) (engine.Report, error) {
	repo, base := e.flag("repo"), e.flag("base")
	if len(e.pos) != 1 || repo == "" || base == "" {
		return engine.Report{}, errEngineUsage
	}
	c, err := engine.LoadCriteria(e.pos[0])
	if err != nil {
		return engine.Report{}, err
	}
	prof, err := profile.Detect(repo, nil)
	if err != nil {
		return engine.Report{}, err
	}
	qaFiles, err := engine.LoadQATests(base)
	if err != nil {
		return engine.Report{}, err
	}
	problems := append(engine.ValidateCriteria(c, prof.Language), engine.QAOwnershipProblems(c, qaFiles)...)
	if len(problems) == 0 {
		return engine.Report{Lines: []string{"criteria ok"}, OK: true}, nil
	}
	fmt.Fprintf(e.stderr, "CRITERIA PROBLEMS:\n  - %s\n", strings.Join(problems, "\n  - "))
	return engine.Report{}, nil
}

func planInit(_ context.Context, e engineRun) (engine.Report, error) {
	base, key := e.flag("base"), e.flag("key")
	if len(e.pos) != 1 || base == "" || key == "" {
		return engine.Report{}, errEngineUsage
	}
	parent := e.flag("parent")
	if parent == "" {
		parent = e.env.Getenv(agtermSessionEnv)
	}
	if parent == "" {
		return engine.Report{}, errors.New(engineParentSessionHint)
	}
	p, err := engine.LoadPlan(e.pos[0])
	if err != nil {
		return engine.Report{}, err
	}
	if _, err := engine.Init(base, p, key, parent); err != nil {
		return engine.Report{}, err
	}
	return engine.Report{Lines: []string{fmt.Sprintf("run folder %s", engine.RunFolder(base))}, OK: true}, nil
}

// waveArgs reads <base> <n>.
func waveArgs(e engineRun) (base string, wave int, err error) {
	if len(e.pos) != 2 {
		return "", 0, errEngineUsage
	}
	wave, err = strconv.Atoi(e.pos[1])
	if err != nil || wave < 1 {
		return "", 0, fmt.Errorf("the wave number counts from 1: %w", errEngineUsage)
	}
	return e.pos[0], wave, nil
}

func waveSpawn(ctx context.Context, e engineRun) (engine.Report, error) {
	base, wave, err := waveArgs(e)
	if err != nil {
		return engine.Report{}, err
	}
	r, err := e.runner()
	if err != nil {
		return engine.Report{}, err
	}
	return r.Spawn(ctx, base, wave, e.flag("part"))
}

func waveWait(ctx context.Context, e engineRun) (engine.Report, error) {
	base, wave, err := waveArgs(e)
	if err != nil {
		return engine.Report{}, err
	}
	minutes := defaultWaveTimeoutMins
	if v := e.flag("timeout-minutes"); v != "" {
		minutes, err = strconv.Atoi(v)
		if err != nil || minutes < 1 {
			return engine.Report{}, fmt.Errorf("--timeout-minutes needs a positive number: %w", errEngineUsage)
		}
	}
	r, err := e.runner()
	if err != nil {
		return engine.Report{}, err
	}
	return r.Wait(ctx, base, wave, time.Duration(minutes)*time.Minute)
}

func waveVerify(ctx context.Context, e engineRun) (engine.Report, error) {
	base, wave, err := waveArgs(e)
	if err != nil {
		return engine.Report{}, err
	}
	r, err := e.runner()
	if err != nil {
		return engine.Report{}, err
	}
	return r.Verify(ctx, base, wave)
}

func waveClose(ctx context.Context, e engineRun) (engine.Report, error) {
	base, wave, err := waveArgs(e)
	if err != nil {
		return engine.Report{}, err
	}
	r, err := e.runner()
	if err != nil {
		return engine.Report{}, err
	}
	if err := r.CloseWave(ctx, base, wave); err != nil {
		return engine.Report{}, err
	}
	return engine.Report{Lines: []string{fmt.Sprintf("wave %d closed", wave)}, OK: true}, nil
}

func waveRead(ctx context.Context, e engineRun) (engine.Report, error) {
	if len(e.pos) != 2 {
		return engine.Report{}, errEngineUsage
	}
	r, err := e.runner()
	if err != nil {
		return engine.Report{}, err
	}
	text, err := r.Read(ctx, e.pos[0], e.pos[1])
	if err != nil {
		return engine.Report{}, err
	}
	return engine.Report{Lines: []string{strings.TrimRight(text, "\n")}, OK: true}, nil
}

func say(ctx context.Context, e engineRun) (engine.Report, error) {
	if len(e.pos) < 4 {
		return engine.Report{}, errEngineUsage
	}
	r, err := e.runner()
	if err != nil {
		return engine.Report{}, err
	}
	if err := r.Say(ctx, e.pos[0], e.pos[1], e.pos[2], strings.Join(e.pos[3:], " ")); err != nil {
		return engine.Report{}, err
	}
	return engine.Report{Lines: []string{fmt.Sprintf("sent to %s", e.pos[2])}, OK: true}, nil
}
