---
name: sadensmol-project
description: "Use when the user wants to start, add, create or set up a new project in a workspace: 'project new <name>', 'new project', 'start a project', 'add a project', '/sadensmol project new <name>'. Covers the checks of the workspace settings, the Slack channel of the project, the Linear project or team (whatever the workspace structure maps a project to) and the explanation of how Linear and Slack work for the new project. Not for a new feature, task or Linear issue inside an existing project."
---

# Project

Project management for a workspace. A project is a project inside the workspace: a Linear
project or team, a Slack channel and usually a repo. The workspace structure says which.

## Subcommands

| Subcommand | What it does |
|---|---|
| `new <name>` | Start a new project: Steps 1–7 below |

No subcommand, or an unknown one: show this table and stop.

`new` takes the project name. No name: ask the user for it. Do not invent one.

## Registry

Every workspace keeps its names in a local registry in `.workspace/` of the workspace folder.
It is local only and never committed (`.workspace/` is `chmod 700`, each file `chmod 600`).
`sadensmol-workspace new` sets it up once, when the workspace is created; `new` below adds one
project file.

```
.workspace/
├── settings.json             # the workspace
└── projects/
    ├── <project1>.json       # one file per project
    └── <project2>.json
```

| File | Keys |
|---|---|
| `settings.json` | `company` (the workspace name), `agterm_workspace`, `slack.workspace`, `linear.workspace`, `linear.team` (the shared team), `linear.statuses.<phase>`, `linear.default_label` |
| `projects/<project>.json` | `slack_channel`, `linear_team`, `linear_project` (required when projects share a team), `repos[]` (`name`, `default_branch`, `deploy`: see *Deploy of a repo*), and the project's own settings, tokens and secrets |

- `scripts/registry.py check` validates both. `registry.py products` / `repos` list the
  projects. `registry.py get <key>` reads `settings.json` only.
- Never print a project file whole, and never print a secret key's value.
- A file or key is missing: stop, name the key and the file. Never guess a value.

### Deploy of a repo

Every repo needs `deploy`. It says how a merge of the repo goes live. Autopilot ends a run
only when the merge is live.

| Key | Meaning |
|---|---|
| `job` | The CI job on the merge commit that publishes the artifact. |
| `check` | A shell command that exits 0 when the merge commit is live. `{sha}` and `{short_sha}` (7 characters) are replaced first. Example: `ssh <host> '<cmd>' \| grep -q {short_sha}`, or `curl -fsS https://<host>/version \| grep -q {short_sha}`. |
| `timeout_minutes` | Optional, default 60. How long after the merge the job and the check may take. |
| `manual` | The repo has no automatic deploy. The text says how a person deploys it. |

`manual` stands alone. Without it, `deploy` needs `job`, `check` or both. Example:
`"deploy": {"job": "deploy", "check": "curl -fsS https://<host>/version | grep -q {short_sha}"}`.

# `new` — start a new project

## Step 1 — resolve the workspace structure

1. The agterm workspace name gives `<workspace>`.
2. Workspace `(unknown)` or no agterm: use the local routes file, as in `sadensmol-task` →
   Step 2.
3. No match: stop. Ask which workspace the project belongs to.

When a `<workspace>-<workspace>` skill exists (workspace `acme` → `acme-acme`), load it and read
its *Workspace structure* and *Add a new project* sections. Else the structure is the
defaults: the registry (`.workspace/settings.json` and `.workspace/projects/<project>.json`), read with `scripts/registry.py`,
`sadensmol-linear` → *Defaults* and `sadensmol-slack` → *Defaults*. These are the source for
every name, key and channel below. When a `<workspace>-project` skill exists and it is not this
skill, load it too: its steps replace Steps 4–7 of this skill.

## Step 2 — check the workspace settings

Walk up from the cwd to the first folder that holds `.workspace/`. Read
`.workspace/workspace.json`. Print key names and booleans only, never values.

| Key | Check |
|---|---|
| `linear.server` | its tools are in this session |
| `browser.server` | its tools are in this session |

- A key or the file is missing: stop. Name the key and the file path.
- A server is missing from the session: stop and say so. Never borrow another server.
- Call Linear `whoami`. The workspace must be the one the workspace skill names. Else stop.

## Step 3 — derive the names and check for collisions

