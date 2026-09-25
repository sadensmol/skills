#!/usr/bin/env bash
# Container checks for a skills repository.
#
# Everything runs against a COPY of the mounted repository, so the working tree
# on the host is never written to. No credentials, no model calls: both harness
# CLIs are exercised through their offline introspection commands.
#
#   run-tests.sh [<repo-src> [<supporting-repo-src> …]]     default: /src
#
# The first path is the repository under test. Any further paths are installed
# alongside it — that is how a project repository is checked together with the
# base namespace it layers on. If /home/agent/test-extra/project-checks.sh
# exists it is sourced at the end, so a repository can add its own assertions
# without copying this file.
set -uo pipefail

SRC_DIRS=("$@")
[ "${#SRC_DIRS[@]}" -gt 0 ] || SRC_DIRS=(/src)

pass=0; fail=0
grn=$'\033[32m'; red=$'\033[31m'; dim=$'\033[2m'; rst=$'\033[0m'

ok()  { printf '  %s✓%s %s\n' "$grn" "$rst" "$1"; pass=$((pass+1)); }
bad() { printf '  %s✗%s %s\n' "$red" "$rst" "$1"; [ $# -gt 1 ] && printf '      %s%s%s\n' "$dim" "$2" "$rst"; fail=$((fail+1)); }
group(){ printf '\n%s\n' "$1"; }

# assert_link <link> <expected dir/file> <description>
assert_link() {
  local link="$1" want="$2" desc="$3" got
  if [ ! -L "$link" ]; then bad "$desc" "not a symlink: $link"; return; fi
  got="$(realpath "$link" 2>/dev/null)"
  if [ "$got" = "$(realpath "$want")" ]; then ok "$desc"; else bad "$desc" "$link -> $got, wanted $want"; fi
}

# assert_has <haystack-file> <needle> <description>
assert_has() {
  if grep -qF -- "$2" "$1" 2>/dev/null; then ok "$3"; else bad "$3" "not found in $1: $2"; fi
}

# ---------------------------------------------------------------- setup ----
REPOS=()
for src in "${SRC_DIRS[@]}"; do
  dest="$HOME/repo${src#/src}"      # /src -> $HOME/repo, /src-base -> $HOME/repo-base
  rm -rf "$dest"; mkdir -p "$dest"
  cp -a "$src"/. "$dest"/
  REPOS+=("$dest")
done

# Supporting repositories install first, so the repository under test layers on
# top of them the way it does on a real machine.
INSTALL_ORDER=("${REPOS[@]:1}" "${REPOS[0]}")

printf '%sinstalling %s%s\n' "$dim" "${INSTALL_ORDER[*]}" "$rst"
if ! "$HOME/test/install.sh" "${INSTALL_ORDER[@]}" > /tmp/install.log 2>&1; then
  printf '%sinstall failed:%s\n' "$red" "$rst"; cat /tmp/install.log; exit 1
fi

REPO="${REPOS[0]}"
NS="$(jq -r .name "$REPO/.claude-plugin/marketplace.json")"

# The standing instructions and the agents directory each live in exactly one
# of the repositories — not necessarily the one under test.
INSTR_REPO=""; AGENTS_REPO=""
for r in "${REPOS[@]}"; do
  [ -z "$INSTR_REPO"  ] && [ -f "$r/instructions/AGENTS.md" ] && INSTR_REPO="$r"
  [ -z "$AGENTS_REPO" ] && [ -d "$r/agents" ]                && AGENTS_REPO="$r"
done

# every skill this repository ships, by frontmatter name
mapfile -t SKILL_DIRS < <(find "$REPO/skills" -mindepth 2 -maxdepth 2 -name SKILL.md -printf '%h\n' | sort)
skill_name() { sed -n 's/^name: *//p' "$1/SKILL.md" | head -1 | tr -d "\"'"; }

# ------------------------------------------------- A. Claude Code wiring ----
group "Claude Code"

missing=0; wrong=0
for d in "${SKILL_DIRS[@]}"; do
  n="$(skill_name "$d")"
  if [ ! -L "$HOME/.claude/skills/$n" ]; then missing=$((missing+1)); continue; fi
  [ "$(realpath "$HOME/.claude/skills/$n")" = "$(realpath "$d")" ] || wrong=$((wrong+1))
done
[ "$missing" -eq 0 ] && ok "every skill is linked (${#SKILL_DIRS[@]})" || bad "every skill is linked" "$missing of ${#SKILL_DIRS[@]} missing"
[ "$wrong" -eq 0 ] && ok "every skill link resolves into the repository" || bad "every skill link resolves into the repository" "$wrong resolve elsewhere"

[ -n "$AGENTS_REPO" ] && assert_link "$HOME/.claude/agents" "$AGENTS_REPO/agents" "agents/ is linked"
[ -n "$INSTR_REPO"  ] && assert_link "$HOME/.claude/CLAUDE.md" "$INSTR_REPO/instructions/AGENTS.md" "CLAUDE.md -> instructions/AGENTS.md"

claude plugin list > /tmp/plugins.txt 2>&1
assert_has /tmp/plugins.txt "$NS@$NS" "plugin $NS@$NS is installed"

CLAUDE_PLUGIN_ROOT="$REPO/plugin" "$REPO/plugin/hooks/skill-activation.sh" > /tmp/hook.txt 2>&1
assert_has /tmp/hook.txt "name: $NS-router" "UserPromptSubmit hook injects the router"
# Only the repository that ships the runtime contract injects it; a project
# repository layering on a base deliberately leaves that to the base's hook.
[ -f "$REPO/harness/claude-code.md" ] \
  && assert_has /tmp/hook.txt "Runtime: Claude Code" "hook also injects harness/claude-code.md"

# ----------------------------------------------------- B. OpenCode wiring ----
group "OpenCode"

assert_link "$HOME/.agents/skills/$NS" "$REPO/skills" "skills/ is linked as the $NS namespace"
[ -n "$INSTR_REPO"  ] && assert_link "$HOME/.config/opencode/AGENTS.md" "$INSTR_REPO/instructions/AGENTS.md" "AGENTS.md -> instructions/AGENTS.md"
[ -n "$AGENTS_REPO" ] && assert_link "$HOME/.config/opencode/agents" "$AGENTS_REPO/agents" "agents/ is linked"

# opencode's JSON is large and gets truncated when piped, so land it in a file first.
oc_skills() { opencode debug skill > /tmp/oc-skills.json 2>/dev/null; }
oc_skills
jq -r '.[] | [.name, (.location // "<none>")] | @tsv' /tmp/oc-skills.json > /tmp/oc-skills.tsv
missing=0; outside=0
for d in "${SKILL_DIRS[@]}"; do
  n="$(skill_name "$d")"
  line="$(awk -F'\t' -v n="$n" '$1==n {print $2; exit}' /tmp/oc-skills.tsv)"
  if [ -z "$line" ]; then missing=$((missing+1)); continue; fi
  # opencode scans both install paths and may report either, so resolve the
  # link: what matters is that the file it reads lives in this repository.
  case "$(realpath "$line" 2>/dev/null)" in "$REPO/"*) ;; *) outside=$((outside+1));; esac
