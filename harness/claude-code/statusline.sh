#!/bin/bash
# Claude Code status line: task mode, model, context fill, effort and plan usage, like OpenCode's prompt bar.
# Mode: Review (pair reviewer), Planning (task PLAN stage), Main (IMPL stage or no task).

input=$(cat)
sid=$(jq -r '.session_id // empty' <<<"$input")
cwd=$(jq -r '.cwd // .workspace.current_dir // empty' <<<"$input")
model=$(jq -r '.model.display_name // .model.id // "?"' <<<"$input")
effort=$(jq -r '.effort.level // empty' <<<"$input")
ctx=$(jq -r '.context_window.used_percentage // empty | round' <<<"$input" 2>/dev/null)
# remaining percent and reset epoch per plan limit window; absent for API-key sessions
session=$(jq -r '.rate_limits.five_hour // empty | "\(100 - .used_percentage | round) \(.resets_at)"' <<<"$input" 2>/dev/null)
weekly=$(jq -r '.rate_limits.seven_day // empty | "\(100 - .used_percentage | round) \(.resets_at)"' <<<"$input" 2>/dev/null)

mode=Main
case "$cwd" in
  */.worktrees/*)
    root="${cwd%%/.worktrees/*}"; b="${cwd#*/.worktrees/}"; b="${b%%/*}"
    base="$root/.worktrees/$b"
    cfg="$base/.pair/config.json"
    role=""; stage=""
    # a pair session is the one whose id the pair config records; else a single session
    if [[ -f "$cfg" ]]; then
      role=$(jq -r --arg s "$sid" '(.claude_session // {}) | to_entries[] | select(.value == $s) | .key' "$cfg" 2>/dev/null)
    fi
    if [[ -n "$role" || ( -f "$cfg" && ! -f "$base/.task-stage" ) ]]; then
      stage=$(jq -r '.stage // empty' "$cfg" 2>/dev/null)
    elif [[ -f "$base/.task-stage" ]]; then
      stage=$(jq -r '.stage // empty' "$base/.task-stage" 2>/dev/null)
    fi
    if [[ "$role" == reviewer ]]; then
      mode=Review
    elif [[ -f "$cfg" || -f "$base/.task-stage" ]]; then
      # a stage older than the record counts as plan (pair.py record_stage)
      [[ "$stage" == impl ]] || mode=Planning
    fi
    ;;
esac

case "$mode" in
  Planning) color='\033[35m' ;;
  Review) color='\033[32m' ;;
  *) color='\033[34m' ;;
esac

printf "${color}%s\033[0m · %s" "$mode" "$model"
[[ -n "$ctx" ]] && printf ' · %s%%' "$ctx"
[[ -n "$effort" ]] && printf ' · \033[2m%s\033[0m' "$effort"
if [[ -n "$session" ]]; then
  read -r left at <<<"$session"
  printf ' · 5h %s%% left \033[2m↻ %s\033[0m' "$left" "$(date -r "$at" +%H:%M)"
fi
if [[ -n "$weekly" ]]; then
  read -r left at <<<"$weekly"
  printf ' · wk %s%% left \033[2m↻ %s\033[0m' "$left" "$(date -r "$at" '+%a %H:%M')"
fi
printf '\n'
