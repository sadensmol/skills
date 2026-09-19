---
description: Project task/worktree management — resolves the project and dispatches to its task skill
---

Invoke the `sadensmol-task` skill now. Your harness runtime file answers `load-skill`
and says how to invoke a skill here; reading the `SKILL.md` file is not a substitute.

Forward the argument string below to it **verbatim** as the subcommand and its arguments:

$ARGUMENTS

If the argument string is empty, invoke the skill with no arguments and let it apply its
own default. Do no task, worktree or git work before the skill is loaded.
