# `fresh` — restart both agents with a clean context

Read this when the user runs `task fresh`, or when `plan` or `finish` tells you to run
`fresh`.

`fresh` restarts the writer, and the reviewer in pair mode, in new conversations. The
task, the worktree, the agterm session and the pair stay the same. Run:

```bash
python3 <sadensmol-task-skill-dir>/scripts/pair.py fresh --base <worktree-base> [--then plan|impl|finish|reviews] [--single]
```

**Resolve the live session mode first.** A `.pair/` directory does not prove this session
is paired. Check `pair.py inspect --base <base> --session <current-session-id> --get paired`
and confirm the session's worktree in the terminal tree. For a single session beside a
pair record, pass `--single` and use `.task-stage`. Never restart the pair's saved session
instead of the current single session. Apply this check to every `fresh` purpose.

| Argument | Stage | First prompt of the writer |
|---|---|---|
| `--then plan` | PLAN | `Plan discussion:` — load the task skill, read `PLAN.md`, open the plan gate. Refused on IMPL |
| `--then impl` | IMPL | `Implementation:` — load the task skill, read `PLAN.md`, the decisions logs and the git diff, then implement |
| `--then reviews` | PLAN | `Plan review:` — load the task skill, read `PLAN.md`, the decisions logs and the git diff, then walk the plan steps with the user. Reopens an approved plan (`references/reviews.md`) |
| `--then finish` | IMPL | `Finish:` — the same reads, then run `finish` from its start, without its clean-context step |
| no `--then` | the recorded stage | `Fresh start:` — load no skill, read nothing, wait for the user |

`fresh` refuses with exit 5 while the task session has a companion, as `stage` does
(`references/stages.md`). Wait for the companion or close it, then run `fresh` again.

## What the new conversations load

Only what the first prompt's action needs (MUST FOLLOW). Every first prompt starts with
`<label>: your context was cleared on purpose`. It gets no routing pass: the Claude Code
router hooks skip it, and every router's rules skip it (`sadensmol-router` → *Rules* →
*Task fresh start*). The pair hook does not forward it to the peer as a user message.
Any other skill loads only when a step of that action needs it. The next user message
routes as usual. The pair brief (pair mode) and the harness runtime contract still load:
they are the session's wiring, not task context.

A plugin whose `SessionStart` hook loads a skill into every new conversation
(`AUTO_CONTEXT_PLUGINS` in `pair.py`: superpowers) runs without its hooks in a fresh Claude
conversation. `pair.py` switches it off and loads a hook-less copy under the same name from
`<worktree-base>/.task-fresh/`, so its skills (`superpowers:brainstorming`, …) still load on
request. Claude Code applies only the last `--settings`, so that file carries the pair hooks
too. A later `stage` resumes the conversation with the plugin as installed.

- `--single`: a single session opened beside a pair (record `.task-stage`).
- `--launcher <workspace new-task.sh> --branch <branch>`: only when the error asks for it
  (a task opened before stages).

## When it runs

- **After the planner writes the first `PLAN.md`** — `--then plan`, once
  (`references/plan.md`, step 6).
- **After the user explicitly approves `PLAN.md`** — `--then impl` (`references/plan.md`,
  step 7).
- **When the user runs `reviews` on IMPL** — `--then reviews` (`references/reviews.md`).
- **At the start of `finish`** — the user's command or the end of implementation — `--then finish` (`references/finish.md`).
- **By hand** — the user says `task fresh`. Add `--then` only when the user names the
  next step.

`fresh` is never an approval. `--then impl` runs only after an explicit approval, and
`--then finish` never approves a diff.

## Verified plan handoff (MUST FOLLOW)

For `--then plan`, the handoff ends with a working plan review, not a detached launch.

1. Before restarting, check the recorded task session against `agtermctl tree --json`.
   Confirm its identity and worktree. Never assume a saved session id still exists.
   If it is missing, do not launch against it or guess another session id.
   Continue the plan gate in the current task session (`references/plan.md`, step 6).
2. Check the task's existing review with `plannotator-session.sh --status`.
   Reuse a current plan gate. Do not restart or close it merely to clear context.
   Preserve notes before replacing a stale review (`references/plannotator.md`).
3. With a valid target and no live review, request the restart once.
   `restart: started` means launch requested, not restart completed. Read the returned log
   and confirm the replacement writer received the `Plan discussion:` prompt.
   If this session survives while startup runs, keep a background watcher through the
   harness. Do not leave an unobserved handoff or claim completion.
4. A missing target, failed launcher, or exited startup without a replacement is a failed
   handoff. Keep the plan and its logs. Continue the gate in the current session without
   asking the user to repeat the command. Do not retry the same stale target.
5. The replacement writer, or the recovering writer, owns the gate until its page loads.
   Keep the pair review and explicit approval rules. Never infer approval from recovery.
   Verify the loaded review companion before telling the user the plan review is open.

This recovery is specific to the plan handoff. It does not authorize implementation
before approval or bypass the model change required for the implementation stage.

## Verified implementation handoff (MUST FOLLOW)

After explicit approval, transition to implementation without another user command.
Validate the recorded session against the live session and worktree before restarting.
If the record is stale, recover the task session through the task helper. Use only a
verified session for this worktree; never guess an id or repeatedly use the stale one.
Preserve the approval and decision logs throughout recovery.

Observe the restart through the harness's completion path. A `restart: started` response
only confirms a launch request. Read its log and confirm the replacement writer received
the `Implementation:` prompt. If startup fails, diagnose and recover in this continuation.
Do not return a successful handoff report or require the user to run another command.
Keep the required implementation model and explicit approval guards.
If recovery genuinely needs user action, name that specific blocker instead of going idle.

## What it does

- It refuses with exit 5 while a pair thread is open, and with exit 1 without an agterm
  session or a recorded launcher. On IMPL `--then plan` refuses with exit 1 before these
  checks (`references/stages.md`). No `--then`, `--then impl` and `--then finish` stay
  allowed on IMPL. A refusal changes nothing.
- It drops the recorded conversations: the Claude session ids and the OpenCode session.
  It never resumes an old one, and never the newest OpenCode session of the base.
- In pair mode it marks every unread message and every held `FYI:` as read. The new
  agents never get the old backlog. The reviewer starts with a catch-up for the purpose.
- The decisions log is `.pair/decisions.md` in pair mode and `.task-decisions.md` in a
  single session. User decisions are in `PLAN.md` on PLAN and in `.task-user-decisions.md`
  on IMPL, in both modes. Before you run `fresh`, make sure these logs hold every decision:
  the new conversations know nothing else.
- The restart runs detached. For `--then plan`, follow *Verified plan handoff* before
  ending the turn. For `--then impl`, follow *Verified implementation handoff*.
  For other purposes, report only the restart state the output proves.
- A later `stage` resumes the new conversations.