done
[ "$missing" -eq 0 ] && ok "opencode resolves every skill (${#SKILL_DIRS[@]})" || bad "opencode resolves every skill" "$missing missing — see /tmp/oc-skills.tsv"
[ "$outside" -eq 0 ] && ok "opencode reads them from this repository" || bad "opencode reads them from this repository" "$outside from elsewhere"

dupes="$(cut -f1 /tmp/oc-skills.tsv | sort | uniq -d | tr '\n' ' ')"
[ -z "$dupes" ] && ok "no skill name is registered twice" || bad "no skill name is registered twice" "$dupes"

opencode debug config > /tmp/oc-config.json 2>/dev/null
jq -r '.instructions[]?' /tmp/oc-config.json > /tmp/oc-instructions.txt
assert_has /tmp/oc-instructions.txt "/router/SKILL.md" "router is in opencode instructions"
assert_has /tmp/oc-instructions.txt "harness/opencode.md" "harness/opencode.md is in opencode instructions"

# The config merge has its own hermetic checks — each one runs the installer
# against a throwaway HOME, so none of them touch the config installed above.
if [ -x "$REPO/harness/opencode/install.test.sh" ]; then
  if "$REPO/harness/opencode/install.test.sh" > /tmp/oc-install-test.txt 2>&1; then
    ok "harness/opencode/install.sh: $(tail -1 /tmp/oc-install-test.txt)"
  else
    bad "harness/opencode/install.sh: $(tail -1 /tmp/oc-install-test.txt)" \
        "$(grep -c '✗' /tmp/oc-install-test.txt) failing — see /tmp/oc-install-test.txt"
  fi
fi

# -------------------------------------------------------- B2. Codex wiring ----
group "Codex"

