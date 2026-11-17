---
description: Sadensmol (generic) commands — list them, or run one as /sadensmol <command> [args]
---

This command lists and runs the `sadensmol-` skills. Do no other work, and load no skill, until
the lister below has answered.

The lister is `scripts/list-commands.py` in the `sadensmol` skill directory:
`$HOME/.claude/skills/sadensmol`, or `$HOME/.agents/skills/sadensmol/general`
where skills are installed by namespace. Run it with `python3 -I`.

Argument string: $ARGUMENTS

- **Empty:** run `list-commands.py sadensmol` and show its output as it is. Add nothing.
- **Otherwise:** the first word is the command, the rest are its arguments. Run
  `list-commands.py sadensmol <command>`. It prints the skill id `sadensmol-<command>`, or fails.
  - On success, ensure that skill is loaded. If its full instructions are already in the
    current context, do not invoke it again. Otherwise invoke it now: your harness runtime
    file answers `load-skill`, and reading the `SKILL.md` file is not a substitute. When
    the skill says to load a `sadensmol-` base first, load the base first. Then forward
    the remaining arguments to it verbatim.
  - On failure, say in one line that `/sadensmol <command>` does not exist, then show the list.
    Never fall back to a skill outside the `sadensmol-` namespace.
