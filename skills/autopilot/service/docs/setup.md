# Autopilot setup guide

This guide lists every manual step to run autopilot for one workspace. Never paste a secret value in chat, a log or a repo. This guide names keys only.

## 1. Overview

- **Autopilot** is a Go service. It listens to Slack, turns a request into a Linear issue, and runs the plan, QA, implementation, gate and review phases in agterm sessions. The pull request is the only human gate.
- **Settings and secrets** are local. They live in `<workspace folder>/.workspace/autopilot/` and never enter a repo.
- **The workspace registry** (workspace name, Slack and Linear workspaces, Linear statuses, projects, repos) is local too. It lives in `<workspace folder>/.workspace/settings.json` and `<workspace folder>/.workspace/projects/<project>.json`. The service finds it from the settings folder. Autopilot knows no project by itself: it serves the projects of the registry.

## 2. The local data rule

| What | Where |
|---|---|
| Tokens, API keys | `.workspace/autopilot/secrets.env` (mode 600) |
| Owner id, folders, timings | `.workspace/autopilot/config.yaml` (mode 600 advised) |
| Workspace, statuses, default label | `.workspace/settings.json` |
| One project: channel, Linear team and project, repos | `.workspace/projects/<project>.json` |

The service refuses to start when the folder `.workspace/autopilot` has a mode other than 700, or when `secrets.env` has a mode other than 600. The error names the folder or file.

## 3. Create the local folder

1. `mkdir -p <workspace folder>/.workspace/autopilot && chmod 700 <workspace folder>/.workspace <workspace folder>/.workspace/autopilot`.
2. `cp deploy/examples/config.yaml <workspace folder>/.workspace/autopilot/config.yaml`, then `chmod 600` on it.
3. Edit `config.yaml`. Keys:
   - `owner.slack_user_id` (required): your Slack member ID.
   - `service.source_dir` (required): the folder of this repo checkout.
   - `repo_paths` (optional): checkout folder per repo. Default: `<workspace folder>/<repo>`.
   - Timings, all whole numbers above zero. Defaults: `heartbeat_minutes` 30, `loop_cap` 3, `wave_timeout_minutes` 45, `pr_poll_minutes` 5, `gate_timeout_minutes` 45.
   Unknown keys are an error.

## 4. Slack app

1. Open https://api.slack.com/apps and choose **Create New App**. Create it from an app manifest. Pick your workspace. Paste `deploy/slack-manifest.yaml` on the **YAML** tab. Help: https://docs.slack.dev/app-manifests/configuring-apps-with-app-manifests
2. Check that **Socket Mode** is on. The manifest turns it on. The manifest also lists the bot scopes, among them `reactions:write` for the reactions on the owner's requests. When a new version of the manifest adds a scope, add it under **OAuth & Permissions → Bot Token Scopes**, then reinstall the app. The bot token stays the same.
3. Under **Basic Information → App-Level Tokens**, generate a token with the scope `connections:write`. It is `SLACK_APP_TOKEN`.
4. Under **Install App**, install the app. Copy the Bot User OAuth Token. It is `SLACK_BOT_TOKEN`.
5. In the Slack channel of each project, type `/invite @autopilot`. The service does not start when a project channel is missing or the bot is not a member of it.
6. Copy your member ID from your Slack profile. It starts with `U`. Put it in `owner.slack_user_id`.

## 5. Linear

1. Open Linear → Settings → Account → Security & access. Create a personal API key. It is `LINEAR_API_KEY`.
2. Find the teams that autopilot uses: the `linear_team` of each project file, and `linear.team` in `settings.json` when it is set. Do the next three steps once for each of these teams.
3. Open Team settings → Workflow. Add the statuses that `linear.statuses` names and the team does not have yet, all of type started (for example `Planning`, `In QA`, `In Review`, `PR Open`, `Blocked`). Do not rename or remove existing ones.
4. Make sure that the label `linear.default_label` exists in the team, or in the whole Linear workspace.
5. When a project file that uses this team names a `linear_project`, make sure that this Linear project exists in the team. New issues of the project go to it.

## 6. secrets.env

1. `touch .workspace/autopilot/secrets.env && chmod 600 .workspace/autopilot/secrets.env`.
2. Add the lines `SLACK_APP_TOKEN=...`, `SLACK_BOT_TOKEN=...` and `LINEAR_API_KEY=...`. These three keys are required.
3. Check the key names only: `cut -d= -f1 .workspace/autopilot/secrets.env | sort`. Expect `LINEAR_API_KEY`, `SLACK_APP_TOKEN` and `SLACK_BOT_TOKEN`.

## 7. Workspace registry

Copy `deploy/examples/settings.json` to `<workspace folder>/.workspace/settings.json`. Copy `deploy/examples/projects/myproduct.json` to `<workspace folder>/.workspace/projects/<project>.json`, one file for each project. Replace the example names, and run `chmod 600` on each file. Rules:

