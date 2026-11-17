package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/sadensmol/autopilot/internal/agterm"
	"github.com/sadensmol/autopilot/internal/domain"
)

const (
	maxSessionLabel = 40
	readLines       = 80
	// say sends to the main pane of a session.
	mainPane = "left"
)

var requiredSections = []string{"Summary", "Files Modified"}

// ISessions is the part of the agterm client that the engine uses.
type ISessions interface {
	NewSession(ctx context.Context, d agterm.NewSessionData) (string, error)
	Realized(ctx context.Context, sessionID string) (bool, error)
	Text(ctx context.Context, sessionID string) (string, error)
	TextTail(ctx context.Context, sessionID string, lines int) (string, error)
	Type(ctx context.Context, sessionID, text string) error
	Close(ctx context.Context, sessionID string) error
	Wait(ctx context.Context, sessionID string, timeout time.Duration) (agterm.WaitResult, error)
	Send(ctx context.Context, to, text string) error
}

type RunnerData struct {
	Sessions ISessions
	Now      func() time.Time
	// PollInterval is the pause between two polls of a new session.
	PollInterval time.Duration
	// WaitSlice is how long one `session wait` call blocks; the result file and the wave limit are checked between calls.
	WaitSlice time.Duration
	// RealizedAttempts bounds the wait for a new session to get a terminal.
	RealizedAttempts int
	// DialogAttempts bounds the watch for the start dialogs after the session has a terminal.
	DialogAttempts int
}

// Runner runs the wave commands of one run.
type Runner struct {
	d RunnerData
}

func NewRunner(d RunnerData) *Runner {
	return &Runner{d: d}
}

// Report is what a wave command tells the caller: lines to show and whether everything went well.
type Report struct {
	Lines []string
	OK    bool
}

func (r *Report) add(format string, args ...any) {
	r.Lines = append(r.Lines, fmt.Sprintf(format, args...))
}

func loadWave(base string, wave int) (*store, error) {
	st, err := LoadState(base)
	if err != nil {
		return nil, err
	}
	if wave < 1 || wave > len(st.Waves) {
		return nil, fmt.Errorf("wave %d does not exist: the plan has %d waves", wave, len(st.Waves))
	}
	return &store{base: base, st: st, wave: wave}, nil
}

func (s *store) parts() []PartState {
	return s.st.Waves[s.wave-1].Parts
}

