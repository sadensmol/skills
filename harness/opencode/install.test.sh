#!/usr/bin/env bash
# Checks for install.sh, the OpenCode config merge.
#
# Hermetic: every case runs the installer against a throwaway HOME, so nothing
# here reads or writes the real configuration. No OpenCode, no network — only
# jq — which is why this runs on a bare machine as well as inside the container
# suite, which calls it.
#
#   usage: harness/opencode/install.test.sh
set -uo pipefail

REPO="$(cd "$(dirname "$0")/../.." && pwd -P)"
INSTALL="$REPO/harness/opencode/install.sh"
NS="$(jq -r .name "$REPO/.claude-plugin/marketplace.json")"
RUNTIME="$REPO/harness/opencode/opencode.md"

pass=0; fail=0
grn=$'\033[32m'; red=$'\033[31m'; dim=$'\033[2m'; rst=$'\033[0m'
ok()  { printf '  %s✓%s %s\n' "$grn" "$rst" "$1"; pass=$((pass+1)); }
bad() { printf '  %s✗%s %s\n' "$red" "$rst" "$1"; [ $# -gt 1 ] && printf '      %s%s%s\n' "$dim" "$2" "$rst"; fail=$((fail+1)); }

# eq <got> <want> <description>
eq() { [ "$1" = "$2" ] && ok "$3" || bad "$3" "got: $1 | want: $2"; }

# A fresh HOME per case. The router is a real file, so the installer does not
# warn about it; nothing else in the fake home matters.
new_home() {
  local h; h="$(mktemp -d)"
  mkdir -p "$h/.agents/skills/$NS/router" "$h/.config/opencode"
  echo "router" > "$h/.agents/skills/$NS/router/SKILL.md"
  echo "$h"
}
run() { HOME="$1" "$INSTALL" "${@:2}"; }              # exit status is the caller's business
ROUTER_OF() { echo "$1/.agents/skills/$NS/router/SKILL.md"; }

printf '\n%s\n' "install.sh"

# --- 1. no config at all ------------------------------------------------------
h="$(new_home)"; rm -f "$h/.config/opencode/"*.json*
out="$(run "$h" 2>&1)"; rc=$?
cfg="$h/.config/opencode/opencode.json"
eq "$rc" "0" "fresh machine: exits 0"
if [ -f "$cfg" ]; then
  ok "fresh machine: writes opencode.json"
  eq "$(jq -r '.["$schema"]' "$cfg")" "https://opencode.ai/config.json" "fresh machine: sets \$schema"
  eq "$(jq -r '.skills.paths | join(",")' "$cfg")" "$h/.agents/skills" "fresh machine: sets skills.paths"
  eq "$(jq -r '.instructions | join(",")' "$cfg")" "$(ROUTER_OF "$h"),$RUNTIME" "fresh machine: router then runtime"
else
  bad "fresh machine: writes opencode.json" "$out"
fi

# --- 2. an existing config keeps everything it already had --------------------
h="$(new_home)"; cfg="$h/.config/opencode/opencode.jsonc"
cat > "$cfg" <<JSON
{
  "instructions": ["/elsewhere/other-namespace/router/SKILL.md"],
  "default_agent": "main",
  "agent": { "main": { "model": "someprovider/some-model", "variant": "high" } },
  "permission": { "external_directory": { "*": "deny" } },
  "lsp": true,
  "mcp": { "example": { "type": "remote", "url": "https://example.com/mcp" } }
}
JSON
before="$(jq -S 'del(.instructions, .skills, .["$schema"])' "$cfg")"
run "$h" >/dev/null 2>&1
eq "$(jq -S 'del(.instructions, .skills, .["$schema"])' "$cfg")" "$before" "existing config: agent/permission/lsp/mcp untouched"
eq "$(jq -r '.instructions[0]' "$cfg")" "/elsewhere/other-namespace/router/SKILL.md" "existing config: a foreign instruction survives"
eq "$(jq -r '.instructions | length' "$cfg")" "3" "existing config: ours are appended, not substituted"
[ -f "$h/.config/opencode/opencode.json" ] \
  && bad "existing config: no second config file" "opencode.json was created beside opencode.jsonc" \
  || ok "existing config: edits the .jsonc it found, writes no .json"
[ -f "$cfg.bak" ] && ok "existing config: leaves a .bak" || bad "existing config: leaves a .bak"

# --- 3. an entry from an earlier location of the runtime file is replaced -----
h="$(new_home)"; cfg="$h/.config/opencode/opencode.json"
jq -n --arg stale "$REPO/harness/opencode.md" --arg keep "/elsewhere/router/SKILL.md" \
  '{instructions: [$keep, $stale]}' > "$cfg"
run "$h" >/dev/null 2>&1
eq "$(jq -r '.instructions | join(",")' "$cfg")" \
   "/elsewhere/router/SKILL.md,$(ROUTER_OF "$h"),$RUNTIME" "stale harness/ entry is dropped, not duplicated"

# --- 4. running it again changes nothing --------------------------------------
h="$(new_home)"
run "$h" >/dev/null 2>&1; cfg="$h/.config/opencode/opencode.json"; once="$(cat "$cfg")"
run "$h" >/dev/null 2>&1; run "$h" >/dev/null 2>&1
eq "$(cat "$cfg")" "$once" "idempotent: three runs, one result"

# --- 5. a .jsonc with real comments is refused, not mangled -------------------
h="$(new_home)"; cfg="$h/.config/opencode/opencode.jsonc"
printf '{\n  // a comment jq cannot parse\n  "lsp": true\n}\n' > "$cfg"
before="$(cat "$cfg")"
out="$(run "$h" 2>&1)"; rc=$?
eq "$rc" "1" "commented .jsonc: exits non-zero"
eq "$(cat "$cfg")" "$before" "commented .jsonc: file is left alone"
[ -f "$cfg.bak" ] && bad "commented .jsonc: writes no .bak" "a backup was made anyway" \
                  || ok "commented .jsonc: writes no .bak"

# --- 6. --dry-run prints and writes nothing ----------------------------------
h="$(new_home)"; cfg="$h/.config/opencode/opencode.json"
echo '{"lsp": true}' > "$cfg"; before="$(cat "$cfg")"
out="$(run "$h" --dry-run 2>/dev/null)"; rc=$?
eq "$rc" "0" "--dry-run: exits 0"
eq "$(printf '%s' "$out" | jq -r '.instructions | length')" "2" "--dry-run: prints the merged config"
eq "$(cat "$cfg")" "$before" "--dry-run: leaves the config unchanged"
[ -f "$cfg.bak" ] && bad "--dry-run: writes no .bak" "a backup was made anyway" || ok "--dry-run: writes no .bak"

# --- 7. a missing runtime file is an error, not a half-written config ---------
# A clone with the script but no harness/opencode/opencode.md beside it: the
# installer must refuse before it touches the config.
h="$(new_home)"; fake="$(cd "$(mktemp -d)" && pwd -P)"
mkdir -p "$fake/.claude-plugin" "$fake/harness/opencode"
echo '{"name":"someone-else"}' > "$fake/.claude-plugin/marketplace.json"
cp "$INSTALL" "$fake/harness/opencode/install.sh"
out="$(HOME="$h" "$fake/harness/opencode/install.sh" 2>&1)"; rc=$?
eq "$rc" "1" "missing runtime file: exits 1"
case "$out" in *"missing $fake/harness/opencode/opencode.md"*) ok "missing runtime file: says which file" ;;
               *) bad "missing runtime file: says which file" "$out" ;; esac
