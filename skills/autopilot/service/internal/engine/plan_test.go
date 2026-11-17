package engine_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sadensmol/autopilot/internal/engine"
)

func part(id string, files ...string) engine.Part {
	return engine.Part{ID: id, Label: "label " + id, Subtask: "do " + id, Role: "implementer", Files: files}
}

func plan(parts ...engine.Part) engine.Plan {
	return engine.Plan{Task: "ship it", Waves: []engine.Wave{{Parts: parts}}}
}

func TestValidatePlan_AcceptsDisjointZones(t *testing.T) {
	require.Empty(t, engine.ValidatePlan(plan(part("part-a", "src/a.go"), part("part-b", "src/b/**"))))
}

func TestValidatePlan_RejectsMoreThanFivePerWave(t *testing.T) {
	var parts []engine.Part
	for i := range 6 {
		parts = append(parts, part(fmt.Sprintf("part-%c", 'a'+i), fmt.Sprintf("dir%d/x.go", i)))
	}
	problems := engine.ValidatePlan(plan(parts...))
	require.Len(t, problems, 1)
	require.Contains(t, problems[0], "6 parts (max 5)")
}

func TestValidatePlan_RejectsBadAndDuplicateIDs(t *testing.T) {
	problems := engine.ValidatePlan(plan(part("Bad_ID", "a"), part("ok", "b"), part("ok", "c")))
	joined := strings.Join(problems, "\n")
	require.Contains(t, joined, `part id "Bad_ID" must match`)
	require.Contains(t, joined, `duplicate part id "ok"`)
}

func TestValidatePlan_RejectsMissingZoneLabelSubtaskAndTask(t *testing.T) {
	p := engine.Plan{Waves: []engine.Wave{{Parts: []engine.Part{{ID: "a-1"}}}}}
	joined := strings.Join(engine.ValidatePlan(p), "\n")
	require.Contains(t, joined, `missing "task"`)
	require.Contains(t, joined, "part a-1 has no label")
	require.Contains(t, joined, "part a-1 has no subtask")
	require.Contains(t, joined, `part a-1 has no "files"`)
}

func TestValidatePlan_RejectsEmptyWaves(t *testing.T) {
	require.Contains(t, strings.Join(engine.ValidatePlan(engine.Plan{Task: "t"}), "\n"), `"waves" must be a non-empty array`)
	require.Contains(t, strings.Join(engine.ValidatePlan(engine.Plan{Task: "t", Waves: []engine.Wave{{}}}), "\n"), "wave 1 has no parts")
}

func TestValidatePlan_RejectsOverlappingZonesInOneWave(t *testing.T) {
	problems := engine.ValidatePlan(plan(part("part-a", "src/**"), part("part-b", "src/b/x.go")))
	require.Len(t, problems, 1)
	require.Contains(t, problems[0], "wave 1: part-a and part-b both write")
}

func TestValidatePlan_AllowsSameZoneInDifferentWaves(t *testing.T) {
	p := engine.Plan{Task: "t", Waves: []engine.Wave{{Parts: []engine.Part{part("part-a", "src/a.go")}}, {Parts: []engine.Part{part("part-b", "src/a.go")}}}}
	require.Empty(t, engine.ValidatePlan(p))
}

func TestOverlaps(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"src/a.go", "src/a.go", true},
		{"./src/a.go", "src/a.go", true},
		{"src/**", "src/a/b.go", true},
		{"src/a/b.go", "src/**", true},
		{"src/*", "src/a.go", true},
		{"src/**", "src/a/**", true},
		{"src/a/**", "src/**", true},
		{"*", "anything.go", true},
		{"**", "x/y.go", true},
		{"src/**", "lib/**", false},
		{"src/a.go", "src/b.go", false},
		{"src/**", "srcother/x.go", false},
		{`src\a.go`, "src/a.go", true},
	}
	for _, c := range cases {
		require.Equalf(t, c.want, engine.Overlaps(c.a, c.b), "%s vs %s", c.a, c.b)
	}
}

func TestPrefixOf(t *testing.T) {
	prefix, ok := engine.PrefixOf("src/**")
	require.True(t, ok)
	require.Equal(t, "src/", prefix)
	prefix, ok = engine.PrefixOf("*")
	require.True(t, ok)
	require.Equal(t, "", prefix)
	_, ok = engine.PrefixOf("src/a.go")
	require.False(t, ok)
}

func roleOf(role string, p engine.Part) engine.Part {
	p.Role = role
	return p
}

