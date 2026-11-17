package gate

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/sadensmol/autopilot/internal/domain"
	"github.com/sadensmol/autopilot/internal/gate/profile"
)

var hunkHeader = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// testCountCheck is check (f): no fewer tests run on the head than on the default branch, and the change adds no
// skip marker of the profile. Skipped testcases do not count as run.
func (r *gateRun) testCountCheck(ctx context.Context, det Detected, compare string, head junitResults) domain.CheckResult {
	baseRan, err := r.baseTestCount(ctx, compare)
	if err != nil {
		return stepResult(ctx, CheckTestCount, err)
	}
	markers, err := addedSkipMarkers(ctx, r.d.Checkout, compare, r.sha, det.SkipMarkers)
	if err != nil {
		return stepResult(ctx, CheckTestCount, err)
	}
	var problems []string
	if head.ran < baseRan {
		problems = append(problems, fmt.Sprintf("%d tests on the head, %d on %s", head.ran, baseRan, r.d.DefaultBranch))
	}
	for _, at := range markers {
		problems = append(problems, fmt.Sprintf("new skip marker at %s", at))
	}
	if len(problems) > 0 {
		return domain.CheckResult{Name: CheckTestCount, Status: domain.CheckFail, Detail: strings.Join(problems, "; ")}
	}
	return domain.CheckResult{Name: CheckTestCount, Status: domain.CheckPass,
		Detail: fmt.Sprintf("%d tests on the head, %d on %s, no new skip marker", head.ran, baseRan, r.d.DefaultBranch)}
}

// baseTestCount runs the tests of the default branch; a default branch with no test profile has no tests.
func (r *gateRun) baseTestCount(ctx context.Context, compare string) (int, error) {
	dir, err := r.trees.add(ctx, "base", compare)
	if err != nil {
		return 0, err
	}
	_, _, results, err := r.gate.testRun(ctx, profile.Dirs{Repo: dir, Out: filepath.Join(r.out, "base")}, r.d.Overrides)
	if errors.Is(err, profile.ErrNoProfile) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return results.ran, nil
}

// addedSkipMarkers lists file:line of every added line, since the merge base with compare, that holds a marker.
func addedSkipMarkers(ctx context.Context, dir, compare, sha string, markers []*regexp.Regexp) ([]string, error) {
	raw, err := gitRaw(ctx, dir, "diff", "-U0", "--no-renames", "--no-color", "--no-ext-diff", "--end-of-options", fmt.Sprintf("%s...%s", compare, sha))
	if err != nil {
		return nil, fmt.Errorf("cannot read the diff: %w", err)
	}
	var found []string
	file, line := "", 0
	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		text := scanner.Text()
		switch {
		case strings.HasPrefix(text, "+++ "):
			file = strings.TrimPrefix(strings.TrimPrefix(text, "+++ "), "b/")
		case strings.HasPrefix(text, "@@"):
			m := hunkHeader.FindStringSubmatch(text)
			if m == nil {
				return nil, fmt.Errorf("cannot read the diff hunk header %q", text)
			}
			if line, err = strconv.Atoi(m[1]); err != nil {
				return nil, fmt.Errorf("cannot read the diff hunk header %q: %w", text, err)
			}
		case strings.HasPrefix(text, "+"):
			for _, marker := range markers {
				if marker.MatchString(text[1:]) {
					found = append(found, fmt.Sprintf("%s:%d", file, line))
					break
				}
			}
			line++
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("cannot read the diff: %w", err)
	}
	return found, nil
}
