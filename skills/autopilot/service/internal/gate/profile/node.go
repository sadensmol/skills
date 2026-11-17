package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	c8Version      = "12.0.0"
	strykerVersion = "10.1.1"
)

var stryker = fmt.Sprintf("npx --yes @stryker-mutator/core@%s", strykerVersion)

// nodeRunner is the package's own test runner: vitest when the package depends
// on it, else the runner built into Node.
type nodeRunner struct {
	test   string
	plain  string
	probes []probe
}

func nodeProfile(dir string) (Profile, error) {
	runner, err := detectNodeRunner(dir)
	if err != nil {
		return Profile{}, err
	}
	setup := "npm install --no-package-lock --no-audit --no-fund"
	if exists(filepath.Join(dir, "package-lock.json")) {
		setup = "npm ci --no-audit --no-fund"
	}
	return Profile{
		Language: LanguageNode,
		SkipMarkers: []*regexp.Regexp{
			regexp.MustCompile(`\b(it|test|describe|suite|context)(\.(concurrent|sequential))?\.(skip|skipIf|runIf|todo|only|fixme)\b`),
			regexp.MustCompile(`\b(xit|xtest|xdescribe)\(`),
			regexp.MustCompile(`\b(skip|todo|only)\s*:\s*(true|['"\x60])`),
			regexp.MustCompile(`\b(t|this|ctx|context)\.(skip|todo)\(`),
		},
		runtime:       []string{"node", "npm", "npx"},
		probes:        append(runner.probes, probe{tool: fmt.Sprintf("stryker %s", strykerVersion), command: fmt.Sprintf("%s --version", stryker)}),
		setup:         setup,
		test:          runner.test,
		rawCoverage:   filepath.Join("coverage", "lcov.info"),
		mutateCommand: strykerCommand(runner.plain),
		mutable:       nodeMutable,
		reportPath:    func(d Dirs) string { return filepath.Join(d.Out, "stryker.json") },
		readReport:    readStryker,
		testID:        nodeTestID,
		assertion: func(f Failure) bool {
			return nodeAssertion.MatchString(fmt.Sprintf("%s\n%s\n%s", f.Type, f.Message, f.Body))
		},
	}, nil
}

// nodeAssertion names the error of node:assert, chai and vitest (AssertionError) or Jest (expect(received)); a
// TypeError, a ReferenceError or a plain thrown Error is not an assertion.
var nodeAssertion = regexp.MustCompile(`\bAssertionError\b|\bERR_ASSERTION\b|\bexpect\(received\)`)

func detectNodeRunner(dir string) (nodeRunner, error) {
	body, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return nodeRunner{}, fmt.Errorf("cannot read package.json in %s: %w", dir, err)
	}
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(body, &pkg); err != nil {
		return nodeRunner{}, fmt.Errorf("cannot parse package.json in %s: %w", dir, err)
	}
	if _, ok := pkg.DevDependencies["vitest"]; ok {
		return vitestRunner(), nil
	}
	if _, ok := pkg.Dependencies["vitest"]; ok {
		return vitestRunner(), nil
	}
	c8 := fmt.Sprintf("npx --yes c8@%s", c8Version)
	return nodeRunner{
		test:   fmt.Sprintf("%s --all --reporter=lcov --report-dir={coverage_dir} node --test --test-reporter=spec --test-reporter-destination=stdout --test-reporter=junit --test-reporter-destination={junit}", c8),
		plain:  "node --test",
		probes: []probe{{tool: fmt.Sprintf("c8 %s", c8Version), command: fmt.Sprintf("%s --version", c8)}},
	}, nil
}

// vitestRunner uses the package's installed vitest, which needs @vitest/coverage-v8 for coverage.
func vitestRunner() nodeRunner {
	return nodeRunner{
		test:  "npx --no-install vitest run --reporter=default --reporter=junit --outputFile.junit={junit} --coverage.enabled=true --coverage.provider=v8 --coverage.reporter=lcov --coverage.reportsDirectory={coverage_dir}",
		plain: "npx --no-install vitest run",
	}
}

// strykerCommand runs Stryker with its command runner, so any test runner
// works; the config file lives in Dirs.Out to keep the checkout clean.
func strykerCommand(testCommand string) func(d Dirs, files []string) (string, error) {
	return func(d Dirs, files []string) (string, error) {
		config := map[string]any{
			"testRunner":    "command",
			"commandRunner": map[string]string{"command": testCommand},
			"mutate":        files,
			"reporters":     []string{"json"},
			"jsonReporter":  map[string]string{"fileName": filepath.Join(d.Out, "stryker.json")},
			"tempDirName":   filepath.Join(d.Out, "stryker-tmp"),
			"cleanTempDir":  true,
		}
		body, err := json.Marshal(config)
		if err != nil {
			return "", fmt.Errorf("cannot encode the Stryker config: %w", err)
		}
		path := filepath.Join(d.Out, "stryker.config.json")
		if err := os.WriteFile(path, body, 0o644); err != nil {
			return "", fmt.Errorf("cannot write the Stryker config: %w", err)
		}
		return fmt.Sprintf("%s run %s", stryker, quote(path)), nil
	}
}

var nodeSourceExt = map[string]bool{
	".js": true, ".jsx": true, ".mjs": true, ".cjs": true,
	".ts": true, ".tsx": true, ".mts": true, ".cts": true,
}

func nodeMutable(file string) bool {
	base := path.Base(file)
	if !nodeSourceExt[path.Ext(file)] || strings.HasSuffix(base, ".d.ts") ||
		strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") {
		return false
	}
	for _, dir := range strings.Split(path.Dir(file), "/") {
		if dir == "test" || dir == "tests" || dir == "__tests__" || dir == "node_modules" {
			return false
		}
	}
	return true
}

func readStryker(path string) (Mutation, error) {
	var report struct {
		Files map[string]struct {
			Mutants []struct {
				Status string `json:"status"`
			} `json:"mutants"`
		} `json:"files"`
	}
	if err := readJSONReport(path, &report); err != nil {
		return Mutation{}, err
	}
	var m Mutation
	for _, f := range report.Files {
		for _, mu := range f.Mutants {
			switch mu.Status {
			case "Killed":
				m.Killed++
			case "Survived":
				m.Survived++
			case "Timeout":
				m.TimedOut++
			case "NoCoverage":
				m.NoCoverage++
			case "CompileError", "RuntimeError":
				m.Invalid++
			default:
				m.Ignored++
			}
		}
	}
	return m, nil
}

// nodeTestID is "<file> > <suite> > <test>". Vitest puts the file in the
// classname and the suites in the name; Node's runner puts the file in the file
// attribute, the innermost suite in the classname, and "test" at top level.
func nodeTestID(repoDir string, c JUnitCase) string {
	if c.File == "" {
		return fmt.Sprintf("%s > %s", c.Classname, c.Name)
	}
	file := c.File
	if filepath.IsAbs(c.File) {
		file = filepath.ToSlash(relativeTo(repoDir, c.File))
	}
	if c.Classname == "test" {
		return fmt.Sprintf("%s > %s", file, c.Name)
	}
	return fmt.Sprintf("%s > %s > %s", file, c.Classname, c.Name)
}
