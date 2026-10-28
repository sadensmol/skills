---
description: Clean up Claude Code, Claude Desktop, Codex chat, OpenCode data and Docker images older than a month
---

Ensure the `sadensmol-cleanup` skill is loaded. If its full instructions are already present
in the current conversation context, do not invoke it again; apply those instructions to
this request. Otherwise, invoke it now. Your harness runtime file answers `load-skill` and
says how to invoke a skill here; reading the `SKILL.md` file is not a substitute.

Forward the argument string below to it **verbatim**:

$ARGUMENTS

If the argument string is empty, apply the skill with no arguments and let it use its
default age. Do not scan or delete anything before the skill is loaded.
