---
name: sadensmol-sadensmol
description: "How the skill set is built: the generic sadensmol- layer, the private per-project layers, which way dependencies may point, hooks, shared names, and where a rule or learning belongs. Use BEFORE creating, editing, moving, splitting or reviewing ANY skill, in any namespace (sadensmol-* or a project's own namespace), and before adding a hook, a shared skill name, a routing rule or a project skill — also when the user asks where a rule should live, or says a generic skill knows too much about a project."
---

# Sadensmol skill architecture

Two layers. The generic layer knows nothing about any project. A project layer knows its
own project and builds on the generic layer. Every skill edit starts by deciding which
layer the knowledge belongs to.

## The two layers

| | Generic layer | Project layer |
|---|---|---|
| Name | `sadensmol-<name>` | `<project>-<name>` |
| Source | the public skills repository, `skills/<name>/` | the project's private skills repository (a `work_<project>` checkout), `skills/<name>/` |
| Readers | anyone, forever (public, git history included) | the author only |
| Holds | principles, workflows, mechanics that work for every project | the project's facts: names, repos, trackers, teams, statuses, people, flows, envs |

The generic repository's own rules (no secrets, no local paths, the pre-commit sweep) are in
its `CLAUDE.md`. They bind every edit there, also from a session that started elsewhere:
read that file before you change a generic skill.

## Dependency direction (MUST FOLLOW)

**A project skill may depend on a generic skill. A generic skill never depends on a project.**

- A project skill says `Load sadensmol-<name> first` and adds only its project's part.
- A generic skill never names a real project, project skill, repo, tracker site, team,
  status name, person, workspace or path. It writes `<project>`, `<project>-<name>`, or an
  invented example that is no real project (`acme`, `ABC-123`).
- A generic skill never has an `if <project>` branch, and never explains one project's flow
  "for example". That text goes to the project skill, whole.

## Hooks — how a generic skill reaches project detail

When a generic workflow has a project-specific step, it defines a **hook**:

1. The generic skill names the hook point and the placeholder skill: "step 4 — when a
   `<project>-<name>` skill exists, load it and run its steps here".
2. It resolves `<project>` generically: the agterm workspace name, then the local
   routes file (`sadensmol-task` → Step 2 is the reference procedure). Never a list of
   projects.
3. The project skill `<project>-<name>` holds the step itself, and says which generic step
   it hooks into.
4. A project with no `<project>-<name>` skill skips the hook silently.

`sadensmol-task` with `<project>-task`, and `sadensmol-retro` with `<project>-retro`, are
built this way.

## Where does a rule or learning go?

Decide this before you open a file:

| The knowledge is… | It goes to |
|---|---|
| true for every project (a principle, a tool mechanic) | the matching `sadensmol-<name>` skill |
| about one project (its tracker flow, its people, its repos, its envs) | that project's `<project>-<name>` skill |
| a general step whose details differ per project | a hook in the generic skill + the details in each `<project>-<name>` |
| a project rule with no matching project skill yet | a new `<project>-<name>` skill (below) — never the generic skill "for now" |

**Pick the skill by subject, not by the workflow that hit it.** A tracker rule (states,
assignees, flow) goes to the tracker skill (`sadensmol-linear` / `<project>-linear`), even
when a task or retro step found it. A step of a workflow goes to that workflow's skill
(`<project>-task`, `<project>-retro`), and it points to the subject skill instead of
copying its rules.

Test: would a stranger with none of the author's projects find the sentence useful and
fully understandable? No → it is project knowledge.

## Adding a project skill that pairs with a generic one

1. Create `skills/<name>/SKILL.md` in the **project's** skills repository, with
   frontmatter `name: <project>-<name>`. Add `user-invocable: false` when it is reached
   only through the generic skill's hook (the skill packager's validator rejects that key;
   the harness accepts it).
2. First line of the body: `Load sadensmol-<name> first (MUST FOLLOW)`, then the hook
   steps only. Do not repeat the generic workflow.
3. Register it the way the project repository's `README.md` → *Install* says (a link
   named after the frontmatter `name`), and update the skill count there.
4. Add the bare name to the project router's shared skill names, so `<name>` loads the
   generic base first and then the project skill.
5. When the generic skill has no hook point yet, add one (see Hooks). That is the only
   change the generic skill gets.

## Shared (bare) names

A bare name that several namespaces define (`task`, `linear`, `retro`, …) is resolved per
session by the routers: the generic base first, then the project skill on top.
`sadensmol-router` → `references/shared-names.md` owns the rule. A slash command never
claims such a name for one namespace.

## Before saving any skill edit

- Grep the generic skills you touched for project names, private namespaces, local paths
  and secrets (the sweep is in the generic repository's `CLAUDE.md`). Read every hit.
- Check that no generic skill text names a `<project>-…` skill by its real name.
- Check that each project skill you touched loads its generic base and holds only project
  detail.
- When a rule was broken, fix the rule that let it happen as well as the line.
