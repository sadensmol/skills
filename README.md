# skills

Source of truth for my agent skills. One copy of every skill, shared by **Claude Code**,
**Codex** and **OpenCode**.

Skills use the open [Agent Skills](https://agentskills.io) format: a folder with a
`SKILL.md` that carries YAML frontmatter (`name`, `description`) and instructions in the
body. All three harnesses read that format, so the same folder serves all of them.

> **Status:** the `cc-stuff` plugin has been imported and restructured. The harness
> wiring in *Install* below has not been run yet.

## Layout

```
.
├── .claude-plugin/marketplace.json   Claude Code marketplace manifest
├── skills/                           every skill; installs as the `sadensmol` namespace
│   ├── router/SKILL.md               name: sadensmol-router
│   ├── task/SKILL.md                 name: sadensmol-task
│   └── …
├── agents/
│   └── code-reviewer/                subagents; the filename is the id
├── harness/                          one runtime file per harness
│   ├── README.md                     the capability contract skills refer to
│   ├── claude-code.md
│   └── opencode.md
├── plugin/                           Claude Code only — hooks, no skills, no agents
│   ├── .claude-plugin/plugin.json
│   └── hooks/                        skill activation, secret scan, pre-commit
└── CLAUDE.md                         rules for working in this repo
```

A skill folder holds `SKILL.md` plus optional `references/`, `scripts/` and `assets/`.

## Naming

Claude Code namespaces plugin skills with a colon. Codex and OpenCode have one flat
namespace, so a bare `router` from two namespaces would collide. The namespace therefore
goes into the frontmatter `name`:

```yaml
# skills/router/SKILL.md
---
name: sadensmol-router
description: "…"
---
```

The skill is `sadensmol-router` in every harness. The folder is called `skills/`, not
`sadensmol/`: nothing derives the namespace from it. Claude Code takes the name from the
frontmatter, and Codex and OpenCode see the namespace as the symlink's name under
`~/.agents/skills`. No skills CLI option can add the prefix at install time, so it lives
in `name`.

Quote any `description` that contains a colon followed by a space — an unquoted one is
invalid YAML and tools skip the skill without failing.

## Install

Clone this repository, then run the commands below once, from the clone. They create
symlinks, so the clone stays the only copy of every skill.

### Claude Code

```bash
npx skills add ./skills --agent claude-code --global   # 21 skills -> ~/.claude/skills
ln -s "$PWD/agents" ~/.claude/agents                   # 5 subagents
claude plugin marketplace add "$PWD"                   # hooks
claude plugin install sadensmol@sadensmol
```

Point the CLI at `skills/`, not at the repository root. From the root it also
finds the NVIDIA sub-skills bundled inside `skills/jetson-orin-nano/references/` and
installs 35 skills instead of 21.

### Codex and OpenCode

```bash
mkdir -p ~/.agents/skills
ln -s "$PWD/skills" ~/.agents/skills/sadensmol
ln -s "$PWD/agents" ~/.config/opencode/agents
```

One link per namespace. Codex reads `~/.agents/skills` on its own, and OpenCode searches
`.agents/skills` at project and global level. To point OpenCode at the tree explicitly,
set it in `~/.config/opencode/opencode.json`:

```json
{ "skills": { "paths": ["~/.agents/skills"] } }
```

### The always-on router

`skills/router/` is not an ordinary skill. It has to be in context before the model
decides anything, because it is what tells the model which other skills to load — nothing
here loads automatically. Each harness delivers it differently, and both are set up above:

| Harness | Mechanism | Frequency |
| --- | --- | --- |
| Claude Code | the plugin's `UserPromptSubmit` hook runs `skill-activation.sh`, which prints the router **and `harness/claude-code.md`** | every prompt |
| OpenCode | `instructions` in `~/.config/opencode/opencode.json`, listing the router **and `harness/opencode.md`** | once per session, in the system prompt |

OpenCode needs one entry, which keeps working across updates because the path is the
symlink, not the clone:

```json
{
  "instructions": [
    "~/.agents/skills/sadensmol/router/SKILL.md",
    "<clone>/harness/opencode.md"
  ],
  "skills": { "paths": ["~/.agents/skills"] }
}
```

## Harness differences

Skills are harness-neutral prose. What differs between harnesses is mechanical — how to run
something in the background, launch and stop a subagent, fan out, ask the user, keep a todo
list, isolate work — and those are properties of the harness, not of the skill. They are
written once each in `harness/claude-code.md` and `harness/opencode.md`, against the
capability contract in [harness/README.md](harness/README.md).

A skill therefore says "fan out one reviewer per area" or "run this as a background
subagent", never `Agent` or `task`. Whichever runtime file is in context is the harness you
are in, so nothing has to detect anything.

The differences are real, not cosmetic. Claude Code fans out with concurrent `Agent` calls
or an opted-in `Workflow` script, backgrounds by default, stops work with `TaskStop` and
isolates it with `isolation: "worktree"`. OpenCode has `task` with foreground as the
default, needs `OPENCODE_EXPERIMENTAL_BACKGROUND_SUBAGENTS=true` before it will background
one, offers no orchestration script, no stop tool and no worktree tool, and expects
`todowrite` to be kept live — which Claude Code does not have in this build at all.

Codex is not wired up. It supports the same `UserPromptSubmit` hook as Claude Code, and
`skill-activation.sh` already works there unchanged, but no `hooks.json` for it ships here
yet.

### Agents

`agents/` works the same way, and subfolders are free: Claude Code scans `~/.claude/agents`
recursively and takes a subagent's identity from the frontmatter `name`, OpenCode scans
`{agent,agents}/**/*.md` and takes it from the filename. Keep the two equal and the id is
the same in both — `agents/code-reviewer/quality.md` with `name: quality` is the agent
`quality`. The folder groups them for humans and never enters the id.

Two frontmatter rules, both verified against the harnesses:

- **No `color:` with a named value.** Claude Code accepts `red`; OpenCode rejects it and
  fails the whole config load with *Configuration is invalid*. A hex value passes OpenCode,
  but Claude Code documents an enum, so the portable answer is to omit the field.
- **Keep `mode: subagent`.** OpenCode needs it or the agent is offered as a primary agent
  too; Claude Code ignores the field and still validates.

## Refresh

Editing a skill needs no refresh: every harness reads through a symlink into this clone,
so the change is live in the next session. Only these cases need an action.

| What changed | Claude Code | Codex, OpenCode |
| --- | --- | --- |
| A `SKILL.md` or a reference file | nothing | nothing |
| A new skill folder | `npx skills add ./skills --agent claude-code --global` | nothing |
| A skill renamed or deleted | `npx skills remove --agent claude-code --global`, then add again | nothing |
| An agent in `agents/` | nothing | nothing |
| A new agent file | nothing — the whole directory is linked | nothing |
| `plugin/hooks/` | bump `version` in `plugin/.claude-plugin/plugin.json`, then `claude plugin marketplace update sadensmol` and `claude plugin update sadensmol` | not applicable |

After `git pull`, apply the rows that match what the commits changed. Nothing else is
needed, because the working tree is what every harness reads.

Check what is installed:

```bash
npx skills list --global      # links per agent
claude plugin list            # plugin version Claude Code is running
```

## Where each harness looks

| Harness | Global | Project |
| --- | --- | --- |
| Claude Code | `~/.claude/skills` | `.claude/skills` |
| Codex | `~/.agents/skills`, `~/.codex/skills` | `.agents/skills` |
| OpenCode | `~/.config/opencode/skills`, `~/.agents/skills` | `.opencode/skills`, `.claude/skills`, `.agents/skills` |

## Checking your work

```bash
npx skills add ./skills --list   # lists all 21; a missing one has broken frontmatter
claude plugin validate .           # marketplace manifest
claude plugin validate ./plugin    # plugin manifest
```

## Rules for this repo

This repository is **public**. Every file must be usable by a stranger who has none of my
projects, machines or accounts.

- No secrets. Reference them by environment variable name only.
- No local paths (`/Users/…`, `/home/…`, `~/work/…`). Derive paths at runtime or ask.
- No project-specific knowledge — that lives in private namespace repos of the same shape.
- Keep `SKILL.md` under ~500 lines. Move detail into `references/`.
- Write a trigger-rich `description`: it is the only thing an agent sees until the skill loads.

`CLAUDE.md` holds the long form of these rules.
