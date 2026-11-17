package profile_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sadensmol/autopilot/internal/gate/profile"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func repoWith(t *testing.T, files ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, f := range files {
		body := ""
		if f == "package.json" {
			body = "{}"
		}
		writeFile(t, filepath.Join(dir, f), body)
	}
	return dir
}

// fakePath puts only the named fake tools on PATH; each one runs script.
func fakePath(t *testing.T, tools map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, script := range tools {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, []byte(fmt.Sprintf("#!/bin/sh\n%s\n", script)), 0o755))
	}
	t.Setenv("PATH", dir)
	return dir
}

// fakePathWithSystem is fakePath plus the system folders, for commands that need coreutils.
func fakePathWithSystem(t *testing.T, tools map[string]string) {
	t.Helper()
	dir := fakePath(t, tools)
	t.Setenv("PATH", fmt.Sprintf("%s:/bin:/usr/bin", dir))
}

func dirs(t *testing.T, repo string) profile.Dirs {
	return profile.Dirs{Repo: repo, Out: t.TempDir()}
}

func TestDetect(t *testing.T) {
	tests := map[string]struct {
		files []string
		want  profile.Language
	}{
		"go.mod":               {files: []string{"go.mod"}, want: profile.LanguageGo},
		"pyproject.toml":       {files: []string{"pyproject.toml"}, want: profile.LanguagePython},
		"setup.py":             {files: []string{"setup.py"}, want: profile.LanguagePython},
		"requirements-dev.txt": {files: []string{"requirements-dev.txt"}, want: profile.LanguagePython},
		"package.json":         {files: []string{"package.json"}, want: profile.LanguageNode},
		"go.mod beats package": {files: []string{"package.json", "go.mod"}, want: profile.LanguageGo},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			p, err := profile.Detect(repoWith(t, tc.files...), nil)
			require.NoError(t, err)
			assert.Equal(t, tc.want, p.Language)
			assert.Equal(t, profile.DefaultFloors, p.Floors)
		})
	}
}

func TestDetect_NoMarker(t *testing.T) {
	_, err := profile.Detect(repoWith(t, "README.md", "src/main.rs"), nil)
	require.ErrorIs(t, err, profile.ErrNoProfile)
}

func TestUsable(t *testing.T) {
	tests := map[string]struct {
		marker  string
		onPath  []string
		missing string
	}{
		"go ready":           {marker: "go.mod", onPath: []string{"go", "uvx"}},
		"go without go":      {marker: "go.mod", onPath: []string{"uvx"}, missing: "go"},
		"go without uvx":     {marker: "go.mod", onPath: []string{"go"}, missing: "uvx"},
		"python ready":       {marker: "pyproject.toml", onPath: []string{"uv", "uvx"}},
		"python without uv":  {marker: "pyproject.toml", onPath: []string{"uvx", "python3"}, missing: "uv"},
		"node ready":         {marker: "package.json", onPath: []string{"node", "npm", "npx", "uvx"}},
		"node without npx":   {marker: "package.json", onPath: []string{"node", "npm", "uvx"}, missing: "npx"},
		"node without node":  {marker: "package.json", onPath: []string{"npm", "npx", "uvx"}, missing: "node"},
		"python without uvx": {marker: "requirements.txt", onPath: []string{"uv"}, missing: "uvx"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tools := map[string]string{}
			for _, bin := range tc.onPath {
				tools[bin] = "exit 0"
			}
			fakePath(t, tools)
			_, err := profile.Usable(repoWith(t, tc.marker), nil)
			if tc.missing == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, profile.ErrNoProfile)
			assert.Contains(t, err.Error(), fmt.Sprintf("%q", tc.missing))
		})
	}
}

func TestUsable_NoMarker(t *testing.T) {
	fakePath(t, map[string]string{"go": "exit 0", "uvx": "exit 0"})
	_, err := profile.Usable(repoWith(t, "Makefile"), nil)
	require.ErrorIs(t, err, profile.ErrNoProfile)
}

func TestUsable_DoesNotRunTools(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran")
	fakePath(t, map[string]string{
		"go":  fmt.Sprintf("touch %s; exit 1", marker),
		"uvx": fmt.Sprintf("touch %s; exit 1", marker),
	})
	_, err := profile.Usable(repoWith(t, "go.mod"), nil)
	require.NoError(t, err)
	assert.NoFileExists(t, marker)
}

