---
name: sadensmol-task
description: "Task/worktree management entry point: dispatches `/task SUBCOMMAND` to the project's `PROJECT-task` skill by agterm workspace or cwd, and runs the project-agnostic subcommands itself. Use when the user says: \"task new BRANCH\", \"new task\", \"task plan\", \"task list\", \"task status\", \"task switch\", \"task switch --pair\", \"task switch --single\", \"task pair\", \"task fresh\", \"task changes\", \"show changes\", \"task zed\", \"task code\", \"task console\", \"task review\", \"task reviewd\", \"review decisions\", \"task reviewq\", \"review questions\", \"task reviews\", \"review the plan steps\", \"task finish\", \"task cleanup\", \"task pull\", \"create worktree\", \"task help\", or a bare \"/task\" or \"/task …\"."
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

1. **agterm workspace name** → `<name>-task` directly. When that is this dispatcher
   itself (workspace `sadensmol`), use `work-<name>-task` — the private company plugin
   whose namespace cannot reuse the base prefix.
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

## Step 3 — handle here, or delegate

**Handled here.** Read the reference when the subcommand runs, and follow it:

| Subcommand | Read now |
|---|---|
| bare `/task` (no args), `help` | `references/help.md` |
| `zed`, `code`, `console` | `references/editor-console.md` |
| `review` (and "open plannotator" / "open annotate") | `references/review.md` |
| `changes` ("show changes") | `references/changes.md` |
| `reviewd` ("review decisions") | `references/reviewd.md` |
| `reviewq` ("review questions") | `references/reviewq.md` |
| `reviews` ("review the plan steps") | `references/reviews.md` |
| `status`, `status --all`, `list`, `switch` (incl. `--pair` / `--single`) | `references/status.md` |
| `pair` | `references/pair-mode.md` → *`pair`* |
| `fresh` | `references/fresh.md` |

**`switch` picks an existing worktree, not a session.** It selects the worktree's live
session, or opens one first with the project's `new-task.sh <branch> --open`. It never does
git work or setup, so never delegate it to `new`. Follow `references/status.md` → *`switch`*.

**Delegated:** `new`, `plan`, `finish`, `cleanup`, `pull`. Ensure the resolved `<project>-task`
skill is loaded (do not reload it when its full instructions are already in context) and
apply the user's **full argument string verbatim** as its args:

- `/task new ABC-1 ABC-2` → `<project>-task` with args `new ABC-1 ABC-2`
- `/task finish` → args `finish`. A bare `/task` is never delegated: it is `help`.

Never re-interpret, expand or run a delegated subcommand yourself.

A base on `new` is forwarded verbatim, never interpreted; an issue-tracker URL as the
branch becomes the tracker's real branch name. Before you forward `new` or `plan`, read
`references/delegation.md`.

**Shared phase references for project skills.** A project's `new` / `plan` / `finish` /
`cleanup` reads the matching shared file in this skill's directory and adds its own
hooks: `references/new.md`, `references/plan.md`, `references/finish.md`,
`references/cleanup.md`. A project skill whose scripts need agterm starts every
subcommand with this pre-flight: `[ -n "${AGTERM_ENABLED:-}" ]`, otherwise tell the user
"You need to run Claude inside agterm first." and stop.

## Worktree helpers

`<sadensmol-task-skill-dir>` is this skill's base directory ("Base directory for this
skill" when it loads). `$root`, `$branch`, `$base`, `$WT` and the `real_changes` filter are
in `references/worktree-helpers.md`.

## Pair mode

`task new <branch> --pair` opens the task session as a split: a writer agent (the only
one that edits) and a read-only reviewer of the other agent kind. It is pair programming
(MUST FOLLOW): the writer decides clear cases alone, a hard decision is made by both
agents BEFORE the writer types it, and the reviewer is in every code review — the full
rule is `references/pair-mode.md` → *Pair programming*.

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
approves `PLAN.md` (`references/plan.md`, step 6). **IMPL** runs after the approval. PLAN →
IMPL is one-way: the approval ends PLAN for the task. The one exception is the user's
`reviews` (`references/reviews.md`): it reopens PLAN, and the next approval returns to IMPL. Both main sessions (the
writer and the pair reviewer, or the single session) run on the stage model:

| Stage | Claude Code | OpenCode |
|---|---|---|
| PLAN | `claude-opus-5-5`, effort `xhigh` | `openai/gpt-6-astra`, variant `high` (agent `planning`; reviewer `review`) |
| IMPL | `claude-opus-5-5`, effort `high` | `openai/gpt-6.1-sol`, variant `high` (agent `main`; reviewer `review`) |

