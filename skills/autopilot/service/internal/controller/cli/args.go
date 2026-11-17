package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/sadensmol/autopilot/internal/controller/socket"
)

type verbSpec struct {
	usage      string
	valueFlags []string
	boolFlags  []string
	// build turns positional arguments and flag values into a request
	build func(pos []string, flags map[string][]string) (socket.Request, error)
}

var errArgs = errors.New("wrong arguments")

func exactly(n int) func(pos []string) error {
	return func(pos []string) error {
		if len(pos) != n {
			return errArgs
		}
		return nil
	}
}

func required(flags map[string][]string, names ...string) error {
	for _, n := range names {
		if len(flags[n]) != 1 || flags[n][0] == "" {
			return errArgs
		}
	}
	return nil
}

var specs = map[string]verbSpec{
	socket.VerbPing: {
		usage: "autopilot ping",
		build: func(pos []string, _ map[string][]string) (socket.Request, error) {
			return socket.Request{Verb: socket.VerbPing}, exactly(0)(pos)
		},
	},
	socket.VerbRuns: {
		usage: "autopilot runs",
		build: func(pos []string, _ map[string][]string) (socket.Request, error) {
			return socket.Request{Verb: socket.VerbRuns}, exactly(0)(pos)
		},
	},
	socket.VerbPost: {
		usage: `autopilot post <KEY> "<note>"`,
		build: func(pos []string, _ map[string][]string) (socket.Request, error) {
			if err := exactly(2)(pos); err != nil {
				return socket.Request{}, err
			}
			return socket.Request{Verb: socket.VerbPost, Key: pos[0], Text: pos[1]}, nil
		},
	},
	socket.VerbStatus: {
		usage: "autopilot status <KEY> <phase>",
		build: func(pos []string, _ map[string][]string) (socket.Request, error) {
			if err := exactly(2)(pos); err != nil {
				return socket.Request{}, err
			}
			return socket.Request{Verb: socket.VerbStatus, Key: pos[0], Phase: pos[1]}, nil
		},
	},
	socket.VerbAsk: {
		usage:      `autopilot ask <KEY> "<question>" [--option id=label ...] | autopilot ask --wait <id>`,
		valueFlags: []string{"option", "wait"},
		build:      buildAsk,
	},
	socket.VerbRed:     repoSHASpec(socket.VerbRed),
	socket.VerbGate:    repoSHASpec(socket.VerbGate),
	socket.VerbComment: fileSpec(socket.VerbComment),
	socket.VerbManual:  fileSpec(socket.VerbManual),
	socket.VerbPR: {
		usage: "autopilot pr <KEY> <url>",
		build: func(pos []string, _ map[string][]string) (socket.Request, error) {
			if err := exactly(2)(pos); err != nil {
				return socket.Request{}, err
			}
			return socket.Request{Verb: socket.VerbPR, Key: pos[0], URL: pos[1]}, nil
		},
	},
	socket.VerbFix: {
		usage: "autopilot merge-fix <KEY> <url>",
		build: func(pos []string, _ map[string][]string) (socket.Request, error) {
			if err := exactly(2)(pos); err != nil {
				return socket.Request{}, err
			}
			return socket.Request{Verb: socket.VerbFix, Key: pos[0], URL: pos[1]}, nil
		},
	},
	socket.VerbCancel: {
		usage: "autopilot cancel <KEY>",
		build: func(pos []string, _ map[string][]string) (socket.Request, error) {
			if err := exactly(1)(pos); err != nil {
				return socket.Request{}, err
			}
			return socket.Request{Verb: socket.VerbCancel, Key: pos[0]}, nil
		},
	},
	socket.VerbResume: {
		usage: "autopilot resume <KEY>",
		build: func(pos []string, _ map[string][]string) (socket.Request, error) {
			if err := exactly(1)(pos); err != nil {
				return socket.Request{}, err
			}
			return socket.Request{Verb: socket.VerbResume, Key: pos[0]}, nil
		},
	},
}