The workspace structure says what a project is in Linear: its own **team**, or a **Linear
project in a shared team** (`linear_team` and `linear_project` of the registry's project files). Take the
Linear names (team name and key, or the shared team and the Linear project name), the channel
name and the repo name from it. Then:

- Linear, project = team: list the teams. A team with the same name or key exists: stop and
  say so.
- Linear, project = Linear project: list the Linear projects of the shared team. A Linear
  project with the same name exists: stop and say so.
- Slack: open the Slack workspace in the browser and search the channel name. It exists:
  stop and say so.
- GitHub: the repository is `github.com/<workspace>/<project>`, unless the workspace skill names
  another owner. Load `sadensmol-github`. Check the repository with `gh repo view
  <workspace>/<project>`, and the local clone `<workspace folder>/<project>` (the workspace
  folder holds `.workspace/`).
  - It exists: the project is for an existing repository. Use it, and say so in the plan.
  - It does not exist: ask the user. Create it (private or public), use another existing
    repository (the user gives `owner/name`), or no repository now.

Show the user one short plan: the Linear names (team and key, or the Linear project in the
shared team), the channel, the workspace settings it copies, and the repository (existing, to
create, or none). Ask for a yes before the
first write. The writes are visible to the whole workspace.

## Step 4 — Slack: create the project channel

Use the browser server from Step 2 only. In the workspace's Slack workspace:

1. Create the channel `#<project>`. Public, unless the workspace skill or the user asks for
   private.
2. Post `/invite @Linear` in the channel. The bot must be a member for linkbacks and routing.
3. When the workspace runs autopilot (`.workspace/autopilot/` exists), also post
   `/invite @autopilot`. The service does not start when a project channel lacks its bot.
4. Check: the channel shows in the sidebar, and each invited bot is in its member list.

Create the channels that the workspace structure gives a project. Create none for a feature
unless the structure says so.

## Step 5 — Linear: create and set up the project

Follow the variant the workspace structure names.

### Project = Linear project in a shared team

1. Create the Linear project with the Linear MCP (`save_project`): name = project name, team =
   the shared team, status started, lead `"me"`. The summary names the repo and the channel.
2. Project page > bell (*Setup project notifications*) > Slack: send the notifications to
   `#<project>`. Use the browser server from Step 2. The shared team itself keeps no team
   Slack channel, or it would get every project's issues.
3. Settings > Integrations > Slack > *Project channels*: it must be off, so Linear creates no
   extra channel for the Linear project.
4. Check with the Linear MCP: the Linear project exists in the shared team.
5. Check the statuses of the shared team (list issue statuses): every name in
   `linear.statuses` of `settings.json` exists. A gap: add it in Team settings > Workflow
   with the browser, or name it to the user. Autopilot does not start with a missing status.

### Project = team

Linear MCP cannot create a team. Use the browser server from Step 2.

1. Settings > Teams > create team. Name = project name, key = the key from Step 3.
   When the form offers to copy settings from a team, pick the reference team the workspace
   skill names.
2. Team settings > Notifications > Slack: send the notifications to `#<project>`.
3. Settings > Integrations > Slack > *Project channels*: set it as the workspace structure says.
   When features get no channel, it must be off. Else Linear creates a `p-<project>` channel
   for each new Linear project.
4. Check with the Linear MCP: the team exists with the key. Its statuses (list issue statuses)
   match the reference team. A gap: fix it in the browser, or name it to the user.

Then find out how each repo deploys (MUST FOLLOW). Read the repo: its CI workflows (for
example `.github/workflows/`), its `Makefile`, its compose files and deploy scripts, and its
`README`. Use the local clone, or read the files on GitHub when there is no clone yet. Find
the job that publishes the merge commit, and a command that proves the commit is live (a
version endpoint, an image tag, a file on the host). Write them as `job` and `check`
(*Deploy of a repo*). Write `manual` only when nothing automatic exists, for example a new,
empty repo; its text says how a person deploys. You cannot tell: ask the user. Never guess.

Then register the project. A workspace skill's *Add a new project* names its own places. The
default is one registry entry, `registry.py add-product`, with one `--deploy
<repo>=<deploy JSON>` for each `--repo`. The task scripts and the autopilot
service read the registry, so no skill or script changes. Add a line to the router's local
routes file only when the new repo needs its own route.

Then render the Linear Agent guidance (`registry.py render-guidance`) and tell the user which
Linear field to paste each block into.

Then, when the workspace runs autopilot (`.workspace/autopilot/` exists), check that the
service accepts the new project. `<settings>` is `<workspace folder>/.workspace/autopilot`:

1. `autopilot check-config --settings "<settings>"` → `OK`.
2. `autopilot check-linear --settings "<settings>"` → the team, the Linear project and the
   statuses exist.
3. `autopilot check-products --settings "<settings>"` → each channel exists and has the bot.

A check fails: fix the cause it names, or list it as an open item in Step 7. The service
reads the registry only at start. When it runs (`autopilot ping` → `pong`), tell the user
to type `/autopilot restart` in the `auto main` session. Control-room verbs run only there.

## Step 6 — GitHub: the project repository

Do what the user chose in Step 3:

- Existing repository: when the local clone is missing, clone it to
  `<workspace folder>/<project>`.
- Create: `gh repo create <workspace>/<project>` with the visibility the user chose. Then clone
  it to `<workspace folder>/<project>`.
- No repository: skip. List it as an open item in Step 7.

Check: `gh repo view` finds the repository, and the `origin` of the local clone points to it.

## Step 7 — explain how the new project works

End with a short answer in chat. Build it from the workspace structure, with the real names:

- What the project is in Linear, in Slack and in git (for example: a Linear project in team
  `<TEAM>`, a channel, a repo).
- What a feature is in Linear (for example: a milestone inside the Linear project), and
  where issues go.
- Where a feature is discussed in Slack (for example: the thread under its feature-request
  message in the project channel, with no channel of its own).
- The issue key format `<KEY>-<n>`, and the branch rule of the Linear skill (when projects
  share a team, the key does not name the project: the issue's Linear project does).
- Every user links their personal Slack account in Linear, or `@Linear` refuses them.

List what you created, with links, and any step that failed or that the user must do.

## Browser notes

- A click that times out while "waiting for element to be stable": the tab is in the
  background. Bring the tab to the front, then retry.
- Linear settings toggles are hidden checkboxes. When a click on the toggle fails, click the
  input from a page script. Reload the page and read the state again to check it was saved.
- Never take a token, cookie or session value from a page.

## Common mistakes

| Mistake | Fix |
|---|---|
| A Slack channel per feature | Follow the workspace structure. It usually has one channel per project. |
| Linear *Project channels* left on | Check it in Step 5. It creates a `p-` channel per Linear project. |
| A team without the copied statuses | Compare the statuses with the reference team in Step 5. |
| The guidance edited in Linear only | Change the registry or `references/agent-guidance.md`, render, then paste. |
| The claude.ai Linear connector used | Use only `linear.server` from `.workspace/workspace.json`. |
