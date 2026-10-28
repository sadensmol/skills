#!/usr/bin/env bash
# Checks for cleanup.sh.
#
# Hermetic: every case runs against a throwaway HOME with a fake `opencode` CLI
# (it deletes from the fixture database), a fake `docker` (it serves a fixture
# image list) and a fake `pgrep` (it decides which apps look running), so nothing
# here reads or writes real data. Needs sqlite3.
#
#   usage: skills/cleanup/scripts/cleanup.test.sh
set -uo pipefail

SCRIPT="$(cd "$(dirname "$0")" && pwd -P)/cleanup.sh"
unset CLAUDE_CONFIG_DIR CODEX_HOME XDG_CACHE_HOME XDG_CONFIG_HOME XDG_DATA_HOME XDG_STATE_HOME

pass=0; fail=0
grn=$'\033[32m'; red=$'\033[31m'; dim=$'\033[2m'; rst=$'\033[0m'
ok()  { printf '  %s✓%s %s\n' "$grn" "$rst" "$1"; pass=$((pass+1)); }
bad() { printf '  %s✗%s %s\n' "$red" "$rst" "$1"; [ $# -gt 1 ] && printf '      %s%s%s\n' "$dim" "$2" "$rst"; fail=$((fail+1)); }
has()   { grep -qF -- "$2" "$1" && ok "$3" || bad "$3" "missing: $2"; }
lacks() { grep -qF -- "$2" "$1" && bad "$3" "present: $2" || ok "$3"; }
exists() { [ -e "$1" ] && ok "$2" || bad "$2" "gone: $1"; }
gone()   { [ -e "$1" ] && bad "$2" "still there: $1" || ok "$2"; }

OLD=202001010000                                   # touch -t stamp, years in the past
old() { find "$@" -exec touch -t "$OLD" {} +; }    # backdate paths and everything under them
mk()  { mkdir -p "$(dirname "$1")"; echo x > "$1"; }
img() { printf 'sha256:%064d' "$(printf '%d' "'$1")"; }  # a fake image id per letter
desk_of() { if [ "$(uname -s)" = Darwin ]; then echo "$1/Library/Application Support/Claude"; else echo "$1/.config/Claude"; fi; }

# A fake home with one old and one fresh item for most targets, plus protected
# files that are old too, so only the rules keep them alive.
new_home() {
  local h; h="$(mktemp -d)"; h="$(cd "$h" && pwd -P)"
  local cc="$h/.claude" p="$h/.claude/projects/-work-acme" D C
  D="$(desk_of "$h")"
  if [ "$(uname -s)" = Darwin ]; then C="$h/Library/Caches/claude-cli-nodejs"; else C="$h/.cache/claude-cli-nodejs"; fi
  mk "$p/11111111-aaaa-bbbb-cccc-000000000001.jsonl"
  mk "$p/11111111-aaaa-bbbb-cccc-000000000001/subagents/a.jsonl"
  mk "$p/22222222-aaaa-bbbb-cccc-000000000002.jsonl"           # fresh session
  mk "$p/memory/MEMORY.md"
  mk "$h/.claude/projects/-work-gone/33333333-aaaa-bbbb-cccc-000000000003.jsonl"
  mk "$cc/file-history/old-session/f.txt"; mk "$cc/file-history/new-session/f.txt"
  mk "$cc/todos/old.json"; mk "$cc/jobs/abc123/out.log"; mk "$cc/jobs/pins.json"
  mk "$cc/settings.json"; mk "$cc/settings.json.bak-1"; mk "$cc/CLAUDE.md"; mk "$cc/history.jsonl"
  mk "$cc/skills/mine/SKILL.md"
  mk "$cc/plugins/cache/mkt/tool/1.0.0/a"; mk "$cc/plugins/cache/mkt/tool/2.0.0/a"; mk "$cc/plugins/cache/mkt/tool/3.0.0/a"
  printf '{"plugins":{"tool@mkt":[{"installPath": "%s"}]}}\n' "$cc/plugins/cache/mkt/tool/2.0.0" > "$cc/plugins/installed_plugins.json"
  mk "$C/-work-acme/mcp.log"
  mk "$D/local-agent-mode-sessions/acct/org/local_old.json"
  mk "$D/Cache/Cache_Data/f_0001"
  mk "$D/vm_bundles/claudevm.bundle/machineIdentifier"
  mk "$D/claude-code/1.0.0/claude"; mk "$D/claude-code/2.0.0/claude"
  mk "$h/.local/share/opencode/tool-output/tool_old"; mk "$h/.local/share/opencode/tool-output/tool_new"
  mk "$h/.config/opencode/opencode.jsonc"; mk "$h/.config/opencode/opencode.jsonc.bak"
  mk "$h/Downloads/old.zip"                                     # outside every root
  mk "$h/.codex/projectless/2020-01-01/old-chat/outputs/report.md"
  mk "$h/.codex/projectless/2020-01-02/new-chat/outputs/report.md"
  mk "$h/Documents/Codex/2020-01-01/doc-chat/a.md"
  printf '[desktop]\nprojectlessWorkspaceRoot = "%s"\n\n[other]\nx = 1\n' "$h/.codex/projectless" > "$h/.codex/config.toml"

  old "$h"
  touch "$p/22222222-aaaa-bbbb-cccc-000000000002.jsonl" "$cc/file-history/new-session/f.txt" \
        "$h/.local/share/opencode/tool-output/tool_new" "$h/.codex/projectless/2020-01-02/new-chat/outputs/report.md"
  touch "$cc/plugins/cache/mkt/tool/3.0.0" "$cc/plugins/cache/mkt/tool/3.0.0/a"   # the newest version
  touch "$D/claude-code/2.0.0" "$D/claude-code/2.0.0/claude"
  # 1.0.0 is unused and old; 2.0.0 is installed (old); 3.0.0 is newest. Keep 2 and 3.

  # OpenCode: tree A is all old; tree B has an old root and a fresh child.
  local db="$h/.local/share/opencode/opencode.db" ms now
  now=$(( $(date +%s) * 1000 )); ms=$(( now - 90 * 86400000 ))
  sqlite3 "$db" "
    CREATE TABLE session (id text PRIMARY KEY, parent_id text, time_updated integer NOT NULL);
    CREATE TABLE message (id text PRIMARY KEY, session_id text NOT NULL REFERENCES session(id) ON DELETE CASCADE, data text);
    CREATE TABLE part (id text PRIMARY KEY, message_id text, session_id text NOT NULL REFERENCES session(id) ON DELETE CASCADE, data text);
    INSERT INTO session VALUES ('ses_A', NULL, $ms), ('ses_A1', 'ses_A', $ms), ('ses_B', NULL, $ms), ('ses_B1', 'ses_B', $now);
    INSERT INTO message VALUES ('m1', 'ses_A', 'hello'), ('m2', 'ses_A1', 'hi');"

  mkdir -p "$h/bin"
  cat > "$h/bin/opencode" <<EOF
#!/usr/bin/env bash
[ "\$1 \$2" = "session delete" ] || exit 1
sqlite3 "$db" "PRAGMA foreign_keys = ON; DELETE FROM session WHERE id = '\$3';"
EOF
  printf '#!/usr/bin/env bash\n[ -f "%s/running" ] && grep -qx -- "${@: -1}" "%s/running"\n' "$h" "$h" > "$h/bin/pgrep"

  # Docker: id, created, last tagged (epoch seconds), unique size, tags. A is old and
  # unused; B is old but a stopped container uses it; C was pulled recently; D is new.
  local dd="$h/docker" s; s=$(( now / 1000 )); mkdir -p "$dd"
  printf '%s\t%s\t%s\t%s\t%s\n' \
    "$(img A)" $((s - 90*86400)) $((s - 90*86400)) 2.5GB "app:old,app:older" \
    "$(img B)" $((s - 90*86400)) $((s - 90*86400)) 10MB  "db:1" \
    "$(img C)" $((s - 90*86400)) $((s - 86400))    10MB  "pulled:1" \
    "$(img D)" $((s - 86400))    $((s - 86400))    10MB  "new:1" > "$dd/images"
  printf 'c1\t%s\n' "$(img B)" > "$dd/containers"
  printf '#!/usr/bin/env bash\nD="%s"\n' "$dd" > "$h/bin/docker"
  cat >> "$h/bin/docker" <<'FAKE'
case "$1 $2" in
  "info "*) [ ! -f "$D/down" ] ;;
  "context show") echo default ;;
  "ps -aq") cut -f1 "$D/containers" ;;
  "inspect --format") shift 2; for c in "$@"; do awk -F'\t' -v c="$c" '$1 == c { print $2 }' "$D/containers"; done ;;
  "image ls") a="$*"; h="${a##*until=}"; h="${h%%h*}"; awk -F'\t' -v c=$(( $(date +%s) - h * 3600 )) '$2 < c { print $1 }' "$D/images" ;;
  "system df") awk -F'\t' '{ print $1, $4 }' "$D/images" ;;
  "image inspect")
    if [ "$3" = --format ]; then f="$4"; shift 4; else f=""; shift 2; fi
    for id in "$@"; do
      line="$(awk -F'\t' -v i="$id" '$1 == i' "$D/images")"; [ -n "$line" ] || exit 1
      # Like the real CLI: LastTagTime is a string, so a method on it fails.
      case "$f" in *LastTagTime.*) echo "template: can't evaluate field" >&2; exit 1 ;;
                   *LastTagTime*) e="$(cut -f3 <<<"$line")"   # both formats the CLI prints
                     fmt=%Y-%m-%dT%H:%M:%S.000000000Z; [ "$id" = "$(cut -f1 <<<"$line")" ] && grep -q pulled <<<"$line" && fmt='%Y-%m-%d %H:%M:%S.000000000 +0000 UTC'
                     printf '%s %s\n' "$id" "$(date -u -r "$e" +"$fmt" 2>/dev/null || date -u -d "@$e" +"$fmt")" ;;
                   *RepoTags*) cut -f5 <<<"$line" | tr ',' ' ' ;; esac
    done ;;
  "image rm") shift 2
    for a in "$@"; do        # a tag drops that tag, the last tag or the id drops the image
      awk -F'\t' -v OFS='\t' -v a="$a" '{ n = split($5, t, ","); k = ""
        for (i = 1; i <= n; i++) if (t[i] != a) k = k (k == "" ? "" : ",") t[i]
        if ($1 == a || (n > 0 && k == "")) next; $5 = k; print }' "$D/images" > "$D/images.new" && mv "$D/images.new" "$D/images"
    done ;;
  *) exit 1 ;;
