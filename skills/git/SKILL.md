---
name: sadensmol-git
description: "Local git safety net — take a restore point BEFORE any destructive change, and restore from it when something breaks. Default-on for code work: load it BEFORE the FIRST `git` command of the session, whatever that command is, including read-only ones (`git status`, `diff`, `log`, `branch`) — do not wait to recognise a destructive command, because by then the snapshot is already too late. Covers `git reset --hard`, `git restore` / `git checkout -- <path>`, `git clean -fd`, `git rebase` (incl. `--update-refs`, `--onto`, `-i`), `git merge`, `git switch`/`checkout` with a dirty tree, `git branch -D`, `git stash drop`/`clear`, `git commit --amend`, `git push --force` / `--force-with-lease`, `git filter-branch` / `filter-repo`, `git gc --prune`. Load it equally before any BULK automated edit that rewrites many files at once (mass `sed -i`, codemod, formatter or linter run with --fix across the tree, generated-code regeneration, find -exec rewrite, a scripted mass rename, deleting or moving a directory), and when work has already been lost and must be recovered — stash list, reflog, dangling commits. Do NOT load it for a turn that never touches git and never bulk-edits files: explaining code, answering a question, reading, planning, or discussing an approach. Provides the stash-and-reapply snapshot, the pre-op branch marker for history rewrites, restore procedures, and the cases where stash is not enough."
---

# Git Safety Net

The rule: **before anything that can destroy work, create a restore point. After
it, verify. If it broke, restore.**

Snapshots are cheap. Losing a working tree the user cannot reproduce is not.

## ⚠️ Take the snapshot BEFORE, not after

You cannot snapshot after the destructive command. The moment you think "this
should be safe" is exactly the moment to snapshot. There is no situation where
skipping the snapshot is the right call — it costs one command.

This applies **whether or not you are the one who broke it**: a rebase that hits
conflicts, a `--fix` run that mangles files, a codemod that matched too broadly.

This is why the skill loads on the **first** git command of the session, even a
read-only `git status`: recognising a destructive command is not a reliable
trigger, because sequences turn destructive mid-way (a `git switch` that needs a
dirty tree cleared, a rebase that hits conflicts, a `--fix` that follows a diff).
Snapshot when git work starts, not when it looks risky.

## Triggers — snapshot before any of these

**Git commands that discard or rewrite:**

- `git reset --hard`, `git reset` to an earlier commit
- `git restore <path>`, `git checkout -- <path>`, `git restore --staged --worktree`
- `git clean -fd` / `-fdx` (deletes untracked and ignored files outright)
- `git rebase` in any form — `-i`, `--onto`, `--update-refs`, `--continue` after conflicts
- `git switch` / `git checkout <branch>` with uncommitted changes
- `git merge`, `git cherry-pick`, `git revert` on a dirty or complicated tree
- `git branch -D`, `git stash drop`, `git stash clear`
- `git commit --amend` (rewrites the tip)
- `git push --force` / `--force-with-lease`
- `git filter-branch`, `git filter-repo`, `git gc --prune=now`, `git reflog expire`

**Bulk edits that are not git at all** — these are the ones that actually lose
work, because git does not know they happened:

- mass `sed -i` / `perl -pi -e` across many files
- a codemod, migration script, or scripted mass rename
- a formatter or linter run with `--fix`/`--write` across the whole tree
- regenerating generated code over hand-edited files
- `find … -exec` that rewrites or deletes
- deleting or moving a directory
- `rm` of an **untracked** file — git has never seen it, so there is nothing to
  restore from: no reflog entry, no stash, no `git restore`

**Rule of thumb:** if a single command touches more files than you could review
in the diff, snapshot first.

### Moving code between files — write the destination FIRST, delete last

When relocating code (a test into an existing suite, a helper into another
package, a section into another doc), the order is: **write the destination,
verify it compiles and carries every piece, and only then delete the source.**
Never delete first and reconstruct the moved content from memory — what is
missing is exactly what you fail to remember, and with an untracked source there
is no diff to check yourself against afterwards.

