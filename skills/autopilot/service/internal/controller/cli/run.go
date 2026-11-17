package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/sadensmol/autopilot/internal/controller/socket"
)

// exit codes of the verbs.
const (
	ExitOK          = 0
	ExitError       = 1
	ExitUsage       = 2
	ExitServiceDown = 3
	ExitPending     = 75
)

type Env struct {
	Getenv func(string) string
	Getwd  func() (string, error)
	// AgtermBin is the agtermctl program of the engine commands; empty means agtermctl from PATH.
	AgtermBin string
	// PollInterval is the pause between polls of a new session in the engine commands; zero means one second.
	PollInterval time.Duration
}

// IsVerb reports whether name is a verb this CLI sends to the service.
func IsVerb(name string) bool {
	_, ok := specs[name]
	return ok
}

// Run sends one verb to the service and returns the exit code.
func Run(args []string, env Env, stdout, stderr io.Writer) int {
	if len(args) == 0 || !IsVerb(args[0]) {
		fmt.Fprintln(stderr, "usage: autopilot ping | post | status | ask | red | gate | comment | pr | manual | runs | cancel | resume | merge-fix")
		return ExitUsage
	}
	spec := specs[args[0]]
	req, err := buildRequest(spec, args[1:])
	if err != nil {
		fmt.Fprintf(stderr, "autopilot %s: %v\nusage: %s\n", args[0], err, spec.usage)
		return ExitUsage
	}
	if slices.Contains(ownerVerbs, args[0]) {
		if err := refuseInRunWorktree(env, args[0]); err != nil {
			fmt.Fprintf(stderr, "autopilot %s: %v\n", args[0], err)
			return ExitError
		}
	}
	path, err := SocketPath(env)
	if err != nil {
		fmt.Fprintf(stderr, "autopilot %s: %v\n", args[0], err)
		return ExitError
	}
	resp, err := socket.Call(context.Background(), path, req)
	if errors.Is(err, socket.ErrServiceDown) {
		fmt.Fprintf(stderr, "autopilot %s: %v\n", args[0], err)
		return ExitServiceDown
	}
	if err != nil {
		fmt.Fprintf(stderr, "autopilot %s: %v\n", args[0], err)
		return ExitError
	}
	if resp.Output != "" {
		fmt.Fprintln(stdout, trimNewline(resp.Output))
	}
	if resp.Error != "" {
		fmt.Fprintf(stderr, "autopilot %s: %s\n", args[0], resp.Error)
	}
	switch resp.Code {
	case socket.CodeOK:
		return ExitOK
	case socket.CodePending:
		return ExitPending
	default:
		return ExitError
	}
}

func buildRequest(spec verbSpec, args []string) (socket.Request, error) {
	pos, flags, err := parse(spec, args)
	if err != nil {
		return socket.Request{}, err
	}
	return spec.build(pos, flags)
}

func trimNewline(s string) string {
	for len(s) > 0 && s[len(s)-1] == '\n' {
		s = s[:len(s)-1]
	}
	return s
}
