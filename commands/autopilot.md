---
description: Autopilot — start or control the unattended agent factory, or run one issue
---

Ensure the `sadensmol-autopilot` skill is loaded. If its full instructions are already present
in the current conversation context, do not invoke it again; apply those instructions to
this request. Otherwise, invoke it now. Your harness runtime file answers `load-skill` and
says how to invoke a skill here; reading the `SKILL.md` file is not a substitute.

Forward the argument string below to it **verbatim** as the mode and its arguments:

$ARGUMENTS

If the argument string is empty, do the skill's control-room start. Do no autopilot,
agterm, git or service work before the skill is loaded.
