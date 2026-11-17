package profile

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
)

// Language is the language a repo's profile was detected for.
type Language string

const (
	LanguageGo     Language = "go"
	LanguagePython Language = "python"
	LanguageNode   Language = "node"
)

const diffCoverVersion = "11.0.2"

var (
	// ErrNoProfile marks a repo the gate cannot serve: no marker file or no base runtime.
	ErrNoProfile = errors.New("no usable test profile")
	// ErrCouldNotRun marks a tool that did not run or gave no report, which is not a test failure.
	ErrCouldNotRun = errors.New("gate could not run")
)

// Floors are the thresholds the gate holds a change to.
type Floors struct {
	DiffCoverageMin float64
	MutationMin     float64
	MutationTimeout time.Duration
}

var DefaultFloors = Floors{DiffCoverageMin: 80, MutationMin: 60, MutationTimeout: 30 * time.Minute}

// Status says whether a measurement exists. StatusNotApplicable means there was
// nothing to measure: no changed line with coverage data, or no mutable code.
type Status string

const (
	StatusMeasured      Status = "measured"
	StatusTimeBoxHit    Status = "time_box_hit"
	StatusNotApplicable Status = "n/a"
)

// Dirs are where a profile works: Repo is the checkout, Out takes the reports and logs.
type Dirs struct {
	Repo string
	Out  string
}

// TestRun is the raw result of the test command. Coverage is empty when the run wrote no coverage report.
type TestRun struct {
	ExitCode int
	JUnit    string
	Coverage string
}

// DiffCoverage is diff-cover's count of the changed lines that carry coverage data.
type DiffCoverage struct {
	Status  Status
	Lines   int
	Missing int
	Percent float64
}

// Mutation counts mutants by outcome. Invalid mutants did not compile or crashed
// the run; Ignored ones were skipped or never checked.
type Mutation struct {
	Status     Status
	Killed     int
	Survived   int
	TimedOut   int
	NoCoverage int
	Invalid    int
	Ignored    int
}

func (m Mutation) total() int {
	return m.Killed + m.Survived + m.TimedOut + m.NoCoverage + m.Invalid
}

// JUnitCase is one testcase element of a JUnit report.
type JUnitCase struct {
	Classname string
	Name      string
	File      string
}

// Failure is one failure element of a JUnit testcase: its type and message attributes and its text.
type Failure struct {
	Type    string
	Message string
	Body    string
}

// CouldNotRunError says which step failed; it matches ErrCouldNotRun.
type CouldNotRunError struct {
	Step     string
	Log      string
	ExitCode int
	Err      error
}

func (e *CouldNotRunError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %s", ErrCouldNotRun, e.Step, e.Err)
	}
	return fmt.Sprintf("%s: %s exited with code %d, log %s", ErrCouldNotRun, e.Step, e.ExitCode, e.Log)
}

func (e *CouldNotRunError) Unwrap() error {
	return e.Err
}

func (e *CouldNotRunError) Is(target error) bool {
	return target == ErrCouldNotRun
}

type probe struct {
	tool    string
	command string
}

// Profile holds the commands the gate runs for one repo. Every tool is fetched
// on demand at a version pinned here. A command template takes {name}
// placeholders, which are replaced by shell-quoted paths:
//   - test: {junit} (JUnit XML to write), {coverage} (raw coverage to write:
//     Go coverprofile, Cobertura XML for Python, LCOV for Node), {coverage_dir}.
//   - mutation: {files} (the changed mutable files), {report} (the report the
//     language's default tool writes).
type Profile struct {
	Language    Language
	Floors      Floors
	SkipMarkers []*regexp.Regexp

	runtime []string
	probes  []probe
	setup   string
	test    string
	// rawCoverage is the file the test command writes, relative to Dirs.Out.
	rawCoverage string
	// convert turns {coverage} into {coverage_report} when the raw format is not one diff-cover reads.
	convert string
	mutate  string
	// mutateCommand builds the default mutation command when no override replaces it.
	mutateCommand func(d Dirs, files []string) (string, error)
	mutable       func(path string) bool
	// collect runs after the mutation command, also after a time box hit, to write the report.
	collect string
	// noMutantsOutput is what the mutation tool prints, with a failing exit, when no mutant matches the files.
	noMutantsOutput string
	reportPath      func(d Dirs) string
	readReport      func(path string) (Mutation, error)
	testID          func(repoDir string, c JUnitCase) string
	// assertion tells an assertion failure from a test that failed because it threw or panicked.
	assertion func(f Failure) bool
	// assertionOverride is an extra assertion pattern from the repo's overrides.
	assertionOverride *regexp.Regexp
	// parent is the testcase a subtest runs in, for a runner that reports a crash only on the parent.
	parent func(c JUnitCase) (JUnitCase, bool)
}