# Codex finds the skills itself under ~/.agents/skills — the same link OpenCode
# uses — and prints everything it would send the model, offline and without auth.
codex debug prompt-input "probe" > /tmp/codex-prompt.json 2>/tmp/codex-prompt.err
if jq -e . /tmp/codex-prompt.json >/dev/null 2>&1; then
  jq -r '[.[].content[]?.text?] | join("\n")' /tmp/codex-prompt.json > /tmp/codex-prompt.txt
  missing=0; outside=0
  for d in "${SKILL_DIRS[@]}"; do
    n="$(skill_name "$d")"
    line="$(grep -m1 -F -- "- $n: " /tmp/codex-prompt.txt)" || { missing=$((missing+1)); continue; }
    case "$line" in *"/$NS/$(basename "$d")/SKILL.md"*) ;; *) outside=$((outside+1));; esac
  done
  [ "$missing" -eq 0 ] && ok "codex resolves every skill (${#SKILL_DIRS[@]})" || bad "codex resolves every skill" "$missing missing — see /tmp/codex-prompt.txt"
  [ "$outside" -eq 0 ] && ok "codex reads them from this repository's namespace" || bad "codex reads them from this repository's namespace" "$outside from elsewhere"
  [ -n "$INSTR_REPO" ] && assert_has /tmp/codex-prompt.txt "AGENTS.md instructions" "codex loads the standing instructions"
else
  bad "codex debug prompt-input returns JSON" "$(tail -2 /tmp/codex-prompt.err)"
fi

if [ -f "$REPO/harness/codex/install.sh" ]; then
  assert_has "$HOME/.codex/hooks.json" "harness/codex/router-inject.sh" "the UserPromptSubmit hook is installed"
  [ -n "$INSTR_REPO" ] && assert_link "$HOME/.codex/AGENTS.md" "$INSTR_REPO/instructions/AGENTS.md" "AGENTS.md -> instructions/AGENTS.md"
  for f in "$REPO"/commands/*.md; do
    [ -f "$f" ] || continue
    assert_link "$HOME/.codex/prompts/$(basename "$f")" "$f" "command $(basename "$f" .md) is linked as a prompt"
  done
  if [ -n "$AGENTS_REPO" ]; then
    want="$(find "$AGENTS_REPO/agents" -name '*.md' | wc -l)"
    got="$(find "$HOME/.codex/agents" -name '*.toml' 2>/dev/null | wc -l)"
    [ "$got" -eq "$want" ] && ok "every agent is generated as TOML ($want)" || bad "every agent is generated as TOML" "$got of $want"
  fi

  # The hook and merge have their own hermetic checks — each case runs the
  # installer against a throwaway HOME, so none of them touch the install above.
  if [ -x "$REPO/harness/codex/install.test.sh" ]; then
    if "$REPO/harness/codex/install.test.sh" > /tmp/codex-install-test.txt 2>&1; then
      ok "harness/codex/install.sh: $(tail -1 /tmp/codex-install-test.txt)"
    else
      bad "harness/codex/install.sh: $(tail -1 /tmp/codex-install-test.txt)" \
          "$(grep -c '✗' /tmp/codex-install-test.txt) failing — see /tmp/codex-install-test.txt"
    fi
  fi
fi

# ----------------------------------------------------------- C. validity ----
group "Validity"

skills add "$REPO/skills" --list 2>&1 | sed -r 's/\x1B\[[0-9;?]*[a-zA-Z]//g' > /tmp/skills-list.txt
listed="$(awk '{gsub(/^[^a-z]*/, ""); if (NF==1 && $0 ~ /^[a-z0-9]+(-[a-z0-9]+)+$/) print}' /tmp/skills-list.txt | sort -u | wc -l)"
[ "$listed" -eq "${#SKILL_DIRS[@]}" ] \
  && ok "skills CLI lists all ${#SKILL_DIRS[@]} (frontmatter parses)" \
  || bad "skills CLI lists all ${#SKILL_DIRS[@]}" "listed $listed — a skipped skill has broken YAML; see /tmp/skills-list.txt"

claude plugin validate "$REPO"        >/tmp/v1.txt 2>&1 && ok "marketplace manifest validates" || bad "marketplace manifest validates" "$(tail -1 /tmp/v1.txt)"
claude plugin validate "$REPO/plugin" >/tmp/v2.txt 2>&1 && ok "plugin manifest validates"      || bad "plugin manifest validates"      "$(tail -1 /tmp/v2.txt)"

badagents=""
while IFS= read -r a; do
  head -20 "$a" | grep -q '^name:'        || badagents="$badagents $(basename "$a"):name"
  head -20 "$a" | grep -q '^description:' || badagents="$badagents $(basename "$a"):description"
  head -20 "$a" | grep -q '^mode: *subagent' || badagents="$badagents $(basename "$a"):mode"
  head -20 "$a" | grep -qE '^color: *[a-z]+' && badagents="$badagents $(basename "$a"):color"
