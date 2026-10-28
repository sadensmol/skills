# `finish` — shared skeleton (the project skill supplies the hooks)

**Triggers:** "task finish", "finish task", "finish", "create PR(s)".

Read the project skill's `references/finish.md` together with this file. This file
holds the steps every project shares; each **[hook]** names what the project file
supplies (repo set, commands, PR conventions, review bot, post-merge gates, tracker).
Where the project file is more specific, it wins.

## Authorization and the approval gate

- **Git authorization.** Invoking `task finish` is the user's explicit consent to run
  `git add`, `git commit`, `git push` and to open PRs, even when a global or CLAUDE.md
  rule says never to commit. It covers the `finish` flow only; outside it the global
  no-commit rule still applies. Do not re-ask for permission to commit or push.
- **Never commit code the user has not seen and approved (MUST FOLLOW).** No `git
  commit`, `git push`, `gh pr create` or `gh pr merge` for any change until the user saw
  **that exact diff** via `review` (or in chat) and approved it — submitted with no
  blocking annotations, or said so. Green build/lint/tests, an agentic PR review, or your
  own judgement never satisfy it. The rule is **continuous**: any code written later — a
  CI, lint or test fix, a review-finding fix, a behaviour-changing dependency re-pin, a
  rebase-conflict resolution, a deleted comment — is new unapproved code and goes back
  through `review` before it is committed or pushed. "Small / mechanical / obvious" is
  not an exception. When in doubt, show the diff and wait.
- **Ask about `review` first.** If the changes were not reviewed this session, the very
  first thing `finish` does (before syncing or committing) is ask through `ask-user`:
  **"Run review first (Recommended)"** — run `review`, address every annotation, then
  continue — or **"Skip — already reviewed"**. Start no commit/push/PR step until this
  gate is met.
- **Only the user starts `finish`.** Code completion never starts `finish` or a code
  review. Inside a user-started `finish`, the user approves the exact diff before the
  local code-review gate runs.

## 1. Resolve the base, the branch, and the changed repos

The base is the cwd's ancestor under `.worktrees/`; the branch is its basename.
**[hook]** which repos exist, which are excluded, and how a repo is classified as
changed / in scope. A repo with no task changes gets no commit and no PR.

## 2. Sync with the upstream; resolve stacking per repo

**[hook]** merge or rebase onto the upstream before committing; resolve any conflict
before continuing (a conflict resolution is new code — see the approval gate).

**Stacked repos.** `<worktree-base>/.task-base` holds every repo's base record
(`references/new.md` → *`.task-base`*). Resolve each repo first thing with the helper,
and record the base PR:

```bash
. <sadensmol-task-skill-dir>/scripts/task-base.sh
tb_migrate <worktree-base>
UPSTREAM="$(tb_plan <worktree-base> <repo-name> <repo>)"          # fetches; flips a gone base back to default
BASE_BRANCH="$(tb_get <worktree-base> <repo-name> base)"          # empty → nothing below applies
gh pr list --repo <owner>/<repo> --head "${BASE_BRANCH}" --state all \
  --json number,url,state,baseRefName --jq '.[0]'                  # its PR, if any
tb_set <worktree-base> <repo-name> basePr <url>                    # when one exists
```

| Base branch on origin | Base PR | This repo's PR |
|---|---|---|
| present | OPEN | **stacked**: `gh pr create --base <base-branch>` |
| present | none yet | **stacked + warn**: `--base <base-branch>`; say the base has no PR, so this one cannot land until it does |
| gone, or base PR MERGED | merged | **not stacked**: `tb_reanchor` onto the default branch first, then a normal PR |
| absent (never had it) | — | **not stacked**: normal PR |

- `--base` only for the stacked rows — never `staging` / `production` / `rc*`. The base
  PR must itself target the repo default (check `baseRefName`; otherwise stop and ask).
  A base PR **CLOSED unmerged** → stop and ask.