// Detect picks the profile by marker file and applies the repo's overrides.
func Detect(dir string, overrides map[string]string) (Profile, error) {
	var p Profile
	switch {
	case exists(filepath.Join(dir, "go.mod")):
		p = goProfile()
	case exists(filepath.Join(dir, "pyproject.toml")), exists(filepath.Join(dir, "setup.py")), hasRequirements(dir):
		p = pythonProfile(dir)
	case exists(filepath.Join(dir, "package.json")):
		node, err := nodeProfile(dir)
		if err != nil {
			return Profile{}, err
		}
		p = node
	default:
		return Profile{}, fmt.Errorf("no go.mod, pyproject.toml, setup.py, requirements*.txt or package.json in %s: %w", dir, ErrNoProfile)
	}
	p.Floors = DefaultFloors
	p.runtime = append(p.runtime, "uvx")
	p.probes = append(p.probes, probe{
		tool:    fmt.Sprintf("diff-cover %s", diffCoverVersion),
		command: fmt.Sprintf("uvx diff-cover@%s --version", diffCoverVersion),
	})
	if err := p.applyOverrides(overrides); err != nil {
		return Profile{}, err
	}
	return p, nil
}

// Usable is the intake check: a marker file and the base runtime on PATH. It
// runs no tool, so a tool that cannot be fetched shows later at the gate.
func Usable(dir string, overrides map[string]string) (Profile, error) {
	p, err := Detect(dir, overrides)
	if err != nil {
		return Profile{}, err
	}
	for _, bin := range p.runtime {
		if _, err := exec.LookPath(bin); err != nil {
			return Profile{}, fmt.Errorf("the %s profile needs %q on PATH: %w", p.Language, bin, ErrNoProfile)
		}
	}
	return p, nil
}

func (p *Profile) applyOverrides(overrides map[string]string) error {
	keys := make([]string, 0, len(overrides))
	for k := range overrides {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := overrides[key]
		switch key {
		case "test", "mutation":
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("profile override %s is empty", key)
			}
			if key == "test" {
				p.test = value
			} else {
				p.mutate = value
			}
		case "diff_coverage_min", "mutation_min":
			floor, err := strconv.ParseFloat(value, 64)
			if err != nil || math.IsNaN(floor) || floor < 0 || floor > 100 {
				return fmt.Errorf("profile override %s is %q, want a percent from 0 to 100", key, value)
			}
			if key == "diff_coverage_min" {
				p.Floors.DiffCoverageMin = floor
			} else {
				p.Floors.MutationMin = floor
			}
		case "assertion_pattern":
			pattern, err := regexp.Compile(value)
			if err != nil || value == "" {
				return fmt.Errorf("profile override %s is %q, want a regular expression", key, value)
			}
			p.assertionOverride = pattern
		case "mutation_timeout_minutes":
			minutes, err := strconv.Atoi(value)
			if err != nil || minutes <= 0 {
				return fmt.Errorf("profile override %s is %q, want whole minutes above 0", key, value)
			}
			p.Floors.MutationTimeout = time.Duration(minutes) * time.Minute
		default:
			return fmt.Errorf("profile override %s is not a known key", key)
		}
	}
	return nil
}

// Probe runs each pinned tool once, which also fetches it. A failure is ErrCouldNotRun.
func (p Profile) Probe(ctx context.Context, d Dirs) error {
	for i, pr := range p.probes {
		step := fmt.Sprintf("probe %s", pr.tool)
		if err := p.runStep(ctx, d, step, fmt.Sprintf("probe-%d", i), pr.command); err != nil {
			return err
		}
	}
	return nil
}

