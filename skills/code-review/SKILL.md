---
name: sadensmol-code-review
description: "Review code changes for defects, missing docs, over-engineering, and weak tests. Use when: (1) user asks to review code, (2) user asks to review the current branch, (3) user asks for a code review before a PR or a commit, (4) user says 'review my code' or 'check my changes', (5) user gives a PR link or number and asks what is wrong with it."
---

# Review Code

Five specialized reviewers examine one change, each on a single concern. They are fanned
out, one per area, by whatever mechanism the harness offers — never reviewed serially in
your own context.

## Priority Levels

Every finding MUST carry a priority.

| Priority | Meaning | Action Required |
|----------|---------|-----------------|
| **P0** | Critical defect, security vulnerability, data loss risk, or broken functionality | **Must fix before merge. PR is blocked.** |
| **P1** | Important issue that should be addressed but doesn't block the PR | Should fix in this PR or create a follow-up ticket |
| **P2** | Suggestion for improvement, minor style issue, or nice-to-have | Optional, at author's discretion |

The workflow enforces this: each reviewer returns findings through a schema whose
`priority` field accepts only `P0`, `P1`, or `P2`.

## How the reviewers are dispatched

Five reviewers, one per area, each in its own context. **Fan out** — one per area, in
parallel where the harness allows — and pass each one the **command** that produces the
diff, never the diff text. Inlining it sends the whole diff six times, once into your own
context and once per reviewer, and overflows on a large branch.

How to fan out is harness-specific, and the mechanics are not restated here:

- Claude Code → `references/dispatch-claude-code.md`
- OpenCode → `references/dispatch-opencode.md`

Read the one for the harness you are in, which the injected runtime file names. Everything
else on this page — what each reviewer looks for, what counts as P0, how the report is
rendered — is the same everywhere.

## How the reviewers load project skills

**You** work out the skill list once, and the script passes it to every reviewer. This
replaces each agent re-deriving it five times over.

1. **One language skill per language in the diff — by file extension, NOT by cwd
   (MUST FOLLOW).** A diff routinely spans several languages and the cwd is only one of
   them, so cwd-only routing silently under-loads: a mostly-Flutter diff with two `.go`
   files would get its Go reviewed without `go-programming`, producing findings that
   contradict that skill's own rules. Map every changed path:
   `*.go` → `sadensmol-go-programming` (plus `sadensmol-go-integration-tests` when Go
   test files changed); `*.dart` → `sadensmol-flutter-programming` +
   `sadensmol-dart-programming`; `*.ts` / `*.tsx` → `sadensmol-typescript-programming`;
   `*.swift` → `sadensmol-swift-programming`; extend as the diff shows more.
   **A diff touching even one `.go` file MUST carry `sadensmol-go-programming`** — its
   rules (no comment by default, so "add a doc comment" is usually the *wrong* finding;
   naming; error handling; mappers) decide whether a finding is valid at all.
2. **Add the routing pass** — `sadensmol-router`, plus any project router whose cwd
   matches (`<project>-router`), for project-specific conventions.
3. **Add concern-specific skills** — the testing reviewer also needs the stack's testing
   skill (for Go, `sadensmol-go-integration-tests`); the script appends it for that
   reviewer only.

Run the same routing pass in your own context too, before reviewing: you need the
project's conventions for the PR/commit checks below.

## Project PR conventions (apply when the project defines them)

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

## Workflow

### 0. Choose review mode

Before anything else, ask the user what to review (use `AskUserQuestion`):

- **Local** — review the current branch's local code changes. Even in Local mode you **always detect an attached PR** (`gh pr view --json number,title,url,body,baseRefName,commits`) — a PR attached to this code is never out of scope. If one exists, also run the PR-level checks (title/description per **Project PR conventions** + commit & branch hygiene) and surface them as findings. In Local mode you **report** those PR issues and **ask the user to fix them** — you do NOT `gh pr edit`/`gh pr comment` (that's PR mode).
- **PR** — review a pull request. Detect the current branch's PR with `gh pr view --json number,title,url,body`. If one exists, use it. If none exists, ask the user to provide a PR link (or number).

Then follow the matching path below. **Local** → the **Local review** section. **PR** → the **PR review** section.

## Local review

### 1. Resolve the base branch and the diff commands

**Never hardcode `main`.** Resolve the base in this order, first hit wins:

1. the attached PR's `baseRefName` (`gh pr view --json baseRefName`);
2. the remote's default head — `git rev-parse --abbrev-ref origin/HEAD` (strip the `origin/` prefix);
3. whichever of `main` / `master` / `develop` exists (`git rev-parse --verify --quiet <name>`);
4. ask the user.

Then build — and **run once yourself, only to check the change is non-empty**:

```
BASE=<resolved base>
git merge-base "$BASE" HEAD
git diff --name-only "$BASE"...HEAD
git log "$BASE"..HEAD --oneline
```

