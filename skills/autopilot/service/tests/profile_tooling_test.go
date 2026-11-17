//go:build tooling

package tests_test

import (
	"context"
	"encoding/xml"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sadensmol/autopilot/internal/gate/profile"
)

// toolingFixture is a tiny repo: base holds the first commit on main, change the
// feature commit. changed is the covered, fully tested file the mutation run gets;
// the change also adds an untested file, so diff coverage is below 100 % and any
// mutant of that file would show as survived or not covered.
type toolingFixture struct {
	runtime []string
	base    map[string]string
	change  map[string]string
	changed string
	testIDs []string
}

var toolingFixtures = map[string]toolingFixture{
	"go": {
		runtime: []string{"go", "uvx"},
		base: map[string]string{
			"go.mod":       "module example.com/acmecalc\n\ngo 1.27\n",
			"calc.go":      "package acmecalc\n\nfunc Add(a, b int) int {\n\treturn a + b\n}\n",
			"calc_test.go": "package acmecalc\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(2, 3) != 5 {\n\t\tt.Fatal(\"add\")\n\t}\n}\n",
		},
		change: map[string]string{
			"calc.go": "package acmecalc\n\nfunc Add(a, b int) int {\n\treturn a + b\n}\n\nfunc Triple(a int) int {\n\treturn a * 3\n}\n",
			"calc_test.go": "package acmecalc\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(2, 3) != 5 {\n\t\tt.Fatal(\"add\")\n\t}\n}\n\n" +
				"func TestTriple(t *testing.T) {\n\tt.Run(\"of two\", func(t *testing.T) {\n\t\tif Triple(2) != 6 {\n\t\t\tt.Fatal(\"triple\")\n\t\t}\n\t})\n}\n",
			"other.go": "package acmecalc\n\nfunc Neg(a int) int {\n\tif a > 0 {\n\t\treturn -a\n\t}\n\treturn a\n}\n",
		},
		changed: "calc.go",
		testIDs: []string{"example.com/acmecalc.TestAdd", "example.com/acmecalc.TestTriple", "example.com/acmecalc.TestTriple/of_two"},
	},
	"python": {
		runtime: []string{"uv", "uvx"},
		base: map[string]string{
			"pyproject.toml":       "[project]\nname = \"acmecalc\"\nversion = \"0.1.0\"\nrequires-python = \">=3.10\"\n\n[tool.pytest.ini_options]\npythonpath = [\".\"]\n\n[tool.mutmut]\nsource_paths = [\"acmecalc\"]\n",
			"acmecalc/__init__.py": "",
			"acmecalc/core.py":     "def add(a, b):\n    return a + b\n",
			"tests/test_core.py":   "from acmecalc.core import add\n\n\ndef test_add():\n    assert add(2, 3) == 5\n",
		},
		change: map[string]string{
			"acmecalc/core.py":   "def add(a, b):\n    return a + b\n\n\ndef triple(a):\n    return a * 3\n",
			"acmecalc/other.py":  "def neg(a):\n    if a > 0:\n        return -a\n    return a\n",
			"tests/test_core.py": "import acmecalc.other\nfrom acmecalc.core import add, triple\n\n\ndef test_add():\n    assert add(2, 3) == 5\n\n\nclass TestTriple:\n    def test_of_two(self):\n        assert triple(2) == 6\n",
		},
		changed: "acmecalc/core.py",
		testIDs: []string{"tests.test_core.test_add", "tests.test_core.TestTriple.test_of_two"},
	},
	"node test": {
		runtime: []string{"node", "npm", "npx", "uvx"},
		base: map[string]string{
			"package.json":      "{\"name\": \"acmecalc\", \"version\": \"0.1.0\", \"type\": \"module\", \"scripts\": {\"test\": \"node --test\"}}\n",
			"src/calc.js":       "export function add(a, b) {\n  return a + b;\n}\n",
			"test/calc.test.js": "import { test } from 'node:test';\nimport assert from 'node:assert/strict';\nimport { add } from '../src/calc.js';\n\ntest('add', () => {\n  assert.equal(add(2, 3), 5);\n});\n",
		},
		change: map[string]string{
			"src/calc.js":  "export function add(a, b) {\n  return a + b;\n}\n\nexport function triple(a) {\n  return a * 3;\n}\n",
			"src/other.js": "export function neg(a) {\n  if (a > 0) {\n    return -a;\n  }\n  return a;\n}\n",
			"test/calc.test.js": "import { test, describe } from 'node:test';\nimport assert from 'node:assert/strict';\nimport { add, triple } from '../src/calc.js';\nimport '../src/other.js';\n\n" +
				"test('add', () => {\n  assert.equal(add(2, 3), 5);\n});\n\ndescribe('triple', () => {\n  test('of two', () => {\n    assert.equal(triple(2), 6);\n  });\n});\n",
		},
		changed: "src/calc.js",
		testIDs: []string{"test/calc.test.js > add", "test/calc.test.js > triple > of two"},
	},
	"node vitest": {
		runtime: []string{"node", "npm", "npx", "uvx"},
		base: map[string]string{
			"package.json":      "{\"name\": \"acmecalc\", \"version\": \"0.1.0\", \"type\": \"module\", \"devDependencies\": {\"vitest\": \"5.1.0\", \"@vitest/coverage-v8\": \"5.1.0\"}}\n",
			"src/calc.js":       "export function add(a, b) {\n  return a + b;\n}\n",
			"test/calc.test.js": "import { test, expect } from 'vitest';\nimport { add } from '../src/calc.js';\n\ntest('add', () => {\n  expect(add(2, 3)).toBe(5);\n});\n",
		},
		change: map[string]string{
			"src/calc.js":  "export function add(a, b) {\n  return a + b;\n}\n\nexport function triple(a) {\n  return a * 3;\n}\n",
			"src/other.js": "export function neg(a) {\n  if (a > 0) {\n    return -a;\n  }\n  return a;\n}\n",
			"test/calc.test.js": "import { test, describe, expect } from 'vitest';\nimport { add, triple } from '../src/calc.js';\nimport '../src/other.js';\n\n" +
				"test('add', () => {\n  expect(add(2, 3)).toBe(5);\n});\n\ndescribe('triple', () => {\n  test('of two', () => {\n    expect(triple(2)).toBe(6);\n  });\n});\n",
		},
		changed: "src/calc.js",
		testIDs: []string{"test/calc.test.js > add", "test/calc.test.js > triple > of two"},
	},
}

