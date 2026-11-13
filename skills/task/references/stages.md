# Changing the task stage

Read this when you change the task stage (`pair.py stage … impl`).

A running session cannot change its model, so a stage change restarts the sessions: run
`python3 <sadensmol-task-skill-dir>/scripts/pair.py stage --base <worktree-base> impl`
(add `--single` in a single session opened beside a pair). It records the stage, respawns
the writer terminal with `--resume` of its conversation on the stage model, and in pair
mode starts the reviewer again in the new split (`launch --reopen`). The restart runs
detached: tell the user in one line that both sessions restart and continue, then end
the turn. It refuses with exit 5 while a pair thread is open: close the thread first.
It also refuses with exit 5 while the task session has a companion, because closing the
task session closes its companions: wait for the companion or close it, then run it again.
`stage` always resumes. A restart in new conversations is `pair.py fresh`
(`references/fresh.md`): it runs after the planner writes `PLAN.md` (`--then plan`),
after the user explicitly approves `PLAN.md` (`--then impl`) and at the start of
`finish` (`--then finish`).

The stage goes one way (`SKILL.md` → *Stages*). On IMPL, `stage plan` and `fresh --then
plan` refuse with exit 1, before any write or restart, and before the exit 5 check. No flag
overrides it. The one way back is `fresh --then reviews`, run only for the user's
`reviews` (`references/reviews.md`): it clears `impl` in both records, after the exit 5
checks, and restarts on PLAN. `impl` in either record (`.pair/config.json` or `.task-stage`) counts for
both modes, so a stale record never reopens the plan. A task never on IMPL keeps its plan
stage: `stage plan` changes nothing, and `fresh --then plan` runs.
