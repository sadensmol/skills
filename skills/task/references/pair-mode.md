# Pair mode — launch and mode mechanics

The protocol both agents follow is `../pair.md` (each agent gets its path in its start
brief). A liaison subagent reads only `../pair-liaison.md`; a thread reviewer reads only
`../pair-thread.md`. This file is for the project skills' `new` and for mode changes.

`--pair` makes the project's `new-task.sh` open the task session as a split: the
**writer** (driver, the only agent that edits) in the left pane, a read-only **reviewer**
of the other agent kind (navigator) in the right pane — Claude Code ↔ OpenCode. The
reviewer runs on the model of the task stage, as the writer does (`../SKILL.md` →
*Stages*; the ids are in `MODELS` in `scripts/pair.py`). Without `--pair` nothing changes. Apart from the decision loop it is
the same task flow with two agents: the writer is seeded with `task plan` by the same
rule as a solo session, every user gate is unchanged, and the plan gate opens only after
the reviewer has answered on `PLAN.md`.

## Mechanics — `scripts/pair.py`

- `init`: create `.pair/` at the worktree base (cleanup removes it with the base).
- `brief`: the start text for a role.
- `launch`: split, reviewer start, and lookup of the session OpenCode created from its
  `--prompt` (OpenCode always starts with a first prompt, never on its start screen).
- `restart-reviewer [--check]`: kill a dead or stuck reviewer (identity-checked by its
  command line) and start it again in the right pane with the catch-up brief. An
  OpenCode reviewer gets a fresh port and session, and its old threads are closed; a
  lock and `reviewer_generation` in `config.json` make a second caller a no-op.
  `PAIR_FREE_PORT` is a test seam for the new port.
- `stage plan|impl`: the stage change (`../SKILL.md` → *Stages*). It respawns the writer
  with the command the project launcher prints (`new-task.sh <branch> --print-command`),
  then runs `launch --reopen`. `claude-args` / `opencode-args` print the stage flags of
  a launch and record the session id and the launcher.
- `send` / `wait` / `status`: the mailbox. A Claude Code agent runs `wait --follow` under
  the Monitor tool: one event per batch, no re-arm per batch. `send` holds an `FYI:` in
  `.pair/held/<role>/` and releases the held ones as ONE message with the next other
  message to that role, a forwarded user message, `idle`, `flush`, or at
  `PAIR_FYI_BATCH` (default 5) held FYIs: every delivery costs the peer a turn.
- `edit-guard`: the `PreToolUse` hook of a Claude writer (`Edit|Write|MultiEdit|
  NotebookEdit`, subagents included). It denies an edit to a file an `open` thread
  covers; an agreed or closed thread, `.pair/` and paths outside the base pass. It
  never blocks on its own error.
- `decided --topic <t> --stdin` and `thread close --status agreed|disputed` append the
  decision to `.pair/decisions.md`, the log both agents catch up from after `/clear` or
  a restart.
- `thread open|send|resume|wait|close` / `threads`: one decision per thread, discussed
  with a reviewer subagent (an OpenCode child session), in parallel with other threads.
  A Claude Code writer runs each thread in a background liaison subagent, so the
  exchange stays out of its context (`pair.md` → *Liaison*). With an OpenCode
  reviewer, `send` refuses a writer `DECISION:`/`VIEW:`/`MERGE:` (exit 7): decisions
  never fall back to the one main reviewer session.
- Every prompt to OpenCode (main session or thread child) is confirmed: `prompt_async`
  answers before it runs, so `pair.py` reads the session's newest messages until the
  prompt is stored (`PAIR_CONFIRM_SECONDS`, default 5). A dropped prompt fails the call;
  a failed `thread open` deletes its child session and leaves no thread.
- Routing: the writer's `user-prompt` hook lists the threads of the current reviewer
  (`PAIR THREADS, newest first:`) with the rule that sends a follow-up to its thread
  (`pair.md` → *Liaison*, step 0).
- `PAIR_REAL_OPENCODE=1 python3 -m unittest test_pair` also runs the tests against a
  real `opencode serve` (they start short model turns and abort them).
- `user-prompt`: the `UserPromptSubmit` hook of a Claude Code agent; it forwards each
  user message to the peer. `init` writes its settings to
  `.pair/claude-<role>-settings.json`, and a project's `new-task.sh` MUST start a Claude
  writer with `--settings` pointing at that file.
- `session-start`: the `SessionStart` hook (matcher `clear`) in the same settings file.
  After `/clear` it stops the stale waiter, refocuses an OpenCode peer's pane and
  re-arms the agent. `wait` also retires the stale waiter of its role.
- `pair-opencode-plugin.js`: loaded by the reviewer's OpenCode config; keeps the pane on
  the pair session after a `/new` there and forwards the user's text to it.
- A project's `new-task.sh` calls `pair.py init` and `brief` before `agtermctl session
  new`, and `pair.py launch` after it — only when it creates the session, never for an
  existing one.

## Changing a task's mode

`task switch --pair` / `task switch --single` apply to the selected worktree; `task
pair` enables pair mode for the current one. They delegate to the project skill with
`new <branch> --pair` or `new <branch> --single`:

- No live session → open one in the selected mode.
- A live session already paired → only select it.
- A live session without a reviewer → **replace** it: `pair.py respawn` opens a new
  session right after it and closes the old one. The writer resumes its recorded
  conversation (`claude --resume <id>` / `opencode --session <id>`, from `pair.py
  claude-args` / `opencode-args`); the reviewer starts with
  the catch-up brief. The script refuses while that writer is busy, unless the request
  comes from the writer itself. Tell the user the old session is being replaced first.

`switch --single` on a worktree started in pair mode opens a separate single-agent
session and keeps the paired one open. The project skill gives it a distinct name and
includes it in cleanup.

## `pair`

**Triggers:** "task pair", "turn on pair mode", "continue this task in pair mode".

Resolve the current worktree's `$branch` (SKILL.md → *Worktree helpers*). Outside a
worktree, say `task pair` must run from inside one (or use `task switch --pair`) and
stop. Then invoke the project `-task` skill with args `new <branch> --pair`.
