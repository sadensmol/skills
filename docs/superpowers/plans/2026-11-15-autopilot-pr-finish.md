# Autopilot PR finish Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** PR buttons (Review, Merge), one finish path on merge (done line, ✅ reaction, branch cleanup), a ⏳ reaction while a run works, and a deploy line from a CI job.

**Architecture:** Slack reactions and a button card live in `internal/slackpost`. A new `PRMerge` usecase handles the Merge click and only runs `gh pr merge`; the existing `PRWatch` stays the one finish path and gains the done line, the reactions and the branch cleanup. A new `DeployWatch` loop reads run repos with `deploy_state = 'pending'`.

**Tech Stack:** Go, slack-go, gh CLI, git CLI, SQLite + goose migrations, testify suites with fake gh/agtermctl scripts under `tests/testdata`.

**Spec:** `docs/superpowers/specs/2026-11-15-autopilot-pr-finish-design.md`

Service root: `skills/autopilot/service`. Run tests with `make test` there.

## Global Constraints

- Reactions: `hourglass_flowing_sand` while active, `white_check_mark` on merge. `already_reacted` and `no_reaction` count as success.
- Merge button: action id `merge-pr`, value `<KEY> <PR URL>`; only the owner's click in a served channel counts.
- Merge method order: merge commit, squash, rebase — the first the repo allows. `gh pr merge --delete-branch`.
- Deploy texts: `🚀 deployed <repo> <short sha>`, `❌ deploy failed <job url>`, `⏭ deploy skipped`, `⚠️ no deploy seen for <repo>` after 2 hours.
- Done text: `✅ <KEY> done`.
- A cancel and a PR closed without a merge keep the branch and the worktree.
- Every Slack/git/gh side effect after the run ended is best effort: log it at the call site, never return it.

## Review Focus

- A repo of the run has no worktree any more (removed by hand): cleanup logs and goes on with the next step.
- The remote branch is already gone (GitHub auto-delete, or `--delete-branch`): no error, no warning.
- A Merge click on a PR that is already merged or closed: thread gets the gh error text, nothing else changes.
- The deploy job has not started yet when the watch first looks: stays pending, no post.
- A run that ended before the upgrade has no deploy columns: migration defaults make it `deploy_state = ''` and the deploy watch ignores it.

---

### Task 1: Slack reactions and the PR card

**Files:**
- Modify: `internal/slackpost/poster.go`
- Modify: `deploy/slack-manifest.yaml` (add `reactions:write` to `scopes.bot`)
- Test: `internal/slackpost/poster_test.go`

**Interfaces:**
- Produces: `const MergeAction = "merge-pr"`, `const ReactionWorking = "hourglass_flowing_sand"`, `const ReactionDone = "white_check_mark"`;
  `func (p *Poster) React(ctx, channelID, ts, name string) error` and `func (p *Poster) Unreact(ctx, channelID, ts, name string) error` (both treat `already_reacted` / `no_reaction` as nil);
  `type PRCard struct { IssueKey, PRURL, Text string }`;
  `func (p *Poster) PRCard(ctx, channelID, threadTS string, c PRCard) error` — posts `c.Text` (mrkdwn section) plus an action block: a link button "Review" with url `<PR URL>/files`, and a button "Merge" with action id `MergeAction`, value `<KEY> <PR URL>`, style primary.

- [ ] Test `TestReact_AlreadyReactedIsNoError` and `TestUnreact_NoReactionIsNoError` against the existing fake Slack server in `poster_test.go`; `TestPRCard_HasReviewLinkAndMergeButton` asserts the posted blocks hold url `https://github.com/o/r/pull/7/files` and a button with action id `merge-pr`, value `ACM-7 https://github.com/o/r/pull/7`.
- [ ] Implement with `api.AddReactionContext` / `api.RemoveReactionContext` and `slack.NewItemRef`; check `ErrorCode(err)`.
- [ ] `make test` passes.

### Task 2: ⏳ on start, off on every end

