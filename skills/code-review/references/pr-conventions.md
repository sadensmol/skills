# Project PR conventions

Read this when a PR is attached to the reviewed branch (Local step 1b) or you are in PR mode (PR review step 4).

Projects carry their own PR/commit rules in their **own** skills — this sadensmol skill
owns none of those mechanics, it only requires that you **discover and enforce them**.
After the routing pass, look for the project's PR/commit/branch rules. Examples of
where they live:

- `<project>-linear` / `<project>-jira` — task/PR **title format** (e.g. an issue-key prefix) and the project's **description style**.
- `<project>-task` — commit/PR shape (how many commits, how many PRs, the finish flow).
- `<project>-<project>` (the dev skill) — living docs that must be updated in the same change, and any other repo-wide rule.

Apply whatever the loaded project skills define:

- **Title** — the PR title (and commit titles, where the project formats them) MUST match the project's format. Flag a mismatch.
- **Description** — structure the PR body the way the project's skill prescribes; only fall back to the generic shape in PR-review step 4 when the project defines none.
- **Commit & branch hygiene** — review the branch's *commits*, not just the squashed diff:
  - **Up to date with base** — the branch is rebased on / not behind its base; no accidental base-merge commits when the project rebases. Note if a rebase is needed (`git log --oneline <base>..HEAD`, `git rev-list --count HEAD..<base>`).
  - **No junk commits** — no leftover `wip`/`fixup`/duplicate/revert-your-own-change commits that should be squashed per the project's commit rule.
  - **No stray files** — nothing committed that should be gitignored: build artifacts, generated code, IDE/ephemeral files, logs, secrets.
  - **Message format** — each commit message follows the project's convention.

Report these under a **[Hygiene]** area in the summary, on the same P0/P1/P2 scale
(committed secret / broken base = P0; junk commits / wrong title format = P1; message
nits = P2). These findings are yours, not the workflow's. When the project defines no
such conventions, still apply the generic checks (junk commits, stray files) and skip
the project-specific title/message-format checks.
