---
description: Project task/worktree management — resolves the project and dispatches to its task skill
---

Ensure the `sadensmol-task` skill is loaded. If its full instructions are already present
in the current conversation context, do not invoke it again; apply those instructions to
this request. Otherwise, invoke it now. Your harness runtime file answers `load-skill` and
says how to invoke a skill here; reading the `SKILL.md` file is not a substitute.

Forward the argument string below to it **verbatim** as the subcommand and its arguments:

$ARGUMENTS

If the argument string is empty, apply the skill with no arguments and let it use its own
default. Do no task, worktree or git work before the skill is loaded.
