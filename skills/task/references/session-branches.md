# Session branches — tiers, briefs, ownership, merge-back

Read this when you spawn a session branch (pick its tier, write its brief, name it) or
merge one back. The rule itself is `SKILL.md` → *Session branches*.

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
NEW runner re-runs what the fix must pass — a new lane, not a follow-up message. A runner
closes its agterm test sessions as `SKILL.md` → *Test sessions* says.

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
rejected: <alternative> — <file:symbol>` line per decision it made itself (`SKILL.md` → *Model
decisions log*).

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

**Waiting inside a lane.** A lane waits for a long run with one `run-background-shell`
until-loop on the run's process, and its brief says so. Its last message is the report, never
a status such as "waiting for the monitor". The main session watches the same run:
`SKILL.md` → *Never wait on a lane's notification alone*.

**Check on them and merge back.** Use the harness's branch-management facilities to see
what is live, steer a branch or resume a finished one; never re-spawn for a follow-up. A
completion arrives as a `<task-notification>` that the operator does not see: read the
report, keep what belongs to the task (edits are already in the shared worktree), and say
in chat what it produced — or that it produced nothing.

**Branch ≠ console.** A branch is an agent lane. A console is an agterm session for the
operator to type in, opened only by `console`. A test runs in a branch; the infra it uses
lives in its own agterm session.
