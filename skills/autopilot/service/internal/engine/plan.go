// Package engine runs the waves of a run: plan validation, wave spawn, wait and verify, zone checks and messaging.
// It is ported from herdr-orchestrator (MIT, see NOTICE) onto agterm sessions.
package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// maxPartsPerWave is how many parts may work at the same time.
const maxPartsPerWave = 5

var partIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// dirGlobPattern splits a trailing "/*", "/**", "*" or "**" off a zone pattern.
var dirGlobPattern = regexp.MustCompile(`^(.*?)/?\*{1,2}$`)

// Plan is the work of a run: waves of parts, each part with the files it may write.
type Plan struct {
	Task string `json:"task"`
	// Cwd is the git folder that the zones are relative to.
	Cwd   string `json:"cwd"`
	Waves []Wave `json:"waves"`
}

type Wave struct {
	Parts []Part `json:"parts"`
}

// Part is one unit of work that one agent session does.
type Part struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Subtask string `json:"subtask"`
	// role stays unprefixed in saved plans; the engine adds the agent namespace.
	Role string `json:"role"`
	// Files is the write zone: file paths, or directory prefixes that end in "/**".
	Files        []string `json:"files"`
	ExcludeFiles []string `json:"exclude_files,omitempty"`
}

// LoadPlan reads a plan file.
func LoadPlan(path string) (Plan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Plan{}, fmt.Errorf("cannot read plan %s: %w", path, err)
	}
	var p Plan
	if err := json.Unmarshal(data, &p); err != nil {
		return Plan{}, fmt.Errorf("cannot parse plan %s: %w", path, err)
	}
	return p, nil
}

func normalize(p string) string {
	return strings.TrimPrefix(strings.ReplaceAll(p, `\`, "/"), "./")
}

// PrefixOf returns the directory prefix of a zone pattern that ends in a glob; the empty prefix means everything.
// It reports false for a plain path.
func PrefixOf(pattern string) (string, bool) {
	n := normalize(pattern)
	if n == "*" || n == "**" {
		return "", true
	}
	m := dirGlobPattern.FindStringSubmatch(n)
	if m == nil {
		return "", false
	}
	return m[1] + "/", true
}

// Overlaps reports whether two zone patterns can match the same file. It is conservative:
// a trailing glob is a directory prefix, anything else must match exactly.
func Overlaps(a, b string) bool {
	na, nb := normalize(a), normalize(b)
	if na == nb {
		return true
	}
	pa, aGlob := PrefixOf(na)
	pb, bGlob := PrefixOf(nb)
	if aGlob && strings.HasPrefix(nb+"/", pa) {
		return true
	}
	if bGlob && strings.HasPrefix(na+"/", pb) {
		return true
	}
	return aGlob && bGlob && (strings.HasPrefix(pa, pb) || strings.HasPrefix(pb, pa))
}

// ValidatePlan returns every problem of the plan; an empty list means the plan may run.
func ValidatePlan(p Plan) []string {
	var problems []string
	if p.Task == "" {
		problems = append(problems, `missing "task"`)
	}
	if len(p.Waves) == 0 {
		problems = append(problems, `"waves" must be a non-empty array`)
	}
	seen := map[string]bool{}
	for wi, wave := range p.Waves {
		n := wi + 1
		if len(wave.Parts) == 0 {
			problems = append(problems, fmt.Sprintf("wave %d has no parts", n))
		}
		if len(wave.Parts) > maxPartsPerWave {
			problems = append(problems, fmt.Sprintf("wave %d has %d parts (max %d)", n, len(wave.Parts), maxPartsPerWave))
		}
		for _, part := range wave.Parts {
			problems = append(problems, validatePart(part, seen)...)
			seen[part.ID] = true
		}
		problems = append(problems, overlapProblems(n, wave.Parts)...)
	}
	return problems
}

func validatePart(part Part, seen map[string]bool) []string {
	var problems []string
	if !partIDPattern.MatchString(part.ID) {
		problems = append(problems, fmt.Sprintf("part id %q must match [a-z][a-z0-9-]*", part.ID))
	}
	if seen[part.ID] {
		problems = append(problems, fmt.Sprintf("duplicate part id %q", part.ID))
	}
	if part.Label == "" {
		problems = append(problems, fmt.Sprintf("part %s has no label", part.ID))
	}
	if part.Subtask == "" {
		problems = append(problems, fmt.Sprintf("part %s has no subtask", part.ID))
	}
	if len(part.Files) == 0 {
		problems = append(problems, fmt.Sprintf(`part %s has no "files" (allowed write zone)`, part.ID))
	}
	return problems
}

// overlapProblems finds two parts of one wave that may write the same file.
func overlapProblems(wave int, parts []Part) []string {
	var problems []string
	for i := range parts {
		for j := i + 1; j < len(parts); j++ {
			for _, fa := range parts[i].Files {
				for _, fb := range parts[j].Files {
					if Overlaps(fa, fb) {
						problems = append(problems, fmt.Sprintf("wave %d: %s and %s both write %q / %q - merge them or move one to another wave", wave, parts[i].ID, parts[j].ID, fa, fb))
					}
				}
			}
		}
	}
	return problems
}
