# Set up a workspace by hand

These are the steps of `workspace new`, written for a person. `<name>` is the workspace name
(lowercase, digits, hyphens). `<folder>` is the workspace folder.

## 1. Local folder and registry

1. Create `<folder>` and `<folder>/.workspace/`. Run `chmod 700 <folder>/.workspace`.
2. Run `python3 -I <skill-dir>/scripts/workspace.py init --folder <folder> --name <name>
   --email <email> --user-name "<full name>"`. It writes:
   - `.workspace/settings.json`: `company` and `agterm_workspace` (both `<name>`).
   - `.workspace/workspace.json`: `user.email`, `user.name`, `linear.server`
     (`linear-<name>`), `linear.authStore`, `linear.mcpConfig`, `browser.server`
     (`playwright-<name>`), and `opencode.mcp.linear`: the Linear server for OpenCode, read by
     `harness/opencode/workspace-mcp.py` (register it once with
     `harness/opencode/install.sh --workspace-mcp linear`).
   - `.workspace/private-names.txt` and `.workspace/projects/`.
   - `<folder>/.mcp.json`: the `linear-<name>` server, enabled in
     `<folder>/.claude/settings.local.json`.
3. When `<folder>` is a git repository, `init` adds `.workspace/` to `.git/info/exclude`.
   Never commit `.workspace/`.

## 2. Browser server

Give the workspace its own browser profile, so its logins stay apart:

```bash
claude mcp add --scope user playwright-<name> -- \
  npx @playwright/mcp@latest --user-data-dir "$HOME/.playwright-profiles/<name>"
```

## 3. agterm

Create the agterm workspace `<name>` with a session in `<folder>`:

```bash
agtermctl session new --workspace-name <name> --create-workspace --cwd <folder> --name <name>
```

Open that session. Start the agent there and approve the `linear-<name>` server.

## Create a Slack workspace

1. Open `https://slack.com/get-started#/createnew`.
2. Enter your email. Type the 6-digit code from the email.
3. Click *Create a workspace*. Enter the company name: it becomes the workspace name and its
   URL slug (`https://<slug>.slack.com`). Change the slug later in *Settings & administration*
   if needed.
4. Enter your name. Skip the invites for now.
5. When Slack asks for a first channel, enter any name (you can archive it later). Project
   channels come from `sadensmol-project new`.
6. Pick the free plan when Slack offers a paid trial.
7. Write the slug down: it is `slack.workspace` in `settings.json`.

## Create a Linear workspace

1. Open `https://linear.app/signup` and sign in (Google or email code).
2. Click *Create workspace*. Enter the workspace name and its URL slug
   (`https://linear.app/<slug>`).
3. Linear creates a first team. Rename it to the shared team name, and pick its issue key
   (Settings > Teams > the team). All projects of the workspace share this team.
4. Skip the invites and the imports.
5. Write down the slug (`linear.workspace`) and the team name (`linear.team`).

## Statuses

Team settings > Workflow. Each phase needs a status. Keep the names that exist. Add a missing
one with the type and a name from this table:

| Phase | Type | Name for a new status |
|---|---|---|
| `todo` | Unstarted | Todo |
| `planning` | Started | Planning |
| `implementing` | Started | In Progress |
| `qa` | Started | QA |
| `review` | Started | In Review |
| `pr` | Started | PR |
| `blocked` | Backlog | Blocked |
| `done` | Completed | Done |
| `canceled` | Canceled | Canceled |

Write each name to `linear.statuses.<phase>` in `settings.json`. Pick the default type label
(`Bug`, `Feature` or `Improvement`) as `linear.default_label`.

## Connect Linear and Slack

1. Linear > Settings > Integrations > Slack > *Connect*. Pick the Slack workspace and allow.
2. On the same page, turn *Project channels* off. Each project gets its channel from
   `sadensmol-project new`.
3. Each user: Linear > Settings > Connected accounts > Slack > *Connect*. Without it, `@Linear`
   in Slack refuses the user.

## Check

Run `python3 -I <skill-dir>/scripts/workspace.py status --folder <folder>`. Every line says
`ok`. Then add the first project with `/sadensmol project new <project>`.