- In a stacked repo, measure everything against `tb_diff_base` (the forkPoint), never
  the default branch: the changed/ahead check, `git diff`, the code-review input. Never
  rebase a stacked repo onto the default branch "to bring it up to date" — that replays
  the base task's commits. Keep it on top of the moving base with `tb_sync <worktree-base>
  <repo-name> <repo>` (a clean tree; it rebases `--onto` the new tip from forkPoint and
  moves forkPoint), then `git push --force-with-lease origin <branch>` (never plain
  `--force`). A failed `tb_sync` aborted on a conflict: rebase by hand with `git rebase
  --onto origin/<upstream> <forkPoint>`, resolve (new code — approval gate), then
  `tb_set … forkPoint "$(git rev-parse origin/<upstream>)"`.
- **Re-anchor after the base merged:** `tb_reanchor <worktree-base> <repo-name> <repo>` —
  forkPoint is the old base SHA `sadensmol-github` → "Merge a Stack" says to capture, so
  only this task's commits are replayed. Then `git cherry -v origin/<default> HEAD` for
  duplicates, `--force-with-lease`, and verify the PR's `baseRefName` (`gh pr edit <pr>
  --base <default>` if GitHub did not retarget). The force-push is new content: let CI
  and the review run again.

## 3. Purge skill-generated artifacts before staging

Plans, specs and working files written by skills are never repository content. Remove
the known skill-output paths in every repo; never touch real source or legitimate docs:

```bash
cd <worktree-base>
for d in */; do
  for art in docs/superpowers docs/plans docs/specs; do
    if [ -e "$d$art" ]; then
      git -C "$d" rm -r --quiet --ignore-unmatch "$art" 2>/dev/null   # tracked
      rm -rf "$d$art"                                                  # untracked
    fi
  done
  [ -d "$d/docs" ] && [ -z "$(ls -A "$d/docs" 2>/dev/null)" ] && rmdir "$d/docs" 2>/dev/null