esac
FAKE
  chmod +x "$h/bin/opencode" "$h/bin/pgrep" "$h/bin/docker"
  echo "$h"
}
run() { local h="$1"; shift; HOME="$h" PATH="$h/bin:$PATH" bash "$SCRIPT" "$@"; }
sessions() { sqlite3 "$1/.local/share/opencode/opencode.db" "SELECT group_concat(id, ' ') FROM (SELECT id FROM session ORDER BY id)"; }

printf '\n%s\n' "cleanup.sh"

# --- 1. scan lists only old, unprotected items and deletes nothing --------------
h="$(new_home)"; M="$h/manifest.tsv"; out="$h/scan.out"
run "$h" scan --out "$M" > "$out" 2>&1; rc=$?
[ "$rc" = 0 ] && ok "scan exits 0" || bad "scan exits 0" "rc=$rc: $(cat "$out")"
has   "$M" "000000000001.jsonl"                     "old transcript listed"
grep -q $'\t'"$h/.claude/projects/-work-acme/11111111-aaaa-bbbb-cccc-000000000001$" "$M" && ok "session folder listed" || bad "session folder listed"
lacks "$M" "000000000002"                            "fresh transcript not listed"
lacks "$M" "/memory"                                 "memory not listed"
has   "$M" "file-history/old-session"                "old file history listed"
lacks "$M" "file-history/new-session"                "fresh file history not listed"
has   "$M" "settings.json.bak-1"                     "old config backup listed"
grep -q '/settings.json$' "$M" && bad "settings.json not listed" || ok "settings.json not listed"
has   "$M" "jobs/abc123"                             "old job folder listed"
lacks "$M" "pins.json"                               "job pins not listed"
lacks "$M" "/skills/"                                "skills not listed"
has   "$M" "tool/1.0.0"                              "unused plugin version listed"
lacks "$M" "tool/2.0.0"                              "installed plugin version not listed"
lacks "$M" "tool/3.0.0"                              "newest plugin version not listed"
has   "$M" "claude-cli-nodejs/-work-acme"            "old MCP log cache listed"
has   "$M" "local_old.json"                          "old Desktop session listed"
has   "$M" "Cache_Data/f_0001"                       "old Desktop cache file listed while the app is closed"
lacks "$M" "vm_bundles"                              "VM bundle not listed"
has   "$M" "claude-code/1.0.0"                       "old bundled Claude Code listed"
lacks "$M" "claude-code/2.0.0"                       "newest bundled Claude Code not listed"
has   "$M" "opencode.jsonc.bak"                      "old OpenCode config backup listed"
grep -q '/opencode.jsonc$' "$M" && bad "opencode.jsonc not listed" || ok "opencode.jsonc not listed"
has   "$M" "tool_old"                                "old tool output listed"
lacks "$M" "tool_new"                                "fresh tool output not listed"
lacks "$M" "Downloads"                               "nothing outside the roots listed"
has   "$M" "projectless/2020-01-01/old-chat"         "old Codex chat listed"
lacks "$M" "new-chat"                                "Codex chat with a fresh file not listed"
lacks "$M" "Documents"                               "nothing in Documents listed"
lacks "$M" "config.toml"                             "Codex config not listed"
grep -q $'^opencode-session\tOpenCode|sessions\t[0-9]*\tses_A1$' "$M" && ok "old child session listed" || bad "old child session listed"
lacks "$M" "ses_B"                                   "tree with a fresh child not listed"
[ "$(grep -n 'ses_A1$' "$M" | cut -d: -f1)" -lt "$(grep -n 'ses_A$' "$M" | cut -d: -f1)" ] \
  && ok "child session ordered before its parent" || bad "child session ordered before its parent"