**Files:**
- Modify: `internal/usecase/intake.go` (after `SetCardTS` succeeds: React working on `m.TS`, best effort)
- Modify: `internal/usecase/start.go` (`report`: Unreact working)
- Modify: `internal/usecase/cancel.go` (after the run ends: Unreact working)
- Modify: `internal/usecase/prwatch.go` (`end`: Unreact working for merged and closed)
- Modify: each usecase's Slack interface to add `React`/`Unreact`; `cmd/autopilot/serve.go` needs no change (Poster already passed).
- Test: `tests/intake_test.go`, `tests/start_test.go`, `tests/cancel_test.go`, `tests/prwatch_test.go`

**Interfaces:**
- Consumes: Task 1 `React`, `Unreact`, `ReactionWorking`.

- [ ] Extend `tests/fake-slack.go` to record `reactions.add` / `reactions.remove` (name, channel, timestamp) and expose `Reactions()`.
- [ ] Tests: `TestIntake_StartedRunGetsTheWorkingReaction` (on the request ts); `TestStart_FailedStartRemovesTheWorkingReaction`; `TestCancel_RemovesTheWorkingReaction`; `TestPRWatch_ClosedPRRemovesTheWorkingReaction`.
- [ ] Implement; `make test` passes.

### Task 3: Merge button

**Files:**
- Modify: `internal/gh/client.go` — `func (c *Client) MergeMethod(ctx, ref PRRef) (string, error)` (`gh api repos/<o>/<r>` → `allow_merge_commit`, `allow_squash_merge`, `allow_rebase_merge` → `"--merge"`, `"--squash"`, `"--rebase"`; none → error) and `func (c *Client) Merge(ctx, prURL, method string) error` (`gh pr merge <url> <method> --delete-branch`).
- Create: `internal/usecase/pr-merge.go` — `type PRMerge`, `PRMergeData{ GH IPRMergeGit; Slack IStatusSlack; Runs IPRMergeRuns }`, `func (m *PRMerge) Merge(ctx, click controllerslack.MergeClick) error`: the run of the key must be active and in the click's channel, the URL must equal one of its run repo PR URLs; on a gh error post `could not merge <url>: <error>` to the thread and return nil.
- Modify: `internal/controller/slack/interaction.go` + `message.go` — `type MergeClick struct{ IssueKey, PRURL, ChannelID string }`, `IMerger` in `HandlerData`, dispatch `slackpost.MergeAction` like Cancel (split value on the first space).
- Modify: `internal/usecase/pr-open.go` — `mention` for the PR open text goes through `Slack.PRCard` (text keeps the owner mention `<@owner> ` prefix that `Mention` adds today).
- Modify: `cmd/autopilot/serve.go` wiring.
- Modify: `tests/testdata/fake-gh.sh` for `api repos/*` and `pr merge`.
- Test: `tests/prmerge_test.go`, `internal/controller/slack/slack_test.go`, `tests/prwatch_test.go` (PR open card)

- [ ] Tests: `TestMerge_UsesTheFirstAllowedMethod` (only squash allowed → argv has `--squash --delete-branch`); `TestMerge_UnknownPRIsRefused` (URL not of the run → no gh merge call); `TestMerge_GHFailurePostsTheError`; handler test `TestBlockActions_MergeClickOfOtherUserIgnored`; `TestPROpen_PostsReviewAndMergeButtons`.
- [ ] Implement; `make test` passes.

### Task 4: Finish on merge — done line, ✅, branch cleanup

**Files:**
- Modify: `internal/gh/client.go` — `PullRequest` gains `HeadRefName string \`json:"headRefName"\`` and `MergeCommit struct{ OID string \`json:"oid"\` } \`json:"mergeCommit"\``; `View` asks for `headRefName,mergeCommit`. Add `func (c *Client) DeleteBranch(ctx, ref PRRef, branch string) error` (`gh api -X DELETE repos/<o>/<r>/git/refs/heads/<branch>`; HTTP 422 or 404 in stderr → nil).
- Modify: `internal/usecase/prwatch.go` — `pollPR` on merge stores the branch and merge SHA (Task 5 columns); `end` when merged: after `closeRunSessions`, post `✅ <KEY> done`, Unreact working, React done, then per repo with a PR: `Worktrees.RemoveWorktree(ctx, RepoPath(repo), <WorktreeBase>/<repo>, branch, true)` and `GH.DeleteBranch`. `PRWatchData` gains `Worktrees IWorktrees` and `RepoPath func(string) string`.
- Modify: `cmd/autopilot/serve.go` wiring.
- Test: `tests/prwatch_test.go`

