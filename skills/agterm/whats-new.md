# What's new in agterm

Run this when the user asks what is new in agterm, or asks to update this skill. It does three things:
syncs this skill with the app's bundled skill, checks the cookbook for new recipes, and maps what is
new onto the user's current workflow.

## State

The baseline lives outside the skill repository (the repository is public):

```bash
state="${XDG_STATE_HOME:-$HOME/.local/state}/sadensmol-agterm"
mkdir -p "$state/upstream"
# $state/upstream/    — the bundled skill files as they were at the last sync
# $state/cookbook-sha — the cookbook commit read at the last check
```

No state yet (first run) → the baseline for the bundle is `git show HEAD:<file>` of this skill's own
copy, and every cookbook recipe counts as unseen.

## 1. Sync the bundled skill

1. Locate the bundle from the running client, not a guessed path:
   `agtermctl version --socket "$AGTERM_SOCKET"` prints `client: <app>/Contents/MacOS/agtermctl`; the
   bundle is `<app>/Contents/Resources/agterm/`. Report the running `version` and the `installed` one
   from `--json`; when they differ, the app was upgraded under the running process.
2. Find this skill's source directory: resolve the loaded skill directory through its symlink
   (`readlink -f`). Take a `sadensmol-git` snapshot of it before copying anything.
3. Diff each bundle file against the baseline. `cookbook.md`, `examples.md`, `reference.md`,
   `troubleshooting.md` and `scripts/` are copied verbatim, then re-apply any local edit to them
   (check `git diff HEAD` for one first, and keep it). `scripts/agst.sh` is not in the bundle — it
   comes from the cookbook (step 2a): never delete it while copying `scripts/`.
4. `SKILL.md` is NOT copied: this copy is a slimmed summary that points into `reference.md`. Diff the
   bundle `SKILL.md` against `$state/upstream/SKILL.md` and port only the new parts: description and
   `when_to_use` triggers, corrections, new commands into the command summary (one line each, with the
   `## section` of `reference.md`), and short how-to sections. Keep this skill's `name`, its wording
   choices, and this file's pointer.
5. Copy the whole bundle into `$state/upstream/`.

## 2. Check the cookbook

Follow `cookbook.md`: resolve `master` to a SHA, fetch the index at that SHA, never guess recipe names.

- With a saved SHA, list what changed under `cookbook/` since then:
  `https://api.github.com/repos/umputun/agterm/compare/<old-sha>...<new-sha>` (keep the files whose
  path starts with `cookbook/`). New directories are new recipes; changed ones are updates.
- Without one, every recipe is unseen; report only those that fit the workflow (step 3).
- Check each candidate's minimum version against the running version.
- Write the new SHA to `$state/cookbook-sha` after reporting.

Read a recipe's README (from the pinned archive, as `cookbook.md` says) only for one you recommend.
Never install anything in this flow — except the bundled `agst.sh` below, which this skill owns.

### 2a. Refresh the bundled `agst.sh` (every run)

At the same `<sha>`, fetch `cookbook/long-commands-status/agst.sh` (raw URL at that SHA; the recipe
must still be linked from the index) and compare it with this skill's `scripts/agst.sh`:

```bash
new="$(mktemp -d)/agst.sh"
curl -fsSL "https://raw.githubusercontent.com/umputun/agterm/<sha>/cookbook/long-commands-status/agst.sh" -o "$new"
cmp -s "$new" <skill-source-dir>/scripts/agst.sh && echo AGST_CURRENT || diff -u <skill-source-dir>/scripts/agst.sh "$new"
```

- `AGST_CURRENT` → nothing to do.
- A diff → read the new script in full (it runs in every infra session). Check the recipe README's
  minimum agterm version against the running one. Then copy it over `scripts/agst.sh` verbatim and
  keep its mode `+x`.
- The recipe is gone from the index, or the fetch fails → keep the current copy and report it.

Report the result as its own line: unchanged, updated (with a one-line summary of the diff), or kept.

## 3. Map it onto the current workflow

Read what the user actually runs before suggesting anything:

- The workflow skills that already drive agterm: grep the installed skill directories for
  `agtermctl` (the task / worktree flow, pair mode, review and plan viewers, status hooks).
- The current context: cwd, branch, the task in hand, and the user's agterm keymap and hooks
  (`agtermctl keymap list`, `agtermctl hooks list`).
- Existing installed recipes, so you do not suggest one already in place.

## 4. Report

One list, most useful first. Per item: what is new (feature or recipe, with its version), and the
concrete place it fits — which workflow skill, step, or habit it would change or replace. Mark items
that need nothing but a skill edit apart from items that need an install. Drop new things that fit
nothing. Then ask which to adopt; adopt nothing unasked.
