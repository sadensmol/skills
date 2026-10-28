#!/usr/bin/env bash
# Finds Claude Code, Claude Desktop, Codex chat and OpenCode data, and Docker images,
# older than N days, and deletes exactly what a reviewed manifest lists — no file
# outside those tools' folders.
#
#   usage: cleanup.sh scan   [--days N] --out MANIFEST   list old items, write the manifest, print the table
#          cleanup.sh table  --manifest MANIFEST          print the table of a (filtered) manifest
#          cleanup.sh delete --manifest MANIFEST          delete what the manifest lists
#
# Scan never deletes. Delete re-checks every manifest line: the target must sit
# inside one of the ROOTS below, must not be protected, and must still be older
# than N days. A line that fails a check is skipped, never forced.
#
# Manifest: '#'-prefixed header lines, then one tab-separated line per item:
#   kind  category  size-in-KB  target
# kind is `path` (a file or folder), `app-cache` (a path inside Claude Desktop's
# caches, deleted only while the app is closed), `opencode-session` (a session
# id, deleted through the OpenCode CLI so the database cascades it) or
# `docker-image` (an image id in the current Docker context, removed without force).
set -uo pipefail
shopt -s nullglob

DAYS=30
CC="${CLAUDE_CONFIG_DIR:-$HOME/.claude}"
if [ "$(uname -s)" = Darwin ]; then
  CC_CACHE="$HOME/Library/Caches/claude-cli-nodejs"
  DESKTOP="$HOME/Library/Application Support/Claude"
else
  CC_CACHE="${XDG_CACHE_HOME:-$HOME/.cache}/claude-cli-nodejs"
  DESKTOP="${XDG_CONFIG_HOME:-$HOME/.config}/Claude"
fi
OC_DATA="${XDG_DATA_HOME:-$HOME/.local/share}/opencode"
OC_CONFIG="${XDG_CONFIG_HOME:-$HOME/.config}/opencode"
OC_STATE="${XDG_STATE_HOME:-$HOME/.local/state}/opencode"
OC_DB="$OC_DATA/opencode.db"
CODEX="${CODEX_HOME:-$HOME/.codex}"
# The Codex app's folder for chats outside a project (desktop.projectlessWorkspaceRoot).
# It is a root only when it sits inside CODEX_HOME; its default is ~/Documents/Codex.
CODEX_CHATS="$(sed -n '/^\[desktop\]/,/^\[/s/^projectlessWorkspaceRoot *= *"\(.*\)" *$/\1/p' "$CODEX/config.toml" 2>/dev/null | head -1)"
ROOTS=("$CC" "$CC_CACHE" "$DESKTOP" "$OC_DATA" "$OC_CONFIG" "$OC_STATE")
case "$CODEX_CHATS" in *'/..'*|*'/./'*) CODEX_CHATS="(a path with . or ..)" ;; esac
case "$CODEX_CHATS" in "$CODEX"/?*) ROOTS+=("$CODEX_CHATS") ;; esac
# Chromium caches of the Desktop app; the app owns their index while it runs.
APP_CACHES=("Cache" "Code Cache" "GPUCache" "DawnGraphiteCache" "DawnWebGPUCache" "GraphiteDawnCache")

die() { echo "cleanup: $*" >&2; exit 2; }

# fresh <path>: true when the path, or anything under it, changed within DAYS.
fresh() { [ -n "$(find "$1" -mtime "-$DAYS" -print -quit 2>/dev/null)" ]; }

desktop_running() { pgrep -x Claude >/dev/null 2>&1 || pgrep -f claude-desktop >/dev/null 2>&1; }
opencode_running() { pgrep -x opencode >/dev/null 2>&1; }

note() { printf '%s\n' "$1" >> "$NOTES"; }
add()  { printf '%s\t%s\t%s\n' "$1" "$2" "$3" >> "$CAND"; }    # add <kind> <category> <path>

