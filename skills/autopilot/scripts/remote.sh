#!/usr/bin/env bash
# remote.sh [--size SIZE] [--force] [--alias NAME] — `/autopilot remote` on the Mac: bring this workspace's
# autopilot up in its remote box, in ONE agterm-server session of the box that is attached here, right
# after the calling session. The calling session is never touched and no window opens.
#   1. The box answers over ssh, else remote-setup.sh installs it; agterm-server runs in it.
#   2. A new box session (in the box workspace named like this one) runs box-session.sh from this skill as
#      its command: disk passphrase and sign-ins typed by the owner there, then the workspace, repos,
#      skills, plugins and checks, then `claude /autopilot` in that same session (`auto main`).
#      When `auto main` already runs, a short `autopilot sync` session does the same without Claude, and
#      `auto main` is attached instead.
#   3. This script sends the data the box cannot read from the Mac (manifest, .workspace/, skills that are
#      not in a git repo, the router routes files) once the session reports `connected`.
# A workspace has one remote, so it takes no host: the box comes from .workspace/autopilot/remote.yaml, else
# from --alias, else from the one `# autopilot-remote` block of ~/.ssh/config, which remote setup writes.
# --force moves .workspace/ in again (the box keeps a backup). Last line: `AUTO_MAIN=<local session id>`.
set -euo pipefail

die() { echo "remote: $*" >&2; exit 1; }

alias="" size="" force=0
while [ $# -gt 0 ]; do
  case "$1" in
    --alias) alias="${2:?--alias needs a value}"; shift 2 ;;
    --size) size="${2:?--size needs a value}"; shift 2 ;;
    --force) force=1; shift ;;
    -*) die "unknown option $1" ;;
    *) die "it takes no ssh host: a workspace has one remote (remote.yaml, else the box of remote setup)" ;;
  esac
done
[ -z "$size" ] || [[ "$size" =~ ^[0-9]+[MGT]$ ]] || die "the size must look like 100G"

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
ctl="${AGTERMCTL:-agtermctl}"
command -v "$ctl" >/dev/null || die "agtermctl is not on PATH"
[ -n "${AGTERM_SESSION_ID:-}" ] && [ -n "${AGTERM_WINDOW_ID:-}" ] || die "run it in an agterm session"
socket=()
[ -n "${AGTERM_SOCKET:-}" ] && socket=(--socket "$AGTERM_SOCKET")
L() { "$ctl" "$@" "${socket[@]}"; }

W="$PWD"
while [ "$W" != / ] && [ ! -d "$W/.workspace/autopilot" ]; do W="$(dirname "$W")"; done
[ -d "$W/.workspace/autopilot" ] || die "no folder above $PWD holds .workspace/autopilot/"
for f in settings.json autopilot/config.yaml autopilot/secrets.env; do
  [ -f "$W/.workspace/$f" ] || die "missing $W/.workspace/$f"
done
ws="$(jq -r '.agterm_workspace // empty' "$W/.workspace/settings.json")"
[ -n "$ws" ] || die "key agterm_workspace is missing in $W/.workspace/settings.json"
[[ "$ws" =~ ^[A-Za-z0-9._-]+$ ]] || die "agterm_workspace may hold letters, digits, dot, dash and underscore only"
cfg="$W/.workspace/autopilot/config.yaml"
remote_file="$W/.workspace/autopilot/remote.yaml"

# --- the box: remote.yaml, else the one block of ~/.ssh/config ---
host=""
yaml_value() { sed -n "s/^$1: *\"\{0,1\}\([^\"]*\)\"\{0,1\} *$/\1/p" "$remote_file"; }
if [ -f "$remote_file" ]; then
  had="$(yaml_value box_alias)"
  [ -z "$alias" ] || [ "$alias" = "$had" ] || die "this workspace already has the remote $had: remove $remote_file to move it"
  alias="$had"
  host="$(yaml_value ssh_host)"
fi
if [ -z "$alias" ]; then
  blocks="$(sed -n 's/^# autopilot-remote \(.*\) begin$/\1/p' "$HOME/.ssh/config" 2>/dev/null || true)"
  [ -n "$blocks" ] || die "no box yet: run /autopilot remote setup <ssh-host> first"
  [ "$(wc -l <<<"$blocks")" -eq 1 ] || die "several boxes in ~/.ssh/config ($(tr '\n' ' ' <<<"$blocks")): pass --alias NAME"
  alias="$blocks"
