// Package gate runs the service's own test checks on an exact commit: the red proof of QA's tests and the hard gate.
// Every check runs in a detached temporary worktree, so the run's own checkout is never touched.
package gate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/sadensmol/autopilot/internal/domain"
	"github.com/sadensmol/autopilot/internal/gate/profile"
)

// the six checks of a verdict, in the order the verdict lists them.
const (
	CheckTestsPass    = "tests-pass"
	CheckRedGreen     = "red-green"
	CheckCriteria     = "criteria"
	CheckDiffCoverage = "diff-coverage"
	CheckMutation     = "mutation"
	CheckTestCount    = "test-count"
)

var checkOrder = []string{CheckTestsPass, CheckRedGreen, CheckCriteria, CheckDiffCoverage, CheckMutation, CheckTestCount}

var (
	// ErrRefused means the gate did not start: the commit, the tree or the run's files are not what it needs.
	ErrRefused = errors.New("gate refused")
	// ErrNotRed means QA's tests did not all fail by assertion on the red base.
	ErrNotRed = errors.New("red not proven")
)

var shaPattern = regexp.MustCompile(`^[0-9a-f]{7,64}$`)

const cleanupTimeout = time.Minute

// IProfile runs a repo's tests and measurements; profile.Profile is the real one.
type IProfile interface {
	Probe(ctx context.Context, d profile.Dirs) error
	Tests(ctx context.Context, d profile.Dirs) (profile.TestRun, error)
	DiffCoverage(ctx context.Context, d profile.Dirs, coverage, compareRef string) (profile.DiffCoverage, error)
	Mutation(ctx context.Context, d profile.Dirs, files []string) (profile.Mutation, error)
	TestID(repoDir string, c profile.JUnitCase) string
	AssertionFailure(f profile.Failure) bool
	Parent(c profile.JUnitCase) (profile.JUnitCase, bool)
	UnreportedCode(d profile.Dirs, coverage string, files []string) ([]string, error)
}

// Detected is the profile of a checkout with the data the gate reads from it.
type Detected struct {
	Profile     IProfile
	Language    profile.Language
	Floors      profile.Floors
	SkipMarkers []*regexp.Regexp
}

// DetectFunc picks the profile of a checkout and applies the repo's overrides.
type DetectFunc func(dir string, overrides map[string]string) (Detected, error)

// DetectProfile is the DetectFunc of the real language profiles.
func DetectProfile(dir string, overrides map[string]string) (Detected, error) {
	p, err := profile.Detect(dir, overrides)
	if err != nil {
		return Detected{}, err
	}
	return Detected{Profile: p, Language: p.Language, Floors: p.Floors, SkipMarkers: p.SkipMarkers}, nil
}

type Data struct {
	Detect DetectFunc
	// Timeout is the time box of one red proof or one gate run.
	Timeout time.Duration
}

type Gate struct {
	d Data
}

func New(d Data) *Gate {
	return &Gate{d: d}
}

// RunData is what one gate run needs.
type RunData struct {
	// Checkout is the run's worktree of the repo; its HEAD must be SHA and its tree clean.
	Checkout string
	SHA      string
	// RunBase is the run's folder that holds run/criteria.json and run/qa-tests.json.
	RunBase       string
	DefaultBranch string
	Overrides     map[string]string
	// Red is the repo's red proof; an empty TestsSHA means none.
	Red domain.RedBase
	// OtherCheckouts are the run's worktrees of the product's other repos, where a criterion test may live.
	OtherCheckouts []string
	// Out takes the reports and logs; it is emptied first.
	Out string
}

// Result is the gate's verdict for the full SHA.
type Result struct {
	SHA    string
	Checks []domain.CheckResult
	Manual []string
}

