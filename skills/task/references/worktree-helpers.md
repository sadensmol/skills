# Worktree helpers and workspace root

Read this when a handled-here subcommand or `references/infra.md` needs `$root`, `$branch`,
`$base`, `$WT` or `real_changes`.

## Workspace root — derived from the cwd only

`list`, `switch` and `status --all` work on the workspace root (the directory holding
`.worktrees/`):

```bash
cwd="$(pwd)"; root=""
case "$cwd" in */.worktrees/*) root="${cwd%%/.worktrees/*}" ;; esac
if [ -z "$root" ]; then
  d="$cwd"
  while [ "$d" != "/" ]; do [ -d "$d/.worktrees" ] && { root="$d"; break; }; d="$(dirname "$d")"; done
fi
echo "${root:-NO_ROOT}"
```

`NO_ROOT` → tell the user to run it from inside the workspace folder or one of its worktrees, and
stop. Never guess a path and never ask a skill for one.

## Worktree helpers (used by every handled-here subcommand)

`<sadensmol-task-skill-dir>` is this skill's base directory ("Base directory for this
skill" when it loads). Shell state does not survive between commands — inline these in
each command that needs them.

```bash
cwd="$(pwd)"
case "$cwd" in
  */.worktrees/*)
    root="${cwd%%/.worktrees/*}"; branch="${cwd#*/.worktrees/}"; branch="${branch%%/*}"; b="$branch"
    base="$root/.worktrees/$branch"; WT="$base" ;;
  *) echo "NOT_IN_WORKTREE" ;;
esac
```

**The "real work" filter.** `task new` refreshes dependency pins in every repo of a
worktree, so counting those would mark an untouched repo as the one being worked on.
`real_changes <repo>` prints the changed paths that are real work:

```bash
real_changes() {
  git -C "$1" status --porcelain -uall 2>/dev/null | while IFS= read -r line; do
    f="${line:3}"
    case "$f" in
      go.mod|go.sum|*/go.mod|*/go.sum|pubspec.lock|*/pubspec.lock) ;;   # dep pins from `task new`
      local.properties|*/local.properties|.DS_Store|*/.DS_Store) ;;     # generated / OS
      .claude/*|*/.claude/*) ;;                                         # harness config
      *) printf '%s\n' "$f" ;;
    esac
  done
}
```
