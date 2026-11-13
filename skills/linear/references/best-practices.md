# Linear best practices

Use when setting up or restructuring a workspace, or when choosing where new work goes.
Facts come from Linear's help center (`search_documentation`); the source page is in brackets.
Lines marked **Convention** are our own choice, not Linear's.

## Hierarchy

| Level | Holds | Use for |
|---|---|---|
| Workspace | everything | One per company [import-issues] |
| Team | issues, workflow, cycles, labels, triage | A group that works together, or an area of work ("mobile app") [teams] |
| Initiative | projects | A goal across projects or teams; why the work matters [initiatives] |
| Project | issues + milestones | A shared outcome with an end: feature launch, migration, redesign [conceptual-model] |
| Milestone | issues of one project | A stage of completion inside a project [conceptual-model] |
| Parent issue | sub-issues | Too large for one issue, too small for a project; can convert to a project [parent-and-sub-issues] |

- A project is finished work, not a product. Do not model a product as a project.
- Every issue belongs to exactly one team. A project can span many teams [teams].
- One team is the simplest setup for small teams. Start with one or two; adding later is easy [teams].
- Team limits: Free 2, Basic 5, Business/Enterprise unlimited [teams]. Check the plan before adding a team.
- Sub-teams inherit workflow, cycles, labels and templates from the parent [conceptual-model, labels, issue-templates].

## Several products in one workspace

Linear does not say whether a product is a team or a project. **Convention:**

- **Product = team** when it has its own repo, branch↔issue mapping, or cadence. The team key
  becomes the issue prefix (`APP-12`), so branches map per repo.
- **Feature = project** inside that team, with milestones and a target date.
- **Goal across products = initiative**.
- **Product = Slack channel** (`#<team>`) in ONE Slack workspace that mirrors the Linear
  workspace — see Slack below. Never a Slack workspace per product.
- Keep a catch-all team for small one-off work.
- On a plan with a team limit, use one team and a workspace label group `Product/<name>` instead.

## Teams

- Set the key in Team settings > General > Identifier [teams]. **Convention:** 2–5 uppercase
  letters, derived from the repo name, fixed once issues exist.
- Moving an issue to another team gives it a new ID; old IDs still resolve. Team labels, cycle
  and non-shared projects drop off [creating-issues]. Pick the team at creation.
- Never share an issue prefix with another tracker (Jira); GitHub links the PR to the wrong issue [jira].
- Copy settings from an existing team when creating a new one [teams].
- Retire a team (read-only, history kept) instead of deleting it [teams].

## Workflow and settings

- Default statuses: Backlog > Todo > In Progress > Done > Canceled, plus system Duplicate [configuring-workflows].
- Add `In Review` (started category) when PRs are reviewed. Linear's own workflow adds
  In Review and Ready to Merge [configuring-workflows].
- Enable triage when issues arrive from integrations or non-members [triage].
- Cycles are opt-in per team: start date + N-week duration [teams]. Moving an issue into a cycle
  sets it to Todo [default-team-pages]. Disable cycles on a team that does not plan in sprints.
- Enable estimates in Team settings > General. T-shirt map: XS 1, S 2, M 3, L 5, XL 8 [teams, creating-issues].
- Turn on auto-close / auto-archive to keep stale work out of the backlog [configuring-workflows].

## Labels

- Labels used by every team (`Bug`, `Feature`, `Improvement`) live at workspace level [labels].
- Use label groups for one-of-a-kind dimensions; typing `Type/Bug` creates group + label.
  One label per group per issue; max 250 per group [labels].
- Add label descriptions; Triage Intelligence reads them [labels].
- Reserved names: assignee, cycle, effort, estimate, hours, priority, project, state, status [labels].
- Group projects by goal with initiatives; by finer dimension with project labels [project-labels].

## Naming

Linear gives no naming rules for teams or projects. **Convention:**

- Team: product name (`acme-app`), key from it (`ACME`).
- Project: the outcome, not the activity — `Bulk export`, not `Work on export`.
- Issue title: imperative, one outcome — `Add retry to failed uploads`.
- Write issues an agent can execute: outcome, scope, constraints, what must not change [coding-sessions].

## Git

- Name the branch with the issue ID (`acme-12-…`); the PR links to the issue and the merge can close it [jira].
- Git automations live per team in Team settings > Workflows & automations; copy them to other teams [teams].
- Enable "On git branch copy, move issues to a started status" [code-and-reviews].
- A PR is "closing" or "contributing"; only closing PRs drive status changes [releases].

## Agents

- Delegate an issue to an agent; a human stays the assignee and is responsible. The agent needs
  access to the issue's team [assigning-issues].
- Trigger an agent by delegating or @mentioning it [agents-in-linear].
- Write agent guidance in Settings > Agents > Additional guidance (workspace) and in team settings
  (team wins). Put repo, commit/PR issue references, review process and issue-title format there [agents-in-linear, linear-agent].
- @mention a team doc in guidance to give the agent more context [documents].
- Grow agent skills: chat → personal skill → share to team when stable [linear-agent].
- Loops automate agent work; prove the result in chat first and grant minimal access [loops].
- MCP: `https://mcp.linear.app/mcp` (read-only `/mcp/readonly`). Each workspace needs its own auth
  context [mcp]. With several workspaces, give each one its own MCP server and auth dir, and call
  `whoami` before any write to confirm the workspace.

## Slack

- A Linear workspace has ONE Slack integration and ONE `@Linear` bot. There is no per-team bot.
  Connecting more than one Slack workspace needs Enterprise [slack].
- **Convention:** one Slack workspace per Linear workspace, one channel per team. A Slack
  workspace per product hits the one-integration limit as soon as a second product appears.
- Get per-team behaviour from the single bot:
  - Team notifications: team settings > Notifications > Slack → the team's channel. Posts issue
    creation, comments, status changes and project updates [slack].
  - Slack agent guidance (Settings > Integrations > Slack): map each channel to its team, e.g.
    `#acme → team ACME`. The bot also infers the project from a channel that receives that
    project's updates [slack].
  - A fixed team per channel (Asks per-channel configuration) is Enterprise only [linear-asks-slack].
- Setup order: an admin enables the workspace integration (Settings > Integrations > Slack), THEN
  every user connects their personal Slack account (Settings > Connected accounts > Slack).
  Only users with Linear accounts can use the bot [slack].
- Bot replies "you can't message this instance of @Linear as it belongs to the <x> workspace" →
  that Slack user has no connected personal Slack account. Fix the connection, not the bot.
- `@Linear` mention gets no reply at all, and linking the personal account fails with "Connect a
  Slack workspace to Linear … before linking your personal account" → the channel's Slack
  workspace is not the one connected to Linear. The Linear app can still be installed and added
  to a channel there. Compare Settings > Integrations > Slack > Connected workspaces with the
  channel's Slack workspace, then switch (below).
- AI credits (even a $0 balance) cover only Coding sessions and Loops, not the Linear Agent.
  They do not explain a silent bot.
- Public channel: mention `@Linear` directly. Private channel: `/invite @Linear` first [slack].
- Switching Slack workspaces: disconnect the old one in Slack settings, connect the new one, then
  reconnect personal accounts and re-point every team's notification channel.
