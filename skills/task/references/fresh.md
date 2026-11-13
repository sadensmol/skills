# `fresh` — restart both agents with a clean context

Read this when the user runs `task fresh`, or when `plan` or `finish` tells you to run
`fresh`.

`fresh` restarts the writer, and the reviewer in pair mode, in new conversations. The
task, the worktree, the agterm session and the pair stay the same. Run:

```bash
python3 <sadensmol-task-skill-dir>/scripts/pair.py fresh --base <worktree-base> [--then plan|impl|finish|reviews] [--single]
```

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
- `--launcher <project new-task.sh> --branch <branch>`: only when the error asks for it
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
- The restart runs detached. Tell the user in one line that both sessions restart with a
  clean context, then end the turn.
- A later `stage` resumes the new conversations.