[ -f "$h/.config/opencode/opencode.json" ] \
  && bad "missing runtime file: writes no config" "a config was created anyway" \
  || ok "missing runtime file: writes no config"

# --- 8. the namespace comes from the manifest, not from a hardcoded name ------
h="$(new_home)"
mkdir -p "$h/.agents/skills/someone-else/router" && echo router > "$h/.agents/skills/someone-else/router/SKILL.md"
cp "$RUNTIME" "$fake/harness/opencode/opencode.md"
HOME="$h" "$fake/harness/opencode/install.sh" >/dev/null 2>&1
eq "$(jq -r '.instructions[0]' "$h/.config/opencode/opencode.json")" \
   "$h/.agents/skills/someone-else/router/SKILL.md" "namespace is read from marketplace.json"

# --- 9. both spellings present: edit the one OpenCode applies last -----------
# OpenCode reads opencode.json from the config directory and then merges
# opencode.jsonc over it, so the .jsonc is the file whose value wins.
h="$(new_home)"
echo '{"lsp": true}'  > "$h/.config/opencode/opencode.json"
echo '{"lsp": false}' > "$h/.config/opencode/opencode.jsonc"
run "$h" >/dev/null 2>&1
eq "$(jq -r '.instructions | length' "$h/.config/opencode/opencode.jsonc")" "2" "both spellings: the .jsonc gets the entries"
eq "$(cat "$h/.config/opencode/opencode.json")" '{"lsp": true}' "both spellings: the .json is left alone"