func TestDetect_Overrides(t *testing.T) {
	p, err := profile.Detect(repoWith(t, "go.mod"), map[string]string{
		"diff_coverage_min":        "90",
		"mutation_min":             "72.5",
		"mutation_timeout_minutes": "5",
	})
	require.NoError(t, err)
	assert.Equal(t, profile.Floors{DiffCoverageMin: 90, MutationMin: 72.5, MutationTimeout: 5 * time.Minute}, p.Floors)
}

func TestDetect_BadOverride(t *testing.T) {
	tests := map[string]map[string]string{
		"unknown key":            {"lint": "make lint"},
		"floor not a number":     {"diff_coverage_min": "high"},
		"floor over 100":         {"mutation_min": "101"},
		"floor below 0":          {"diff_coverage_min": "-1"},
		"floor NaN":              {"mutation_min": "NaN"},
		"floor Inf":              {"diff_coverage_min": "+Inf"},
		"timeout zero":           {"mutation_timeout_minutes": "0"},
		"timeout not integer":    {"mutation_timeout_minutes": "1.5"},
		"empty test command":     {"test": " "},
		"empty mutation command": {"mutation": ""},
	}
	for name, overrides := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := profile.Detect(repoWith(t, "go.mod"), overrides)
			require.Error(t, err)
			assert.NotErrorIs(t, err, profile.ErrNoProfile)
			for key := range overrides {
				assert.Contains(t, err.Error(), key)
			}
		})
	}
}

func TestTests_OverrideReplacesCommand(t *testing.T) {
	repo := repoWith(t, "pyproject.toml")
	p, err := profile.Detect(repo, map[string]string{
		"test": "printf '<testsuites/>' > {junit}; printf 'cov' > {coverage}",
	})
	require.NoError(t, err)
	fakePath(t, nil)

	run, err := p.Tests(context.Background(), dirs(t, repo))

	require.NoError(t, err)
	assert.Equal(t, 0, run.ExitCode)
	junit, err := os.ReadFile(run.JUnit)
	require.NoError(t, err)
	assert.Equal(t, "<testsuites/>", string(junit))
	cov, err := os.ReadFile(run.Coverage)
	require.NoError(t, err)
	assert.Equal(t, "cov", string(cov))
}

func TestTests_FailingTestsKeepTheirReport(t *testing.T) {
	repo := repoWith(t, "pyproject.toml")
	p, err := profile.Detect(repo, map[string]string{
		"test": "printf '<testsuites/>' > {junit}; exit 1",
	})
	require.NoError(t, err)
	fakePath(t, nil)

	run, err := p.Tests(context.Background(), dirs(t, repo))

	require.NoError(t, err)
	assert.Equal(t, 1, run.ExitCode)
	assert.FileExists(t, run.JUnit)
}

func TestTests_NoJUnitMeansCouldNotRun(t *testing.T) {
	repo := repoWith(t, "pyproject.toml")
	p, err := profile.Detect(repo, map[string]string{"test": "exit 2"})
	require.NoError(t, err)
	fakePath(t, nil)

	_, err = p.Tests(context.Background(), dirs(t, repo))

	require.ErrorIs(t, err, profile.ErrCouldNotRun)
}

func TestTests_NodeRunnerFromPackage(t *testing.T) {
	tests := map[string]struct {
		pkg  string
		want string
	}{
		"vitest":    {pkg: `{"devDependencies":{"vitest":"5.1.0"}}`, want: "vitest run"},
		"node test": {pkg: `{"scripts":{"test":"node --test"}}`, want: "node --test"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			repo := t.TempDir()
			writeFile(t, filepath.Join(repo, "package.json"), tc.pkg)
			args := filepath.Join(t.TempDir(), "args")
			fakePath(t, map[string]string{
				"npx": fmt.Sprintf(`echo "$@" >> %s; for a in "$@"; do case "$a" in --test-reporter-destination=*) printf x > "${a#*=}";; --outputFile.junit=*) printf x > "${a#*=}";; esac; done`, args),
				"npm": "exit 0",
			})
			p, err := profile.Detect(repo, nil)
			require.NoError(t, err)

			_, err = p.Tests(context.Background(), dirs(t, repo))

			require.NoError(t, err)
			got, err := os.ReadFile(args)
			require.NoError(t, err)
			assert.Contains(t, string(got), tc.want)
		})
	}
}

