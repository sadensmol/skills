# `plan` — shared flow (the project skill supplies the tracker steps)

**Triggers:** "task plan", "plan task", "start plan". `new` auto-starts it in a fresh
worktree; it also runs manually from inside a worktree. These rules bind every
project, including new ones.

**OpenCode mode:** Start and run this flow in the write-enabled `task` mode, including
when a new session begins with `task plan`. Do not use OpenCode's read-only `plan` mode.
This flow writes `PLAN.md` and opens an approval gate; it does not edit application code
until the user approves the plan.

1. **Resolve the worktree base and branch.** The cwd is the base
   (`…/.worktrees/<branch>/`, a non-git dir holding the repos); the branch is its
   basename.
2. **Tracker steps — from the project skill:** derive the issue id from the branch,
   fetch the issue (title, description, acceptance criteria, comments — the source of
   truth), and mark it started. No id in the branch → skip the tracker and ask the user
   what the task is (or plan from the branch name).
   **The PLAN stage is on from here** (`SKILL.md` → *Stages*): the sessions run on the
   planning model until the user approves `PLAN.md`. A session that `new` started for
   this task already does; a session on IMPL runs `pair.py stage --base
   <worktree-base> plan` first.
3. **Clarity gate — never plan what you do not understand (MUST FOLLOW).** Before any
   brainstorming, `PLAN.md`, or plannotator gate, you must be able to answer all of:
   - **Observed behaviour** — the concrete symptom: failing request/response, exact
     error text, stack trace or log lines, the env, and the affected entity (the
     project skill names which: partner/ids, device/build, …).
   - **Expected behaviour** — acceptance criteria, or the exact desired output.
   - **Where** — the repo/service/screen/flow, precise enough to point at the code.
   - **How to reproduce or verify** — steps, a request, a test, or real data.

   Close the gaps yourself first: the whole ticket, every comment and every
   attachment/image, then the worktree's repos. Never ask for what the ticket contains,
   and never ask the user to fetch what you can fetch. If anything is still missing, or
   you catch yourself inventing the scenario from a one-line title: **stop** — no
   `PLAN.md`, no plannotator, no code. Ask ONE short concrete list in chat (`ask-user`
   when the options are enumerable): the raw log excerpt or stack trace the ticket came
   from, the real request/response and failing ids, the env/build/partner/device and
   since when, screenshots or the alert, what "done" means without acceptance criteria,
   or which of two readings is meant. **Wait** for the answer. A plan on a guessed
   scenario gets approved, built and reviewed against the wrong problem.
4. **Brainstorm.** Invoke `superpowers:brainstorming` to align on intent with the user.
   The main session holds this conversation; the planner cannot ask the user.
5. **Planner writes `<worktree-base>/PLAN.md`** — a `planner-subagent` (`SKILL.md` →
   *Session branches*, tier **planner**), in the foreground. Its brief: the base and
   repos, the ticket facts from step 2–3, the user's decisions from step 4, and the
   output path. `PLAN.md` must carry all of them: it is the only context left after the
   reset of step 6. It also carries `## Model decisions`: one entry per choice the
   planner made that neither the ticket nor the user settled, in the entry format of
   `SKILL.md` → *Model decisions log* (`where: PLAN.md` or the planned file:symbol), so
   the plan gate and `reviewd` show what the model chose on its own. It explores the relevant repos so the plan rests on the real code,
   states which repos the change touches, and writes `PLAN.md` at the base, never inside
   a repo, so it never reaches a PR. It validates any mermaid block first
   (`references/mermaid.md`). It writes no other file.

   **Every later change to `PLAN.md` goes to the planner too** — gate annotations, a
   pair reviewer's objection, or a user request to add, drop or refactor a part, before
   or after approval. Continue the same planner when it is still available, else start
   a new one with the current `PLAN.md` and the change. The main session never edits
   `PLAN.md` itself. A rework after approval goes back to PLAN first: run `pair.py stage
   --base <worktree-base> plan` (`SKILL.md` → *Stages*) before the planner, and `stage
   … impl` after the user approves the new `PLAN.md`.
6. **Open the gate** — the plan discussion starts in a clean context (MUST FOLLOW).
   When the planner has just written the first `PLAN.md` and your conversation did not
   start with the `Plan discussion:` prompt, do not open the gate yourself. Run `python3
   <sadensmol-task-skill-dir>/scripts/pair.py stage --base <worktree-base> plan --fresh`
   (`SKILL.md` → *Stages*): it restarts every agent — the writer, and the reviewer in
   pair mode — in a new conversation whose only context is `PLAN.md`. Tell the user in
   one line that the sessions restart with a clean context for the plan discussion, then
   end the turn. The restarted writer opens the gate. A session that started with
   `Plan discussion:` opens it directly, and reworks and re-opens during the discussion
   never reset again.

   Open it in the background, through the helper (SKILL.md →
   *Plannotator binary (shared)*):

   ```bash
   cd <worktree-base> && bash <sadensmol-task-skill-dir>/scripts/plannotator-session.sh annotate PLAN.md --gate
   ```

   `--gate` adds an Approve button. Address every annotation and re-open when needed.
   Editing `PLAN.md` while the gate is open makes it stale: follow the stale-`annotate`
   rule (ask the user to submit/close, wait, relaunch) — never force-close it. Do not use
   `plannotator review` here: there is no diff yet.

   **Approval is explicit, never inferred (MUST FOLLOW).** `Resolved: PLAN.md`, a closed
   window, an empty annotation set, or a completed background notification only mean the
   session ended. Approved = the tool reports an explicit Approve, or the user says so in
   chat ("approved", "lgtm", "go", "implement"). Unsure → it was not approved; ask.
7. **Approved → switch to IMPL, then implement at once.** Run `python3
   <sadensmol-task-skill-dir>/scripts/pair.py stage --base <worktree-base> impl`
   (`SKILL.md` → *Stages*): tell the user in one line that both sessions restart on the
   main model and continue, then end the turn. The restarted session implements.
   Approval is the instruction to implement; do not ask "should I implement?" or wait
   for another command.

**Until approved, `plan` writes no code and runs no verification (MUST FOLLOW).** No
source, test or config edit, no test run, no local infra start/stop, no deploy, no
change to an external environment. Read-only inspection to close a clarity gap is
allowed. Running another subcommand (`review`, `finish`, …) is not approval.