// Run runs the six checks on SHA. A refused start is ErrRefused; a cancelled ctx is an error with no result. A check
// that cannot run is recorded as could-not-run; a time box hit fails every check that had not finished.
func (g *Gate) Run(ctx context.Context, d RunData) (Result, error) {
	sha, err := refuseUnlessAt(ctx, d.Checkout, d.SHA)
	if err != nil {
		return Result{}, err
	}
	out, err := emptyFolder(d.Out)
	if err != nil {
		return Result{}, err
	}
	boxCtx, cancel := context.WithTimeout(ctx, g.d.Timeout)
	defer cancel()
	trees, err := newScratch(d.Checkout)
	if err != nil {
		return Result{}, err
	}
	defer trees.remove(ctx)

	r := &gateRun{gate: g, d: d, sha: sha, out: out, trees: trees, checks: newChecks()}
	r.run(boxCtx)
	if ctx.Err() != nil {
		return Result{}, fmt.Errorf("gate of %s stopped: %w", sha, ctx.Err())
	}
	if boxCtx.Err() != nil {
		r.checks.fillRest(domain.CheckFail, fmt.Sprintf("gate time box of %s hit", g.d.Timeout))
	}
	return Result{SHA: sha, Checks: r.checks.list(), Manual: r.manual}, nil
}

type gateRun struct {
	gate   *Gate
	d      RunData
	sha    string
	out    string
	trees  *scratch
	checks *checks
	manual []string
}

// run fills the checks; it stops early when the time box ends or nothing more can run.
func (r *gateRun) run(ctx context.Context) {
	compare, err := fetchDefault(ctx, r.d.Checkout, r.d.DefaultBranch)
	if err != nil {
		r.abort(ctx, err)
		return
	}
	head, err := r.trees.add(ctx, "head", r.sha)
	if err != nil {
		r.abort(ctx, err)
		return
	}
	headDirs := profile.Dirs{Repo: head, Out: filepath.Join(r.out, "head")}
	det, run, results, err := r.gate.testRun(ctx, headDirs, r.d.Overrides)
	if err != nil {
		r.abort(ctx, err)
		return
	}
	tests := testsPassCheck(run.ExitCode, results)
	if tests.Status == domain.CheckPass {
		tests = r.againCheck(ctx, det, head, tests)
	}
	r.checks.set(tests)
	r.checks.set(r.redGreenCheck(ctx, results))
	criteria, manual := r.criteriaCheck(ctx, det.Language, results)
	r.checks.set(criteria)
	r.manual = manual
	if ctx.Err() != nil {
		return
	}
	r.checks.set(diffCoverageCheck(ctx, det, headDirs, run.Coverage, compare, r.sha))
	if ctx.Err() != nil {
		return
	}
	r.checks.set(mutationCheck(ctx, det, headDirs, compare, r.sha))
	if ctx.Err() != nil {
		return
	}
	r.checks.set(r.testCountCheck(ctx, det, compare, results))
}

// againCheck runs the test command once more at the head, after the first run passed. A test that fails in this
// second run is flaky and fails tests-pass. The other checks keep the first run.
func (r *gateRun) againCheck(ctx context.Context, det Detected, head string, passed domain.CheckResult) domain.CheckResult {
	dirs := profile.Dirs{Repo: head, Out: filepath.Join(r.out, "head-again")}
	if err := os.MkdirAll(dirs.Out, 0o755); err != nil {
		return stepResult(ctx, CheckTestsPass, fmt.Errorf("cannot create report folder: %w", err))
	}
	again, err := det.Profile.Tests(ctx, dirs)
	if err != nil {
		return stepResult(ctx, CheckTestsPass, err)
	}
	cases, err := readJUnit(again.JUnit)
	if err != nil {
		return stepResult(ctx, CheckTestsPass, &profile.CouldNotRunError{Step: "JUnit report of the second test run", Err: err})
	}
	return flakyCheck(passed, again.ExitCode, newJUnitResults(cases, det.Profile, head))
}

// abort records err for every check left, unless the time box ended the step: Run fills those as fails.
func (r *gateRun) abort(ctx context.Context, err error) {
	if ctx.Err() == nil {
		r.checks.fillRest(domain.CheckCouldNotRun, err.Error())
	}
}