# --- 10. the TUI plugin is registered in tui.json, other plugins kept ---------
PLUGIN_URL="file://$REPO/harness/opencode/tui/codex-resets"
h="$(new_home)"; tui="$h/.config/opencode/tui.json"
jq -n --arg stale "file://$REPO/harness/opencode/tui/old-plugin" \
  '{theme: "system", plugin: ["file:///elsewhere/plugin", $stale]}' > "$tui"
run "$h" >/dev/null 2>&1
eq "$(jq -r '.plugin | join(",")' "$tui")" "file:///elsewhere/plugin,$PLUGIN_URL" \
   "tui.json: foreign plugin kept, stale harness/ entry dropped, ours appended"
eq "$(jq -r .theme "$tui")" "system" "tui.json: other keys untouched"
[ -f "$tui.bak" ] && ok "tui.json: leaves a .bak" || bad "tui.json: leaves a .bak"
run "$h" >/dev/null 2>&1
eq "$(jq -r '.plugin | length' "$tui")" "2" "tui.json: idempotent"

h="$(new_home)"; tui="$h/.config/opencode/tui.json"
run "$h" >/dev/null 2>&1
eq "$(jq -r '.plugin | join(",")' "$tui" 2>/dev/null)" "$PLUGIN_URL" "tui.json: created when missing"

h="$(new_home)"; tui="$h/.config/opencode/tui.json"; echo '{"theme": "system"}' > "$tui"
run "$h" --dry-run >/dev/null 2>&1
eq "$(cat "$tui")" '{"theme": "system"}' "tui.json: --dry-run leaves it unchanged"

h="$(new_home)"; tui="$h/.config/opencode/tui.json"; cfg="$h/.config/opencode/opencode.json"
echo '{ broken' > "$tui"; echo '{"lsp": true}' > "$cfg"
run "$h" >/dev/null 2>&1; rc=$?
eq "$rc" "1" "unparseable tui.json: exits non-zero"
eq "$(cat "$cfg")" '{"lsp": true}' "unparseable tui.json: opencode config not half-written"

# --- 11. missing task agents are reported, never written ----------------------
h="$(new_home)"; cfg="$h/.config/opencode/opencode.json"
echo '{"agent": {"main": {"model": "someprovider/some-model"}}}' > "$cfg"
out="$(run "$h" 2>&1)"
case "$out" in *'no "planning" agent'*) ok "agents: a missing planning agent is reported" ;;
               *) bad "agents: a missing planning agent is reported" "$out" ;; esac
case "$out" in *'no "main" agent'*) bad "agents: a present main agent is not reported" "$out" ;;
               *) ok "agents: a present main agent is not reported" ;; esac
