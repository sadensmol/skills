package tests

import (
	"bufio"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/sadensmol/autopilot/internal/gate"
	"github.com/sadensmol/autopilot/internal/gate/profile"
)

// StubProfile is a test profile whose commands write prepared reports. It reads the checkout it runs in:
//   - every *_test.stub file holds lines "<classname> <name> <rule>"; a rule is pass, fail, error, skip,
//     needs:<path> (passes when path exists, else an assertion failure) or compiles-with:<path>
//     (passes when path exists, else an error, as a test that does not compile), panic-unless:<path>
//     (passes when path exists, else a failure whose output holds a panic) or raise-unless:<path>:<Exception>
//     (passes when path exists, else a failure whose message is "<Exception>: boom"), or flaky (passes in the first
//     test run of a checkout, fails in a later one);
//   - .stub/coverage.xml is the Cobertura report the test run writes;
//   - .stub/coverage-files lists the files the coverage report holds; a *.code file is the stub's code file;
//   - .stub/diff-cover.json {"lines","missing","percent"} is what diff coverage reports;
//   - .stub/mutation.json {"killed","survived","invalid"} is what the mutation tool reports;
//   - .stub/mutation-timeout makes the mutation run hit its time box;
//   - .stub/probe-fails makes the tool probe fail, as a failed download does;
//   - .stub/sleep makes the test run wait until its context ends.
type StubProfile struct {
	mu            sync.Mutex
	compareRefs   []string
	mutationFiles [][]string
	testRuns      map[string]int
	testOuts      []string
}

func (p *StubProfile) Detect(string, map[string]string) (gate.Detected, error) {
	return gate.Detected{
		Profile:     p,
		Language:    profile.LanguageGo,
		Floors:      profile.DefaultFloors,
		SkipMarkers: []*regexp.Regexp{regexp.MustCompile(`\.Skip(f|Now)?\(`)},
	}, nil
}

// CompareRefs are the refs diff coverage was asked to compare against, in call order.
func (p *StubProfile) CompareRefs() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.compareRefs...)
}

// TestOuts are the names of the report folders of the test runs, in call order.
func (p *StubProfile) TestOuts() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.testOuts...)
}

// MutationFiles are the file lists the mutation runs got, in call order.
func (p *StubProfile) MutationFiles() [][]string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([][]string(nil), p.mutationFiles...)
}

func (p *StubProfile) Probe(_ context.Context, d profile.Dirs) error {
	if stubExists(d.Repo, ".stub/probe-fails") {
		return &profile.CouldNotRunError{Step: "probe stub-tool 1.0", Err: errors.New("download failed")}
	}
	return nil
}

type stubJUnitCase struct {
	Classname string    `xml:"classname,attr"`
	Name      string    `xml:"name,attr"`
	Failure   *stubNote `xml:"failure"`
	Error     *stubNote `xml:"error"`
	Skipped   *stubNote `xml:"skipped"`
}

type stubNote struct {
	Message string `xml:"message,attr"`
	Body    string `xml:",chardata"`
}

type stubSuite struct {
	XMLName xml.Name        `xml:"testsuite"`
	Name    string          `xml:"name,attr"`
	Cases   []stubJUnitCase `xml:"testcase"`
}

type stubSuites struct {
	XMLName xml.Name    `xml:"testsuites"`
	Suites  []stubSuite `xml:"testsuite"`
}

func (p *StubProfile) Tests(ctx context.Context, d profile.Dirs) (profile.TestRun, error) {
	if stubExists(d.Repo, ".stub/sleep") {
		<-ctx.Done()
		return profile.TestRun{}, &profile.CouldNotRunError{Step: "test command", Err: fmt.Errorf("command stopped: %w", ctx.Err())}
	}
	p.mu.Lock()
	if p.testRuns == nil {
		p.testRuns = map[string]int{}
	}
	again := p.testRuns[d.Repo] > 0
	p.testRuns[d.Repo]++
	p.testOuts = append(p.testOuts, filepath.Base(d.Out))
	p.mu.Unlock()
	cases, err := stubCases(d.Repo, again)
	if err != nil {
		return profile.TestRun{}, err
	}
	exit := 0
	for _, c := range cases {
		if c.Failure != nil || c.Error != nil {
			exit = 1
		}
	}
	body, err := xml.Marshal(stubSuites{Suites: []stubSuite{{Name: "stub", Cases: cases}}})
	if err != nil {
		return profile.TestRun{}, err
	}
	run := profile.TestRun{ExitCode: exit, JUnit: filepath.Join(d.Out, "junit.xml")}
	if err := os.WriteFile(run.JUnit, body, 0o644); err != nil {
		return profile.TestRun{}, err
	}
	if cobertura, err := os.ReadFile(filepath.Join(d.Repo, ".stub", "coverage.xml")); err == nil {
		run.Coverage = filepath.Join(d.Out, "coverage.xml")
		if err := os.WriteFile(run.Coverage, cobertura, 0o644); err != nil {
			return profile.TestRun{}, err
		}
	}
	return run, nil
}

