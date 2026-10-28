#!/usr/bin/env bash
# Run plannotator in its own agterm session, placed directly below the calling
# session and named like a tree leaf: "└─ review <task id>". Blocks until
# plannotator exits, then prints its output and exits with its status, so an
# agent running this in a background shell gets the annotations back.
#
# Usage: bash plannotator-session.sh <plannotator args...>
#   e.g. bash plannotator-session.sh review
#        bash plannotator-session.sh annotate PLAN.md --gate
#        bash plannotator-session.sh --status   # RUNNING: <args> (session <id>) | NONE
#        bash plannotator-session.sh --close    # close this base's live instance
#
# Run it from the worktree (any depth): plannotator runs at the worktree base.
# Outside agterm it runs plannotator directly in the calling shell.
#
# One live instance per base (worktree): review and annotate share the slot, held by
# the lock dir <base>/.scratch/plannotator.lock. A launch while it is held prints
# PLANNOTATOR_ALREADY_RUNNING and exits 4. Other worktrees run their own in parallel.
set -u

[ $# -gt 0 ] || { echo "usage: $0 <plannotator args...> | --status | --close" >&2; exit 2; }

PLANNOTATOR="${PLANNOTATOR:-$(command -v plannotator || echo "$HOME/.local/bin/plannotator")}"
[ -x "$PLANNOTATOR" ] || { echo "PLANNOTATOR_NOT_FOUND"; exit 127; }

cwd="$(pwd)"
case "$cwd" in
  */.worktrees/*)
    root="${cwd%%/.worktrees/*}"
    branch="${cwd#*/.worktrees/}"; branch="${branch%%/*}"
    base="$root/.worktrees/$branch"
    ;;
  *) base="$cwd"; branch="$(basename "$cwd")" ;;
esac
task_id="$(printf '%s' "$branch" | grep -oiE '^[a-z]+-[0-9]+' | tr '[:lower:]' '[:upper:]')"
name="└─ review ${task_id:-$branch}"

lock="$base/.scratch/plannotator.lock"

# lock_held: 0 when a live instance owns the lock. The owner is alive while its helper
# pid runs or its review session is still in the agterm tree. A lock with no pid file
# yet is being taken right now, so it counts as held.
lock_held() {
  [ -d "$lock" ] || return 1
  [ -f "$lock/pid" ] || return 0
  kill -0 "$(cat "$lock/pid")" 2>/dev/null && return 0
  lsid="$(cat "$lock/sid" 2>/dev/null)"
  [ -n "$lsid" ] && agtermctl tree --json 2>/dev/null | grep -qi "\"$lsid\"" && return 0
  return 1
}

lock_info() {
  printf '%s (session %s)' "$(cat "$lock/args" 2>/dev/null)" "$(cat "$lock/sid" 2>/dev/null || echo none)"
}

case "$1" in
  --status)
    if lock_held; then echo "RUNNING: $(lock_info)"; else echo "NONE"; fi
    exit 0
    ;;
  --close)
    lock_held || { echo "NONE"; exit 0; }
    lsid="$(cat "$lock/sid" 2>/dev/null)"
    if [ -n "$lsid" ]; then
      agtermctl session close --target "$lsid" >/dev/null 2>&1
    else
      kill "$(cat "$lock/pid" 2>/dev/null)" 2>/dev/null   # outside agterm the pid is plannotator itself
    fi
    for _ in $(seq 20); do lock_held || break; sleep 0.5; done
    if lock_held; then echo "CLOSE_FAILED: $(lock_info)"; exit 1; fi
    rm -rf "$lock"
    echo "CLOSED"
    exit 0
    ;;
esac

mkdir -p "$base/.scratch"
if ! mkdir "$lock" 2>/dev/null; then
  if lock_held; then echo "PLANNOTATOR_ALREADY_RUNNING: $(lock_info)"; exit 4; fi
  rm -rf "$lock"   # stale: its owner is gone
  mkdir "$lock" 2>/dev/null || { echo "PLANNOTATOR_ALREADY_RUNNING: $(lock_info)"; exit 4; }
fi
printf '%s\n' "$*" > "$lock/args"
echo "$$" > "$lock/pid"
release_lock() { [ "$(cat "$lock/pid" 2>/dev/null)" = "$$" ] && rm -rf "$lock"; }
trap release_lock EXIT

if [ -z "${AGTERM_ENABLED:-}" ] || [ -z "${AGTERM_SESSION_ID:-}" ] || ! command -v agtermctl >/dev/null 2>&1; then
  # exec keeps this pid, so the lock follows plannotator and goes stale when it exits
  cd "$base" && exec "$PLANNOTATOR" "$@"
fi

# The review session does not inherit the caller's env, so plannotator would fall
# back to the default browser. Pass the overlay opener explicitly: the page opens as
# an agterm overlay inside the review session itself.
opener="${PLANNOTATOR_BROWSER:-$HOME/.local/bin/plannotator-agterm-open}"
[ -x "$opener" ] || { echo "PLANNOTATOR_OVERLAY_OPENER_NOT_FOUND: $opener"; exit 127; }

run="$(mktemp -d "${TMPDIR:-/tmp}/plannotator-session.XXXXXX")"
trap 'rm -rf "$run"; release_lock' EXIT

# The session runs this file; tee keeps the output visible there and gives the
# agent a copy. pipestatus[1] is plannotator's own exit status.
{
  printf 'export PLANNOTATOR_BROWSER=%q\n' "$opener"
  printf 'cd %q || exit 1\n' "$base"
  printf '%q' "$PLANNOTATOR"; printf ' %q' "$@"; printf ' 2>&1 | tee %q\n' "$run/out"
  printf 'echo "${pipestatus[1]}" > %q\n' "$run/rc"
} > "$run/run.zsh"

sid="$(agtermctl session new --cwd "$base" --name "$name" --after "$AGTERM_SESSION_ID" \
  --no-select --command "zsh -l $run/run.zsh")" || { echo "AGTERM_SESSION_FAILED" >&2; exit 1; }
echo "$sid" > "$lock/sid"
echo "plannotator is running in agterm session '$name' ($sid)"

while [ ! -f "$run/rc" ]; do
  if tree="$(agtermctl tree --json 2>/dev/null)" && ! printf '%s' "$tree" | grep -qi "\"$sid\""; then
    sleep 1
    [ -f "$run/rc" ] && break
    cat "$run/out" 2>/dev/null
    echo "REVIEW_SESSION_CLOSED: the session closed before plannotator finished"
    exit 3
  fi
  sleep 2
done

cat "$run/out"
rc="$(cat "$run/rc")"
# In a Live-sessions restore mode the session keeps a shell after the command
# exits, so close it explicitly; otherwise it is already gone.
agtermctl session close --target "$sid" >/dev/null 2>&1 || true
exit "${rc:-1}"