- The project skill's `registry.py check` checks the keys below. Run it after each change. The service asks for the same keys, by their exact names (`Slack_Channel` is not `slack_channel`), and also needs these values to be strings: each key named below and each status name. A number in their place passes `registry.py check` but stops the service; `check-config` names the file and the key.
- `settings.json` needs these keys: `company`, `agterm_workspace`, `slack.workspace`, `linear.workspace`, `linear.default_label` and all nine `linear.statuses`. `linear.team` (a shared team) is optional. Other keys are allowed. A `products` key is an error: each project has its own file.
- The nine status keys are phases: `todo`, `planning`, `implementing`, `qa`, `review`, `pr`, `blocked`, `done`, `canceled`. Each value is the status name in Linear.
- A project file needs `slack_channel`, `linear_team` and at least one repo. When two or more projects use the same `linear_team`, each of them needs `linear_project`: the Linear project of that team that its new issues go to. A project without `linear_project` gets new issues with no Linear project.
- Each repo needs `name`, `default_branch` and `deploy`. `repos[].name` must equal the GitHub repo name, so a pull request URL maps to the repo. A repo can also have `profile_overrides` (section 8). Each override value is a string, a number or a boolean; the service reads it as text.
- `deploy` says how a merge of the repo goes live. A run is done only when its merge is live. Its keys:
  - `job`: the CI job on the merge commit that publishes the artifact. A failed or skipped job makes the run stuck.
  - `check`: a shell command that exits 0 when the merge commit is live, for example `ssh <host> '<cmd>' | grep -q {short_sha}`. The service replaces `{sha}` and `{short_sha}` (7 characters), runs it with `sh -c` after the job passed, and tries again each poll. One try may take 60 seconds.
  - `timeout_minutes`: a whole number above zero, default 60. The time after the merge that the service waits for the job and the check. After it, the run is stuck.
  - `manual`: the repo has no automatic deploy. The text says how a person deploys it. The run is done at the merge, and the done line shows the text.

  `manual` stands alone. Without it, `deploy` needs `job`, `check` or both. A missing or wrong `deploy` stops `check-config`; the error names the repo, the key and the file.
- Two project files with the same `slack_channel` are an error. The error names both files.
- A project file can hold other keys, for example tokens of that project. The service never reads them and never prints them.
- There must be at least one project file.

To add a project, add its file. Then restart the service.

## 8. Test profiles and overrides

The gate picks the test profile by marker file in the repo: `go.mod` (Go), `pyproject.toml`, `setup.py` or `requirements*.txt` (Python), `package.json` (Node). A repo with none of them is refused at intake. The base runtime, and `uvx`, must be on PATH.

A repo in a project file can set `profile_overrides` (values are strings, numbers or booleans):

| Key | Meaning | Default |
|---|---|---|
| `test` | the test command | the profile's own |
| `mutation` | the mutation command | the profile's own |
| `diff_coverage_min` | percent, 0 to 100 | 80 |
| `mutation_min` | percent, 0 to 100 | 60 |
| `assertion_pattern` | regular expression for an assertion failure | none |
| `mutation_timeout_minutes` | whole minutes above 0 | 15 |

An unknown key is an error.

## 9. Check and run

Build the program with `make build` (output in `bin/`). Then:

- `autopilot check-config --settings <workspace folder>/.workspace/autopilot` loads the settings and the workspace registry and prints `OK`.
- `autopilot check-linear --settings ...` checks the API key and its workspace, every team used (the team of each project, and `linear.team` when it is set), every status and the default label in each of these teams, and the Linear project of each project that names one. It prints `OK`.
- `autopilot check-products --settings ...` prints one line for each project: its Slack channel exists and has the bot as a member, its Linear project exists (`project: none` when the project names none), and each repo folder is a git checkout. Each part is `ok`, `missing (<reason>)` or `error (<text>)`. It exits with code 1 when a part is not `ok`. It prints no secret and no local path.
- `autopilot serve --settings ... [--agterm-socket <path>]` runs the service. Without the flag it uses `AGTERM_SOCKET`. Another serve on the same folder exits with code 4.
- `autopilot supervise --settings ... --agterm-socket <path>` keeps one serve running and restarts it when it dies.

When a request comes in, the service checks the Linear team, the Linear project (when the project names one) and the repo folders of the project first. When a part is missing, it creates no issue and posts one reply in the request thread that names the missing part.

Then, in Claude Code, run `/autopilot` for the run skill.

## 10. Troubleshooting

- **`folder ... has mode ..., want 700`.** Run `chmod 700` on the folder.
- **`file ... has mode ..., want 600`.** Run `chmod 600 secrets.env`.
- **`key ... is missing in ...`.** Add the named key to the named file.
- **`linear.statuses.<x> is not a phase`.** Use only the nine phase keys.
- **`check-linear` reports a missing status.** Add it in the team workflow (section 5).
- **`check-linear` reports a missing Linear project.** Create the project in the named team, or fix `linear_project` in the project file.
- **`slack_channel ... is in both ... and ...`.** Two project files name the same channel. Remove or change one of them.
- **`check-products` reports `the bot is not a member`.** Type `/invite @autopilot` in that channel.
- **`check-products` reports `is not a git checkout`.** Clone the repo to that folder, or set its folder in `repo_paths`.
- **The agterm socket is missing.** Pass `--agterm-socket` or set `AGTERM_SOCKET`.
