# `new` — shared behaviour (the project skill adds its layout and hooks)

**Triggers:** "task new", "new task", "start task", "task <branch-name>", "create
worktree(s)", "task new <branch> --pair".

`new <branch>` and a bare `<branch>` are the same. The project's `new-task.sh` does the
work:

```bash
bash <project-skill-dir>/scripts/new-task.sh <branch> [--base <branch> | --base <repo>:<branch>]... [--pair | --single]
```

- **Branch does not exist** → cut a new branch and worktree(s), refresh deps per the
  project, open the session, seed `task plan` (per the project's auto-plan rule).
- **Branch or worktree already exists** → reuse it (a switch): no new branch, open or
  select the session.

## Mode question (MUST FOLLOW)

When `new` is about to create the worktree (`<root>/.worktrees/<branch>/` does not exist)
and the arguments carry neither `--pair` nor `--single`, ask the user through `ask-user`
before running the script: `Single` (no flag) or `Pair` (`--pair`). The first run records
the mode — `.pair/config.json` at the base means pair, no `.pair/` means single — and
every later open reuses it without asking (`switch`, `new <existing>`, a re-run after a
closed session). `--single` is an explicit answer and never removes `.pair/`. Pair
mechanics: `references/pair-mode.md`.

## Terminal session

An agterm session named `<task id> <descriptor>` — the task id plus the branch
descriptor after the `<prefix>-<n>-` part (`abc-12-be-fix-thing` → `ABC-12
be-fix-thing`); fallback: the whole branch basename. Created with

```bash
agtermctl session new --cwd <worktree-base> --after "$AGTERM_SESSION_ID" --no-select --command "zsh -lc '<harness> …'"
```

The anchor places it in the current session's workspace, directly below it. **`--no-select`
is required (MUST FOLLOW):** the session opens in the background, and the user's focus never
moves to it. An existing session with that name is reused and is not selected either. The script then sets the title-bar context
to the task id (`agtermctl session context <ID> --target <id>`) and flags it
(`agtermctl session flag on --target <id>`), so the flagged view lists every live task
(agterm 0.26.0+, best-effort). `cleanup-task.sh` mirrors this naming to close it.

## Autopilot

Claude Code launches with `--dangerously-skip-permissions` and the appended autopilot
prompt. OpenCode (`OPENCODE=1`, where the project supports it) launches with `--auto`
and always a first `--prompt` (`task plan`, or a resume prompt), so it never sits on its
start screen. The launch line carries the stage (`SKILL.md` → *Stages*): Claude Code the
flags `pair.py claude-args` prints (`--session-id`/`--resume`, `--model`, `--effort`),
OpenCode the agent `pair.py opencode-args` prints (`--agent planning` in PLAN, `--agent
main` in IMPL). The project `new-task.sh` also answers `--print-command`, the writer
start line that `pair.py stage` uses. Both resolve routine questions alone and ask the user only for
destructive or irreversible actions, production impact, scope changes, and the plan and
review approval gates.

## Fresh from remote (new worktrees)

A new worktree starts on the **remote** tip of its upstream — the repo's default branch,
or `origin/<base-branch>` when stacked — via `git fetch` + `git worktree add -b <branch>
<path> origin/<upstream>`. Never a stale local ref, never the source tree's uncommitted
changes; the source checkout is left untouched. An existing remote branch is preferred
over a stale local copy; a local ref is the last resort.

## Base branch — a stacked task

`--base` starts the task off another task's branch. Repeatable, in two forms:

- `--base <branch>` — one branch for every repo of the worktree.
- `--base <repo>:<branch>` — that repo only (`:` cannot appear in a git ref, so the form
  is unambiguous). Used when the base task's PRs sit on different branches per repo.

A value is always a plain branch name (no `origin/`, no PR number). Prose forms mean the
same: "based on `X`", "on top of `X`", "stacked on `X`", "parent branch `X`", "parent
task `X`". Everything after the branch token that is not a recognised flag is context,
never part of the branch name. The project skill turns what the user gave into `--base`
values before it runs the script:

| The user gave | `--base` values |
|---|---|
| a plain branch | `<branch>` |
| a PR number or URL | `<repo>:<headRefName>` for that PR's repo (`gh pr view <pr> --json headRefName -q .headRefName`) |
| a tracker task — key or URL (Linear, Jira) | one `<repo>:<headRefName>` per repo, from *Based on a task* below |

Per repo:

- Repo **has** `origin/<its base>` → its worktree is cut off it, and re-opening rebases
  onto it instead of the default branch.
- Repo **has no base**, or its base is not on origin → its default branch, with a
  `NOTE:` line.

So one worktree can be stacked in some repos and plain in others — expected. If **no**
repo has its base, the script warns and nothing is stacked; that is almost always a
typo.

### `.task-base` — the per-repo base record

The script records every repo of the worktree, stacked or not, in
`<worktree-base>/.task-base`: the worktree base, outside every repo, never committed.
It is git-config format, one section per repo; `scripts/task-base.sh` (this skill)
documents every key and holds every read and write — never parse or edit the file by
hand:

```ini
[repo "<repo>"]
    default   = main          # the repo's default branch
    base      = <branch>      # requested base (stacked task); absent → none
    stacked   = true          # the base is on origin and the branch follows it
    upstream  = <branch>      # what the branch follows and its PR targets
    forkPoint = <sha>         # the origin/<upstream> commit the branch sits on
    baseTask  = <KEY>         # base task (optional)
    basePr    = <url>         # the base branch's PR in this repo (optional)
    pr        = <url>         # this task's PR in this repo, set by `finish`
```

```bash
. <sadensmol-task-skill-dir>/scripts/task-base.sh
tb_migrate <wb>                        # an old one-line-per-base file → sections (idempotent)
tb_plan <wb> <repo> <dir> [<default>]  # resolve stacked/upstream from `base`; prints the upstream
tb_mark_fork <wb> <repo> <worktree>    # after creating a worktree: record forkPoint
tb_sync <wb> <repo> <worktree>         # rebase onto origin/<upstream> via forkPoint; moves forkPoint
tb_reanchor <wb> <repo> <worktree>     # base merged → follow the default branch again
tb_diff_base <wb> <repo> <worktree>    # the commit this task's own changes start from
tb_get / tb_set <wb> <repo> <key> [<value>];  tb_show <wb>
```

- A re-run with `--base` replaces every repo's `base` (and clears `basePr` /
  `baseTask`); a re-run without it keeps the record.
