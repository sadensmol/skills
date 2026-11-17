#!/bin/bash
# box-session.sh MODE WORKSPACE TOKEN [SIZE] — the box side of `/autopilot remote`. remote.sh sends this file
# as the command of a new agterm-server session in the box, so it is never stored there, and the owner
# watches and answers it in that session.
#   1. Open the encrypted disk (the owner types the passphrase; SIZE for a new disk) and link the logins
#      and the workspaces into it; sign in to Claude and GitHub when signed out.
#   2. Wait for the Mac's data in ~/.claude/.autopilot-incoming/TOKEN/ (manifest, .workspace/, skills).
#   3. Move the workspace in when the box lacks it, mirror the repos, install the Mac's skills and plugins,
#      rewrite Mac paths, check the settings and the secret keys (names only), trust the folder.
#   4. MODE main: run `claude /autopilot` in the workspace folder, in this session. MODE sync: end.
# ~/.autopilot-remote-TOKEN (outside the disk, so it works before the disk opens) tells the Mac the stage:
# connected, done or failed. On a failure the session keeps a shell open, so the owner reads what is NOT READY.
set -uo pipefail

mode="${1:?mode}" ws="${2:?workspace}" token="${3:?token}" size="${4:-}"  # pragma: allowlist secret
dest="$HOME/workspaces/$ws"
status="$HOME/.autopilot-remote-$token"
problems=0
bad() { echo "   NOT READY: $*"; problems=$((problems + 1)); }
hold() {
  echo failed > "$status"
  printf '\n== /autopilot remote stopped here: fix what is above, then run /autopilot remote again\n'
  exec zsh -l
}
die() { echo "box: $*"; hold; }

echo "== disk"
if [ "$(sudo -n /usr/local/sbin/autopilot-disk status 2>/dev/null)" != open ]; then
  sudo -n /usr/local/sbin/autopilot-disk open "$size" || die "the disk did not open"
fi
# a plain folder at a link's place (an installer that ran while the disk was closed) moves onto the disk
# first; what the disk already has wins
link_into_disk() {
  if [ -d "$1" ] && [ ! -L "$1" ]; then cp -a --update=none "$1"/. "$2"/; rm -rf "$1"; fi
  ln -sfn "$2" "$1"
}
mkdir -p "$HOME/.config" /secure/home/.claude /secure/home/.config/gh /secure/home/workspaces  # pragma: allowlist secret
link_into_disk "$HOME/.claude" /secure/home/.claude
link_into_disk "$HOME/.config/gh" /secure/home/.config/gh  # pragma: allowlist secret
link_into_disk "$HOME/workspaces" /secure/home/workspaces
echo "   open"

echo "== sign-ins"
if ! claude auth status --json 2>/dev/null | jq -e '.loggedIn == true' >/dev/null; then
  claude auth login || die "the Claude sign-in did not finish"
fi
if ! gh auth status >/dev/null 2>&1; then
  echo "   GitHub: open https://github.com/login/device on your Mac and type the code below"
  gh auth login --hostname github.com --git-protocol https --web </dev/null || die "the GitHub sign-in did not finish"
fi
gh auth setup-git >/dev/null
echo "   Claude and GitHub signed in"

inc="$HOME/.claude/.autopilot-incoming/$token"
mkdir -p "$inc" && chmod 700 "$HOME/.claude/.autopilot-incoming" "$inc"
echo connected > "$status"

echo "== waiting for the Mac's data"
for _ in $(seq 600); do [ -f "$inc/ready" ] && break; sleep 1; done
[ -f "$inc/ready" ] || die "no data from the Mac within 10 minutes"
manifest="$(cat "$inc/manifest")"
field() { awk -F'\t' -v k="$1" '$1 == k { print $2; exit }' <<<"$manifest"; }