After the approval (MUST FOLLOW):

1. `PLAN.md` is frozen, `## Decisions` included. It is general direction only. No planner,
   no plan diff, no plan gate, no stage change, no `PLAN.md` edit — until the user runs
   `reviews`.
2. All feedback is implementation work: a chat message, a code review, `reviewd` or
   `reviewq` annotation, a pair objection, a new request, a schema change. It goes through the
   decision loop, a builder or fixer, and the review checkpoint with the code delta.
3. A critical decision (*Autonomy*): ask the user through `ask-user`, then implement.
   Any other design choice: decide it, log it, implement it. Never a plan gate.
4. The truth is the code, the decision logs and `PLAN.md` as direction. A later user
   decision wins over `PLAN.md`. Ask the user about a contradiction that the words do not
   settle.

In both stages, what only this task added has no old data: `sadensmol-programming` →
*Backward-Compatible Data*, `sadensmol-programming-db` → `references/migrations.md`.

A stage change restarts the sessions: read `references/stages.md` before you run
`pair.py stage`. `pair.py fresh` restarts them in new conversations, with a clean
context: after the planner writes `PLAN.md`, after the plan approval, and at the start
of `finish` (`references/fresh.md`).
The record is `.pair/config.json` (`stage`) in pair mode and `<worktree-base>/.task-stage`
in a single session. A session older than the record counts as PLAN: its hook says so,
and you run `fresh … --then impl` only when the user has explicitly approved `PLAN.md` —
never infer it from the file. Subagents follow the stage too: in PLAN the planner and the
liaisons run on `planner-subagent`; in IMPL builders, fixers, code-review agents and
liaisons run on `main-subagent`; runners run on `cheap-subagent` in both stages.

Every branch runs on its tier's model, and a runner never edits a file. Before you spawn
a branch or merge one back, read `references/session-branches.md` (tiers, the brief, file
ownership, naming, merge-back).

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

**Run them in parallel.** Every branch that edits owns its files, and branches that run
at the same time never share one (`references/session-branches.md`).

**Never wait on a lane's notification alone (MUST FOLLOW).** A lane can stop on a "waiting"
message and never wake, so its silence says nothing about its work.

- For every test run, the main session starts its own watcher when the lane reports the log
  path: one `run-background-shell` until-loop, `until ! pgrep -f '[t]ee .*<log>' >/dev/null; do
  sleep 5; done`. It exits on pass, fail and crash alike. Keep the `[t]`: without it, `pgrep -f`
  matches the watcher's own shell and the loop never ends.
- When the watcher exits, read `^(ok|FAIL)` from the log and act on it at once.
- A lane's final message is its report. "Waiting for the monitor" is not a report. No report
  1 minute after the run ended → `SendMessage` the lane for it.
- Never end a turn on "waiting" without a live watcher. A lane not listed by `ListAgents` is
  not working, whatever it said last.

**Close every branch you opened** (`stop-subagent`), pass or fail, as soon as its unit of
work ends; a stuck branch makes the session look busy. Without `stop-subagent`, say so and
ask the user to close it — never claim you stopped one. `cleanup` is the backstop.

**Stays in the main session:** the plan-approval and pre-commit review gates; every
question to the user (a branch that needs a decision reports back); `git commit` /
`push` / `gh pr merge`; quick reads, greps and single-file edits; the one-line status read
that decides whether to spawn or close a branch (`gh pr view <pr> --json
state,mergedAt`, `gh run list -L 1`) — once it is "not green", the diagnosis moves into a
branch.

## User decisions log (MUST FOLLOW)

In both modes, every decision the user accepts — an answer to a question, a plan-gate
annotation, an approved suggestion, a disputed finding settled — is logged as the user's
words, verbatim, with the date:

- **PLAN:** the planner appends it to `<worktree-base>/PLAN.md` → `## Decisions` as a
  `U<n>` entry: the user's words under `asked:`, what was decided from them under
  `decided:`, and what was done according to it under `done:` (`sadensmol-plan` → *The
  logs*). Without a `PLAN.md` yet, keep the entries and write them in when the planner
  creates it.
- **IMPL:** `PLAN.md` is frozen. Run `python3
  <sadensmol-task-skill-dir>/scripts/pair.py user-decided --base <worktree-base> --stdin`
  with the user's words. It appends them to `<worktree-base>/.task-user-decisions.md`.

