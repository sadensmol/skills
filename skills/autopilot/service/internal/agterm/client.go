// Package agterm drives the agterm terminal through its agtermctl command.
package agterm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ErrNoSession means the session is not in the agterm tree: it was closed, or its command exited.
var ErrNoSession = errors.New("not in the agterm tree")

// Client calls agtermctl with an explicit socket and a clean environment.
type Client struct {
	bin    string
	socket string
}

func New(bin, socket string) *Client {
	return &Client{bin: bin, socket: socket}
}

type NewSessionData struct {
	// Parent makes the new session a companion of this session; the companion sits in the parent's workspace.
	Parent string
	// After places the new session right after this session, in that session's workspace; it is not used when Parent is set.
	After   string
	Cwd     string
	Name    string
	Command string
}

// NewSession creates a session in the background and returns its id. The command runs in an interactive zsh login shell:
// agterm starts it with the PATH of the app, which lacks the folders that the zsh profile and rc files add.
func (c *Client) NewSession(ctx context.Context, d NewSessionData) (string, error) {
	args := []string{"session", "new", "--socket", c.socket, "--json"}
	if d.Parent != "" {
		args = append(args, "--parent", d.Parent)
	} else {
		args = append(args, "--after", d.After)
	}
	args = append(args, "--no-select", "--cwd", d.Cwd, "--name", d.Name, "--command", fmt.Sprintf("zsh -lic %s", shellQuote(d.Command)))
	out, err := c.run(ctx, args...)
	if err != nil {
		return "", err
	}
	result, err := unwrap(out)
	if err != nil {
		return "", fmt.Errorf("agtermctl session new: %w", err)
	}
	var reply struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(result, &reply); err != nil {
		return "", fmt.Errorf("cannot read the new session id: %w", err)
	}
	if reply.ID == "" {
		return "", fmt.Errorf("agtermctl session new returned no session id")
	}
	return reply.ID, nil
}

// SessionByName returns the id of the session with this name in the workspace with this name.
func (c *Client) SessionByName(ctx context.Context, workspace, name string) (string, error) {
	out, err := c.run(ctx, "tree", "--socket", c.socket, "--json")
	if err != nil {
		return "", err
	}
	result, err := unwrap(out)
	if err != nil {
		return "", fmt.Errorf("agtermctl tree: %w", err)
	}
	var tree any
	if err := json.Unmarshal(result, &tree); err != nil {
		return "", fmt.Errorf("cannot read the agterm tree: %w", err)
	}
	sessions, found := findWorkspaceSessions(tree, workspace)
	if !found {
		return "", fmt.Errorf("workspace %s is not in the agterm tree", workspace)
	}
	for _, node := range sessions {
		session, _ := node.(map[string]any)
		id, _ := session["id"].(string)
		if session["name"] == name && id != "" {
			return id, nil
		}
	}
	return "", fmt.Errorf("workspace %s has no session named %s", workspace, name)
}

// MoveToTop moves the session to the top of its workspace.
func (c *Client) MoveToTop(ctx context.Context, sessionID string) error {
	_, err := c.run(ctx, "session", "move", "--socket", c.socket, "--to", "top", "--target", sessionID)
	return err
}

// shellQuote quotes s as one word for a POSIX shell.
func shellQuote(s string) string {
	return fmt.Sprintf("'%s'", strings.ReplaceAll(s, "'", `'\''`))
}

// SetContext sets what the session is about.
func (c *Client) SetContext(ctx context.Context, sessionID, text string) error {
	_, err := c.run(ctx, "session", "context", "--socket", c.socket, "--target", sessionID, text)
	return err
}

// SetInfo shows one activity line under the session's sidebar row; agterm drops it after hideAfter unless it is set again.
func (c *Client) SetInfo(ctx context.Context, sessionID, text string, hideAfter time.Duration) error {
	seconds := max(1, int(math.Ceil(hideAfter.Seconds())))
	_, err := c.run(ctx, "session", "info", "--socket", c.socket, "--target", sessionID, "--hide-after", strconv.Itoa(seconds), text)
	return err
}

// Realized reports whether the session's main pane has a live terminal.
func (c *Client) Realized(ctx context.Context, sessionID string) (bool, error) {
	state, err := c.State(ctx, sessionID)
	return state.Realized, err
}

// SessionState is what the tree says about one session.
type SessionState struct {
	Realized bool
	// Status is the agent status of the session, such as active, completed or blocked; empty when none is set.
	Status string
}

