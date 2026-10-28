#!/bin/sh
# Claude Code wiring for this repository that the plugin cannot do.
#
# The plugin brings the hooks, but a plugin cannot set the status lines, so this
# script points two keys of the user settings at the scripts beside it:
#
#   statusLine          statusline.sh: the task mode (Planning, Main, Review),
#                       the model and the effort
#   subagentStatusLine  subagent-statusline.sh: each subagent row labelled by
#                       its tier (Runner, Planner, Main) and model
#
# It owns exactly those two keys; every other key in settings.json is yours and
# is left untouched.
#
#   usage: harness/claude-code/install.sh [--dry-run]
#
# Nothing here is specific to a machine: the scripts are found from this file's
# own location, so a stranger's clone wires itself up the same way.
set -eu

DRY_RUN=0
[ "${1:-}" = "--dry-run" ] && DRY_RUN=1

command -v jq >/dev/null 2>&1 || { echo "jq is required" >&2; exit 1; }

# Resolve through the symlink, not along it: this file may be reached from an
# install location that is itself a link back into the clone.
DIR="$(cd "$(dirname "$0")" && pwd -P)"
STATUS="$DIR/statusline.sh"
SUBAGENT="$DIR/subagent-statusline.sh"
for f in "$STATUS" "$SUBAGENT"; do
  [ -x "$f" ] || { echo "missing or not executable: $f" >&2; exit 1; }
done

CONFIG_DIR="${CLAUDE_CONFIG_DIR:-$HOME/.claude}"
CONFIG="$CONFIG_DIR/settings.json"

if [ -f "$CONFIG" ]; then
  jq empty "$CONFIG" >/dev/null 2>&1 || {
    echo "$CONFIG is not parseable as JSON — merge by hand" >&2
    exit 1
  }
else
  mkdir -p "$CONFIG_DIR"
  echo '{}' > "$CONFIG"
fi

MERGED="$(jq --arg status "$STATUS" --arg subagent "$SUBAGENT" \
  '.statusLine = {type: "command", command: $status}
   | .subagentStatusLine = {type: "command", command: $subagent}' "$CONFIG")"

if [ "$DRY_RUN" -eq 1 ]; then
  printf '%s\n' "$MERGED"
  exit 0
fi

cp -p "$CONFIG" "$CONFIG.bak"
printf '%s\n' "$MERGED" > "$CONFIG.tmp"
mv "$CONFIG.tmp" "$CONFIG"

echo "wired the status lines into $CONFIG (backup: $CONFIG.bak)"
jq -r '"  statusLine: \(.statusLine.command)",
       "  subagentStatusLine: \(.subagentStatusLine.command)"' "$CONFIG"
