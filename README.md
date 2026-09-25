# skills

Source of truth for my agent skills. One copy of every skill, shared by **Claude Code**,
**Codex** and **OpenCode**.

Skills use the open [Agent Skills](https://agentskills.io) format: a folder with a
`SKILL.md` that carries YAML frontmatter (`name`, `description`) and instructions in the
body. All three harnesses read that format, so the same folder serves all of them.

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
├── commands/                         short slash-command aliases for skills
│   ├── task.md                       /task -> the `sadensmol-task` skill
│   ├── retro.md                      /retro -> `sadensmol-retro`
│   └── learn.md                      /learn -> `sadensmol-learn`
├── instructions/
│   └── AGENTS.md                     standing instructions, every harness reads this one
├── harness/                          one runtime file per harness
│   ├── README.md                     the capability contract skills refer to
│   ├── claude-code.md                injected by the plugin's hook
│   ├── codex/
│   │   ├── codex.md                  injected by the UserPromptSubmit hook
│   │   ├── router-inject.sh          that hook — Codex reads a hook's stdout as JSON
│   │   ├── install.sh                writes the hook, links AGENTS.md and the commands,
│   │   │                             generates the agents as TOML
│   │   └── install.test.sh           its checks; hermetic, no Codex needed
│   └── opencode/
│       ├── opencode.md               injected through the config's `instructions`
│       ├── install.sh                writes that entry into the config
│       └── install.test.sh           its checks; hermetic, no OpenCode needed
├── test/                             container checks for the whole install
│   ├── Dockerfile                    both harness CLIs
│   └── run-tests.sh                  the assertions
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

Claude Code reads `~/.claude/skills/<dir>/SKILL.md`, one directory per skill, and the
directory has to be named after the frontmatter `name`: the plugin's
`skill-activation.sh` looks the router up as `~/.claude/skills/sadensmol-router/SKILL.md`.
So link every skill folder under its own name.

```bash
mkdir -p ~/.claude/skills
for d in "$PWD"/skills/*/; do                          # 24 skills -> ~/.claude/skills
  name=$(sed -n 's/^name: *//p' "$d/SKILL.md" | head -1 | tr -d "\"'")
  rm -rf "$HOME/.claude/skills/$name"                  # replaces the link from last time
  ln -s "${d%/}" "$HOME/.claude/skills/$name"
done
ln -s "$PWD/agents" ~/.claude/agents                   # 5 subagents
rm -f ~/.claude/CLAUDE.md                              # replaces any existing copy
ln -s "$PWD/instructions/AGENTS.md" ~/.claude/CLAUDE.md  # standing instructions
claude plugin marketplace add "$PWD"                   # hooks
claude plugin install sadensmol@sadensmol
```

**Do not install these with `npx skills add … --agent claude-code --global`.** As of
skills CLI 1.7.0 that *copies* a local source tree into `~/.claude/skills` — the output
says `(copied)` — which forks every skill: edits in the clone then never reach Claude
Code, and the hook injects a stale router. The `--copy` flag reads as though linking
were the default, but a local path is copied regardless.

The read-only `npx skills` commands stay useful, and `npx skills list --global` does see
links made by hand. Point them at `skills/`, not at the repository root: from the root
they also find the NVIDIA sub-skills bundled inside
`skills/jetson-orin-nano/references/`, which are not skills of this namespace.

### Codex and OpenCode

Both find skills in the same tree, so that link is made once:

```bash
mkdir -p ~/.agents/skills
ln -s "$PWD/skills" ~/.agents/skills/sadensmol
```

One link per namespace. Codex reads `~/.agents/skills` on its own, and OpenCode searches
`.agents/skills` at project and global level. Everything else differs, and each harness has
an installer for its own side:

```bash
harness/codex/install.sh       # hook, AGENTS.md, commands, agents
harness/opencode/install.sh    # instructions, skills.paths
ln -s "$PWD/agents" ~/.config/opencode/agents            # OpenCode reads the markdown ones
rm -f ~/.config/opencode/AGENTS.md
ln -s "$PWD/instructions/AGENTS.md" ~/.config/opencode/AGENTS.md
```

OpenCode needs the router in its config because it has no hook to inject one; the installer
writes that entry and points the skills tree out explicitly at the same time. Codex does
have hooks, so its installer writes one, and covers three more things Codex cannot work out
for itself: `~/.codex/AGENTS.md` -> `instructions/AGENTS.md`, `commands/*.md` linked into
`~/.codex/prompts/`, and `agents/` generated into `~/.codex/agents/*.toml`, because a Codex
custom agent is TOML (`name`, `description`, `developer_instructions`) and cannot be the
markdown file the other two read.

**Codex skips a new or changed hook until it is trusted.** After the first run, and after
any edit to `router-inject.sh`, open Codex and run `/hooks` to trust the entry — until then
the router is not injected and nothing says so.

See [The always-on router](#the-always-on-router) for what the installers do and do not
touch.

### Short commands

Skill ids stay namespaced (`sadensmol-task`), so typing `/sadensmol-task new …` is what a
bare skill invocation costs. `commands/` fixes that without un-prefixing anything: one
markdown file per short name, linked into each harness's command directory.

```bash
mkdir -p ~/.claude/commands ~/.config/opencode/command
for f in "$PWD"/commands/*.md; do                      # /task, /retro, /learn
  n=$(basename "$f")
  ln -sf "$f" "$HOME/.claude/commands/$n"              # Claude Code
  ln -sf "$f" "$HOME/.config/opencode/command/$n"      # OpenCode
done
```

Codex's copies are linked by `harness/codex/install.sh`, into `~/.codex/prompts/`, so they
are not in the loop above.

All three take the command name from the **filename**, read `description` from the
frontmatter for the `/` menu, and substitute the user's arguments for `$ARGUMENTS` in the
body. So one file serves all of them, the same way one `SKILL.md` does. Codex offers them
under a prefix — `task.md` is `/prompts:task` there, `/task` in the other two.

See [Adding a command](#adding-a-command) for how to add the next one.

### Standing instructions

`instructions/AGENTS.md` is the one copy of the standing instructions. All three harnesses
read it through a symlink — Claude Code as `~/.claude/CLAUDE.md`, Codex as
`~/.codex/AGENTS.md`, OpenCode as `~/.config/opencode/AGENTS.md` — so an edit lands in all
of them at once and none can drift.
It holds nothing harness-specific: rules that apply to only one runtime go in that
harness's file under `harness/`, which is in context there anyway.

The file lives in `instructions/`, not at the repository root: a root `AGENTS.md` is what Codex
and OpenCode read as the *project* rules for this repository, which is `CLAUDE.md`'s job.

### The always-on router

`skills/router/` is not an ordinary skill. It has to be in context before the model
decides anything, because it is what tells the model which other skills to load — nothing
here loads automatically. Each harness delivers it differently, and both are set up above:

| Harness | Mechanism | Frequency |
| --- | --- | --- |
| Claude Code | the plugin's `UserPromptSubmit` hook runs `skill-activation.sh`, which prints the router **and `harness/claude-code.md`** | every prompt |
| Codex | the `UserPromptSubmit` hook in `~/.codex/hooks.json` runs `harness/codex/router-inject.sh`, which prints the router **and `harness/codex/codex.md`** | every prompt |
| OpenCode | `instructions` in `~/.config/opencode/opencode.json`, listing the router **and `harness/opencode/opencode.md`** | once per session, in the system prompt |

Codex's hook is the same event as Claude Code's, but not the same contract, and
`skill-activation.sh` does **not** work there unchanged: Codex parses a hook's stdout as
JSON and takes the model-visible text from `hookSpecificOutput.additionalContext`, so plain
text is a parse error rather than context. It also caps that text at ~2500 tokens and
spills the rest to a file, which would cut the router in half — the installed handler
therefore sets `additionalContextLimit: 0`. Hence a second injector,
`harness/codex/router-inject.sh`, rather than a shared one.

Its installer owns one handler and nothing else: other hook events, anyone else's
`UserPromptSubmit` hook, and every other key in `hooks.json` are carried through untouched,
a `.bak` is kept, and an unparseable file is refused rather than rewritten. A real
`~/.codex/AGENTS.md` you wrote yourself is moved to `AGENTS.md.bak` rather than destroyed,
and a hand-written `~/.codex/agents/*.toml` is never removed — only the files the generator
stamped as its own.

There is no hook on the OpenCode side, so that entry has to be in the config. Write it with
the installer, which is why `harness/opencode/` is a folder:

```bash
harness/opencode/install.sh            # --dry-run prints the merged config instead
harness/opencode/install.test.sh       # its checks: every case runs against a throwaway HOME
harness/codex/install.sh               # --dry-run prints the merged hooks.json instead
harness/codex/install.test.sh          # the same, for the Codex side and its injector
```

It owns two keys and nothing else — `instructions` and `skills.paths` — the same ground the
plugin's hook covers on the Claude Code side. Your `agent`, `permission`, `model` and `mcp`
blocks are left alone, other namespaces' routers already in `instructions` are kept, and an
entry from an earlier location of the runtime file is replaced rather than duplicated. It
edits whichever of `opencode.jsonc` / `opencode.json` already exists, keeps a `.bak`, and
refuses to touch a `.jsonc` that has real comments in it, since `jq` cannot parse one.

The result is the two entries below. The router goes in through the symlink, not the clone,
so it keeps working across updates:

```json
{
  "instructions": [
    "~/.agents/skills/sadensmol/router/SKILL.md",
    "<clone>/harness/opencode/opencode.md"
  ],
  "skills": { "paths": ["~/.agents/skills"] }
}
```

## Harness differences

Skills are harness-neutral prose. What differs between harnesses is mechanical — how to run
something in the background, launch and stop a subagent, fan out, ask the user, keep a todo
list, isolate work — and those are properties of the harness, not of the skill. They are
written once each in `harness/claude-code.md`, `harness/codex/codex.md` and
`harness/opencode/opencode.md`, against the capability contract in
[harness/README.md](harness/README.md).

A skill therefore says "fan out one reviewer per area" or "run this as a background
subagent", never `Agent` or `task`. Whichever runtime file is in context is the harness you
are in, so nothing has to detect anything.

The differences are real, not cosmetic. Claude Code fans out with concurrent `Agent` calls
or an opted-in `Workflow` script, backgrounds by default, stops work with `TaskStop` and
isolates it with `isolation: "worktree"`. OpenCode has `task` with foreground as the
default, needs `OPENCODE_EXPERIMENTAL_BACKGROUND_SUBAGENTS=true` before it will background
one, offers no orchestration script, no stop tool and no worktree tool, and expects
`todowrite` to be kept live — which Claude Code does not have in this build at all.

Codex is a third shape again. It has no skill tool at all: skills arrive as a catalog of
names, descriptions and paths, and loading one means reading that `SKILL.md` and following
it. It delegates with `spawn_agent` / `wait_agent` / `interrupt_agent`, asks with
`request_user_input` (absent in `codex exec`), keeps a plan with `update_plan`, backgrounds
a command through unified exec rather than a job flag, and has no worktree tool. It also
tells the model not to delegate unless the user, `AGENTS.md`, or a skill asks — which a
skill saying "fan out" does.

### Agents

`agents/` works the same way for two of the three, and subfolders are free: Claude Code
scans `~/.claude/agents` recursively and takes a subagent's identity from the frontmatter
`name`, OpenCode scans `{agent,agents}/**/*.md` and takes it from the filename. Keep the two
equal and the id is the same in both — `agents/code-reviewer/quality.md` with `name:
quality` is the agent `quality`. The folder groups them for humans and never enters the id.

Codex is the exception: a custom agent there is a TOML file in `~/.codex/agents/` with
`name`, `description` and `developer_instructions`, so the markdown cannot simply be linked.
`harness/codex/install.sh` generates one TOML per markdown agent, keeping the same id, and
drops the frontmatter keys that mean nothing there (`mode`, and `model` — a Claude model
slug is not a Codex one). They are generated files: after editing an agent, re-run that
installer.

Two frontmatter rules, both verified against the harnesses:

- **No `color:` with a named value.** Claude Code accepts `red`; OpenCode rejects it and
  fails the whole config load with *Configuration is invalid*. A hex value passes OpenCode,
  but Claude Code documents an enum, so the portable answer is to omit the field.
- **Keep `mode: subagent`.** OpenCode needs it or the agent is offered as a primary agent
  too; Claude Code ignores the field and still validates.

## Adding a command

A command is a **prompt template**, not a skill. The harness expands it into the
conversation, and the text then tells the model which skill to invoke. That indirection is
the whole point: the skill keeps its namespaced id, and the user types a short name.

| Harness | Command directory | Name comes from | Arguments |
| --- | --- | --- | --- |
| Claude Code | `~/.claude/commands/` | the filename (`task.md` -> `/task`) | `$ARGUMENTS` |
| Codex | `~/.codex/prompts/` | the filename (`task.md` -> `/prompts:task`) | `$ARGUMENTS` |
| OpenCode | `~/.config/opencode/command/` | the filename (`task.md` -> `/task`) | `$ARGUMENTS` |

Same filename, same frontmatter key, same placeholder — so one file in `commands/` is
linked into all three and there is nothing per-harness to keep in sync. Codex scans only
the top level of `prompts/`, which the flat `commands/` folder already satisfies.

To add `/<name>` for the skill `sadensmol-<skill>`:

1. Write `commands/<name>.md`:

   ```markdown
   ---
   description: One line, shown in the `/` menu
   ---

   Ensure the `sadensmol-<skill>` skill is loaded. If its full instructions are already
   present in the current conversation context, do not invoke it again; apply those
   instructions to this request. Otherwise, invoke it now. Your harness runtime file
   answers `load-skill` and says how to invoke a skill here; reading the `SKILL.md` file
   is not a substitute.

   Forward the argument string below to it **verbatim**:

   $ARGUMENTS
   ```

2. Re-run the command link loop from [Short commands](#short-commands). `ln -sf` is
   idempotent, so it costs nothing to run it again.

Five rules, each of them load-bearing:

- **Keep `description` as the only frontmatter key.** Claude Code accepts more
  (`argument-hint`, `allowed-tools`, `model`); Codex reads `description` and
  `argument-hint`; OpenCode has its own set (`agent`, `subtask`). Anything outside the
  intersection is a key some harness does not expect, so the portable file carries just the
  one all three read.
- **Name the skill, never a tool.** The body says "invoke the skill" and points at the
  runtime file for `load-skill`, exactly as a `SKILL.md` would — a command is not exempt
  from the no-harness-tool-names rule.
- **Do not reload an already-loaded skill.** Commands run on every invocation, but a skill's
  full instructions persist across requests while they remain in context. Ensure the skill
  is loaded, then reuse it; a new command invocation alone is not a reload trigger.
- **Say what not to do before the skill loads.** A command is text the model may act on
  directly; without that line it will happily start the work itself and skip the skill.
- **Do not claim a name another namespace owns.** `/task` is safe because
  `sadensmol-task` is the dispatcher that resolves the project. A `/linear` or `/router`
  alias would hard-code one namespace's answer to a name three plugins define, which is
  what the router's disambiguation rules exist to decide per session.

## Refresh

Editing a skill needs no refresh: every harness reads through a symlink into this clone,
so the change is live in the next session. Only these cases need an action.

| What changed | Claude Code | Codex | OpenCode |
| --- | --- | --- | --- |
| A `SKILL.md` or a reference file | nothing | nothing | nothing |
| A new skill folder | re-run the link loop from *Install* — it is idempotent | nothing | nothing |
| A skill renamed or deleted | re-run the loop, then `rm ~/.claude/skills/<old-name>` | nothing | nothing |
| An agent in `agents/` | nothing | re-run `harness/codex/install.sh` — the TOML is generated | nothing |
| A new agent file | nothing — the whole directory is linked | re-run `harness/codex/install.sh` | nothing |
| `instructions/AGENTS.md` | nothing — the destination is a symlink | nothing — same symlink | nothing — same symlink |
| A `commands/*.md` body | nothing — the destination is a symlink | nothing — same symlink | nothing — same symlink |
| A new command file | re-run the command link loop from *Install* — `ln -sf` is idempotent | re-run `harness/codex/install.sh` | re-run the same loop |
| A command renamed or deleted | re-run the loop, then `rm ~/.claude/commands/<old>.md` | re-run the installer, then `rm ~/.codex/prompts/<old>.md` | re-run the loop, then `rm ~/.config/opencode/command/<old>.md` |
| `harness/codex/router-inject.sh` | not applicable | re-run the installer if it moved, then trust it again with `/hooks` | not applicable |
| `plugin/hooks/` | bump `version` in `plugin/.claude-plugin/plugin.json`, then `claude plugin marketplace update sadensmol` and `claude plugin update sadensmol` | not applicable | not applicable |

After `git pull`, apply the rows that match what the commits changed. Nothing else is
needed, because the working tree is what every harness reads.

Check what is installed:

```bash
npx skills list --global      # links per agent
claude plugin list            # plugin version Claude Code is running
```

## Where each harness looks

Skills:

| Harness | Global | Project |
| --- | --- | --- |
| Claude Code | `~/.claude/skills` | `.claude/skills` |
| Codex | `~/.agents/skills`, `~/.codex/skills` | `.agents/skills` |
| OpenCode | `~/.config/opencode/skills`, `~/.agents/skills` | `.opencode/skills`, `.claude/skills`, `.agents/skills` |

Commands:

| Harness | Global | Project |
| --- | --- | --- |
| Claude Code | `~/.claude/commands` | `.claude/commands` |
| Codex | `~/.codex/prompts` (top level only) | — |
| OpenCode | `~/.config/opencode/command` | `.opencode/command` |

## Checking your work

```bash
npx skills add ./skills --list   # lists all 24; a missing one has broken frontmatter
claude plugin validate .           # marketplace manifest
claude plugin validate ./plugin    # plugin manifest
```

For the whole install rather than one file, run the container checks:

```bash
docker compose -f test/compose.yaml run --rm test
```

They install this repository into a throwaway container the way *Install* above
says to, then assert that all three harnesses resolve every skill from this
repository and that an edit made through a harness's install path lands
back here. The repository is mounted read-only and copied inside first, so a run
cannot write to your working tree. No login and no model calls are involved —
the CLIs are driven through their offline introspection commands (`opencode debug
skill`, `codex debug prompt-input`, the Claude Code hook itself). Details and
the full check list: [test/README.md](test/README.md).

## Rules for this repo

This repository is **public**. Every file must be usable by a stranger who has none of my
projects, machines or accounts.

- No secrets. Reference them by environment variable name only.
- No local paths (`/Users/…`, `/home/…`, `~/work/…`). Derive paths at runtime or ask.
- No project-specific knowledge — that lives in private namespace repos of the same shape.
- Keep `SKILL.md` under ~500 lines. Move detail into `references/`.
- Write a trigger-rich `description`: it is the only thing an agent sees until the skill loads.

`CLAUDE.md` holds the long form of these rules.
