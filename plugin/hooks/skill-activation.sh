#!/bin/sh
# UserPromptSubmit hook — router injector.
#
# Fires on EVERY request. Injects the FULL sadensmol-router SKILL.md into
# context every turn, so the routing rules are ALWAYS present regardless of
# whether the Skill tool was ever invoked. This closes the failure mode where
# Claude assumes "a router already ran" and skips the routing pass — leaving
# downstream skills (go-programming, typescript-programming, …) unloaded.
#
# The skills no longer live inside this plugin: they are installed as Agent
# Skills, shared with the other harnesses. So the router is resolved from the
# install locations, in order, and every one of them is a symlink back to the
# skills repository — the injected text is never a stale copy.

for candidate in \
  "${CLAUDE_PLUGIN_ROOT}/../skills/router/SKILL.md" \
  "${HOME}/.claude/skills/sadensmol-router/SKILL.md" \
  "${HOME}/.agents/skills/sadensmol/router/SKILL.md"
do
  if [ -f "$candidate" ]; then
    ROUTER="$candidate"
    break
  fi
done

printf '%s\n\n' '<SKILL_ACTIVATION_REQUIRED priority="override-default-behavior">'
printf '%s\n\n' 'EXTREMELY IMPORTANT: The sadensmol-router rules are reproduced IN FULL below and are therefore ALWAYS in context. Evaluate them against the current cwd and prompt NOW, and invoke (via the Skill tool) EVERY downstream skill they name that is not already loaded — BEFORE reading files, editing, running tools, or answering. A different router having run is NOT this router having run; never assume routing is already handled or was cached.'

if [ -n "$ROUTER" ]; then
  cat "$ROUTER"
else
  printf '%s\n' 'router SKILL.md not found in any install location — invoke the `sadensmol-router` skill now if it is not already loaded.'
fi

# The runtime file sits beside the skills in the same repository the router came from.
# Every install location is a symlink, so resolve to the real directory first (pwd -P)
# instead of walking the link path, which would land in ~/.claude or ~/.agents.
if [ -n "$ROUTER" ]; then
  RUNTIME="$(cd "$(dirname "$ROUTER")" 2>/dev/null && pwd -P)/../../harness/claude-code.md"
  if [ -f "$RUNTIME" ]; then
    printf '\n%s\n\n' '--- Runtime contract for this harness (how to background, spawn, ask, isolate) ---'
    cat "$RUNTIME"
  fi
fi

printf '\n%s\n' '</SKILL_ACTIVATION_REQUIRED>'
