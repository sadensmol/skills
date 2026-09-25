#!/usr/bin/env bash
# Install one or more skill repositories into this container, exactly the way
# README.md tells a human to install them: symlinks only, never copies.
#
#   install.sh <repo> [<repo> …]
#
# Every repository is optional in shape. A repository contributes whatever it
# has: skills/, agents/, instructions/AGENTS.md, harness/opencode/opencode.md,
# harness/codex/install.sh, a plugin.
# The namespace comes from .claude-plugin/marketplace.json, so nothing here
# names a specific repository.
set -euo pipefail

[ "$#" -ge 1 ] || { echo "usage: install.sh <repo> [<repo> …]" >&2; exit 2; }

mkdir -p "$HOME/.claude/skills" "$HOME/.agents/skills" "$HOME/.config/opencode"

oc_instructions=()

for repo in "$@"; do
  ns="$(jq -r .name "$repo/.claude-plugin/marketplace.json")"
  echo "installing $ns from $repo"

  # --- skills: one link per skill for Claude Code, one per namespace for the rest
  if [ -d "$repo/skills" ]; then
    for d in "$repo"/skills/*/; do
      [ -f "$d/SKILL.md" ] || continue
      name=$(sed -n 's/^name: *//p' "$d/SKILL.md" | head -1 | tr -d "\"'")
      [ -n "$name" ] || { echo "  !! no frontmatter name in $d/SKILL.md" >&2; continue; }
      rm -rf "$HOME/.claude/skills/$name"
      ln -s "${d%/}" "$HOME/.claude/skills/$name"
    done
    rm -rf "$HOME/.agents/skills/$ns"
    ln -s "$repo/skills" "$HOME/.agents/skills/$ns"
    [ -f "$repo/skills/router/SKILL.md" ] \
      && oc_instructions+=("$HOME/.agents/skills/$ns/router/SKILL.md")
  fi

  # --- agents: whole directory, both harnesses read it recursively
  if [ -d "$repo/agents" ]; then
    rm -rf "$HOME/.claude/agents" "$HOME/.config/opencode/agents"
    ln -s "$repo/agents" "$HOME/.claude/agents"
    ln -s "$repo/agents" "$HOME/.config/opencode/agents"
  fi

  # --- standing instructions: one file, both harnesses point at it
  if [ -f "$repo/instructions/AGENTS.md" ]; then
    rm -f "$HOME/.claude/CLAUDE.md" "$HOME/.config/opencode/AGENTS.md"
    ln -s "$repo/instructions/AGENTS.md" "$HOME/.claude/CLAUDE.md"
    ln -s "$repo/instructions/AGENTS.md" "$HOME/.config/opencode/AGENTS.md"
  fi

  # --- runtime contract for OpenCode, loaded beside the router
  [ -f "$repo/harness/opencode/opencode.md" ] && oc_instructions+=("$repo/harness/opencode/opencode.md")

  # --- Codex: hook, standing instructions, prompts and agents, by its own installer
  [ -x "$repo/harness/codex/install.sh" ] && "$repo/harness/codex/install.sh" >/dev/null

  # --- hooks ship as a Claude Code plugin
  if [ -d "$repo/plugin" ]; then
    claude plugin marketplace add "$repo" >/dev/null
    claude plugin install -y "$ns@$ns" >/dev/null
  fi
done

# OpenCode has no hook, so the router arrives through `instructions`.
jq -n --arg skills "$HOME/.agents/skills" \
  '{"$schema":"https://opencode.ai/config.json",
    instructions: $ARGS.positional,
    skills: {paths: [$skills]}}' \
  --args "${oc_instructions[@]}" > "$HOME/.config/opencode/opencode.json"

echo "installed: $* "