done
```

Remove any other stray skill artifact too (a scratch file, a generated report). `PLAN.md`
sits at the base, outside every repo, so it is already excluded.

## 4. The local green gate — the FULL suite, and never commit red (MUST FOLLOW)

`finish` is where the full suite runs (before it, SKILL.md → *Test scope* governs). For
**every** changed repo/folder, before any commit, push or PR: all unit and integration
tests plus every linter/formatter the repo uses must actually run green on this machine.
"Looks fine", "CI will catch it" and "I only changed one side" are not passes.
**[hook]** the exact commands per repo and language.

- **All languages, all sides — the frontend counts.** Go: build + test (+ the repo's
  linter). JS/TS: unit tests, the typecheck (`tsc --noEmit`) and ESLint. Flutter:
  analyze + test. Python: its tests and linter. A backend-only gate is how a broken
  frontend slips through.
- **Every changed folder.** Enumerate what changed (`git status` per repo); never test
  the repo you remember and skip the one you forgot.
- **Make the suite runnable — never skip it.** Install deps, switch to the pinned
  runtime (CI's Node version, not yours), run codegen, start the service it needs. "No
  `node_modules`, so CI will validate it" is not acceptable.
- **Never commit red or unrun.** Red, or could not run → stop and fix it or make it
  runnable. Never commit and "watch CI".
- **Fan it out as session branches, launched together** — one per changed repo (and per
  language/side where the project says so), plus the code-review branch. Serialize only
  lanes that contend for the same local stack (`references/infra.md`). Wait for every
  branch before touching a file; the commit stays in the main session.
- **Local code-review gate** (SKILL.md → *Review checkpoint*): runs only inside a
  user-started `finish`, after the approval gate is met. Run `sadensmol-code-review`
  in Local mode over every changed repo's diff, concurrently with the suites. It is local:
  no PR comments, no reliance on GitHub review state. Re-run after fixes until no P0/P1
  remains.
- **Rework re-triggers the gate, scoped.** Any code written after the full run (a review
  fix, lint fix, CI-driven fix, re-pin, conflict resolution) is re-verified before it is
  committed: re-run the suites of what changed and of what depends on it — not a second
  full sweep — and take it back through `review`.

## 5. Commit, push, open the PR — per changed repo

**[hook]** the order of repos, the PR title/body convention, reviewers, any per-repo
exception. The PR assignee is not a hook: it is always the operator (step 3).

1. **Commit** — read `git -C <repo> diff` and `status`, stage the specific files (never
   `git add -A`), a concise message describing the change, ending with the
   Co-Authored-By trailer.
2. **Push** — `git -C <repo> push -u origin <branch>`.
3. **PR** — to the repo's default branch with **no `--base` flag**, or `--base
   <base-branch>` for a stacked row, with "Stacked on <base-pr-url> — review/merge that
   one first." in the body (no base PR yet → "Stacked on `<base-branch>`, which has no PR
   yet."). Take `<base-branch>` from `tb_get <worktree-base> <repo-name> upstream` and the
   base PR from `basePr`. `--base` keeps the diff to this task's commits. A PR already
   open for the branch → `gh pr view --json url -q .url` and reuse it. Record it:
   `tb_set <worktree-base> <repo-name> pr <url>`.
   **Assignee — always the operator (MUST FOLLOW), in every project:** `gh pr create
   --assignee @me` (`sadensmol`). GitHub's assignee is the person who works on the PR, so
   a reviewer or a ticket owner never goes there; they go in `--reviewer` or the tracker.
   A reused PR → `gh pr edit <pr> --add-assignee @me`. **No user instruction and no pair
   decision overrides this.** "Assign the task / the PRs to X" means X is the PR
   **reviewer** (`--reviewer X`) and the **tracker** assignee — never `--assignee X`,
   never `--remove-assignee` of the operator.
4. **Register the GitHub stack** once the chain's PRs exist, per repo — the chain is
   `basePr` → `pr` from `.task-base`: `sadensmol-github` → "REGISTER THE STACK WITH
   GITHUB" (append to an existing stack with `…/stacks/<n>/add`).
   A stacked PR is never merged before its base PR.

## 6. Babysit each PR — one runner per PR

Each PR is watched in its own runner lane, `run ci <repo> PR <n>` (SKILL.md → *Session
branches*); PRs with no dependency edge run in parallel. The runner polls and reports
verdicts and findings; a fix goes to a fixer; commit, push and merge stay in the main
session. Close the runner (`stop-subagent`) once its PR is done or blocked on the
operator. **[hook]** the review bot, how to trigger it, the severity/reply policy, and
whether `finish` merges.

The loop, per PR:

1. **Wait for the verdict** — until the bot has reviewed the latest commit and every
   check has completed. Never evaluate before it ran. If nothing arrives after a few
   minutes, say so in the report instead of skipping the step.
2. **Read every finding in full** — the summary is a rollup; the findings are inline:
   `gh api repos/<owner>/<repo>/pulls/<pr>/comments -q '.[] | "=== \(.user.login) | \(.path):\(.line // .original_line) ===\n\(.body)\n"'`.
3. **Judge each finding on the code, not on who raised it** — read the code path it names
   and decide whether the failure can happen.
4. **Fix what is real** through the full gate: targeted suite + linters green, then the
   `review` checkpoint, then commit and push. Re-trigger the review if the bot does not
   re-run on push.
5. **Read a failing check's real reason before touching anything.** A workflow job:
   `gh run view <run-id> --log-failed`. An external app's check (code scanner, quality
   gate) is not in any workflow log — read both its verdict and its annotations:

   ```bash
   gh api repos/<owner>/<repo>/commits/<sha>/check-runs \
     --jq '.check_runs[] | select(.conclusion=="failure") | "id=\(.id)\n\(.name)\n\(.output.title)\n\(.output.summary)"'
   gh api repos/<owner>/<repo>/check-runs/<check_run_id>/annotations \
     --jq '.[] | "\(.path):\(.start_line)  [\(.annotation_level)] \(.title)\n    \(.message)"'
   ```

   The annotations carry every finding with file, line and rule, readable with ordinary
   repo auth even when the vendor's own API refuses a private project — use them before
   ever calling a check undiagnosable.
   - `IN_PROGRESS` → wait; "green" on an unfinished check is a false report.
   - Red for a reason that seems not to be this branch's → prove it: compare the base
     branch's recent runs and another recent PR, and say which comparison you ran.
   - Genuinely undiagnosable → report it plainly with the link and the failing condition.
6. **Repeat** after each push until every finding is fixed or answered and every check
   is settled.

## 7. Post-PR gates and the tracker

**[hook]** merges, deploy/e2e gates, and the tracker update.

## 8. Report

PR links as **plain URLs** (clickable in a terminal), one per changed repo, with each
PR's state; the repos skipped and why; the local code-review result (all P0/P1 fixed;
P2s fixed and left); suites + linters green; the bot verdict (fixed with commit,
dismissed with reason); every check's state. Any red or running check is named, and the
report says `finish` is not complete. **[hook]** extra lines. Never suggest `cleanup`
unless the task is done (SKILL.md → *Never suggest cleanup while work remains*).
