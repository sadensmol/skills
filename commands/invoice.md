---
description: Generate an invoice PDF — resolves whose billing data to use, then builds and renders it
---

Ensure the `sadensmol-invoice` skill is loaded. If its full instructions are already
present in the current conversation context, do not invoke it again; apply those
instructions to this request. Otherwise, invoke it now. Your harness runtime file answers
`load-skill` and says how to invoke a skill here; reading the `SKILL.md` file is not a
substitute.

Forward the argument string below to it **verbatim** as the invoice request — a period, a
client, an amount, or whatever the user typed:

$ARGUMENTS

If the argument string is empty, apply the skill with no arguments and let it resolve the
data source and the period from its own rules. Do not assemble a payload, pick an invoice
number, guess any party/bank/tax value, or render anything before the skill is loaded.
