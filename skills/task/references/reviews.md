# `reviews` — handled by the dispatcher

**Triggers:** "task reviews", "review the plan steps", "walk me through the plan",
"let's go through the plan step by step".

`reviews` walks the `PLAN.md` → `## Plan` steps (`S<n>`) with the user, one step at a
time. The user keeps, discusses, changes, removes, moves or adds steps. Then the planner
reworks `PLAN.md` once, the plan gate opens again, and the approval returns the task to
IMPL. It runs at any time, in any stage. It is the only way back from IMPL to PLAN
(`SKILL.md` → *Stages*). It is started by the user only, never by an agent.

## 1. Check the plan

Derive `base` as in `references/worktree-helpers.md`. No `PLAN.md`, or no `S<n>` step
under `## Plan` → say "This task has no plan with steps to review." and stop.

## 2. Switch to PLAN

- **On PLAN** (`pair.py plan-open --base <base>` exits 0): no restart. Close an open plan
  gate first and keep its notes (`references/plannotator.md`). Go to step 3.
- **On IMPL** (exit 1): stop every running branch (`stop-subagent`) and close this
  task's passive companion pages. Do not stop a running infra stack. Then run:

  ```bash
  python3 <sadensmol-task-skill-dir>/scripts/pair.py fresh --base <worktree-base> --then reviews
  ```

  Add `--single` in a single session opened beside a pair. It clears the IMPL record in
  both modes, and restarts the sessions on the PLAN models in new conversations. The
  writer starts with the `Plan review:` prompt. Tell the user in one line that the
  sessions restart on PLAN for the review, then end the turn. Exit 5 (an open thread or a
  companion) → settle it, then run it again.

## 3. Walk the steps — one step per turn (MUST FOLLOW)

Read `PLAN.md`, the decision logs and the git diff of each repo. Take the live steps in
plan order. Skip a struck-through (dropped) step.

For each step, post in chat:

```
Step <k>/<N> — [S<n>] <title>
<3–5 short lines in plain words: what this step changes, why, and how we will check it>
Status: not started | partly in the code (<file>) | done in the code (<file>)
```

The overview is top-level. Name no function bodies and no long file lists. Get the status
from the git diff, never from memory.

Then ask with `ask-user`, one question per call:

- `Keep — next step`
- `Discuss`
- `Change`
- `Remove`

A move or an added step comes through "Other" or in chat. Wait for the answer. While the
user discusses a step, answer each message (quote it first), and stay on that step until
the user picks an outcome. Never go to the next step on your own.

Record each outcome in `<base>/.scratch/plan-reviews.md`, new on each `reviews` run:

```markdown
## [S<n>] <title>
- outcome: kept | changed | removed | moved after [S<m>] | added after [S<m>]
- user: <the user's words, verbatim>
- change: <what changes in the plan; "none" for kept>
```

Never edit `PLAN.md` during the walk. Log every question and its answer
(`SKILL.md` → *Q&A log*). A move or a removal can make a later step change too: say so
when you reach that step.

When the user stops the walk early ("enough", "rest is fine"), the steps not shown are
`kept`.

## 4. Close the walk

After the last step, ask with `ask-user`: "Add a new step, or change the order?"
(`No — rework the plan` first). Then post the outcome list in chat: one line per step
that is not `kept`, and the new step order when it changed.

No outcome other than `kept` → the plan stays as it is. On a reopened IMPL task, the
plan gate still opens (step 6), because only the approval returns the task to IMPL.

## 5. Rework the plan — one planner run

Send every outcome from `.scratch/plan-reviews.md` to the planner in one brief, as user
decisions (`references/plan.md`, step 5). The planner follows `sadensmol-plan`:

- One `U<n>` entry per outcome that is not `kept`, with the user's words verbatim.
- A removed step stays as one struck-through line. Ids never change: a moved step keeps
  its id at its new position, and a new step takes the next free `S<n>`.
- Every part of `PLAN.md` that carries the old choice changes too (Task, Now, R&D,
  Testing).
- A step already in the code that the user removed or changed gets a line that says what
  code must change or be reverted. The reverted work is part of the plan.
- The planner runs `plan-links.py fix`, then `check`.

In pair mode, the writer runs the walk. Before the planner brief, the writer sends the
outcomes to the reviewer through the decision loop (`pair.md`). The reviewer's objection
goes to the user as one `ask-user` question, never as a silent change.

## 6. Review the plan again, then back to IMPL

Open the plan gate as in `references/plan.md`, step 6: flush the logs, check the links,
post the change summary (one line per outcome), then `annotate PLAN.md --gate`. The
delta shows what the review changed. Handle every annotation as in step 6, and open the
gate again when needed.

On the explicit approval, follow `references/plan.md`, step 7: run `pair.py fresh --base
<worktree-base> --then impl`. The restarted writer implements the plan from the current
git diff: it builds what is new and changes or reverts what the review changed.

## Common mistakes

| Mistake | Fix |
|---|---|
| Shows all steps in one message | One step per turn, then `ask-user` |
| Goes to the next step while the user still discusses | Stay until the user picks an outcome |
| Edits `PLAN.md` during the walk | Only the planner changes it, once, in step 5 |
| Renumbers steps after a move | Ids never change; the order changes |
| Starts coding after the walk | No code until the plan gate approval |
| Runs `stage plan` to reopen | Only `fresh --then reviews` reopens an approved plan |