// State reads the realized flag and the agent status of a session from the tree.
func (c *Client) State(ctx context.Context, sessionID string) (SessionState, error) {
	out, err := c.run(ctx, "tree", "--socket", c.socket, "--json")
	if err != nil {
		return SessionState{}, err
	}
	result, err := unwrap(out)
	if err != nil {
		return SessionState{}, fmt.Errorf("agtermctl tree: %w", err)
	}
	var tree any
	if err := json.Unmarshal(result, &tree); err != nil {
		return SessionState{}, fmt.Errorf("cannot read the agterm tree: %w", err)
	}
	node, found := findSession(tree, sessionID)
	if !found {
		return SessionState{}, fmt.Errorf("session %s: %w", sessionID, ErrNoSession)
	}
	realized, _ := node["realized"].(bool)
	status, _ := node["status"].(string)
	return SessionState{Realized: realized, Status: status}, nil
}

// Text returns the visible text of the session's terminal.
func (c *Client) Text(ctx context.Context, sessionID string) (string, error) {
	return c.run(ctx, "session", "text", "--socket", c.socket, "--target", sessionID)
}

// TextTail returns the last lines of the session's terminal buffer.
func (c *Client) TextTail(ctx context.Context, sessionID string, lines int) (string, error) {
	return c.run(ctx, "session", "text", "--socket", c.socket, "--target", sessionID, "--lines", strconv.Itoa(lines))
}

// WaitResult is how a status wait ended. Exactly one field is set.
type WaitResult struct {
	// Status is the matched status, for example completed or blocked.
	Status string
	// TimedOut is set when the timeout passed first.
	TimedOut bool
	// Left is set when the session left the open tree.
	Left bool
}

const (
	exitLeftTree = 2
	exitTimedOut = 124
)

// Wait blocks until the session's agent status is completed or blocked, or the timeout passes.
// A status set before the call still matches, so a reused session may answer at once.
func (c *Client) Wait(ctx context.Context, sessionID string, timeout time.Duration) (WaitResult, error) {
	seconds := max(1, int(math.Ceil(timeout.Seconds())))
	out, err := c.run(ctx, "session", "wait", "--socket", c.socket, "--target", sessionID,
		"--until", "completed", "--until", "blocked", "--timeout", strconv.Itoa(seconds))
	if err == nil {
		return WaitResult{Status: strings.TrimSpace(out)}, nil
	}
	// a session that is gone before the wait starts is an error of agtermctl, not exit 2
	if strings.Contains(err.Error(), "no such session") {
		return WaitResult{Left: true}, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		switch exit.ExitCode() {
		case exitTimedOut:
			return WaitResult{TimedOut: true}, nil
		case exitLeftTree:
			return WaitResult{Left: true}, nil
		}
	}
	return WaitResult{}, err
}

// Send sends a message to the pane of a session: to is SESSION:PANE.
func (c *Client) Send(ctx context.Context, to, text string) error {
	_, err := c.run(ctx, "msg", "send", "--socket", c.socket, "--to", to, text)
	return err
}

// Type injects text into the session exactly as given.
func (c *Client) Type(ctx context.Context, sessionID, text string) error {
	_, err := c.run(ctx, "session", "type", "--socket", c.socket, "--target", sessionID, text)
	return err
}

// Close closes the session; a session that is already gone counts as closed.
func (c *Client) Close(ctx context.Context, sessionID string) error {
	_, err := c.run(ctx, "session", "close", "--socket", c.socket, "--target", sessionID)
	if err != nil && strings.Contains(err.Error(), "no such session") {
		return nil
	}
	return err
}

type AskOpenData struct {
	// Target is the session the dialog shows in.
	Target  string
	Title   string
	Message string
	// Choices make a radio list with a free-text row; no choices make a text field.
	Choices []AskChoice
}

type AskChoice struct {
	ID    string
	Label string
}

// OpenAsk opens a terminal dialog in the session without waiting and returns the dialog id.
func (c *Client) OpenAsk(ctx context.Context, d AskOpenData) (string, error) {
	args := []string{"ask", "open", "--socket", c.socket, "--json", "--no-block", "--style", "terminal", "--target", d.Target, "--message", d.Message}
	if len(d.Choices) == 0 {
		args = append(args, "--input", "line")
	}
	for _, choice := range d.Choices {
		args = append(args, "--choice", fmt.Sprintf("%s=%s", choice.ID, choice.Label))
	}
	if len(d.Choices) > 0 {
		args = append(args, "--other", "Other answer")
	}
	args = append(args, d.Title)
	out, runErr := c.run(ctx, args...)
	result, err := unwrap(out)
	switch {
	case runErr != nil && strings.TrimSpace(out) == "":
		return "", runErr
	case err != nil:
		// a refusal such as "ask already pending" is on stdout, not stderr
		return "", fmt.Errorf("agtermctl ask open: %w", err)
	case runErr != nil:
		return "", runErr
	}
	var reply struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(result, &reply); err != nil {
		return "", fmt.Errorf("cannot read the new dialog id: %w", err)
	}
	if reply.ID == "" {
		return "", errors.New("agtermctl ask open returned no dialog id")
	}
	return reply.ID, nil
}

