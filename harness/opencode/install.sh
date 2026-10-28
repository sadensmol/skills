#!/bin/sh
# OpenCode wiring for this repository.
#
# OpenCode has no hook, so the router and this harness's runtime file arrive
# through the `instructions` list in the global config. This script owns exactly
# those two keys — `instructions` and `skills.paths` — the same ground the Claude
# Code plugin covers with its UserPromptSubmit hook. Everything else in the
# config (models, agents, permissions, mcp) is yours and is left untouched unless
# --workspace-mcp explicitly registers a workspace-selected server; it
# only warns when an agent the task skill starts (`planning`, `main`) is missing.
#
# It also registers this repository's TUI plugin (tui/codex-resets: the Codex
# limit reset countdown beside the prompt) in the `plugin` list of tui.json,
# keeping every other plugin there.
#
#   usage: harness/opencode/install.sh [--dry-run] [--workspace-mcp <name>]...
#
# Nothing here is specific to a machine or a project: the repository root comes
# from this script's own location and the namespace from the marketplace
# manifest, so a stranger's clone wires itself up the same way.
set -eu

DRY_RUN=0

command -v jq >/dev/null 2>&1 || { echo "jq is required" >&2; exit 1; }

WORKSPACE_MCPS='[]'
while [ "$#" -gt 0 ]; do
  case "$1" in
    --dry-run) DRY_RUN=1; shift ;;
    --workspace-mcp)
      [ "$#" -ge 2 ] || { echo "--workspace-mcp needs a server name" >&2; exit 1; }
      printf '%s' "$2" | LC_ALL=C grep -Eq '^[a-z0-9][a-z0-9_-]*$' \
        || { echo "invalid MCP server name: $2" >&2; exit 1; }
      WORKSPACE_MCPS="$(printf '%s' "$WORKSPACE_MCPS" | jq --arg name "$2" '. + [$name] | unique')"
      shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 1 ;;
  esac
done

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
TUI="$CONFIG_DIR/tui.json"
PLUGIN="$REPO/harness/opencode/tui/codex-resets"

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
  --arg repo "$REPO" \
  --argjson servers "$WORKSPACE_MCPS" \
  '(.["$schema"] //= $schema)
   | .skills.paths = ((.skills.paths // []) + [$skills] | unique)
   | .instructions = (
       ((.instructions // [])
         | map(select(startswith($harness) | not))
         | map(select(. != $router)))
        + [$router, $runtime])
   | reduce $servers[] as $name (.; .mcp[$name] = {
       type: "local", command: ["python3", ($repo + "/harness/opencode/workspace-mcp.py"), $name], enabled: true
     })' \
  "$CONFIG")"

# The TUI plugin: drop entries pointing into this repository's harness/ (an
# earlier location), keep every other plugin, then append ours. A clone without
# the plugin directory skips this part.
TUI_MERGED=""
if [ -f "$PLUGIN/index.js" ]; then
  if [ -f "$TUI" ]; then
    jq empty "$TUI" >/dev/null 2>&1 || { echo "$TUI is not parseable as JSON — merge by hand" >&2; exit 1; }
    TUI_IN="$(cat "$TUI")"
  else
    TUI_IN='{}'
  fi
  TUI_MERGED="$(printf '%s' "$TUI_IN" | jq \
    --arg schema "https://opencode.ai/tui.json" \
    --arg harness "file://$REPO/harness/" \
    --arg plugin "file://$PLUGIN" \
    '(.["$schema"] //= $schema)
     | .plugin = (((.plugin // []) | map(select((type == "string" and startswith($harness)) | not)))
                  + [$plugin])')"
fi

if [ "$DRY_RUN" -eq 1 ]; then
  printf '%s\n' "$MERGED"
  [ -n "$TUI_MERGED" ] && echo "would register file://$PLUGIN in $TUI" >&2
  exit 0
fi

cp -p "$CONFIG" "$CONFIG.bak"
printf '%s\n' "$MERGED" > "$CONFIG.tmp"
mv "$CONFIG.tmp" "$CONFIG"

if [ -n "$TUI_MERGED" ]; then
  [ -f "$TUI" ] && cp -p "$TUI" "$TUI.bak"
  printf '%s\n' "$TUI_MERGED" > "$TUI.tmp"
  mv "$TUI.tmp" "$TUI"
  echo "registered the TUI plugin in $TUI: file://$PLUGIN"
fi

# The task skill starts its sessions on these agents (opencode.md); their models
# are yours to choose, so a missing one is reported, never written.
for agent in planning main; do
  jq -e --arg a "$agent" '.agent[$a]' "$CONFIG" >/dev/null 2>&1 \
    || echo "note: no \"$agent\" agent in $CONFIG — the task skill needs it (harness/opencode/opencode.md)" >&2
done

echo "wired $NS into $CONFIG (backup: $CONFIG.bak)"
jq -r '"  skills.paths: \(.skills.paths | join(", "))",
       "  instructions:", (.instructions[] | "    \(.)")' "$CONFIG"
