---
name: sadensmol-linear
description: Linear issue management via the Linear MCP server that the project skill names (one server per Linear workspace). Use when the user wants to (1) check status of a Linear issue (e.g. "what's the status of PROJ-123"), (2) list their assigned/in-progress/backlog issues ("show my tasks", "what am I working on"), (3) search issues by query/label/project ("find issues about TLS"), (4) update an existing issue (change status, reassign, edit title/description/priority, add labels), (5) add subtasks/child issues to an existing parent, (6) read or post comments on issues, or (7) any other reference to Linear, an issue identifier matching `[A-Z]+-\d+` (e.g. PROJ-123), or a Linear URL.
---

# Linear

Manage Linear issues through the Linear MCP tools (`mcp__<server>__*`). This skill encodes workspace defaults and the standard workflows so calls are correct on the first try.

## Workspace configuration

This skill is workspace-agnostic. **Never hardcode workspace identity** (user email, user/team UUIDs, team names, issue prefixes, or status names) into this skill. Resolve them at runtime, in this order:

1. **Environment variables** (highest precedence):
   - `LINEAR_DEFAULT_TEAM` — team name used when the user doesn't specify one (e.g. `Engineering`).
   - `LINEAR_ISSUE_PREFIX` — issue identifier prefix for that team (e.g. `ENG`), used for branch↔issue mapping.
   - `LINEAR_USER` — optional; the current user's email or name. Usually unnecessary — prefer `assignee: "me"`, which the MCP server resolves.
2. **Per-project config** (external to this skill): if a project documents its Linear setup (e.g. in the repo's `CLAUDE.md`, `AGENTS.md`, or a `.linear` config file), read team / prefix / statuses from there.
3. **Fall back to the MCP server**: if neither is set, don't guess. Use `list_teams` to discover teams, `list_issue_statuses` for a team's legal `state` values, and `assignee: "me"` for the current user. Ask the user which team to default to if it's ambiguous.

**Status names are workspace-specific** custom workflow states — fetch them with `list_issue_statuses` for the relevant team rather than assuming a fixed set. A typical Linear team maps states to these *types*: `triage`, `backlog`, `unstarted` (Todo), `started` (In Progress / In Review), `completed` (Done), `canceled`. Pass the exact **name** (case-sensitive) returned by the MCP server when setting `state`.

## Server — one per Linear workspace (MUST FOLLOW)

Each Linear workspace has its own MCP server and auth. The project skill
(`<project>-linear` or `<project>-<project>`) names the server for the current folder; this
skill never picks one. The claude.ai Linear connector is bound to ONE account — use it only
when the project skill says so.

- Before the first write, call `whoami` and confirm its `workspace` is the project's. Wrong
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

**Follow the project-level skill first.** If a `<project>-linear` (or equivalent project)
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
   by `team`) shows the options; the project-level skill usually names the routing rule.
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

## When NOT to use this skill

- Non-Linear ticketing systems (Jira, GitHub Issues, etc.) — those have their own tools.
- Reading external Linear docs/help articles — use `search_documentation` for the product help center, but general programming questions don't belong here.