func TestProbe(t *testing.T) {
	fakePath(t, map[string]string{"go": "exit 0", "uvx": "exit 0"})
	p, err := profile.Detect(repoWith(t, "go.mod"), nil)
	require.NoError(t, err)

	require.NoError(t, p.Probe(context.Background(), dirs(t, t.TempDir())))
}

func TestProbe_FailingToolMeansCouldNotRun(t *testing.T) {
	repo := repoWith(t, "go.mod")
	fakePath(t, map[string]string{
		"go":  `case "$*" in *gremlins*) echo "download failed" >&2; exit 1;; esac`,
		"uvx": "exit 0",
	})
	p, err := profile.Detect(repo, nil)
	require.NoError(t, err)

	err = p.Probe(context.Background(), dirs(t, repo))

	require.ErrorIs(t, err, profile.ErrCouldNotRun)
	assert.Contains(t, err.Error(), "gremlins")
}

func TestDiffCoverage(t *testing.T) {
	tests := map[string]struct {
		report string
		want   profile.DiffCoverage
	}{
		"measured": {
			report: `{"total_num_lines": 4, "total_num_violations": 1, "total_percent_covered": 75, "src_stats": {"calc.go": {"covered_lines": [1, 2, 3], "violation_lines": [4]}}}`,
			want:   profile.DiffCoverage{Status: profile.StatusMeasured, Lines: 4, Missing: 1, Percent: 75},
		},
		"no changed lines": {
			report: `{"total_num_lines": 0, "total_num_violations": 0, "total_percent_covered": 100, "src_stats": {}}`,
			want:   profile.DiffCoverage{Status: profile.StatusNotApplicable},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			repo := repoWith(t, "go.mod")
			args := filepath.Join(t.TempDir(), "args")
			fakePath(t, map[string]string{
				"uvx": fmt.Sprintf(`echo "$@" > %s; for a in "$@"; do case "$a" in json:*) printf '%%s' '%s' > "${a#json:}";; esac; done`, args, tc.report),
			})
			p, err := profile.Detect(repo, nil)
			require.NoError(t, err)

			got, err := p.DiffCoverage(context.Background(), dirs(t, repo), "/cov/coverage.xml", "origin/main")

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			call, err := os.ReadFile(args)
			require.NoError(t, err)
			assert.Contains(t, string(call), "/cov/coverage.xml --compare-branch=origin/main")
		})
	}
}

func TestDiffCoverage_ToolFailsMeansCouldNotRun(t *testing.T) {
	repo := repoWith(t, "go.mod")
	fakePath(t, map[string]string{"uvx": "exit 1"})
	p, err := profile.Detect(repo, nil)
	require.NoError(t, err)

	_, err = p.DiffCoverage(context.Background(), dirs(t, repo), "/cov/coverage.xml", "origin/main")

	require.ErrorIs(t, err, profile.ErrCouldNotRun)
}

const (
	gremlinsReport = `{"files":[{"file_name":"calc.go","mutations":[
{"status":"KILLED"},{"status":"KILLED"},{"status":"LIVED"},{"status":"TIMED OUT"},
{"status":"NOT COVERED"},{"status":"NOT VIABLE"},{"status":"SKIPPED"}]}]}`
	strykerReport = `{"files":{"src/calc.js":{"mutants":[
{"status":"Killed"},{"status":"Killed"},{"status":"Survived"},{"status":"Timeout"},
{"status":"NoCoverage"},{"status":"CompileError"},{"status":"RuntimeError"},{"status":"Ignored"}]}}}`
	mutmutStats = `{"killed": 2, "survived": 1, "total": 9, "no_tests": 1, "skipped": 1,
"suspicious": 1, "timeout": 1, "check_was_interrupted_by_user": 0, "segfault": 1}`
)