echo "== workspace $ws"
if [ ! -f "$dest/.workspace/autopilot/config.yaml" ] || [ "$(field force)" = 1 ]; then
  mkdir -p "$dest" && chmod 700 "$dest"
  if [ -d "$dest/.workspace" ]; then
    mv "$dest/.workspace" "$dest/.workspace.bak-$(date +%Y%m%d-%H%M%S)"
    echo "   kept the old .workspace/ as a .workspace.bak-* folder"
  fi
  tar -C "$dest" -xf "$inc/workspace.tar" --warning=no-unknown-keyword --no-same-owner \
    && chmod -R go-rwx "$dest/.workspace" && echo "   moved in from the Mac" || die "cannot unpack .workspace/"
else
  echo "   already in the box"
fi
rm -f "$inc/workspace.tar"

echo "== repos"
while IFS=$'\t' read -r kind a b c; do
  case "$kind" in
    mirror|project)
      if [ ! -d "$a/.git" ]; then
        mkdir -p "$(dirname "$a")"
        gh repo clone "$b" "$a" -- -q >/dev/null 2>&1 || { bad "cannot clone $b"; continue; }
        echo "   $b: cloned"
      fi
      git -C "$a" remote set-url origin "https://github.com/$b.git"
      if [ "$kind" = mirror ]; then
        if git -C "$a" fetch -q origin "$c" && git -C "$a" checkout -q -f -B "$c" FETCH_HEAD; then
          git -C "$a" branch -q --set-upstream-to "origin/$c" >/dev/null 2>&1 || true
        else
          bad "cannot update $b to origin/$c"
        fi
      fi ;;
  esac
done <<<"$manifest"

echo "== skills"
skills="$HOME/.claude/skills"
mkdir -p "$skills" "$inc/copies"
[ ! -f "$inc/copies.tar" ] || tar -C "$inc/copies" -xf "$inc/copies.tar" --warning=no-unknown-keyword --no-same-owner
want=" "
while IFS=$'\t' read -r kind a b c; do
  case "$kind" in
    skill)
      want+="$a "
      [ -f "$b/SKILL.md" ] || { bad "skill $a: $b/SKILL.md is not in the box (is it pushed?)"; continue; }
      if [ "$(readlink "$skills/$a" 2>/dev/null)" != "$b" ]; then rm -rf "${skills:?}/$a"; ln -s "$b" "$skills/$a"; fi ;;
    copy)
      want+="$a "
      rm -rf "${skills:?}/$a"; mv "$inc/copies/$a" "$skills/$a" ;;
    link)
      [ -e "$b" ] || { bad "~/.claude/$a: $b is not in the box (is it pushed?)"; continue; }
      if [ "$(readlink "$HOME/.claude/$a" 2>/dev/null)" != "$b" ]; then rm -rf "${HOME:?}/.claude/$a"; ln -s "$b" "$HOME/.claude/$a"; fi ;;
  esac
