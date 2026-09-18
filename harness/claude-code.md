# Runtime: Claude Code

You are in Claude Code. This file answers the capability contract in `harness/README.md`.

## load-skill

The `Skill` tool, with the skill's flat id: `Skill(sadensmol-go-programming)`. Reading the
`SKILL.md` file instead does not count — the skill must be invoked.

## ask-user

`AskUserQuestion`. Up to 4 questions per call, 2-4 options each, `multiSelect` when the
choices are not exclusive. Options carry an optional `preview` for side-by-side comparison.

## track-todos

Not available in this build. Keep the plan in your reply text instead, and do not claim a
todo list exists.

## run-background-shell

`Bash` with `run_in_background: true`. It keeps running across turns and re-invokes you when
it exits, so never poll it with `sleep`. Collect output with `TaskOutput`, wait on a
condition with `Monitor`, and kill it with `TaskStop`.

## spawn-subagent

The `Agent` tool: `subagent_type` picks the agent, `prompt` carries the work. Subagents run
in the background by default; pass `run_in_background: false` only when your next action
depends on the result and nothing else could usefully happen meanwhile.

`ListAgents` shows what is running, and `SendMessage` continues one with its context intact
— a new `Agent` call always starts fresh.

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

## Also here, with no OpenCode equivalent

Hooks (`UserPromptSubmit`, `PreToolUse`, …) from the plugin, plan mode
(`EnterPlanMode` / `ExitPlanMode`), `ReportFindings` for typed review output, scheduled work
(`CronCreate`, `ScheduleWakeup`), and `Artifact` for published pages. A skill that depends on
one of these states that it is Claude-Code-only.
