---
name: sadensmol-retro
description: "Run a weekly Scrum retrospective over the current calendar week. Use when the user asks for a retro, retrospective, weekly review, or analysis of agent mistakes, rework, repeated corrections, or user frustration in Claude Code and OpenCode sessions."
---

# Retro

## Scope

Review only sessions from the current local calendar week through the present moment:

```text
[start of the current calendar week, now]
```

Use Monday 00:00 as the start of the calendar week and the runtime's local timezone.
State the exact start and end timestamps before analysis. Do not include older sessions.

## Workflow

### 1. Collect sessions

Inspect both Claude Code and OpenCode session histories.
Use each runtime's session-history capability or its documented local session store.
Enumerate sessions by timestamp, then load every session from each runtime in the time window.
Keep the session identifier, project context, timestamps, and complete message order.

If either runtime cannot enumerate or load its sessions, ask the user for access before analysis.
Do not bypass a permission boundary. The user may grant access, provide session identifiers,
or provide exported transcripts. If only one runtime remains available, stop until the user
chooses whether to continue without the other runtime. Do not analyze the current conversation
as a substitute for missing history.

Before delegation, create a sanitized review set:

- Preserve user requests, agent responses, tool calls, errors, retries, and rework messages.
- Remove tokens, passwords, cookies, private keys, authorization headers, and unrelated personal data.
- Keep timestamps and turn boundaries so reviewers can cite evidence.
- Pass the same bounded, sanitized review set from both runtimes to exactly ten reviewers.

### 2. Fan out ten reviewers

Fan out exactly one reviewer for each role below. Ask each reviewer to return evidence, not edits.
Each reviewer must identify the session and turn that supports every finding.

1. **Requirements**: detect missed, invented, or misunderstood requirements.
2. **Rework**: detect explicit corrections, repeated attempts, and avoidable retries.
3. **Tool use**: detect wrong tools, invalid commands, stale reads, or skipped verification.
4. **Scope**: detect unrequested work, unsafe changes, and missed boundaries.
5. **Technical quality**: detect bugs, regressions, weak error handling, or bad implementation choices.
6. **Testing**: detect missing, incorrect, or misleading validation.
7. **Communication**: detect unclear questions, unsupported claims, or poor progress updates.
8. **User experience**: detect evidence of frustration, urgency, confusion, or repeated explanation.
9. **Safety and privacy**: detect secret exposure, destructive actions, or unsafe data handling.
10. **Prevention**: independently propose the smallest evidence-backed skill, hook, config, or process fix.

Treat a user request to redo, rework, correct, revert, or try again as a high-value signal.
Treat strong frustration or urgency as a signal only when the transcript shows it explicitly.
Do not infer emotion from short messages alone.

### 3. Consolidate findings

Merge duplicate findings into incidents. Keep only incidents supported by transcript evidence.
For each incident, record:

- The exact problem and a short evidence quote.
- The likely cause, separating facts from hypotheses.
- The smallest prevention change.
- The target owner: skill, hook, configuration, project guidance, or operator process.
- Confidence: `clear`, `probable`, or `unclear`.

Do not treat reviewer agreement as proof. A finding still needs direct evidence.

### 4. Apply learning

For `clear` findings, update only the smallest relevant target and preserve unrelated user changes.
Load `sadensmol-learn` when a lesson belongs to a shared `sadensmol-*` skill. Use the owning
project skill for project-specific lessons. Update hooks or configuration only when the transcript
shows that they caused or could directly prevent the incident.

Do not add speculative rules, broad refactors, duplicate guidance, or secrets.
Do not edit the retro skill from a retro unless the user explicitly approves that change.
Run the narrowest available validation after each change.

For `unclear` findings, ask the user before editing. Include the evidence, the unresolved choice,
and these options: `learn this`, `fix a different target`, or `skip this incident`.
Wait for the user's decision, then apply only the selected action.

### 5. Report

**Mandatory completion gate:** Applying changes and running validation are
intermediate steps, not the final deliverable. Do not stop after step 4. Before
the final response, verify that it contains the reviewed time window, separate
Claude Code and OpenCode counts, and the complete table below. If no incidents
remain, include a table row stating that no supported incidents were found.

Finish with a concise Markdown table. Use one row per consolidated incident and
these columns. Put the runtime and incident date in the `Problem` cell using
this format: `<problem> (`YYYY-MM-DD`, Claude Code|OpenCode|Both)`. Use the
runtime's local timezone for the date.

| Problem | Why it happened | What changed | Improve next time or operator suggestion |
|---|---|---|---|

Link each changed file or configuration path. Mark unresolved incidents as `needs user decision`.
Include the reviewed time window and separate Claude Code and OpenCode session counts above the table.
The workflow is incomplete until this table appears in the same final response as the retro result.
