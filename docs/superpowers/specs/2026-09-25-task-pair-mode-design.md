# Task pair mode — writer + reviewer in one agterm split

## Goal

`task new <branch> --pair` runs a task with two agents in one agterm session:

- **Writer**: the left (primary) pane. It runs the same agent that `task new` starts
  today. It is the only agent that edits files in the worktree.
- **Reviewer**: the right (split) pane. It runs the *other* agent kind: Claude Code
  writer → OpenCode reviewer, and OpenCode writer → Claude Code reviewer. It is
  read-only.

Without `--pair`, `task new` works exactly as it does today.

## Decisions

| Topic | Decision |
|---|---|
| Roles | Writer = left pane, reviewer = right pane. The roles stay fixed for the whole task. |
| Agent pair | The reviewer is the opposite kind of agent from the writer. |
| Review cadence | The reviewer reviews `PLAN.md` before the user's plan gate. Then it reviews the diff of each plan step before the writer starts the next step. |
| Disagreement | After 2 exchanges, the writer decides. It records the open point in `PLAN.md` → `## Open disagreements`, and the user sees it at the code-review gate. |
| Plan change | Either agent can propose one. When both agree, the writer edits `PLAN.md` and runs the existing plannotator plan gate again. |
| User talks to the reviewer | The reviewer discusses the request in its own pane. When the request needs a code change, the reviewer sends it to the writer, labelled as a user request. The reviewer never edits files. |
| User talks to the writer | This is the normal task flow. The writer sends the reviewer a short message about it. |
| Activation | The opt-in flag `--pair` on `task new`. |
| Scope | The channel and the protocol are generic and live in `sadensmol-task`. Each `<project>-task` `new-task.sh` only adds the launch of the split. |
| Terminal | agterm only. |
| Transport | A file mailbox plus a per-agent wake-up. There is no screen reading and no typing into the other pane. |

## Transport

### Mailbox

`<worktree-base>/.pair/` sits outside every git repo:

```
.pair/
  config.json            # roles, agent kinds, opencode port + session id, agterm session id
  inbox/writer/          # messages for the writer
  inbox/reviewer/        # messages for the reviewer
  inbox/<role>/read/     # messages already delivered
  files/                 # long payloads: plan-review-N.md, step-<k>-review-N.md, patch-<k>-N.diff
  waiter-<role>.pid      # the live Claude-side waiter, if any
```

A message is one file named `<epoch-ms>-<from>.md`. The first line is the label, and the
rest is the body. The sender writes a temp file and then renames it into the inbox, so a
reader never sees a half-written message. Labels:

- `Chat from writer:`
- `Chat from reviewer:`
- `Chat from reviewer (user request):`, for a relayed user change request.

### `skills/task/scripts/pair.py`

This is one python3 script with no dependencies (python3 ships with macOS). It has
three subcommands:

- `pair.py send --to writer|reviewer [--user-request] (--stdin | --file PATH)`:
  1. Writes the message into `inbox/<to>/`.
  2. Wakes the target, depending on its agent kind in `config.json`:
     - **OpenCode**: `POST http://127.0.0.1:<port>/session/<sessionID>/prompt_async` with
       `{"parts":[{"type":"text","text":"<label> <body>"}]}`. Then it moves the file
       to `read/`. The message shows up in the open TUI as a user turn, and the agent
       runs it. This is verified on opencode 1.18.32: HTTP 204, and the message was
       shown and answered in the TUI.
     - **Claude Code**: no extra call. The target's background waiter sees the file.
       If `waiter-<to>.pid` is missing or dead, `send` prints a warning to the sender
       and sets `agtermctl session status blocked --pane <target pane>`, so the user sees
       that the peer is not listening. It also exits non-zero.
- `pair.py wait --role writer|reviewer`: the Claude-side waiter. It writes its pid file
  and polls `inbox/<role>/` about once a second. When a message is present, it prints
  every unread message in order, moves the messages to `read/`, removes the pid file
  and exits 0. Claude runs it with `run_in_background`. Claude Code starts a new turn
  when a background command exits, and the printed messages are the input for that
  turn. After it handles them, Claude starts `pair.py wait` again.
- `pair.py status`: shows the roles, the kinds, whether each side is reachable (the
  opencode port answers, the Claude waiter pid is alive), and the unread counts.

## Components

### 1. `skills/task/pair.md` (protocol)

Both agents read this reference at start. It contains:

