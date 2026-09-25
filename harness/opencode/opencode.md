# Runtime: OpenCode

You are in OpenCode. This file answers the capability contract in `harness/README.md`.

Its tool registry is: `bash`, `read`, `write`, `edit`, `patch`, `glob`, `grep`, `list`,
`webfetch`, `skill`, `task`, `question`, `todowrite`. Anything a skill asks for that is not
on that list does not exist here — do not simulate it, say so and take the stated fallback.

## Global config locations

- Load global instructions and configuration only from `~/.config/opencode`.
- Load global skills only from `~/.agents` and `~/.config/opencode`.
- Never load or inspect files under `~/.claude` — that tree belongs to another harness.

`~/.config/opencode/AGENTS.md` is a symlink to `instructions/AGENTS.md` in the skills repository,
the same file Claude Code reads as `~/.claude/CLAUDE.md`, so the standing instructions are
identical in both harnesses. Anything that holds only for OpenCode belongs in this file,
never in that shared one.

## load-skill

The `skill` tool: `skill({ name: "sadensmol-go-programming" })`. Skill ids are flat and
namespace-prefixed; there is no colon form. Reading the `SKILL.md` file is not a
substitute.

The tool does not deduplicate calls. Before calling it, check whether a prior
`<skill_content name="…">` block for that skill is still present in the current context.
If present, do not call the tool again; apply those instructions to the current request.
Reload only when the block is absent, such as in a fresh subagent or after compaction.

## ask-user

The `question` tool — for clarifying questions and for weighing tradeoffs. The global
OpenCode config must explicitly set `"question": "allow"` under `permission`; otherwise
the tool may not be exposed. Use it for every approval gate a skill demands; do not assume
consent and continue.

If `question` is absent from the live tool registry, use the same flat numbered choice in
chat and wait for the user's selection. Never claim that the capability is unavailable
when the chat fallback can perform the same gate.

## track-todos

`todowrite`, and it is expected: keep the visible list synchronized with the real state of
a multi-step flow. A single read-only action does not need a list. Note that `todowrite`
can be denied by config, in which case keep the plan in your reply.

## browser

Use the existing Chrome DevTools page for browser work. Call `chrome-devtools_list_pages`, select an
existing page, and navigate it with `chrome-devtools_navigate_page`. Do not call
`chrome-devtools_new_page` unless the user explicitly asks for a new tab.

## run-background-shell

`bash` with a trailing `&`, for commands that will not stop on their own. A background
process **cannot read terminal input and cannot write to terminal output**, so redirect its
output to a file and read that. Nothing notifies you when it exits.

## spawn-subagent

The `task` tool — "launch a new agent to handle complex, multistep tasks autonomously".
**Foreground is the default**, and the harness itself says to use it when you need the
result before continuing. The agent id is the filename of its definition, so the reviewers
in `agents/code-reviewer/` are `quality`, `testing`, `documentation`, `implementation`,
`simplification`.

## fan-out

Several `task` calls. There is no deterministic orchestration script here — no equivalent of
a workflow with phases, typed schemas or pipelines. A skill that describes one must instead
issue one `task` per unit of work and assemble the results itself, keeping each brief
self-contained so a failed branch does not poison the rest.

## background-subagent

`task` with `background: true` launches asynchronously and returns immediately, and **you
are notified automatically when it finishes**. The harness is explicit about the rest: do
not sleep, do not poll for progress, do not ask the task for status, and do not duplicate
its work while it runs.

This mode requires `OPENCODE_EXPERIMENTAL_BACKGROUND_SUBAGENTS=true` in the environment. If
it is not set, treat `background-subagent` as absent and run the work in the foreground.

## stop-subagent

No tool for it in the registry. A running task is interrupted by the user from the TUI, so a
skill must not promise to cancel one — it should avoid launching work it cannot finish.

## isolate-work

No worktree tool in the registry. Create one with `bash` and plain git, and keep every path
under it for the rest of the session. There is no session fork either; `opencode session`
offers only `list` and `delete`.

## Not here at all

Hooks, plan mode, typed finding reports, scheduled runs and published artifacts have no
OpenCode counterpart. Standing instructions arrive through `AGENTS.md` and the
`instructions` list in `opencode.json` instead of a per-prompt hook, which is why the router
is loaded once per session here rather than re-injected every turn.
