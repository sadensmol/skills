#!/usr/bin/env bash
# Checks for install.sh, the Codex hook/link wiring, and for router-inject.sh,
# the hook it installs.
#
# Hermetic: every case runs the installer against a throwaway HOME and
# CODEX_HOME, so nothing here reads or writes the real configuration. No Codex,
# no network — only jq and python3 — which is why this runs on a bare machine as
# well as inside the container suite, which calls it.
#
#   usage: harness/codex/install.test.sh
set -uo pipefail

REPO="$(cd "$(dirname "$0")/../.." && pwd -P)"
INSTALL="$REPO/harness/codex/install.sh"
INJECT="$REPO/harness/codex/router-inject.sh"
RUNTIME="$REPO/harness/codex/codex.md"
NS="$(jq -r .name "$REPO/.claude-plugin/marketplace.json")"

pass=0; fail=0
grn=$'\033[32m'; red=$'\033[31m'; dim=$'\033[2m'; rst=$'\033[0m'
ok()  { printf '  %s✓%s %s\n' "$grn" "$rst" "$1"; pass=$((pass+1)); }
bad() { printf '  %s✗%s %s\n' "$red" "$rst" "$1"; [ $# -gt 1 ] && printf '      %s%s%s\n' "$dim" "$2" "$rst"; fail=$((fail+1)); }

# eq <got> <want> <description>
eq() { [ "$1" = "$2" ] && ok "$3" || bad "$3" "got: $1 | want: $2"; }

new_home() { mktemp -d; }
run() { HOME="$1" CODEX_HOME="$1/.codex" "$INSTALL" "${@:2}"; }
ours() { jq -r --arg c "$INJECT" '[.hooks.UserPromptSubmit[]?.hooks[]? | select(.command == $c)] | length' "$1"; }

printf '\n%s\n' "install.sh"

# --- 1. a machine with no Codex config at all --------------------------------
h="$(new_home)"; out="$(run "$h" 2>&1)"; rc=$?
cfg="$h/.codex/hooks.json"
eq "$rc" "0" "fresh machine: exits 0"
if [ -f "$cfg" ]; then
  ok "fresh machine: writes hooks.json"
  eq "$(ours "$cfg")" "1" "fresh machine: installs the UserPromptSubmit hook"
  eq "$(jq -r '.hooks.UserPromptSubmit[0].hooks[0].additionalContextLimit' "$cfg")" "0" \
     "fresh machine: additionalContextLimit 0, so the router is not spilled to a file"
  eq "$(readlink "$h/.codex/AGENTS.md")" "$REPO/instructions/AGENTS.md" "fresh machine: AGENTS.md is linked"
  eq "$(readlink "$h/.codex/prompts/task.md")" "$REPO/commands/task.md" "fresh machine: commands are linked as prompts"
  [ -f "$h/.codex/agents/quality.toml" ] && ok "fresh machine: agents are generated" \
                                         || bad "fresh machine: agents are generated" "$out"
else
  bad "fresh machine: writes hooks.json" "$out"
fi

# --- 2. an existing hooks.json keeps every hook it already had ---------------
h="$(new_home)"; mkdir -p "$h/.codex"; cfg="$h/.codex/hooks.json"
cat > "$cfg" <<JSON
{
  "description": "mine",
  "hooks": {
    "Stop": [{"hooks": [{"type": "command", "command": "/elsewhere/notify", "timeout": 30}]}],
    "UserPromptSubmit": [{"hooks": [{"type": "command", "command": "/elsewhere/other-namespace/inject.sh"}]}]
  }
}
JSON
run "$h" >/dev/null 2>&1
eq "$(jq -r '.hooks.Stop[0].hooks[0].command' "$cfg")" "/elsewhere/notify" "existing config: another event's hook survives"
eq "$(jq -r '.hooks.UserPromptSubmit[0].hooks[0].command' "$cfg")" "/elsewhere/other-namespace/inject.sh" \
   "existing config: a foreign UserPromptSubmit hook survives"
eq "$(jq -r '.description' "$cfg")" "mine" "existing config: unrelated keys survive"
eq "$(ours "$cfg")" "1" "existing config: ours is appended, not substituted"
[ -f "$cfg.bak" ] && ok "existing config: leaves a .bak" || bad "existing config: leaves a .bak"

# --- 3. running it again changes nothing -------------------------------------
h="$(new_home)"
run "$h" >/dev/null 2>&1; cfg="$h/.codex/hooks.json"; once="$(cat "$cfg")"
run "$h" >/dev/null 2>&1; run "$h" >/dev/null 2>&1
eq "$(cat "$cfg")" "$once" "idempotent: three runs, one result"
eq "$(ours "$cfg")" "1" "idempotent: still exactly one handler of ours"

# --- 4. an entry from an earlier location of the hook is replaced ------------
h="$(new_home)"; mkdir -p "$h/.codex"; cfg="$h/.codex/hooks.json"
jq -n --arg stale "$REPO/harness/codex-inject.sh" --arg keep "/elsewhere/inject.sh" \
  '{hooks: {UserPromptSubmit: [{hooks: [{type: "command", command: $keep}, {type: "command", command: $stale}]}]}}' > "$cfg"
run "$h" >/dev/null 2>&1
eq "$(jq -r '[.hooks.UserPromptSubmit[].hooks[].command] | join(",")' "$cfg")" \
   "/elsewhere/inject.sh,$INJECT" "stale harness/ entry is dropped, not duplicated"

# --- 5. --dry-run prints and writes nothing ----------------------------------
h="$(new_home)"
out="$(run "$h" --dry-run 2>/dev/null)"; rc=$?
eq "$rc" "0" "--dry-run: exits 0"
eq "$(printf '%s' "$out" | jq -r '.hooks.UserPromptSubmit[0].hooks[0].command')" "$INJECT" "--dry-run: prints the merged config"
[ -e "$h/.codex" ] && bad "--dry-run: touches nothing" "$h/.codex was created" || ok "--dry-run: touches nothing"

# --- 6. an unparseable hooks.json is refused, not mangled --------------------
h="$(new_home)"; mkdir -p "$h/.codex"; cfg="$h/.codex/hooks.json"
printf '{\n  // a comment jq cannot parse\n  "hooks": {}\n}\n' > "$cfg"
before="$(cat "$cfg")"
out="$(run "$h" 2>&1)"; rc=$?
eq "$rc" "1" "unparseable hooks.json: exits non-zero"
eq "$(cat "$cfg")" "$before" "unparseable hooks.json: file is left alone"
[ -f "$cfg.bak" ] && bad "unparseable hooks.json: writes no .bak" "a backup was made anyway" \
                  || ok "unparseable hooks.json: writes no .bak"

# --- 7. a missing runtime file is an error, not a half-written config --------
h="$(new_home)"; fake="$(cd "$(mktemp -d)" && pwd -P)"
mkdir -p "$fake/.claude-plugin" "$fake/harness/codex"
echo '{"name":"someone-else"}' > "$fake/.claude-plugin/marketplace.json"
cp "$INSTALL" "$fake/harness/codex/install.sh"
out="$(HOME="$h" CODEX_HOME="$h/.codex" "$fake/harness/codex/install.sh" 2>&1)"; rc=$?
eq "$rc" "1" "missing runtime file: exits 1"
case "$out" in *"missing $fake/harness/codex/codex.md"*) ok "missing runtime file: says which file" ;;
               *) bad "missing runtime file: says which file" "$out" ;; esac