Verify by comparing the source's contents against the destination item by item
(each test case, each assertion, each function), not by an overall impression.

## The snapshot

### Working-tree changes → stash, then immediately re-apply

This is the default. It stores a full copy in the stash **and leaves the working
tree exactly as it was**, so work continues normally:

```bash
git stash push -u -m "pre-<short-op-label>"   # -u includes untracked files
git stash apply                               # tree restored; the snapshot stays in the list
```

Two commands, no behaviour change, and a labelled restore point in `git stash
list`. Use `apply`, never `pop` — `pop` deletes the snapshot, which is the whole
point of having it.

- `-u` includes untracked files. Use it — untracked new files are the most common
  casualty of `git clean` and of codemods.
- Do **not** use `-a`/`--all` by default: it also stashes ignored files, which
  can mean `node_modules`, build output, and local `.env` files. Slow, and it can
  move a secret into git objects.

### History rewrites → a branch marker instead

`rebase`, `reset --hard`, and `push --force` need the commit the branch pointed
at, not the working tree. Rebase also refuses to start on a dirty tree, so the
stash-and-reapply trick does not apply:

```bash
git branch pre-rebase/<branch-name>   # a plain pointer at the current tip, costs nothing
```

Do both when the tree is also dirty: stash-and-apply for the uncommitted work,
branch marker for the commits.

### Not a git repo, or nothing committed yet

`git stash` fails in a repo with no commits, and does nothing outside a repo. Fall
back to a plain copy into the session's scratch directory (see the router's
`.scratch/` rule) and tell the user where it is:

```bash
cp -a <dir> "$base/.scratch/pre-<op>-<dir>"
```

## After the operation — verify

Check that the change did what was intended before moving on:

```bash
git status          # unexpected deletions or untracked losses show here
git diff --stat     # scale of the change — surprising size means stop
```

If tests existed and passed before, they must still pass. A destructive operation
that "looks fine" in `git status` can still have reverted content.

## Restore

**From a stash snapshot** — overwrite the working tree with the snapshot's
content, keeping the snapshot:

```bash
git stash list                        # find the labelled entry
git checkout stash@{0} -- .           # restore all files from it
git checkout stash@{0} -- path/to/file   # or just one
```

`git stash apply stash@{n}` also works, but merges rather than overwrites and can
conflict; `checkout … -- .` is the blunt, predictable option.

**From a branch marker:**

```bash
git reset --hard pre-rebase/<branch-name>
```

**When the snapshot was never taken** — the reflog is the last resort. It holds
every position HEAD had, for ~90 days:

```bash
git reflog                     # find the sha from before the damage
git reset --hard <sha>
git fsck --lost-found          # dangling commits, incl. dropped stashes
```

The reflog covers commits. It does **not** cover uncommitted work or untracked
files destroyed by `git clean` — those are gone. That asymmetry is the reason the
stash snapshot exists.

## Cleaning up

**Never** run `git stash drop` or `git stash clear` on a snapshot you created.
Leave it — the user decides when the work is safe. A stale stash entry costs
nothing; a deleted one costs the recovery path.

Delete a `pre-<op>/…` branch marker only when the user says the result is good.

## Interaction with the never-commit rule

Creating commits is the user's job, and `git commit` / `git add` stay off-limits.
`git stash push` and a `git branch` marker are **not** that: they take a snapshot
without touching the user's branch, index state, or history, and are explicitly
sanctioned as the safety net. Do not reach for `git commit -m "wip"` as a
substitute.

## Worktrees

Stash entries and branches live in the shared repo, so a snapshot taken in one
worktree is visible from all of them. Label the stash with the branch name so it
is identifiable later. Always run these commands with the worktree's own cwd —
see the router's worktree-confinement rule.

## Tell the user

When you take a snapshot before something risky, say so in one line, and name the
restore command:

> Snapshotted first: `git stash list` → `pre-mass-rename`. Restore with
> `git checkout stash@{0} -- .`

If something did break, say what broke and that you restored it. Never restore
silently and continue as if nothing happened.
