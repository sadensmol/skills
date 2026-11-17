package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/sadensmol/autopilot/internal/gate/profile"
)

// Level is the test level of a criterion's test.
type Level string

const (
	LevelUnit        Level = "unit"
	LevelIntegration Level = "integration"
	LevelE2E         Level = "e2e"
)

// Kind says what a test proves: the happy path, an edge, or a refusal.
type Kind string

const (
	KindHappy    Kind = "happy"
	KindEdge     Kind = "edge"
	KindNegative Kind = "negative"
)

const nodeIDSeparator = " > "

// Criteria is run/criteria.json: every acceptance criterion of a run with the tests that prove it.
type Criteria struct {
	Criteria []Criterion `json:"criteria"`
}

type Criterion struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	// HasInputs is set by the QA plan when the criterion takes input; such a criterion needs an edge or negative test.
	HasInputs bool      `json:"has_inputs"`
	Tests     []TestRef `json:"tests,omitempty"`
	// Manual is what a human must check, for a criterion no automated test can prove.
	Manual string `json:"manual,omitempty"`
}

// TestRef is one test of a criterion. JUnitName is the id the gate finds in the JUnit report.
type TestRef struct {
	ID        string `json:"id"`
	Level     Level  `json:"level"`
	Kind      Kind   `json:"kind"`
	File      string `json:"file"`
	JUnitName string `json:"junit_name"`
}

// LoadCriteria reads a criteria file.
func LoadCriteria(path string) (Criteria, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Criteria{}, fmt.Errorf("cannot read criteria %s: %w", path, err)
	}
	var c Criteria
	if err := json.Unmarshal(data, &c); err != nil {
		return Criteria{}, fmt.Errorf("cannot parse criteria %s: %w", path, err)
	}
	return c, nil
}

// ValidateCriteria returns every problem of the table; an empty list means the table may drive the gate.
// Each JUnit name must have the shape the language's JUnit report gives.
func ValidateCriteria(c Criteria, language profile.Language) []string {
	if len(c.Criteria) == 0 {
		return []string{"no criteria"}
	}
	var problems []string
	criteria, tests := map[string]bool{}, map[string]bool{}
	for _, cr := range c.Criteria {
		if cr.ID == "" {
			problems = append(problems, "a criterion has no id")
		} else if criteria[cr.ID] {
			problems = append(problems, fmt.Sprintf("duplicate criterion id %q", cr.ID))
		}
		criteria[cr.ID] = true
		problems = append(problems, criterionProblems(cr, language, tests)...)
	}
	return problems
}

func criterionProblems(cr Criterion, language profile.Language, seenTests map[string]bool) []string {
	var problems []string
	if cr.Text == "" {
		problems = append(problems, fmt.Sprintf("%s: no text", cr.ID))
	}
	if len(cr.Tests) == 0 && cr.Manual == "" {
		problems = append(problems, fmt.Sprintf("%s: no test and no manual check", cr.ID))
	}
	proves := false
	for _, t := range cr.Tests {
		problems = append(problems, testProblems(cr.ID, t, language)...)
		if t.ID != "" && seenTests[t.ID] {
			problems = append(problems, fmt.Sprintf("%s: duplicate test id %q", cr.ID, t.ID))
		}
		seenTests[t.ID] = true
		proves = proves || t.Kind == KindEdge || t.Kind == KindNegative
	}
	if cr.HasInputs && !proves {
		problems = append(problems, fmt.Sprintf("%s: has inputs but no edge or negative test", cr.ID))
	}
	return problems
}

func testProblems(criterion string, t TestRef, language profile.Language) []string {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf("%s: test %s %s", criterion, t.ID, fmt.Sprintf(format, args...)))
	}
	if t.ID == "" {
		problems = append(problems, fmt.Sprintf("%s: a test has no id", criterion))
	}
	switch t.Level {
	case LevelUnit, LevelIntegration, LevelE2E:
	case "":
		add("has no level")
	default:
		add("level %q is not unit, integration or e2e", t.Level)
	}
	switch t.Kind {
	case KindHappy, KindEdge, KindNegative:
	case "":
		add("has no kind")
	default:
		add("kind %q is not happy, edge or negative", t.Kind)
	}
	if t.File == "" {
		add("has no file")
	}
	switch {
	case t.JUnitName == "":
		add("has no JUnit name")
	case !junitNameShapeOK(t.JUnitName, language):
		add("JUnit name %q is not %s", t.JUnitName, junitShape(language))
	}
	return problems
}

func junitShape(language profile.Language) string {
	if language == profile.LanguageNode {
		return `"file > suite > test"`
	}
	return `"classname.name"`
}

func junitNameShapeOK(name string, language profile.Language) bool {
	if language == profile.LanguageNode {
		parts := strings.Split(name, nodeIDSeparator)
		if len(parts) < 2 {
			return false
		}
		for _, p := range parts {
			if strings.TrimSpace(p) == "" {
				return false
			}
		}
		return true
	}
	return !strings.Contains(name, nodeIDSeparator) && strings.Contains(name, ".") &&
		!strings.HasPrefix(name, ".") && !strings.HasSuffix(name, ".")
}
