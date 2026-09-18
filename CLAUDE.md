# skills — repository rules

**This repository is PUBLIC.** It is the source of truth for the skills used by
Claude Code, Codex and OpenCode, and it also carries a Claude Code plugin
(`plugin/`) for the hooks, which the other harnesses cannot install.
Everything committed here is world-readable, forever, including git history.

Therefore every file in `skills/`, `agents/`, `harness/` and `plugin/` must be **generic**: usable by
a stranger who has none of the author's projects, machines, accounts, or tools.
Project-specific knowledge lives in the private namespace repositories, never here.

Layout and install instructions are in `README.md`. Two rules follow from the
layout and are easy to get wrong:

- A skill's frontmatter `name` carries its namespace (`name: sadensmol-router` in
  `skills/router/SKILL.md`), because Codex and OpenCode have one flat namespace.
- `plugin/` holds no `skills/` and no `agents/` directory. Anything there is copied
  into Claude Code's plugin cache at install time, so it goes stale; skills and agents
  are linked from this repository instead, and the plugin carries only hooks.

## 1. Never commit secrets

Not now, not "temporarily", not in an example.

- No tokens, API keys, passwords, cookies, session ids, private keys, `.env`
  contents, or `kubeconfig`/`sops` material.
- Reference secrets **only by env-var name** (`$JIRA_API_TOKEN`, `$SLACK_TOKEN`).
  Show the shape (`ATATT3x…`), never a real value.
- Skills must instruct the **user** to put secrets in `~/.profile` themselves, and
  must never echo, print, log, or write a secret to a file.
- Placeholders only in examples: `you@example.com`, `yourorg.atlassian.net`,
  `<LIST_ID>`, `<paste your token>`.
- A leaked secret is not fixed by a follow-up commit — it stays in history. Rotate
  it, then rewrite history.

## 2. No project-specific knowledge in `skills/`, `agents/` or `plugin/`

A `sadensmol-` skill is the **generic layer**. It must not contain:

- Project, company, product, repo, or service names.
- Workspace names, plugin namespaces of private projects, or branch/issue prefixes
  (`ABC-\d+` as a *pattern* is fine; a real project's prefix is not).
- Tracker sites, team names, status names, CI workflow names, environment names.
- Domain vocabulary that identifies the business (entities, currencies, partners).
- Repo layout facts ("project X is single-repo, project Y is multi-repo").

Write `<project>` / `<project>-task` / `<project>-<project>` instead. If an example
needs a name, invent a neutral one (`acme`, `myproject`, `org/…`).

**Dispatch, don't special-case.** The top-level skill resolves *which* project skill
to call, then forwards the request verbatim. Anything needing repository knowledge is
passed down; anything it handles itself, it handles generically from the cwd alone. If
you want to add an `if <project>` branch to a `sadensmol-` skill, that logic belongs in
that project's plugin.

**Where project detail actually lives**, in order:

1. The project's own plugin (`<project>-<project>`, `<project>-task`, …) — the single
   place a project's facts and paths are written down.
2. The local, unpublished `~/.claude/sadensmol-router-routes.json` — maps cwd
   substrings to project skills, so names stay off GitHub.
3. Naming convention — an agterm workspace named after its plugin resolves directly
   to `<name>-task`, needing no lookup at all.

## 3. No local or machine-specific paths

Never write `/Users/<name>/…`, `/home/<name>/…`, or `~/work/…` — the home directory
layout is as identifying as a username, and `~/work` is a leak just like the absolute
form. Resolve values at runtime, in this order:

1. **Derive** — from the cwd (walk up to the marker directory), `$HOME`,
   `command -v <tool>`, `git rev-parse --show-toplevel`.
2. **Env var**, override first: `TOOL="${TOOL:-$(command -v tool || echo "$HOME/.local/bin/tool")}"`.
3. **Ask the user.** Never guess a path.

Do not invent a new contract for the project plugins to satisfy just to obtain a
value ("every project skill must declare X"). Either derive it generically, or hand
the whole request to the project skill and let it do the work with its own knowledge.
The generic layer routes; it does not interview.

Acceptable, because they are conventions rather than identities: `$HOME/.local/bin`,
`~/.claude`, `$CLAUDE_PLUGIN_ROOT`. Remember shell state does not survive between
`Bash` calls — repeat a resolution snippet in each command instead of relying on an
earlier assignment.

## 4. Before every commit

Run the sweep, and read the hits — do not trust a clean exit code alone:

```bash
grep -rnE '(/Users/|/home/[a-z]|~/[A-Za-z])' skills/ agents/ harness/ plugin/   # local paths
grep -rniE '<your project names here>' skills/ agents/ harness/ plugin/         # project names
grep -rniE '(xoxb-|xoxp-|ghp_|github_pat_|ATATT|AKIA[0-9A-Z]{16}|sk-[A-Za-z0-9]{20,}|BEGIN [A-Z ]*PRIVATE KEY)' skills/ agents/ harness/ plugin/
git log --all -p -G'(xoxb-|ghp_|ATATT|AKIA[0-9A-Z]{16}|BEGIN [A-Z ]*PRIVATE KEY)' --oneline   # history too
```

Then check the things grep cannot: does any example, failure story, or table imply a
real project? A skill that says "this happened — don't repeat" is valuable; write it
without naming who it happened to.

## 5. When a rule gets violated

Fix the **rule**, not just the line. If a project name or path slipped into a
`sadensmol-` skill, remove it *and* strengthen the guidance that let it in — the same
hole otherwise reopens in the next session.

## Conventions

- Do not create git commits — the user commits.
- A skill never names a harness's tool. Write the intent (`fan out`, `run in the
  background`, `ask the user`) and let `harness/<harness>.md` translate it. When a harness
  gains or loses a capability, that file changes and no skill does.
- Agent files need `name` (matching the filename, and kept short — the folder already
  says what they are), `description` and `mode: subagent`.
  Never add `color:` with a named value — OpenCode rejects it and refuses to load any
  config at all.
- Skills are markdown with YAML frontmatter (`name`, `description`); keep the
  description trigger-rich, since it is what routes work to the skill. Quote any
  description containing a colon followed by a space — unquoted, it is invalid
  YAML and tools skip the skill silently.
- Prefer editing an existing skill over adding a near-duplicate one.
