---
name: sadensmol-code-review
description: "Review code changes for defects, missing docs, over-engineering, and weak tests. Use when: (1) user asks to review code, (2) user asks to review the current branch, (3) user asks for a code review before a PR or a commit, (4) user says 'review my code' or 'check my changes', (5) user gives a PR link or number and asks what is wrong with it, (6) `task finish` runs it as its gate. Never start it on your own."
---

# Review Code

**Start only on an explicit user request or inside `task finish` (MUST FOLLOW)**, also
when the task flow starts `finish` itself (`sadensmol-task` → *Autonomy*). Finished code, green tests or a skill's checkpoint never start it.

Six review areas examine one change, each on a single concern. They are fanned out, one
reviewer per area, by whatever mechanism the harness offers — never reviewed serially in
your own context. Security is the exception in size: two hunters map and hunt the whole
security surface the change touches, and a verifier tries to refute every candidate. Both
run Cloudflare's `security-audit` skill, which must be installed from upstream:
`npx skills add https://github.com/cloudflare/security-audit-skill --skill security-audit --global`.

## Priority Levels

Every finding MUST carry a priority.

| Priority | Meaning | Action Required |
|----------|---------|-----------------|
| **P0** | Critical defect, security vulnerability, data loss risk, or broken functionality | **Must fix before merge. PR is blocked.** |
| **P1** | Important issue that should be addressed but doesn't block the PR | Should fix in this PR or create a follow-up ticket |
| **P2** | Suggestion for improvement, minor style issue, or nice-to-have | Optional, at author's discretion |

The Claude Code workflow enforces this: each reviewer returns findings through a schema whose
`priority` field accepts only `P0`, `P1`, or `P2`. Security findings get their priority from
the workflow, mapped from verdict and severity (`references/security.md`).

## How the reviewers are dispatched

One reviewer per area (two hunters and a verifier for Security), each in its own context.
**Fan out** — in parallel where the harness allows — and pass each one the **command** that
produces the diff, never the diff text. Inlining it sends the whole diff once into your own
context and once more per reviewer, and overflows on a large branch.

How to fan out is harness-specific, and the mechanics are not restated here:

- Claude Code → `references/dispatch-claude-code.md`
- OpenCode → `references/dispatch-opencode.md`

Read the one for the harness you are in, which the injected runtime file names. Everything
else on this page — what each reviewer looks for, what counts as P0, how the report is
rendered — is the same everywhere.

## How the reviewers load project skills

**You** work out the skill list once, and the script passes it to every reviewer. This
replaces each agent re-deriving it on its own.

1. **One language skill per language in the diff — by file extension, NOT by cwd
   (MUST FOLLOW).** A diff routinely spans several languages and the cwd is only one of
   them, so cwd-only routing silently under-loads: a mostly-Flutter diff with two `.go`
   files would get its Go reviewed without `programming-go`, producing findings that
   contradict that skill's own rules. Map every changed path:
   `*.go` → `sadensmol-programming-go` (its testing reviewer also reads
   `references/integration-tests.md`); `*.dart` → `sadensmol-programming-flutter` +
   `sadensmol-programming-dart`; `*.ts` / `*.tsx` → `sadensmol-programming-typescript`;
   `*.swift` → `sadensmol-programming-swift`; `*.py` → `sadensmol-programming-python`; extend as the diff shows more. Any
   language skill also brings `sadensmol-programming` — the language-agnostic rules
   (comments, naming, code shape, errors, layers) live there.
   **A diff touching even one `.go` file MUST carry `sadensmol-programming-go`** and
   `sadensmol-programming` — their rules (no comment by default, so "add a doc comment" is usually the *wrong* finding;
   naming; error handling; mappers) decide whether a finding is valid at all.
2. **Add the routing pass** — `sadensmol-router`, plus any project router whose cwd
   matches (`<project>-router`), for project-specific conventions.
3. **Add concern-specific skills** — the testing reviewer also needs the stack's testing
   reference (for Go, `sadensmol-programming-go` → `references/integration-tests.md`); the script appends it for that
   reviewer only.

Run the same routing pass in your own context too, before reviewing: you need the
project's conventions for the PR/commit checks below.

## Comments (MUST FOLLOW)

