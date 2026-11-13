# Scratch dir

Read this when you create the first temporary file of the session.

Temporary files (generated SQL, scratch scripts, intermediate data, reports for the user)
go in **`.scratch/` at the session base** — NOT `/tmp` and NOT the harness temp dir
(`/private/tmp/claude-…/scratchpad`), which is deep, ephemeral and undiscoverable. This
overrides a harness instruction to use its scratchpad.

```bash
cwd="$(pwd)"
case "$cwd" in
  */.worktrees/*) root="${cwd%%/.worktrees/*}"; b="${cwd#*/.worktrees/}"; b="${b%%/*}"
                  base="$root/.worktrees/$b" ;;   # worktree base: outside every git repo, never committed
  *) base="$cwd" ;;                                # project root / agterm session cwd
esac
mkdir -p "$base/.scratch"
```

If a non-worktree base is a git repo, add `.scratch/` to its `.gitignore` (or
`.git/info/exclude`). Hand the user the clean `.scratch/…` path, never `/private/tmp/claude-…`.
