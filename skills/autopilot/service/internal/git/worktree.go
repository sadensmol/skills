// Package git runs the git commands that start and clean up a run's worktrees.
package git

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strings"
)

const remote = "origin"

// Git runs git in a repo checkout.
type Git struct{}

func New() *Git {
	return &Git{}
}

type AddWorktreeData struct {
	// Repo is the repo's own checkout.
	Repo string
	// Dir is the new worktree folder.
	Dir string
	// Branch is the branch the worktree is on.
	Branch string
	// DefaultBranch is the branch of origin the new branch starts from.
	DefaultBranch string
}

// Added says what AddWorktree made; an undo removes only what was made.
type Added struct {
	// Worktree is set when this call made the worktree folder.
	Worktree bool
	// Branch is set when this call made the branch.
	Branch bool
}

// AddWorktree puts Branch in a new worktree at Dir; a Dir that already holds a worktree on Branch is kept. A new branch starts at origin's DefaultBranch,
// fetched first; a branch that already exists is reused. It reports what it made.
func (g *Git) AddWorktree(ctx context.Context, d AddWorktreeData) (Added, error) {
	if head, err := g.run(ctx, d.Dir, "rev-parse", "--abbrev-ref", "HEAD"); err == nil && head == d.Branch {
		return Added{}, nil // a restart: the worktree is already there
	}
	if _, err := g.run(ctx, d.Repo, "fetch", remote, d.DefaultBranch); err != nil {
		return Added{}, fmt.Errorf("cannot fetch %s of %s: %w", d.DefaultBranch, d.Repo, err)
	}
	exists, err := g.branchExists(ctx, d.Repo, d.Branch)
	if err != nil {
		return Added{}, err
	}
	if exists {
		if _, err := g.run(ctx, d.Repo, "worktree", "add", "--", d.Dir, d.Branch); err != nil {
			return Added{}, fmt.Errorf("cannot add worktree %s: %w", d.Dir, err)
		}
		return Added{Worktree: true}, nil
	}
	start := fmt.Sprintf("%s/%s", remote, d.DefaultBranch)
	if _, err := g.run(ctx, d.Repo, "worktree", "add", "--no-track", "-b", d.Branch, "--", d.Dir, start); err != nil {
		return Added{}, fmt.Errorf("cannot add worktree %s: %w", d.Dir, err)
	}
	return Added{Worktree: true, Branch: true}, nil
}

// RemoveWorktree deletes a worktree, and the branch too when deleteBranch is set. A worktree folder that is gone
// already is pruned, and its branch is still deleted.
func (g *Git) RemoveWorktree(ctx context.Context, repo, dir, branch string, deleteBranch bool) error {
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		if _, err := g.run(ctx, repo, "worktree", "prune"); err != nil {
			return fmt.Errorf("cannot prune worktree %s: %w", dir, err)
		}
	} else if _, err := g.run(ctx, repo, "worktree", "remove", "--force", dir); err != nil {
		return fmt.Errorf("cannot remove worktree %s: %w", dir, err)
	}
	if !deleteBranch {
		return nil
	}
	if _, err := g.run(ctx, repo, "branch", "-D", "--", branch); err != nil {
		return fmt.Errorf("cannot delete branch %s: %w", branch, err)
	}
	return nil
}

func (g *Git) branchExists(ctx context.Context, repo, branch string) (bool, error) {
	_, err := g.run(ctx, repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	if err == nil {
		return true, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("cannot look up branch %s: %w", branch, err)
}

func (g *Git) run(ctx context.Context, repo string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s: %w", args[0], strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}