The default for any change is **zero comments**. **Never propose adding a comment** — no
finding whose fix is "add a comment" / "add a doc comment" / "document this"; the fix for an
unreadable hunk is *rename it* / *split it* / *make the type say it*. Only the **Quality**
reviewer flags junk comments the diff adds. **Generated files are exempt** from every comment
rule. Full rules, scope and priorities: `references/comments.md`.

## Project PR conventions (apply when the project defines them)

When a PR is attached, or in PR mode, discover the project's PR/commit/branch rules in its
own skills and enforce them: title format, description shape, commit & branch hygiene.
Report them under a **[Hygiene]** area on the same P0/P1/P2 scale. These findings are yours,
not the workflow's. The checks, where the rules live, and the priority mapping:
`references/pr-conventions.md`.

## Full security audit (opt-in)

When the user asks for a full security audit or security test of the change ("full
security", "security audit", "pen-test this change"), the Security area runs
`security-audit` in its full audit mode, scoped to the change, instead of the two hunters.
Never start it on your own; `task finish` never asks for it. How to run it and how its
records become rows: `references/security.md`.

## Workflow

### 0. Choose review mode

Before anything else, ask the user what to review (through `ask-user`). When the caller names the mode (`task finish` names Local), skip the question:

- **Local** — review the current branch's local code changes. Even in Local mode you **always detect an attached PR** (`gh pr view --json number,title,url,body,baseRefName,commits`) — a PR attached to this code is never out of scope. If one exists, also run the PR-level checks (title/description per **Project PR conventions** + commit & branch hygiene) and surface them as findings. In Local mode you **report** those PR issues and **ask the user to fix them** — you do NOT `gh pr edit`/`gh pr comment` (that's PR mode).
- **PR** — review a pull request. Detect the current branch's PR with `gh pr view --json number,title,url,body`. If one exists, use it. If none exists, ask the user to provide a PR link (or number).

Then follow the matching path. **Local** → the **Local review** section below. **PR** → `references/pr-review.md`.

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
  "skills":   ["sadensmol-router", "<project>-router", "sadensmol-programming-go"],
  "testReads": ["sadensmol-programming-go → references/integration-tests.md"],
  "goal":     "<the stated goal of the change, if you know it — issue title, plan step, or user's words>",
  "security": "review"
}
```

Pass `args` as a real JSON object, never as a JSON-encoded string. Then wait for the
completion notification and collect the returned reports.

**Pair mode** — the worktree base has `.pair/config.json` and your start brief made you
the WRITER. Then each subagent's findings go to the pair reviewer as that subagent
returns, and you present the report only after every finding has a common decision.
Follow the *Code review* section of the task skill's `pair.md` (its path is in your start
brief). Your harness's dispatch reference says how to get the findings per subagent.

### 3. Present results

Assemble one report from the workflow's return value plus your own **[Hygiene]**
findings, grouped by priority.

**PRINT THE FINDINGS. ALWAYS. AS TABLES. (MUST FOLLOW)**

The findings ARE the deliverable (why: `references/reporting.md`).

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
- **The Security area adds its own sections below the tables**: a detail block per
  Security row, the boundaries it covered, hardening notes, and what the verifier
  rejected. Their shape: `references/security.md` → *Report*.

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

**Then, and only then**, add the framing around the tables:

**If any P0 findings exist**, open with a prominent blocker notice:

```
## Code Review Summary

> **BLOCKED**: This PR has P0 findings that must be resolved before merge.

{P0 table}
{P1 table}
{P2 table}

{Security sections}

### Linter
{the Quality reviewer's linter output, or "No issues found"}
```

**If no P0 findings exist:**

```
## Code Review Summary

> **No blockers found.** PR is clear to merge (after addressing P1s if desired).

{P1 table}
{P2 table}

{Security sections}

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

Mode-specific: read `references/pr-review.md` when the user chose **PR** in step 0.

## Read on demand

**When a trigger below matches what you are about to do, read that file first** — before
you write, review or answer. The summary in this skill is not the full rule.

| When | Read |
|------|------|
| You are in Claude Code, before the fan-out | `references/dispatch-claude-code.md` |
| You are in OpenCode, before the fan-out | `references/dispatch-opencode.md` |
| You judge, decline or file a comment finding | `references/comments.md` |
| A PR is attached (Local step 1b) or you are in PR mode | `references/pr-conventions.md` |
| The user chose **PR** in step 0 | `references/pr-review.md` |
| You assemble a large report, or check the common mistakes | `references/reporting.md` |
| You render the Security sections, or the user asked for a full security audit | `references/security.md` |
