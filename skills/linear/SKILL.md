---
name: sadensmol-linear
description: Linear issue management via the Linear MCP server that the workspace skill names (one server per Linear workspace). Use when the user wants to (1) check status of a Linear issue (e.g. "what's the status of PROJ-123"), (2) list their assigned/in-progress/backlog issues ("show my tasks", "what am I working on"), (3) search issues by query/label/project ("find issues about TLS"), (4) update an existing issue (change status, reassign, edit title/description/priority, add labels), (5) add subtasks/child issues to an existing parent, (6) read or post comments on issues, or (7) any other reference to Linear, an issue identifier matching `[A-Z]+-\d+` (e.g. PROJ-123, ABC-<n>), or a Linear URL (e.g. `linear.app/<workspace>`). Its defaults read the workspace registry (`.workspace/settings.json`); a workspace's own `<workspace>-linear` skill replaces them.
---

# Linear

Manage Linear issues through the Linear MCP tools (`mcp__<server>__*`). This skill encodes workspace defaults and the standard workflows so calls are correct on the first try.

## Workspace configuration

The general mechanics of this skill are workspace-agnostic. **Never hardcode workspace identity** (user email, user/team UUIDs) into this skill. The workspace skill names the team names, issue prefixes and status names; with no workspace skill, the workspace registry supplies them (*Defaults* below). When neither names them, resolve them at runtime, in this order:

1. **Environment variables** (highest precedence):
   - `LINEAR_DEFAULT_TEAM` — team name used when the user doesn't specify one (e.g. `Engineering`).
   - `LINEAR_ISSUE_PREFIX` — issue identifier prefix for that team (e.g. `ENG`), used for branch↔issue mapping.
   - `LINEAR_USER` — optional; the current user's email or name. Usually unnecessary — prefer `assignee: "me"`, which the MCP server resolves.
