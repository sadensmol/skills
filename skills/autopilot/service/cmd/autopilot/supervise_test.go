package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// fakeExit is the error of a child that exited with a code.
type fakeExit int

func (e fakeExit) Error() string { return fmt.Sprintf("exit status %d", int(e)) }
func (e fakeExit) ExitCode() int { return int(e) }

type fakeProcess struct {
	mu       sync.Mutex
	signals  []os.Signal
	done     chan error
	exitSelf bool
	// stubborn ignores everything but SIGKILL.
	stubborn bool
}

func newFakeProcess(exitSelf bool) *fakeProcess {
	p := &fakeProcess{done: make(chan error, 1), exitSelf: exitSelf}
	if exitSelf {
		p.done <- errors.New("exit status 1")
	}
	return p
}

func (p *fakeProcess) Wait() error { return <-p.done }

func (p *fakeProcess) Signal(sig os.Signal) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.signals = append(p.signals, sig)
	if sig == syscall.SIGKILL || (!p.stubborn && (sig == syscall.SIGTERM || sig == syscall.SIGINT || sig == syscall.SIGHUP)) {
		select {
		case p.done <- nil:
		default:
		}
	}
	return nil
}

func (p *fakeProcess) got() []os.Signal {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.signals)
}

type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	delays []time.Duration
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// After records the delay, moves the clock by it and fires at once.
func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.delays = append(c.delays, d)
	c.now = c.now.Add(d)
	ch := make(chan time.Time, 1)
	ch <- c.now
	return ch
}

func newSupervisor(clock *fakeClock, starts *int, next func() *fakeProcess, signals <-chan os.Signal) superviseData {
	return superviseData{
		Start: func() (process, error) {
			*starts++
			return next(), nil
		},
		Signals: signals,
		Now:     clock.Now,
		After:   clock.After,
	}
}

func TestSupervise_GivesUpAfterFiveRestartsInTenMinutesAndSaysWhy(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 11, 14, 5, 0, 0, 0, time.UTC)}
	starts := 0
	var errOut bytes.Buffer

	got := runSupervisor(newSupervisor(clock, &starts, func() *fakeProcess { return newFakeProcess(true) }, nil), &errOut)

	if got != exitError || starts != 6 {
		t.Fatalf("code %d, starts %d (want 6: the first start and five restarts)", got, starts)
	}
	for _, want := range []string{"giving up", "5 restarts in 10m0s", "exit status 1"} {
		if !strings.Contains(errOut.String(), want) {
			t.Fatalf("stderr lacks %q: %q", want, errOut.String())
		}
	}
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}
	if !slices.Equal(clock.delays, want) {
		t.Fatalf("backoff %v, want %v", clock.delays, want)
	}
}

func TestSupervise_RestartsOlderThanTheWindowDoNotCount(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 11, 14, 5, 0, 0, 0, time.UTC)}
	starts := 0
	next := func() *fakeProcess {
		if starts <= 3 {
			return newFakeProcess(true)
		}
		clock.mu.Lock()
		clock.now = clock.now.Add(11 * time.Minute)
		clock.mu.Unlock()
		if starts <= 8 {
			return newFakeProcess(true)
		}
		return newFakeProcess(false)
	}
	signals := make(chan os.Signal, 1)
	var errOut bytes.Buffer
	go func() {
		for {
			clock.mu.Lock()
			n := len(clock.delays)
			clock.mu.Unlock()
			if n >= 8 {
				signals <- syscall.SIGTERM
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()

	got := runSupervisor(newSupervisor(clock, &starts, next, signals), &errOut)

	if got != exitOK {
		t.Fatalf("code %d, stderr %q: the supervisor gave up although old restarts fell out of the window", got, errOut.String())
	}
}

func TestSupervise_ForwardsTheSignalToTheChildAndExitsCleanly(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 11, 14, 5, 0, 0, 0, time.UTC)}
	starts := 0
	child := newFakeProcess(false)
	signals := make(chan os.Signal, 1)
	signals <- syscall.SIGTERM
	var errOut bytes.Buffer
	data := newSupervisor(clock, &starts, func() *fakeProcess { return child }, signals)
	data.After = func(time.Duration) <-chan time.Time { return make(chan time.Time) } // the grace never ends

	got := runSupervisor(data, &errOut)

	if got != exitOK || starts != 1 {
		t.Fatalf("code %d, starts %d, stderr %q", got, starts, errOut.String())
	}
	if sigs := child.got(); len(sigs) != 1 || sigs[0] != syscall.SIGTERM {
		t.Fatalf("child got %v, want one SIGTERM", sigs)
	}
}

