package gate

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/sadensmol/autopilot/internal/domain"
	"github.com/sadensmol/autopilot/internal/gate/profile"
)

// diffCoverageCheck is check (d): the coverage of the code lines changed since compare reaches the floor. A changed
// code file that the coverage report does not hold, such as a module no test loads, counts as 0 % covered. It is
// n/a only when no changed code line exists.
func diffCoverageCheck(ctx context.Context, det Detected, dirs profile.Dirs, coverage, compare, sha string) domain.CheckResult {
	dc, err := det.Profile.DiffCoverage(ctx, dirs, coverage, compare)
	if err != nil {
		return stepResult(ctx, CheckDiffCoverage, err)
	}
	files, err := changedFiles(ctx, dirs.Repo, compare, sha)
	if err != nil {
		return stepResult(ctx, CheckDiffCoverage, err)
	}
	unreported, err := det.Profile.UnreportedCode(dirs, coverage, files)
	if err != nil {
		return stepResult(ctx, CheckDiffCoverage, &profile.CouldNotRunError{Step: "coverage report files", Err: err})
	}
	extra, err := addedLines(ctx, dirs.Repo, compare, sha, unreported)
	if err != nil {
		return stepResult(ctx, CheckDiffCoverage, err)
	}
	total, covered := dc.Lines+extra, dc.Lines-dc.Missing
	floor := det.Floors.DiffCoverageMin
	if total == 0 {
		return domain.CheckResult{Name: CheckDiffCoverage, Status: domain.CheckNotApplicable, Detail: "no changed code line"}
	}
	percent := 100 * float64(covered) / float64(total)
	var detail strings.Builder
	fmt.Fprintf(&detail, "%.1f%% of %d changed lines covered (%d missing)", percent, total, total-covered)
	if len(unreported) > 0 {
		fmt.Fprintf(&detail, ", %d of them in files the coverage report does not hold: %s", extra, strings.Join(unreported, ", "))
	}
	if percent >= floor {
		fmt.Fprintf(&detail, ", floor %.0f%%", floor)
		return domain.CheckResult{Name: CheckDiffCoverage, Status: domain.CheckPass, Detail: detail.String()}
	}
	fmt.Fprintf(&detail, ", below the floor of %.0f%%", floor)
	return domain.CheckResult{Name: CheckDiffCoverage, Status: domain.CheckFail, Detail: detail.String()}
}

// addedLines counts the lines the change adds to files since the merge base with compare.
func addedLines(ctx context.Context, dir, compare, sha string, files []string) (int, error) {
	if len(files) == 0 {
		return 0, nil
	}
	args := append([]string{"diff", "--numstat", "-z", "--no-renames", "--end-of-options", fmt.Sprintf("%s...%s", compare, sha), "--"}, files...)
	raw, err := gitRaw(ctx, dir, args...)
	if err != nil {
		return 0, fmt.Errorf("cannot count the changed lines: %w", err)
	}
	total := 0
	for _, entry := range splitZ(raw) {
		added, _, _ := strings.Cut(entry, "\t")
		if added == "-" {
			continue
		}
		n, err := strconv.Atoi(added)
		if err != nil {
			return 0, fmt.Errorf("cannot read the line count %q: %w", entry, err)
		}
		total += n
	}
	return total, nil
}
