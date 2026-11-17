package gate

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sadensmol/autopilot/internal/domain"
	"github.com/sadensmol/autopilot/internal/engine"
	"github.com/sadensmol/autopilot/internal/gate/profile"
)

// RedData is what one red proof needs. SHA is the commit that adds QA's tests; its first parent is the red base.
// Previous is the repo's stored proof when this is a re-proof: it keeps that proof's red base.
type RedData struct {
	Checkout  string
	SHA       string
	Previous  domain.RedBase
	RunBase   string
	Overrides map[string]string
	Out       string
}

// RedResult is a new proof, without the repo name, and for a re-proof the QA test files it changed.
type RedResult struct {
	Proof          domain.RedBase
	ChangedQAFiles []string
}

// Red applies only QA's test files of SHA onto the red base and runs the tests there. Every criterion test in
// those files must fail by assertion: an error, a skip, a pass or a missing testcase is ErrNotRed. A re-proof must
// keep every QA test file, red test and criterion test of the previous proof.
func (g *Gate) Red(ctx context.Context, d RedData) (RedResult, error) {
	proof, changed, err := g.red(ctx, d)
	return RedResult{Proof: proof, ChangedQAFiles: changed}, err
}

func (g *Gate) red(ctx context.Context, d RedData) (domain.RedBase, []string, error) {
	sha, err := refuseUnlessAt(ctx, d.Checkout, d.SHA)
	if err != nil {
		return domain.RedBase{}, nil, err
	}
	criteria, qaFiles, err := loadRunTests(d.RunBase)
	if err != nil {
		return domain.RedBase{}, nil, fmt.Errorf("%w: %w", ErrRefused, err)
	}
	if problems := engine.QAOwnershipProblems(criteria, qaFiles); len(problems) > 0 {
		return domain.RedBase{}, nil, fmt.Errorf("%w: run/criteria.json:\n  - %s", ErrRefused, strings.Join(problems, "\n  - "))
	}
	tracked, err := trackedFiles(ctx, d.Checkout, sha)
	if err != nil {
		return domain.RedBase{}, nil, fmt.Errorf("cannot list the files of %s: %w", sha, err)
	}
	files, tests := redTests(criteria, qaFiles, tracked)
	ids := junitNames(tests)
	if len(ids) == 0 {
		return domain.RedBase{}, nil, fmt.Errorf("%w: no criterion test of run/criteria.json is in a QA test file of %s at %s", ErrRefused, d.Checkout, sha)
	}
	if problems := droppedFromProof(d.Previous, files, tests); len(problems) > 0 {
		return domain.RedBase{}, nil, fmt.Errorf("%w: a re-proof keeps every test of the proof at %s:\n  - %s", ErrRefused, d.Previous.TestsSHA, strings.Join(problems, "\n  - "))
	}
	parent, err := redBase(ctx, d.Checkout, sha, d.Previous.BaseSHA)
	if err != nil {
		return domain.RedBase{}, nil, err
	}
	var changed []string
	if d.Previous.TestsSHA != "" {
		if changed, err = changedQAFiles(ctx, d.Checkout, d.Previous.TestsSHA, sha, union(d.Previous.QAFiles, files)); err != nil {
			return domain.RedBase{}, nil, err
		}
	}
	out, err := emptyFolder(d.Out)
	if err != nil {
		return domain.RedBase{}, nil, err
	}

	boxCtx, cancel := context.WithTimeout(ctx, g.d.Timeout)
	defer cancel()
	trees, err := newScratch(d.Checkout)
	if err != nil {
		return domain.RedBase{}, nil, err
	}
	defer trees.remove(ctx)
	det, results, err := g.redRun(boxCtx, trees, redRunData{parent: parent, sha: sha, files: files, overrides: d.Overrides, out: out})
	switch {
	case ctx.Err() != nil:
		return domain.RedBase{}, nil, fmt.Errorf("red proof of %s stopped: %w", sha, ctx.Err())
	case err != nil && boxCtx.Err() != nil:
		return domain.RedBase{}, nil, fmt.Errorf("red proof of %s hit its time box of %s: %w", sha, g.d.Timeout, err)
	case err != nil:
		return domain.RedBase{}, nil, err
	}
	if problems := engine.ValidateCriteria(criteria, det.Language); len(problems) > 0 {
		return domain.RedBase{}, nil, fmt.Errorf("%w: run/criteria.json:\n  - %s", ErrRefused, strings.Join(problems, "\n  - "))
	}
	if problems := notRed(ids, results); len(problems) > 0 {
		return domain.RedBase{}, nil, fmt.Errorf("%w for %s on base %s:\n  - %s", ErrNotRed, sha, parent, strings.Join(problems, "\n  - "))
	}
	return domain.RedBase{BaseSHA: parent, TestsSHA: sha, TestIDs: ids, QAFiles: files, CriteriaTests: tests}, changed, nil
}

