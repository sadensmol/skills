# `cleanup` — default specifics

Read `<sadensmol-task-skill-dir>/references/cleanup.md` and follow it. The hooks:

- Inside a worktree = the cwd is under `<workspace folder>/.worktrees/<name>/`.
  Confirm: "Delete worktree `<name>`? This removes the local branch and the whole worktree
  directory."
- Readiness: every PR of the task is `MERGED`
  (`gh pr view <pr> --json state,mergedAt --jq '"\(.state) mergedAt=\(.mergedAt // "-")"'`)
  and its checks, if any, are green. Any failure → stop and name the PR.
- `tracker_state`: per the sadensmol defaults of `sadensmol-linear` → *Status at task finish*. After the merge
  the issue is in the registry's `done` status, with the assignee unchanged from `finish`
  (or the user said to leave it). Not done → ask the user before setting it.
- Script: `bash <sadensmol-task-skill-dir>/scripts/cleanup-task.sh <branch-name>`. The task session is
  named by the task id (`ABC-21 …`).
- Remote branch, only when the PR is merged:

  ```bash
  git -C <worktree-base>/<project> ls-remote --exit-code origin <branch> >/dev/null 2>&1 &&
    git -C <worktree-base>/<project> push origin --delete <branch>
  ```