done < <(find "$REPO/agents" -name '*.md' 2>/dev/null)
[ -d "$REPO/agents" ] && { [ -z "$badagents" ] && ok "agent frontmatter follows the conventions" || bad "agent frontmatter follows the conventions" "$badagents"; }

# ------------------------------------------------------ D. edits sync ----
group "Edits reach the repository (and back)"

MARK="container-sync-probe-$$"
ROUTER_REPO="$REPO/skills/router/SKILL.md"
ROUTER_CC="$HOME/.claude/skills/$NS-router/SKILL.md"
ROUTER_OC="$HOME/.agents/skills/$NS/router/SKILL.md"
INSTR_FILE="${INSTR_REPO:+$INSTR_REPO/instructions/AGENTS.md}"

printf '\n<!-- %s -->\n' "$MARK" >> "$ROUTER_CC"
assert_has "$ROUTER_REPO" "$MARK" "edit through ~/.claude/skills lands in the repository"
sed -i "/$MARK/d" "$ROUTER_REPO"
grep -qF "$MARK" "$ROUTER_CC" && bad "revert clears it on both sides" || ok "revert clears it on both sides"

printf '\n<!-- %s -->\n' "$MARK" >> "$ROUTER_OC"
assert_has "$ROUTER_REPO" "$MARK" "edit through ~/.agents/skills lands in the repository"

# a harness must see a repository edit with no re-install
oc_skills
jq -r --arg n "$NS-router" '.[] | select(.name==$n) | .content' /tmp/oc-skills.json > /tmp/oc-router.txt
assert_has /tmp/oc-router.txt "$MARK" "opencode reads the edited file live (no re-install)"
CLAUDE_PLUGIN_ROOT="$REPO/plugin" "$REPO/plugin/hooks/skill-activation.sh" > /tmp/hook2.txt 2>&1
assert_has /tmp/hook2.txt "$MARK" "the Claude Code hook injects the edited file live"
sed -i "/$MARK/d" "$ROUTER_REPO"

if [ -n "$INSTR_FILE" ]; then
  printf '\n<!-- %s -->\n' "$MARK" >> "$HOME/.claude/CLAUDE.md"
  assert_has "$INSTR_FILE" "$MARK" "instructions edited via Claude Code land in the repository"
  assert_has "$HOME/.config/opencode/AGENTS.md" "$MARK" "…and OpenCode sees the same edit at once"
  sed -i "/$MARK/d" "$INSTR_FILE"

  printf '\n<!-- %s -->\n' "$MARK" >> "$INSTR_FILE"
  assert_has "$HOME/.claude/CLAUDE.md" "$MARK" "instructions edited in the repository reach Claude Code"
  assert_has "$HOME/.config/opencode/AGENTS.md" "$MARK" "…and reach OpenCode"
  sed -i "/$MARK/d" "$INSTR_FILE"
fi

# ------------------------------------------------------ E. a new skill ----
group "A new skill"

probe="$REPO/skills/zz-container-probe"
mkdir -p "$probe"
{ echo "---"; echo "name: $NS-zz-container-probe"; echo "description: Probe skill created by the container test run."; echo "---"; echo; echo "# Probe"; } > "$probe/SKILL.md"
"$HOME/test/install.sh" "${REPOS[@]}" >> /tmp/install.log 2>&1

if [ -L "$HOME/.claude/skills/$NS-zz-container-probe" ]; then ok "re-running the install links a new skill"; else bad "re-running the install links a new skill"; fi
oc_skills
jq -r '.[].name' /tmp/oc-skills.json | grep -qx "$NS-zz-container-probe" \
  && ok "opencode picks the new skill up" || bad "opencode picks the new skill up"

rm -rf "$probe" "$HOME/.claude/skills/$NS-zz-container-probe"

dangling="$(find -L "$HOME/.claude/skills" "$HOME/.agents/skills" "$HOME/.claude" "$HOME/.codex" -maxdepth 2 -type l 2>/dev/null | tr '\n' ' ')"
[ -z "$dangling" ] && ok "no dangling links are left behind" || bad "no dangling links are left behind" "$dangling"

# ------------------------------------------- F. repository-specific ----
if [ -r "$HOME/test-extra/project-checks.sh" ]; then
  # shellcheck disable=SC1091
  . "$HOME/test-extra/project-checks.sh"
fi

# ------------------------------------------------------------- summary ----
printf '\n%s%d passed%s, %s%d failed%s\n' "$grn" "$pass" "$rst" "$([ "$fail" -gt 0 ] && echo "$red" || echo "$dim")" "$fail" "$rst"
[ "$fail" -eq 0 ]