// droppedFromProof lists the QA test files and criterion tests of the previous proof that a re-proof no longer has.
// The red ids are those tests' JUnit names, so keeping every criterion test also keeps every red id.
func droppedFromProof(previous domain.RedBase, files []string, tests []domain.CriterionTest) []string {
	var problems []string
	for _, old := range previous.QAFiles {
		if !slices.ContainsFunc(files, func(f string) bool { return engine.SameFile(f, old) }) {
			problems = append(problems, fmt.Sprintf("QA test file %s is dropped", old))
		}
	}
	for _, old := range previous.CriteriaTests {
		if !slices.ContainsFunc(tests, func(t domain.CriterionTest) bool { return sameCriterionTest(t, old) }) {
			problems = append(problems, fmt.Sprintf("criterion test %s (%s in %s) is dropped or changed", old.ID, old.JUnitName, old.File))
		}
	}
	return problems
}

func sameCriterionTest(a, b domain.CriterionTest) bool {
	return a.ID == b.ID && a.JUnitName == b.JUnitName && engine.SameFile(a.File, b.File)
}

func junitNames(tests []domain.CriterionTest) []string {
	var ids []string
	for _, t := range tests {
		if !slices.Contains(ids, t.JUnitName) {
			ids = append(ids, t.JUnitName)
		}
	}
	return ids
}

func union(a, b []string) []string {
	out := append([]string(nil), a...)
	for _, f := range b {
		if !slices.ContainsFunc(out, func(o string) bool { return strings.EqualFold(o, f) }) {
			out = append(out, f)
		}
	}
	return out
}

// redBase is base when an earlier proof set it, which must be an ancestor of sha, else the first parent of sha.
func redBase(ctx context.Context, checkout, sha, base string) (string, error) {
	if base == "" {
		parent, err := git(ctx, checkout, "rev-parse", "--verify", "--quiet", "--end-of-options", fmt.Sprintf("%s^1", sha))
		if err != nil {
			return "", fmt.Errorf("%w: %s has no parent to be the red base: %w", ErrRefused, sha, err)
		}
		return parent, nil
	}
	if _, err := git(ctx, checkout, "merge-base", "--is-ancestor", "--end-of-options", base, sha); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return "", fmt.Errorf("%w: the red base %s is not an ancestor of %s", ErrRefused, base, sha)
		}
		return "", fmt.Errorf("cannot compare the red base %s with %s: %w", base, sha, err)
	}
	return base, nil
}

type redRunData struct {
	parent    string
	sha       string
	files     []string
	overrides map[string]string
	out       string
}

func (g *Gate) redRun(ctx context.Context, trees *scratch, d redRunData) (Detected, junitResults, error) {
	dir, err := trees.add(ctx, "red", d.parent)
	if err != nil {
		return Detected{}, junitResults{}, err
	}
	args := append([]string{"checkout", "--quiet", d.sha, "--"}, d.files...)
	if _, err := git(ctx, dir, args...); err != nil {
		return Detected{}, junitResults{}, &profile.CouldNotRunError{Step: "apply QA's test files", Err: err}
	}
	det, _, results, err := g.testRun(ctx, profile.Dirs{Repo: dir, Out: filepath.Join(d.out, "red")}, d.overrides)
	return det, results, err
}

// redTests picks QA's test files that the commit holds, with their real spelling, and the criterion tests in them.
func redTests(criteria engine.Criteria, qaFiles, tracked []string) ([]string, []domain.CriterionTest) {
	var files []string
	for _, f := range qaFiles {
		if real, ok := trackedPath(tracked, f); ok {
			files = append(files, real)
		}
	}
	var tests []domain.CriterionTest
	for _, cr := range criteria.Criteria {
		for _, t := range cr.Tests {
			if real, ok := trackedPath(files, t.File); ok {
				tests = append(tests, domain.CriterionTest{ID: t.ID, JUnitName: t.JUnitName, File: real})
			}
		}
	}
	return files, tests
}

