---
description: Capture new knowledge into the sadensmol skills
---

Ensure the `sadensmol-learn` skill is loaded. If its full instructions are already present
in the current conversation context, do not invoke it again; apply those instructions to
this request. Otherwise, invoke it now. Your harness runtime file answers `load-skill` and
says how to invoke a skill here; reading the `SKILL.md` file is not a substitute.

Forward the argument string below to it **verbatim** as the knowledge to capture:

$ARGUMENTS

If the argument string is empty, the lesson is whatever this conversation just
established — apply the skill with no arguments and let it work that out. Do not edit
any skill file before the skill is loaded.
