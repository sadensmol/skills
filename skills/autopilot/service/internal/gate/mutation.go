package gate

import (
	"context"
	"fmt"

	"github.com/sadensmol/autopilot/internal/domain"
	"github.com/sadensmol/autopilot/internal/gate/profile"
)

// mutationCheck is check (e): the mutation score on the changed files reaches the floor. A time box hit fails,
// whatever ran; no mutable code in the changed files, or no viable mutant, is n/a.
func mutationCheck(ctx context.Context, det Detected, dirs profile.Dirs, compare, sha string) domain.CheckResult {
	files, err := changedFiles(ctx, dirs.Repo, compare, sha)
	if err != nil {
		return stepResult(ctx, CheckMutation, err)
	}
	m, err := det.Profile.Mutation(ctx, dirs, files)
	if err != nil {
		return stepResult(ctx, CheckMutation, err)
	}
	detected := m.Killed + m.TimedOut
	scored := detected + m.Survived + m.NoCoverage
	floor := det.Floors.MutationMin
	switch {
	case m.Status == profile.StatusTimeBoxHit:
		return domain.CheckResult{Name: CheckMutation, Status: domain.CheckFail,
			Detail: fmt.Sprintf("mutation timed out after %s (%d of %d mutants killed when it stopped)", det.Floors.MutationTimeout, detected, scored)}
	case m.Status == profile.StatusNotApplicable:
		return domain.CheckResult{Name: CheckMutation, Status: domain.CheckNotApplicable, Detail: "no mutable code in the changed files"}
	case scored == 0:
		return domain.CheckResult{Name: CheckMutation, Status: domain.CheckNotApplicable, Detail: fmt.Sprintf("no viable mutant (%d invalid)", m.Invalid)}
	}
	score := 100 * float64(detected) / float64(scored)
	result := domain.CheckResult{Name: CheckMutation, Status: domain.CheckPass,
		Detail: fmt.Sprintf("score %.1f%% (%d of %d mutants killed), floor %.0f%%", score, detected, scored, floor)}
	if score < floor {
		result.Status = domain.CheckFail
		result.Detail = fmt.Sprintf("score %.1f%% (%d of %d mutants killed), below the floor of %.0f%%", score, detected, scored, floor)
	}
	return result
}

// changedFiles lists the files that the change since its merge base with compare adds or modifies.
func changedFiles(ctx context.Context, dir, compare, sha string) ([]string, error) {
	raw, err := gitRaw(ctx, dir, "diff", "--name-only", "-z", "--no-renames", "--diff-filter=d", "--end-of-options", fmt.Sprintf("%s...%s", compare, sha))
	if err != nil {
		return nil, fmt.Errorf("cannot list the changed files: %w", err)
	}
	return splitZ(raw), nil
}
