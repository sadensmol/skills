# Dispatching the reviewers — OpenCode

Read this only in OpenCode. The neutral instructions live in `SKILL.md`; this file is one
harness's answer to `fan-out` from the runtime contract.

There is no orchestration script here, so you are the orchestrator.

The reviewers' models live in **`opencode.jsonc`**, not in the agent markdown: its
`agent.<name>.model` / `variant` entries are this harness's place for an `openai/*` model and
they override the frontmatter. The shared `agents/code-reviewer/*.md` files (the same
directory Claude Code reads) carry that harness's `model: sonnet` + `effort: high` — leave
them alone.

1. Build each reviewer's brief exactly as `SKILL.md` describes: its area, its focus, its
   P0/P1/P2 definitions, the skills it must load, and the **diff command** — never the diff
   text.
2. Issue one `task` call per reviewer, agent ids `documentation`, `implementation`,
   `quality`, `simplification`, `testing`. Foreground is the default and is the right choice
   here: you need all five results before you can write the report.
   - With `OPENCODE_EXPERIMENTAL_BACKGROUND_SUBAGENTS=true` set you may pass
     `background: true` per reviewer and be notified as each finishes. Without it, run them
     in the foreground; do not sleep or poll either way.
3. Ask each reviewer to end with a findings block in the shape `SKILL.md` specifies —
   priority, title, file, line, issue, fix — one block per finding, and an explicit "no
   findings" when it has none. Structured output is not enforced by a schema here, so state
   the shape in the brief and reject a reply that ignores it.
4. A reviewer that fails or returns nothing usable is recorded as failed for its area, and
   said so in the report. Do not silently drop an area, and do not re-run the whole fan-out
   to paper over one branch.
5. Assemble the tables yourself, in the order and format `SKILL.md` requires.