eq "$(jq -c .agent "$cfg")" '{"main":{"model":"someprovider/some-model"}}' "agents: the agent block is not written"

# One MCP name delegates to workspace-specific settings.
h="$(new_home)"; cfg="$h/.config/opencode/opencode.json"
echo '{"model":"provider/model","mcp":{"existing":{"type":"remote","url":"https://existing.example/mcp"}}}' > "$cfg"
out="$(run "$h" --workspace-mcp tracker 2>&1)"; rc=$?
eq "$rc" "0" "workspace MCP: exits 0"
eq "$(jq -r '.mcp | keys | join(",")' "$cfg")" "existing,tracker" "workspace MCP: one entry, existing server kept"
eq "$(jq -r '.mcp.tracker.command | join(",")' "$cfg")" "python3,$REPO/harness/opencode/workspace-mcp.py,tracker" "workspace MCP: delegates to repository helper"
eq "$(jq -r .model "$cfg")" "provider/model" "workspace MCP: other settings kept"
once="$(cat "$cfg")"
run "$h" --workspace-mcp tracker >/dev/null 2>&1
eq "$(cat "$cfg")" "$once" "workspace MCP: idempotent"
out="$(run "$h" --workspace-mcp extra --dry-run 2>/dev/null)"
eq "$(printf '%s' "$out" | jq -r .mcp.extra.type)" "local" "workspace MCP: dry-run shows entry"
eq "$(cat "$cfg")" "$once" "workspace MCP: dry-run leaves config unchanged"
run "$h" --workspace-mcp '../invalid' >/dev/null 2>&1; rc=$?
eq "$rc" "1" "workspace MCP: invalid name refused"
eq "$(cat "$cfg")" "$once" "workspace MCP: invalid input leaves config unchanged"

# Fake workspaces prove that cwd never selects the account and stores stay apart.
mkdir -p "$h/bin" "$h/.agents/skills/alpha/tracker" "$h/.agents/skills/beta/tracker"
cat > "$h/bin/agtermctl" <<'SH'
#!/bin/sh
echo '{"result":{"tree":{"workspaces":[{"id":"a","name":"alpha"},{"id":"b","name":"beta"}]}}}'
SH
chmod +x "$h/bin/agtermctl"
for project in alpha beta; do
  jq -n --arg project "$project" '{mcp:{tracker:{type:"local",command:["sh","-c","printf '\''%s'\'' \"$MCP_REMOTE_CONFIG_DIR\""],environment:{MCP_REMOTE_CONFIG_DIR:("{env:HOME}/.mcp-auth/" + $project)}}}}' \
    > "$h/.agents/skills/$project/tracker/opencode.json"
done
out="$(HOME="$h" PATH="$h/bin:$PATH" AGTERM_WORKSPACE_ID=a python3 "$REPO/harness/opencode/workspace-mcp.py" tracker 2>/dev/null)"
eq "$out" "$h/.mcp-auth/alpha" "workspace MCP: first workspace selects its auth store"
out="$(HOME="$h" PATH="$h/bin:$PATH" AGTERM_WORKSPACE_ID=b python3 "$REPO/harness/opencode/workspace-mcp.py" tracker 2>/dev/null)"
eq "$out" "$h/.mcp-auth/beta" "workspace MCP: second workspace selects a separate store"
HOME="$h" PATH="$h/bin:$PATH" AGTERM_WORKSPACE_ID=missing python3 "$REPO/harness/opencode/workspace-mcp.py" tracker >/dev/null 2>&1; rc=$?
eq "$rc" "1" "workspace MCP: unknown workspace cannot use another account"
HOME="$h" PATH="$h/bin:$PATH" AGTERM_WORKSPACE_ID='' python3 "$REPO/harness/opencode/workspace-mcp.py" tracker >/dev/null 2>&1; rc=$?
eq "$rc" "1" "workspace MCP: missing workspace stops instead of choosing an account"

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
