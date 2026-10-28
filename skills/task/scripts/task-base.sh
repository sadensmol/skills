# shellcheck shell=bash
# task-base.sh — the per-repo base record of a task worktree. Source it:
#
#   . <sadensmol-task-skill-dir>/scripts/task-base.sh
#
# <worktree-base>/.task-base sits at the worktree base, outside every repo, so it is
# never committed. It is git-config format, one section per repo of the worktree:
#
#   [repo "<repo>"]
#       default   = main         # the repo's default branch
#       base      = <branch>     # requested base branch of a stacked task; absent → none
#       stacked   = true         # the base exists on origin and the branch follows it
#       upstream  = <branch>     # what the branch follows and its PR targets: base when stacked, else default
#       forkPoint = <sha>        # the origin/<upstream> commit the branch sits on; tb_sync moves it
#       baseTask  = <KEY>        # tracker key of the base task (optional)
#       basePr    = <url>        # the base branch's PR in this repo (optional)
#       pr        = <url>        # this task's own PR in this repo, set by `finish`
#
# forkPoint is what makes a stacked rebase safe: when the base is force-pushed, or
# squash-merged and deleted, `rebase --onto <new tip> <forkPoint>` replays only this
# task's commits, never the base task's.
#
# Every function takes the worktree base first. Getters print nothing for a missing
# key and always return 0, so they are safe in `x="$(…)"` under `set -e`.
# Bash 3.2 compatible (macOS).

tb_get() {   # <wb> <repo> <key>
  [ -f "$1/.task-base" ] || return 0
  git config -f "$1/.task-base" --get "repo.$2.$3" 2>/dev/null || true
}

tb_set() {   # <wb> <repo> <key> <value>
  git config -f "$1/.task-base" "repo.$2.$3" "$4"
}

tb_unset() { # <wb> <repo> <key>
  [ -f "$1/.task-base" ] || return 0
  git config -f "$1/.task-base" --unset-all "repo.$2.$3" 2>/dev/null || true
}

tb_repos() { # <wb> → every repo the record holds, one per line
  [ -f "$1/.task-base" ] || return 0
  git config -f "$1/.task-base" --name-only --get-regexp '^repo\.' 2>/dev/null \
    | sed -E 's/^repo\.(.*)\.[^.]+$/\1/' | awk '!seen[$0]++' || true
}

# The `--base` value for one repo: its `<repo>:<branch>`, else the first bare
# `<branch>`, else nothing.
tb_pick_base() { # <repo> <line>...
  local r="$1" l bare=""
  shift
  for l in "$@"; do
    case "$l" in
      "$r":*) printf '%s\n' "${l#*:}"; return 0 ;;
      *:*) ;;
      *) [ -n "$bare" ] || bare="$l" ;;
    esac
  done
  if [ -n "$bare" ]; then printf '%s\n' "$bare"; fi
}

