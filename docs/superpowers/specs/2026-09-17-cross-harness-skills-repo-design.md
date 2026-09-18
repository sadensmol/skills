# Cross-harness skills repository — design

Date: 2026-09-17
Status: approved, not yet implemented

## Problem

The same skills are maintained in three places and two of them drift:

1. `cc-stuff` — a Claude Code marketplace installed from a local directory. Claude Code
   does not read that directory. It copies the plugin into
   `~/.claude/plugins/cache/<marketplace>/<plugin>/<version>/` at install time. The
   installed copy of `router/SKILL.md` already differs from the repository, because
   refreshing it needs a version bump plus `/plugin update`.
2. `~/.agents/skills/<namespace>/<skill>/` — hand-made copies for Codex and OpenCode,
   adapted by renaming `name:` to `<namespace>-<skill>`. Also diverged from `cc-stuff`.
3. Private namespace repositories of the same shape, with the same duplication.

All three harnesses now read the same open Agent Skills format, so the duplication buys
nothing.

## Goal

One copy of every skill in this repository. Editing a file here changes the behaviour of
Claude Code, Codex and OpenCode with no copy step and no re-install.

## Decisions

### Layout

```
.
├── .claude-plugin/marketplace.json
├── CLAUDE.md
├── README.md
├── <namespace>/                      e.g. sadensmol/
│   └── <skill>/SKILL.md
└── plugin/
    ├── .claude-plugin/plugin.json
    ├── hooks/
    └── agents/
```

The namespace folder mirrors the structure already running in `~/.agents/skills`, so the
whole namespace can be exposed to Codex and OpenCode with a single symlink.

The plugin folder deliberately contains **no** `skills/` directory. If it did, Claude Code
would load every skill twice — once from the plugin cache, once from the symlinks — and the
cached copy would be the stale one.

### Naming

Directory: `<namespace>/<skill>/`. Frontmatter: `name: <namespace>-<skill>`.

Claude Code namespaces plugin skills as `plugin:skill`. Codex and OpenCode have a single
flat namespace, where a bare `router` from two namespaces collides. Putting the namespace
in `name` gives one identifier that works in all three: `sadensmol-router`.

This is the convention already used in `~/.agents/skills`, and the skills CLI follows it —
it links a skill under its frontmatter `name`, falling back to the directory name
(`installName: entry.name`, `sanitizeName(skill.name || basename(skill.path))`).

Cost: the Agent Skills specification says `name` must equal the parent directory name, so
`skills-ref validate` is expected to reject these skills. The specification's rule exists to
make a skill's identity unambiguous; the prefix serves the same purpose across harnesses
that have no namespacing. We keep the prefix and do not adopt `skills-ref` as a gate.

### Distribution

| Target | Mechanism | Live on edit |
| --- | --- | --- |
| Codex, OpenCode | `~/.agents/skills/<namespace>` → symlink to `<repo>/<namespace>` | yes |
| Claude Code skills | `~/.claude/skills/<namespace>-<skill>` → symlink per skill, created by `npx skills add . --agent claude-code --global` | yes |
| Claude Code hooks and agents | plugin installed from the marketplace manifest | no — needs version bump and `/plugin update` |

Claude Code follows symlinks in the personal skills directory and loads a target only once
even when several locations link to it. Codex reads `~/.agents/skills` natively. OpenCode
reads it through `skills.paths` in `opencode.json`, which is already configured.

Hooks, agents and commands have no equivalent in the other two harnesses and cannot be
installed by the skills CLI, so they stay in a plugin. They change rarely, which makes the
bump-and-update cycle acceptable for them alone.

### Update flow

Edit the `SKILL.md` in this repository. Every harness sees the change at its next session.

- A new skill folder needs one new symlink for Claude Code; Codex and OpenCode need
  nothing, because the namespace folder is linked as a whole.
- A hook or agent change needs a version bump in `plugin/.claude-plugin/plugin.json` and
  `/plugin update`.

### Scope of the import

Everything in the `cc-stuff` plugin moves here: 21 skill folders (35 `SKILL.md` files
including nested ones), 5 code-reviewer agents, `hooks/` (skill activation, secret scan,
pre-commit), `CLAUDE.md`, the plugin and marketplace manifests, and the version-bump
workflow. Files are copied, not history-filtered; `cc-stuff` keeps its history and gets a
pointer to this repository.

Private namespace repositories adopt the same shape afterwards, as a separate phase. They
stay private and keep their own namespace folders.

## Migration phases

1. **Import.** Copy the plugin contents into the new layout: skills to
   `<namespace>/<skill>/`, hooks and agents to `plugin/`, `CLAUDE.md` and the manifests to
   the root. Adjust the marketplace manifest to point at `plugin/`.
2. **Rename.** Set `name: <namespace>-<skill>` in every `SKILL.md`. Rewrite `<namespace>:x`
   references inside skill bodies, in `plugin/hooks/skill-activation.sh`, and in `CLAUDE.md`
   to the new identifiers. The copies in `~/.agents/skills` already carry most of this
   transformation and are the reference for wording, but the `cc-stuff` text is canonical
   where the two differ.
3. **Fix frontmatter.** `router/SKILL.md` has an unquoted `description` containing a colon
   followed by a space. It is invalid YAML: the skills CLI reports
   "Nested mappings are not allowed in compact mappings" and skips the skill. Quote it, and
   check every other description for the same pattern.
4. **Wire and verify.** Create the symlinks, install the plugin, and confirm in each
   harness that skills resolve under the new identifiers.
5. **Retire `cc-stuff`.** Replace its content with a pointer once the new setup is verified.
6. **Private repositories.** Repeat phases 1–4 for each private namespace repository.

## Verification

Per phase:

- After phase 3: `npx skills add . --list` lists every skill, with none skipped.
- After phase 4, with a throwaway skill first, then for real:
  - Claude Code lists the skill as `<namespace>-<skill>` and invoking it loads the file
    from this repository, not from a plugin cache path.
  - Codex lists it after a restart.
  - OpenCode lists it after a restart.
  - Editing the body in this repository changes what the harness shows, with no re-install.
- After phase 5: a fresh session in each harness still resolves every skill, and no skill
  appears twice.

## Open items

Both are settled by the throwaway skill in phase 4, before anything is migrated in bulk.

1. Whether Claude Code identifies a personal skill by the symlink's directory name or by
   the frontmatter `name`. The plan makes them equal at the link
   (`~/.claude/skills/<namespace>-<skill>`), so either answer gives the same identifier.
2. Whether any harness rejects a `name` that differs from the directory name inside the
   repository. The current `~/.agents/skills` tree already has this mismatch and works,
   which is evidence but not proof for the other two harnesses.

## Rejected alternatives

- **Keep plugin namespacing for Claude Code** (`<namespace>:router`) and install the same
  folders flat for Codex and OpenCode. Rejected: the flat installs collide as soon as a
  second namespace ships a `router`, `task` or `linear`.
- **Prefix the directory names** (`<namespace>/<namespace>-router/`). Spec-clean, but it
  repeats the namespace in every path and abandons the layout already in use.
- **Copy instead of symlink** (`skills add --copy`). Restores the drift this design exists
  to remove.
- **A custom sync script.** Nothing left to do once the symlinks exist.
