package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const (
	maxRestarts     = 5
	restartWindow   = 10 * time.Minute
	backoffStart    = time.Second
	backoffCeiling  = time.Minute
	childStopGrace  = 30 * time.Second
	childStopSignal = syscall.SIGKILL
)

// process is a running serve.
type process interface {
	Wait() error
	Signal(sig os.Signal) error
}

// superviseSignals are the signals the supervisor forwards to serve. SIGHUP is one of them: a closed terminal
// must not orphan a serve that holds the lock. SIGKILL of the supervisor cannot be caught.
var superviseSignals = []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}

type superviseData struct {
	Start   func() (process, error)
	Signals <-chan os.Signal
	Now     func() time.Time
	// After fires once the duration has passed.
	After func(d time.Duration) <-chan time.Time
}

type superviseFlags struct {
	Settings     string
	AgtermSocket string
}

// supervise keeps one serve running: it restarts a serve that dies, and forwards SIGTERM and SIGINT to it.
func supervise(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("supervise", flag.ContinueOnError)
	fs.SetOutput(stderr)
	flags := superviseFlags{}
	fs.StringVar(&flags.Settings, "settings", "", "settings folder (.workspace/autopilot)")
	fs.StringVar(&flags.AgtermSocket, "agterm-socket", "", "agterm control socket (default: AGTERM_SOCKET of the environment)")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if flags.AgtermSocket == "" {
		flags.AgtermSocket = os.Getenv("AGTERM_SOCKET")
	}
	if flags.Settings == "" || flags.AgtermSocket == "" {
		fmt.Fprintln(stderr, "usage: autopilot supervise --settings <dir> --agterm-socket <path>")
		return exitUsage
	}
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintf(stderr, "autopilot supervise: cannot find the autopilot program: %v\n", err)
		return exitError
	}
	env := os.Environ()
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, superviseSignals...)
	defer signal.Stop(signals)
	return runSupervisor(superviseData{
		Start: func() (process, error) {
			cmd := childCommand(self, flags, env)
			cmd.Stdout, cmd.Stderr = os.Stdout, stderr
			if err := cmd.Start(); err != nil {
				return nil, err
			}
			return osProcess{cmd}, nil
		},
		Signals: signals,
		Now:     time.Now,
		After:   time.After,
	}, stderr)
}

type osProcess struct{ cmd *exec.Cmd }

func (p osProcess) Wait() error                { return p.cmd.Wait() }
func (p osProcess) Signal(sig os.Signal) error { return p.cmd.Process.Signal(sig) }

// childCommand builds the serve command. The child gets no AGTERM_* variable: it reaches agterm through the
// flag only, so it never reports to the session that started the supervisor.
func childCommand(self string, f superviseFlags, env []string) *exec.Cmd {
	cmd := exec.Command(self, "serve", "--settings", f.Settings, "--agterm-socket", f.AgtermSocket)
	cmd.Env = make([]string, 0, len(env))
	for _, kv := range env {
		if !strings.HasPrefix(kv, "AGTERM_") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	return cmd
}

// runSupervisor starts serve and restarts it with a doubling backoff. It gives up when serve dies once more after
// maxRestarts restarts within restartWindow. A signal goes to the child, and the supervisor exits cleanly.
func runSupervisor(d superviseData, stderr io.Writer) int {
	var restarts []time.Time
	for {
		child, err := d.Start()
		var died error
		if err != nil {
			died = fmt.Errorf("cannot start serve: %w", err)
		} else {
			exited := make(chan error, 1)
			go func() { exited <- child.Wait() }()
			select {
			case sig := <-d.Signals:
				stopChild(d, child, sig, exited, stderr)
				return exitOK
			case died = <-exited:
				if died == nil {
					died = fmt.Errorf("serve exited without an error")
				}
				var exit interface{ ExitCode() int }
				if errors.As(died, &exit) && exit.ExitCode() == exitAlreadyRunning {
					fmt.Fprintln(stderr, "autopilot supervise: serve is already running on this folder: stop the other serve first")
					return exitError
				}
			}
		}
		now := d.Now()
		restarts = recentRestarts(restarts, now)
		if len(restarts) >= maxRestarts {
			fmt.Fprintf(stderr, "autopilot supervise: giving up: serve died again after %d restarts in %s; last failure: %v\n", maxRestarts, restartWindow, died)
			return exitError
		}
		delay := min(backoffStart<<len(restarts), backoffCeiling)
		fmt.Fprintf(stderr, "autopilot supervise: serve died (%v); restart %d in %s\n", died, len(restarts)+1, delay)
		restarts = append(restarts, now)
		select {
		case <-d.After(delay):
		case <-d.Signals:
			return exitOK
		}
	}
}

func recentRestarts(restarts []time.Time, now time.Time) []time.Time {
	var recent []time.Time
	for _, at := range restarts {
		if now.Sub(at) < restartWindow {
			recent = append(recent, at)
		}
	}
	return recent
}

// stopChild forwards the signal and waits for the child; a child that does not stop in time is killed.
func stopChild(d superviseData, child process, sig os.Signal, exited <-chan error, stderr io.Writer) {
	if err := child.Signal(sig); err != nil {
		fmt.Fprintf(stderr, "autopilot supervise: cannot signal serve: %v\n", err)
	}
	select {
	case <-exited:
		return
	default:
	}
	select {
	case <-exited:
	case <-d.After(childStopGrace):
		if err := child.Signal(childStopSignal); err != nil {
			fmt.Fprintf(stderr, "autopilot supervise: cannot kill serve: %v\n", err)
		}
		<-exited
	}
}
