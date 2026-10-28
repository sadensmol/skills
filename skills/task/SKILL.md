---
name: sadensmol-task
description: "Task/worktree management entry point: dispatches `/task SUBCOMMAND` to the project's `PROJECT-task` skill by agterm workspace or cwd, and runs the project-agnostic subcommands itself. Use when the user says: \"task new BRANCH\", \"new task\", \"task plan\", \"task list\", \"task status\", \"task switch\", \"task switch --pair\", \"task switch --single\", \"task pair\", \"task changes\", \"show changes\", \"task zed\", \"task code\", \"task console\", \"task review\", \"task reviewd\", \"review decisions\", \"task finish\", \"task cleanup\", \"task pull\", \"create worktree\", or a bare \"/task …\"."
---

# Task (dispatcher)

`/task` is the single visible task entry across the personal plugins. Each project's
`<project>-task` skill is hidden from the `/` menu (`user-invocable: false`) and is
reached through this dispatcher, or loaded directly through `load-skill`.

**This skill knows nothing about any specific project (MUST FOLLOW).** No project, repo,
workspace, path, tracker or issue prefix appears here. It resolves everything at runtime,
handles its own subcommands from the cwd alone, and passes anything that needs repository
knowledge down to the project skill. Logic that special-cases a project belongs in that
project's plugin.

## Step 1 — detect the project (the agterm workspace decides first)

```bash
echo "--- agterm workspace ---"
if [ -n "${AGTERM_WORKSPACE_ID:-}" ] && command -v agtermctl >/dev/null 2>&1; then
  agtermctl tree --json 2>/dev/null | python3 -c "import sys,json,os;d=json.load(sys.stdin);w=os.environ.get('AGTERM_WORKSPACE_ID');print(next((x.get('name') for x in d.get('result',{}).get('tree',{}).get('workspaces',[]) if x.get('id')==w),'(unknown)'))" 2>/dev/null || echo "(unknown)"
else
  echo "(not in agterm)"
fi
echo "--- cwd ---"; pwd
```

The workspace name, when present, is authoritative: a session lives under a workspace
named after its project, which stays right even when the cwd wandered (for example into
a `work_<project>` skill-source checkout).

## Step 2 — resolve to a project skill

You need the plugin namespace `<project>`, which gives `<project>-task`. In order:

1. **agterm workspace name** → `<name>-task` directly.
2. **cwd, via the local, unpublished routes file** (when the workspace is `(unknown)` or
   `(not in agterm)`):

   ```bash
   pwd | sed 's|/work_|/|g'                          # normalize a work_<project> skill-source checkout
   cat ~/.agents/sadensmol-router-routes.json 2>/dev/null || cat ~/.claude/sadensmol-router-routes.json 2>/dev/null
   ```

   A JSON array of `{ "cwd": "<substring>", "skill": "<plugin>:<skill>" }`. Take the entry
   whose `cwd` appears in the normalized path; the part before `:` is `<project>`.
3. **No match → stop.** A personal session, an unknown workspace, a namespace with no
   `-task` skill, or a route that resolves to `sadensmol` itself (this dispatcher — you
   would loop) all mean: `/task` needs a project workspace. Say so and ask which project;
   never guess.

### Project root — derived from the cwd only

`list`, `switch` and `status --all` work on the project root (the directory holding
`.worktrees/`):

```bash
cwd="$(pwd)"; root=""
case "$cwd" in */.worktrees/*) root="${cwd%%/.worktrees/*}" ;; esac
if [ -z "$root" ]; then
  d="$cwd"
  while [ "$d" != "/" ]; do [ -d "$d/.worktrees" ] && { root="$d"; break; }; d="$(dirname "$d")"; done
fi
echo "${root:-NO_ROOT}"
```

`NO_ROOT` → tell the user to run it from inside the project or one of its worktrees, and
stop. Never guess a path and never ask a skill for one.

## Step 3 — handle here, or delegate

**Handled here.** Read the reference when the subcommand runs, and follow it:

| Subcommand | Read now |
|---|---|
| `zed`, `code`, `console` | `references/editor-console.md` |
| `review` (and "open plannotator" / "open annotate") | `references/review.md` |
| `changes` ("show changes") | `references/changes.md` |
| `reviewd` ("review decisions") | `references/reviewd.md` |
| `status`, `status --all`, `list`, `switch` (incl. `--pair` / `--single`) | `references/status.md` |
| `pair` | `references/pair-mode.md` → *`pair`* |

**Delegated:** `new`, `plan`, `finish`, `cleanup`, `pull`. Ensure the resolved `<project>-task`
skill is loaded (do not reload it when its full instructions are already in context) and
apply the user's **full argument string verbatim** as its args:

- `/task new ABC-1 ABC-2` → `<project>-task` with args `new ABC-1 ABC-2`
- `/task finish` → args `finish`; bare `/task` → no args (the project skill defaults to `new`)

Never re-interpret, expand or run a delegated subcommand yourself.

**OpenCode mode for `plan`:** When a new OpenCode session starts `task plan`, use the
write-enabled `task` mode. Do not use OpenCode's read-only `plan` mode. The `plan`
subcommand writes `<worktree-base>/PLAN.md` and opens its approval gate; it does not edit
application code until approval.

- **A base on `new` is forwarded, never interpreted.** `--base <branch>`, a base task
  (key or tracker URL), or the prose form ("based on `X`", "on top of `X`", "stacked on
  `X`") goes down verbatim: `/task new abc-2-foo --base abc-1-foo` → args `new abc-2-foo
  --base abc-1-foo`; `/task new abc-2-foo based on ABC-1` → args `new abc-2-foo based on
  ABC-1`. Do not resolve it to a PR, SHA or `origin/<branch>`, and do not drop it as stray
  prose. The project's `new` resolves a base task to its PRs per repo
  (`references/new.md` → *Based on a task*); its `finish` opens the stacked PRs and
  registers the GitHub stack (`sadensmol-github` → "Stacked Pull Requests"); never do
  either here.
- **An issue-tracker URL as the branch → the tracker's real branch name.**
  Never slugify the URL path (`|` in titles becomes `-or-` junk). Fetch the issue through
  the tracker skill and pass its canonical branch name (Linear's `gitBranchName`, e.g.
  `proj-12-be-acme-inbound-session-auth`). A slug-derived name creates a second worktree
  for the same ticket.

**Shared phase references for project skills.** A project's `new` / `plan` / `finish` /
`cleanup` reads the matching shared file in this skill's directory and adds its own
hooks: `references/new.md`, `references/plan.md`, `references/finish.md`,
`references/cleanup.md`. A project skill whose scripts need agterm starts every
subcommand with this pre-flight: `[ -n "${AGTERM_ENABLED:-}" ]`, otherwise tell the user
"You need to run Claude inside agterm first." and stop.

## Worktree helpers (used by every handled-here subcommand)

`<sadensmol-task-skill-dir>` is this skill's base directory ("Base directory for this
skill" when it loads). Shell state does not survive between commands — inline these in
each command that needs them.

```bash
cwd="$(pwd)"
case "$cwd" in
  */.worktrees/*)
    root="${cwd%%/.worktrees/*}"; branch="${cwd#*/.worktrees/}"; branch="${branch%%/*}"; b="$branch"
    base="$root/.worktrees/$branch"; WT="$base" ;;
  *) echo "NOT_IN_WORKTREE" ;;
esac
```

**The "real work" filter.** `task new` refreshes dependency pins in every repo of a
worktree, so counting those would mark an untouched repo as the one being worked on.
`real_changes <repo>` prints the changed paths that are real work:

```bash
real_changes() {
  git -C "$1" status --porcelain -uall 2>/dev/null | while IFS= read -r line; do
    f="${line:3}"
    case "$f" in
      go.mod|go.sum|*/go.mod|*/go.sum|pubspec.lock|*/pubspec.lock) ;;   # dep pins from `task new`
      local.properties|*/local.properties|.DS_Store|*/.DS_Store) ;;     # generated / OS
      .claude/*|*/.claude/*) ;;                                         # harness config
      *) printf '%s\n' "$f" ;;
    esac
  done
}
```

## Pair mode

`task new <branch> --pair` opens the task session as a split: a writer agent (the only
one that edits) and a read-only reviewer of the other agent kind. It is pair programming
(MUST FOLLOW): every decision — approach, new file or type, where code lives, what it
reuses, how to do a user request, accepting or rejecting a finding — is made by both
agents BEFORE the writer types it. The writer frames the problem only, each agent forms
its own view without seeing the other's, and the writer merges the two; the reviewer
agrees only after naming the strongest objection it checked. How much loop a decision
gets follows its weight: a small one is typed with an `FYI:`, an architectural one also
goes to the user (`pair.md` → *Decision weight*). A user request or opinion question in
either pane goes through the loop before the user gets an answer; a plain fact answered
with a citation goes to the user at once. A disagreement after two
exchanges goes to the user; the writer never settles it alone (`pair.md` → *The decision
loop*). A message that only runs a command or subcommand (`/task
finish`, "run the tests") is not a decision: run it and send the peer `FYI: running
<command>`. Waiting for the reviewer is not stopping: end the turn when no read-only work
is left; the `pair.py wait` monitor or the liaison wakes you. With no work left at all,
go idle (`pair.py idle`): restart nothing and wait for new work (`pair.md` → *Idle*).

The protocol is `pair.md` (each agent gets its path in its start brief). Mode question,
mode switches and `pair.py` mechanics: `references/pair-mode.md` and `references/new.md`.

## Session branches — every parallel unit of work runs in a branch (MUST FOLLOW)

The operator watches the branch list to see what runs and what is stuck; work done
inline in the main session is invisible. So every self-contained unit of work gets its
own session branch, and the main session stays a thin coordinator.

**A branch is a fresh subagent with a written brief** — `spawn-subagent` as a
`background-subagent`; its tool output stays out of the main context. It NEVER inherits
the conversation: every turn of a forked lane re-reads the whole main context, which
made lanes cost more tokens than the main session. Write what it needs (a user decision,
an agreed approach) into the brief or `PLAN.md`.

**Stages — the models follow the task stage (MUST FOLLOW).** A task has two stages.
**PLAN** runs from `task plan` (before the clarity gate) until the user explicitly
approves `PLAN.md` (`references/plan.md`, step 6), and again during any later rework of
`PLAN.md` until the user approves it again. **IMPL** runs after the approval. Both main
sessions (the writer and the pair reviewer, or the single session) run on the stage model:

| Stage | Claude Code | OpenCode |
|---|---|---|
| PLAN | `claude-opus-5-5`, effort `xhigh` | `openai/gpt-6-astra`, variant `high` (agent `planning`; reviewer `review`) |
| IMPL | `claude-opus-5-5`, effort `high` | `openai/gpt-6.1-sol`, variant `high` (agent `main`; reviewer `review`) |

A running session cannot change its model, so a stage change restarts the sessions: run
`python3 <sadensmol-task-skill-dir>/scripts/pair.py stage --base <worktree-base> plan|impl`
(add `--single` in a single session opened beside a pair). It records the stage, respawns
the writer terminal with `--resume` of its conversation on the stage model, and in pair
mode starts the reviewer again in the new split (`launch --reopen`). The restart runs
detached: tell the user in one line that both sessions restart and continue, then end
the turn. It refuses with exit 5 while a pair thread is open: close the thread first.
`stage … plan --fresh` restarts on PLAN without `--resume`: every agent starts a new
conversation on `PLAN.md` for the plan discussion (`references/plan.md`, step 6).
The record is `.pair/config.json` (`stage`) in pair mode and `<worktree-base>/.task-stage`
in a single session. A session older than the record counts as PLAN: its hook says so,
and you run `stage … impl` only when the user has explicitly approved `PLAN.md` — never
infer it from the file. Subagents follow the stage too: in PLAN the planner and the
liaisons run on `planner-subagent`; in IMPL builders, fixers, code-review agents and
liaisons run on `main-subagent`; runners run on `cheap-subagent` in both stages.

| Tier | What it does | Model |
|---|---|---|
| **runner** | runs commands and reports: tests, build, lint, CI / deploy / rollout watching, PR check and review polling, an e2e dispatch, re-running failed jobs on a listed retry condition | `cheap-subagent` |
| **planner** | explores the repos and writes or revises `PLAN.md` (`references/plan.md`) | `planner-subagent` |
| **fixer** | edits code: fixes failing tests, lint, CI or review findings, re-runs the targeted check until green | `main-subagent` |
| **builder** | implements a new part of the task from `PLAN.md` | `main-subagent` |
| **code review** | the review agents of the code-review gate (`task review`, `task finish`) | `main-subagent` |
| **liaison** | settles one decision with the pair reviewer (`pair.md` → *Liaison*) | PLAN: `planner-subagent`; IMPL: `main-subagent` |

A runner never edits a file. Red for any reason other than a listed retry condition →
it stops and reports; the main session starts a fixer with that report. After a fixer, a
NEW runner re-runs what the fix must pass — a new lane, not a follow-up message.

**The brief, in this order:** (1) where — base, repo, branch (or PR/run URL); every path
under the base; (2) inputs — for a fixer/builder the `PLAN.md` path, the failure or
finding, the files in scope; (3) steps — exact commands; a stack-bound test lane gets a
stack the main session already brought up healthy and runs only the test command
against it; for a runner the retry conditions and limit; (4) done — the condition that
ends the lane; (5) never — commit, push, merge, ask the user, start/stop/restart a
stack, or take/release the infra lock (stack trouble → `BLOCKED`, the main session
handles it); (6) report — first line
`GREEN`, `RED` or `BLOCKED: <reason>`, then at most 20 lines, one per failure: `<test or
job> — <file:line> — <one-line error>`; a long log goes to a file whose path it gives.
A builder, fixer or planner then adds one `DECISION: <topic> — <what> — <why> —
rejected: <alternative> — <file:symbol>` line per decision it made itself (*Model
decisions log*).

**Mandatory — start the branch before you begin, not after it turns out slow:**

- Any new part of the task — a second repo, a second layer, a distinct subsystem.
- Running tests (targeted runs included) and fixing what they find.
- **Every interaction with an external system — no exceptions:** GitHub/PRs
  (babysitting, review verdicts, comments, check rollups, bot replies), CI/CD
  (`gh workflow run`, `gh run watch`, re-runs, and above all reading a failure —
  `--log-failed`, check-run annotations), deploys (ArgoCD sync, `kubectl rollout
  status`, promotion gates), remote test suites (e2e against dev or prod, with the
  triage-and-re-run loop). **One branch per external thing** (per PR, per run, per gate),
  never one for "all the CI stuff": these outputs are the worst context polluters, and a
  red job is diagnosed in its runner and fixed by a fixer.
- Anything that polls or waits; a code-review pass; a research sweep; a doc-and-e2e
  update alongside implementation.

**Run them in parallel** — independent branches go out in one `fan-out`; serialize only
true dependencies. **Every branch that edits owns its files** (both modes): its brief
lists them (a directory ends with `/`), and the lists of branches that run at the same
time do not overlap — split the work by file, and put a branch that needs a file of a
running one after it. A branch that finds it must touch a file outside its list stops and
reports `BLOCKED: needs <file>`; it never edits it. In pair mode a builder that types an
agreed thread owns that thread's `files:` (`pair.md` → *Liaison*). **Name them** what +
where, a runner's label starting with `run`:
`run tests api`, `run ci api PR 1465`, `run e2e dev regression`, `fix refund test`,
`impl inbound bet` — never `work`, `fix` or `task`.

**Check on them and merge back.** Use the harness's branch-management facilities to see
what is live, steer a branch or resume a finished one; never re-spawn for a follow-up. A
completion arrives as a `<task-notification>` that the operator does not see: read the
report, keep what belongs to the task (edits are already in the shared worktree), and say
in chat what it produced — or that it produced nothing.

**Close every branch you opened** (`stop-subagent`), pass or fail, as soon as its unit of
work ends; a stuck branch makes the session look busy. Without `stop-subagent`, say so and
ask the user to close it — never claim you stopped one. `cleanup` is the backstop.

**Stays in the main session:** the plan-approval and pre-commit review gates; every
question to the user (a branch that needs a decision reports back); `git commit` /
`push` / `gh pr merge`; quick reads, greps and single-file edits; the one-line status read
that decides whether to spawn or close a branch (`gh pr view <pr> --json
state,mergedAt`, `gh run list -L 1`) — once it is "not green", the diagnosis moves into a
branch.

**Branch ≠ console.** A branch is an agent lane. A console is an agterm session for the
operator to type in, opened only by `console`. A test runs in a branch; the infra it uses
lives in its own agterm session.

## User decisions log (MUST FOLLOW)

In both modes, every decision the user accepts — an answer to a question, a plan-gate
annotation, an approved suggestion, a disputed finding settled — is appended to
`<worktree-base>/PLAN.md` under `## Decisions`, as the user's words, verbatim, with the
date. It is passed to the planner and to every builder or fixer brief verbatim, never
paraphrased. Before any reworked plan or diff goes to the user, check it against that
list. A proposal that reverses an entry needs the user's explicit yes first. Without a
`PLAN.md` yet, keep the entries and write them in when the planner creates it.

## Model decisions log (MUST FOLLOW)

The user reads few diffs while a task runs, so every decision the model makes without
the user is logged, for review later with `reviewd` (`references/reviewd.md`). A model
decision is a **normal** or **architectural** one (`pair.md` → *Decision weight*): the
approach, a new file, type, package, endpoint, table, migration or dependency, where
code lives, reuse or replacement of existing code, a choice between two options, a
deviation from `PLAN.md`, or how a vague request was read. Mechanical and small ones
are not logged. A decision the user made is not a model decision: it goes to `##
Decisions`.

- **The planner** lists its own choices in `PLAN.md` under `## Model decisions`
  (`references/plan.md`, step 5).
- **Single mode:** the main session appends each one to `<worktree-base>/.task-decisions.md`
  when it is made — its own, and every `DECISION:` line of a branch report. Only the main
  session writes the file. One entry each, `D<n>` numbered on from the last id in `PLAN.md`
  and the log:

  ```markdown
  ### D<n> — <topic> (<YYYY-MM-DD>, <PLAN|IMPL>, <main|builder|fixer>)
  - what: <the choice>
  - why: <the reason from the code, the ticket or a convention>
  - rejected: <the alternative not taken>
  - where: <file:symbol, or PLAN.md>
  ```

- **Pair mode:** the pair log `<worktree-base>/.pair/decisions.md` is the log
  (`pair.md` → *The decision loop*, step 6). Do not write the decision twice.

## Quote the question in every answer (MUST FOLLOW)

An answer often arrives late: a branch, a liaison or the pair settles it while the user
moves on. So every reply that answers a user question or reports on a user request
starts with a quote of it, then the answer:

```
> <the user's question or request, verbatim; shorten a long one with "…">
```

This covers a direct answer, a pair joint answer, and a report from a branch
(`<task-notification>`). A reply that answers several questions quotes each one before
its own answer.

## Never move the user's focus (MUST FOLLOW)

Every agterm session a task flow opens (task, single-mode, pair respawn, console, infra,
test run, plannotator) is created with `agtermctl session new … --no-select`, so it
opens in the background. Never pass `--follow`, and never run `session select` /
`session go` / `workspace select` on it. When the session already exists, reuse it and
leave it unselected. This binds every project's `<project>-task` script too.

## Running infra (MUST FOLLOW)

Before the main session starts a local stack, read `references/infra.md` and follow it.
The hard points: **the main session owns every stack** — it runs the project's single
bring-up command itself; no subagent (runner, fixer, builder) ever starts or stops a
stack or touches the lock; check the container runtime first; take the worktree's
`.infra.lock`; the stack runs in its own agterm session (`--no-select`, below the task
session); one infra session at a time, serving one related test run, so stack-bound
runners are spawned one at a time against the healthy stack; tear down and release the
lock in one command the moment the run ends.

## Plannotator binary (shared)

Every `plannotator …` launch — `review`, `annotate`, `annotate PLAN.md --gate` — goes
through the helper, **in the background** (`run-background-shell`) (MUST FOLLOW):

```bash
cd <worktree-base> && bash <sadensmol-task-skill-dir>/scripts/plannotator-session.sh <plannotator args…>
```

`<helper>` below = `<sadensmol-task-skill-dir>/scripts/plannotator-session.sh`.

- The helper resolves the binary (`PLANNOTATOR` env, else `command -v plannotator`, else
  `~/.local/bin/plannotator`; never type a bare `plannotator`). On
  `PLANNOTATOR_NOT_FOUND`, tell the user to install it or export `PLANNOTATOR`, and stop.
- It opens plannotator in its own agterm session `└─ review <task ID>` directly below the
  task session (`--no-select`), runs it at the worktree base, prints its output, exits
  with its status and closes that session — so the background run returns the
  annotations as a `<task-notification>`. Outside agterm it runs plannotator directly.
- The page opens as an agterm HTML overlay inside that review session (via
  `PLANNOTATOR_BROWSER`, default `~/.local/bin/plannotator-agterm-open`) — never on the
  task session, never in a browser. `PLANNOTATOR_OVERLAY_OPENER_NOT_FOUND` or
  `OVERLAY_OPEN_FAILED` → report it; never fall back to a browser.
- Never a bare binary launch, never a trailing `&`, `nohup` or `disown` (the notification
  would be lost). A foreground call freezes the session. One launch; no polling, no
  relaunch while it runs. `REVIEW_SESSION_CLOSED` = the operator closed it: no
  annotations, do not relaunch.
- **One live instance per worktree (MUST FOLLOW).** `review` and `annotate` share one
  slot per worktree base; the helper holds it as `<base>/.scratch/plannotator.lock`.
  Other worktrees (other tasks) run their own instance and review session in parallel —
  never wait on them. Check this worktree's slot with
  `cd <worktree-base> && bash <helper> --status` (`RUNNING: <args> (session <id>)` or
  `NONE`). A launch while the slot is held prints `PLANNOTATOR_ALREADY_RUNNING` and
  exits 4 without launching. Never `pgrep`/`pkill` plannotator by name: that matches
  other tasks' instances.
- **Force-closing needs the user's permission.** Never kill or `session close` a live
  instance (closing the `└─ review` session kills it too) on your own — not even on
  "reopen"/"refresh". Ask through `ask-user`: **Force-close & open new** (then
  `bash <helper> --close` from the worktree base, expect `CLOSED`, launch one) or **Keep
  existing** (wait until the user closes it).
- **Stale `annotate`.** It reads the file once at launch. When you edit the exact `.md`
  an open `annotate` shows: finish all edits for the turn, tell the user the tab is
  stale, ask them to submit/close it, wait until `bash <helper> --status` shows `NONE`,
  then relaunch on the same file. Edits to another doc → just say which file
  changed. Never turn an annotate into a `review` (or back) as a refresh. No `annotate`
  running → a doc edit opens nothing; mention it and offer to reopen.

## Mermaid

Before you open plannotator on, hand over, or commit any `.md` containing a mermaid
block, read `references/mermaid.md` and validate every block.

## Test scope — targeted while working, FULL suite only at `finish` (MUST FOLLOW)

This overrides any project wording that asks for the whole suite earlier.

- While the functionality is in progress (coding, iterating, fixing a finding, reworking
  after `review`), run **only** the tests that cover the code you change — the test file,
  package, suite or `-run`/`--name` filter, plus direct callers. A full-repo run here is
  waste; do not ask the operator to stage shared infra for it.
- The full suite runs only after the user says `task finish` (every changed repo: unit,
  integration, frontend, e2e — whatever the project lists).
- `review` needs only the targeted tests for the changed behaviour to be green.
- "Never commit red" is enforced at `finish`. In-progress code may leave untouched parts
  unverified; it may not have its own tests failing.
- No targeted run possible → run the narrowest scope that covers it and say which.
- Every run, targeted or full, happens in a session branch.

## Run the task to completion — never stop in the middle (MUST FOLLOW)

A task runs start to finish in one push. Do not halt to report progress, describe the
next step, or ask permission to continue. **Not reasons to stop:** a step finished and
the next is big; you want to share findings; a command failed, a test went red, a linter
complained, CI is running (diagnose and fix — that is the task); a dependency PR is
unmerged, a review or deploy is pending (do the independent work, then drive the
blocker); two reasonable implementations where the ticket or code implies one (pick the
one matching existing code, say so in one line at the end); the task is bigger than
expected.

**Stop only when the user must decide:** the plan-approval gate and the pre-commit review
gate; a destructive or irreversible action (dropping data, force-pushing over someone
else's work, deleting a branch/env, rewriting shared history); production impact; a real
scope change (report it in one or two lines and ask); a genuine ambiguity that changes the
deliverable and that the ticket, code and conventions cannot settle (ask ONE short
question). Then report the finished result, not a mid-way status.

## Review checkpoint — stop after code changes (MUST FOLLOW)

After every batch of code changes, put the diff in front of the user before committing
or running `finish`: ask the user to review and wait, run `changes`, or run `review` (the
default; it picks `review` or `annotate` by state, so a docs-only task is checkpointed
with `annotate`). Then wait, or address the annotations. Going from code straight to
`finish` is not allowed. No annotations = approved. Every re-opened code `review` first
explains what changed since the last one and why, as a plannotator Guided Review plus a
chat notice (`references/review.md` → *Changes since the last review*).

**Local code-review runs ONLY on the user's request or inside `task finish` (MUST
FOLLOW).** Run the `sadensmol-code-review` skill only when the user asks for a review, or
inside a `finish` the user started, after the approval gate. Never start it on your own:
not after a batch of code changes, not when tests go green, not on pair agreement, not
before the user approved the diff. There, **fix every P0 and every P1**; report every P2
and fix it only when it is critical or trivially easy; say which P2s you fixed and which
you left. Re-run until no P0/P1 remains. Green tests, a clean plannotator review, or an
agentic PR review never replace it. Each project's `finish` places this gate.

## Never suggest cleanup while work remains

Never end `review`, `changes`, `status` or `finish` by proposing `cleanup` (or "remove the
worktree?"). Mention it only when every PR of the branch is **merged** (not just
approved), the project's definition-of-done gates are green (for example an e2e PR and its
regression run), and no work is left — or when the user asks. Otherwise the next step is
`finish` or more work. An operator-accepted final state with failed or bypassed checks is
reported as complete with accepted exceptions, and cleanup as `force required`.

## Adding a new project

1. Give the project's plugin a `task` skill with `user-invocable: false`. It implements
   only `new` / `plan` / `finish` / `cleanup` (reading the shared `references/` files for
   each), `pull` (its source checkouts fast-forwarded to their default branches, refused
   inside a worktree), plus its layout, preflight and definition of done. Its `new` writes the ticket id
   into the agterm session context; `status` reports that id without interpreting it.
2. Name the agterm workspace after the plugin — Step 2 then resolves it with no edit here.
3. For cwd resolution outside agterm, add `{ "cwd": "<substring>", "skill":
   "<plugin>:<skill>" }` to the local, unpublished `~/.agents/sadensmol-router-routes.json`.
   Nothing is added to this file.