func TestMutation_ReadsReport(t *testing.T) {
	tests := map[string]struct {
		marker  string
		changed string
		report  string
		tools   map[string]string
		want    profile.Mutation
	}{
		"go gremlins": {
			marker: "go.mod", changed: "calc.go", report: gremlinsReport,
			want: profile.Mutation{Status: profile.StatusMeasured, Killed: 2, Survived: 1, TimedOut: 1, NoCoverage: 1, Invalid: 1, Ignored: 1},
		},
		"node stryker": {
			marker: "package.json", changed: "src/calc.js", report: strykerReport,
			tools: map[string]string{"npm": "exit 0"},
			want:  profile.Mutation{Status: profile.StatusMeasured, Killed: 2, Survived: 1, TimedOut: 1, NoCoverage: 1, Invalid: 2, Ignored: 1},
		},
		"python mutmut": {
			marker: "pyproject.toml", changed: "calc/core.py", report: mutmutStats,
			tools: map[string]string{"uv": "exit 0"},
			want:  profile.Mutation{Status: profile.StatusMeasured, Killed: 2, Survived: 1, TimedOut: 1, NoCoverage: 1, Invalid: 2, Ignored: 2},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			repo := repoWith(t, tc.marker)
			fixture := filepath.Join(t.TempDir(), "report.json")
			writeFile(t, fixture, tc.report)
			fakePathWithSystem(t, tc.tools)
			p, err := profile.Detect(repo, map[string]string{
				"mutation": fmt.Sprintf("mkdir -p \"$(dirname {report})\" && cp %s {report}", fixture),
			})
			require.NoError(t, err)

			got, err := p.Mutation(context.Background(), dirs(t, repo), []string{tc.changed})

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestMutation_GetsOnlyChangedFiles(t *testing.T) {
	repo := repoWith(t, "go.mod")
	writeFile(t, filepath.Join(repo, "calc.go"), "package calc\n")
	writeFile(t, filepath.Join(repo, "other.go"), "package calc\n")
	writeFile(t, filepath.Join(repo, "sub", "deep.go"), "package sub\n")
	args := filepath.Join(t.TempDir(), "args")
	fakePath(t, map[string]string{"go": fmt.Sprintf(`echo "$@" > %s`, args)})
	p, err := profile.Detect(repo, nil)
	require.NoError(t, err)

	_, err = p.Mutation(context.Background(), dirs(t, repo), []string{"calc.go", "README.md"})

	require.NoError(t, err)
	call, err := os.ReadFile(args)
	require.NoError(t, err)
	assert.Contains(t, string(call), `other\.go`)
	assert.Contains(t, string(call), `sub/deep\.go`)
	assert.NotContains(t, string(call), `calc\.go`)
}

func TestMutation_KeepsChangedFilesGivenInAnyPathForm(t *testing.T) {
	repo := repoWith(t, "go.mod")
	writeFile(t, filepath.Join(repo, "calc.go"), "package calc\n")
	writeFile(t, filepath.Join(repo, "sub", "deep.go"), "package sub\n")
	writeFile(t, filepath.Join(repo, "other.go"), "package calc\n")
	args := filepath.Join(t.TempDir(), "args")
	fakePath(t, map[string]string{"go": fmt.Sprintf(`echo "$@" > %s`, args)})
	p, err := profile.Detect(repo, nil)
	require.NoError(t, err)

	_, err = p.Mutation(context.Background(), dirs(t, repo), []string{"./calc.go", filepath.Join(repo, "sub", "deep.go")})

	require.NoError(t, err)
	call, err := os.ReadFile(args)
	require.NoError(t, err)
	assert.Contains(t, string(call), `other\.go`)
	assert.NotContains(t, string(call), `calc\.go`)
	assert.NotContains(t, string(call), `deep\.go`)
}

func TestMutation_FileNamedLikeAnOptionStaysAFile(t *testing.T) {
	t.Run("mutmut", func(t *testing.T) {
		repo := repoWith(t, "pyproject.toml")
		args := filepath.Join(t.TempDir(), "args")
		fakePath(t, map[string]string{"uv": fmt.Sprintf(`echo "$@" >> %s`, args)})
		p, err := profile.Detect(repo, nil)
		require.NoError(t, err)

		_, err = p.Mutation(context.Background(), dirs(t, repo), []string{"-x.py"})

		require.NoError(t, err)
		call, err := os.ReadFile(args)
		require.NoError(t, err)
		assert.Contains(t, string(call), "mutmut run -- -x.x*")
	})
	t.Run("override", func(t *testing.T) {
		repo := repoWith(t, "go.mod")
		out := filepath.Join(t.TempDir(), "files")
		fakePathWithSystem(t, nil)
		p, err := profile.Detect(repo, map[string]string{"mutation": fmt.Sprintf("printf '%%s\\n' {files} > %s", out)})
		require.NoError(t, err)

		_, err = p.Mutation(context.Background(), dirs(t, repo), []string{"-x.go", "calc.go"})

		require.NoError(t, err)
		got, err := os.ReadFile(out)
		require.NoError(t, err)
		assert.Equal(t, "./-x.go\ncalc.go\n", string(got))
	})
}

func TestMutation_NothingMutableIsNotApplicable(t *testing.T) {
	tests := map[string]struct {
		marker  string
		changed []string
	}{
		"go tests and docs":     {marker: "go.mod", changed: []string{"calc_test.go", "README.md"}},
		"python tests and data": {marker: "pyproject.toml", changed: []string{"tests/test_core.py", "data.json"}},
		"node tests and config": {marker: "package.json", changed: []string{"src/calc.test.ts", "tsconfig.json"}},
		"no changed files":      {marker: "go.mod"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			repo := repoWith(t, tc.marker)
			fakePath(t, nil)
			p, err := profile.Detect(repo, nil)
			require.NoError(t, err)

			got, err := p.Mutation(context.Background(), dirs(t, repo), tc.changed)

			require.NoError(t, err)
			assert.Equal(t, profile.Mutation{Status: profile.StatusNotApplicable}, got)
		})
	}
}

func TestMutation_NoMutantsIsNotApplicable(t *testing.T) {
	repo := repoWith(t, "go.mod")
	fakePath(t, nil)
	p, err := profile.Detect(repo, map[string]string{"mutation": "true"})
	require.NoError(t, err)

	got, err := p.Mutation(context.Background(), dirs(t, repo), []string{"calc.go"})

	require.NoError(t, err)
	assert.Equal(t, profile.Mutation{Status: profile.StatusNotApplicable}, got)
}

func TestMutation_TimeBoxHit(t *testing.T) {
	repo := repoWith(t, "go.mod")
	t.Setenv("PATH", "/bin:/usr/bin")
	p, err := profile.Detect(repo, map[string]string{"mutation": "sleep 30"})
	require.NoError(t, err)
	p.Floors.MutationTimeout = 200 * time.Millisecond

	start := time.Now()
	got, err := p.Mutation(context.Background(), dirs(t, repo), []string{"calc.go"})

	require.NoError(t, err)
	assert.Equal(t, profile.Mutation{Status: profile.StatusTimeBoxHit}, got)
	assert.Less(t, time.Since(start), 10*time.Second)
}

func TestMutation_ToolFailsMeansCouldNotRun(t *testing.T) {
	repo := repoWith(t, "go.mod")
	fakePath(t, nil)
	p, err := profile.Detect(repo, map[string]string{"mutation": "exit 1"})
	require.NoError(t, err)

	_, err = p.Mutation(context.Background(), dirs(t, repo), []string{"calc.go"})

	require.ErrorIs(t, err, profile.ErrCouldNotRun)
}

func TestTestID(t *testing.T) {
	const repo = "/work/acme-api"
	tests := map[string]struct {
		marker string
		c      profile.JUnitCase
		want   string
	}{
		"go subtest": {
			marker: "go.mod",
			c:      profile.JUnitCase{Classname: "example.com/calc", Name: "TestMax/first"},
			want:   "example.com/calc.TestMax/first",
		},
		"python class method": {
			marker: "pyproject.toml",
			c:      profile.JUnitCase{Classname: "tests.test_core.TestBiggest", Name: "test_first[2-1]"},
			want:   "tests.test_core.TestBiggest.test_first[2-1]",
		},
		"vitest": {
			marker: "package.json",
			c:      profile.JUnitCase{Classname: "test/calc.test.js", Name: "biggest > first"},
			want:   "test/calc.test.js > biggest > first",
		},
		"node test in a suite": {
			marker: "package.json",
			c:      profile.JUnitCase{Classname: "biggest", Name: "first", File: "/work/acme-api/test/calc.test.js"},
			want:   "test/calc.test.js > biggest > first",
		},
		"node test at top level": {
			marker: "package.json",
			c:      profile.JUnitCase{Classname: "test", Name: "add", File: "/work/acme-api/test/calc.test.js"},
			want:   "test/calc.test.js > add",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			p, err := profile.Detect(repoWith(t, tc.marker), nil)
			require.NoError(t, err)
			assert.Equal(t, tc.want, p.TestID(repo, tc.c))
		})
	}
}

func TestSkipMarkers(t *testing.T) {
	tests := map[string]struct {
		marker string
		skip   []string
		keep   []string
	}{
		"go": {
			marker: "go.mod",
			skip:   []string{`	t.Skip("later")`, `t.Skipf("no %s", x)`, `t.SkipNow()`},
			keep:   []string{`skipped := 0`, `t.Run("skip", f)`},
		},
		"python": {
			marker: "pyproject.toml",
			skip: []string{`@pytest.mark.skip(reason="x")`, `@pytest.mark.skipif(True, reason="x")`,
				`@pytest.mark.xfail`, `    pytest.skip("x")`, `@unittest.skip("x")`,
				`pytestmark = pytest.mark.skip(reason="x")`, `pytestmark = [pytest.mark.skipif(True, reason="x")]`,
				`    pytest.param(1, marks=pytest.mark.xfail),`, `@mark.skip`, `np = pytest.importorskip("numpy")`,
				`@skip("x")`},
			keep: []string{`def skip_header(lines):`, `assert skipped == 1`, `@pytest.mark.parametrize("a", [1])`},
		},
		"node": {
			marker: "package.json",
			skip: []string{`it.skip('x', () => {})`, `describe.skip('x', () => {})`, `test.todo('x')`,
				`xit('x', () => {})`, `xdescribe('x', () => {})`, `test('x', { skip: true }, () => {})`, `it.only('x', f)`,
				`it.skipIf(isCI)('x', f)`, `describe.skipIf(isCI)('x', f)`, `test.runIf(isCI)('x', f)`,
				`it.concurrent.skip('x', f)`, `test.sequential.only('x', f)`, `    this.skip();`, `  ctx.skip()`,
				`test.fixme('x', f)`},
			keep: []string{`const skipped = 0`, `it('skips nothing', f)`, `it.concurrent('x', f)`, `const skipIfs = 1`},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			p, err := profile.Detect(repoWith(t, tc.marker), nil)
			require.NoError(t, err)
			for _, line := range tc.skip {
				assert.True(t, matchesAny(p, line), line)
			}
			for _, line := range tc.keep {
				assert.False(t, matchesAny(p, line), line)
			}
		})
	}
}