# old_entries <category> <dir> [glob...]: direct children of dir (matching a glob,
# default all) with nothing under them changed within DAYS.
old_entries() {
  local cat="$1" dir="$2" g p; shift 2
  [ -d "$dir" ] || return 0
  [ $# -gt 0 ] || set -- '*'
  for g in "$@"; do
    for p in "$dir"/$g; do
      p="${p%/}"                                               # a '*/' glob selects folders only
      fresh "$p" || add path "$cat" "$p"
    done
  done
}

scan_claude_code() {
  [ -d "$CC" ] || return 0
  local p s d inst newest
  # A session is <id>.jsonl plus an optional <id>/ folder (subagents, tool results).
  for p in "$CC"/projects/*/*.jsonl; do
    s="${p%.jsonl}"
    fresh "$p" && continue
    [ -d "$s" ] && fresh "$s" && continue
    add path "Claude Code|session transcripts" "$p"
    [ -d "$s" ] && add path "Claude Code|session subagents + tool results" "$s"
  done
  for s in "$CC"/projects/*/*-*-*-*-*/; do                     # a session folder whose transcript is gone
    s="${s%/}"
    [ -e "$s.jsonl" ] || fresh "$s" || add path "Claude Code|session subagents + tool results" "$s"
  done
  old_entries "Claude Code|file history" "$CC/file-history"
  old_entries "Claude Code|jobs" "$CC/jobs" '*/'               # pins.json beside them is state
  for d in todos plans shell-snapshots session-env session-state sessions paste-cache debug telemetry statsig cache backups; do
    old_entries "Claude Code|$d" "$CC/$d"
  done
  old_entries "Claude Code|security state + rotated logs" "$CC/security" 'security_warnings_state_*' 'log.txt.*'
  old_entries "Claude Code|config backups" "$CC" '*.bak' '*.bak[.-]*' '*.backup*'
  old_entries "Claude Code|temp files" "$CC" '*.tmp'
  old_entries "Claude Code|temp files" "$CC/plugins" '*.tmp'
  old_entries "Claude Code|MCP log cache" "$CC_CACHE"

  # Plugin versions no install record points at. The newest version of each
  # plugin stays even then, and without the record nothing is touched.
  inst="$CC/plugins/installed_plugins.json"
  if [ -f "$inst" ]; then
    for d in "$CC"/plugins/cache/*/*/; do
      newest="$(ls -td "$d"*/ 2>/dev/null | head -1)"
      for p in "$d"*/; do
        [ "$p" = "$newest" ] && continue
        p="${p%/}"
        grep -qF "\"$p\"" "$inst" && continue
        fresh "$p" || add path "Claude Code|unused plugin versions" "$p"
      done
    done
  fi
}

scan_desktop() {
  [ -d "$DESKTOP" ] || return 0
  local d newest p c
  for d in "$DESKTOP"/local-agent-mode-sessions/*/*/; do old_entries "Claude Desktop|agent-mode sessions" "${d%/}" 'local_*'; done
  for d in "$DESKTOP"/claude-code-sessions/*/*/;      do old_entries "Claude Desktop|code sessions" "${d%/}" 'local_*'; done
  for d in "$DESKTOP"/scratch-workspaces/*/*/;        do old_entries "Claude Desktop|scratch workspaces" "${d%/}" 'scratch-*'; done
  newest="$(ls -td "$DESKTOP"/claude-code/*/ 2>/dev/null | head -1)"
  for p in "$DESKTOP"/claude-code/*/; do
    [ "$p" = "$newest" ] && continue
    fresh "$p" || add path "Claude Desktop|old bundled Claude Code" "${p%/}"
  done
  if desktop_running; then
    note "Claude Desktop caches skipped: the app is running. Quit it and scan again to include them."
    return 0
  fi
  for c in "${APP_CACHES[@]}"; do
    [ -d "$DESKTOP/$c" ] || continue
    find "$DESKTOP/$c" -type f ! -mtime "-$DAYS" 2>/dev/null |
      while IFS= read -r p; do add app-cache "Claude Desktop|browser caches" "$p"; done
  done
}

# Whole session trees only: a root and every descendant older than DAYS, listed
# deepest first so each CLI delete removes one session.
scan_opencode_sessions() {
  [ -f "$OC_DB" ] || return 0
  command -v sqlite3 >/dev/null || { note "OpenCode sessions skipped: sqlite3 is not on PATH."; return 0; }
  command -v opencode >/dev/null || { note "OpenCode sessions skipped: the opencode CLI deletes them, and it is not on PATH."; return 0; }
  local cutoff=$(( ($(date +%s) - DAYS * 86400) * 1000 ))
  sqlite3 -readonly -separator $'\t' "$OC_DB" "
    WITH RECURSIVE tree(root, id, depth) AS (
      SELECT id, id, 0 FROM session
       WHERE parent_id IS NULL OR parent_id NOT IN (SELECT id FROM session)
      UNION ALL
      SELECT tree.root, s.id, tree.depth + 1 FROM session s JOIN tree ON s.parent_id = tree.id),
    stale AS (
      SELECT tree.root FROM tree JOIN session s ON s.id = tree.id
       GROUP BY tree.root HAVING max(s.time_updated) < $cutoff)
    SELECT 'opencode-session', 'OpenCode|sessions',
           (coalesce((SELECT sum(length(data)) FROM message WHERE session_id = t.id), 0)
          + coalesce((SELECT sum(length(data)) FROM part    WHERE session_id = t.id), 0)) / 1024,
           t.id
      FROM tree t JOIN stale ON stale.root = t.root
     ORDER BY t.depth DESC;" >> "$ROWS" || note "OpenCode sessions skipped: the database could not be read."
}

# Chat folders are <root>/<YYYY-MM-DD>/<chat>; each goes once nothing in it is fresh.
scan_codex() {
  local d
  case "$CODEX_CHATS" in
    "$CODEX"/?*) ;;
    "") [ -d "$CODEX" ] && note "Codex chats skipped: desktop.projectlessWorkspaceRoot is not set, so they live in ~/Documents/Codex. Point it inside $CODEX to include them."; return 0 ;;
    *) note "Codex chats skipped: their folder $CODEX_CHATS is outside $CODEX."; return 0 ;;
  esac
  for d in "$CODEX_CHATS"/*/; do old_entries "Codex|projectless chats" "${d%/}"; done
}

# Docker keeps no last-used time, so an image counts as unused for DAYS when no
# container (running or stopped) uses it and it was neither built nor tagged or
# pulled within DAYS. Sizes are unique sizes: layers other images share stay.
docker_up() { command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; }
docker_used() { local c; c="$(docker ps -aq)" || return 1; [ -z "$c" ] || docker inspect --format '{{.Image}}' $c; }
# LastTagTime prints in UTC, as RFC 3339 or as "2006-01-02 15:04:05 +0000 UTC";
# either way its date and time, joined by T, sort as time.
docker_old() {
  local c=$(( $(date +%s) - DAYS * 86400 )) cut ids
  cut="$(date -u -r "$c" +%Y-%m-%dT%H:%M:%S 2>/dev/null || date -u -d "@$c" +%Y-%m-%dT%H:%M:%S)"
  ids="$(docker image ls -a -q --no-trunc --filter "until=$((DAYS * 24))h" | sort -u)" || return 1
  [ -z "$ids" ] || docker image inspect --format '{{.Id}} {{.Metadata.LastTagTime}}' $ids |
    awk -v c="$cut" 'length($1) == 71 && $1 ~ /^sha256:[0-9a-f]+$/ {
      t = $3 ~ /^[0-9][0-9]:/ ? $2 "T" $3 : $2; if (substr(t, 1, 19) < c) print $1 }'
}

