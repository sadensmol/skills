# `help` — bare `/task`, handled by the dispatcher

**Triggers:** a bare `/task` with no arguments, "task help", "task --help", "what can
task do".

`help` lists every subcommand that works in the current project, with a one-line
explanation each. It answers in chat, changes nothing, and runs no subcommand. It never
falls through to `new`.

## 1. Resolve the project and the context

Run Steps 1–2 of `SKILL.md`. No project resolves → show only the *Generic* table below,
and say that `new`, `plan`, `finish`, `cleanup` and `pull` need a project workspace.

With a project, load `<project>-task` (do not reload it when its instructions are in
context) and read its `## Subcommands` section: the delegated subcommands it supports,
their aliases (e.g. `cleanup` / `delete`), and any subcommand it adds.

Then read the context, read-only:

```bash
case "$(pwd)" in */.worktrees/*) b="${PWD#*/.worktrees/}"; echo "WORKTREE=${b%%/*}";; *) echo "WORKTREE=none";; esac
```

In a worktree, also get the stage (`pair.py plan-open --base <base>`: exit 0 = PLAN,
exit 1 = IMPL), the mode (`<base>/.pair/config.json` present = pair, else single) and
whether `<base>/PLAN.md` exists.

## 2. Answer

One context line, then one table. Mark a row `(needs a worktree)` when the session is not
in one, and leave out nothing.

```
Project: <project> · <worktree <branch>, <PLAN|IMPL>, <pair|single> | not in a task worktree>
```

| Command | What it does |
|---|---|
| … | … |

Take the delegated rows from the project skill, in its table order, and say in each
row what that project's version does (e.g. its `pull` repo set). Then add the *Generic*
rows below, unchanged. Add a project-only subcommand at the end.

### Generic rows (this dispatcher)

| Command | What it does |
|---|---|
| `new <branch\|key\|URL> [--pair] [--base X]` | Creates the task worktrees, opens the task session, and starts `plan` (delegated) |
| `plan` | Writes `PLAN.md` and opens the plan gate for your approval (delegated) |
| `reviews` | Walks the plan steps with you one by one; reopens an approved plan, then back to IMPL |
| `review` | Opens plannotator on the current diff (or on `PLAN.md` / docs) |
| `changes` | Shows this task's diffs on one page, a screenful at a time |
| `reviewd` | Shows every logged decision, so you can keep, change or revert it |
| `reviewq` | Shows every question of the task and its answer, so you can comment |
| `status [--all]` | Where this task stands: phase, PRs, CI, what blocks `finish`; `--all` for every task |
| `list` | Lists the task worktrees of the project |
| `switch <branch> [--pair\|--single]` | Selects that task's session, opening it if needed |
| `pair` | Continues the current task in pair mode (writer + reviewer) |
| `fresh [--then …]` | Restarts the task sessions with a clean context |
| `zed` / `code` / `console` | Opens the task repo in Zed, VS Code or a console |
| `finish` | Local checks, code review, the diff gate, then commits, PRs and merge (delegated) |
| `cleanup` | Removes the task worktrees and sessions after the merge (delegated) |
| `pull` | Puts the project checkouts on their default branch (delegated) |

Drop a delegated generic row when the project skill does not support it. End with one
line: "Run `/task <command>`." Nothing else.
