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
   basename. Then run `python3 <sadensmol-task-skill-dir>/scripts/pair.py plan-open --base
   <worktree-base>`. Exit 1 means the task is on IMPL and the plan is closed (`SKILL.md` →
   *Stages*): stop here. No restart, no gate. Tell the user that the plan is closed and
   that the request is implementation work, then handle it as implementation work.
   **Code already exists → no plan (MUST FOLLOW).** Before `plan-open`, run `git status
   --short` in each repo under the base. Source changes mean the task is in implementation,
   even when the stage record says PLAN. Skip `plan-open` and do the request as
   implementation work. Only an explicit `task plan` from the user overrides this.
2. **Tracker steps — from the project skill:** derive the issue id from the branch,
   fetch the issue (title, description, acceptance criteria, comments — the source of
   truth), and mark it started. No id in the branch → skip the tracker and ask the user
   what the task is (or plan from the branch name).
   **The PLAN stage is on from here** (`SKILL.md` → *Stages*): the sessions run on the
   planning model until the user approves `PLAN.md`. A session that `new` started for
   this task already does.
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
   and never ask the user to fetch what you can fetch.
   Then sort what is still missing. **Do not stop the planning to ask (MUST FOLLOW).**
   A gap that changes what is built or how it is accepted (the raw log excerpt or stack
   trace, the real request/response and failing ids, the env/build/partner/device and
   since when, what "done" means without acceptance criteria, which of two readings is
   meant) becomes a question in `PLAN.md` → `## Questions to clarify`
   (`sadensmol-plan` → *Questions to clarify*): the plan is written on the most likely
   reading, says so in `assumed:`, and tags every item that rests on it
   `(assumes Q<n>)`. Any other gap goes into `PLAN.md` as an open item with how the plan
   checks it, never as a fact. Never present a guessed scenario as a fact: a plan on a
   hidden guess gets approved, built and reviewed against the wrong problem.
   The one stop: there is no task at all (no issue, no description, a branch name that
   says nothing) — ask the user what the task is.
   Log every question and its answer (`SKILL.md` → *Q&A log*), here and in every later
   step until the approval. Before `PLAN.md` exists, `pair.py` holds the entries for it.
4. **Brainstorm without the user.** Invoke `superpowers:brainstorming` for its method
   only. Answer its questions yourself from the ticket, the code and the conventions.
   Compare two or three approaches, pick the best fit, and log the choice as a model
   decision (`SKILL.md` → *Model decisions log*). The plan gate is its design approval:
   write no design doc and commit nothing. Ask the user nothing here: a critical
   question (`SKILL.md` → *Autonomy*) goes into the brief for the planner, which writes
   it to `## Questions to clarify`. The planner cannot ask the user.