func TestSupervise_ASignalDuringBackoffExitsWithoutAnotherStart(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 11, 14, 5, 0, 0, 0, time.UTC)}
	starts := 0
	signals := make(chan os.Signal, 1)
	data := newSupervisor(clock, &starts, func() *fakeProcess { return newFakeProcess(true) }, signals)
	data.After = func(time.Duration) <-chan time.Time {
		signals <- syscall.SIGINT
		return make(chan time.Time) // never fires
	}

	got := runSupervisor(data, &bytes.Buffer{})

	if got != exitOK || starts != 1 {
		t.Fatalf("code %d, starts %d", got, starts)
	}
}

func TestChildCommand_PassesTheFlagsAndDropsAgtermVariables(t *testing.T) {
	env := []string{"PATH=/bin", "AGTERM_SOCKET=/s", "AGTERM_SESSION_ID=x", "HOME=/h"}

	cmd := childCommand("/bin/autopilot", superviseFlags{Settings: "/w/.workspace/autopilot", AgtermSocket: "/a.sock"}, env)

	wantArgs := []string{"/bin/autopilot", "serve", "--settings", "/w/.workspace/autopilot", "--agterm-socket", "/a.sock"}
	if !slices.Equal(cmd.Args, wantArgs) {
		t.Fatalf("args %v, want %v", cmd.Args, wantArgs)
	}
	if !slices.Equal(cmd.Env, []string{"PATH=/bin", "HOME=/h"}) {
		t.Fatalf("env %v", cmd.Env)
	}
}

func TestSupervise_RefusesMissingFlags(t *testing.T) {
	t.Setenv("AGTERM_SOCKET", "")
	var errOut bytes.Buffer

	got := supervise([]string{"--settings", "x"}, &errOut)

	if got != exitUsage || !strings.Contains(errOut.String(), "usage") {
		t.Fatalf("code %d, err %q", got, errOut.String())
	}
}

func TestSupervise_KillsAChildThatIgnoresTheSignalAfterTheGrace(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 11, 14, 5, 0, 0, 0, time.UTC)}
	starts := 0
	child := newFakeProcess(false)
	child.stubborn = true
	signals := make(chan os.Signal, 1)
	signals <- syscall.SIGTERM

	got := runSupervisor(newSupervisor(clock, &starts, func() *fakeProcess { return child }, signals), &bytes.Buffer{})

	if got != exitOK {
		t.Fatalf("code %d", got)
	}
	if sigs := child.got(); len(sigs) != 2 || sigs[1] != syscall.SIGKILL {
		t.Fatalf("child got %v, want SIGTERM then SIGKILL", sigs)
	}
}

func TestSupervise_StopsAtOnceWhenServeIsAlreadyRunning(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 11, 14, 5, 0, 0, 0, time.UTC)}
	starts := 0
	var errOut bytes.Buffer
	next := func() *fakeProcess {
		p := &fakeProcess{done: make(chan error, 1)}
		p.done <- fakeExit(exitAlreadyRunning)
		return p
	}

	got := runSupervisor(newSupervisor(clock, &starts, next, nil), &errOut)

	if got != exitError || starts != 1 || len(clock.delays) != 0 {
		t.Fatalf("code %d, starts %d, delays %v: want one start and no restart", got, starts, clock.delays)
	}
	if !strings.Contains(errOut.String(), "already running") {
		t.Fatalf("stderr lacks the reason: %q", errOut.String())
	}
}

func TestSupervise_ForwardsSIGHUPLikeTheOtherStopSignals(t *testing.T) {
	if !slices.Contains(superviseSignals, os.Signal(syscall.SIGHUP)) {
		t.Fatalf("supervisor listens to %v, not SIGHUP: a closed terminal would orphan serve", superviseSignals)
	}
	clock := &fakeClock{now: time.Date(2026, 11, 14, 5, 0, 0, 0, time.UTC)}
	starts := 0
	child := newFakeProcess(false)
	signals := make(chan os.Signal, 1)
	signals <- syscall.SIGHUP
	data := newSupervisor(clock, &starts, func() *fakeProcess { return child }, signals)
	data.After = func(time.Duration) <-chan time.Time { return make(chan time.Time) }

	got := runSupervisor(data, &bytes.Buffer{})

	if sigs := child.got(); got != exitOK || len(sigs) != 1 || sigs[0] != syscall.SIGHUP {
		t.Fatalf("code %d, child got %v, want one SIGHUP", got, sigs)
	}
}
