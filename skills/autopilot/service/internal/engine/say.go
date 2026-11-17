package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxMessageRunes = 500
	messagesLog     = "messages.log"
)

// Say sends a one-line message to a session and appends it to run/messages.log, so one file holds the talk of a run.
// to is a session id; the message goes to its main pane unless to names a pane as SESSION:PANE.
func (r *Runner) Say(ctx context.Context, base, from, to, message string) error {
	if from == "" || to == "" || strings.TrimSpace(message) == "" {
		return fmt.Errorf("say needs a sender, a receiver and a message")
	}
	line := strings.NewReplacer("\r", " ", "\n", " ").Replace(message)
	if runes := []rune(line); len(runes) > maxMessageRunes {
		line = string(runes[:maxMessageRunes])
	}
	ts := r.d.Now().UTC().Format("15:04:05")
	if err := os.MkdirAll(RunFolder(base), 0o755); err != nil {
		return fmt.Errorf("cannot make the run folder: %w", err)
	}
	logPath := filepath.Join(RunFolder(base), messagesLog)
	if err := appendLine(logPath, fmt.Sprintf("%s %s -> %s: %s", ts, from, to, line)); err != nil {
		return err
	}
	target := to
	if !strings.Contains(to, ":") {
		target = fmt.Sprintf("%s:%s", to, mainPane)
	}
	if err := r.d.Sessions.Send(ctx, target, fmt.Sprintf("%s: %s", from, line)); err != nil {
		if logErr := appendLine(logPath, fmt.Sprintf("%s   (delivery failed)", ts)); logErr != nil {
			return logErr
		}
		return fmt.Errorf("cannot send to %s: %w", to, err)
	}
	return nil
}

func appendLine(path, line string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("cannot open %s: %w", path, err)
	}
	if _, err := fmt.Fprintln(f, line); err != nil {
		_ = f.Close()
		return fmt.Errorf("cannot write %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("cannot close %s: %w", path, err)
	}
	return nil
}