5. **Planner writes `<worktree-base>/PLAN.md`** — a `planner-subagent` (`references/session-branches.md`,
   tier **planner**), in the foreground. Its brief: load `sadensmol-plan` and follow its
   structure (it owns the content of `PLAN.md`), the base and
   repos, the ticket facts from step 2–3, the decisions from step 4, and the
   output path. `PLAN.md` must carry all of them: it is the only context left after the
   reset of step 6. It also carries `## Decisions`: one `U<n>` entry per user decision
   (the user's words, what the plan does with them and what was changed) and one `D<n>`
   entry per choice the planner made that neither the ticket nor the user settled, in
   the entry formats of `sadensmol-plan` → *The logs* (`where: PLAN.md` or the planned
   file:symbol), so the plan gate and `reviewd` show what was decided and what was done.
   It also carries `## Questions to clarify` (every question planning could not settle,
   each with its `assumed:` answer, `none` when empty) and `## Q&A`, the answered
   questions logged so far (empty when none). Both sections
   are logs that `pair.py` appends to during PLAN (`SKILL.md` → *Q&A log*): the planner
   keeps every entry in them verbatim, never rewrites, merges or drops one (it updates
   only `done:` lines), and adds its own entries at the end of `## Decisions` with the
   next `U<n>` or `D<n>`. `PLAN.md` → `## Task` carries
   `### Not requested`: one line for every repo, file, status change or default that goes
   beyond the ticket and the user's words (`none` when empty). Each line needs an explicit
   user yes; a general plan approval does not cover it. A user correction outranks plan
   order: it goes into the next slice, and before you say "start testing", list every open
   user request. It explores the relevant repos so the plan rests on the real code,
   states which repos the change touches, and writes `PLAN.md` at the base, never inside
   a repo, so it never reaches a PR. It validates any mermaid block first
   (`references/mermaid.md`). It writes item references as `[ID]` and, after each write,
   runs `python3 <sadensmol-plan-skill-dir>/scripts/plan-links.py PLAN.md fix`, then
   `… check`, which must exit 0 (`sadensmol-plan` → *Item links*). It writes no other file.

   **Every later change to `PLAN.md` before the approval goes to the planner too** — gate
   annotations, a pair reviewer's objection, or a user request to add, drop or refactor a
   part. Continue the same planner when it is still available, else start a new one with
   the current `PLAN.md` and the change. The main session never edits `PLAN.md` itself;
   the only exception is the log appends of `SKILL.md` → *Q&A log*, through `pair.py`.
   The approval freezes `PLAN.md`: after it, nobody changes the file (`SKILL.md` →
   *Stages*).
6. **Open the gate** — the plan discussion starts in a clean context (MUST FOLLOW).
   When the planner has just written the first `PLAN.md` and your conversation did not
   start with the `Plan discussion:` prompt, do not open the gate yourself. Run `python3
   <sadensmol-task-skill-dir>/scripts/pair.py fresh --base <worktree-base> --then plan`
   once (`references/fresh.md`): it restarts every agent — the writer, and the reviewer
   in pair mode — in a new conversation whose only context is `PLAN.md`. Tell the user in
   one line that the sessions restart with a clean context for the plan discussion, then
   end the turn. The restarted writer opens the gate. A session that started with
   `Plan discussion:` opens it directly, and reworks and re-opens during the discussion
   never reset again.

   **Questions to clarify — settle what you can, then the user answers one at a time
   (MUST FOLLOW).** Read all of `PLAN.md` → `## Questions to clarify`. A question is open
   while it has an empty `answer:` and is not struck through.

   1. **Before each gate opening, settle what needs no user.** For each open question:
      - Look for the answer in the sources. Found → log it with its evidence and
        `resolves: Q<n>` (`pair.py qa`).
      - A noncritical choice (`SKILL.md` → *Autonomy*) → decide it alone; in pair mode,
        with the reviewer only when it is hard (`pair.md` → *Decision weight*). Log a `D<n>`, then resolve `Q<n>` with the decision
        reference.
      - Send what you settled to the planner (step 5) before the gate opens.
   2. **Open the gate with the remaining questions in the plan.** Do not ask them first.
      The user reviews the plan and its questions together, and may answer a question
      by annotating its `Q<n>` or filling its `answer:` line.
   3. **After the gate returns, ask what is still open — one question at a time.** Take
      the questions by the section's order (biggest impact first). For each one, ask
      the user with `ask-user`, one question per call: the recommended option first,
      marked `(Recommended)`, with its reason, and the `assumed:` option named. In pair
      mode, settle a hard recommendation with the reviewer first. Wait for the answer, log
      the user's words with `pair.py qa` and `resolves: Q<n>`, then ask the next one.
      Do not ask the next question in the same call, and do not rework between
      questions.
   4. **Rework once.** Send all the answers, and the gate annotations, to the planner in
      one brief as user decisions (step 5). An answer equal to `assumed:` changes no plan
      item. Then open the gate again (the delta shows the change).

   The user approves the plan while a question is still open → ask the open questions
   (step 3) before step 7. Every answer equals its `assumed:` → the approval stands; the
   planner closes the questions with no other change. An answer that changes the plan →
   rework and open the gate again; that approval no longer counts.

   "Later" from the user is an answer: log it, the `assumed:` answer stays, and the
   planner marks what waits for it. A deferral never lets you choose for the user.

   Open it in the background, through the helper
   (`references/plannotator.md`). The plan gate keeps its own baseline, apart from the
   code `review` one. Check it first:

   ```bash
   H=<sadensmol-task-skill-dir>/scripts/plannotator-session.sh
   S=<sadensmol-task-skill-dir>/scripts/review-snapshot.sh
   python3 <sadensmol-task-skill-dir>/scripts/pair.py plan-log-flush --base <worktree-base>
   cd <worktree-base> && bash "$S" plan-delta
   ```

   `plan-log-flush` writes the Q&A and decision entries held while the gate was open
   (`SKILL.md` → *Q&A log*), so the delta shows them. Then check the item links,
   read-only: `python3 <sadensmol-plan-skill-dir>/scripts/plan-links.py
   <worktree-base>/PLAN.md check`. Exit 1 → send its lines to the planner (step 5), which
   runs `fix`, before the gate opens. The main session never runs `fix` on `PLAN.md`.

   - `NO_PREVIOUS` (first gate) or `NO_DELTA` (`PLAN.md` unchanged; say "No changes since
     the last review.") → open the gate, no guide:

     ```bash
     cd <worktree-base> && PLANNOTATOR_PROMOTE=plan bash "$H" annotate PLAN.md --gate
     ```

     `PLAN.md` follows `references/plannotator.md` → *Theme*.

   - `DELTA <patch>` → read the patch. Post a change summary in chat: one line per reason
     (a gate annotation, a pair objection, a user request), with the item ids and what
     changed. Then open the gate, no guide, with the same `annotate` command as above. No
     separate diff view: it costs the user one more round. Open the guided diff view
     (guide checked with `bash "$S" guide --plan <guide>`, then `PLANNOTATOR_GUIDE=<guide>
     bash "$H" review --patch-file .scratch/plan-review/delta.patch`) only when the user
     asks for it.

   The baseline moves only when the gate page opens (`PLANNOTATOR_PROMOTE=plan`); the
   diff view never moves it. `--gate` adds an Approve button. Address every annotation
   and re-open when needed. Editing `PLAN.md` while the gate is open makes it stale:
   follow the stale-`annotate` rule in `references/plannotator.md`; preserve review notes
   before closing or relaunching.

   **Approval is explicit, never inferred (MUST FOLLOW).** `Resolved: PLAN.md`, a closed
   window, an empty annotation set, or a completed background notification only mean the
   session ended. Approved = the tool reports an explicit Approve, or the user says so in
   chat ("approved", "lgtm", "go", "implement"). Unsure → it was not approved; ask.
7. **Approved → switch to IMPL, then implement at once.** Only after the explicit
   approval, run `python3 <sadensmol-task-skill-dir>/scripts/pair.py fresh --base
   <worktree-base> --then impl` (`references/fresh.md`): it restarts both sessions on the
   main model in new conversations. Tell the user in one line that both sessions restart
   with a clean context and implement, then end the turn. The restarted writer starts
   with the `Implementation:` prompt and implements. Approval is the instruction to
   implement; do not ask "should I implement?" or wait for another command. This switch is
   one-way (`SKILL.md` → *Stages*): `pair.py` and the gate scripts refuse the plan stage,
   the plan gate and a `PLAN.md` edit from now on. Only the user's `reviews` reopens the
   plan (`references/reviews.md`).

**Until approved, `plan` writes no code and runs no verification (MUST FOLLOW).** No
source, test or config edit, no test run, no local infra start/stop, no deploy, no
change to an external environment. Read-only inspection to close a clarity gap is
allowed. So are the R&D measurements (`sadensmol-plan` → template → *R&D*): existing
tests, benchmarks and tools run unchanged in a session branch, and read-only metrics,
logs and queries. Running another subcommand (`review`, `finish`, …) is not approval.
