---
name: sadensmol-workspace
description: "Use when the user wants to create, add or set up a new workspace (a company context): 'workspace new <name>', 'new workspace', '/sadensmol workspace new <name>', set up agterm, Slack and Linear for a new company, or connect an existing Slack or Linear workspace to a new local workspace folder. Also use to resume an unfinished workspace setup. Not for a new project inside an existing workspace (that is sadensmol-project)."
---

# Workspace

Creates a workspace: one company context with its own folder and `.workspace/` registry, one
agterm workspace, one Linear workspace and one Slack workspace (`sadensmol` → *Terms*).
Projects inside it are `sadensmol-project new`, not this skill.

## Subcommands

| Subcommand | What it does |
|---|---|
| `new <name>` | Set up a workspace, or resume an unfinished setup: Phases 1–3 below |

No subcommand, or an unknown one: show this table and stop. No name: ask the user. Never
invent one. The name is lowercase letters, digits and hyphens.

`<skill-dir>` below is this skill's directory. Run the script as
`python3 -I <skill-dir>/scripts/workspace.py`.

The manual steps for a human are in `references/manual.md`. Every *create* step below
offers them first, then offers to do the same steps in the browser.

## Ask with a selection (MUST FOLLOW)

Every choice in this skill is a selection dialog (`ask the user`, with options), never a
free-text question. Free text only for a value: an email, a URL, a slug, a name.

## Resume first

Each run starts here. Find the folder: `<folder>` is the folder of the current session when
it holds `.workspace/settings.json` whose `company` is `<name>`. Else ask the user for the
folder. First option: `<name>` next to the nearest folder above the current directory that
holds `.workspace/` (a sibling of an existing workspace). Never guess another path.

Run `workspace.py status --folder <folder>`. Skip each phase whose items all say `ok`.
Start at the first `missing` item.

# Phase 1 — local setup (any session)

1. Ask the user for the email and the full name to store in `workspace.json` (`user.email`,
   `user.name`). Never print them back.
2. `workspace.py init --folder <folder> --name <name> --email <email> --user-name "<full name>"`.
   It creates `.workspace/` (700, files 600) with `settings.json`, `workspace.json`,
   `private-names.txt` and `projects/`. It registers the Linear MCP server `linear-<name>`
   in `<folder>/.mcp.json` (auth store `$HOME/.mcp-auth/<name>`) and enables it for the
   folder. It keeps existing files and keys.
3. The browser server `playwright-<name>`: ask with a selection.
   - **Own browser profile (recommended):** logins of this workspace stay apart from the
     others. Add it at user scope:
     `claude mcp add --scope user playwright-<name> -- npx @playwright/mcp@latest --user-data-dir "$HOME/.playwright-profiles/<name>"`.
     In another harness, add the same server to its MCP config.
   - **An existing server:** list the user-scope browser servers, let the user pick one, then
     `workspace.py set --folder <folder> --file workspace browser.server <server>`.
4. Append `<name>` to the `private-names.txt` that the base skills repository's pre-commit
   sweep reads (its `CLAUDE.md` → *Before every commit*), when that file exists and lacks it.
5. agterm (when `AGTERM_ENABLED=1`, load `sadensmol-agterm`):
   `agtermctl session new --workspace-name <name> --create-workspace --cwd <folder> --name <name> --socket "$AGTERM_SOCKET"`.
   Outside agterm, tell the user to open a new session in `<folder>`.
6. Stop here. New MCP servers load only in a new session. Tell the user, in one line: in the
   new session, approve the `linear-<name>` server, then run `/sadensmol workspace new <name>`
   again. Phase 2 needs both servers.

# Phase 2 — Slack and Linear (a session in `<folder>`)

Check first: the tools of `linear.server` and `browser.server` (from `workspace.json`) are in
this session. One is missing: stop and say which. Never borrow another server. The browser
rules are `sadensmol-router` → `references/browser-gate.md`.

Do Slack, then Linear. For each, ask with a selection: **Existing, connect** or **Create new**.

## Slack

**Existing:** ask for the workspace URL (`https://<slug>.slack.com`). Open it with the
browser server. The user logs in when asked. Check that the page shows the workspace, then
`workspace.py set --folder <folder> slack.workspace <slug>`.

**Create new:** show `references/manual.md` → *Create a Slack workspace* to the user as a page.
Then ask with a selection: **I do it myself** or **Do it in the browser**.
- Myself: wait until the user says it is done, then continue as **Existing**.
- Browser: follow the same steps. Stop at each email code and each payment page, and ask the
  user to finish that step in the browser. Then continue as **Existing**.

## Linear

**Existing:** call `whoami` on `linear.server`. A sign-in window opens on the first call;
the user signs in to the right workspace. Show the workspace name and URL slug from
`whoami`, and ask with a selection to confirm it. Wrong workspace: tell the user to delete
`$HOME/.mcp-auth/<name>`, restart the session and sign in again. Then:

1. `workspace.py set --folder <folder> linear.workspace <slug>`.
2. The shared team: list the teams and ask with a selection (up to 4 per dialog, the rest by
   name). One team only: propose it. `workspace.py set … linear.team "<team name>"`.

**Create new:** show `references/manual.md` → *Create a Linear workspace*. Then ask
**I do it myself** or **Do it in the browser**, as for Slack. Then continue as **Existing**.

## Linear setup (both paths)

1. **Statuses.** List the team's statuses (`list_issue_statuses`). Build a table: each phase
   of `sadensmol-linear` → *Statuses* to one status, matched by type and name. Show the table
   and ask with a selection: **Use this mapping**, **Change some**, **Add the missing
   statuses**. A missing status gets the name of `references/manual.md` → *Statuses*, added
   in Team settings > Workflow with the browser or by the user. Then write each phase:
   `workspace.py set … linear.statuses.<phase> "<status name>"`.
2. **Default label.** List the team's labels. Ask with a selection among `Bug`, `Feature`,
   `Improvement` that exist. None exists: ask before you create them.
   `workspace.py set … linear.default_label "<label>"`.
3. **Slack integration.** Settings > Integrations > Slack: connected to the Slack workspace
   above. *Project channels* off. Do it in the browser after a yes, or the user does it from
   `references/manual.md` → *Connect Linear and Slack*. Check: the integration page shows the
   Slack workspace.

# Phase 3 — finish

1. `workspace.py status --folder <folder>`: every item says `ok`. A `missing` item: go back to
   its step.
2. Answer in chat, short: the folder, the agterm workspace, the Slack and Linear workspaces,
   the team, and anything the user must still do (each user links their Slack account in
   Linear > Settings > Connected accounts).
3. The next step is the first project: `/sadensmol project new <project>`. `registry.py check`
   passes only after it, because it needs a project file.

## Common mistakes

| Mistake | Fix |
|---|---|
| Phase 2 run in the session that ran Phase 1 | New MCP servers are not loaded there. Start the new session. |
| The claude.ai Linear connector used | Use only `linear.server` from `workspace.json`. It is another account. |
| A secret or the email printed | Print key names and `ok` / `missing` only. |
| A Slack channel or Linear project made for the workspace | Channels and Linear projects are per project: `sadensmol-project`. |
| A choice asked as free text | Use a selection. Free text is only for a value. |
