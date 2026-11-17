---
description: Workspace task/worktree management — resolves the workspace and dispatches to its task skill
---

Ensure the `sadensmol-task` skill is loaded. If its full instructions are already present
in the current conversation context, do not invoke it again; apply those instructions to
this request. Otherwise, invoke it now. Your harness runtime file answers `load-skill` and
says how to invoke a skill here; reading the `SKILL.md` file is not a substitute.

Forward the argument string below to it **verbatim** as the subcommand and its arguments:

$ARGUMENTS

If the argument string is empty, run the skill's `help` (`references/help.md`): list the
subcommands of the current workspace with their explanations. Do no task, worktree or git
work before the skill is loaded.