has   "$out" "| **Total** |"                         "table printed with a total"
has   "$M" "$(img A)"                                "old unused Docker image listed"
lacks "$M" "$(img B)"                                "image a stopped container uses not listed"
lacks "$M" "$(img C)"                                "image pulled recently not listed"
lacks "$M" "$(img D)"                                "new image not listed"
has   "$out" "| Docker | unused images | \`docker context default\` | 1 | 2.3 GB |" "Docker row shows context and unique size"
has   "$out" "| Codex | projectless chats | \`~/.codex/projectless/2020-01-01\` |" "table shows where a row deletes from"
has   "$out" "| OpenCode | sessions | \`~/.local/share/opencode/opencode.db\` |" "sessions row points at the database"
exists "$h/.claude/projects/-work-acme/11111111-aaaa-bbbb-cccc-000000000001.jsonl" "scan deleted nothing"

# --- 2. delete removes what the manifest lists, and only that -------------------
# Smuggled lines: a path outside the roots, a protected path, and a traversal.
printf 'path\tX|outside\t1\t%s\n' "$h/Downloads/old.zip" >> "$M"
printf 'path\tX|protected\t1\t%s\n' "$h/.claude/projects/-work-acme/memory" >> "$M"
printf 'path\tX|protected\t1\t%s\n' "$h/.claude/settings.json" >> "$M"
printf 'path\tX|traversal\t1\t%s\n' "$h/.claude/../Downloads/old.zip" >> "$M"
printf 'opencode-session\tX|bad id\t1\t%s\n' "x' OR 1=1 --" >> "$M"
printf 'docker-image\tX|used image\t1\t%s\n' "$(img B)" >> "$M"
printf 'docker-image\tX|fresh image\t1\t%s\n' "$(img C)" >> "$M"
printf 'docker-image\tX|bad image\t1\t%s\n' "app:old; rm -rf /" >> "$M"
touch "$h/.local/share/opencode/tool-output/tool_old"         # changed after the scan
out="$h/delete.out"
run "$h" delete --manifest "$M" > "$out" 2>&1; rc=$?
[ "$rc" = 0 ] && ok "delete exits 0" || bad "delete exits 0" "rc=$rc: $(cat "$out")"
gone   "$h/.claude/projects/-work-acme/11111111-aaaa-bbbb-cccc-000000000001.jsonl" "old transcript deleted"
gone   "$h/.claude/projects/-work-acme/11111111-aaaa-bbbb-cccc-000000000001" "its folder deleted"
gone   "$h/.claude/projects/-work-gone"                "emptied project folder removed"
exists "$h/.claude/projects/-work-acme/22222222-aaaa-bbbb-cccc-000000000002.jsonl" "fresh transcript kept"
exists "$h/.claude/projects/-work-acme/memory/MEMORY.md" "memory kept"
exists "$h/.claude/settings.json"                      "settings.json kept"
exists "$h/.claude/CLAUDE.md"                          "CLAUDE.md kept"
exists "$h/.claude/history.jsonl"                      "history.jsonl kept"
exists "$h/.claude/skills/mine/SKILL.md"               "skills kept"
gone   "$h/.claude/settings.json.bak-1"                "config backup deleted"
gone   "$h/.claude/plugins/cache/mkt/tool/1.0.0"       "unused plugin version deleted"
exists "$h/.claude/plugins/cache/mkt/tool/2.0.0"       "installed plugin version kept"
exists "$h/Downloads/old.zip"                          "file outside the roots kept"
exists "$h/.local/share/opencode/tool-output/tool_old" "item changed since the scan kept"
exists "$h/.config/opencode/opencode.jsonc"            "opencode.jsonc kept"
gone   "$h/.codex/projectless/2020-01-01"              "old Codex chat and its emptied date folder deleted"
exists "$h/.codex/projectless/2020-01-02/new-chat/outputs/report.md" "fresh Codex chat kept"
exists "$h/.codex/config.toml"                         "Codex config kept"
[ "$(sessions "$h")" = "ses_B ses_B1" ] && ok "old session tree deleted, live tree kept" || bad "old session tree deleted, live tree kept" "left: $(sessions "$h")"
[ "$(sqlite3 "$h/.local/share/opencode/opencode.db" 'SELECT count(*) FROM message')" = 0 ] && ok "messages cascaded" || bad "messages cascaded"
has "$out" "skipped: outside the allowed folders: 2"   "outside and traversal lines refused"
has "$out" "skipped: protected: 2"                     "protected lines refused"
has "$out" "skipped: not a session id: 1"              "injected session id refused"
has "$out" "skipped: changed since the scan: 2"        "fresh-again file and recently pulled image reported"
has "$out" "OpenCode database compacted"               "database compacted while OpenCode is closed"
grep -q "$(img A)" "$h/docker/images" && bad "old unused image removed with all its tags" || ok "old unused image removed with all its tags"
grep -q "$(img B)" "$h/docker/images" && ok "image in use kept" || bad "image in use kept"
grep -q "$(img C)" "$h/docker/images" && ok "recently pulled image kept" || bad "recently pulled image kept"
has "$out" "skipped: a container uses it: 1"           "used image line refused"
has "$out" "skipped: not an image id: 1"               "bad image id refused"