scan_docker() {
  command -v docker >/dev/null 2>&1 || return 0
  docker_up || { note "Docker images skipped: the Docker daemon is not reachable. Start Docker and scan again to include them."; return 0; }
  local t; t="$(mktemp -d)"
  if ! docker_used > "$t/used" || ! docker_old > "$t/all"; then
    note "Docker images skipped: Docker could not list its images or containers."; rm -rf "$t"; return 0
  fi
  sort -u "$t/used" -o "$t/used"
  sort -u "$t/all" | comm -23 - "$t/used" > "$t/old"
  if [ -s "$t/old" ]; then
    docker system df -v --format '{{range .Images}}{{.ID}} {{.UniqueSize}}
{{end}}' > "$t/sizes" 2>/dev/null
    awk 'function kb(s,   n, u) { n = s + 0; u = s; sub(/^[0-9.]+/, "", u)
           return int(n * (u == "TB" ? 1e12 : u == "GB" ? 1e9 : u == "MB" ? 1e6 : u ~ /^[kK]B$/ ? 1e3 : 1) / 1024) }
         NR == FNR { size[$1] = kb($2); next }
         { printf "docker-image\tDocker|unused images\t%d\t%s\n", size[$1], $1 }' "$t/sizes" "$t/old" >> "$ROWS"
  fi
  rm -rf "$t"
}