func repoSHASpec(verb string) verbSpec {
	return verbSpec{
		usage:      fmt.Sprintf("autopilot %s <KEY> --repo <name> --sha <sha>", verb),
		valueFlags: []string{"repo", "sha"},
		build: func(pos []string, flags map[string][]string) (socket.Request, error) {
			if err := exactly(1)(pos); err != nil {
				return socket.Request{}, err
			}
			if err := required(flags, "repo", "sha"); err != nil {
				return socket.Request{}, err
			}
			return socket.Request{Verb: verb, Key: pos[0], Repo: flags["repo"][0], SHA: flags["sha"][0]}, nil
		},
	}
}

// fileSpec reads the markdown file in the CLI, so the service needs no access to the caller's folders.
func fileSpec(verb string) verbSpec {
	return verbSpec{
		usage:      fmt.Sprintf("autopilot %s <KEY> --file <md>", verb),
		valueFlags: []string{"file"},
		build: func(pos []string, flags map[string][]string) (socket.Request, error) {
			if err := exactly(1)(pos); err != nil {
				return socket.Request{}, err
			}
			if err := required(flags, "file"); err != nil {
				return socket.Request{}, err
			}
			body, err := readBody(flags["file"][0])
			if err != nil {
				return socket.Request{}, err
			}
			return socket.Request{Verb: verb, Key: pos[0], Body: string(body)}, nil
		},
	}
}

func buildAsk(pos []string, flags map[string][]string) (socket.Request, error) {
	if wait, ok := flags["wait"]; ok {
		if len(wait) != 1 || len(pos) != 0 || len(flags["option"]) != 0 {
			return socket.Request{}, errArgs
		}
		id, err := strconv.ParseInt(wait[0], 10, 64)
		if err != nil || id <= 0 {
			return socket.Request{}, fmt.Errorf("--wait needs a positive ask id: %w", errArgs)
		}
		return socket.Request{Verb: socket.VerbAskWait, ID: id}, nil
	}
	if err := exactly(2)(pos); err != nil {
		return socket.Request{}, err
	}
	var options []socket.Option
	for _, raw := range flags["option"] {
		id, label, ok := strings.Cut(raw, "=")
		if !ok || id == "" || label == "" {
			return socket.Request{}, fmt.Errorf("--option needs id=label, got %q: %w", raw, errArgs)
		}
		options = append(options, socket.Option{ID: id, Label: label})
	}
	return socket.Request{Verb: socket.VerbAsk, Key: pos[0], Text: pos[1], Options: options}, nil
}

// parse splits args into positionals and flags: `--name value` or `--name=value`, in any position.
func parse(spec verbSpec, args []string) ([]string, map[string][]string, error) {
	isValue := func(n string) bool { return contains(spec.valueFlags, n) }
	var pos []string
	flags := map[string][]string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") || arg == "--" {
			pos = append(pos, arg)
			continue
		}
		name, value, hasValue := strings.Cut(arg[2:], "=")
		if !isValue(name) {
			return nil, nil, fmt.Errorf("unknown flag --%s: %w", name, errArgs)
		}
		if !hasValue {
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("--%s needs a value: %w", name, errArgs)
			}
			i++
			value = args[i]
		}
		flags[name] = append(flags[name], value)
	}
	return pos, flags, nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// readBody reads a regular file of at most socket.MaxBodyBytes; a FIFO or device could never end.
func readBody(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read --file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("--file %s is not a regular file", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read --file: %w", err)
	}
	defer f.Close() //nolint:errcheck // a read-only file
	body, err := io.ReadAll(io.LimitReader(f, socket.MaxBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("cannot read --file: %w", err)
	}
	if len(body) > socket.MaxBodyBytes {
		return nil, fmt.Errorf("--file %s is larger than %d bytes", path, socket.MaxBodyBytes)
	}
	return body, nil
}