func TestValidatePlanQA_RejectsImplementerZoneOverAQATestFile(t *testing.T) {
	qa := []string{"pkg/a_test.go"}
	cases := map[string]string{
		"exact file":       "pkg/a_test.go",
		"directory glob":   "pkg/**",
		"star":             "*",
		"dotted prefix":    "./pkg/a_test.go",
		"backslash prefix": `pkg\a_test.go`,
	}
	for name, zone := range cases {
		t.Run(name, func(t *testing.T) {
			problems := engine.ValidatePlanQA(plan(part("part-a", zone)), qa)
			require.Len(t, problems, 1)
			require.Contains(t, problems[0], fmt.Sprintf("part part-a zone %q covers QA test file %q", zone, "pkg/a_test.go"))
		})
	}
}

func TestValidatePlanQA_NormalisesQAPathsLikeZones(t *testing.T) {
	problems := engine.ValidatePlanQA(plan(part("part-a", "pkg/a_test.go")), []string{`.\pkg\a_test.go`})
	require.Len(t, problems, 1)
}

func TestValidatePlanQA_AcceptsZonesThatMissQATests(t *testing.T) {
	require.Empty(t, engine.ValidatePlanQA(plan(part("part-a", "pkg/a.go", "internal/**")), []string{"pkg/a_test.go"}))
}

func TestValidatePlanQA_QARoleMayCoverQATests(t *testing.T) {
	p := plan(roleOf("qa-verify", part("part-q", "pkg/**")))
	require.Empty(t, engine.ValidatePlanQA(p, []string{"pkg/a_test.go"}))
}

func TestValidatePlanQA_StillReportsPlainPlanProblems(t *testing.T) {
	problems := engine.ValidatePlanQA(engine.Plan{}, []string{"pkg/a_test.go"})
	require.NotEmpty(t, problems)
}

func TestLoadQATests_MissingFileIsAnError(t *testing.T) {
	_, err := engine.LoadQATests(t.TempDir())
	require.ErrorIs(t, err, engine.ErrQATestsMissing)
}

func TestQATestsFor_MissingFileIsToleratedOnlyForAQAOnlyPlan(t *testing.T) {
	base := t.TempDir()
	qaOnly := plan(roleOf("qa-plan", part("part-q", "tests/**")))
	files, err := engine.QATestsFor(base, qaOnly.Waves[0].Parts)
	require.NoError(t, err)
	require.Empty(t, files)

	mixed := plan(roleOf("qa-plan", part("part-q", "tests/**")), part("part-a", "src/a.go"))
	_, err = engine.QATestsFor(base, mixed.Waves[0].Parts)
	require.ErrorIs(t, err, engine.ErrQATestsMissing)
}

func TestLoadQATests_ReadsNormalisedFiles(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, os.MkdirAll(engine.RunFolder(base), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(engine.RunFolder(base), "qa-tests.json"), []byte(`{"files":["./pkg/a_test.go","b_test.go"]}`), 0o644))
	got, err := engine.LoadQATests(base)
	require.NoError(t, err)
	require.Equal(t, []string{"pkg/a_test.go", "b_test.go"}, got)
}

func TestLoadQATests_MalformedFileIsAnError(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, os.MkdirAll(engine.RunFolder(base), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(engine.RunFolder(base), "qa-tests.json"), []byte("["), 0o644))
	_, err := engine.LoadQATests(base)
	require.Error(t, err)
}

func TestLoadQATests_RejectsEntriesThatAreNotCleanFilePaths(t *testing.T) {
	bad := []string{"tests/", "tests/**", "pkg/*_test.go", "a//b_test.go", "a/./b_test.go", "../x_test.go", "a/../b_test.go", "/abs/x_test.go", "", ".", "dir/?.go", "dir/[a].go"}
	for _, entry := range bad {
		t.Run(entry, func(t *testing.T) {
			base := t.TempDir()
			require.NoError(t, os.MkdirAll(engine.RunFolder(base), 0o755))
			body := fmt.Sprintf(`{"files":[%q]}`, entry)
			require.NoError(t, os.WriteFile(filepath.Join(engine.RunFolder(base), "qa-tests.json"), []byte(body), 0o644))
			_, err := engine.LoadQATests(base)
			require.Error(t, err)
			require.Contains(t, err.Error(), "not a clean repo-relative file path")
		})
	}
}

func TestValidatePlanQA_ComparesCaseInsensitively(t *testing.T) {
	problems := engine.ValidatePlanQA(plan(part("part-a", "Pkg/A_Test.go")), []string{"pkg/a_test.go"})
	require.Len(t, problems, 1)
}
