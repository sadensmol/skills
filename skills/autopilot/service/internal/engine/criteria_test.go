package engine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sadensmol/autopilot/internal/engine"
	"github.com/sadensmol/autopilot/internal/gate/profile"
)

func testRef(id string, kind engine.Kind) engine.TestRef {
	return engine.TestRef{ID: id, Level: engine.LevelUnit, Kind: kind, File: "pkg/a_test.go", JUnitName: "example.com/pkg.TestA_" + id}
}

func criterion(id string, hasInputs bool, tests ...engine.TestRef) engine.Criterion {
	return engine.Criterion{ID: id, Text: "text " + id, HasInputs: hasInputs, Tests: tests}
}

func problemsOf(c ...engine.Criterion) string {
	return strings.Join(engine.ValidateCriteria(engine.Criteria{Criteria: c}, profile.LanguageGo), "\n")
}

func TestValidateCriteria_AcceptsATestedCriterionWithInputs(t *testing.T) {
	require.Empty(t, engine.ValidateCriteria(engine.Criteria{Criteria: []engine.Criterion{
		criterion("c1", true, testRef("t1", engine.KindHappy), testRef("t2", engine.KindNegative)),
		criterion("c2", false, testRef("t3", engine.KindHappy)),
	}}, profile.LanguageGo))
}

func TestValidateCriteria_RefusesCriterionWithNoTestAndNoManual(t *testing.T) {
	require.Contains(t, problemsOf(criterion("c1", false)), "c1: no test and no manual check")
}

func TestValidateCriteria_AcceptsManualInstead(t *testing.T) {
	c := criterion("c1", false)
	c.Manual = "open the page and look at the banner"
	require.Empty(t, problemsOf(c))
}

func TestValidateCriteria_RefusesEmptyCriteriaList(t *testing.T) {
	require.Contains(t, problemsOf(), "no criteria")
}

func TestValidateCriteria_RefusesTestWithoutLevelOrKind(t *testing.T) {
	noLevel := testRef("t1", engine.KindHappy)
	noLevel.Level = ""
	noKind := testRef("t2", engine.KindHappy)
	noKind.Kind = ""
	out := problemsOf(criterion("c1", false, noLevel), criterion("c2", false, noKind))
	require.Contains(t, out, "c1: test t1 has no level")
	require.Contains(t, out, "c2: test t2 has no kind")
}

func TestValidateCriteria_RefusesUnknownLevelAndKind(t *testing.T) {
	bad := testRef("t1", engine.KindHappy)
	bad.Level = "system"
	bad.Kind = "sad"
	out := problemsOf(criterion("c1", false, bad))
	require.Contains(t, out, `c1: test t1 level "system"`)
	require.Contains(t, out, `c1: test t1 kind "sad"`)
}

func TestValidateCriteria_RefusesInputsWithoutEdgeOrNegativeTest(t *testing.T) {
	require.Contains(t, problemsOf(criterion("c1", true, testRef("t1", engine.KindHappy))), "c1: has inputs but no edge or negative test")
}

func TestValidateCriteria_EdgeAloneSatisfiesInputsRule(t *testing.T) {
	require.Empty(t, problemsOf(criterion("c1", true, testRef("t1", engine.KindHappy), testRef("t2", engine.KindEdge))))
}

func TestValidateCriteria_NoInputsNeedsNoEdgeOrNegative(t *testing.T) {
	require.Empty(t, problemsOf(criterion("c1", false, testRef("t1", engine.KindHappy))))
}

func TestValidateCriteria_RefusesDuplicateAndMissingIdentifiers(t *testing.T) {
	blank := criterion("", false, testRef("t1", engine.KindHappy))
	dupTest := criterion("c3", false, testRef("t1", engine.KindHappy))
	out := problemsOf(criterion("c1", false, testRef("t1", engine.KindHappy)), criterion("c1", false, testRef("t9", engine.KindHappy)), blank, dupTest)
	require.Contains(t, out, `duplicate criterion id "c1"`)
	require.Contains(t, out, "a criterion has no id")
	require.Contains(t, out, `duplicate test id "t1"`)
}