# Rewrite an old one-line-per-base file (`<branch>` / `<repo>:<branch>`) as sections.
# forkPoint comes from the base's remote-tracking ref while it is still there.
tb_migrate() { # <wb>
  local wb="$1" f="$1/.task-base" l d r b def ref
  local lines=()
  [ -f "$f" ] || return 0
  if grep -q '^\[' "$f"; then return 0; fi
  while IFS= read -r l; do [ -n "$l" ] && lines+=("$l"); done < "$f"
  rm -f "$f"
  for d in "$wb"/*/; do
    git -C "$d" rev-parse --git-dir >/dev/null 2>&1 || continue
    r="$(basename "$d")"
    def="$(git -C "$d" symbolic-ref -q --short refs/remotes/origin/HEAD 2>/dev/null | sed 's|^origin/||')"
    tb_set "$wb" "$r" default "${def:-main}"
    b="$(tb_pick_base "$r" ${lines[@]+"${lines[@]}"})"
    ref="origin/${def:-main}"
    if [ -n "$b" ]; then
      tb_set "$wb" "$r" base "$b"
      if git -C "$d" show-ref --verify --quiet "refs/remotes/origin/$b"; then ref="origin/$b"; fi
    fi
    if b="$(git -C "$d" merge-base "$ref" HEAD 2>/dev/null)"; then tb_set "$wb" "$r" forkPoint "$b"; fi
  done
  echo "Migrated ${f} to the per-repo format"
}

# Resolve one repo against origin: record `default`, decide `stacked` and `upstream`
# from the requested `base`, fetch the upstream. Prints the upstream branch. A base
# that is gone from origin (merged and deleted) turns the repo back to its default
# branch; the next tb_sync re-anchors it through forkPoint. Offline → the local
# remote-tracking ref decides.
tb_plan() { # <wb> <repo> <repo-or-worktree-dir> [<default-branch>]
  local wb="$1" r="$2" d="$3" def="${4:-}" b was heads up
  [ -n "$def" ] || def="$(tb_get "$wb" "$r" default)"
  [ -n "$def" ] || def="$(git -C "$d" symbolic-ref -q --short refs/remotes/origin/HEAD 2>/dev/null | sed 's|^origin/||')"
  [ -n "$def" ] || def=main
  tb_set "$wb" "$r" default "$def"
  b="$(tb_get "$wb" "$r" base)"
  was="$(tb_get "$wb" "$r" stacked)"
  up="$def"
  if [ -n "$b" ]; then
    if heads="$(git -C "$d" ls-remote --heads origin "$b" 2>/dev/null)"; then
      if [ -n "$heads" ]; then
        git -C "$d" fetch origin "+refs/heads/$b:refs/remotes/origin/$b" >/dev/null 2>&1 || true
        up="$b"
      elif [ "$was" = true ]; then
        echo "NOTE: $r: origin/$b is gone (base merged?) — re-anchoring onto origin/$def" >&2
        tb_unset "$wb" "$r" base
      else
        echo "NOTE: $r: no origin/$b — using origin/$def" >&2
      fi
    elif git -C "$d" show-ref --verify --quiet "refs/remotes/origin/$b"; then
      up="$b"   # offline: trust the local remote-tracking ref
    fi
  fi
  if [ "$up" = "$def" ]; then tb_set "$wb" "$r" stacked false; else tb_set "$wb" "$r" stacked true; fi
  tb_set "$wb" "$r" upstream "$up"
  git -C "$d" fetch origin "$up" >/dev/null 2>&1 || true
  printf '%s\n' "$up"
}

# Record where a freshly created (or re-attached) branch sits on its upstream.
tb_mark_fork() { # <wb> <repo> <worktree>
  local up fp
  up="$(tb_get "$1" "$2" upstream)"
  [ -n "$up" ] || return 0
  if fp="$(git -C "$3" merge-base "origin/$up" HEAD 2>/dev/null)"; then tb_set "$1" "$2" forkPoint "$fp"; fi
}

# Rebase the task branch onto the current origin/<upstream> tip, replaying only the
# commits after forkPoint, then move forkPoint to that tip. Needs a clean tree (stash
# first). A conflict aborts the rebase and returns 1 with the branch unchanged.
tb_sync() { # <wb> <repo> <worktree>
  local wb="$1" r="$2" wt="$3" up fp tip
  up="$(tb_get "$wb" "$r" upstream)"
  [ -n "$up" ] || return 1
  git -C "$wt" fetch origin "+refs/heads/$up:refs/remotes/origin/$up" >/dev/null 2>&1 || true
  tip="$(git -C "$wt" rev-parse -q --verify "origin/$up^{commit}" 2>/dev/null)" || return 1
  fp="$(tb_get "$wb" "$r" forkPoint)"
  if [ -z "$fp" ] || ! git -C "$wt" merge-base --is-ancestor "$fp" HEAD 2>/dev/null; then
    fp="$(git -C "$wt" merge-base "$tip" HEAD 2>/dev/null)" || return 1
  fi
  if git -C "$wt" rebase --onto "$tip" "$fp" >/dev/null 2>&1; then
    tb_set "$wb" "$r" forkPoint "$tip"
  else
    git -C "$wt" rebase --abort >/dev/null 2>&1 || true
    echo "WARN: $r: rebase onto origin/$up failed (conflict?) — branch left as it was" >&2
    return 1
  fi
}

# The base merged → follow the default branch again; forkPoint keeps the base's
# commits out of the replay.
tb_reanchor() { # <wb> <repo> <worktree>
  tb_set "$1" "$2" stacked false
  tb_set "$1" "$2" upstream "$(tb_get "$1" "$2" default)"
  tb_sync "$@"
}

# The commit this task's own changes start from: forkPoint in a stacked repo, else
# the merge-base with origin/<upstream> (a merge of the default branch keeps it right).
tb_diff_base() { # <wb> <repo> <worktree>
  local up fp
  up="$(tb_get "$1" "$2" upstream)"
  fp="$(tb_get "$1" "$2" forkPoint)"
  if [ "$(tb_get "$1" "$2" stacked)" = true ] && [ -n "$fp" ] \
     && git -C "$3" merge-base --is-ancestor "$fp" HEAD 2>/dev/null; then
    printf '%s\n' "$fp"; return 0
  fi
  [ -n "$up" ] || up="$(git -C "$3" symbolic-ref -q --short refs/remotes/origin/HEAD 2>/dev/null | sed 's|^origin/||')"
  git -C "$3" merge-base "origin/${up:-main}" HEAD 2>/dev/null || true
}

tb_show() { # <wb> → one line per repo
  local r fp
  for r in $(tb_repos "$1"); do
    fp="$(tb_get "$1" "$r" forkPoint)"
    printf '%s: upstream=%s stacked=%s forkPoint=%s basePr=%s pr=%s\n' "$r" \
      "$(tb_get "$1" "$r" upstream)" "$(tb_get "$1" "$r" stacked)" "${fp:0:10}" \
      "$(tb_get "$1" "$r" basePr)" "$(tb_get "$1" "$r" pr)"
  done
}