func notRed(ids []string, results junitResults) []string {
	var problems []string
	for _, id := range ids {
		o, ok := results.outcomeOf(id)
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("%s is missing from the JUnit report", id))
		case o == outcomePassed:
			problems = append(problems, fmt.Sprintf("%s passes on the red base: it must fail by assertion", id))
		case o == outcomeErrored:
			problems = append(problems, fmt.Sprintf("%s errored (does not compile or import), which is not an assertion failure", id))
		case o == outcomeSkipped:
			problems = append(problems, fmt.Sprintf("%s is skipped on the red base", id))
		}
	}
	return problems
}

// redGreenCheck is check (b): the repo has a red proof whose tests commit the head descends from, QA's test files
// are unchanged since that commit, and every test it proved red passes on the head. A repo that holds no criterion
// test has nothing to prove, which is n/a.
func (r *gateRun) redGreenCheck(ctx context.Context, results junitResults) domain.CheckResult {
	check := domain.CheckResult{Name: CheckRedGreen, Status: domain.CheckFail}
	checkout, sha, red := r.d.Checkout, r.sha, r.d.Red
	if red.TestsSHA == "" {
		criteria, qaFiles, err := loadRunTests(r.d.RunBase)
		if err != nil {
			check.Detail = err.Error()
			return check
		}
		here, err := trackedFiles(ctx, checkout, sha)
		if err != nil {
			return stepResult(ctx, CheckRedGreen, err)
		}
		if _, tests := redTests(criteria, qaFiles, here); len(tests) == 0 {
			return domain.CheckResult{Name: CheckRedGreen, Status: domain.CheckNotApplicable, Detail: "no criterion test of the run is in this repo"}
		}
		check.Detail = "no red proof for this repo: run `autopilot red` on the commit with QA's tests"
		return check
	}
	if _, err := git(ctx, checkout, "merge-base", "--is-ancestor", "--end-of-options", red.TestsSHA, sha); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			check.Detail = fmt.Sprintf("the red proof's tests commit %s is not an ancestor of the head", red.TestsSHA)
			return check
		}
		return stepResult(ctx, CheckRedGreen, err)
	}
	qaFiles, err := engine.LoadQATests(r.d.RunBase)
	if err != nil {
		check.Detail = err.Error()
		return check
	}
	var dropped []string
	for _, f := range red.QAFiles {
		if !slices.ContainsFunc(qaFiles, func(q string) bool { return strings.EqualFold(q, f) }) {
			dropped = append(dropped, f)
		}
	}
	if len(dropped) > 0 {
		check.Detail = fmt.Sprintf("run/qa-tests.json no longer lists the red proof's QA test files: %s", strings.Join(dropped, ", "))
		return check
	}
	changed, err := changedQAFiles(ctx, checkout, red.TestsSHA, sha, red.QAFiles)
	if err != nil {
		return stepResult(ctx, CheckRedGreen, err)
	}
	if len(changed) > 0 {
		check.Detail = fmt.Sprintf("QA test files changed after the red proof at %s: %s; prove red again", red.TestsSHA, strings.Join(changed, ", "))
		return check
	}
	var problems []string
	for _, id := range red.TestIDs {
		o, ok := results.outcomeOf(id)
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("%s is missing on the head", id))
		case o != outcomePassed:
			problems = append(problems, fmt.Sprintf("%s %s on the head", id, o))
		}
	}
	if len(problems) > 0 {
		check.Detail = strings.Join(problems, "; ")
		return check
	}
	check.Status = domain.CheckPass
	check.Detail = fmt.Sprintf("%d tests proven red at %s pass on the head", len(red.TestIDs), red.TestsSHA)
	return check
}

// changedQAFiles lists the files among files that differ between from and to: edited, deleted or renamed away.
func changedQAFiles(ctx context.Context, checkout, from, to string, files []string) ([]string, error) {
	if len(files) == 0 {
		return nil, nil
	}
	args := append([]string{"diff", "--name-only", "-z", "--no-renames", "--end-of-options", from, to, "--"}, files...)
	raw, err := gitRaw(ctx, checkout, args...)
	if err != nil {
		return nil, fmt.Errorf("cannot compare QA's test files: %w", err)
	}
	return splitZ(raw), nil
}