done <<<"$manifest"
for e in "$skills"/*; do
  n="$(basename "$e")"
  [ -f "$e/SKILL.md" ] || continue
  case "$want" in *" $n "*) ;; *) rm -rf "$e"; echo "   removed $n (not on the Mac)" ;; esac
done
# file by file: ~/.claude is a link onto the disk, which a recursive copy would not enter
if [ -d "$inc/home" ]; then
  (cd "$inc/home" && find . -type f) | while read -r f; do
    mkdir -p "$(dirname "$HOME/$f")" && cp "$inc/home/$f" "$HOME/$f"
  done
fi
echo "   $(wc -w <<<"$want" | tr -d ' ') skills, the same as the Mac"

echo "== plugins"
markets="$(claude plugin marketplace list --json 2>/dev/null || echo '[]')"
while IFS=$'\t' read -r kind a b c; do
  [ "$kind" = market ] || continue
  have="$(jq -r --arg n "$a" '.[] | select(.name == $n) | (.repo // .path // "")' <<<"$markets")"
  if [ -n "$have" ] && [ "$have" != "$c" ]; then claude plugin marketplace remove "$a" >/dev/null 2>&1; have=""; fi
  if [ -z "$have" ]; then
    claude plugin marketplace add "$c" >/dev/null 2>&1 && echo "   marketplace $a: added" || bad "cannot add the marketplace $a ($c)"
  elif [ "$b" = directory ]; then
    claude plugin marketplace update "$a" >/dev/null 2>&1 || bad "cannot update the marketplace $a"
  fi
done <<<"$manifest"
installed="$(claude plugin list --json 2>/dev/null || echo '[]')"
while IFS=$'\t' read -r kind a b c; do
  [ "$kind" = plugin ] || continue
  if jq -e --arg id "$a" 'any(.[]; .id == $id)' <<<"$installed" >/dev/null; then
    claude plugin update "$a" >/dev/null 2>&1 || true
  else
    claude plugin install "$a" >/dev/null 2>&1 && echo "   $a: installed" || bad "cannot install the plugin $a"
  fi
done <<<"$manifest"
for id in $(jq -r '.[].id' <<<"$installed"); do
  awk -F'\t' -v id="$id" '$1 == "plugin" && $2 == id { f = 1 } END { exit !f }' <<<"$manifest" \
    || echo "   note: the plugin $id is in the box but not on the Mac"
done

echo "== settings"
mac_home="$(field mac_home)" mac_ws="$(field mac_workspace)"
files="$(cd "$dest/.workspace" && grep -rlIF --exclude='*.bak*' "$mac_home/" . || true)"
for f in $files; do
  # the longest Mac path first, then the workspace folder, then the Mac home (tools such as ~/.local/bin)
  while IFS=$'\t' read -r _ from to; do
    sed -i "s|$from|$to|g" "$dest/.workspace/$f"
  done < <(awk -F'\t' '$1 == "path" { print length($2) "\t" $2 "\t" $3 }' <<<"$manifest" | sort -rn)
  sed -i -e "s|$mac_ws/|$dest/|g" -e "s|$mac_home/|$HOME/|g" "$dest/.workspace/$f"
  echo "   rewrote Mac paths in .workspace/${f#./}"
done
left="$(cd "$dest/.workspace" && grep -rlIF --exclude='*.bak*' "$mac_home/" . || true)"
[ -z "$left" ] || bad "Mac paths remain in .workspace/: $(tr '\n' ' ' <<<"$left")"
[ "$(jq -r '.agterm_workspace // empty' "$dest/.workspace/settings.json" 2>/dev/null)" = "$ws" ] \
  || bad "agterm_workspace of .workspace/settings.json is not $ws"
ls "$dest"/.workspace/projects/*.json >/dev/null 2>&1 || bad ".workspace/projects/ holds no .json file"
cfg="$dest/.workspace/autopilot/config.yaml"
[ -f "$cfg" ] || bad "there is no .workspace/autopilot/config.yaml"
for key in SLACK_APP_TOKEN SLACK_BOT_TOKEN LINEAR_API_KEY; do
  grep -qE "^$key=.+" "$dest/.workspace/autopilot/secrets.env" 2>/dev/null || bad "secrets.env has no value for $key"
done
src="$(sed -n 's/^ *source_dir: *"\{0,1\}\([^"]*\)"\{0,1\} *$/\1/p' "$cfg" 2>/dev/null | head -n 1)"
[ -d "$src" ] || bad "service.source_dir of config.yaml is not a folder in the box: $src"

echo "== folder trust"
dir="$(readlink -f "$dest")"
conf="${CLAUDE_CONFIG_DIR:-$HOME}/.claude.json"
[ -f "$conf" ] || echo '{}' > "$conf"
if [ "$(jq -r --arg d "$dir" '.projects[$d].hasTrustDialogAccepted // false' "$conf")" != true ]; then
  tmp="$(mktemp "$conf.XXXXXX")"
  jq --arg d "$dir" '.projects[$d].hasTrustDialogAccepted = true' "$conf" > "$tmp" && chmod 600 "$tmp" && mv "$tmp" "$conf" \
    || bad "cannot record the folder trust of $dir"
fi
echo "   $dir is trusted"

rm -rf "$inc/copies" "$inc/copies.tar" "$inc/home"
[ "$problems" = 0 ] || hold
echo done > "$status"
echo "== the workspace is ready"
if [ "$mode" = main ]; then
  cd "$dest" && exec zsh -lc 'claude /autopilot'
fi