If the range diff is empty, fall back to the working tree (`git diff --name-only`, and
`git diff` as the diff command). If that is empty too, tell the user there is nothing to
review and stop.

Keep the two **commands** — they are what the workflow passes to the reviewers:

- `filesCmd`: `git diff --name-only <BASE>...HEAD`
- `diffCmd`: `git diff <BASE>...HEAD`

Derive the skill list from the file list per **How the reviewers load project skills**.

### 1b. Check the attached PR (if any)

If `gh pr view` found a PR for this branch, run the **Project PR conventions** checks
against it now — title format, description shape, and commit & branch hygiene (rebased /
not behind base, no `wip`/duplicate/fixup commits, no stray or generated files
committed). Collect the results as **[Hygiene]** findings for the final report. **Local
mode reports and asks the user to fix** — never `gh pr edit`/`gh pr comment` here. If the
user wants you to update the title/description or post the summary as a comment, that's
the **PR review** path — offer to switch.

### 2. Run the review workflow

Fan the reviewers out as your harness's dispatch reference describes, passing these
inputs:

```json
{
  "filesCmd": "git diff --name-only <BASE>...HEAD",
  "diffCmd":  "git diff <BASE>...HEAD",
  "branch":   "<current branch>",
  "commits":  "<git log <BASE>..HEAD --oneline output>",
  "skills":   ["sadensmol-router", "<project>-router", "sadensmol-go-programming"],
  "testSkills": ["sadensmol-go-integration-tests"],
  "goal":     "<the stated goal of the change, if you know it — issue title, plan step, or user's words>"
}
```

Pass `args` as a real JSON object, never as a JSON-encoded string. Then wait for the
completion notification and collect the returned reports.

### 3. Present results

Assemble one report from the workflow's return value plus your own **[Hygiene]**
findings, grouped by priority.

**PRINT THE FINDINGS. ALWAYS. AS TABLES. (MUST FOLLOW)**

The findings ARE the deliverable — running the reviewers and then describing
them in prose is the single most common way this skill fails the user. A
sentence like "the review found 1 P0 and 15 P1s, I fixed the ones that were
mine" is **not** a report: the user cannot see what was found, cannot judge a
verdict they never read, and cannot tell a finding you skipped from one that
does not exist.

So, before any commentary, any summary of what you fixed, and any question you
ask the user:

- **Render EVERY finding, in a markdown table, one row per finding.** Never a
  prose paragraph, never a bulleted digest, never "the remaining ones are
  mostly docs", never a count standing in for the list.
- **Never truncate.** No "…and 9 more", no "the rest are minor", no showing
  only P0/P1 because the P2s felt unimportant. 28 findings means 28 rows. If
  that is long, it is long — the user asked for a review.
- **One table per priority**, in P0 → P1 → P2 order, each under its own
  heading carrying the count. Omit a priority only when it has zero findings.
- Your own **[Hygiene]** findings are rows in these same tables, not a
  separate prose note.
- A reviewer that came back `"failed": true` gets a line naming the area that
  did not run — a silent gap reads as "clean".

Table shape (keep these columns, in this order):

```
### P1 — 15 findings

| # | Area | Finding | Location |
|---|---|---|---|
| 1 | Docs | api.md still documents the removed DELETE endpoint | `docs/api.md:498` |
| 2 | Quality | Dead private method KeyManager.remoteGet | `internal/service/backup/key_manager.go:203` |
```

`Finding` is the finding's own title, escaped for the table (`|` → `\|`).
`Location` is `file:line` in backticks, file alone when the finding carries no
line. Keep the title verbatim — do not rewrite it into your own words.

When the run is large, generating the tables from the workflow's returned JSON
beats retyping them: the return value is `{reports: [{area, findings: [...]}]}`,
so a short script that sorts by `priority` and prints the rows cannot drop or
paraphrase a finding the way hand-transcription does.

**Then, and only then**, add the framing around the tables:

**If any P0 findings exist**, open with a prominent blocker notice:

```
## Code Review Summary

> **BLOCKED**: This PR has P0 findings that must be resolved before merge.

{P0 table}
{P1 table}
{P2 table}

### Linter
{the Quality reviewer's linter output, or "No issues found"}
```

**If no P0 findings exist:**

```
## Code Review Summary

> **No blockers found.** PR is clear to merge (after addressing P1s if desired).

{P1 table}
{P2 table}

### Linter
{the Quality reviewer's linter output, or "No issues found"}
```

**Follow the tables with a disposition table when you acted on any finding** —
the user needs to see what happened to each, not just what was found:

```
| Status | Count | Which |
|---|---|---|
| ✅ Fixed | 6 | P1 #12, #14; P2 #3, #4, #9 |
| ⛔ Declined, with reason | 1 | P1 #15 — <the reason> |
| ⬜ Open | 21 | P0 #1; P1 #1–5, … |
```