func matchesAny(p profile.Profile, line string) bool {
	for _, re := range p.SkipMarkers {
		if re.MatchString(line) {
			return true
		}
	}
	return false
}

// failures of the real tools: gotestsum 1.13.0, pytest 9.2.0, the Node test runner's junit reporter, vitest's junit reporter.
func TestAssertionFailure(t *testing.T) {
	nodeBody := func(cause string) string {
		return fmt.Sprintf("\n[Error [ERR_TEST_FAILURE]: x] {\n  code: 'ERR_TEST_FAILURE',\n  failureType: 'testCodeFailure',\n  cause: %s\n}", cause)
	}
	tests := map[string]struct {
		marker  string
		failure profile.Failure
		want    bool
	}{
		"go t.Errorf":                {"go.mod", profile.Failure{Message: "Failed", Body: "=== RUN   TestAssert\n    x_test.go:5: got 2, want 3\n--- FAIL: TestAssert (0.00s)\n"}, true},
		"go panic":                   {"go.mod", profile.Failure{Message: "Failed", Body: "=== RUN   TestPanic\n--- FAIL: TestPanic (0.00s)\npanic: assignment to entry in nil map [recovered, repanicked]\n\ngoroutine 4 [running]:\n"}, false},
		"python assert":              {"pyproject.toml", profile.Failure{Message: "assert (1 + 1) == 3"}, true},
		"python unittest assertion":  {"pyproject.toml", profile.Failure{Message: "AssertionError: 1 != 2"}, true},
		"python pytest.fail":         {"pyproject.toml", profile.Failure{Message: "Failed: deliberate"}, true},
		"python AttributeError":      {"pyproject.toml", profile.Failure{Message: "AttributeError: module 'os' has no attribute 'nothere'"}, false},
		"python NameError":           {"pyproject.toml", profile.Failure{Message: "NameError: name 'undefined_fn' is not defined"}, false},
		"python TypeError":           {"pyproject.toml", profile.Failure{Message: "TypeError: object of type 'int' has no len()"}, false},
		"python ModuleNotFoundError": {"pyproject.toml", profile.Failure{Message: "ModuleNotFoundError: No module named 'calc_missing'"}, false},
		"python NotImplementedError": {"pyproject.toml", profile.Failure{Message: "NotImplementedError: x"}, false},
		"node assert": {"package.json", profile.Failure{Type: "testCodeFailure", Message: "Expected values to be strictly equal:\n\n2 !== 3",
			Body: nodeBody("AssertionError [ERR_ASSERTION]: Expected values to be strictly equal:")}, true},
		"node not a function": {"package.json", profile.Failure{Type: "testCodeFailure", Message: "mod.add is not a function",
			Body: nodeBody("TypeError [Error]: mod.add is not a function")}, false},
		"node not defined": {"package.json", profile.Failure{Type: "testCodeFailure", Message: "undefinedFn is not defined",
			Body: nodeBody("ReferenceError [Error]: undefinedFn is not defined")}, false},
		"node thrown Error":      {"package.json", profile.Failure{Type: "testCodeFailure", Message: "not implemented", Body: nodeBody("Error: not implemented")}, false},
		"vitest expect":          {"package.json", profile.Failure{Type: "AssertionError", Message: "expected 2 to be 3 // Object.is equality"}, true},
		"vitest TypeError":       {"package.json", profile.Failure{Type: "TypeError", Message: "add is not a function"}, false},
		"vitest expect to throw": {"package.json", profile.Failure{Type: "AssertionError", Message: "expected [Function] to throw error matching TypeError"}, true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			p, err := profile.Detect(repoWith(t, tc.marker), nil)
			require.NoError(t, err)
			assert.Equal(t, tc.want, p.AssertionFailure(tc.failure))
		})
	}
}

