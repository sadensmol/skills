package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const qaTestsFile = "qa-tests.json"

// qaRolePrefix starts the role of the QA agents; only they may write QA test files.
const qaRolePrefix = "qa-"

// QATests is run/qa-tests.json: the repo-relative files that QA's tests live in.
type QATests struct {
	Files []string `json:"files"`
}

// ErrQATestsMissing means run/qa-tests.json does not exist.
var ErrQATestsMissing = errors.New("run/qa-tests.json is missing: qa-plan must write it (`{\"files\":[]}` when it has no test files)")

// LoadQATests reads the QA test files of a run, normalised like zones. A missing file is ErrQATestsMissing, so
// ownership never silently turns off. Every entry must be a clean repo-relative file path.
func LoadQATests(base string) ([]string, error) {
	path := filepath.Join(RunFolder(base), qaTestsFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrQATestsMissing
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}
	var q QATests
	if err := json.Unmarshal(data, &q); err != nil {
		return nil, fmt.Errorf("cannot parse %s: %w", path, err)
	}
	files := make([]string, 0, len(q.Files))
	for _, f := range q.Files {
		n := normalize(f)
		if !cleanFilePath(n) {
			return nil, fmt.Errorf("%s: entry %q is not a clean repo-relative file path (no directory, glob, '..', '.', empty segment or leading '/')", path, f)
		}
		files = append(files, n)
	}
	return files, nil
}

func cleanFilePath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.ContainsAny(p, "*?[") {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

// QATestsFor loads the QA test files for a plan or wave. Only a QA-only set of parts may run before the file exists.
func QATestsFor(base string, parts []Part) ([]string, error) {
	files, err := LoadQATests(base)
	if !errors.Is(err, ErrQATestsMissing) {
		return files, err
	}
	for _, p := range parts {
		if !isQARole(p.Role) {
			return nil, err
		}
	}
	return nil, nil
}

func planParts(p Plan) []Part {
	var parts []Part
	for _, w := range p.Waves {
		parts = append(parts, w.Parts...)
	}
	return parts
}

// SameFile reports whether two repo-relative paths name one file: normalised and ignoring case.
func SameFile(a, b string) bool {
	return strings.EqualFold(normalize(a), normalize(b))
}

// QAOwnershipProblems refuses every test whose file is not one of QA's test files, so an implementer's test cannot
// prove a criterion. Paths compare normalised and ignoring case (macOS file systems do).
func QAOwnershipProblems(c Criteria, qaFiles []string) []string {
	var problems []string
	for _, cr := range c.Criteria {
		for _, t := range cr.Tests {
			if t.File == "" {
				continue
			}
			if !slices.ContainsFunc(qaFiles, func(f string) bool { return SameFile(f, t.File) }) {
				problems = append(problems, fmt.Sprintf("%s: test %s file %q is not a QA test file in run/qa-tests.json", cr.ID, t.ID, t.File))
			}
		}
	}
	return problems
}

func isQARole(role string) bool {
	return strings.HasPrefix(role, qaRolePrefix)
}

// ValidatePlanQA is ValidatePlan plus the QA ownership rule: no zone of a non-QA part may cover a QA test file.
func ValidatePlanQA(p Plan, qaFiles []string) []string {
	problems := ValidatePlan(p)
	for _, wave := range p.Waves {
		for _, part := range wave.Parts {
			if isQARole(part.Role) {
				continue
			}
			for _, zone := range part.Files {
				for _, f := range qaFiles {
					if zoneAllows(strings.ToLower(normalize(f)), strings.ToLower(zone)) {
						problems = append(problems, fmt.Sprintf("part %s zone %q covers QA test file %q: only QA may write it", part.ID, zone, normalize(f)))
					}
				}
			}
		}
	}
	return problems
}

// qaEdits returns the changed files that are QA test files and that no QA part's zone allows.
func qaEdits(changed, qaFiles []string, parts []PartState) []string {
	var zones []string
	for _, p := range parts {
		if isQARole(p.Role) {
			zones = append(zones, p.Files...)
		}
	}
	var edits []string
	for _, f := range changed {
		f = normalize(f)
		if slices.ContainsFunc(qaFiles, func(q string) bool { return SameFile(q, f) }) && len(OutsideZones([]string{f}, zones)) > 0 {
			edits = append(edits, f)
		}
	}
	return edits
}