Every entry is passed to the planner (PLAN) and to every builder or fixer brief verbatim,
never paraphrased. Read both logs; a later explicit decision supersedes an earlier one.
Before any plan or diff goes to the user, check it against them. A proposal that reverses
an entry needs the user's explicit yes first.

## Model decisions log (MUST FOLLOW)

The user reads few diffs while a task runs, so every decision the model makes without
the user is logged, for review later with `reviewd` (`references/reviewd.md`). A model
decision is one of these: the approach, a new file, type, package, endpoint, table,
migration or dependency, where code lives, reuse or replacement of existing code, a
choice between two options, a deviation from `PLAN.md`, or how a vague request was read.
Mechanical edits, and small changes to one existing file inside an agreed decision, are
not logged. A decision the user made is not a model decision: it goes to the *User
decisions log*. On PLAN both share `PLAN.md` → `## Decisions`.

- **The planner** lists its own choices in `PLAN.md` under `## Decisions` as `D<n>` entries
  (`references/plan.md`, step 5).
- **Single mode:** the main session logs each one when it is made — its own, and every
  `DECISION:` line of a branch report. Only the main session logs, and only through
  `pair.py`, which numbers the entry `D<n>` and routes it by stage: on PLAN into `PLAN.md`
  → `## Decisions`, on IMPL into `<worktree-base>/.task-decisions.md`.

  ```bash
  python3 <sadensmol-task-skill-dir>/scripts/pair.py model-decided --base <worktree-base> \
    --topic "<topic>" --by main|builder|fixer --stdin <<'MSG'
  - decided: <the choice, first person: "I decided to …">
  - done: <what was done according to it: file:symbol, ticket or plan id; else "not done yet: <what waits>">
  - why: <the reason from the code, the ticket or a convention>
  - rejected: <the alternative not taken>
  - where: <file:symbol, or PLAN.md>
  MSG
  ```

  It writes `### D<n> — <topic> (<YYYY-MM-DD>, <PLAN|IMPL>, <by>)` and the body.

- **Pair mode:** the pair log `<worktree-base>/.pair/decisions.md` is the log
  (`pair.md` → *The decision loop*, step 6). The writer logs a decision of this list that
  it made alone (a clear one, `pair.md` → *Decision weight*) there too, with `pair.py
  decided`. On PLAN, `pair.py` also shows each entry in
  `PLAN.md` → `## Decisions` under the same heading. Do not write it a third time. Its
  body has `decided:`, `done:` (the state when it is logged) and `where:`.

## Q&A log (MUST FOLLOW)

Every question of the task and its answer is logged, in both modes, for `reviewq`
(`references/reviewq.md`): a user question to either agent, and a question an agent asks
the user (an `ask-user` item or a question in reply text) with the user's answer. A
command that only runs something (`pair.md` → *Running a command is not a decision*) is
not a question.

```bash
python3 <sadensmol-task-skill-dir>/scripts/pair.py qa --base <worktree-base> --topic "<topic>" --stdin <<'MSG'
- asked by: user → writer | user → reviewer | user | writer → user | reviewer → user | agent → user
- question: <verbatim>
- answer: <single mode, or the user's answer to an agent question: verbatim>
- writer: <pair: the writer's first answer>
- reviewer: <pair: the reviewer's answer>
- merged: <pair: the joint answer>
- result: <pair: agreed | changed after review | disputed | answered alone>
MSG
```

- **Single mode:** `asked by`, `question`, `answer`. Log it right after the answer.
- **Pair mode:** the agent that closes the loop with the user logs it, once (`pair.md` →
  *User requests go through the pair*, step 5): its own first answer, the peer's view and
  the merged answer, under `writer:` / `reviewer:` / `merged:`. A plain fact gets
  `answer:` and `result: answered alone`. A question the pair asks the user gets the joint
  recommendation under `merged:` and the user's words under `answer:`.
- **Routing (by `pair.py`):** on PLAN into `PLAN.md` → `## Q&A`; on IMPL into
  `<worktree-base>/.task-qa.md`. The command numbers the entry `Q<n>` and adds the date and
  stage.