func TestAssertionFailure_OverrideAddsAPattern(t *testing.T) {
	p, err := profile.Detect(repoWith(t, "package.json"), map[string]string{"assertion_pattern": `^ExpectationFailed\b`})
	require.NoError(t, err)
	assert.True(t, p.AssertionFailure(profile.Failure{Message: "ExpectationFailed: want 3"}))
	assert.True(t, p.AssertionFailure(profile.Failure{Type: "AssertionError", Message: "expected 2 to be 3"}))
	assert.False(t, p.AssertionFailure(profile.Failure{Type: "TypeError", Message: "add is not a function"}))

	_, err = profile.Detect(repoWith(t, "package.json"), map[string]string{"assertion_pattern": `(`})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "assertion_pattern")
}

func TestUnreportedCode(t *testing.T) {
	repo := t.TempDir()
	real, err := filepath.EvalSymlinks(repo)
	require.NoError(t, err)
	tests := map[string]struct {
		marker string
		report string
		files  []string
		want   []string
	}{
		"python cobertura relative to a source folder": {
			marker: "pyproject.toml",
			report: fmt.Sprintf(`<?xml version="1.0" ?><coverage><sources><source>%s/src</source></sources><packages><package><classes><class filename="pkg/calc.py"></class></classes></package></packages></coverage>`, repo),
			files:  []string{"src/pkg/calc.py", "src/pkg/orphan.py", "tests/test_calc.py", "README.md"},
			want:   []string{"src/pkg/orphan.py"},
		},
		"go cobertura from the real path": {
			marker: "go.mod",
			report: fmt.Sprintf(`<coverage><sources><source>%s</source></sources><packages><package><classes><class filename="add.go"></class><class filename="sub/lonely.go"></class></classes></package></packages></coverage>`, real),
			files:  []string{"add.go", "./sub/lonely.go", "sub/new.go", "add_test.go"},
			want:   []string{"sub/new.go"},
		},
		"node lcov": {
			marker: "package.json",
			report: fmt.Sprintf("TN:\nSF:%s/src/a.js\nDA:1,1\nend_of_record\nSF:src/c.ts\nend_of_record\n", real),
			files:  []string{"src/a.js", "src/b.ts", "src/c.ts", "src/a.test.js"},
			want:   []string{"src/b.ts"},
		},
		"everything reported": {
			marker: "go.mod",
			report: `<coverage><packages><package><classes><class filename="add.go"></class></classes></package></packages></coverage>`,
			files:  []string{"add.go"},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			writeFile(t, filepath.Join(repo, tc.marker), "{}")
			defer os.Remove(filepath.Join(repo, tc.marker)) //nolint:errcheck // the next case writes its own marker
			p, err := profile.Detect(repo, nil)
			require.NoError(t, err)
			report := filepath.Join(t.TempDir(), "coverage")
			writeFile(t, report, tc.report)

			got, err := p.UnreportedCode(profile.Dirs{Repo: repo, Out: t.TempDir()}, report, tc.files)

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestUnreportedCode_UnreadableReportIsAnError(t *testing.T) {
	repo := repoWith(t, "go.mod")
	p, err := profile.Detect(repo, nil)
	require.NoError(t, err)

	_, err = p.UnreportedCode(profile.Dirs{Repo: repo, Out: t.TempDir()}, filepath.Join(t.TempDir(), "missing.xml"), []string{"a.go"})

	require.Error(t, err)
}

// the shape of a real diff-cover 11.0.2 report on a pytest --cov=. run: the change adds pkg/calc.py, 4 of 6
// lines uncovered, and a large test file that diff-cover also measures.
func TestDiffCoverage_LeavesTestFilesOut(t *testing.T) {
	var covered strings.Builder
	for i := 1; i <= 124; i++ {
		if i > 1 {
			covered.WriteString(",")
		}
		fmt.Fprintf(&covered, "%d", i)
	}
	report := fmt.Sprintf(`{"total_num_lines": 130, "total_num_violations": 4, "total_percent_covered": 96, "src_stats": {
		"pkg/calc.py": {"percent_covered": 33.3, "covered_lines": [1, 2], "violation_lines": [3, 4, 5, 6]},
		"tests/test_calc.py": {"percent_covered": 100.0, "covered_lines": [%s], "violation_lines": []}}}`, covered.String())
	repo := repoWith(t, "pyproject.toml")
	fakePath(t, map[string]string{
		"uvx": fmt.Sprintf(`for a in "$@"; do case "$a" in json:*) printf '%%s' '%s' > "${a#json:}";; esac; done`, report),
	})
	p, err := profile.Detect(repo, nil)
	require.NoError(t, err)

	got, err := p.DiffCoverage(context.Background(), dirs(t, repo), "/cov/coverage.xml", "origin/main")

	require.NoError(t, err)
	assert.Equal(t, 6, got.Lines)
	assert.Equal(t, 4, got.Missing)
	assert.InDelta(t, 33.3, got.Percent, 0.1)
}

func TestDiffCoverage_OnlyTestFilesChangedIsNotApplicable(t *testing.T) {
	report := `{"total_num_lines": 3, "total_num_violations": 0, "total_percent_covered": 100, "src_stats": {
		"tests/test_calc.py": {"percent_covered": 100.0, "covered_lines": [1, 2, 3], "violation_lines": []}}}`
	repo := repoWith(t, "pyproject.toml")
	fakePath(t, map[string]string{
		"uvx": fmt.Sprintf(`for a in "$@"; do case "$a" in json:*) printf '%%s' '%s' > "${a#json:}";; esac; done`, report),
	})
	p, err := profile.Detect(repo, nil)
	require.NoError(t, err)

	got, err := p.DiffCoverage(context.Background(), dirs(t, repo), "/cov/coverage.xml", "origin/main")

	require.NoError(t, err)
	assert.Equal(t, profile.StatusNotApplicable, got.Status)
}

func TestDiffCoverage_ReportWithoutFileStatsCouldNotRun(t *testing.T) {
	repo := repoWith(t, "go.mod")
	fakePath(t, map[string]string{
		"uvx": `for a in "$@"; do case "$a" in json:*) printf '%s' '{"total_num_lines": 4}' > "${a#json:}";; esac; done`,
	})
	p, err := profile.Detect(repo, nil)
	require.NoError(t, err)

	_, err = p.DiffCoverage(context.Background(), dirs(t, repo), "/cov/coverage.xml", "origin/main")

	require.ErrorIs(t, err, profile.ErrCouldNotRun)
}
