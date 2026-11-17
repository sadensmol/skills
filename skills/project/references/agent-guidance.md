# Linear Agent guidance — template

Linear's Agent reads two guidance fields and cannot read files. `scripts/registry.py
render-guidance` fills this template from the workspace registry and prints both fields.
Paste each fenced block into the field it names. Never edit the fields in Linear only:
change the registry or this template, render again, then paste.

## Field 1 — Settings → AI & agents → Linear Agent → Workspace guidance

Covers Linear Agent chat (⌘J) and `@Linear` in comments.

```markdown
{{workspace}} is one workspace with several projects. {{model}} Current projects: {{projects}}.

Structure
- Never create a team, a project or a Slack channel for a feature, and never turn on per-project Slack channels.
- Every issue belongs to its project (the project's team, and its Linear project when projects share a team); a sub-issue copies its parent's project and feature. If the project is unclear, ask. Issues that autopilot creates get the project of their Slack channel and no feature.
- Initiative = a goal across projects.

Issues
- Title: what changes, in plain words. Research issues start with "r&d | " and end with the question they answer.
- Labels: exactly one type label — Bug, Feature or Improvement. Do not create labels.
- Assignee: the requester unless someone else is named.
- No cycles, no estimates.

Statuses: backlog → {{todo}} → {{implementing}} → {{done}}; {{canceled}} or the duplicate status when the work will not happen. Autopilot runs only: {{autopilot_statuses}}; autopilot runs move their issues through them, and other work never uses them. Outside autopilot runs, an issue with an open PR stays {{implementing}} with a comment linking the PR. Never mark cancelled work {{done}}.
```

## Field 2 — Settings → Integrations → Slack → Linear Agent → Workspace guidance

Covers `@Linear` in Slack.

```markdown
Channel routing (one line per project channel):
{{routing}}

Always use the team and project of the channel. A feature has no channel: its discussion is the thread of its feature-request message in the project channel, so post feature updates in that thread. Create new issues in {{todo}}, assigned to the person who asked, in the channel's project, in the feature that matches the discussion; if none matches, leave the feature empty.

Issues: one type label (Bug, Feature or Improvement); title states what changes in plain words; research questions start with "r&d | ". No cycles, no estimates. Autopilot runs only: {{autopilot_statuses}} — autopilot runs move their issues through them, and other work never uses them.
```
