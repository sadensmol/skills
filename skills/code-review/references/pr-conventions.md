# Workspace PR conventions

Read this when a PR is attached to the reviewed branch (Local step 1b) or you are in PR mode (PR review step 4).

Workspaces carry their own PR/commit rules in their **own** skills — this sadensmol skill
owns none of those mechanics, it only requires that you **discover and enforce them**.
After the routing pass, look for the workspace's PR/commit/branch rules. Examples of
where they live:

- `<workspace>-linear` / `<workspace>-jira` — task/PR **title format** (e.g. an issue-key prefix) and the workspace's **description style**.
- `<workspace>-task` — commit/PR shape (how many commits, how many PRs, the finish flow).
- `<workspace>-<workspace>` (the dev skill) — living docs that must be updated in the same change, and any other repo-wide rule.

Apply whatever the loaded workspace skills define:

- **Title** — the PR title (and commit titles, where the workspace formats them) MUST match the workspace's format. Flag a mismatch.
- **Description** — structure the PR body the way the workspace's skill prescribes; only fall back to the generic shape in PR-review step 4 when the workspace defines none.
- **Commit & branch hygiene** — review the branch's *commits*, not just the squashed diff:
  - **Up to date with base** — the branch is rebased on / not behind its base; no accidental base-merge commits when the workspace rebases. Note if a rebase is needed (`git log --oneline <base>..HEAD`, `git rev-list --count HEAD..<base>`).
  - **No junk commits** — no leftover `wip`/`fixup`/duplicate/revert-your-own-change commits that should be squashed per the workspace's commit rule.
  - **No stray files** — nothing committed that should be gitignored: build artifacts, generated code, IDE/ephemeral files, logs, secrets.
  - **Message format** — each commit message follows the workspace's convention.

Report these under a **[Hygiene]** area in the summary, on the same P0/P1/P2 scale
(committed secret / broken base = P0; junk commits / wrong title format = P1; message
nits = P2). These findings are yours, not the workflow's. When the workspace defines no
such conventions, still apply the generic checks (junk commits, stray files) and skip
the workspace-specific title/message-format checks.