2. **Per-project config** (external to this skill): if a project documents its Linear setup (e.g. in the repo's `CLAUDE.md`, `AGENTS.md`, or a `.linear` config file), read team / prefix / statuses from there.
3. **Fall back to the MCP server**: if neither is set, don't guess. Use `list_teams` to discover teams, `list_issue_statuses` for a team's legal `state` values, and `assignee: "me"` for the current user. Ask the user which team to default to if it's ambiguous.

**Status names are workspace-specific** custom workflow states — fetch them with `list_issue_statuses` for the relevant team rather than assuming a fixed set. A typical Linear team maps states to these *types*: `triage`, `backlog`, `unstarted` (Todo), `started` (In Progress / In Review), `completed` (Done), `canceled`. Pass the exact **name** (case-sensitive) returned by the MCP server when setting `state`.

## Server — one per Linear workspace (MUST FOLLOW)

Each Linear workspace has its own MCP server and auth. The workspace skill
(`<workspace>-linear`) names the server for the current folder; with no workspace skill,
`.workspace/workspace.json` names it (*Defaults* below). This skill never picks one in any
other way. The claude.ai Linear connector is bound to ONE account — use it only when the
workspace skill says so.

- Before the first write, call `whoami` and confirm its `workspace` is the current workspace's. Wrong
  workspace → stop.
- The named server is missing from the session → say so and stop. Never fall back to another
  workspace's server, even when its key happens to resolve.
- Below, tool names are bare (`get_issue`); call them as `mcp__<server>__get_issue`.

## Tool loading (deferred)

All Linear tools are **deferred** — schemas aren't loaded by default. Load them via `ToolSearch` before the first call:

```
ToolSearch query="select:mcp__<server>__list_issues,mcp__<server>__get_issue"
```

Batch related tools in one ToolSearch call.

## Workflows

**Read on demand** (`references/workflows.md`):

- Read *Search / list issues* when you list or search issues (filters, common recipes, `limit`).
- Read *Add a subtask to an existing issue* when you create a child issue under a parent.
- Read *Reassign an issue* when you change an issue's assignee.
- Read *Issue labels* when you list or create labels.

Read `references/best-practices.md` before you set up or restructure teams, projects or labels,
or when you choose where new work goes (team vs project vs initiative, naming, agent setup).

### Get a single issue

`get_issue` with `id` set to either the UUID or the human identifier (`PROJ-123`). Use this to read the full description, parent/children, project/cycle, labels, and current state before updating.

### Update an issue (status, assignee, fields)

`save_issue` is upsert — supply `id` to update, omit it to create.

To update, pass `id` plus only the fields you're changing. Common fields:

- `state` — pass the **status name** (e.g. `"In Review"`) or its id. Names are case-sensitive.
- `assignee` — name, email, UUID, or `"me"`. Pass `null` to unassign.
- `title`, `description` (markdown), `priority` (0–4).
- `labels` — array of label names or ids. Replaces the set; fetch existing first if you need to add.
- `project`, `cycle`, `parent` (parent issue id/identifier).

Always confirm with the user before changing `state`/`assignee` on an issue you didn't just create — these are visible to the team.

Change the state only of the issue the user named or the branch maps to. Another issue whose
scope the work partly covers keeps its state; post a comment on it instead.

### Create an issue

**Follow the workspace-level skill first.** If a `<workspace>-linear` (or equivalent workspace)
skill is loaded, its rules for status, cycle, estimate, title format, project/milestone and
labels win — this skill only supplies the mechanics. Read it before the create call, and pass
every field it mandates **in the create call itself**.

**Assignee default: `"me"`.** When the request names nobody, set `assignee: "me"` — never
leave a new issue unassigned. Pass a different person only when the user names one (resolve
with `list_users` if only a first name is given).

**Every issue belongs to a project.** Never create one with `project` unset. Resolve it in
this order:

1. **It has a parent** → `get_issue` the parent and copy its `project` (and `milestone`). A
   subtask in a different project than its story is a mistake, not a choice.
2. **No parent** → pick the project whose subject matches the work. `list_projects` (filter
   by `team`) shows the options; the workspace-level skill usually names the routing rule.
3. **Still ambiguous** → ask the user which project, and name the two or three candidates.
   Do not create the issue project-less "for now".

Skip completed / archived / canceled projects — a new ticket goes in an active one.

**Relate a ticket by its subject, not by where it was found.** A ticket found during other
work takes its parent, project and `relatedTo` link from that issue only when its work belongs
to that issue's subject. An unrelated finding (a pre-existing failure, another service) is
routed by its own subject, as if no task were active.

### Comments

- Read: `list_comments` with `issue` (UUID or `PROJ-123`).
- Post: `save_comment` with `issue` and `body` (markdown). Omit `id` to create.
- Send markdown content with **real newlines**, never literal `\n` escape sequences (per the MCP server's instructions).

## Conventions

- **Identifiers in output**: when reporting on issues, always show the `<PREFIX>-###` identifier (and URL if available) so the user can click through.
- **Don't bulk-update** without explicit confirmation. If a query returns >5 issues and the user asks to change them, list them first and ask before applying.
- **Don't create issues unprompted** — even when fixing bugs or doing related work, only `save_issue` (create form) when the user explicitly asks.
- **Follow-up tickets go to Todo, on top (MUST FOLLOW)**: a ticket created out of other work (deferred finding, split-off scope, bug found on the way) is NOT started work. Create it in the team's `unstarted` status (`Todo`), never in a `started` one (`In Progress`), and never copy the source issue's started status onto it. Put it at the top of the Todo list. `save_issue` has no position field, so when it does not land on top, say so in one line with the key, so the user drags it there. This overrides any project rule that copies the source issue's `state` onto a new ticket.
- **Status transitions**: map the user's intent to the workspace's actual status names (fetch via `list_issue_statuses` if unknown). E.g. "mark done" → the `completed`-type status, "in review" → the relevant `started` status, "block it" → a blocked/needs-info `backlog` status.
- **Branch ↔ issue mapping**: branches are typically named `<prefix>-123-...` (lowercase prefix, dashes). When the user is on such a branch, default Linear queries to that issue.

## Defaults — the workspace registry

This section applies only when no `<workspace>-linear` skill is loaded. A loaded workspace Linear
skill replaces this whole section. Use none of it then.

No workspace fact is written in this skill. Two local files of the workspace folder hold them
(find `.workspace/` by walking up from the current directory; never use an absolute path):

- `.workspace/workspace.json` → `linear.server`: the MCP server. Call its tools as
  `mcp__<server>__*`. The claude.ai Linear connector is another account; never use it here.
  Claude Code loads the server from the MCP config file named by `linear.mcpConfig` (it asks
  to approve it in the first session). OpenCode uses `opencode.mcp.linear` of the same
  `workspace.json`, through the repository's `harness/opencode/workspace-mcp.py`. Both use the auth store named by `linear.authStore`.
- the registry (`sadensmol-project` → *Registry*). `.workspace/settings.json` holds the Linear
  workspace (`linear.workspace`), the statuses (`linear.statuses`), the default type label
  (`linear.default_label`); `.workspace/projects/<project>.json` holds each project (Slack channel → team, Linear project,
  repos). Read it with `python3 -I <sadensmol-project-skill-dir>/scripts/registry.py`.

Folder, file or key missing: stop. Name the key and the file. Never guess a value.

- Call `whoami` before the first write. Its workspace must be the registry's
  `linear.workspace`. Else stop.
- Pass `assignee: "me"`.
- Default team: the team of the project the work belongs to (`registry.py products`).
  When every project shares one team, that team is the default for everything.

### Workspace structure

- A project is a Linear project in the shared team (`registry.py products`). Never a team per
  project: one team keeps the Free plan (2 teams) open to any number of projects.
- A project is ongoing: its Linear project stays started. A feature is a milestone inside it,
  a finishable piece with an outcome. A goal across projects is an initiative.
- Never create a team, a Linear project or a Slack channel for a feature.
- All projects share the team's issue key. The key does not name the project; the issue's
  Linear project does.

### Linear and Slack

The Linear workspace has one Slack integration and one `@Linear` bot (registry
`slack.workspace`). Each project gets its own behaviour from that one bot:

1. **Notifications:** the Linear project's bell (*Slack notifications*) → its channel. The
   shared team has no team channel: it would get every project's issues.
2. **Project channels:** Settings > Integrations > Slack > *Project channels* stays off. The
   project channel exists already, and a feature gets none.
3. **Routing:** the Slack agent guidance keeps one line per project channel. It is rendered from
   the registry (*Keep the Linear Agent guidance in step* below).
4. **Access:** every user connects their personal Slack account (Settings > Connected
   accounts). Without it the bot replies "you can't message this instance of @Linear".

Which channels exist is `sadensmol-slack` → *Defaults*.

### Statuses

The registry maps phases to status names: `todo`, `implementing`, `done`, `canceled`, and the
autopilot phases `planning`, `qa`, `review`, `pr`, `blocked`. Only autopilot runs
(`sadensmol-autopilot`) move an issue to an autopilot status. The task flow never uses them.
The backlog and duplicate statuses are not phases: find them by type (`backlog`,
`duplicate`) with `list_issue_statuses`. Intent mapping:

- "start" → `implementing`.
- "in review" → stay `implementing` and comment the PR.
- "mark done" → `done`.
- "cancel" → `canceled`.
- "duplicate of" → the `duplicate`-type status.

### Cancelling a ticket

Use `canceled`. Use the duplicate status when another ticket covers the work. Never use
`done`. Comment the reason first. For a duplicate, link the covering ticket.

### Branch from issue

Branch = Linear's `gitBranchName` without the user prefix (`ann/abc-21-add-health-endpoint`
→ `abc-21-add-health-endpoint`). The key is the first `<team>-<n>` part, in uppercase
(`ABC-21`). The project is the registry project of the issue: its Linear `project` when projects
share the team (`registry.py product-of-project <project>`), else its team. The project's
first repo is the repo of the task (`registry.py repo-of-product <project>`).

### Creating a ticket

- **Team and Linear project:** the project's (`registry.py products`). When projects share a team,
  the Linear project is required: the task flow and autopilot find the repo through it. A subtask
  copies the Linear project of its parent. If the project is not clear, ask with the candidates.
- **Feature:** the milestone inside the project's Linear project when projects share a team (else a
  Linear project inside the project's team), when the ticket belongs to one. A subtask copies its
  parent's feature.
- **Assignee:** `"me"` unless the user names someone.
- **Labels:** one type label: `Bug`, `Feature` or `Improvement`. Never create a label without asking.
- **Title:** what changes, in plain words. Research work starts with `r&d | ` and ends with
  the question it answers (`r&d | how to cache feeds?`).
- No cycles and no estimates. Leave both unset.
- Issues that autopilot creates get the Linear project of their Slack channel's project, and no
  feature.

### Status at task finish

After the PR is open, post one comment on the issue with the PR URL. The status stays
`implementing`. `done` is set when the PR merges (by the user, or by the Linear GitHub
automation when it is connected). An r&d task with no code PR: comment the location of the
findings document. Then ask the user before you set `done`.

### Keep the Linear Agent guidance in step

The Linear Agent guidance fields are rendered from the registry and the template
`sadensmol-project` → `references/agent-guidance.md` (`registry.py render-guidance`). A
convention change in this section updates that template in the same change. Linear cannot
read files, so the rendered text is pasted into Linear. Never edit the fields in Linear only.

## When NOT to use this skill

- Non-Linear ticketing systems (Jira, GitHub Issues, etc.) — those have their own tools.
- Reading external Linear docs/help articles — use `search_documentation` for the product help center, but general programming questions don't belong here.