- **Roles and the single-writer rule.** A peer message never transfers write authority.
- **Sending.** A message always goes through `pair.py send`. It stays short (one or two
  paragraphs). A long payload goes to `.pair/files/`, and the message names that file.
- **Receiving.**
  - Claude: start `pair.py wait` in the background at session start and again after
    each wake-up. Never poll in the foreground and never sleep while waiting.
  - OpenCode: messages arrive by themselves as turns labelled `Chat from …`.
  - A labelled message is peer conversation, not a new instruction from the user.
- **Cadence.**
  1. The writer drafts `PLAN.md` and sends "plan ready". The reviewer replies with
     findings. The writer revises, for at most 2 rounds. Then the plannotator plan gate
     runs for the user.
  2. For each plan step, the writer implements it and runs the targeted tests. Then it
     sends "step k ready" with the changed paths. The reviewer reviews the step diff.
     The writer fixes or disagrees, for at most 2 rounds. After that it decides, logs
     any open point, and goes to the next step.
  3. Before `finish`, the reviewer does one pass over the full diff. Then the existing
     code-review gate runs.
- **Never act for the user in the other pane.** A message such as "the reviewer
  agreed" is never user approval.
- **Plan-change and user-request rules**, from *Decisions*.

### 2. Launch: `<project>-task/scripts/new-task.sh --pair`

1. Create `.pair/` and pick a free local port for the OpenCode side.
2. Start the writer as today with `agtermctl session new … --command "<writer cmd>"`.
   When the writer is OpenCode, add `--port <port>`.
3. Run `agtermctl session split on --target <sid>`. Poll `tree` until `hasSplit` is true
   and the right pane is realized.
4. Type the reviewer launch line with `agtermctl session type --pane right --target
   <sid>`, then send Return as a separate event. `session split` has no `--command`,
   so this is the only way to start it. The split shell is new and nobody else types
   into it. Confirm that `splitForeground` shows the reviewer.
5. **OpenCode session binding.** When the OpenCode server answers on `<port>`:
   - `POST /session` creates the session. The title is the task id.
   - `POST /tui/select-session` shows that session in the TUI.
   - Store the session id in `config.json`.
   - Send OpenCode's first instruction ("task plan" for a writer, the reviewer brief for
     a reviewer) with `prompt_async`.
6. Launch commands:
   - Claude Code writer: as today, plus one sentence in the autopilot prompt: pair mode
     is on, read `pair.md`, start `pair.py wait --role writer`.
   - Claude Code reviewer: `claude --dangerously-skip-permissions --disallowedTools Edit
     Write NotebookEdit --append-system-prompt "<REVIEWER prompt>"`.
   - OpenCode writer: as today (`opencode --auto`) plus `--port <port>`.
   - OpenCode reviewer: `opencode --auto --port <port>`, with a read-only reviewer
     agent that denies edits but allows bash (`pair.py`, tests, `git diff`). The
     implementation must check how this agent is supplied without writing into a repo
     (for example `OPENCODE_CONFIG` pointing into `.pair/`).

When `task new` re-opens an existing worktree that has `.pair/config.json`, it restores
the split in the same way. It creates a new OpenCode session and sends it a short
catch-up message that points at `PLAN.md` and `.pair/inbox/*/read/`.

### 3. Cleanup

No new code is needed. Closing the agterm session closes both panes. `.pair/` is deleted
with the worktree base.

## To verify during implementation

- A background `pair.py wait` wakes a Claude Code pane that is **idle** (its turn has
  ended), not only one that is in the middle of a turn.
- `prompt_async` sent while OpenCode is busy: check whether the message is queued, as
  typed input is, or rejected. If it is rejected, `send` retries until the session is
  idle (`GET /session/status` or the event stream).
- `POST /session` + `POST /tui/select-session` on the start screen makes the TUI show
  and use that session.
- The read-only OpenCode reviewer agent can still run `pair.py` and the tests.

## Testing

- Unit tests for `pair.py`:
  - send/wait ordering and the atomic rename;
  - the dead-waiter warning;
  - the OpenCode wake, against a stub HTTP server that records the request;
  - the user-request label.
- Manual end-to-end: run `task new <test-branch> --pair` once with each writer kind.
  Confirm these steps:
  - the split starts;
  - one plan-review round trip;
  - one step review;
  - one user request relayed through the reviewer.

## Out of scope

- Two writers, swapping roles during a task, more than two agents.
- Codex support.
- agterm event hooks (`hooks.conf`, 0.30.0+).