Reference findings by their table number so the two tables read together.
A declined finding MUST carry its reason inline — "the project skill says X"
is a valid reason, "not relevant" is not.

**Do not replace the tables with the disposition table.** It is an addition.
A reply that shows only what you fixed has hidden the review from the user
again, which is the exact failure this section exists to prevent.

## The review workflow

The fan-out itself is harness-specific: use `references/dispatch-claude-code.md` or
`references/dispatch-opencode.md`. Both produce the same thing — one set of findings per
area, built from the same briefs — and the sections below format the result identically.

## PR review

Reviews a pull request end to end. Same workflow, plus PR description management,
posting the summary as a comment, and re-reviews.

### 1. Load the platform skill

If the PR is hosted on **GitHub**, invoke the `sadensmol-github` skill before doing anything else. (Other hosts: TBD — for now proceed with `gh`/generic tooling.)

### 2. Detect first review vs re-review

Fetch the PR body and existing review comments:

```
gh pr view <pr> --json number,title,url,body,headRefName,baseRefName
gh pr view <pr> --comments
```

- **First review** — the description has no code-review summary and there are no prior review comments from this skill.
- **Re-review** — the description already carries the summary (step 4) **and/or** there are prior review comments from this skill. Treat as re-review and follow step 7.

### 3. Determine the diff commands

Use the PR's own `baseRefName` as the base. The commands the reviewers will run are:

- `filesCmd`: `gh pr diff <pr> --name-only`
- `diffCmd`: `gh pr diff <pr>`

Run `filesCmd` once yourself to derive the skill list, and grab the commit list
(`gh pr view <pr> --json commits`) for context. Do **not** read the full diff into your
own context.

### 4. Ensure the PR title + description follow project conventions

First apply the project's own rules (see **Project PR conventions**): verify the **PR title** matches the project's format (issue-key/type prefix) and fix it via `gh pr edit <pr> --title ...` if it doesn't; structure the **description** the way the project's skill prescribes. Only when the project defines no description convention, fall back to the generic shape below.

The PR description must reflect **the idea of the PR** — what it changes, what feature it implements, or what bug it fixes — plus a short bullet list of the top-level changes (code, architecture, design, etc.).

- If the description **already contains** an adequate summary (someone — possibly you on a previous run, or another agent/tool — already wrote it), leave it as is. Do not rewrite a good description.
- If the description is **missing or inadequate**, write the summary from the diff and update the PR body via `gh pr edit <pr> --body ...`, preserving any existing content worth keeping.
- On a **re-review**, the description will already have this — skip the update entirely.

Summary shape:

```
## Summary
<1–3 sentences: the idea of this PR — what it changes / the feature / the bug fixed>

## Top-level changes
- <change 1 (code / architecture / design / …)>
- <change 2>
- ...
```

### 5. Run the review workflow

Fan out exactly as in Local review, with PR inputs: `filesCmd` / `diffCmd` from step 3,
`branch` = `headRefName`, `commits` from the PR, `skills` / `testSkills` derived from the
changed files, and `goal` from the PR title and description. Wait for every reviewer to
finish before writing the report.

### 6. Post the summary as a PR comment

Build the same prioritized report as Local review step 3 (grouped P0/P1/P2, per-area prefixes, linter section), then post it as a PR comment via `gh pr comment <pr> --body ...`.

**Approval suggestion:** if there are **no P0 findings**, add a line to the comment suggesting the PR is ready to approve — but **do not approve it yourself** (never run `gh pr review --approve`). If there are P0 findings, keep the BLOCKED notice instead.

### 7. Re-review: address prior comments

When step 2 detected a re-review:

- Read the previous review comments from this skill.
- Check the new diff against them: for each prior finding, verify whether it was actually fixed — confirm the fix is real and correct, not just superficially touched. Flag fixes that are wrong, incomplete, or that introduced new problems.
- Run the workflow on the current diff (step 5) to catch newly introduced issues. Put the prior findings in `goal` context so the reviewers know what was supposed to be fixed.
- Post a new comment (step 6) that: (a) states which prior findings are resolved vs still open, and (b) lists any new findings. Apply the same no-P0 → suggest-approve / P0 → BLOCKED logic.

## Common mistakes

| Mistake | Why it hurts |
|---------|--------------|
| Inlining the diff into the agent prompts | Sends the diff 6× and overflows on a large branch. Pass `diffCmd`. |
| Hardcoding `main` as the base | Repos on `master`/`develop` silently produce an empty or wrong diff. Resolve the base. |
| Letting each reviewer re-derive the skill list | Five duplicated routing passes. Derive once, pass it in. |
| Reporting CRITICAL/IMPORTANT/SUGGESTED | The scale is P0/P1/P2. Nothing else is a priority. |
| Dropping a reviewer that returned `failed` | A silent gap reads as "clean". Name the area that did not run. |
| Reviewing in your own context instead of fanning out | One context cannot hold five concerns and the diff. Dispatch. |
