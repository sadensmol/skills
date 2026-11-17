# The default workspace

The default task flow, for a workspace with no `<workspace>-task` skill: what `## Subcommands` in `SKILL.md` and the hook files in
`references/defaults/` build on. The workspace folder is the folder that holds `.workspace/`
(the scripts find it from the cwd, or take `SADENSMOL_DIR`).

## Projects

The projects and their repos are in the workspace registry, one `.workspace/projects/<project>.json` per project
(`sadensmol-project` → *Registry*). Read it with
`python3 -I <sadensmol-project-skill-dir>/scripts/registry.py products`; never copy it into a
skill. The scripts read it themselves.

A task belongs to exactly ONE project: the Linear project of its issue (when projects share
a team, the key does not name the project). `new` reads the Linear project with `get_issue` and
passes it as `--product`. A branch whose key prefix is a repo name still resolves from its
key. A new project is added with `/sadensmol project new <name>`, which writes the registry;
no skill or script changes.

```
<workspace folder>/
├── <project>/                        # source checkout (untouched by tasks)
└── .worktrees/<branch>/              # worktree BASE (not a git repo)
    ├── .claude/settings.local.json   # copied permissions
    ├── .task-base                    # base record: upstream, stacked, forkPoint, PR
    ├── PLAN.md                       # outside the repo — never committed
    └── <project>/                    # on <branch>, off main
```

`REPO="$BASE/<project>"`. The source checkout is never part of the task — confinement is
`sadensmol-router`'s always-on rule.

## Tracker: Linear via the workspace's Linear server (MUST FOLLOW)

Every tracker step goes through `sadensmol-linear` and its sadensmol defaults (load it
before the first tracker call). Server missing → say so, skip the
tracker step, and continue the task. A follow-up ticket goes to the registry's `todo` status, top of the
list, never a started one (`sadensmol-linear` → *Follow-up tickets*).

## The project's own rules win

The project repo's `CLAUDE.md` (or `AGENTS.md`) is binding inside the worktree: its source
of truth docs, its test and lint commands, and what must never run. Read it before the plan.

Never start, restart, poll or drive a project's live services from a task: no running
stack, no deploy, no bot against a real chat, no model load. Verify with the repo's tests
and offline checks. If something is provable only live, hand it to the user. So the default
workspace starts **no infra**: `sadensmol-task` → *Running infra* does not apply.

## Definition of done (every task)

1. **Tests written** — every new module has tests, where the project keeps them.
2. **Tests + lint green** — targeted while working; in full at `finish`: the test and lint
   commands the project repo names (its `CLAUDE.md`, else its `Makefile`), in the worktree.
3. **Docs updated** — the docs the project repo names as its source of truth match the
   code.
4. **Manual check approved by the user** before the PR — the user tests in their own
   stack, or confirms it is not testable by hand (`references/defaults/finish.md` → Step 6b).
5. **An r&d task** (`r&d | …` title) is done when its findings are written down: a doc in
   the repo (`docs/…`) when it changes the design, else a Linear comment or document — the
   plan says which.

## Session branches

The rule is `sadensmol-task` → *Session branches*. Here: one runner per test or lint
command of the project, a fixer on `RED`; one runner per PR if it has checks.