// stubCases reads the testcases of the checkout; again is true for a later test run of the checkout.
func stubCases(repo string, again bool) ([]stubJUnitCase, error) {
	var cases []stubJUnitCase
	err := filepath.WalkDir(repo, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() && e.Name() == ".git" {
			return filepath.SkipDir
		}
		if e.IsDir() || !strings.HasSuffix(path, "_test.stub") {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close() //nolint:errcheck // a read-only fixture file
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) != 3 {
				continue
			}
			rule := fields[2]
			if rule == "flaky" && again {
				rule = "fail"
			}
			cases = append(cases, stubCase(repo, fields[0], fields[1], rule))
		}
		return scanner.Err()
	})
	return cases, err
}

func stubCase(repo, classname, name, rule string) stubJUnitCase {
	c := stubJUnitCase{Classname: classname, Name: name}
	kind, rest, _ := strings.Cut(rule, ":")
	path, exception, _ := strings.Cut(rest, ":")
	switch {
	case kind == "panic-unless" && !stubExists(repo, path):
		c.Failure = &stubNote{Message: "Failed", Body: "=== RUN Test\npanic: runtime error: invalid memory address [recovered]\n"}
	case kind == "raise-unless" && !stubExists(repo, path):
		c.Failure = &stubNote{Message: fmt.Sprintf("%s: boom", exception)}
	case kind == "fail", kind == "needs" && !stubExists(repo, path):
		c.Failure = &stubNote{Message: "expected 3, got 0"}
	case kind == "error", kind == "compiles-with" && !stubExists(repo, path):
		c.Error = &stubNote{Message: "undefined: Add"}
	case kind == "skip":
		c.Skipped = &stubNote{Message: "skipped"}
	}
	return c
}

func (p *StubProfile) DiffCoverage(_ context.Context, d profile.Dirs, coverage, compareRef string) (profile.DiffCoverage, error) {
	p.mu.Lock()
	p.compareRefs = append(p.compareRefs, compareRef)
	p.mu.Unlock()
	if coverage == "" {
		return profile.DiffCoverage{}, &profile.CouldNotRunError{Step: "diff coverage", Err: errors.New("the test run wrote no coverage report")}
	}
	var out struct {
		Lines   int     `json:"lines"`
		Missing int     `json:"missing"`
		Percent float64 `json:"percent"`
	}
	if err := stubJSON(d.Repo, ".stub/diff-cover.json", &out); err != nil {
		return profile.DiffCoverage{}, err
	}
	result := profile.DiffCoverage{Status: profile.StatusMeasured, Lines: out.Lines, Missing: out.Missing, Percent: out.Percent}
	if out.Lines == 0 {
		result.Status = profile.StatusNotApplicable
	}
	return result, nil
}

func (p *StubProfile) Mutation(_ context.Context, d profile.Dirs, files []string) (profile.Mutation, error) {
	p.mu.Lock()
	p.mutationFiles = append(p.mutationFiles, append([]string(nil), files...))
	p.mu.Unlock()
	if stubExists(d.Repo, ".stub/mutation-timeout") {
		return profile.Mutation{Status: profile.StatusTimeBoxHit, Killed: 5}, nil
	}
	var out struct {
		Killed   int `json:"killed"`
		Survived int `json:"survived"`
		Invalid  int `json:"invalid"`
	}
	if err := stubJSON(d.Repo, ".stub/mutation.json", &out); err != nil {
		return profile.Mutation{}, err
	}
	m := profile.Mutation{Status: profile.StatusMeasured, Killed: out.Killed, Survived: out.Survived, Invalid: out.Invalid}
	if out.Killed+out.Survived+out.Invalid == 0 {
		m.Status = profile.StatusNotApplicable
	}
	return m, nil
}

func (p *StubProfile) TestID(_ string, c profile.JUnitCase) string {
	return fmt.Sprintf("%s.%s", c.Classname, c.Name)
}

func stubExists(repo, rel string) bool {
	_, err := os.Stat(filepath.Join(repo, rel))
	return err == nil
}

// stubJSON leaves into unchanged when the file does not exist.
func stubJSON(repo, rel string, into any) error {
	body, err := os.ReadFile(filepath.Join(repo, rel))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(body, into)
}

var stubRaised = regexp.MustCompile(`^\w+(Error|Exception):`)

// AssertionFailure treats a panic or a raised exception other than AssertionError as an error.
func (p *StubProfile) AssertionFailure(f profile.Failure) bool {
	if strings.Contains(f.Body, "panic:") {
		return false
	}
	return strings.HasPrefix(f.Message, "AssertionError") || !stubRaised.MatchString(f.Message)
}

// UnreportedCode lists the *.code files among files that .stub/coverage-files does not name.
func (p *StubProfile) UnreportedCode(d profile.Dirs, _ string, files []string) ([]string, error) {
	listed := map[string]bool{}
	if body, err := os.ReadFile(filepath.Join(d.Repo, ".stub", "coverage-files")); err == nil {
		for _, f := range strings.Fields(string(body)) {
			listed[f] = true
		}
	}
	var missing []string
	for _, f := range files {
		if strings.HasSuffix(f, ".code") && !listed[f] {
			missing = append(missing, f)
		}
	}
	return missing, nil
}

func (p *StubProfile) Parent(profile.JUnitCase) (profile.JUnitCase, bool) {
	return profile.JUnitCase{}, false
}