On PLAN, `pair.py qa`, `model-decided` and `decided` are the only writes to `PLAN.md`
outside the planner: they append to `## Q&A` and `## Decisions` and touch no other
part. While `PLAN.md` does not exist yet, or plannotator shows it, they hold the entry in
`.scratch/plan-log-pending.json`. The next write, or `pair.py plan-log-flush` before the
plan gate opens (`references/plan.md`, step 6), puts it in. Entries still held when the
plan closes go to the IMPL logs; the approved `PLAN.md` never changes. After the approval
`PLAN.md` carries no new Q&A or model decision: `reviewd` and `reviewq` return all of them
from every log.

**Item links in PLAN-stage entries.** In the agent's own fields (`decided`, `done`, `why`,
`rejected`, `where`, `writer`, `reviewer`, `merged`), write plan item ids as `[S3]`
(`sadensmol-plan` → *Item links*). Pipe the entry through the read-only filter before
`pair.py`: `python3 <sadensmol-plan-skill-dir>/scripts/plan-links.py <worktree-base>/PLAN.md
resolve <<'MSG' | python3 <pair.py> qa … --stdin`. The user's words (`asked`, `question`,
`answer`) go in unbracketed and pass unchanged. Old entries keep their bytes. On IMPL,
entries keep plain ids: `.task-decisions.md` and `.task-qa.md` do not hold the plan.

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

## Never move the user's focus unless asked (MUST FOLLOW)

**Explicit `switch` is the exception:** select the task session as requested, and open it
first when it is missing (`references/status.md` → *`switch`*). Never run setup.

Every agterm session a task flow opens (task, single-mode, pair respawn, console, infra,
test run, plannotator) is created with `agtermctl session new … --no-select`, so it
opens in the background. Never pass `--follow`, and never run `session select` /
`session go` / `workspace select` on it. When the session already exists, reuse it and
leave it unselected. This binds every project's `<project>-task` script too.

## Every extra session is a companion (MUST FOLLOW)

Only the task session itself is a top-level session: the project's `new-task.sh` and
`pair.py respawn` open it. Every other session a task flow opens (test run, infra, review,
console, any later helper) is a companion of the task session. Open it only through
`<sadensmol-task-skill-dir>/scripts/child-session.sh`. This binds plans, runner briefs and
scratch scripts too. The one exception is a page companion (next section): it has no
shell, and agterm creates it from the page.

- Never call `agtermctl session new` for it directly.
- Never pass `--after` for it, and never write `└─` or `├─` into a session name.
- There is no fallback. If the helper fails, stop and report the error.

## Show the user anything in a page companion (MUST FOLLOW)

Anything you show the user to look at or listen to goes in a local HTML page under the
scratch dir, opened as a page companion of the task session. Never ask the user to open a
scratch file, an `.md` file or any other path. Chat answers stay in chat; plannotator gates
stay in plannotator.

```bash
agtermctl session overlay open --html "$F" --companion --target "$AGTERM_SESSION_ID" --socket "$AGTERM_SOCKET"
```

- Style the page in the plannotator theme (`references/plannotator.md` → *Theme*).
- No `--follow` (*Never move the user's focus*). `--cwd DIR` only when the page loads files,
  and only the directory of those files; `--js` only when the page needs it.
- Save the printed id. Check the node in `tree --json`: `content` is `page`, `parent` is
  this session, `htmlOverlays[0].state` is `loaded`.
- After you rewrite the page: `agtermctl session overlay reload --target <saved id>`.
- A failed open inside agterm is an error: report it. Outside agterm (`AGTERM_ENABLED`
  unset), run `open "$F"` on the HTML page and say so in chat.
- `pair.py` `stage`, `fresh` and `respawn` exit 5 while a companion is open. Before a
  restart, close only this task's passive pages by saved id. For live reviews, follow
  `references/plannotator.md`. Never restart while a current gate waits for the user.

## Running infra (MUST FOLLOW)

Before the main session starts a local stack, read `references/infra.md` and follow it.
**The main session owns every stack**: no subagent (runner, fixer, builder) ever starts or
stops a stack or touches the lock.

## Plannotator binary (shared)

Every `plannotator …` launch — `review`, `annotate`, `annotate PLAN.md --gate` — goes
through the helper, **in the background** (`run-background-shell`) (MUST FOLLOW):

```bash
cd <worktree-base> && bash <sadensmol-task-skill-dir>/scripts/plannotator-session.sh <plannotator args…>
```

One live instance per worktree; follow the reuse and note-preservation rules in
`references/plannotator.md`. Read
`references/plannotator.md` before every launch, and when you edit a `.md` that an open
`annotate` shows. A generated page follows the plannotator theme (`plannotator.md` →
*Theme*).

## Mermaid

Before you open plannotator on, hand over, or commit any `.md` containing a mermaid
block, read `references/mermaid.md` and validate every block.

## Test scope — targeted while working, full suite in CI at `finish` (MUST FOLLOW)

This overrides any project wording that asks for the whole suite earlier.

- While the functionality is in progress (coding, iterating, fixing a finding, reworking
  after `review`), run **only** the tests that cover the code you change — the test file,
  package, suite or `-run`/`--name` filter, plus direct callers. A full-repo run here is
  waste; do not ask the operator to stage shared infra for it.
- The full suite runs only after the user says `task finish`, in each PR's CI (every
  changed repo: unit, integration, frontend, e2e — whatever the project lists). Locally
  `finish` runs build, lint and the changed packages' suites (`references/finish.md` §4).