fi
[[ "$alias" =~ ^[A-Za-z0-9._-]+$ ]] || die "the alias may hold letters, digits, dot, dash and underscore only"
block_value() {
  awk -v a="$alias" -v k="$1" '$0 == "# autopilot-remote " a " begin" {f=1; next}
    $0 == "# autopilot-remote " a " end" {f=0} f && $1 == k {print $2}' "$HOME/.ssh/config" 2>/dev/null
}
[ -n "$host" ] || host="$(block_value ProxyJump)"
[ -n "$host" ] || die "no ~/.ssh/config block names the host of $alias: run /autopilot remote setup <ssh-host>"
port="$(block_value Port)"

B() { ssh -o BatchMode=yes -o ConnectTimeout=8 -o ServerAliveInterval=5 -o ServerAliveCountMax=3 "$alias" "$@"; }
bctl="\$HOME/.local/bin/agtermctl"

echo "== box $alias (through $host)"
if B true 2>/dev/null; then
  echo "   answers"
else
  echo "   does not answer: installing it"
  args=("$host" --alias "$alias")
  [ -z "$port" ] || args+=(--port "$port")
  bash "$here/remote-setup.sh" "${args[@]}" || die "remote-setup failed"
fi
if ! B "test -x $bctl"; then
  set +e; "$ctl" server install "$alias"; rc=$?; set -e
  case "$rc" in 0|3) ;; *) die "agtermctl server install failed (exit $rc)" ;; esac
fi
B "test -x $bctl" || die "agterm-server is missing in the box"
echo "   agterm-server runs"

if [ ! -f "$remote_file" ] && (cd "$W" && autopilot ping >/dev/null 2>&1); then
  die "autopilot runs for this workspace on this Mac: stop it first (/autopilot stop), one service per workspace"
fi