// AskState is how far a dialog is.
type AskState string

const (
	AskPending   AskState = "pending"
	AskAnswered  AskState = "answered"
	AskDismissed AskState = "dismissed"
)

// AskResult is the current result of a dialog. Answer is set when State is AskAnswered.
type AskResult struct {
	State  AskState
	Answer string
}

// AskResult reads the current result of a dialog. A dialog that is still open is not an error.
func (c *Client) AskResult(ctx context.Context, dialogID string) (AskResult, error) {
	out, runErr := c.run(ctx, "ask", "result", dialogID, "--socket", c.socket, "--json")
	result, err := unwrap(out)
	if err != nil {
		if runErr != nil {
			return AskResult{}, runErr
		}
		return AskResult{}, fmt.Errorf("agtermctl ask result: %w", err)
	}
	var reply struct {
		Result string   `json:"result"`
		ID     string   `json:"id"`
		IDs    []string `json:"ids"`
		Text   string   `json:"text"`
	}
	if err := json.Unmarshal(result, &reply); err != nil {
		return AskResult{}, fmt.Errorf("cannot read the dialog result: %w", err)
	}
	switch reply.Result {
	case "pending":
		return AskResult{State: AskPending}, nil
	case "answered":
		return AskResult{State: AskAnswered, Answer: firstSet(reply.Text, firstOf(reply.IDs), reply.ID)}, nil
	case "escaped", "cancelled":
		return AskResult{State: AskDismissed}, nil
	}
	return AskResult{}, fmt.Errorf("agtermctl ask result: unknown result %q", reply.Result)
}

// CancelAsk closes a dialog without an answer; a dialog that is already finished is not an error.
func (c *Client) CancelAsk(ctx context.Context, dialogID string) error {
	_, err := c.run(ctx, "ask", "cancel", dialogID, "--socket", c.socket)
	return err
}

// SetStatus sets the agent status indicator of the session.
func (c *Client) SetStatus(ctx context.Context, sessionID, state string, blink bool) error {
	args := []string{"session", "status", state, "--socket", c.socket, "--target", sessionID}
	if blink {
		args = append(args, "--blink")
	}
	_, err := c.run(ctx, args...)
	return err
}

func firstOf(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func firstSet(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func (c *Client) run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, c.bin, args...)
	cmd.Env = cleanEnv(os.Environ())
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// stdout comes back with the error: a pending ask result exits 1 and still says so on stdout
		return stdout.String(), fmt.Errorf("agtermctl %s: %s: %w", strings.Join(args[:min(2, len(args))], " "), strings.TrimSpace(stderr.String()), err)
	}
	return stdout.String(), nil
}

// unwrap reads the reply envelope {"ok":..,"result":{..}} and returns the result; ok false is an error.
// A reply without "ok" is the result itself: the ask verbs answer {"id":..} and {"result":..} bare.
func unwrap(out string) (json.RawMessage, error) {
	var env struct {
		OK     *bool           `json:"ok"`
		Error  string          `json:"error"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		return nil, fmt.Errorf("cannot read the reply: %w", err)
	}
	if env.OK == nil {
		return json.RawMessage(out), nil
	}
	if !*env.OK {
		return nil, fmt.Errorf("agterm refused: %s", env.Error)
	}
	if len(env.Result) == 0 {
		return nil, errors.New("the reply has no result")
	}
	return env.Result, nil
}

// cleanEnv drops every AGTERM_* variable, so a call never reaches the wrong instance.
func cleanEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if !strings.HasPrefix(kv, "AGTERM_") {
			out = append(out, kv)
		}
	}
	return out
}

// findWorkspaceSessions walks the tree for the workspace node with this name and returns its sessions.
func findWorkspaceSessions(node any, name string) ([]any, bool) {
	switch n := node.(type) {
	case map[string]any:
		if sessions, ok := n["sessions"].([]any); ok && n["name"] == name {
			return sessions, true
		}
		for _, child := range n {
			if found, ok := findWorkspaceSessions(child, name); ok {
				return found, true
			}
		}
	case []any:
		for _, child := range n {
			if found, ok := findWorkspaceSessions(child, name); ok {
				return found, true
			}
		}
	}
	return nil, false
}

// findSession walks the tree for the node with this id that carries a realized flag.
func findSession(node any, id string) (map[string]any, bool) {
	switch n := node.(type) {
	case map[string]any:
		if _, ok := n["realized"].(bool); ok && n["id"] == id {
			return n, true
		}
		for _, child := range n {
			if found, ok := findSession(child, id); ok {
				return found, true
			}
		}
	case []any:
		for _, child := range n {
			if found, ok := findSession(child, id); ok {
				return found, true
			}
		}
	}
	return nil, false
}
