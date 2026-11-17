package profile

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	pytestVersion    = "9.2.0"
	pytestCovVersion = "7.1.0"
	mutmutVersion    = "3.8.1"
)

// mutmut fails with this text when no mutant matches the given names.
const mutmutNoMutants = "Filtered for specific mutants, but nothing matches"

func pythonProfile(dir string) Profile {
	uvRun := pythonRun(dir)
	pytest := fmt.Sprintf("%s --with pytest==%s --with pytest-cov==%s pytest", uvRun, pytestVersion, pytestCovVersion)
	mutmut := fmt.Sprintf("%s --with pytest==%s --with mutmut==%s mutmut", uvRun, pytestVersion, mutmutVersion)
	return Profile{
		Language: LanguagePython,
		SkipMarkers: []*regexp.Regexp{
			regexp.MustCompile(`\bmark\.(skip|skipif|xfail)\b`),
			regexp.MustCompile(`\bpytest\.(skip|xfail|importorskip)\(`),
			regexp.MustCompile(`\bunittest\.(skip|skipIf|skipUnless|expectedFailure)\b`),
			regexp.MustCompile(`@(skip|skipIf|skipUnless|expectedFailure)\b`),
			regexp.MustCompile(`\.skipTest\(`),
		},
		runtime: []string{"uv"},
		probes: []probe{
			{tool: fmt.Sprintf("pytest %s with pytest-cov %s", pytestVersion, pytestCovVersion), command: fmt.Sprintf("%s --version", pytest)},
			{tool: fmt.Sprintf("mutmut %s", mutmutVersion), command: fmt.Sprintf("%s --with mutmut==%s python -c 'import mutmut'", uvRun, mutmutVersion)},
		},
		test:        fmt.Sprintf("%s --junitxml={junit} --cov=. --cov-report=xml:{coverage}", pytest),
		rawCoverage: "coverage.xml",
		mutateCommand: func(_ Dirs, files []string) (string, error) {
			names := make([]string, len(files))
			for i, f := range files {
				names[i] = quote(mutmutModuleGlob(f))
			}
			return fmt.Sprintf("%s run -- %s", mutmut, strings.Join(names, " ")), nil
		},
		mutable:         pythonMutable,
		collect:         fmt.Sprintf("%s export-cicd-stats", mutmut),
		noMutantsOutput: mutmutNoMutants,
		reportPath:      func(d Dirs) string { return filepath.Join(d.Repo, "mutants", "mutmut-cicd-stats.json") },
		readReport:      readMutmut,
		testID: func(_ string, c JUnitCase) string {
			return fmt.Sprintf("%s.%s", c.Classname, c.Name)
		},
		assertion: func(f Failure) bool {
			return f.Type == "AssertionError" || pythonAssertion.MatchString(f.Message)
		},
	}
}

// pythonAssertion starts the message pytest writes for an assert, an AssertionError (unittest too) or pytest.fail;
// every other exception, such as ImportError, AttributeError or NotImplementedError, is an error.
var pythonAssertion = regexp.MustCompile(`^(assert\b|AssertionError\b|Failed: )`)

// pythonRun is `uv run` with the repo's own dependencies: a pyproject.toml
// project brings them itself, an older layout names them in requirements.txt or setup.py.
func pythonRun(dir string) string {
	if exists(filepath.Join(dir, "pyproject.toml")) {
		return "uv run"
	}
	var run strings.Builder
	run.WriteString("uv run")
	if exists(filepath.Join(dir, "requirements.txt")) {
		run.WriteString(" --with-requirements requirements.txt")
	}
	if exists(filepath.Join(dir, "setup.py")) {
		run.WriteString(" --with-editable .")
	}
	return run.String()
}

func pythonMutable(file string) bool {
	if !strings.HasSuffix(file, ".py") {
		return false
	}
	base := path.Base(file)
	if base == "conftest.py" || strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py") {
		return false
	}
	for _, dir := range strings.Split(path.Dir(file), "/") {
		if dir == "tests" || dir == "test" {
			return false
		}
	}
	return true
}

// mutmutModuleGlob matches the mutants of one file: mutmut names a mutant
// <module>.x_<function>… or <module>.xǁ<class>…, with the src. prefix and
// __init__ dropped from the module.
func mutmutModuleGlob(file string) string {
	module := strings.ReplaceAll(strings.TrimSuffix(file, ".py"), "/", ".")
	module = strings.TrimPrefix(module, "src.")
	module = strings.TrimSuffix(module, ".__init__")
	return fmt.Sprintf("%s.x*", module)
}

func readMutmut(path string) (Mutation, error) {
	var stats struct {
		Killed     int `json:"killed"`
		Survived   int `json:"survived"`
		Total      int `json:"total"`
		NoTests    int `json:"no_tests"`
		Skipped    int `json:"skipped"`
		Suspicious int `json:"suspicious"`
		Timeout    int `json:"timeout"`
		Segfault   int `json:"segfault"`
	}
	if err := readJSONReport(path, &stats); err != nil {
		return Mutation{}, err
	}
	m := Mutation{
		Killed:     stats.Killed,
		Survived:   stats.Survived,
		TimedOut:   stats.Timeout,
		NoCoverage: stats.NoTests,
		Invalid:    stats.Suspicious + stats.Segfault,
	}
	m.Ignored = stats.Total - m.total()
	return m, nil
}