scan_opencode() {
  scan_opencode_sessions
  old_entries "OpenCode|snapshots" "$OC_DATA/snapshot"
  old_entries "OpenCode|tool output" "$OC_DATA/tool-output"
  old_entries "OpenCode|logs" "$OC_DATA/log"
  old_entries "OpenCode|config backups" "$OC_CONFIG" '*.bak' '*.bak[.-]*' '*.pre-*' '*.orig'
  old_entries "OpenCode|stale locks" "$OC_STATE/locks"
}

# The Where column is the deepest folder that holds every item of a row.
print_table() {
  local ctx=""; command -v docker >/dev/null 2>&1 && ctx="$(docker context show 2>/dev/null)"
  awk -F'\t' -v home="$HOME" -v db="$OC_DB" -v ctx="${ctx:-default}" '
    function h(x) { if (x >= 1048576) return sprintf("%.1f GB", x / 1048576)
                    if (x >= 1024) return sprintf("%.1f MB", x / 1024); return sprintf("%d KB", x) }
    function parent(p) { sub(/\/[^\/]*$/, "", p); return p }
    function common(a, b,   i, n) {
      if (a == b) return a
      n = length(a) < length(b) ? length(a) : length(b)
      for (i = 1; i <= n && substr(a, i, 1) == substr(b, i, 1); i++) ;
      if (substr(a, i - 1, 1) == "/") return substr(a, 1, i - 2)
      if ((i - 1 == length(a) && substr(b, i, 1) == "/") || (i - 1 == length(b) && substr(a, i, 1) == "/")) return substr(a, 1, i - 1)
      return parent(substr(a, 1, i - 1))
    }
    function tilde(p) { return index(p, home "/") == 1 ? "~" substr(p, length(home) + 1) : p }
    /^#/ { next }
    { d = $1 == "opencode-session" ? db : $1 == "docker-image" ? "docker context " ctx : parent($4)
      if (!($2 in n)) { order[++k] = $2; where[$2] = d } else where[$2] = common(where[$2], d)
      n[$2]++; kb[$2] += $3; tn++; tk += $3 }
    END {
      if (!tn) { print "Nothing to clean."; exit }
      print "| Tool | What | Where | Items | Size |"; print "|---|---|---|---:|---:|"
      for (i = 1; i <= k; i++) { c = order[i]; split(c, a, "|")
        printf "| %s | %s | `%s` | %d | %s |\n", a[1], a[2], tilde(where[c]), n[c], h(kb[c]) }
      printf "| **Total** | | | **%d** | **%s** |\n", tn, h(tk)
    }' "$1"
}

cmd_scan() {
  local out=""
  while [ $# -gt 0 ]; do
    case "$1" in
      --days) DAYS="${2:-}"; shift 2 ;;
      --out)  out="${2:-}"; shift 2 ;;
      *) die "unknown argument: $1" ;;
    esac
  done
  [ -n "$out" ] || die "scan needs --out MANIFEST"
  case "$DAYS" in ''|*[!0-9]*|0) die "--days must be a positive whole number" ;; esac
  local tmp; tmp="$(mktemp -d)"
  CAND="$tmp/cand"; ROWS="$tmp/rows"; NOTES="$tmp/notes"; : > "$CAND"; : > "$ROWS"; : > "$NOTES"

  scan_claude_code; scan_desktop; scan_codex; scan_opencode; scan_docker

  # Sizes for the path items, in one pass: du's output joined back by path.
  : > "$tmp/sizes"
  [ -s "$CAND" ] && cut -f3 "$CAND" | tr '\n' '\0' | xargs -0 du -sk 2>/dev/null > "$tmp/sizes"
  awk -F'\t' 'NR == FNR { kb[$2] = $1; next } { print $1 "\t" $2 "\t" ($3 in kb ? kb[$3] : 0) "\t" $3 }' \
    "$tmp/sizes" "$CAND" >> "$ROWS"
  {
    printf '# cleanup manifest\n# days=%s\n# created=%s\n' "$DAYS" "$(date '+%Y-%m-%d %H:%M:%S %z')"
    sort -s -t$'\t' -k2,2 "$ROWS"
  } > "$out"

  echo "Older than $DAYS days:"; echo
  print_table "$out"
  [ -s "$NOTES" ] && { echo; cat "$NOTES"; }
  echo; echo "Manifest: $out"
  rm -rf "$tmp"
}

