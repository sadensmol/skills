package usecase

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sadensmol/autopilot/internal/config"
	"github.com/sadensmol/autopilot/internal/domain"
	"github.com/sadensmol/autopilot/internal/engine"
	"github.com/sadensmol/autopilot/internal/gate"
)

const qaPhase = "qa"

// IGateRuns is the part of the run store that the red proof and the gate use.
type IGateRuns interface {
	FindByKey(ctx context.Context, key domain.IssueKey) (domain.Run, error)
	SetRedBase(ctx context.Context, key domain.IssueKey, red domain.RedBase, now time.Time) error
	Repos(ctx context.Context, key domain.IssueKey) ([]domain.RunRepo, error)
}

type IVerdicts interface {
	Save(ctx context.Context, verdict domain.Verdict, now time.Time) error
}

type GateData struct {
	Registry config.Registry
	Runs     IGateRuns
	Verdicts IVerdicts
	Checker  *gate.Gate
	Now      func() time.Time
}

// Gate proves QA's tests red and runs the hard gate for one repo of a run, in that repo's run worktree.
type Gate struct {
	d    GateData
	mu   sync.Mutex
	busy map[string]bool
}

func NewGate(d GateData) *Gate {
	return &Gate{d: d, busy: map[string]bool{}}
}

// gateTarget is one repo of a run, resolved from the store and the registry.
type gateTarget struct {
	run      domain.Run
	repo     config.Repo
	checkout string
	others   []string
}

// Red proves QA's tests at sha red on its first parent and stores the proof.
func (g *Gate) Red(ctx context.Context, key domain.IssueKey, repo, sha string) (domain.RedBase, error) {
	t, err := g.target(ctx, key, repo)
	if err != nil {
		return domain.RedBase{}, err
	}
	release, err := g.claim(key, repo)
	if err != nil {
		return domain.RedBase{}, err
	}
	defer release()
	previous, err := g.previousProof(ctx, t.run, repo)
	if err != nil {
		return domain.RedBase{}, err
	}
	result, err := g.d.Checker.Red(ctx, gate.RedData{
		Checkout: t.checkout, SHA: sha, Previous: previous, RunBase: t.run.WorktreeBase, Overrides: t.repo.ProfileOverrides,
		Out: filepath.Join(engine.RunFolder(t.run.WorktreeBase), "red", repo),
	})
	if err != nil {
		return domain.RedBase{}, fmt.Errorf("cannot prove red for %s in run %s: %w", repo, key, err)
	}
	red := result.Proof
	red.Repo = repo
	if previous.TestsSHA != "" {
		changed := "none"
		if len(result.ChangedQAFiles) > 0 {
			changed = strings.Join(result.ChangedQAFiles, ", ")
		}
		reProof := fmt.Sprintf("%s: red re-proven at %s on base %s; QA test files changed since %s: %s",
			repo, red.TestsSHA, red.BaseSHA, previous.TestsSHA, changed)
		red.ReProofs = append(append([]string(nil), previous.ReProofs...), reProof)
	}
	if err := g.d.Runs.SetRedBase(ctx, key, red, g.d.Now()); err != nil {
		return domain.RedBase{}, err
	}
	return red, nil
}

