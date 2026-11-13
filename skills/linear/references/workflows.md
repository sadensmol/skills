# Linear — list, subtask, reassign and label workflows

Read this when you list or search issues, add a subtask to an existing issue, reassign an issue, or list or create issue labels.

### Search / list issues

Use `list_issues`. Useful filters (combine freely):

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

### Issue labels

- List existing for a team: `list_issue_labels` with `team`.
- Create new: `create_issue_label` (sparingly — labels are workspace-level signal; ask before creating).
