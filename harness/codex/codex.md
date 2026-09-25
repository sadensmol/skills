# Runtime: Codex

You are in Codex. This file answers the capability contract in `harness/README.md`.

Its local tool surface is: shell/unified exec, `apply_patch`, `update_plan`,
`request_user_input`, `view_image`, web search, MCP tools, and the collaboration tools
`spawn_agent`, `followup_task`, `send_message`, `wait_agent`, `interrupt_agent`,
`list_agents`. Anything a skill asks for that is not on that list does not exist here — do
not simulate it, say so and take the stated fallback.

## Global config locations

- Load global instructions and configuration only from `$CODEX_HOME` (`~/.codex` unless
  that variable is set).
- Load global skills only from `~/.agents/skills` and `$CODEX_HOME/skills`.
- Never load or inspect files under `~/.claude` or `~/.config/opencode` — those trees
  belong to other harnesses.

`~/.codex/AGENTS.md` is a symlink to `instructions/AGENTS.md` in the skills repository, the
same file Claude Code reads as `~/.claude/CLAUDE.md` and OpenCode as
`~/.config/opencode/AGENTS.md`, so the standing instructions are identical in all three.
Anything that holds only for Codex belongs in this file, never in that shared one.

## load-skill

There is no skill tool here. The session's `<skills_instructions>` block lists every skill
as a name, a description and a short path against a root table (`r1` =
`~/.agents/skills`), so `r1/sadensmol/router/SKILL.md` is
`~/.agents/skills/sadensmol/router/SKILL.md`. To load a skill, **read that `SKILL.md` and
follow it** — here that is what invoking a skill means, and it is not optional when a skill
names another one.

Skill ids are flat and namespace-prefixed (`sadensmol-go-programming`); Codex takes them
from the frontmatter `name`, not from the directory. There is no colon form. The user can
also pick one with `/skills` or by typing `$` and the name.

Do not read a skill again when its full instructions are already present in the current
context. Apply the existing instructions to the new request. Reload only when they are
absent, such as in a fresh subagent or after compaction.

## ask-user

`request_user_input`. Use it for every approval gate a skill demands; do not assume consent
and continue. It does **not** exist in non-interactive runs (`codex exec`) — there, stop at
the gate and report what you need instead of deciding for the user.

## track-todos

`update_plan`, and it is expected: keep the plan synchronized with the real state of a
multi-step flow. A single read-only action does not need one.

## run-background-shell

Unified exec: start the command in a shell session and poll it with `write_stdin` rather
than waiting on it. Nothing notifies you when it exits, so re-poll, and redirect output to
a file when you need all of it. The user sees these under `/ps` and can end them with
`/stop`.

## spawn-subagent

`spawn_agent`. The built-in agents are `default`, `worker` and `explorer`; the reviewers in
`agents/code-reviewer/` are installed as custom agents (`quality`, `testing`,
`documentation`, `implementation`, `simplification`), so a skill that names one gets it by
that name. `fork_turns` decides how much of your context the child inherits,
`followup_task` gives a running agent more work, and `send_message` passes it something
without triggering a turn.

Codex tells you not to delegate unless the user, `AGENTS.md`, or a skill asks for it. A
skill that says "fan out" or "run this as a subagent" **is** that instruction — follow it.

## fan-out

Several `spawn_agent` calls, then `wait_agent` to collect them. There is no deterministic
orchestration script here — no equivalent of a workflow with phases, typed schemas or
pipelines — so issue one agent per unit of work and assemble the results yourself, keeping
each brief self-contained so a failed branch does not poison the rest. `list_agents` shows
what is running.

## background-subagent

`spawn_agent` returns without blocking, but **nothing notifies you when the child
finishes**: call `wait_agent` at the point you actually need the result. Do not poll it in
a loop and do not duplicate its work meanwhile.

## stop-subagent

`interrupt_agent`.

## isolate-work

No worktree tool. Create one with plain git in the shell and keep every path under it for
the rest of the session. Note that the sandbox limits writes to the workspace roots, so a
worktree outside them needs the user's approval first.

## Also here

Hooks — `UserPromptSubmit`, `SessionStart`, `PreToolUse`, `Stop` and the rest — which is
how the router reaches you every turn. Codex skips a new or changed hook until it is
trusted, so after installing or editing one the user must run `/hooks` and trust it;
say so rather than assuming the hook ran.

Custom agents are TOML files in `~/.codex/agents/`, generated from `agents/` by
`harness/codex/install.sh`, so an edit to an agent needs that installer re-run.

## Not here at all

Plan mode, typed finding reports, published artifacts, and a stop tool for background
shells. Commands from `commands/` arrive as `/prompts:<name>`, not `/<name>`.
