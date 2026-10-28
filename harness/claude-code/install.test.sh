#!/usr/bin/env bash
# Checks for install.sh (the settings merge) and the two status line scripts.
#
# Hermetic: every case runs against a throwaway HOME or throwaway worktree
# bases, so nothing here reads or writes the real configuration. Only jq.
#
#   usage: harness/claude-code/install.test.sh
set -uo pipefail

DIR="$(cd "$(dirname "$0")" && pwd -P)"
INSTALL="$DIR/install.sh"
STATUS="$DIR/statusline.sh"
SUBAGENT="$DIR/subagent-statusline.sh"

pass=0; fail=0
grn=$'\033[32m'; red=$'\033[31m'; dim=$'\033[2m'; rst=$'\033[0m'
ok()  { printf '  %s✓%s %s\n' "$grn" "$rst" "$1"; pass=$((pass+1)); }
bad() { printf '  %s✗%s %s\n' "$red" "$rst" "$1"; [ $# -gt 1 ] && printf '      %s%s%s\n' "$dim" "$2" "$rst"; fail=$((fail+1)); }

# eq <got> <want> <description>
eq() { [ "$1" = "$2" ] && ok "$3" || bad "$3" "got: $1 | want: $2"; }

# The status line without its colours, so the checks read the words.
plain() { sed $'s/\033\\[[0-9;]*m//g'; }

printf '\n%s\n' "install.sh"

# --- 1. no settings at all ----------------------------------------------------
h="$(mktemp -d)"
out="$(HOME="$h" CLAUDE_CONFIG_DIR= "$INSTALL" 2>&1)"; rc=$?
cfg="$h/.claude/settings.json"
eq "$rc" "0" "fresh machine: exits 0"
eq "$(jq -r .statusLine.command "$cfg" 2>/dev/null)" "$STATUS" "fresh machine: statusLine points at the repo script"
eq "$(jq -r .subagentStatusLine.command "$cfg" 2>/dev/null)" "$SUBAGENT" "fresh machine: subagentStatusLine points at the repo script"

# --- 2. an existing config keeps everything else ------------------------------
h="$(mktemp -d)"; mkdir -p "$h/.claude"; cfg="$h/.claude/settings.json"
cat > "$cfg" <<JSON
{
  "model": "opus",
  "statusLine": { "type": "command", "command": "~/old-statusline.sh", "refreshInterval": 60 },
  "permissions": { "allow": ["Bash(git status)"] },
  "hooks": { "Stop": [{ "hooks": [{ "type": "command", "command": "true" }] }] }
}
JSON
before="$(jq -S 'del(.statusLine, .subagentStatusLine)' "$cfg")"
HOME="$h" CLAUDE_CONFIG_DIR= "$INSTALL" >/dev/null 2>&1
eq "$(jq -S 'del(.statusLine, .subagentStatusLine)' "$cfg")" "$before" "existing config: other keys untouched"
eq "$(jq -c .statusLine "$cfg")" "{\"type\":\"command\",\"command\":\"$STATUS\"}" "existing config: an old statusLine is replaced"
[ -f "$cfg.bak" ] && ok "existing config: keeps a .bak" || bad "existing config: keeps a .bak"

# --- 3. idempotent, dry run, refusal -----------------------------------------
first="$(cat "$cfg")"
HOME="$h" CLAUDE_CONFIG_DIR= "$INSTALL" >/dev/null 2>&1
eq "$(cat "$cfg")" "$first" "second run: no change"
h="$(mktemp -d)"; mkdir -p "$h/.claude"; echo '{"model":"opus"}' > "$h/.claude/settings.json"
HOME="$h" CLAUDE_CONFIG_DIR= "$INSTALL" --dry-run >/dev/null 2>&1
eq "$(cat "$h/.claude/settings.json")" '{"model":"opus"}' "dry run: writes nothing"
echo '{ broken' > "$h/.claude/settings.json"
HOME="$h" CLAUDE_CONFIG_DIR= "$INSTALL" >/dev/null 2>&1; rc=$?
eq "$rc" "1" "unparseable settings: refused"

printf '\n%s\n' "statusline.sh"

# line <session id> <cwd> <model> <effort> [context used %]
line() {
  jq -nc --arg s "$1" --arg c "$2" --arg m "$3" --arg e "$4" --arg p "${5:-}" \
    '{session_id: $s, cwd: $c, model: {display_name: $m}} + (if $e == "" then {} else {effort: {level: $e}} end)
     + (if $p == "" then {} else {context_window: {used_percentage: ($p | tonumber)}} end)' \
    | "$STATUS" | plain
}
w="$(mktemp -d)"
mkdir -p "$w/.worktrees/pair/.pair" "$w/.worktrees/old/.pair" "$w/.worktrees/single/repo" "$w/.worktrees/none"
echo '{"stage":"impl","claude_session":{"writer":"w1","reviewer":"r1"}}' > "$w/.worktrees/pair/.pair/config.json"
echo '{"claude_session":{"writer":"w2"}}' > "$w/.worktrees/old/.pair/config.json"
echo '{"stage":"plan"}' > "$w/.worktrees/single/.task-stage"

eq "$(line x /tmp "Opus 5.5" high)" "Main · Opus 5.5 · high" "no task: Main"
eq "$(line w1 "$w/.worktrees/pair" "Opus 5.5" high)" "Main · Opus 5.5 · high" "pair writer in IMPL: Main"
eq "$(line r1 "$w/.worktrees/pair" "Opus 5.5" high)" "Review · Opus 5.5 · high" "pair reviewer: Review"
eq "$(line w2 "$w/.worktrees/old" "Fable 5.1" high)" "Planning · Fable 5.1 · high" "pair without a recorded stage: Planning"
eq "$(line z "$w/.worktrees/single/repo" "Fable 5.1" "")" "Planning · Fable 5.1" "single session in PLAN, from a repo below the base: Planning"
eq "$(line z "$w/.worktrees/none" "Opus 5.5" high)" "Main · Opus 5.5 · high" "worktree without a task record: Main"
eq "$(line x /tmp "Opus 5.5" high 41.6)" "Main · Opus 5.5 · 42% · high" "context fill: rounded, after the model"
eq "$(line x /tmp "Fable 5.1" "" 7)" "Main · Fable 5.1 · 7%" "context fill without effort"

printf '\n%s\n' "subagent-statusline.sh"

rows="$(echo '{"columns":200,"tasks":[
  {"id":"a","model":"claude-sonnet-5-5","effort":"high","label":"run tests api"},
  {"id":"b","model":"claude-fable-5-1","label":"plan ENG-1"},
  {"id":"c","agentType":"general-purpose","description":"impl x"}]}' | "$SUBAGENT")"
eq "$(jq -r 'select(.id=="a") | .content' <<<"$rows" | plain)" "Runner · Sonnet 5.5 · high · run tests api" "sonnet row: Runner"
eq "$(jq -r 'select(.id=="b") | .content' <<<"$rows" | plain)" "Planner · Fable 5.1 · plan ENG-1" "fable row: Planner"
eq "$(jq -r 'select(.id=="c") | .content' <<<"$rows" | plain)" "Main · impl x" "row without a model: Main, falls back to the description"
eq "$(jq -s length <<<"$rows")" "3" "one JSON line per task"

printf '\n  %d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