// Tests installs the dependencies the profile needs, then runs the test command.
// A failing test is a non-zero ExitCode, not an error; a missing JUnit report is ErrCouldNotRun.
func (p Profile) Tests(ctx context.Context, d Dirs) (TestRun, error) {
	if p.setup != "" {
		if err := p.runStep(ctx, d, "dependency install", "setup", p.setup); err != nil {
			return TestRun{}, err
		}
	}
	junit := filepath.Join(d.Out, "junit.xml")
	raw := filepath.Join(d.Out, p.rawCoverage)
	for _, stale := range []string{junit, raw} {
		if err := os.Remove(stale); err != nil && !errors.Is(err, os.ErrNotExist) {
			return TestRun{}, fmt.Errorf("cannot remove old report %s: %w", stale, err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(raw), 0o755); err != nil {
		return TestRun{}, fmt.Errorf("cannot create coverage folder: %w", err)
	}
	command := expand(p.test, map[string]string{"junit": junit, "coverage": raw, "coverage_dir": filepath.Dir(raw)})
	logPath := filepath.Join(d.Out, "test.log")
	code, err := execute(ctx, d.Repo, logPath, command)
	if err != nil {
		return TestRun{}, &CouldNotRunError{Step: "test command", Log: logPath, Err: err}
	}
	if !exists(junit) {
		return TestRun{}, &CouldNotRunError{Step: "test command (no JUnit report)", Log: logPath, ExitCode: code}
	}
	run := TestRun{ExitCode: code, JUnit: junit}
	if !exists(raw) {
		return run, nil
	}
	run.Coverage = raw
	if p.convert != "" {
		run.Coverage = filepath.Join(d.Out, "coverage.xml")
		convert := expand(p.convert, map[string]string{"coverage": raw, "coverage_report": run.Coverage})
		if err := p.runStep(ctx, d, "coverage conversion", "coverage", convert); err != nil {
			return TestRun{}, err
		}
	}
	return run, nil
}

// DiffCoverage measures the coverage of the lines changed since compareRef.
func (p Profile) DiffCoverage(ctx context.Context, d Dirs, coverage, compareRef string) (DiffCoverage, error) {
	if coverage == "" {
		return DiffCoverage{}, &CouldNotRunError{Step: "diff coverage", Err: errors.New("the test run wrote no coverage report")}
	}
	report := filepath.Join(d.Out, "diff-cover.json")
	command := fmt.Sprintf("uvx diff-cover@%s %s --compare-branch=%s --format %s",
		diffCoverVersion, quote(coverage), quote(compareRef), quote(fmt.Sprintf("json:%s", report)))
	if err := p.runStep(ctx, d, "diff coverage", "diff-cover", command); err != nil {
		return DiffCoverage{}, err
	}
	body, err := os.ReadFile(report)
	if err != nil {
		return DiffCoverage{}, &CouldNotRunError{Step: "diff coverage report", Err: err}
	}
	var out struct {
		Files map[string]struct {
			Covered   []int `json:"covered_lines"`
			Violation []int `json:"violation_lines"`
		} `json:"src_stats"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return DiffCoverage{}, &CouldNotRunError{Step: "diff coverage report", Err: err}
	}
	if out.Files == nil {
		return DiffCoverage{}, &CouldNotRunError{Step: "diff coverage report", Err: errors.New("the report has no src_stats")}
	}
	// test files are left out: only changed code lines count, by the profile's own test-file rule
	var result DiffCoverage
	for file, stats := range out.Files {
		if rel, ok := repoPath(d.Repo, file); ok && p.mutable(rel) {
			result.Lines += len(stats.Covered) + len(stats.Violation)
			result.Missing += len(stats.Violation)
		}
	}
	if result.Lines == 0 {
		result.Status = StatusNotApplicable
		return result, nil
	}
	result.Status = StatusMeasured
	result.Percent = 100 * float64(result.Lines-result.Missing) / float64(result.Lines)
	return result, nil
}

// Mutation runs the mutation tool on the mutable files among files, within
// Floors.MutationTimeout. For Node, call Tests first: it installs the dependencies.
func (p Profile) Mutation(ctx context.Context, d Dirs, files []string) (Mutation, error) {
	var mutable []string
	for _, f := range files {
		if rel, ok := repoPath(d.Repo, f); ok && p.mutable(rel) {
			mutable = append(mutable, rel)
		}
	}
	if len(mutable) == 0 {
		return Mutation{Status: StatusNotApplicable}, nil
	}
	report := p.reportPath(d)
	if err := os.Remove(report); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Mutation{}, fmt.Errorf("cannot remove old mutation report %s: %w", report, err)
	}
	command, err := p.mutationCommand(d, mutable, report)
	if err != nil {
		return Mutation{}, err
	}

	boxCtx, cancel := context.WithTimeout(ctx, p.Floors.MutationTimeout)
	defer cancel()
	logPath := filepath.Join(d.Out, "mutation.log")
	code, err := execute(boxCtx, d.Repo, logPath, command)
	if ctx.Err() != nil {
		return Mutation{}, fmt.Errorf("mutation stopped: %w", ctx.Err())
	}
	timeBoxHit := boxCtx.Err() != nil
	if !timeBoxHit {
		if err != nil {
			return Mutation{}, &CouldNotRunError{Step: "mutation", Log: logPath, Err: err}
		}
		if code != 0 {
			if p.noMutantsOutput != "" && logContains(logPath, p.noMutantsOutput) {
				return Mutation{Status: StatusNotApplicable}, nil
			}
			return Mutation{}, &CouldNotRunError{Step: "mutation", Log: logPath, ExitCode: code}
		}
	}
	if p.collect != "" {
		if err := p.runStep(ctx, d, "mutation report", "mutation-report", p.collect); err != nil {
			if !timeBoxHit {
				return Mutation{}, err
			}
			// after a time box hit the result stands with whatever counts exist
			log.Warn().Ctx(ctx).Err(err).Msgf("no mutation report after the time box in %s", d.Repo)
		}
	}
	m, err := p.readReport(report)
	if err != nil {
		return Mutation{}, &CouldNotRunError{Step: "mutation report", Err: err}
	}
	switch {
	case timeBoxHit:
		m.Status = StatusTimeBoxHit
	case m.total() == 0:
		m.Status = StatusNotApplicable
	default:
		m.Status = StatusMeasured
	}
	return m, nil
}

func (p Profile) mutationCommand(d Dirs, files []string, report string) (string, error) {
	if p.mutate == "" {
		return p.mutateCommand(d, files)
	}
	quoted := make([]string, len(files))
	for i, f := range files {
		if strings.HasPrefix(f, "-") {
			f = fmt.Sprintf("./%s", f)
		}
		quoted[i] = quote(f)
	}
	return strings.NewReplacer("{files}", strings.Join(quoted, " "), "{report}", quote(report)).Replace(p.mutate), nil
}

// AssertionFailure reports whether a JUnit failure is an assertion. A test that panicked, or threw anything but
// an assertion error, failed because the code under test is missing or broken, which is an error. The repo's
// assertion_pattern override, matched against the type, the message and the text each, adds an assertion library.
func (p Profile) AssertionFailure(f Failure) bool {
	if p.assertionOverride != nil {
		for _, part := range []string{f.Type, f.Message, f.Body} {
			if p.assertionOverride.MatchString(part) {
				return true
			}
		}
	}
	return p.assertion(f)
}

// Parent is the testcase that c runs in as a subtest; ok is false for a top-level test.
func (p Profile) Parent(c JUnitCase) (JUnitCase, bool) {
	if p.parent == nil {
		return JUnitCase{}, false
	}
	return p.parent(c)
}

// UnreportedCode lists the code files among files that the coverage report does not hold, such as a module no
// test loads. Files are repo-relative; the report is Cobertura XML or LCOV.
func (p Profile) UnreportedCode(d Dirs, coverage string, files []string) ([]string, error) {
	reported, err := reportedFiles(d.Repo, coverage)
	if err != nil {
		return nil, err
	}
	var missing []string
	for _, f := range files {
		if rel, ok := repoPath(d.Repo, f); ok && p.mutable(rel) && !reported[rel] {
			missing = append(missing, rel)
		}
	}
	return missing, nil
}

// TestID is the id a JUnit testcase has in the criteria table.
func (p Profile) TestID(repoDir string, c JUnitCase) string {
	return p.testID(repoDir, c)
}

func (p Profile) runStep(ctx context.Context, d Dirs, step, logName, command string) error {
	logPath := filepath.Join(d.Out, fmt.Sprintf("%s.log", logName))
	code, err := execute(ctx, d.Repo, logPath, command)
	if err != nil {
		return &CouldNotRunError{Step: step, Log: logPath, Err: err}
	}
	if code != 0 {
		return &CouldNotRunError{Step: step, Log: logPath, ExitCode: code}
	}
	return nil
}

// execute runs command through sh in its own process group, so a time box
// stops the tool and every child it started. A non-zero exit is a code, not an error.
func execute(ctx context.Context, dir, logPath, command string) (int, error) {
	logFile, err := os.Create(logPath)
	if err != nil {
		return 0, fmt.Errorf("cannot create log %s: %w", logPath, err)
	}
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	cmd.Dir = dir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	runErr := cmd.Run()
	if err := logFile.Close(); err != nil {
		return 0, fmt.Errorf("cannot close log %s: %w", logPath, err)
	}
	if ctx.Err() != nil {
		return 0, fmt.Errorf("command stopped: %w", ctx.Err())
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	if runErr != nil {
		return 0, fmt.Errorf("cannot start command: %w", runErr)
	}
	return 0, nil
}

func expand(template string, paths map[string]string) string {
	pairs := make([]string, 0, 2*len(paths))
	for name, path := range paths {
		pairs = append(pairs, fmt.Sprintf("{%s}", name), quote(path))
	}
	return strings.NewReplacer(pairs...).Replace(template)
}

func quote(s string) string {
	return fmt.Sprintf("'%s'", strings.ReplaceAll(s, "'", `'\''`))
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// relativeTo makes file relative to dir. Node reports the real path, so a dir
// behind a symlink (macOS /var is /private/var) is resolved first.
func relativeTo(dir, file string) string {
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		if rel, err := filepath.Rel(real, file); err == nil && !strings.HasPrefix(rel, "..") {
			return rel
		}
	}
	if rel, err := filepath.Rel(dir, file); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return file
}

// repoPath is file as a clean slash path relative to repo; ok is false for a
// file outside repo. A changed file may come as "./a.go" or as an absolute path.
func repoPath(repo, file string) (string, bool) {
	if filepath.IsAbs(file) {
		file = relativeTo(repo, file)
		if filepath.IsAbs(file) {
			return "", false
		}
	}
	clean := filepath.ToSlash(filepath.Clean(file))
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false
	}
	return clean, true
}

func hasRequirements(dir string) bool {
	matches, err := filepath.Glob(filepath.Join(dir, "requirements*.txt"))
	return err == nil && len(matches) > 0
}

func logContains(path, text string) bool {
	body, err := os.ReadFile(path)
	return err == nil && strings.Contains(string(body), text)
}

// readJSONReport leaves into unchanged when the tool wrote no report, which it
// does when it found no mutant.
func readJSONReport(path string, into any) error {
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", path, err)
	}
	if err := json.Unmarshal(body, into); err != nil {
		return fmt.Errorf("cannot parse %s: %w", path, err)
	}
	return nil
}

// reportedFiles is the set of repo-relative files a Cobertura or LCOV report holds.
func reportedFiles(repo, coverage string) (map[string]bool, error) {
	body, err := os.ReadFile(coverage)
	if err != nil {
		return nil, fmt.Errorf("cannot read coverage report %s: %w", coverage, err)
	}
	if strings.HasPrefix(strings.TrimSpace(string(body)), "<") {
		return coberturaFiles(repo, body)
	}
	reported := map[string]bool{}
	for _, line := range strings.Split(string(body), "\n") {
		if file, ok := strings.CutPrefix(strings.TrimSpace(line), "SF:"); ok {
			addReported(reported, repo, file)
		}
	}
	return reported, nil
}

// coberturaFiles reads every class filename, which is relative to one of the report's sources or absolute.
func coberturaFiles(repo string, body []byte) (map[string]bool, error) {
	var sources, filenames []string
	decoder := xml.NewDecoder(bytes.NewReader(body))
	inSource := false
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("cannot parse the Cobertura report: %w", err)
		}
		switch t := token.(type) {
		case xml.StartElement:
			inSource = t.Name.Local == "source"
			if t.Name.Local == "class" {
				for _, a := range t.Attr {
					if a.Name.Local == "filename" {
						filenames = append(filenames, a.Value)
					}
				}
			}
		case xml.CharData:
			if inSource {
				sources = append(sources, strings.TrimSpace(string(t)))
			}
		case xml.EndElement:
			inSource = false
		}
	}
	reported := map[string]bool{}
	for _, f := range filenames {
		addReported(reported, repo, f)
		for _, s := range sources {
			if !filepath.IsAbs(f) && s != "" {
				addReported(reported, repo, filepath.Join(s, f))
			}
		}
	}
	return reported, nil
}

func addReported(reported map[string]bool, repo, file string) {
	if rel, ok := repoPath(repo, file); ok {
		reported[rel] = true
	}
}
