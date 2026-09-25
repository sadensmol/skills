---
name: sadensmol-linear
description: Linear issue management via the claude.ai Linear MCP server. Use when the user wants to (1) check status of a Linear issue (e.g. "what's the status of PROJ-123"), (2) list their assigned/in-progress/backlog issues ("show my tasks", "what am I working on"), (3) search issues by query/label/project ("find issues about TLS"), (4) update an existing issue (change status, reassign, edit title/description/priority, add labels), (5) add subtasks/child issues to an existing parent, (6) read or post comments on issues, or (7) any other reference to Linear, an issue identifier matching `[A-Z]+-\d+` (e.g. PROJ-123), or a Linear URL.
---

# Linear

Manage Linear issues through the `mcp__claude_ai_Linear__*` MCP tools. This skill encodes workspace defaults and the standard workflows so calls are correct on the first try.

## Workspace configuration

This skill is workspace-agnostic. **Never hardcode workspace identity** (user email, user/team UUIDs, team names, issue prefixes, or status names) into this skill. Resolve them at runtime, in this order:

1. **Environment variables** (highest precedence):
   - `LINEAR_DEFAULT_TEAM` — team name used when the user doesn't specify one (e.g. `Engineering`).
   - `LINEAR_ISSUE_PREFIX` — issue identifier prefix for that team (e.g. `ENG`), used for branch↔issue mapping.
   - `LINEAR_USER` — optional; the current user's email or name. Usually unnecessary — prefer `assignee: "me"`, which the MCP server resolves.
2. **Per-project config** (external to this skill): if a project documents its Linear setup (e.g. in the repo's `CLAUDE.md`, `AGENTS.md`, or a `.linear` config file), read team / prefix / statuses from there.
3. **Fall back to the MCP server**: if neither is set, don't guess. Use `list_teams` to discover teams, `list_issue_statuses` for a team's legal `state` values, and `assignee: "me"` for the current user. Ask the user which team to default to if it's ambiguous.

**Status names are workspace-specific** custom workflow states — fetch them with `list_issue_statuses` for the relevant team rather than assuming a fixed set. A typical Linear team maps states to these *types*: `triage`, `backlog`, `unstarted` (Todo), `started` (In Progress / In Review), `completed` (Done), `canceled`. Pass the exact **name** (case-sensitive) returned by the MCP server when setting `state`.

## Tool loading (deferred)

All Linear tools are **deferred** — schemas aren't loaded by default. Before calling any `mcp__claude_ai_Linear__*` tool, load it via `ToolSearch`:

```
ToolSearch query="select:mcp__claude_ai_Linear__list_issues,mcp__claude_ai_Linear__get_issue"
```

Batch related tools in one ToolSearch call.

## Workflows

### Search / list issues

Use `mcp__claude_ai_Linear__list_issues`. Useful filters (combine freely):

- `assignee` — `"me"` for own issues, or a name/email/UUID. `null` for unassigned.
- `team` — defaults to `LINEAR_DEFAULT_TEAM` (see Workspace configuration); pass team name or id to override.
- `state` — status name (e.g. `"In Progress"`, `"Todo"`) or type (`started`, `unstarted`, `completed`, `backlog`).
- `query` — fuzzy search on title/description.
- `project`, `cycle`, `label`, `priority` (0=None,1=Urgent,2=High,3=Normal,4=Low).
- `parentId` — to list subtasks of a parent (accepts UUID or `PROJ-123`).
- `updatedAt` / `createdAt` — ISO-8601 or duration like `-P7D` for "last 7 days".

Common recipes:

- **My active work**: `assignee="me"`, `state="In Progress"` (or omit `state` and read all).
- **My backlog**: `assignee="me"`, `state="Todo"`.
- **Default-team issues touching X**: `team=<LINEAR_DEFAULT_TEAM>`, `query="X"`.
- **Subtasks of a parent**: `parentId="<PREFIX>-123"`.

Default `limit` is 50; raise to up to 250 only when needed.

### Get a single issue

`mcp__claude_ai_Linear__get_issue` with `id` set to either the UUID or the human identifier (`PROJ-123`). Use this to read the full description, parent/children, project/cycle, labels, and current state before updating.

### Update an issue (status, assignee, fields)

`mcp__claude_ai_Linear__save_issue` is upsert — supply `id` to update, omit it to create.

To update, pass `id` plus only the fields you're changing. Common fields:

- `state` — pass the **status name** (e.g. `"In Review"`) or its id. Names are case-sensitive.
- `assignee` — name, email, UUID, or `"me"`. Pass `null` to unassign.
- `title`, `description` (markdown), `priority` (0–4).
- `labels` — array of label names or ids. Replaces the set; fetch existing first if you need to add.
- `project`, `cycle`, `parent` (parent issue id/identifier).

Always confirm with the user before changing `state`/`assignee` on an issue you didn't just create — these are visible to the team.

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

### Add a subtask to an existing issue

Subtasks are just regular issues with `parent` set. Create with `save_issue` (no `id`):

- `team` — required; defaults to `LINEAR_DEFAULT_TEAM`.
- `parent` — parent UUID or identifier (e.g. `"<PREFIX>-123"`).
- `title` — required.
- `description` — optional markdown.
- `assignee` — defaults to `"me"` whenever the request names nobody.
- `state` — optional; defaults to the team's default status (`Triage`/`Backlog`).

Verify the parent exists with `get_issue` first if you're not sure of the identifier. Return the new identifier (`<PREFIX>-###`) and URL to the user.

### Reassign an issue

`save_issue` with `id` and `assignee`. Resolve the target person via `list_users` if the user gave just a first name and you need to disambiguate. For "assign to me", pass `assignee: "me"`.

### Comments

- Read: `list_comments` with `issue` (UUID or `PROJ-123`).
- Post: `save_comment` with `issue` and `body` (markdown). Omit `id` to create.
- Send markdown content with **real newlines**, never literal `\n` escape sequences (per the MCP server's instructions).

### Issue labels

- List existing for a team: `list_issue_labels` with `team`.
- Create new: `create_issue_label` (sparingly — labels are workspace-level signal; ask before creating).

## Conventions

- **Identifiers in output**: when reporting on issues, always show the `<PREFIX>-###` identifier (and URL if available) so the user can click through.
- **Don't bulk-update** without explicit confirmation. If a query returns >5 issues and the user asks to change them, list them first and ask before applying.
- **Don't create issues unprompted** — even when fixing bugs or doing related work, only `save_issue` (create form) when the user explicitly asks.
- **Status transitions**: map the user's intent to the workspace's actual status names (fetch via `list_issue_statuses` if unknown). E.g. "mark done" → the `completed`-type status, "in review" → the relevant `started` status, "block it" → a blocked/needs-info `backlog` status.
- **Branch ↔ issue mapping**: branches are typically named `<prefix>-123-...` (lowercase prefix, dashes). When the user is on such a branch, default Linear queries to that issue.

## When NOT to use this skill

- Non-Linear ticketing systems (Jira, GitHub Issues, etc.) — those have their own tools.
- Reading external Linear docs/help articles — use `search_documentation` for the product help center, but general programming questions don't belong here.