# --- 3. a running Desktop app keeps its caches out ------------------------------
h="$(new_home)"; M="$h/manifest.tsv"; echo Claude > "$h/running"
run "$h" scan --out "$M" > "$h/scan.out" 2>&1
lacks "$M" "Cache_Data"                                "caches not listed while the app runs"
has   "$h/scan.out" "Claude Desktop caches skipped"    "the skip is reported"
has   "$M" "local_old.json"                            "Desktop sessions still listed"
# Scanned closed, deleted while running: the cache line is skipped.
rm "$h/running"; run "$h" scan --out "$M" > /dev/null 2>&1; echo Claude > "$h/running"
run "$h" delete --manifest "$M" > "$h/delete.out" 2>&1
exists "$(desk_of "$h")/Cache/Cache_Data/f_0001"                    "cache kept when the app starts before delete"
has   "$h/delete.out" "skipped: Claude Desktop is running" "that skip is reported"

# --- 4. OpenCode running: sessions deleted, VACUUM deferred ---------------------
h="$(new_home)"; M="$h/manifest.tsv"; echo opencode > "$h/running"
run "$h" scan --out "$M" > /dev/null 2>&1; run "$h" delete --manifest "$M" > "$h/delete.out" 2>&1
has "$h/delete.out" "OpenCode is running, so its database was not compacted" "VACUUM deferred with instructions"

