package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sadensmol/autopilot/internal/domain"
	"github.com/sadensmol/autopilot/internal/repository"
)

const (
	engineStateFile = "engine.json"
	runFolderName   = "run"
)

// part and wave statuses.
const (
	StatusPending = "pending"
	StatusRunning = "running"
	// StatusExited means the part settled: its session completed, or its result file appeared.
	StatusExited   = "exited"
	StatusBlocked  = "blocked"
	StatusTimeout  = "timeout"
	StatusClosed   = "closed"
	StatusFailed   = "failed"
	StatusComplete = "complete"
)

// State is what the engine knows about the waves of one run. It lives in <base>/run/engine.json.
// The smaller run/state.json that the service reads is written from it.
type State struct {
	Key  string `json:"key"`
	Task string `json:"task"`
	Cwd  string `json:"cwd"`
	// Parent is the run session; every part session is its companion.
	Parent string      `json:"parent"`
	Waves  []WaveState `json:"waves"`
}

type WaveState struct {
	Status string      `json:"status"`
	Parts  []PartState `json:"parts"`
}

type PartState struct {
	Part
	SessionID  string     `json:"session_id,omitempty"`
	Status     string     `json:"status"`
	ResultFile string     `json:"result_file"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// RunFolder is the folder of a run's engine files, inside the run's worktree base.
func RunFolder(base string) string {
	return filepath.Join(base, runFolderName)
}

// Init validates the plan and writes the engine state and run/state.json of a new run.
func Init(base string, p Plan, key, parent string) (State, error) {
	qaFiles, err := QATestsFor(base, planParts(p))
	if err != nil {
		return State{}, err
	}
	if problems := ValidatePlanQA(p, qaFiles); len(problems) > 0 {
		return State{}, fmt.Errorf("plan problems:\n  - %s", strings.Join(problems, "\n  - "))
	}
	dir := RunFolder(base)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return State{}, fmt.Errorf("cannot make the run folder: %w", err)
	}
	st := State{Key: key, Task: p.Task, Cwd: normalize(p.Cwd), Parent: parent}
	for wi, w := range p.Waves {
		ws := WaveState{Status: StatusPending}
		if wi == 0 {
			ws.Status = StatusRunning
		}
		for _, part := range w.Parts {
			ws.Parts = append(ws.Parts, PartState{Part: part, Status: StatusPending, ResultFile: filepath.Join(dir, part.ID+"-result.md")})
		}
		st.Waves = append(st.Waves, ws)
	}
	if err := SaveState(base, st); err != nil {
		return State{}, err
	}
	if err := writeRunState(base, st, 1); err != nil {
		return State{}, err
	}
	return st, nil
}

func LoadState(base string) (State, error) {
	path := filepath.Join(RunFolder(base), engineStateFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return State{}, fmt.Errorf("no engine state at %s: run plan init first: %w", path, domain.ErrNotFound)
	}
	if err != nil {
		return State{}, fmt.Errorf("cannot read engine state %s: %w", path, err)
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return State{}, fmt.Errorf("cannot parse engine state %s: %w", path, err)
	}
	return st, nil
}

func SaveState(base string, st State) error {
	path := filepath.Join(RunFolder(base), engineStateFile)
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot encode engine state: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("cannot write engine state %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("cannot replace engine state %s: %w", path, err)
	}
	return nil
}

// writeRunState writes the domain.RunState of a wave (numbered from 1) to run/state.json.
func writeRunState(base string, st State, wave int) error {
	ws := st.Waves[wave-1]
	running := []string{}
	for _, p := range ws.Parts {
		if p.Status == StatusRunning {
			running = append(running, p.ID)
		}
	}
	return repository.WriteRunState(base, domain.RunState{Wave: wave, Parts: len(ws.Parts), Running: running})
}

// store keeps the state of one command safe for the goroutines that update parts.
type store struct {
	mu   sync.Mutex
	base string
	st   State
	wave int
}

func (s *store) update(partIndex int, change func(*PartState)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	change(&s.st.Waves[s.wave-1].Parts[partIndex])
	return SaveState(s.base, s.st)
}

// finish saves the wave status and run/state.json.
func (s *store) finish(status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.st.Waves[s.wave-1].Status = status
	if err := SaveState(s.base, s.st); err != nil {
		return err
	}
	return writeRunState(s.base, s.st, s.wave)
}
