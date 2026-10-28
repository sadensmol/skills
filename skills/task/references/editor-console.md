# `zed` / `code` / `console` — handled by the dispatcher

**Triggers:** "task zed" / "open zed"; "task code" / "open code" / "open vscode";
"task console" / "open console" / "open a console" / "new console".

All three start from the worktree base (`$root`, `$branch`, `$base`) and the
`real_changes` filter in SKILL.md → *Worktree helpers*. On `NOT_IN_WORKTREE`, say the
subcommand must run from inside a worktree, and stop.

## Pick the repo (shared by all three)

`zed`, `code` and `console` must resolve the same repo — they must never disagree
about which repo you are in.

```bash
# No arrays: agents run this under bash OR zsh, and zsh indexes arrays from 1.
first=""; count=0; nchanged=0; lastchanged=""; target=""; reponames=""; changednames=""
for d in "$base"/*/; do
  [ -d "$d" ] || continue
  count=$((count + 1)); [ -n "$first" ] || first="${d%/}"
  git -C "$d" rev-parse --git-dir >/dev/null 2>&1 || continue
  if [ -n "$(real_changes "$d")" ]; then
    nchanged=$((nchanged + 1)); lastchanged="${d%/}"; changednames="$changednames ${d%/}"
  fi
  reponames="$reponames ${d%/}"
done
[ "$nchanged" -eq 1 ] && target="$lastchanged"
echo "count=$count changed=$nchanged target=${target:-none}"; echo "REPOS:$reponames"; echo "CHANGED:$changednames"
```

`PLAN.md` at the base never counts (`*/` matches only directories).

- **Exactly one changed repo → open only that repo (MUST FOLLOW).** The base is not a
  git repo and buries the code one level down.
- **A single-repo worktree** (`count -eq 1`) → open that repo. **0 repos** → the base.
- **Ambiguous** (2+ repos, 0 or 2+ changed) → ask the user through `ask-user` which repo
  to open. Never open the bare base silently. List repos by `basename`, changed ones
  first, labelled `<repo> (changes)`, plus a final `open the base (all repos)` option.
  More than 4 repos: show the 4 best candidates (changed first) and let the user type
  "Other". Without an `ask-user` capability, ask the same choice in chat as a flat
  numbered list and wait.
- **`console` only:** when the cwd is already inside a repo under `$base`
  (`git -C "$cwd" rev-parse --show-toplevel` is `$base` or under it), skip the picker and
  use that repo.

## `code` (one-time VS Code trust)

VS Code trusts every subfolder of a trusted folder, so trusting `$root` once makes every
worktree open trusted (worktrees are siblings of the main repo, so trusting the repo is
not enough). A marker records it; `rm "$root/.vscode-parent-trusted"` re-prompts.

```bash
MARKER="$root/.vscode-parent-trusted"             # outside every repo — never committed
if [ ! -f "$MARKER" ]; then echo "NEEDS_TRUST"; code "$root"; else echo "TRUSTED"; fi
```

- `NEEDS_TRUST` → ask the user (`ask-user`) to click **"Yes, I trust the authors"** and
  confirm. Only after they confirm, `touch "$MARKER"`, then open the worktree.
- `TRUSTED` → open it straight away.

```bash
if [ -n "$target" ]; then code "$target"          # the changed repo, or the user's pick
elif [ "$count" -eq 1 ]; then code "$first"
else code "$base"; fi                             # only when there was nothing to ask
```

## `zed`

No trust step. The final fallback opens the top-level dirs as one multiroot project. The
`env -u` flags stop Zed's own Claude integration from conflicting:

```bash
if [ -n "$target" ]; then env -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT zed "$target"
elif [ "$count" -eq 1 ]; then env -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT zed "$first"
else env -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT zed "$base"/*/; fi
```

## `console`

Opens a **new agterm session** at the resolved repo (`proj`; the base when the user
picked `open the base`), named `$(basename "$proj")`. Each call opens a fresh session —
no select-existing logic.

```bash
if [ -n "${AGTERM_ENABLED:-}" ] && command -v agtermctl >/dev/null 2>&1; then
  agtermctl session new --cwd "$proj" --name "$name" --after "$AGTERM_SESSION_ID" --no-select
else
  echo "NOT_IN_AGTERM"
fi
```

`NOT_IN_AGTERM` → say `console` needs agterm, and stop.

**`--after "$AGTERM_SESSION_ID"` is required (MUST FOLLOW).** The anchor carries its own
workspace, so the console lands directly below the task session. Never pass
`--workspace`/`--workspace-name` instead, and never omit the anchor — a console at the
end of another workspace is lost. **`--no-select` is required too:** the console opens in
the background, and the user's focus never moves to it.