// testRun detects the profile in dirs.Repo, probes its tools and runs the tests.
func (g *Gate) testRun(ctx context.Context, dirs profile.Dirs, overrides map[string]string) (Detected, profile.TestRun, junitResults, error) {
	if err := os.MkdirAll(dirs.Out, 0o755); err != nil {
		return Detected{}, profile.TestRun{}, junitResults{}, fmt.Errorf("cannot create report folder: %w", err)
	}
	det, err := g.d.Detect(dirs.Repo, overrides)
	if err != nil {
		return Detected{}, profile.TestRun{}, junitResults{}, fmt.Errorf("cannot detect the test profile: %w", err)
	}
	if err := det.Profile.Probe(ctx, dirs); err != nil {
		return Detected{}, profile.TestRun{}, junitResults{}, err
	}
	run, err := det.Profile.Tests(ctx, dirs)
	if err != nil {
		return Detected{}, profile.TestRun{}, junitResults{}, err
	}
	cases, err := readJUnit(run.JUnit)
	if err != nil {
		return Detected{}, profile.TestRun{}, junitResults{}, &profile.CouldNotRunError{Step: "JUnit report", Err: err}
	}
	return det, run, newJUnitResults(cases, det.Profile, dirs.Repo), nil
}

// checks holds the result of each named check; a check not yet set is filled at the end.
type checks struct {
	results map[string]domain.CheckResult
}

func newChecks() *checks {
	return &checks{results: map[string]domain.CheckResult{}}
}

func (c *checks) set(result domain.CheckResult) {
	c.results[result.Name] = result
}

func (c *checks) fillRest(status domain.CheckStatus, detail string) {
	for _, name := range checkOrder {
		if _, ok := c.results[name]; !ok {
			c.results[name] = domain.CheckResult{Name: name, Status: status, Detail: detail}
		}
	}
}

func (c *checks) list() []domain.CheckResult {
	list := make([]domain.CheckResult, 0, len(checkOrder))
	for _, name := range checkOrder {
		list = append(list, c.results[name])
	}
	return list
}

// stepResult records a step that returned an error: could-not-run, or a fail when the time box ended it.
func stepResult(ctx context.Context, name string, err error) domain.CheckResult {
	if ctx.Err() != nil {
		return domain.CheckResult{Name: name, Status: domain.CheckFail, Detail: fmt.Sprintf("time box hit: %s", err)}
	}
	return domain.CheckResult{Name: name, Status: domain.CheckCouldNotRun, Detail: err.Error()}
}

// refuseUnlessAt returns the full SHA when the checkout's HEAD is sha and its tree is clean.
func refuseUnlessAt(ctx context.Context, checkout, sha string) (string, error) {
	if !shaPattern.MatchString(sha) {
		return "", fmt.Errorf("%w: --sha %q is not a commit SHA in lowercase hex", ErrRefused, sha)
	}
	full, err := git(ctx, checkout, "rev-parse", "--verify", "--quiet", "--end-of-options", fmt.Sprintf("%s^{commit}", sha))
	if err != nil {
		return "", fmt.Errorf("%w: %s is not a commit of %s: %w", ErrRefused, sha, checkout, err)
	}
	head, err := git(ctx, checkout, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("%w: cannot read HEAD of %s: %w", ErrRefused, checkout, err)
	}
	if head != full {
		return "", fmt.Errorf("%w: HEAD of %s is %s, not --sha %s", ErrRefused, checkout, head, full)
	}
	status, err := git(ctx, checkout, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return "", fmt.Errorf("%w: cannot read the status of %s: %w", ErrRefused, checkout, err)
	}
	if status != "" {
		return "", fmt.Errorf("%w: the tree of %s is dirty; commit or remove: %s", ErrRefused, checkout, strings.ReplaceAll(status, "\n", ", "))
	}
	return full, nil
}

