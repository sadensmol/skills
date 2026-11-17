# `finish` — default hooks

Read `<sadensmol-task-skill-dir>/references/finish.md` and follow it with these hooks. Where they
differ, this file wins. `finish` never merges.

## Step 1 — the repo (hook)

One project repo: `<base>/<project>`. Changed = uncommitted changes or commits ahead of its
base.

```bash
. <sadensmol-task-skill-dir>/scripts/task-base.sh
tb_migrate <base>
r=<project>; d="<base>/$r"
git -C "$d" fetch origin --prune >/dev/null 2>&1 || true
up="$(tb_plan <base> "$r" "$d")"
echo "$r: branch=$(git -C "$d" rev-parse --abbrev-ref HEAD) upstream=origin/$up stacked=$(tb_get <base> "$r" stacked)"
```

## Step 2 — rebase (hook)

`tb_sync <base> <project> <repo>` — never a plain `git rebase origin/<upstream>`. Linear
history, no merge commits. Already pushed → `--force-with-lease`.

## Step 4 — the green gate (hook)

In the worktree, one runner per test or lint command the project repo names (its
`CLAUDE.md`, else its `Makefile`), in parallel.

No stack, no model, no call that triggers work on the user's running services (project
`CLAUDE.md`). Then `sadensmol-code-review` over the diff, every P0/P1 fixed.

Also verify the project's source-of-truth docs match the change: the sections and diagrams
the plan named are updated. Missing → fix before commit.

## Step 6b — manual check, approved by the user (MUST FOLLOW)

Before commit, push or PR. The user's `make up` stack runs from the source checkout, not
the worktree, so the user decides how to test:

1. Write the manual test in chat: numbered steps, each with the expected result, plus one
   regression check of the nearest existing flow.
2. **Ask the user**: "Manual check of `<KEY>-<n>` — result?" Options: passed / failed /
   not testable by hand (offer this only for tests-only, docs-only or r&d work, with the
   reason).
3. Failed → a fixer fixes it, targeted tests and the `review` checkpoint run, then back to
   step 1. Only the user's answer passes this step.

## Step 5 — commit, push, PR (hook)

- **Title:** `<KEY>-<n> <short summary>` — key in capitals, first.
- **Base:** `main`, or `--base "$(tb_get <base> <project> upstream)"` when stacked.
- **Reviewer:** none — the operator reviews and merges. Assignee `@me` (shared rule).
- **Body:** `## Summary` (1–3 sentences), `## Top-level changes` (bullets), the
  doc sections changed, then the `🤖 Generated with` line. No session
  links, transcript paths or local paths.

## Step 6 — babysit (hook)

The repo has no review bot. If the PR has checks, one runner drives them to green; none →
say "no CI checks" in the report.

## Step 7 — Linear (hook)

Per the sadensmol defaults of `sadensmol-linear` → *Status at task finish*: one comment with the PR URL; the
status stays the `implementing` status of the registry.

## Step 8 — report (hook)

The PR URL, the gate results, the manual check (the user's answer), the doc sections
changed, and the Linear comment. No auto-merge.
