# Harness runtime contract

A skill describes *what* to do. How to actually run something in the background, launch a
subagent, ask the user, or isolate work differs per harness, and those differences are
properties of the harness, not of the skill. They are written here once instead of being
restated in every process skill.

Each harness injects its own file the same way it injects the router:

| Harness | Injected by |
| --- | --- |
| Claude Code | the plugin's `UserPromptSubmit` hook, after the router |
| OpenCode | the `instructions` list in `opencode.json`, beside the router |

So no detection is needed: whichever file is in context is the harness you are in.

## The contract

A skill refers to these capability names. Each harness file answers all of them, or says
the capability is absent.

| Capability | Meaning |
| --- | --- |
| `load-skill` | Bring another skill's instructions into context |
| `ask-user` | Put a short choice or question to the user and wait |
| `track-todos` | Keep a visible, live task list |
| `run-background-shell` | Start a long-running command, keep working, collect it later |
| `spawn-subagent` | Delegate one unit of work to a fresh context |
| `fan-out` | Run several subagents over independent inputs |
| `background-subagent` | Delegate without blocking, and be told when it finishes |
| `stop-subagent` | Abandon work already started |
| `isolate-work` | Give work its own checkout or session |

A skill never names a tool. It says "fan out one reviewer per area" or "run plannotator as
a background subagent", and the harness file says what that means here.
