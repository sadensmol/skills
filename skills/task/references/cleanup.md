# `cleanup` — shared flow (the project skill supplies the script and its specifics)

**Triggers:** "task cleanup", "cleanup task", "delete task", "task delete".

`cleanup` removes the task's worktree(s), local branches, agterm session, every console
and infra session it opened, and every session branch. It destroys the only local copy
of the work, so it has a hard readiness gate.

## Readiness gate (MUST FOLLOW)

Verify — never assume, and never take the user's "it's done" as the check — and record
explicit values in the temporary `<worktree-base>/.task-cleanup-checklist` (deleted with
the worktree) before running the script. The project supplies the tracker and CI
commands:

- `planned_work=complete` — every planned item is done; cut scope is `related_work=none`
  or `related_work=tracked` with the tracker links.
- `related_prs=none` or `related_prs=verified` — every related PR is approved, merged
  (approved is not merged; "ready to merge" is not merged), and has only `SUCCESS` or
  `SKIPPED` checks, including e2e, regression, deploy and other external checks.
- `commits=complete` — every change committed; every task worktree clean.
- `tests=green` — local tests, CI/CD checks, e2e and regression runs, where they apply.
- `problems=none` or `problems=tracked` — every bug found is fixed or has a new tracker
  task; record each key/link (comments in the checklist).
- `ticket_updates=confirmed` — the task's issue and every follow-up reflect the final
  scope and outcome.
- `infra=clean` — every session branch and infra console closed; no task-owned Docker
  containers, networks or volumes; no `.infra.lock` holding a stack that is still up.

The project's `cleanup-task.sh` validates the checklist, verifies every related PR is
approved and merged with `SUCCESS`/`SKIPPED` checks, rejects dirty worktrees, and
removes task-owned Docker resources. It refuses when any check fails — report every
failed guard and stop; the next step is fixing that check or waiting for the merge,
never removing the worktree. Never bypass it "to just clean up".

**`--force`** only for a deliberately abandoned task, an environment that cannot be
verified, or an explicitly accepted final state that cannot meet a historical
approval/check gate — and only after the user explicitly confirms the destructive
action (say plainly that unmerged work is about to be destroyed). Never on your own
initiative.

**Cleanup state is non-retroactive.** A failed cleanup gate or a `--force` requirement
changes only cleanup eligibility. It never turns a verified `MERGED` PR or an accepted
completed task into an unmerged PR or an unfinished task. Accepting completion does not
authorize `--force`; that needs its own confirmation.

## Steps

1. **Which worktree.** Inside one → take its name and confirm through `ask-user`:
   "Delete worktree `<name>`? This removes the local branch(es) and the worktree
   directory." Outside → list worktrees and let the user pick (`ask-user`).
2. **Close every session branch of this task — before the script (MUST FOLLOW).**
   Branches are in-process agents, so no shell script can stop them. List the task's
   live branches and stop each one (`stop-subagent`) — test runs, CI babysitters, e2e
   gates, research or doc lanes — running or already reported. A branch left alive can
   open a console or bring a stack up inside a worktree being deleted. Report the closed
   list in one line (or say there were none). Without `stop-subagent`, say so and ask the
   user to close them from the UI before you run the script; never claim you stopped one.
3. **Complete the checklist** with the values above.
4. **Run the project's script** (`bash <project-skill-dir>/scripts/cleanup-task.sh
   <branch>`). Before deleting anything it sweeps the task's consoles:
   - **found by `cwd`, not by name** — every agterm session whose cwd is the worktree
     base or under it (`agtermctl tree --json`, needs `jq`); consoles are named after
     their directory, so a name match misses them and they keep holding host ports;
   - the task's own session and the session running the script are excluded;
   - **each console's stack is stopped before it is closed** (Ctrl-C, the project's
     teardown target, wait) — closing a pty does not stop its containers;
   - this runs before the worktree is removed, since the teardown needs the compose file;
   - no `jq` → the script refuses; bypass only with the user's explicit `--force`.

   Then it removes Docker resources, releases `.infra.lock`, removes the worktrees, deletes
   the local branches and the directory, and closes the task's agterm session (delayed a
   couple of seconds; closing the session you are in is safe — agterm selects a
   neighbour). Repeat the printed list of closed consoles back to the user.
5. **A merged branch must not survive on GitHub.** The script deletes local branches
   only; check each repo's remote branch and delete it when — and only when — its PR is
   merged (`sadensmol-github` → "Delete the remote branch after a merge"). Never delete
   the remote branch of an open PR.
6. Tell the user the task's agterm session is closing and agterm will select another.
