package profile

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	gotestsumVersion        = "v1.13.0"
	gocoverCoberturaVersion = "v1.5.0"
	gremlinsVersion         = "v0.6.0"
)

var (
	gotestsum        = fmt.Sprintf("go run gotest.tools/gotestsum@%s", gotestsumVersion)
	gocoverCobertura = fmt.Sprintf("go run github.com/boumenot/gocover-cobertura@%s", gocoverCoberturaVersion)
	gremlins         = fmt.Sprintf("go run github.com/go-gremlins/gremlins/cmd/gremlins@%s", gremlinsVersion)
)

func goProfile() Profile {
	return Profile{
		Language:    LanguageGo,
		SkipMarkers: []*regexp.Regexp{regexp.MustCompile(`\.Skip(f|Now)?\(`)},
		runtime:     []string{"go"},
		probes: []probe{
			{tool: fmt.Sprintf("gotestsum %s", gotestsumVersion), command: fmt.Sprintf("%s --version", gotestsum)},
			{tool: fmt.Sprintf("gocover-cobertura %s", gocoverCoberturaVersion), command: fmt.Sprintf("%s -h", gocoverCobertura)},
			{tool: fmt.Sprintf("gremlins %s", gremlinsVersion), command: fmt.Sprintf("%s --version", gremlins)},
		},
		// -coverpkg lets tests in another package, such as an integration tests folder, count for the code they run;
		// -count=1 keeps the gate's second run, which looks for flaky tests, from reading the test cache
		test:          fmt.Sprintf("%s --format testname --junitfile {junit} -- -count=1 -coverpkg=./... -coverprofile={coverage} ./...", gotestsum),
		rawCoverage:   "cover.out",
		convert:       fmt.Sprintf("%s < {coverage} > {coverage_report}", gocoverCobertura),
		mutateCommand: goMutateCommand,
		mutable: func(path string) bool {
			return strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go")
		},
		reportPath: func(d Dirs) string { return filepath.Join(d.Out, "gremlins.json") },
		readReport: readGremlins,
		testID: func(_ string, c JUnitCase) string {
			return fmt.Sprintf("%s.%s", c.Classname, c.Name)
		},
		assertion: func(f Failure) bool {
			return !goCrash.MatchString(f.Body)
		},
		// a subtest's panic is written only into its top-level test's output
		parent: func(c JUnitCase) (JUnitCase, bool) {
			i := strings.LastIndex(c.Name, "/")
			if i <= 0 {
				return JUnitCase{}, false
			}
			return JUnitCase{Classname: c.Classname, Name: c.Name[:i]}, true
		},
	}
}

// goCrash is the output of a test that panicked or crashed the runtime; t.Error and testify failures are assertions.
var goCrash = regexp.MustCompile(`(?m)^(panic: |fatal error: )`)

// goMutateCommand limits gremlins to files: it takes no file list, so every
// other Go source file of the module is excluded by an exact-path pattern.
func goMutateCommand(d Dirs, files []string) (string, error) {
	keep := map[string]bool{}
	for _, f := range files {
		keep[f] = true
	}
	var cmd strings.Builder
	// --integration runs the whole suite per mutant, so tests in another package can kill it. GOFLAGS=-count=1
	// keeps the baseline run out of the test cache: gremlins sets each mutant's time limit from it, and a cached
	// baseline makes every mutant time out
	cmd.WriteString(fmt.Sprintf("GOFLAGS=-count=1 %s unleash --silent --integration --coverpkg ./... --output %s", gremlins, quote(filepath.Join(d.Out, "gremlins.json"))))
	err := filepath.WalkDir(d.Repo, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() {
			name := e.Name()
			if path != d.Repo && (strings.HasPrefix(name, ".") || name == "vendor" || name == "testdata" || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(d.Repo, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.HasSuffix(rel, ".go") && !strings.HasSuffix(rel, "_test.go") && !keep[rel] {
			cmd.WriteString(fmt.Sprintf(" -E %s", quote(fmt.Sprintf("^%s$", regexp.QuoteMeta(rel)))))
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("cannot list Go files in %s: %w", d.Repo, err)
	}
	cmd.WriteString(" .")
	return cmd.String(), nil
}

func readGremlins(path string) (Mutation, error) {
	var report struct {
		Files []struct {
			Mutations []struct {
				Status string `json:"status"`
			} `json:"mutations"`
		} `json:"files"`
	}
	if err := readJSONReport(path, &report); err != nil {
		return Mutation{}, err
	}
	var m Mutation
	for _, f := range report.Files {
		for _, mu := range f.Mutations {
			switch mu.Status {
			case "KILLED":
				m.Killed++
			case "LIVED":
				m.Survived++
			case "TIMED OUT":
				m.TimedOut++
			case "NOT COVERED":
				m.NoCoverage++
			case "NOT VIABLE":
				m.Invalid++
			default:
				m.Ignored++
			}
		}
	}
	return m, nil
}
