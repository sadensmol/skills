# PR review

Read this when the user chose **PR** in step 0 (Choose review mode).

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
`branch` = `headRefName`, `commits` from the PR, `skills` / `testReads` derived from the
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
