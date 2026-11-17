# `new` — default specifics

Read `<sadensmol-task-skill-dir>/references/new.md` first (mode question, session naming, task autonomy,
fresh-from-remote, `--base`). This file adds the default hooks.

```bash
bash <sadensmol-task-skill-dir>/scripts/new-task.sh <branch-name> --product <project> [--base <branch>] [--pair | --single]
```

## The task argument — a key or URL instead of a branch

`task new ABC-21` or a pasted Linear URL → resolve it yourself; never ask the user to
spell the branch, never slugify the URL.

1. Fetch the issue via `sadensmol-linear` and its sadensmol defaults (`get_issue`, `fields: ["title",
   "gitBranchName", "project"]`).
2. Branch = `gitBranchName` without its `<user>/` prefix
   (`ann/abc-21-add-health-endpoint` → `abc-21-add-health-endpoint`).
3. Project = the issue's Linear `project`. Check it with
   `registry.py product-of-project <project>` (`references/defaults/project.md` → *Projects*).
   No Linear project, or not a project in the registry: stop and ask which project.
4. Say the branch name and the project in chat before creating anything.

A plain `<branch>` with a `<KEY>-<n>` key → fetch that issue for its project the same way.
The script takes `--product`, else the repo folder of an existing worktree, else a key whose
prefix is a repo name, and refuses a repo that is not in the registry.

## What the script does

- **Brand-new branch** → worktree + branch off `origin/main` (or `origin/<base>` when
  stacked); copies gitignored `.env` files; runs the repo's dependency sync when it has one (non-fatal);
  opens the session.
- **Branch exists, no worktree** → attaches a worktree onto it.
- **Worktree exists** → rebases onto its upstream, preserving WIP with an auto-stash.
- **Session exists** → reused, never selected; `task plan` is typed into it when the task
  still needs planning.

Auto-plan follows the TASK's state: `task plan` is seeded when there is no `PLAN.md`, no
uncommitted change and no commit ahead of the upstream. `uv.lock` churn and harness/OS
files do not count.

**Session:** `ABC-21 add-health-endpoint` from the branch; context `ABC-21`; tint `blue`.

## The base

Per the shared `new.md` → *Base branch*. A task key or Linear URL → its PRs from
`get_issue` `attachments` on the workspace's Linear server, then `--base <headRefName>`. A PR number →
`--base <headRefName>`. The base PR must exist and be OPEN before the worktree is created
(`gh pr list --repo <owner>/<repo> --head <base> --state all --json
number,url,state --jq '.[0]'`); none or CLOSED → stop and ask; MERGED → start off `main`.
