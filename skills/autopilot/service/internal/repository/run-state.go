package repository

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sadensmol/autopilot/internal/domain"
)

// RunStateFile is the path of a run's state file, relative to the run's worktree base.
var RunStateFile = filepath.Join("run", "state.json")

// ReadRunState reads the state file of the run at worktreeBase. A missing file is no error: found is false.
func ReadRunState(worktreeBase string) (state domain.RunState, found bool, err error) {
	path := filepath.Join(worktreeBase, RunStateFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return domain.RunState{}, false, nil
	}
	if err != nil {
		return domain.RunState{}, false, fmt.Errorf("cannot read run state %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return domain.RunState{}, false, fmt.Errorf("cannot parse run state %s: %w", path, err)
	}
	return state, true, nil
}

// WriteRunState writes the state file of the run at worktreeBase; a reader never sees a half-written file.
func WriteRunState(worktreeBase string, state domain.RunState) error {
	path := filepath.Join(worktreeBase, RunStateFile)
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("cannot encode run state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("cannot make the folder of %s: %w", path, err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("cannot write run state %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("cannot replace run state %s: %w", path, err)
	}
	return nil
}