// Run runs the gate on sha and stores the verdict for that SHA.
func (g *Gate) Run(ctx context.Context, key domain.IssueKey, repo, sha string) (domain.Verdict, error) {
	t, err := g.target(ctx, key, repo)
	if err != nil {
		return domain.Verdict{}, err
	}
	release, err := g.claim(key, repo)
	if err != nil {
		return domain.Verdict{}, err
	}
	defer release()
	repos, err := g.d.Runs.Repos(ctx, key)
	if err != nil {
		return domain.Verdict{}, err
	}
	var red domain.RedBase
	for _, r := range repos {
		if r.Repo == repo {
			red = redProof(r)
		}
	}
	result, err := g.d.Checker.Run(ctx, gate.RunData{
		Checkout: t.checkout, SHA: sha, RunBase: t.run.WorktreeBase, DefaultBranch: t.repo.DefaultBranch,
		Overrides: t.repo.ProfileOverrides, Red: red, OtherCheckouts: t.others,
		Out: filepath.Join(engine.RunFolder(t.run.WorktreeBase), "gate", repo),
	})
	if err != nil {
		return domain.Verdict{}, fmt.Errorf("cannot run the gate for %s in run %s: %w", repo, key, err)
	}
	now := g.d.Now()
	verdict := domain.Verdict{
		IssueKey: key, Repo: repo, SHA: result.SHA, Checks: result.Checks, Manual: result.Manual, RedReProofs: red.ReProofs, CreatedAt: now,
	}
	if err := g.d.Verdicts.Save(ctx, verdict, now); err != nil {
		return domain.Verdict{}, err
	}
	return verdict, nil
}

// previousProof returns the repo's stored proof, empty for a first proof. The proof is write-once, so a later
// commit cannot swap it for a weaker one; only a run back in QA, where QA verify may change its tests, proves red
// again, and then on the original red base, before any implementation.
func (g *Gate) previousProof(ctx context.Context, run domain.Run, repo string) (domain.RedBase, error) {
	repos, err := g.d.Runs.Repos(ctx, run.IssueKey)
	if err != nil {
		return domain.RedBase{}, err
	}
	for _, r := range repos {
		if r.Repo != repo || r.RedTestsSHA == "" {
			continue
		}
		if run.Phase != qaPhase {
			return domain.RedBase{}, fmt.Errorf("%w: %s in run %s already has a red proof at %s; red runs again only when the run is back in QA",
				gate.ErrRefused, repo, run.IssueKey, r.RedTestsSHA)
		}
		return redProof(r), nil
	}
	return domain.RedBase{}, nil
}

func redProof(r domain.RunRepo) domain.RedBase {
	return domain.RedBase{
		Repo: r.Repo, BaseSHA: r.RedBaseSHA, TestsSHA: r.RedTestsSHA, TestIDs: r.RedTestIDs,
		QAFiles: r.RedQAFiles, CriteriaTests: r.RedCriteriaTests, ReProofs: r.RedReProofs,
	}
}

func (g *Gate) target(ctx context.Context, key domain.IssueKey, repo string) (gateTarget, error) {
	run, err := g.d.Runs.FindByKey(ctx, key)
	if err != nil {
		return gateTarget{}, fmt.Errorf("cannot find run %s: %w", key, err)
	}
	if !run.EndedAt.IsZero() {
		return gateTarget{}, fmt.Errorf("run %s has ended", key)
	}
	if run.WorktreeBase == "" {
		return gateTarget{}, fmt.Errorf("run %s has no worktrees yet", key)
	}
	p, err := runProject(g.d.Registry, run)
	if err != nil {
		return gateTarget{}, err
	}
	t := gateTarget{run: run}
	found := false
	for _, r := range p.Repos {
		if r.Name == repo {
			t.repo, found = r, true
			t.checkout = filepath.Join(run.WorktreeBase, r.Name)
			continue
		}
		t.others = append(t.others, filepath.Join(run.WorktreeBase, r.Name))
	}
	if !found {
		return gateTarget{}, fmt.Errorf("%q is not a repo of product %s", repo, run.Product)
	}
	return t, nil
}

// claim marks a repo of a run busy until release runs; a second red or gate on it meanwhile is refused, as both
// write the same report folder.
func (g *Gate) claim(key domain.IssueKey, repo string) (func(), error) {
	name := fmt.Sprintf("%s/%s", key, repo)
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.busy[name] {
		return nil, fmt.Errorf("%w: a red proof or gate of %s in run %s is already running", gate.ErrRefused, repo, key)
	}
	g.busy[name] = true
	return func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		delete(g.busy, name)
	}, nil
}