[ -f "$h/.codex/hooks.json" ] && bad "missing runtime file: writes no config" "a config was created anyway" \
                              || ok "missing runtime file: writes no config"

# --- 8. an AGENTS.md the user wrote is kept, not destroyed -------------------
h="$(new_home)"; mkdir -p "$h/.codex"
echo "my own standing instructions" > "$h/.codex/AGENTS.md"
run "$h" >/dev/null 2>&1
eq "$(cat "$h/.codex/AGENTS.md.bak" 2>/dev/null)" "my own standing instructions" "a real AGENTS.md is kept as AGENTS.md.bak"
eq "$(readlink "$h/.codex/AGENTS.md")" "$REPO/instructions/AGENTS.md" "…and the link replaces it"

# --- 9. generated agents are valid TOML with the three required fields -------
h="$(new_home)"; run "$h" >/dev/null 2>&1
bad_agents="$(python3 - "$h/.codex/agents" <<'PY'
import os, sys, tomllib
d = sys.argv[1]
problems = []
for f in sorted(os.listdir(d)):
    try:
        with open(os.path.join(d, f), "rb") as fh:
            t = tomllib.load(fh)
    except Exception as e:
        problems.append("%s: %s" % (f, e)); continue
    for k in ("name", "description", "developer_instructions"):
        if not t.get(k):
            problems.append("%s: no %s" % (f, k))