cmd_table() {
  [ "${1:-}" = --manifest ] && [ -f "${2:-}" ] || die "table needs --manifest MANIFEST"
  print_table "$2"
}

# inside_root <path>: prints the root the path sits strictly inside.
inside_root() {
  local r
  case "$1" in *'/../'*|*'/./'*|*/..|*/.) return 1 ;; esac
  for r in "${ROOTS[@]}"; do
    case "$1" in "$r"/?*) echo "$r"; return 0 ;; esac
  done
  return 1
}

# protected <path> <root>: config, credentials, memory and installed extensions.
protected() {
  case "/${1#"$2"/}" in
    */memory|*/memory/*|*/skills|*/skills/*|*/agents|*/agents/*|*/commands|*/commands/*|*/command|*/command/*) return 0 ;;
    */settings.json|*/settings.local.json|*/CLAUDE.md|*/AGENTS.md|*/history.jsonl|*/installed_plugins.json) return 0 ;;
    */opencode.db|*/opencode.db-*|*/auth.json|*/mcp-auth.json|*/opencode.json|*/opencode.jsonc|*/tui.json) return 0 ;;
    /vm_bundles|/vm_bundles/*) return 0 ;;
  esac
  return 1
}

cmd_delete() {
  [ "${1:-}" = --manifest ] && [ -f "${2:-}" ] || die "delete needs --manifest MANIFEST"
  local manifest="$2" kind cat kb target root res ran_oc=0 app_up=0 cutoff
  DAYS="$(sed -n 's/^# days=//p' "$manifest" | head -1)"
  case "$DAYS" in ''|*[!0-9]*|0) die "the manifest has no valid '# days=' header" ;; esac
  cutoff=$(( ($(date +%s) - DAYS * 86400) * 1000 ))
  desktop_running && app_up=1
  local tmp dk=0; tmp="$(mktemp -d)"; : > "$tmp/res"
  # Re-checked once for all image lines: what containers use, and what is still old.
  if grep -q $'^docker-image\t' "$manifest" && docker_up &&
     docker_used > "$tmp/docker-used" && docker_old > "$tmp/docker-old"; then
    dk=1
  fi

  while IFS=$'\t' read -r kind cat kb target <&3; do
    case "$kind" in '#'*|'') continue ;; esac
    res=deleted
    case "$kind" in
      path|app-cache)
        if ! root="$(inside_root "$target")"; then res="skipped: outside the allowed folders"
        elif protected "$target" "$root"; then res="skipped: protected"
        elif [ ! -e "$target" ] && [ ! -L "$target" ]; then res="skipped: already gone"
        elif [ "$kind" = app-cache ] && [ "$app_up" = 1 ]; then res="skipped: Claude Desktop is running"
        elif fresh "$target"; then res="skipped: changed since the scan"
        elif ! rm -rf -- "$target"; then res=failed
        fi ;;
      opencode-session)
        if [[ ! "$target" =~ ^[A-Za-z0-9_]+$ ]]; then res="skipped: not a session id"
        elif [ -z "$(sqlite3 -readonly "$OC_DB" "SELECT 1 FROM session WHERE id = '$target'")" ]; then res="skipped: already gone"
        elif [ -z "$(sqlite3 -readonly "$OC_DB" "SELECT 1 FROM session WHERE id = '$target' AND time_updated < $cutoff")" ]; then res="skipped: changed since the scan"
        else
          opencode session delete "$target" </dev/null >/dev/null 2>&1
          ran_oc=1
          [ -z "$(sqlite3 -readonly "$OC_DB" "SELECT 1 FROM session WHERE id = '$target'")" ] || res=failed
        fi ;;
      docker-image)
        if [[ ! "$target" =~ ^sha256:[0-9a-f]{64}$ ]]; then res="skipped: not an image id"
        elif [ "$dk" = 0 ]; then res="skipped: Docker is not reachable"
        elif ! docker image inspect "$target" >/dev/null 2>&1; then res="skipped: already gone"
        elif grep -qxF "$target" "$tmp/docker-used"; then res="skipped: a container uses it"
        elif ! grep -qxF "$target" "$tmp/docker-old"; then res="skipped: changed since the scan"
        else
          local tags; tags="$(docker image inspect --format '{{range .RepoTags}}{{.}} {{end}}' "$target")"
          docker image rm ${tags:-$target} </dev/null >/dev/null 2>&1
          docker image inspect "$target" >/dev/null 2>&1 && res=failed
        fi ;;
      *) res="skipped: unknown kind" ;;
    esac
    printf '%s\t%s\t%s\t%s\n' "$cat" "$res" "$kb" "$target" >> "$tmp/res"
  done 3< "$manifest"

  # Project and date folders the deletes emptied.
  [ -d "$CC/projects" ] && find "$CC/projects" -mindepth 1 -maxdepth 1 -type d -empty -delete 2>/dev/null
  case "$CODEX_CHATS" in "$CODEX"/?*) [ -d "$CODEX_CHATS" ] && find "$CODEX_CHATS" -mindepth 1 -maxdepth 1 -type d -empty -delete 2>/dev/null ;; esac

  awk -F'\t' '
    function h(x) { if (x >= 1048576) return sprintf("%.1f GB", x / 1048576)
                    if (x >= 1024) return sprintf("%.1f MB", x / 1024); return sprintf("%d KB", x) }
    { if (!($1 in seen)) { seen[$1] = 1; order[++k] = $1 }
      if ($2 == "deleted") { d[$1]++; kb[$1] += $3; td++; tk += $3 } else if ($2 == "failed") { f[$1]++; tf++ } else { s[$1]++; ts++ } }
    END {
      print "| Tool | What | Deleted | Skipped | Failed | Freed |"; print "|---|---|---:|---:|---:|---:|"
      for (i = 1; i <= k; i++) { c = order[i]; split(c, a, "|")
        printf "| %s | %s | %d | %d | %d | %s |\n", a[1], a[2], d[c], s[c], f[c], h(kb[c]) }
      printf "| **Total** | | **%d** | **%d** | **%d** | **%s** |\n", td, ts, tf, h(tk)
    }' "$tmp/res"
  if grep -qv $'\tdeleted\t' "$tmp/res"; then
    echo; echo "Not deleted (reason, count):"
    awk -F'\t' '$2 != "deleted" { n[$2]++ } END { for (r in n) printf "  %s: %d\n", r, n[r] }' "$tmp/res"
  fi

  # SQLite keeps freed pages until VACUUM, and VACUUM needs the database to itself.
  if [ "$ran_oc" = 1 ]; then
    if opencode_running; then
      echo; echo "OpenCode is running, so its database was not compacted. Close it, then run: sqlite3 \"$OC_DB\" 'VACUUM;'"
    else
      local before after
      before="$(du -sk "$OC_DB" | cut -f1)"
      sqlite3 "$OC_DB" 'VACUUM;' && after="$(du -sk "$OC_DB" | cut -f1)" &&
        echo && echo "OpenCode database compacted: $((before / 1024)) MB -> $((after / 1024)) MB."
    fi
  fi
  rm -rf "$tmp"
}

case "${1:-}" in
  scan)   shift; cmd_scan "$@" ;;
  table)  shift; cmd_table "$@" ;;
  delete) shift; cmd_delete "$@" ;;
  *) sed -n '2,10p' "$0" | sed 's/^# \{0,1\}//'; exit 2 ;;
esac
