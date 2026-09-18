---
name: sadensmol-learn
description: "Captures and integrates new knowledge into existing **sadensmol** skills. Use when the user: (1) Points out information you didn't know or got wrong, (2) Asks to 'remember this' or 'update the skill', (3) Shares important context that should be preserved, (4) Identifies missing patterns or procedures, (5) Requests improvements to skill documentation, or (6) Says 'learn from this', 'add this to the docs', etc. Scope is limited to the sadensmol namespace only — it does not edit other plugins' skills, built-in skills, or project docs."
---

# Learn

Integrate new knowledge into existing skills **within the `sadensmol` namespace only**.

## Scope (HARD boundary)

This skill **only** edits skills owned by the `sadensmol` plugin (i.e. `sadensmol-*` skills, whose source lives in the sadensmol plugin's `skills/` directory).

**Never** use this skill to modify:
- Other projects'/plugins' skills (any non-`sadensmol-` namespaced skill) — those have their own repos/owners.
- Built-in or third-party skills.
- Project files, `CLAUDE.md`, or arbitrary documentation.

If the knowledge belongs outside the sadensmol namespace, **stop** and tell the user where it should go instead — do not edit it here. Example: "That's project-specific — update the project's own skill in its repo, not here."

## General vs project-specific (decide BEFORE editing)

The general sadensmol skills (`go-programming`, `typescript-programming`,
`dart-programming`, `github`, `linear`, …) hold ONLY knowledge that is reusable
across every project. **Project/product-specific knowledge must NOT be inlined
into a general skill.**

Tell-tale signs a learning is project-specific (→ belongs in that project's own
skill, which is out of scope here): it names a specific repo, service, deploy
pipeline, environment, auth profile, business entity, or workflow — e.g. a
particular e2e suite, `AWS_PROFILE=<project>`, a named game/currency, a
`task finish` step, a specific CI workflow.

Route by which parts are general vs specific:

- **Purely general** → integrate into the matching general sadensmol skill.
- **Purely project-specific** → STOP; tell the user it belongs in the project's
  own skill (`<project>-<project>` / `<project>-task`), not here.
- **Both** → split. Put the *general principle* in the general skill and have it
  **point to the project-specific skill** for the concrete mechanics — never
  embed the project's commands/env/entities in the general skill. Example: a
  generic "to debug a test against a deployed env, run it locally" principle goes
  in `go-programming`/`go-integration-tests` ONLY as "follow the project's own
  skill for how to run it"; the actual `AWS_PROFILE=…`, env names, and CI steps
  live in the project skill.

## Workflow

### 1. Identify Destination (must be a sadensmol skill)

Determine which **sadensmol** skill the knowledge belongs in:

1. **Check active sadensmol skills first** - If `sadensmol-*` skills are loaded in this conversation:
   - Read their SKILL.md to understand structure
   - Check their `references/` directory for internal documentation
   - The learning likely belongs in one of these locations

2. **Confirm it's in scope** - If the knowledge is not sadensmol-owned, stop (see Scope above).

3. **Ask if unclear** - "Should I add this to the `sadensmol-[skill-name]` skill, or does it belong elsewhere (outside this skill's scope)?"

4. **Consider which sadensmol skill**:
   - Language/domain → the matching sadensmol skill (`go-programming`, `typescript-programming`, etc.)
   - Tool/integration → the matching sadensmol skill (`github`, `linear`, `slack`, etc.)
   - New domain that fits sadensmol → may need a new sadensmol skill (use skill-creator)

### 2. Read and Locate

Before editing, always read the target file to:
- Understand existing structure and style
- Find the right section for new content
- Check for duplicates or contradictions
- Identify any `references/` files that may be the better target

**For skills with references/**: Check if the knowledge fits better in a reference file than SKILL.md. Long skills should keep SKILL.md lean.

### 3. Propose + get approval (MUST FOLLOW — before ANY edit)

For **every** change to a skill, first clearly state:
- **Which skill/file** will be edited.
- **What the change is about** — the learning being captured, in 1–3 lines.
- **Where it goes** (section) and roughly what the added text says.

Then ask for approval via `AskUserQuestion` — options: **apply** / **skip**.
Edit only after an explicit "apply". Multiple changes → per-item approval.
A skipped item is dropped without comment. Never edit skills silently.

### 3a. Integrate (after approval)

Edit the target file:
- Match existing style (heading levels, formatting, code style)
- Be concise - every token must justify its cost
- Use examples over explanations
- Use imperative form ("Use X", not "You should use X")

### 4. Package and Confirm

```bash
python3 "$(ls -d ~/.agents/skills/skill-creator ~/.claude/skills/skill-creator 2>/dev/null | head -1)"/scripts/package_skill.py <path/to/skill-folder>
```

Summarize changes and ask: "Does this capture what you wanted?"

## Handling Contradictions

If new knowledge contradicts existing content:
1. Point out the conflict
2. Ask: "The skill says X, but you're saying Y. Replace X with Y?"
3. Update accordingly

## Creating New Skills

If knowledge doesn't fit existing skills:
1. Confirm: "This needs a new skill. Create one called [name]?"
2. Use skill-creator skill for the creation process