// fetchDefault updates origin's default branch and returns its SHA. An explicit refspec keeps the branch name
// from ever being read as an option.
func fetchDefault(ctx context.Context, checkout, branch string) (string, error) {
	remote := fmt.Sprintf("refs/remotes/origin/%s", branch)
	if _, err := git(ctx, checkout, "fetch", "--quiet", "origin", fmt.Sprintf("+refs/heads/%s:%s", branch, remote)); err != nil {
		return "", &profile.CouldNotRunError{Step: fmt.Sprintf("fetch origin/%s", branch), Err: err}
	}
	sha, err := git(ctx, checkout, "rev-parse", "--verify", "--quiet", "--end-of-options", fmt.Sprintf("%s^{commit}", remote))
	if err != nil {
		return "", &profile.CouldNotRunError{Step: fmt.Sprintf("resolve origin/%s", branch), Err: err}
	}
	return sha, nil
}

// trackedFiles lists the files of rev, as repo-relative slash paths.
func trackedFiles(ctx context.Context, dir, rev string) ([]string, error) {
	raw, err := gitRaw(ctx, dir, "ls-tree", "-r", "-z", "--name-only", "--end-of-options", rev)
	if err != nil {
		return nil, err
	}
	return splitZ(raw), nil
}

// trackedPath finds file among tracked, ignoring case as macOS file systems do; ok is false when it is not there.
func trackedPath(tracked []string, file string) (string, bool) {
	clean := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(file)), "./")
	for _, t := range tracked {
		if strings.EqualFold(t, clean) {
			return t, true
		}
	}
	return "", false
}

func splitZ(raw string) []string {
	var list []string
	for _, s := range strings.Split(raw, "\x00") {
		if s != "" {
			list = append(list, s)
		}
	}
	return list
}

// emptyFolder makes dir empty and returns it with symlinks resolved.
func emptyFolder(dir string) (string, error) {
	if err := os.RemoveAll(dir); err != nil {
		return "", fmt.Errorf("cannot empty %s: %w", dir, err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("cannot create %s: %w", dir, err)
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", fmt.Errorf("cannot resolve %s: %w", dir, err)
	}
	return real, nil
}

// scratch holds the detached temporary worktrees of one check.
type scratch struct {
	checkout string
	dir      string
	trees    []string
}

func newScratch(checkout string) (*scratch, error) {
	dir, err := os.MkdirTemp("", "autopilot-gate-")
	if err != nil {
		return nil, fmt.Errorf("cannot create a temporary folder: %w", err)
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve %s: %w", dir, err)
	}
	return &scratch{checkout: checkout, dir: real}, nil
}

// add makes a detached worktree of rev.
func (s *scratch) add(ctx context.Context, name, rev string) (string, error) {
	path := filepath.Join(s.dir, name)
	s.trees = append(s.trees, path)
	if _, err := git(ctx, s.checkout, "worktree", "add", "--quiet", "--detach", "--", path, rev); err != nil {
		return "", &profile.CouldNotRunError{Step: fmt.Sprintf("worktree of %s", rev), Err: err}
	}
	return path, nil
}

// remove deletes every worktree and the temporary folder, also after a time box hit or a cancel. A failure is
// logged, not returned: the verdict is already decided and a left-over folder does not change it.
func (s *scratch) remove(ctx context.Context) {
	cleanCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()
	for _, tree := range s.trees {
		if _, err := git(cleanCtx, s.checkout, "worktree", "remove", "--force", "--", tree); err != nil {
			log.Warn().Ctx(ctx).Err(err).Msgf("cannot remove the gate worktree %s", tree)
		}
	}
	if err := os.RemoveAll(s.dir); err != nil {
		log.Warn().Ctx(ctx).Err(err).Msgf("cannot remove the gate folder %s", s.dir)
	}
	if _, err := git(cleanCtx, s.checkout, "worktree", "prune"); err != nil {
		log.Warn().Ctx(ctx).Err(err).Msgf("cannot prune the worktrees of %s", s.checkout)
	}
}

// git runs git in dir and returns its trimmed output. Paths are never pathspec patterns.
func git(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := gitRaw(ctx, dir, args...)
	return strings.TrimSpace(out), err
}

func gitRaw(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"--literal-pathspecs", "-c", "core.quotePath=false", "-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %s: %w", args[0], strings.TrimSpace(stderr.String()), err)
	}
	return stdout.String(), nil
}
