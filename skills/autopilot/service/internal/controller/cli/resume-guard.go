package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/sadensmol/autopilot/internal/controller/socket"
	"github.com/sadensmol/autopilot/internal/domain"
)

// ownerVerbs are the verbs only the owner runs, from the control room; a run's own agents work in a run worktree.
var ownerVerbs = []string{socket.VerbResume, socket.VerbCancel, socket.VerbFix}

// refuseInRunWorktree stops an owner verb in the worktree of a run. It is a guard against a mistake, not against a
// caller outside the worktree; the service also bounds the resumes of one run.
func refuseInRunWorktree(env Env, verb string) error {
	cwd, err := env.Getwd()
	if err != nil {
		return fmt.Errorf("cannot read the working folder: %w", err)
	}
	for _, part := range strings.Split(filepath.ToSlash(cwd), "/") {
		if part == domain.RunsFolder {
			return fmt.Errorf("refused in a run worktree (%s): only the owner runs %s, from the control room", domain.RunsFolder, verb)
		}
	}
	return nil
}
