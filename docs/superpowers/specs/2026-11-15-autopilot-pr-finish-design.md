# Autopilot: PR buttons, finish on merge, deploy line, request reactions

Status: approved by the owner (2026-11-15). Small change, short design.

## Intent

A run must finish on its own after the PR, with no step in agterm or GitHub that a person has to
remember. The owner sees the state of every request at a glance in Slack: working, done, deployed.

## Goals

1. The `✅ PR open` post carries two buttons: **Review** (a link to the PR's *Files* tab) and
   **Merge** (owner only, runs the merge).
2. A merge from Slack or from GitHub ends the same way: the thread gets `✅ <KEY> done`, the request
   message gets a ✅ reaction, Linear moves to done (already done by `setDone`, kept), every run
   session closes (already done, kept), and the run branch goes away: the worktree, the local branch
   and the remote branch.
3. While a run works, the request message carries a processing reaction (⏳). It goes away when the
   run ends in any way.
4. A repo that names a deploy job in the registry gets a deploy line after the merge: `🚀 deployed`
   or `❌ deploy failed <url>`, from that CI job on the merge commit.

## Non-goals

- No check on the host that the new image runs (no SSH, no `docker inspect`).
- A PR closed without a merge, and a cancel, keep the branch and the worktree (a cancel never
  deletes work; that rule stays).
- No change to the review, gate or follow-up loops.

## Approaches

- **Deploy signal.** (a) The CI deploy job on the merge commit, named per repo in the registry —
  picked: no host access, works for any project with CI. (b) A per-project host check command —
  rejected: needs SSH and secrets per project. (c) Ask the run session to watch — rejected: the run
  session closes at the merge.
- **Merge button.** (a) The service runs `gh pr merge` and lets the PR watch see the merge —
  picked: one finish path for Slack and GitHub merges. (b) Finish the run inside the click handler —
  rejected: two finish paths.

## Decisions

- Reactions: `hourglass_flowing_sand` while active, `white_check_mark` on merge. Slack answers
  `already_reacted` and `no_reaction` count as success. The bot needs the `reactions:write` scope:
  the manifest gets it, and the owner reinstalls the app.
- ⏳ goes on when the intake creates the run, and off on every end: merge, close without merge,
  cancel, failed start.
- Merge button: action id `merge-pr`, value `<KEY> <PR URL>`. Only the owner's click in a served
  channel counts, as for Cancel. The method is the first the repo allows of merge commit, squash,
  rebase (`gh api repos/<o>/<r>`). `gh pr merge --delete-branch`. A failed merge posts its error to
  the thread. The PR watch then finishes the run on its next poll.
- Branch cleanup on merge, per repo of the run, after the sessions close: `git worktree remove
  --force`, `git branch -D`, and the remote branch through `gh api -X DELETE
  repos/<o>/<r>/git/refs/heads/<branch>` (already gone counts as success). The branch name is the
  PR's `headRefName`. Each step is best effort and logged.
- Deploy: optional repo key `deploy_job` in `.workspace/projects/<project>.json`. At the merge the
  service stores the merge commit SHA and `deploy=pending` on the run repo (a new migration). A
  deploy watch polls the CI jobs of that commit (`gh run list --commit`, `gh run view --json jobs`)
  every PR poll interval. Job success → `🚀 deployed <repo> <short sha>`; failure or cancel →
  `❌ deploy failed <job url>`; skipped → `⏭ deploy skipped`; nothing after 2 hours →
  `⚠️ no deploy seen for <repo>`. Each ends the watch of that repo.

## Left to the builder

File layout, names, the migration number, and test shape follow the service's existing code.
