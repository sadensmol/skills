# Runtime: Claude Code

You are in Claude Code. This file answers the capability contract in `harness/README.md`.

## load-skill

The `Skill` tool, with the skill's flat id: `Skill(sadensmol-programming-go)`. Reading the
`SKILL.md` file instead does not count — the skill must be invoked.

Do not invoke a skill whose full instructions are already present in the current context.
Apply the existing instructions to the new request. Reload only when they are absent, such
as in a fresh subagent or after compaction.

## ask-user

`AskUserQuestion`. Up to 4 questions per call, 2-4 options each, `multiSelect` when the
choices are not exclusive. Options carry an optional `preview` for side-by-side comparison.

## track-todos

Not available in this build. Keep the plan in your reply text instead, and do not claim a
todo list exists.

## browser

The same Playwright MCP extension servers as OpenCode: the workspace server the project's browser
policy names when one is configured, otherwise `playwright-extension`. Read that policy before the
first browser call. Claude in Chrome (`mcp__claude-in-chrome__*`) is not the authenticated browser —
never use it for a project page.

1. The server's `browser_tabs` with `action: "list"`, then `action: "select"` an existing tab.
2. Its `browser_navigate` only when needed.
3. Never create or close a tab unless the user asks.

The named server is missing from this session → stop and say so. Never fall back to Claude in
Chrome, WebFetch, `curl` or another browser.

## run-background-shell

`Bash` with `run_in_background: true`. It keeps running across turns and re-invokes you when
it exits, so never poll it with `sleep`. Collect output with `TaskOutput`, wait on a
condition with `Monitor`, and kill it with `TaskStop`.

`Bash` runs zsh. Quote every word that starts with `=` or holds a glob (`echo '==='`,
`--include='*.go'`): unquoted, `===` fails with `=== not found`, and `--include=*.go` fails
with `no matches found`, so a search returns nothing without an error. macOS has no GNU `timeout`:
use the tool's `timeout` parameter. In zsh, `cmd 2>&1 >/dev/null | x` also sends stdout into the pipe.

## spawn-subagent

The `Agent` tool: `subagent_type` picks the agent, `prompt` carries the work. Subagents run
in the background by default; pass `run_in_background: false` only when your next action
depends on the result and nothing else could usefully happen meanwhile.

`ListAgents` shows what is running, and `SendMessage` continues one with its context intact
— a new `Agent` call always starts fresh. A skill's lane is `subagent_type:
"general-purpose"` on the default model unless it names another agent. `subagent_type: "fork"` instead inherits the
whole conversation, and every one of its turns re-reads it: never use it for a skill's
lane or branch; give a fresh agent a brief instead.

## cheap-subagent

`Agent` with `subagent_type: "general-purpose"`, `model: "sonnet"` and `effort: "high"`.

## planner-subagent

`Agent` with `subagent_type: "general-purpose"`, `model: "opus"` and `effort: "xhigh"`.
`SendMessage` continues the same planner for a revision.

## main-subagent

`Agent` with `subagent_type: "general-purpose"`, no `model:` (it runs on the session
model, the model of the task stage) and `effort: "high"`. `--model` and `--effort` are
launch flags: a running session cannot switch its model, so a stage change restarts the
session (task skill → *Stages*).

## fan-out

Two ways, in order of preference:

1. Several `Agent` calls in a **single message**, which run concurrently. This is the
   default and needs no opt-in.
2. The `Workflow` tool, for deterministic orchestration: `agent()`, `parallel()`,
   `pipeline()`, `phase()`, with typed `schema` results. It is powerful but **requires the
   user to have opted in explicitly** — a skill may not reach for it on its own. Load the
   `workflow-authoring` skill before writing a script.

## background-subagent

The default for `Agent`. You are notified when it completes; never fabricate or predict a
pending agent's result, and never poll for it.

## stop-subagent

`TaskStop`.

## isolate-work

`isolation: "worktree"` on an `Agent` call gives that agent its own git worktree, cleaned up
automatically if unchanged. `EnterWorktree` / `ExitWorktree` move the session itself.

## session-transcript

The JSONL file of this session, found by its id only:

```bash
ls "${CLAUDE_CONFIG_DIR:-$HOME/.claude}"/projects/*/"$CLAUDE_CODE_SESSION_ID".jsonl
```

- Never take the newest file, and never build the path from the cwd.
- `CLAUDE_CODE_SESSION_ID` unset, no file, or more than one file → a blocker: say so and
  stop.
- Check that the `sessionId` of its records is that id.
- A `parentUuid` chain that stops at a parent missing from the file → the result is
  partial: mark it so.

## Also here, with no OpenCode equivalent

Hooks (`UserPromptSubmit`, `PreToolUse`, …) from the plugin, plan mode
(`EnterPlanMode` / `ExitPlanMode`), `ReportFindings` for typed review output, scheduled work
(`CronCreate`, `ScheduleWakeup`), and `Artifact` for published pages. A skill that depends on
one of these states that it is Claude-Code-only.