func TestValidateCriteria_RefusesTestWithoutFileOrJUnitName(t *testing.T) {
	bare := engine.TestRef{ID: "t1", Level: engine.LevelUnit, Kind: engine.KindHappy}
	out := problemsOf(criterion("c1", false, bare))
	require.Contains(t, out, "c1: test t1 has no file")
	require.Contains(t, out, "c1: test t1 has no JUnit name")
}

func TestValidateCriteria_JUnitNameShapePerLanguage(t *testing.T) {
	withName := func(name string) engine.Criteria {
		ref := testRef("t1", engine.KindHappy)
		ref.JUnitName = name
		return engine.Criteria{Criteria: []engine.Criterion{criterion("c1", false, ref)}}
	}
	cases := []struct {
		name     string
		language profile.Language
		junit    string
		ok       bool
	}{
		{"go classname.name", profile.LanguageGo, "example.com/pkg.TestA", true},
		{"go subtest", profile.LanguageGo, "example.com/pkg.TestA/sub_case", true},
		{"go node shape", profile.LanguageGo, "a.test.ts > suite > works", false},
		{"go bare name", profile.LanguageGo, "TestA", false},
		{"go trailing dot", profile.LanguageGo, "example.com/pkg.", false},
		{"python classname.name", profile.LanguagePython, "tests.test_a.TestA.test_x", true},
		{"python bare name", profile.LanguagePython, "test_x", false},
		{"node file suite test", profile.LanguageNode, "src/a.test.ts > suite > works", true},
		{"node file test", profile.LanguageNode, "src/a.test.ts > works", true},
		{"node dotted", profile.LanguageNode, "src.a.test.works", false},
		{"node empty segment", profile.LanguageNode, "src/a.test.ts >  > works", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := strings.Join(engine.ValidateCriteria(withName(tc.junit), tc.language), "\n")
			if tc.ok {
				require.Empty(t, out)
				return
			}
			require.Contains(t, out, "c1: test t1 JUnit name")
		})
	}
}

func TestLoadCriteria_ReadsTheFileAndHasInputsField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "criteria.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"criteria":[{"id":"c1","text":"x","has_inputs":true,"tests":[{"id":"t1","level":"unit","kind":"edge","file":"a_test.go","junit_name":"p.TestA"}]},{"id":"c2","text":"y","manual":"look"}]}`), 0o644))

	got, err := engine.LoadCriteria(path)

	require.NoError(t, err)
	require.Len(t, got.Criteria, 2)
	require.True(t, got.Criteria[0].HasInputs)
	require.Equal(t, engine.KindEdge, got.Criteria[0].Tests[0].Kind)
	require.Equal(t, "look", got.Criteria[1].Manual)
}

func TestLoadCriteria_MissingAndMalformedFilesAreErrors(t *testing.T) {
	dir := t.TempDir()
	_, err := engine.LoadCriteria(filepath.Join(dir, "none.json"))
	require.Error(t, err)
	bad := filepath.Join(dir, "bad.json")
	require.NoError(t, os.WriteFile(bad, []byte("{"), 0o644))
	_, err = engine.LoadCriteria(bad)
	require.Error(t, err)
}

func TestQAOwnershipProblems_RefusesATestFileQADoesNotOwn(t *testing.T) {
	own := testRef("t1", engine.KindHappy)
	foreign := testRef("t2", engine.KindHappy)
	foreign.File = "pkg/impl_test.go"
	c := engine.Criteria{Criteria: []engine.Criterion{criterion("c1", false, own, foreign)}}

	out := strings.Join(engine.QAOwnershipProblems(c, []string{"pkg/a_test.go"}), "\n")

	require.Contains(t, out, `c1: test t2 file "pkg/impl_test.go" is not a QA test file in run/qa-tests.json`)
	require.NotContains(t, out, "test t1")
}

func TestQAOwnershipProblems_NormalisesAndIgnoresCase(t *testing.T) {
	ref := testRef("t1", engine.KindHappy)
	ref.File = `.\Pkg\A_Test.go`
	c := engine.Criteria{Criteria: []engine.Criterion{criterion("c1", false, ref)}}
	require.Empty(t, engine.QAOwnershipProblems(c, []string{"pkg/a_test.go"}))
}