func TestProfileTooling(t *testing.T) {
	for name, fx := range toolingFixtures {
		t.Run(name, func(t *testing.T) {
			for _, bin := range fx.runtime {
				if _, err := exec.LookPath(bin); err != nil {
					t.Skipf("base runtime %q is not on PATH", bin)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			repo := toolingRepo(t, fx)
			dirs := profile.Dirs{Repo: repo, Out: t.TempDir()}

			p, err := profile.Usable(repo, nil)
			require.NoError(t, err)
			require.NoError(t, p.Probe(ctx, dirs))

			run, err := p.Tests(ctx, dirs)
			require.NoError(t, err)
			assert.Zero(t, run.ExitCode)
			assert.ElementsMatch(t, fx.testIDs, junitTestIDs(t, p, repo, run.JUnit))

			cover, err := p.DiffCoverage(ctx, dirs, run.Coverage, "main")
			require.NoError(t, err)
			assert.Equal(t, profile.StatusMeasured, cover.Status)
			assert.Positive(t, cover.Missing)
			assert.Greater(t, cover.Lines, cover.Missing)

			unreported, err := p.UnreportedCode(dirs, run.Coverage, []string{fx.changed})
			require.NoError(t, err)
			assert.Empty(t, unreported, "the real coverage report must hold the changed file")

			mutation, err := p.Mutation(ctx, dirs, []string{fx.changed, "README.md"})
			require.NoError(t, err)
			assert.Equal(t, profile.StatusMeasured, mutation.Status)
			assert.Positive(t, mutation.Killed)
			assert.Zero(t, mutation.Survived)
			assert.Zero(t, mutation.NoCoverage)
			t.Logf("diff coverage %+v, mutation %+v", cover, mutation)
		})
	}
}

func toolingRepo(t *testing.T, fx toolingFixture) string {
	t.Helper()
	repo := t.TempDir()
	writeTree(t, repo, fx.base)
	git(t, repo, "init", "-q", "-b", "main")
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-q", "-m", "base")
	git(t, repo, "checkout", "-q", "-b", "feature")
	writeTree(t, repo, fx.change)
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-q", "-m", "feature")
	return repo
}

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		path := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	base := []string{"-c", "user.name=autopilot-test", "-c", "user.email=autopilot-test@example.com", "-c", "commit.gpgsign=false"}
	cmd := exec.Command("git", append(base, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

type junitNode struct {
	Cases  []junitCase `xml:"testcase"`
	Suites []junitNode `xml:"testsuite"`
}

type junitCase struct {
	Classname string    `xml:"classname,attr"`
	Name      string    `xml:"name,attr"`
	File      string    `xml:"file,attr"`
	Failure   *struct{} `xml:"failure"`
	Error     *struct{} `xml:"error"`
}

func junitTestIDs(t *testing.T, p profile.Profile, repo, path string) []string {
	t.Helper()
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	var root junitNode
	require.NoError(t, xml.Unmarshal(body, &root))
	var ids []string
	var walk func(n junitNode)
	walk = func(n junitNode) {
		for _, c := range n.Cases {
			assert.Nil(t, c.Failure, c.Name)
			assert.Nil(t, c.Error, c.Name)
			ids = append(ids, p.TestID(repo, profile.JUnitCase{Classname: c.Classname, Name: c.Name, File: c.File}))
		}
		for _, s := range n.Suites {
			walk(s)
		}
	}
	walk(root)
	return ids
}
