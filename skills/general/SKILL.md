---
name: sadensmol
description: "The /sadensmol command and the skill architecture rules. Typed by the user as /sadensmol or /sadensmol help, it lists the sadensmol- commands; as /sadensmol <command> [args], it runs the skill sadensmol-<command>. Loaded by Claude, it gives the rules for how the skill set is built: the terms namespace, workspace and project; the base sadensmol- skills are generic and read each workspace's data from its local .workspace/ registry; a workspace skill overrides them on its subject; secrets, settings and personal data stay in .workspace/ too. Use BEFORE creating, editing, moving, splitting or reviewing ANY skill, in any namespace (sadensmol-* or a workspace's own namespace), and before adding a hook, a shared skill name, a routing rule, a namespace command or a workspace skill. Also use when the user asks where a rule or a setting should live, or says a base skill knows too much about a workspace."
---

# Sadensmol skill architecture

This skill has two uses. Find yours first:

- The user's current message invokes the slash command `/sadensmol` (with or without
  arguments): do the steps in *Invoked by the user as `/sadensmol [command]`*. Do nothing else.
- Claude loaded this skill for work on skills: skip that section, and apply the rules
  from *The two layers* to the end.

## Invoked by the user as `/sadensmol [command]`

This command lists and runs the `sadensmol-` skills. Do no other work, and load no skill, until
the lister below has answered.

The lister is `scripts/list-commands.py` in the `sadensmol` skill directory:
`$HOME/.claude/skills/sadensmol`, or `$HOME/.agents/skills/sadensmol/general`
where skills are installed by namespace. Run it with `python3 -I`.

Argument string: $ARGUMENTS (the text after `/sadensmol` in the user's message).

- **Empty, or `help`:** run `list-commands.py sadensmol` and show its output as it is. Add nothing.
- **Otherwise:** the first word is the command, the rest are its arguments. Run
  `list-commands.py sadensmol <command>`. It prints the skill id `sadensmol-<command>`, or fails.
  - On success, ensure that skill is loaded. If its full instructions are already in the
    current context, do not invoke it again. Otherwise invoke it now: your harness runtime
    file answers `load-skill`, and reading the `SKILL.md` file is not a substitute. When
    the skill says to load a `sadensmol-` base first, load the base first. Then forward
    the remaining arguments to it verbatim.
  - On failure, say in one line that `/sadensmol <command>` does not exist, then show the list.
    Never fall back to a skill outside the `sadensmol-` namespace.

## Terms (MUST FOLLOW)

Use these three terms with these meanings only. They match the Linear and Slack terms.

| Term | Meaning | Example |
|---|---|---|
| Namespace `sadensmol` | The base skills, `sadensmol-<name>`. Generic: they work for any workspace and name none. | `sadensmol-task` |
| Workspace | One company context: one agterm workspace, one Linear workspace, one Slack workspace, one `.workspace/` folder. Its optional skills are `<workspace>-<name>`, in its private `work_<workspace>` repository. | `acme` |
| Project | A project inside a workspace: a Linear project (or team), a Slack channel and usually a repository. | `web`, `mobile` in the `acme` workspace |

This skill holds only the terms and the general rules, never workspace or domain data.

## The two layers

Two layers. The base layer gives general instructions, knowledge and default workflows. It
holds no workspace data at all. A workspace layer holds one workspace's own data and
workflows, and builds on the base layer. Every skill edit starts with a decision: which layer
gets the knowledge.

| | Base layer | Workspace layer |
|---|---|---|
| Name | `sadensmol-<name>` | `.workspace/` config; optional `<workspace>-<name>` skills |
| Source | the public skills repository, `skills/<name>/` | the local `.workspace/` folder of the workspace folder; the workspace's private skills repository (a `work_<workspace>` checkout), `skills/<name>/` |
| Readers | anyone, forever (public, git history included) | the author only |
| Holds | principles, workflows and mechanics for every workspace, and default workflows that read the workspace data from `.workspace/` | the workspace's business data, and its own workflows where they differ |

The base repository's own rules (no secrets, no local paths, no workspace facts, the pre-commit
sweep) are in its `CLAUDE.md`. They apply to every edit there, also from a session that
started in a different folder. Read that file before you change a base skill.

## Defaults come from the workspace (MUST FOLLOW)

- A base skill can hold a defaults section: a generic workflow for its subject. It reads every
  workspace fact (workspace names, projects, teams, issue prefixes, statuses, channels, repos,
  history) from the workspace registry (`.workspace/settings.json` and `.workspace/projects/<project>.json`). It never writes one down.
- The user sets the registry up once, when the workspace is created. A missing file or key:
  stop, name the key and the file path. Never guess a value.
- A loaded `<workspace>-<name>` skill replaces the defaults section of its subject whole. Do
  not mix the two. Example: `/task` in a workspace with no `<workspace>-task` skill follows the
  default flow of `sadensmol-task` with that workspace's registry. In the `acme` workspace with
  an `acme-task` skill it follows `acme-task`, and the default flow does not apply.
- The general instructions of the base skill (principles, tool mechanics) stay in force
  under a workspace skill, unless the workspace skill says otherwise on that subject.

## Dependency direction (MUST FOLLOW)

**A workspace skill can depend on a base skill. A base skill never depends on a workspace.**

- A workspace skill says `Load sadensmol-<name> first` and adds only its workspace's part.
- A base skill names no workspace, not even the author's own: no workspace, project, team,
  issue prefix, status, channel, repo or tracker site, and no workspace history. It writes
  `<workspace>`, `<project>`, a registry key, or an invented example that is no real workspace
  (`acme`, `ABC-123`).
- A base skill never has an `if <workspace>` branch, and never explains a workspace's flow
  "for example". That text goes to the workspace skill or the registry, whole.