# --- 5. Codex chats outside CODEX_HOME are never a root ------------------------
h="$(new_home)"; M="$h/manifest.tsv"
printf '[desktop]\nprojectlessWorkspaceRoot = "%s"\n' "$h/Documents/Codex" > "$h/.codex/config.toml"
run "$h" scan --out "$M" > "$h/scan.out" 2>&1
lacks "$M" "Documents"                                 "Documents root not listed"
has   "$h/scan.out" "Codex chats skipped: their folder" "outside root reported"
printf 'path\tX|y\t1\t%s\n' "$h/Documents/Codex/2020-01-01/doc-chat" >> "$M"
run "$h" delete --manifest "$M" > /dev/null 2>&1
exists "$h/Documents/Codex/2020-01-01/doc-chat/a.md"   "Documents chat refused by delete"
printf '[desktop]\nprojectlessWorkspaceRoot = "%s"\n' "$h/.codex/../Documents/Codex" > "$h/.codex/config.toml"
run "$h" scan --out "$M" > "$h/scan.out" 2>&1
lacks "$M" "Documents"                                 "a .. root escaping CODEX_HOME not listed"
rm "$h/.codex/config.toml"; run "$h" scan --out "$M" > "$h/scan.out" 2>&1
has   "$h/scan.out" "desktop.projectlessWorkspaceRoot is not set" "unset root reported"

# --- 6. Docker not reachable ---------------------------------------------------
h="$(new_home)"; M="$h/manifest.tsv"; touch "$h/docker/down"
run "$h" scan --out "$M" > "$h/scan.out" 2>&1
lacks "$M" "docker-image"                              "no images listed while Docker is down"
has   "$h/scan.out" "Docker images skipped"            "the skip is reported"

# --- 7. argument checks --------------------------------------------------------
h="$(new_home)"
run "$h" scan --days 0 --out "$h/m" > /dev/null 2>&1; [ $? = 2 ] && ok "--days 0 refused" || bad "--days 0 refused"
run "$h" scan > /dev/null 2>&1;                        [ $? = 2 ] && ok "missing --out refused" || bad "missing --out refused"
printf 'path\tX|y\t1\t%s\n' "$h/.claude/todos/old.json" > "$h/noheader"
run "$h" delete --manifest "$h/noheader" > /dev/null 2>&1; [ $? = 2 ] && ok "manifest without days header refused" || bad "manifest without days header refused"
exists "$h/.claude/todos/old.json"                     "nothing deleted from a headerless manifest"

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" = 0 ]