func snapshotFile(base string, wave int) string {
	return filepath.Join(RunFolder(base), fmt.Sprintf("tree-before-wave-%d.json", wave))
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Spawn starts one companion session per part of the wave (waves count from 1), or only part partID.
// A part that already has a session gets a new one and the old one is closed: a completed status on the old
// session would end a later wait at once.
func (r *Runner) Spawn(ctx context.Context, base string, wave int, partID string) (Report, error) {
	s, err := loadWave(base, wave)
	if err != nil {
		return Report{}, err
	}
	if s.st.Cwd == "" {
		return Report{}, errors.New("the plan has no cwd: zones need a git folder")
	}
	indexes, err := partIndexes(s.parts(), partID)
	if err != nil {
		return Report{}, err
	}
	if partID == "" {
		snap, err := Snapshot(ctx, s.st.Cwd)
		if err != nil {
			return Report{}, err
		}
		data, err := json.Marshal(snap)
		if err != nil {
			return Report{}, fmt.Errorf("cannot encode the tree snapshot: %w", err)
		}
		if err := os.WriteFile(snapshotFile(base, wave), data, 0o644); err != nil {
			return Report{}, fmt.Errorf("cannot write the tree snapshot: %w", err)
		}
	}
	report := Report{OK: true}
	var started []int
	for _, i := range indexes {
		if err := r.spawnPart(ctx, s, i, &report); err != nil {
			return report, err
		}
		if s.parts()[i].Status == StatusRunning {
			started = append(started, i)
		}
	}
	r.watchDialogs(ctx, s, started, &report)
	if err := s.finish(StatusRunning); err != nil {
		return report, err
	}
	return report, nil
}

func partIndexes(parts []PartState, partID string) ([]int, error) {
	var out []int
	for i, p := range parts {
		if partID == "" || p.ID == partID {
			out = append(out, i)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("part %s is not in this wave: %w", partID, domain.ErrNotFound)
	}
	return out, nil
}

// spawnPart starts the session of one part; a problem of the part fails the part, a problem of agterm fails the command.
func (r *Runner) spawnPart(ctx context.Context, s *store, i int, report *Report) error {
	part := s.parts()[i]
	promptFile := filepath.Join(RunFolder(s.base), part.ID+"-prompt.md")
	fail := func(reason string) error {
		report.OK = false
		report.add("FAIL %s: %s", part.ID, reason)
		return s.update(i, func(p *PartState) { p.Status = StatusFailed })
	}
	if !partIDPattern.MatchString(part.Role) {
		return fail(fmt.Sprintf("role %q must match [a-z][a-z0-9-]*", part.Role))
	}
	if info, err := os.Stat(promptFile); err != nil || info.IsDir() {
		return fail(fmt.Sprintf("prompt file %s is missing", promptFile))
	}
	// wait settles a part on its result file, so a file left by an earlier part with this id would end it at once
	if _, err := os.Lstat(part.ResultFile); err == nil {
		return fail(fmt.Sprintf("result file %s already exists: remove it before a re-dispatch, and keep part ids unique across the run", part.ResultFile))
	} else if !errors.Is(err, os.ErrNotExist) {
		return fail(fmt.Sprintf("cannot check result file %s: %v", part.ResultFile, err))
	}
	if part.SessionID != "" {
		if err := r.d.Sessions.Close(ctx, part.SessionID); err != nil {
			report.add("WARN %s: cannot close the old session %s: %v", part.ID, part.SessionID, err)
		}
	}
	pointer := fmt.Sprintf("Workflow: autopilot. Read the file %s and execute the mission described there exactly. Write your result file to %s as instructed. Send messages only with `autopilot say`, as described in your prompt file.", promptFile, part.ResultFile)
	id, err := r.d.Sessions.NewSession(ctx, agterm.NewSessionData{
		Parent:  s.st.Parent,
		Cwd:     s.st.Cwd,
		Name:    shortLabel(part.Label),
		Command: fmt.Sprintf("claude --dangerously-skip-permissions --agent autopilot-%s %s", part.Role, shellQuote(pointer)),
	})
	if err != nil {
		return fmt.Errorf("cannot start the session of part %s: %w", part.ID, err)
	}
	now := r.d.Now().UTC()
	report.add("spawned %s (%s) as session %s", part.ID, part.Label, id)
	return s.update(i, func(p *PartState) {
		p.SessionID = id
		p.Status = StatusRunning
		p.StartedAt = &now
		p.FinishedAt = nil
	})
}

func shortLabel(label string) string {
	r := []rune(strings.TrimSpace(label))
	if len(r) > maxSessionLabel {
		return string(r[:maxSessionLabel-1]) + "…"
	}
	return string(r)
}

// watchDialogs waits for the new sessions to get a terminal and answers the start dialogs where they show.
func (r *Runner) watchDialogs(ctx context.Context, s *store, indexes []int, report *Report) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, i := range indexes {
		part := s.parts()[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.watchDialog(ctx, part.SessionID); err != nil {
				mu.Lock()
				report.OK = false
				report.add("FAIL %s: %v", part.ID, err)
				mu.Unlock()
				if updateErr := s.update(i, func(p *PartState) { p.Status = StatusFailed }); updateErr != nil {
					mu.Lock()
					report.add("FAIL %s: %v", part.ID, updateErr)
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
}

func (r *Runner) watchDialog(ctx context.Context, sessionID string) error {
	realized := false
	for range r.d.RealizedAttempts {
		ok, err := r.d.Sessions.Realized(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("cannot check session %s: %w", sessionID, err)
		}
		if ok {
			realized = true
			break
		}
		if err := pause(ctx, r.d.PollInterval); err != nil {
			return err
		}
	}
	if !realized {
		return fmt.Errorf("session %s never got a terminal", sessionID)
	}
	answered := map[agterm.Dialog]bool{}
	for range r.d.DialogAttempts {
		text, err := r.d.Sessions.Text(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("cannot read session %s: %w", sessionID, err)
		}
		if agterm.Ready(text) {
			return nil
		}
		if dialog, keys, ok := agterm.DialogAnswer(text); ok && !answered[dialog] {
			for _, k := range keys {
				if err := r.d.Sessions.Type(ctx, sessionID, k); err != nil {
					return fmt.Errorf("cannot answer the %s dialog of session %s: %w", dialog, sessionID, err)
				}
			}
			answered[dialog] = true
		}
		if err := pause(ctx, r.d.PollInterval); err != nil {
			return err
		}
	}
	return nil
}

func pause(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("stopped: %w", ctx.Err())
	case <-time.After(d):
		return nil
	}
}

// Wait blocks until every running part of the wave has settled or limit has passed. A part settles when its session
// reports completed or blocked, when its result file appears, when its session is closed, or at the limit.
// Claude does not report a status on its own, so the result file is the main signal.
func (r *Runner) Wait(ctx context.Context, base string, wave int, limit time.Duration) (Report, error) {
	s, err := loadWave(base, wave)
	if err != nil {
		return Report{}, err
	}
	deadline := r.d.Now().Add(limit)
	report := Report{OK: true}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	for i, part := range s.parts() {
		if part.Status != StatusRunning {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			outcome, detail, err := r.waitPart(ctx, part, deadline)
			if err == nil {
				now := r.d.Now().UTC()
				err = s.update(i, func(p *PartState) {
					p.Status = outcome
					p.FinishedAt = &now
				})
			}
			mu.Lock()
			defer mu.Unlock()
			if err != nil && firstErr == nil {
				firstErr = err
			}
			if outcome != StatusExited {
				report.OK = false
			}
			report.add("%s (%s): %s", part.ID, part.SessionID, detail)
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return report, firstErr
	}
	status := StatusRunning
	if report.OK {
		status = StatusComplete
	}
	if err := s.finish(status); err != nil {
		return report, err
	}
	return report, nil
}

// waitPart returns the status the part gets and what ended the wait.
func (r *Runner) waitPart(ctx context.Context, part PartState, deadline time.Time) (status, detail string, err error) {
	for {
		if hasContent(part.ResultFile) {
			return StatusExited, "result file", nil
		}
		slice := min(r.d.WaitSlice, deadline.Sub(r.d.Now()))
		res, err := r.d.Sessions.Wait(ctx, part.SessionID, slice)
		if err != nil {
			return "", "", fmt.Errorf("cannot wait for session %s: %w", part.SessionID, err)
		}
		switch {
		case res.Left:
			return StatusClosed, "session closed", nil
		case res.Status == "blocked":
			return StatusBlocked, "blocked", nil
		case res.Status != "":
			return StatusExited, res.Status, nil
		}
		if hasContent(part.ResultFile) {
			return StatusExited, "result file", nil
		}
		if !r.d.Now().Before(deadline) {
			return StatusTimeout, "timeout", nil
		}
	}
}

func hasContent(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}

// Verify checks each part of the wave by evidence: the result file, its sections and the files changed since the
// snapshot. A part that fails is marked failed.
func (r *Runner) Verify(ctx context.Context, base string, wave int) (Report, error) {
	s, err := loadWave(base, wave)
	if err != nil {
		return Report{}, err
	}
	report := Report{OK: true}
	for i, part := range s.parts() {
		problem := settledProblem(part)
		if problem == "" {
			problem = resultProblem(part)
		}
		if problem == "" {
			report.add("OK   %s: result file complete", part.ID)
			continue
		}
		report.OK = false
		report.add("FAIL %s: %s", part.ID, problem)
		if err := s.update(i, func(p *PartState) { p.Status = StatusFailed }); err != nil {
			return report, err
		}
	}
	if err := r.verifyZones(ctx, s, &report); err != nil {
		return report, err
	}
	status := StatusComplete
	if !report.OK {
		status = StatusFailed
	}
	if err := s.finish(status); err != nil {
		return report, err
	}
	return report, nil
}

func short(sha string) string {
	if sha == "" {
		return "none"
	}
	return sha[:min(8, len(sha))]
}

// settledProblem refuses a part that failed before verify or never settled: a result file left by an earlier part
// with the same id would otherwise pass it.
func settledProblem(part PartState) string {
	switch part.Status {
	case StatusFailed:
		return "the part failed before verify (at spawn or an earlier verify): re-dispatch it"
	case StatusPending, StatusRunning:
		return fmt.Sprintf("the part has not settled (%s): run wave wait first", part.Status)
	}
	return ""
}

func resultProblem(part PartState) string {
	data, err := os.ReadFile(part.ResultFile)
	if err != nil || len(data) == 0 {
		return fmt.Sprintf("result file missing or empty (%s)", part.ResultFile)
	}
	var missing []string
	for _, sec := range requiredSections {
		if !regexp.MustCompile(`(?im)^#+ *` + regexp.QuoteMeta(sec)).Match(data) {
			missing = append(missing, sec)
		}
	}
	if len(missing) > 0 {
		return fmt.Sprintf("result file lacks section(s): %s", strings.Join(missing, ", "))
	}
	return ""
}

func (r *Runner) verifyZones(ctx context.Context, s *store, report *Report) error {
	data, err := os.ReadFile(snapshotFile(s.base, s.wave))
	if errors.Is(err, os.ErrNotExist) {
		report.OK = false
		report.add("FAIL zone check: no snapshot of the tree before wave %d: spawn the wave first", s.wave)
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot read the tree snapshot: %w", err)
	}
	var before Snap
	if err := json.Unmarshal(data, &before); err != nil {
		return fmt.Errorf("cannot parse the tree snapshot: %w", err)
	}
	// diff from the stored commit, so a commit made in between cannot hide an edit
	after, err := SnapshotSince(ctx, s.st.Cwd, before.Head)
	if err != nil {
		return err
	}
	// only the run session commits (workers never do), so a moved HEAD is itself a violation
	if head, err := headSHA(ctx, s.st.Cwd); err != nil {
		return err
	} else if head != before.Head {
		report.OK = false
		report.add("FAIL HEAD moved during wave %d (%s -> %s): workers must not commit", s.wave, short(before.Head), short(head))
	}
	var zones []string
	for _, p := range s.parts() {
		zones = append(zones, p.Files...)
	}
	changed := ChangedFiles(before.Files, after.Files)
	var waveParts []Part
	for _, p := range s.parts() {
		waveParts = append(waveParts, p.Part)
	}
	qaFiles, err := QATestsFor(s.base, waveParts)
	if err != nil {
		return err
	}
	// the snapshot diff has no renames, so a rename shows both paths and a QA test file moved or deleted is caught
	for _, f := range qaEdits(changed, qaFiles, s.parts()) {
		report.OK = false
		report.add("FAIL QA test file edited: %s", f)
	}
	outside := OutsideZones(changed, zones)
	if len(outside) == 0 {
		report.add("OK   all changed files are inside part zones")
		return nil
	}
	report.OK = false
	report.add("FAIL files changed outside every part's zone:")
	for _, f := range outside {
		report.add("       %s", f)
	}
	return nil
}

// Read returns the last lines of the screen of a part's session.
func (r *Runner) Read(ctx context.Context, base, partID string) (string, error) {
	st, err := LoadState(base)
	if err != nil {
		return "", err
	}
	for _, w := range st.Waves {
		for _, p := range w.Parts {
			if p.ID != partID {
				continue
			}
			if p.SessionID == "" {
				return "", fmt.Errorf("part %s has no session yet", partID)
			}
			return r.d.Sessions.TextTail(ctx, p.SessionID, readLines)
		}
	}
	return "", fmt.Errorf("part %s is not in the plan: %w", partID, domain.ErrNotFound)
}

// CloseWave closes the sessions of the wave; it tries every session and returns the first failure.
func (r *Runner) CloseWave(ctx context.Context, base string, wave int) error {
	s, err := loadWave(base, wave)
	if err != nil {
		return err
	}
	var first error
	for _, p := range s.parts() {
		if p.SessionID == "" {
			continue
		}
		if err := r.d.Sessions.Close(ctx, p.SessionID); err != nil && first == nil {
			first = fmt.Errorf("cannot close session %s of part %s: %w", p.SessionID, p.ID, err)
		}
	}
	return first
}
