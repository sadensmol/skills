#!/bin/sh
# UserPromptSubmit hook for Codex — router injector.
#
# Same job as the Claude Code plugin's skill-activation.sh: put the FULL router
# and this harness's runtime file in front of the model on every request, so the
# routing rules are always present and nothing has to assume "a router already
# ran". The difference is the contract — Codex reads a hook's stdout as JSON and
# takes the model-visible text from hookSpecificOutput.additionalContext, so
# plain text on stdout is a parse error here, not context.
#
# Codex also caps a hook's additionalContext at ~2500 tokens and spills the rest
# to a file, which would truncate the router; the installer therefore sets
# additionalContextLimit to 0 for this handler.
#
# Nothing below is machine- or project-specific: the repository is derived from
# this script's own location, and the namespace from the marketplace manifest,
# so a stranger's clone injects that clone's router.
set -u

HERE="$(cd "$(dirname "$0")" && pwd -P)"
REPO="$(cd "$HERE/../.." && pwd -P)"
NS=""
if [ -f "$REPO/.claude-plugin/marketplace.json" ]; then
  NS="$(sed -n 's/.*"name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' \
        "$REPO/.claude-plugin/marketplace.json" | head -1)"
fi

# The clone first, then the install locations — each of them a symlink back into
# a clone, so the injected text is never a stale copy.
ROUTER=""
for candidate in \
  "$REPO/skills/router/SKILL.md" \
  "${HOME}/.agents/skills/${NS:-sadensmol}/router/SKILL.md" \
  "${CODEX_HOME:-$HOME/.codex}/skills/${NS:-sadensmol}-router/SKILL.md"
do
  if [ -f "$candidate" ]; then
    ROUTER="$candidate"
    break
  fi
done

RUNTIME="$HERE/codex.md"

# Build the model-visible text, then hand it to a JSON encoder. Everything that
# follows is untrusted-by-quoting: no shell interpolation of file contents.
CONTEXT="$(
  printf '%s\n\n' '<SKILL_ACTIVATION_REQUIRED priority="override-default-behavior">'
  printf '%s\n\n' "EXTREMELY IMPORTANT: The ${NS:-sadensmol}-router rules are reproduced IN FULL below and are therefore ALWAYS in context. Evaluate them against the current cwd and prompt NOW, and load EVERY downstream skill they name that is not already loaded — BEFORE reading files, editing, running tools, or answering. A different router having run is NOT this router having run; never assume routing is already handled or was cached."

  if [ -n "$ROUTER" ]; then
    cat "$ROUTER"
  else
    printf '%s\n' "router SKILL.md not found in any install location — read and follow \`${NS:-sadensmol}-router\` from the skills list now if it is not already loaded."
  fi

  if [ -f "$RUNTIME" ]; then
    printf '\n%s\n\n' '--- Runtime contract for this harness (how to background, spawn, ask, isolate) ---'
    cat "$RUNTIME"
  fi

  printf '\n%s\n' '</SKILL_ACTIVATION_REQUIRED>'
)"

if command -v jq >/dev/null 2>&1; then
  printf '%s' "$CONTEXT" | jq -Rs \
    '{hookSpecificOutput: {hookEventName: "UserPromptSubmit", additionalContext: .}}'
elif command -v python3 >/dev/null 2>&1; then
  printf '%s' "$CONTEXT" | python3 -c 'import json,sys; print(json.dumps({"hookSpecificOutput": {"hookEventName": "UserPromptSubmit", "additionalContext": sys.stdin.read()}}))'
else
  # No encoder: say so in a payload that needs no escaping, rather than emitting
  # broken JSON that Codex reports as a failed hook every single turn.
  printf '{"hookSpecificOutput":{"hookEventName":"UserPromptSubmit","additionalContext":"The router could not be injected: neither jq nor python3 is on PATH. Read and follow the %s-router skill from the skills list before doing anything else."}}\n' "${NS:-sadensmol}"
fi
