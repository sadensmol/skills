package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
)

const deletedHash = "deleted"

// Snap is the state of a git folder at one moment: the commit HEAD pointed at and the content hash of every file
// that differs from that commit or is untracked.
type Snap struct {
	// Head is empty when the repo has no commit yet.
	Head  string            `json:"head"`
	Files map[string]string `json:"files"`
}

// Snapshot records the state of the git folder cwd against its own HEAD.
func Snapshot(ctx context.Context, cwd string) (Snap, error) {
	head, err := headSHA(ctx, cwd)
	if err != nil {
		return Snap{}, err
	}
	return SnapshotSince(ctx, cwd, head)
}

// SnapshotSince records the files that differ from commit base (or are untracked), so a later snapshot is comparable
// with an earlier one even when a commit was made in between. An empty base means a repo with no commit.
// Renames are not detected: both the old and the new path are listed.
func SnapshotSince(ctx context.Context, cwd, base string) (Snap, error) {
	var changed []string
	if base != "" {
		var err error
		changed, err = gitNames(ctx, cwd, "diff", "--name-only", "--no-renames", "-z", base)
		if err != nil {
			return Snap{}, fmt.Errorf("cannot list changed files in %s: %w", cwd, err)
		}
	}
	untracked, err := gitNames(ctx, cwd, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return Snap{}, fmt.Errorf("cannot list untracked files in %s: %w", cwd, err)
	}
	snap := Snap{Head: base, Files: map[string]string{}}
	for _, name := range append(changed, untracked...) {
		name = normalize(name)
		out, err := gitOutput(ctx, cwd, "hash-object", "--", name)
		if err != nil {
			snap.Files[name] = deletedHash
			continue
		}
		snap.Files[name] = strings.TrimSpace(out)
	}
	return snap, nil
}

// headSHA returns the commit HEAD points at, or "" when the repo has no commit yet. Any other failure is an error.
func headSHA(ctx context.Context, cwd string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", "--quiet", "HEAD")
	cmd.Dir = cwd
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return strings.TrimSpace(stdout.String()), nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 && stderr.Len() == 0 {
		return "", nil
	}
	return "", fmt.Errorf("cannot read HEAD of %s: %s: %w", cwd, strings.TrimSpace(stderr.String()), err)
}

func gitNames(ctx context.Context, cwd string, args ...string) ([]string, error) {
	out, err := gitOutput(ctx, cwd, args...)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, n := range strings.Split(out, "\x00") {
		if n != "" {
			names = append(names, n)
		}
	}
	return names, nil
}

func gitOutput(ctx context.Context, cwd string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = cwd
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %s: %w", args[0], strings.TrimSpace(stderr.String()), err)
	}
	return stdout.String(), nil
}

// ChangedFiles lists the files whose content hash differs between the two snapshots, including added and gone files.
func ChangedFiles(before, after map[string]string) []string {
	var changed []string
	for name, hash := range after {
		if before[name] != hash {
			changed = append(changed, name)
		}
	}
	for name := range before {
		if _, ok := after[name]; !ok {
			changed = append(changed, name)
		}
	}
	slices.Sort(changed)
	return changed
}

func zoneAllows(file, zone string) bool {
	z := normalize(zone)
	if z == "*" || z == "**" {
		return true
	}
	if m := dirGlobPattern.FindStringSubmatch(z); m != nil {
		return file == m[1] || strings.HasPrefix(file, m[1]+"/")
	}
	return file == z
}

// OutsideZones returns the changed files that no zone allows, in the order given.
func OutsideZones(changed, zones []string) []string {
	var outside []string
	for _, f := range changed {
		f = normalize(f)
		allowed := false
		for _, z := range zones {
			if zoneAllows(f, z) {
				allowed = true
				break
			}
		}
		if !allowed {
			outside = append(outside, f)
		}
	}
	return outside
}