- `review` needs only the targeted tests for the changed behaviour to be green.
- "Never commit red" is enforced at `finish`. In-progress code may leave untouched parts
  unverified; it may not have its own tests failing.
- No targeted run possible → run the narrowest scope that covers it and say which.
- Every run, targeted or full, happens in a session branch.

## Test sessions — close what is done (MUST FOLLOW)

- Capture each test session id. Read and save its result, then close a successful session
  at once: `agtermctl session close --target <id>`. Pass `--socket "$AGTERM_SOCKET"` when set.
- Keep a failed session open only while it helps the diagnosis. Name its id in the report.
- Before you retry that run, save the failure output you need and close the old failed or
  broken session. After `RED`, the main session closes it before it starts the new runner.
  Start the retry as a fresh attempt.
- Never hide a close error. Verify that the id left `agtermctl tree`.
- Close only test sessions that this task opened.
- Infra teardown and lock release still happen on pass or fail, even when a failed terminal
  stays open. Closing an agent branch does not close its agterm session.

## Autonomy — the user is needed at three points only (MUST FOLLOW)

The user's rule (2026-11-11): "I ONLY NEEDED - REVIEW PLAN - REVIEW DIFF BEFORE PR -
APPROVE ONLY CRITICAL DECISIONS." Nothing else stops the task.

1. **The plan gate** (`references/plan.md`, step 6).
2. **The pre-PR diff gate** (`references/finish.md` → §4b). Get there on your own:
   implement, test, code-review, fix, then open it.
3. **A critical decision.** A decision is critical when it:
   - destroys data or cannot be undone: drop data, force-push over another person's
     work, delete a branch or an env, rewrite shared history;
   - touches production: a deploy, a prod config or data change, a run against prod;
   - changes the scope beyond the ticket and the user's words, or adds a `Not requested`
     item;
   - reverses an explicit user decision (*User decisions log*);
   - skips or bends a guard (*Guards*);
   - settles a security, money or compliance trade-off that the ticket, the code and the
     conventions do not settle;
   - depends on a missing fact that changes the deliverable or its acceptance, after you
     searched every source.

Every other decision is yours: the approach, a new file, type, package, endpoint, table,
migration or dependency, how to read a vague request, a pair dispute, a review finding.
Decide from the code, the ticket and the conventions, log it
(*Model decisions log*), and go on. The user checks it later with `reviewd`.

- Ask a critical decision through `ask-user`: the recommended option first, with its
  reason. Ask all critical decisions that are due at the same time in one call.
- **On PLAN, a critical decision never stops the planning.** It goes into `PLAN.md` →
  `## Questions to clarify` with the answer the plan assumes. The user answers these
  after the plan is written, one question per `ask-user` call (`references/plan.md`,
  step 6).
- Before you ask, finish all work that does not need the answer.
- Never end a turn on a question outside these three points. A status line is not a stop.
- A pair agreement is never user approval. A plan approval never approves a diff.

## Run the task to completion — never stop in the middle (MUST FOLLOW)

A task runs start to finish in one push. Do not halt to report progress, describe the
next step, or ask permission to continue. **Not reasons to stop:** a step finished and
the next is big; you want to share findings; a command failed, a test went red, a linter
complained, CI is running (diagnose and fix — that is the task); a dependency PR is
unmerged, a review or deploy is pending (do the independent work, then drive the
blocker); two reasonable implementations where the ticket or code implies one (pick the
one matching existing code, say so in one line at the end); the task is bigger than
expected.

**Stop only at the three points of *Autonomy*.** Then report the finished result, not a
mid-way status.

## Guards — skip only with `ask-user` permission (MUST FOLLOW)

