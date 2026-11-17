// Package shell runs the commands of the workspace registry, such as a deploy check.
package shell

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

type Runner struct{}

func New() *Runner {
	return &Runner{}
}

// Run runs command with sh -c in its own process group, so the end of ctx stops the command and every child it
// started. Any exit but 0 is an error. The output is dropped: a registry command may print what must not be logged.
func (r *Runner) Run(ctx context.Context, command string) error {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	// a child that outlives the kill must not hold the process open
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command failed: %w", err)
	}
	return nil
}