print(" ".join(problems))
PY
)"
[ -z "$bad_agents" ] && ok "generated agents parse as TOML and carry name/description/developer_instructions" \
                     || bad "generated agents parse as TOML" "$bad_agents"
eq "$(python3 -c 'import tomllib,sys; print(tomllib.load(open(sys.argv[1],"rb")).get("model","-"))' "$h/.codex/agents/quality.toml")" \
   "-" "no Claude model slug is carried into a Codex agent"

# --- 10. a stale generated agent goes; a hand-written one stays --------------
echo 'name = "mine"' > "$h/.codex/agents/mine.toml"
cp "$h/.codex/agents/quality.toml" "$h/.codex/agents/removed-agent.toml"
run "$h" >/dev/null 2>&1
[ -f "$h/.codex/agents/removed-agent.toml" ] && bad "a generated agent whose source is gone is removed" \
                                             || ok "a generated agent whose source is gone is removed"
[ -f "$h/.codex/agents/mine.toml" ] && ok "a hand-written agent is left alone" \
                                    || bad "a hand-written agent is left alone"

printf '\n%s\n' "router-inject.sh"

# --- 11. the hook speaks Codex's output contract -----------------------------
out="$("$INJECT" 2>/dev/null)"; rc=$?
eq "$rc" "0" "exits 0"
printf '%s' "$out" | jq empty 2>/dev/null && ok "writes JSON on stdout, not text" || bad "writes JSON on stdout, not text"
eq "$(printf '%s' "$out" | jq -r '.hookSpecificOutput.hookEventName')" "UserPromptSubmit" "names the event"
ctx="$(printf '%s' "$out" | jq -r '.hookSpecificOutput.additionalContext')"
case "$ctx" in *"name: $NS-router"*) ok "carries the whole router" ;;
               *) bad "carries the whole router" "router frontmatter not found in additionalContext" ;; esac
case "$ctx" in *"Runtime: Codex"*) ok "carries harness/codex/codex.md" ;;
               *) bad "carries harness/codex/codex.md" "runtime file not found in additionalContext" ;; esac

# --- 12. it injects the live file, not a copy taken at install time ----------
# The probe edits the repository's own router, so keep a byte-exact copy and put
# it back — a test must not leave the working tree dirty.
MARK="codex-inject-probe-$$"
ROUTER_FILE="$REPO/skills/router/SKILL.md"
ROUTER_COPY="$(mktemp)"; cp -p "$ROUTER_FILE" "$ROUTER_COPY"
printf '\n<!-- %s -->\n' "$MARK" >> "$ROUTER_FILE"
ctx="$("$INJECT" 2>/dev/null | jq -r '.hookSpecificOutput.additionalContext')"
cat "$ROUTER_COPY" > "$ROUTER_FILE"; rm -f "$ROUTER_COPY"
case "$ctx" in *"$MARK"*) ok "injects the repository's current router, live" ;;
               *) bad "injects the repository's current router, live" "the probe marker did not come through" ;; esac

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
