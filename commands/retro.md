---
description: Run a weekly Scrum retrospective over the current calendar week
---

Ensure the `sadensmol-retro` skill is loaded. If its full instructions are already present
in the current conversation context, do not invoke it again; apply those instructions to
this request. Otherwise, invoke it now. Your harness runtime file answers `load-skill` and
says how to invoke a skill here; reading the `SKILL.md` file is not a substitute.

Forward the argument string below to it **verbatim**:

$ARGUMENTS

If the argument string is empty, apply the skill with no arguments and let it use its own
default period. Do not start gathering session data before the skill is loaded.