**Interfaces:**
- Produces for Task 5: `RunRepo.Branch`, `RunRepo.MergeSHA` are set at the merge.

- [ ] Tests: `TestPRWatch_MergePostsDoneAndTicksTheRequest` (thread has `✅ ACM-7 done`; reactions: remove hourglass, add white_check_mark on the request ts); `TestPRWatch_MergeRemovesWorktreeAndBranches` (worktree dir gone, local branch gone, fake gh got `DELETE …/git/refs/heads/acm-7-…`); `TestPRWatch_MergeWithRemoteBranchGoneIsQuiet`; `TestPRWatch_ClosedPRKeepsTheBranch`.
- [ ] Implement; `make test` passes.

### Task 5: Deploy watch

**Files:**
- Create: `db/migrations/00007_run-repo-deploy.sql` — `ALTER TABLE run_repo ADD COLUMN branch TEXT NOT NULL DEFAULT ''`, `merge_sha`, `deploy_state` (`''`, `pending`, `deployed`, `failed`, `skipped`, `unseen`), `merged_at TEXT NOT NULL DEFAULT ''`.
- Modify: `internal/domain/run.go` (`RunRepo.Branch`, `MergeSHA`, `DeployState`, `MergedAt`), `internal/repository/run.go` (`SetMerge(ctx, key, repo, branch, sha string, deployPending bool, now)`, `SetDeployState(ctx, key, repo, state string, now)`, `PendingDeploys(ctx) ([]PendingDeploy, error)` with `PendingDeploy{ Run domain.Run; Repo domain.RunRepo }`, and `Repos` reads the new columns).
- Modify: `internal/config/registry.go` — `Repo.DeployJob string` from optional key `deploy_job`.
- Modify: `internal/gh/client.go` — `func (c *Client) JobOnCommit(ctx, owner, repo, sha, job string) (Job, bool, error)` with `Job{ Status, Conclusion, URL string }` (`gh run list --repo o/r --commit <sha> --json databaseId`, then `gh run view <id> --repo o/r --json jobs`, first job whose `name` equals `job`).
- Create: `internal/usecase/deploy-watch.go` — `DeployWatch` with `Run(ctx)` / `Tick(ctx)` like `PRWatch`; per pending repo: not found or not completed → stay pending unless `now - merged_at > 2h` → `unseen`; completed success → `deployed`; `skipped` → `skipped`; other conclusions → `failed`. Post the matching spec text to the run thread, then set the state.
- Modify: `internal/usecase/prwatch.go` — at the merge, `SetMerge(..., deployPending = project repo has DeployJob)`.
- Modify: `cmd/autopilot/serve.go` — start `DeployWatch.Run` next to the PR watch with the same interval and shutdown.
- Modify: `tests/testdata/fake-gh.sh` for `run list` / `run view`.
- Test: `tests/deploywatch_test.go`, `internal/config/registry_test.go`, `tests/run-repository_test.go`

- [ ] Tests: `TestDeployWatch_SuccessPostsDeployed` (`🚀 deployed blogger abc1234`); `TestDeployWatch_FailurePostsTheJobURL`; `TestDeployWatch_SkippedJob`; `TestDeployWatch_NotStartedStaysPendingQuietly`; `TestDeployWatch_TwoHoursWithoutJobIsUnseen`; `TestDeployWatch_RepoWithoutDeployJobIsNeverPending`; registry `TestRegistry_ReadsOptionalDeployJob`.
- [ ] Implement; `make test` passes.

### Task 6: Docs and skills

**Files:**
- Modify: `skills/autopilot/references/feed.md` — thread lines `✅ <KEY> done`, deploy lines; the ⏳/✅ reactions.
- Modify: `skills/autopilot/references/run.md` — PR follow-up: the owner may merge from the Merge button; the run session does nothing at the merge.
- Modify: `skills/autopilot/references/control-room.md` — Start: the Slack app needs `reactions:write`; the manifest in `service/deploy/slack-manifest.yaml` is the source.
- Modify: `skills/project/` registry docs — optional repo key `deploy_job`.
- Modify: `service/README.md` / `service/docs` where the manifest, scopes or registry keys are listed.

- [ ] `grep -rn "reactions:write\|deploy_job" skills/autopilot skills/project` shows every new fact once in each place that lists its kind.