# --- the data the box cannot read from the Mac ---
echo "== skills, plugins and repos of this Mac"
data="$(mktemp -d)"
chmod 700 "$data"
trap 'rm -rf "$data"' EXIT
box_home="$(B 'printf %s "$HOME"')"
dest="$box_home/workspaces/$ws"
realpath_of() { python3 -I -c 'import os, sys; print(os.path.realpath(sys.argv[1]))' "$1"; }
git_top() { git -C "$1" rev-parse --show-toplevel 2>/dev/null; }
github_slug() {
  local rest
  rest="$(sed -nE 's#^(git@github\.com:|ssh://git@github\.com/|https://github\.com/)##p' <<<"$1")"
  rest="${rest%/}"; rest="${rest%.git}"
  if [[ "$rest" =~ ^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$ ]]; then printf '%s' "$rest"; fi
}
slug_of() { github_slug "$(git -C "$1" remote get-url origin 2>/dev/null || true)"; }
box_path() {
  case "$1" in
    "$W"/*) printf '%s/%s' "$dest" "${1#"$W"/}" ;;
    *) printf '%s/skill-repos/%s' "$box_home" "$(basename "$1")" ;;
  esac
}
mirrors=()
add_mirror() {
  [ -n "$(slug_of "$1")" ] || return 1
  case " ${mirrors[*]-} " in *" $1 "*) return 0 ;; esac
  mirrors+=("$1")
}
warn() { echo "   NOT READY: $*"; }
m="$data/manifest"
put() { local IFS=$'\t'; printf '%s\n' "$*" >> "$m"; }
put mac_home "$HOME"
put mac_workspace "$W"
put force "$force"

copies=()
for entry in "$HOME"/.claude/skills/*; do
  name="$(basename "$entry")"
  rp="$(realpath_of "$entry")"
  [ -f "$rp/SKILL.md" ] || continue
  top="$(git_top "$rp" || true)"
  if [ -n "$top" ] && add_mirror "$top"; then put skill "$name" "$(box_path "$top")${rp#"$top"}"
  else copies+=("$name"); put copy "$name"; fi
done
for rel in agents CLAUDE.md; do
  p="$HOME/.claude/$rel"
  [ -L "$p" ] || continue
  rp="$(realpath_of "$p")"; d="$rp"; [ -d "$d" ] || d="$(dirname "$rp")"
  top="$(git_top "$d" || true)"
  if [ -n "$top" ] && add_mirror "$top"; then put link "$rel" "$(box_path "$top")${rp#"$top"}"
  else warn "~/.claude/$rel is not in a github.com repo: the box cannot get it"; fi
done
km="$HOME/.claude/plugins/known_marketplaces.json"
if [ -f "$km" ]; then
  while IFS=$'\t' read -r mname mkind msrc; do
    case "$mkind" in
      github) put market "$mname" github "$msrc" ;;
      directory)
        top="$(git_top "$(realpath_of "$msrc")" || true)"
        if [ -n "$top" ] && add_mirror "$top"; then put market "$mname" directory "$(box_path "$top")"
        else warn "marketplace $mname is not in a github.com repo: the box cannot get it"; fi ;;
      *) warn "marketplace $mname has the source type $mkind, which the box does not mirror" ;;
    esac
  done < <(jq -r 'to_entries[] | [.key, (.value.source.source // ""), (.value.source.repo // .value.source.path // "")] | @tsv' "$km")
fi
ip="$HOME/.claude/plugins/installed_plugins.json"
if [ -f "$ip" ]; then
  while read -r id; do put plugin "$id"; done < <(jq -r '.plugins | keys[]' "$ip")
fi
src_dir="$(sed -n 's/^ *source_dir: *"\{0,1\}\([^"]*\)"\{0,1\} *$/\1/p' "$cfg" | head -n 1)"
[ -d "$src_dir" ] || die "service.source_dir of config.yaml is not a folder on this Mac: $src_dir"
src_top="$(git_top "$src_dir")" || die "service.source_dir is not in a git repo: $src_dir"
add_mirror "$src_top" || warn "the autopilot source $src_top has no github.com origin"
projects=0
for pf in "$W"/.workspace/projects/*.json; do
  [ -f "$pf" ] || continue
  for name in $(jq -r '.repos[]?.name' "$pf"); do
    mac="$(sed -n "s/^ *$name: *\"\{0,1\}\([^\"#]*\)\"\{0,1\} *$/\1/p" "$cfg" | head -n 1)"
    [ -n "$mac" ] || mac="$W/$name"
    [ -d "$mac/.git" ] || { warn "project repo $name has no clone at $mac"; continue; }
    slug="$(slug_of "$mac")"
    [ -n "$slug" ] || { warn "project repo $name has no github.com origin"; continue; }
    [ -z "$(git -C "$mac" status --porcelain)" ] || echo "   note: $mac has uncommitted changes; they stay on this Mac"
    put project "$(box_path "$mac")" "$slug"; put path "$mac" "$(box_path "$mac")"
    projects=$((projects + 1))
  done
done
for top in ${mirrors[@]+"${mirrors[@]}"}; do
  branch="$(git -C "$top" symbolic-ref --short HEAD 2>/dev/null || true)"
  [ -n "$branch" ] || { warn "$top is on a detached HEAD: check out a branch"; continue; }
  put mirror "$(box_path "$top")" "$(slug_of "$top")" "$branch"
  put path "$top" "$(box_path "$top")"
  dirty="$(git -C "$top" status --porcelain | wc -l | tr -d ' ')"
  ahead="$(git -C "$top" rev-list --count '@{u}..HEAD' 2>/dev/null || echo '?')"
  if [ "$dirty" != 0 ] || [ "$ahead" != 0 ]; then
    echo "   COMMIT AND PUSH: $top ($dirty uncommitted, $ahead unpushed on $branch): the box gets only what is pushed"
  fi
done
echo "   ${#mirrors[@]} mirrored repos, $projects project repos, ${#copies[@]} copied skills"
if [ "${#copies[@]}" -gt 0 ]; then
  COPYFILE_DISABLE=1 tar -C "$HOME/.claude/skills" -chf "$data/copies.tar" --no-xattrs --no-mac-metadata "${copies[@]}"
fi
COPYFILE_DISABLE=1 tar -cf "$data/workspace.tar" -C "$W" --no-xattrs --no-mac-metadata \
  --exclude '*.bak' --exclude '*.bak-*' --exclude '.workspace/autopilot/autopilot.db*' \
  --exclude '.workspace/autopilot/*.sock*' --exclude '.workspace/autopilot/supervise.*' \
  --exclude '.workspace/autopilot/remote.yaml' .workspace
for f in "$HOME"/.claude/*-routes.json "$HOME"/.agents/*-routes.json; do
  [ -f "$f" ] || continue
  rel="${f#"$HOME"/}"
  mkdir -p "$data/home/$(dirname "$rel")" && cp "$f" "$data/home/$rel"
done

# --- the box session ---
# box_session_id <name>: the live session of that name in the box workspace, empty when none
box_session_id() {
  B "$bctl tree --json" | jq -r --arg ws "$ws" --arg n "$1" '[.result.tree.workspaces[]? | select(.name == $ws)
    | .sessions[]? | select(.name == $n and ((.ended // false) | not)) | .id] | first // empty'
}
# attach <box session id>: show it as a new session right after the calling one (or select the row already
# showing it); prints the local session id
attach() {
  local local_id
  local_id="$(L tree --json | jq -r --arg r "$1" '[.. | objects | select(.remoteSessionID? == $r) | .id] | first // empty')"
  if [ -z "$local_id" ]; then
    local_id="$(L zmx attach "$alias" "$1" --window "$AGTERM_WINDOW_ID" --json | jq -r '.result.id')"
    [ -n "$local_id" ] && [ "$local_id" != null ] || die "cannot attach the box session $1"
    L session move --after "$AGTERM_SESSION_ID" --target "$local_id" >/dev/null
  fi
  L session select --target "$local_id" >/dev/null
  printf '%s' "$local_id"
}

# a box session that stopped NOT READY is renamed; a new run replaces it
failed_name="autopilot remote: not ready"
for old in $(B "$bctl tree --json" | jq -r --arg ws "$ws" --arg n "$failed_name" \
    '.result.tree.workspaces[]? | select(.name == $ws) | .sessions[]? | select(.name == $n) | .id'); do
  for row in $(L tree --json | jq -r --arg r "$old" '.. | objects | select(.remoteSessionID? == $r) | .id'); do
    L session close --target "$row" >/dev/null 2>&1 || true
  done
  B "$bctl session close --target $old" >/dev/null 2>&1 || true
done
main_id="$(box_session_id "auto main")"
if [ -n "$main_id" ]; then mode=sync name="autopilot sync"; else mode=main name="auto main"; fi
token="$(openssl rand -hex 8)"  # pragma: allowlist secret
b64="$(base64 < "$here/box-session.sh" | tr -d '\n')"
echo "== box session \"$name\": the owner answers its prompts (disk passphrase, sign-ins) there"
# shellcheck disable=SC2016
rid="$(B "bash -s -- $(printf '%q ' "$b64" "$ws" "$name" "$mode" "$token" "$size")" <<'EOS' | jq -r '.result.id'
b64="$1" ws="$2" name="$3" mode="$4" token="$5" size="${6:-}"
cmd="bash -c \"\$(printf %s $b64 | base64 -d)\" box-session $mode $ws $token $size"
"$HOME/.local/bin/agtermctl" session new --workspace-name "$ws" --create-workspace --cwd "$HOME" --name "$name" \
  --no-select --json --command "zsh -lc $(printf %q "$cmd")"
EOS
)"
[ -n "$rid" ] && [ "$rid" != null ] || die "cannot start the box session"
local_id="$(attach "$rid")"
echo "   shown here as agterm://session/$local_id"

inc="\$HOME/.claude/.autopilot-incoming/$token"
# stage: the box session's status; `gone` when the session ended without one
stage() {
  B "cat \$HOME/.autopilot-remote-$token 2>/dev/null || { $bctl tree --json | jq -e --arg r '$rid' 'any(.. | objects; .id? == \$r)' >/dev/null || echo gone; }" \
    2>/dev/null | tail -n 1
}
sent=0 s=""
for _ in $(seq 1440); do
  s="$(stage || true)"
  case "$s" in
    connected)
      if [ "$sent" = 0 ]; then
        # agterm-server's agent hooks go into ~/.claude, which is on the open disk now
        "$ctl" server install "$alias" >/dev/null 2>&1 || echo "   note: agterm-server's agent hooks are still incomplete"
        tar -C "$data" -cf - . | B "tar -C $inc -xf - --warning=no-unknown-keyword --no-same-owner && touch $inc/ready" \
          || die "cannot send the data to the box"
        echo "   data sent"
        sent=1
      fi ;;
    done|failed|gone) break ;;
  esac
  sleep 5
done
case "$s" in
  done) ;;
  failed)
    B "$bctl session rename '$failed_name' --target $rid" >/dev/null 2>&1 || true
    L session rename "$failed_name" --target "$local_id" >/dev/null 2>&1 || true
    die "the box session \"$failed_name\" shows what is NOT READY: fix it, then run /autopilot remote again" ;;
  gone) die "the box session \"$name\" ended before it finished" ;;
  *) die "the box session \"$name\" did not finish within 2 hours" ;;
esac
B "rm -rf \$HOME/.claude/.autopilot-incoming \$HOME/.autopilot-remote-*" || true
if [ ! -f "$remote_file" ]; then
  (umask 077; printf 'ssh_host: "%s"\nbox_alias: "%s"\nbox_folder: "%s"\n' "$host" "$alias" "$dest" > "$remote_file")
fi
if [ "$mode" = sync ]; then
  L session close --target "$local_id" >/dev/null 2>&1 || true
  local_id="$(attach "$main_id")"
fi
echo "== auto main runs in the box"
echo "AUTO_MAIN=$local_id"