## What a skill holds, and what `.workspace/` holds (MUST FOLLOW)

A base skill holds only principles, mechanics and generic workflows. A workspace skill holds
its workspace's workflows and business data. The local `.workspace/` folder of the workspace
folder holds the rest:

- `settings.json`: the workspace's own data (its name, Linear and Slack workspace, shared team,
  statuses, default label).
- `projects/<project>.json`: one file per project (`projects/<project1>.json`,
  `projects/<project2>.json`, …). Each holds that project's data and settings: its channel,
  Linear team and project, repos, and its own tokens and secrets. A skill reads a project's
  secret keys only for the step that needs them and never prints them.
- `settings.json` and `projects/` together are the workspace **registry**. Every workspace has
  one, set up once when the workspace is created; the base defaults read it.
- `workspace.json`: settings and tool configs (MCP server names, auth stores, permission files).
- secrets and tokens;
- personal data: emails, full names, user ids;
- machine facts: home paths and other local paths.

Rules for `.workspace/`:

1. It is local and never committed. Give it `chmod 700`.
2. When the workspace folder is a git repository, add `.workspace/` to `.git/info/exclude`.
3. A skill reads `.workspace/workspace.json` and the registry. To find them, walk
   up from the cwd to the first folder that holds `.workspace/`.
4. When the folder or a key is missing, stop. Tell the user the key name and the file path
   that must hold it. Do not guess a value, and do not use a value from a different workspace.
5. Print key names only, never the values.

## Hooks — how a base skill reaches workspace detail

When a base workflow has a workspace-specific step, it defines a **hook**:

1. The base skill names the hook point and the placeholder skill: "step 4 — when a
   `<workspace>-<name>` skill exists, load it and run its steps here".
2. It finds `<workspace>` generically: the agterm workspace name, then the local
   routes file (`sadensmol-task` → Step 2 is the reference procedure). Never a list of
   workspaces.
3. The workspace skill `<workspace>-<name>` holds the step itself, and says which base step
   it hooks into.
4. A workspace with no `<workspace>-<name>` skill uses the base defaults, with the data of its
   own registry. When the base skill has no defaults, it skips the hook silently, or stops
   when the base skill says so.

`sadensmol-task` with `<workspace>-task`, and `sadensmol-retro` with `<workspace>-retro`, use
this design.

## Where does a rule or learning go?

Decide this before you open a file:

| The knowledge is… | It goes to |
|---|---|
| true for every workspace (a principle, a tool mechanic, a default workflow) | the matching `sadensmol-<name>` skill |
| a workspace fact (its name, projects, teams, prefixes, statuses, channels, repos, history) | that workspace's registry (`settings.json`, or `projects/<project>.json` for one project), or its `<workspace>-<name>` skill when it has one on that subject |
| a workflow that differs in one workspace | that workspace's `<workspace>-<name>` skill |
| a general step whose details differ per workspace | a hook in the base skill + the details in each `<workspace>-<name>` |
| a workspace rule with no matching workspace skill yet | a new `<workspace>-<name>` skill (below), never the base skill "for now" |
| a secret, token, setting, tool config, personal data or a local path | the workspace's `.workspace/`, never a skill |

**Pick the skill by subject, not by the workflow that found the rule.** A tracker rule
(states, assignees, flow) goes to the tracker skill (`sadensmol-linear` / `<workspace>-linear`),
also when a task or retro step found it. A step of a workflow goes to that workflow's skill
(`<workspace>-task`, `<workspace>-retro`). It points to the subject skill and does not copy its rules.

## Adding a workspace skill that pairs with a base one

1. Create `skills/<name>/SKILL.md` in the **workspace's** skills repository, with
   frontmatter `name: <workspace>-<name>`. Add `user-invocable: false` when only the base
   skill's hook reaches it (the skill packager's validator rejects that key; the harness
   accepts it).
2. First line of the body: `Load sadensmol-<name> first (MUST FOLLOW)`, then the hook
   steps only. Do not repeat the base workflow.
3. Register it the way the workspace repository's `README.md` → *Install* says (a link
   named after the frontmatter `name`), and update the skill count there.
4. Add the bare name to the workspace router's shared skill names, so `<name>` loads the
   base first and then the workspace skill.
5. When the base skill has no hook point yet, add one (see Hooks). That is the only
   change the base skill gets.

## Shared (bare) names

A bare name that several namespaces define (`task`, `linear`, `retro`, …) is resolved per
session by the routers: the base first, then the workspace skill on top.
`sadensmol-router` → `references/shared-names.md` owns the rule. A slash command never
claims such a name for one namespace.

## Namespace commands

`/<namespace>` lists that namespace's user-invocable skills, and `/<namespace> <command> [args]`
runs `<namespace>-<command>`. Both go through `scripts/list-commands.py <namespace> [<command>]`
in this skill's directory, which reads the installed skills. A new skill
therefore appears with no edit. Give a skill that only a hook reaches `user-invocable: false`
to keep it off the list.

- `/sadensmol` is this skill: its first section runs the lister, from its own `scripts/`.
- A workspace namespace repository has one `commands/<namespace>.md`. Its body is the steps of
  *Invoked by the user as `/sadensmol [command]`* with its own namespace.

## Before saving any skill edit

- Grep the skills you touched for workspace and project names, private
  namespaces, local paths, emails, full names and secrets. The sweep is in the base
  repository's `CLAUDE.md`, and the list of the private names is
  `.workspace/private-names.txt`. Read every hit.
- Check that no base skill text names a different workspace's `<workspace>-…` skill by its real name.
- Check that no skill holds a setting, tool config or personal data. Move it to `.workspace/`.
- Check that each workspace skill you touched loads its base and holds only workspace detail.
- When a rule was broken, fix the rule that let it happen as well as the line.