A guard is any gate or MUST rule the task skills state (the project skill names its own:
merge conditions, required checks, e2e gates, review approval, ...).

**A guard that does not apply is not a skip — decide it alone (MUST FOLLOW).** When the
change cannot touch what the guard protects, drop the guard without asking and say so in
one line of the report: no code change (docs and tracker work only) → no test run, e2e
gate or deploy watch. The user's rule (2026-11-12): "you should decide such things
automatically without my attention!!!". Ask only when the guard applies and you want to
bend it.

- Skipping or bending a guard needs the user's explicit permission through `ask-user`:
  one question per guard, naming the guard, the evidence and the risk. Never ask it in
  chat text.
- Only the answer to that `ask-user` question counts. A chat reply is never permission:
  not "go", "continue", "what are we waiting?", nor a yes to a plan that only contained
  the skip.
- No permission = the guard holds: stop and report it as `needs you`.
- One permission covers one guard on one PR or check, never the next.

## Pre-PR diff gate — the user sees the diff once, before any commit (MUST FOLLOW)

Do not stop after a batch of code changes. Finish the work first: every step of
`PLAN.md` or of the user request, its targeted tests green, then the pre-PR preparation
of `references/finish.md` (local green gate, local code review and its fixes, task-scope
check). Then open the gate with `review` (`references/finish.md` → §4b); it picks
`review` or `annotate` by state, so a docs-only task gets `annotate`. No commit, push or
PR before the user approves that exact diff. A re-opened code `review` with a non-empty
delta shows ONLY the changes since the last one (`review --patch-file`), plus a
plannotator Guided Review and a chat notice that explain what changed and why
(`references/review.md` → *Changes since the last review*). An empty delta opens the whole
current uncommitted worktree diff, without a guide. The first review opens the whole diff
too. Plan and code reviews keep separate baselines. Before a code `review` opens, discard
the `go.mod`/`go.sum` changes of every repo that has no other real change
(`references/review.md` → *Drop go mod churn before a code review*). The user can still
ask for `review` or `changes` at any time.

**The local code review runs in the pre-PR preparation, or on the user's request (MUST
FOLLOW).** Run `sadensmol-code-review` in Local mode in `references/finish.md` §4, before
the pre-PR diff gate. The user's rule of 2026-11-11 (*Autonomy*) is the request, and the
mode is Local: do not ask the mode. Never run it at another time: not after each batch,
not when tests go green, not on pair agreement. There, **fix every P0 and every P1**; report every P2
and fix it only when it is critical or trivially easy; say which P2s you fixed and which
you left. Re-run until no P0/P1 remains. Green tests, a clean plannotator review, or an
agentic PR review never replace it. Each project's `finish` places this gate.

## Docs — once, at the end (MUST FOLLOW)

Never update docs while the code still changes. Docs are the last change before the
merge: `references/finish.md` → §6a. No merge and no `cleanup` before it is done.

## Never suggest cleanup while work remains

Never end `review`, `changes`, `status` or `finish` by proposing `cleanup` (or "remove the
worktree?"). Mention it only when every PR of the branch is **merged** (not just
approved), the project's definition-of-done gates are green (for example an e2e PR and its
regression run), and no work is left — or when the user asks. Otherwise the next step is
`finish` or more work. An operator-accepted final state with failed or bypassed checks is
reported as complete with accepted exceptions, and cleanup as `force required`.

## Read on demand

**When a trigger below matches what you are about to do, read that file first** — before
you write, review or answer. The summary in this skill is not the full rule.

| When | Read |
|---|---|
| A handled-here subcommand runs | its file in the Step 3 table |
| You forward `new` or `plan` | `references/delegation.md` |
| A project's `new` / `plan` / `finish` / `cleanup` runs | `references/new.md`, `plan.md`, `finish.md`, `cleanup.md` |
| You need `$root`, `$branch`, `$base`, `$WT` or `real_changes` | `references/worktree-helpers.md` |
| You spawn a session branch or merge one back | `references/session-branches.md` |
| You change the task stage | `references/stages.md` |
| You work in a pair session | `references/pair-mode.md` → *Pair programming*, then `pair.md` |
| You launch plannotator, edit a `.md` that an open `annotate` shows, or generate a page for it (theme) | `references/plannotator.md` |
| You start a local stack | `references/infra.md` |
| A `.md` with a mermaid block goes to plannotator, a hand-over or a commit | `references/mermaid.md` |
| You add a new project | `references/adding-a-project.md` |
