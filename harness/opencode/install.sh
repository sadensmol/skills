#!/bin/sh
# OpenCode wiring for this repository.
#
# OpenCode has no hook, so the router and this harness's runtime file arrive
# through the `instructions` list in the global config. This script owns exactly
# those two keys — `instructions` and `skills.paths` — the same ground the Claude
# Code plugin covers with its UserPromptSubmit hook. Everything else in the
# config (models, agents, permissions, mcp) is yours and is left untouched.
#
#   usage: harness/opencode/install.sh [--dry-run]
#
# Nothing here is specific to a machine or a project: the repository root comes
# from this script's own location and the namespace from the marketplace
# manifest, so a stranger's clone wires itself up the same way.
set -eu

DRY_RUN=0
[ "${1:-}" = "--dry-run" ] && DRY_RUN=1

command -v jq >/dev/null 2>&1 || { echo "jq is required" >&2; exit 1; }

# Resolve through the symlink, not along it: this file may be reached from an
# install location that is itself a link back into the clone.
REPO="$(cd "$(dirname "$0")/../.." && pwd -P)"
NS="$(jq -r .name "$REPO/.claude-plugin/marketplace.json")"
[ -n "$NS" ] && [ "$NS" != "null" ] || { echo "no namespace in .claude-plugin/marketplace.json" >&2; exit 1; }

SKILLS="$HOME/.agents/skills"
ROUTER="$SKILLS/$NS/router/SKILL.md"
RUNTIME="$REPO/harness/opencode/opencode.md"

[ -f "$RUNTIME" ] || { echo "missing $RUNTIME" >&2; exit 1; }
[ -f "$ROUTER" ] || echo "note: $ROUTER does not exist yet — link the skills first (see README)" >&2

# Use whichever config OpenCode already reads. Writing the other spelling would
# leave two configs behind, and only one of them would win.
CONFIG_DIR="$HOME/.config/opencode"
for candidate in "$CONFIG_DIR/opencode.jsonc" "$CONFIG_DIR/opencode.json"; do
  if [ -f "$candidate" ]; then
    CONFIG="$candidate"
    break
  fi
done
CONFIG="${CONFIG:-$CONFIG_DIR/opencode.json}"

if [ -f "$CONFIG" ]; then
  jq empty "$CONFIG" >/dev/null 2>&1 || {
    echo "$CONFIG is not parseable as JSON (comments in a .jsonc file?) — merge by hand" >&2
    exit 1
  }
else
  mkdir -p "$CONFIG_DIR"
  echo '{}' > "$CONFIG"
fi

# Drop any stale entry pointing into this repository's harness/ (an earlier
# location for the runtime file), keep every other instruction — the other
# namespaces' routers live there too — then append ours in order.
MERGED="$(jq \
  --arg schema "https://opencode.ai/config.json" \
  --arg skills "$SKILLS" \
  --arg harness "$REPO/harness/" \
  --arg router "$ROUTER" \
  --arg runtime "$RUNTIME" \
  '(.["$schema"] //= $schema)
   | .skills.paths = ((.skills.paths // []) + [$skills] | unique)
   | .instructions = (
       ((.instructions // [])
         | map(select(startswith($harness) | not))
         | map(select(. != $router)))
       + [$router, $runtime])' \
  "$CONFIG")"

if [ "$DRY_RUN" -eq 1 ]; then
  printf '%s\n' "$MERGED"
  exit 0
fi

cp -p "$CONFIG" "$CONFIG.bak"
printf '%s\n' "$MERGED" > "$CONFIG.tmp"
mv "$CONFIG.tmp" "$CONFIG"

echo "wired $NS into $CONFIG (backup: $CONFIG.bak)"
jq -r '"  skills.paths: \(.skills.paths | join(", "))",
       "  instructions:", (.instructions[] | "    \(.)")' "$CONFIG"