- **Re-open rebases through forkPoint**, never with a plain `git rebase
  origin/<upstream>`: a force-pushed base, or a base squash-merged and deleted, then
  replays only this task's commits. A base gone from origin turns the repo back to its
  default branch (`tb_plan` says so in a `NOTE:`).
- After `new` resolved a base from a PR or a task, record it per stacked repo:
  `tb_set <wb> <repo> basePr <url>` and `tb_set <wb> <repo> baseTask <KEY>`.
- A task found to be stacked after it started: `tb_set <wb> <repo> base <branch>`, then
  `tb_plan` and `tb_sync` for that repo.

Git/gh mechanics and stack registration: `sadensmol-github` → "Stacked Pull Requests".

### Based on a task — stack on that task's PRs, per repo (MUST FOLLOW)

When the base is a tracker task ("task new X based on ABC-12", `--base ABC-12`, a tracker
URL), the base is that task's **PRs**, not a branch named after its key. A task's PRs
often live on a branch with another key or no key, and on a different branch in each
repo. So never resolve the key with `git ls-remote | grep <KEY>`. Before you create
anything:

1. **Fetch the base task** through the project's tracker skill. Collect every GitHub PR
   URL linked to it (`github.com/<owner>/<repo>/pull/<n>`): Linear → the `attachments`
   of `get_issue`; Jira → `GET /rest/api/2/issue/<KEY>/remotelink` (the `object.url`
   values). The tracker cannot be reached → say so and stop; never guess a base.
2. **Search GitHub as well**, in every repo of this worktree (the project skill lists
   them) — PRs whose title or body name the key are not always linked:

   ```bash
   gh pr list --repo <owner>/<repo> --state all --search "<KEY>"      --json number,url,state,headRefName,baseRefName,title
   ```

   Keep a search hit only when its title, body or branch names the key. Merge it with
   the tracker links by URL. Drop PRs in repos that are not part of this worktree, and
   name them in the report.
3. **Pick one base per repo**, by state:

   | The repo's PRs for the base task | Result for this repo |
   |---|---|
   | one OPEN PR | stacked: `--base <repo>:<its headRefName>` |
   | several OPEN PRs | ask (`ask-user`) which one; never guess |
   | only MERGED PRs | not stacked: the work is in the default branch already |
   | only CLOSED unmerged PRs | ask: stack on the abandoned branch, or start off the default branch |
   | none | not stacked |

   An OPEN PR that targets another branch than the repo default is a stack already: stack
   on it too and say so. Ignore MERGED and CLOSED PRs when an OPEN one exists.
4. **Report the chain before creating anything**, one line per repo:
   `<repo>: <KEY> → <PR url> (<headRefName>)` or `<repo>: not stacked (<reason>)`.
   No repo is stacked → say so and ask whether to start off the default branches or to
   stop.
5. **Run the script** with one `--base <repo>:<headRefName>` per stacked repo.

`finish` then opens each repo's PR against its own base and registers the GitHub stack
per repo (`references/finish.md` → *Stacked repos*).
