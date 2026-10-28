#!/usr/bin/env bash
# Remember what each plannotator `review` showed, so the next one can show only what
# changed since. A snapshot is a commit of the repo's whole working state (HEAD + staged
# + unstaged + untracked; .gitignore, .scratch and .claude left out) built on a temp
# index: the working tree, the real index and the branch are never touched. It is kept
# as refs/task-review/last.
#
# Usage (from the worktree, any depth):
#   bash review-snapshot.sh delta          # NO_PREVIOUS | NO_DELTA | DELTA <patch> + --stat + LAST_CHANGE=yes|no
#   bash review-snapshot.sh guide <json>   # install the "since last review" Guided Review
#   bash review-snapshot.sh save           # snapshot every repo now; run it at every review launch
#
# Trees compare by content, so commits, pushes and amends between two reviews do not
# matter: the delta is what the operator has not seen yet. A rebase onto a newer base
# between reviews brings the base's changes into it too.
#
# LAST_CHANGE=yes: every changed repo went from the last review to now in exactly one
# new commit and nothing uncommitted, so plannotator's "Last change" view (each repo's
# last commit) shows the same delta. Repos with no delta still show their own last commit.
#
# guide: <json> is { title, intent, sections[{ title, overview, diffs[{ file, summary }] }],
# unplacedFiles } (plannotator's guide schema). Every delta file must be placed exactly
# once, and no other file may appear. It is saved where the review session lists saved
# guides (~/.plannotator/guides/<repoKey>/), replacing this base's previous one.
set -u

cwd="$(pwd)"
case "$cwd" in
  */.worktrees/*)
    root="${cwd%%/.worktrees/*}"
    branch="${cwd#*/.worktrees/}"; branch="${branch%%/*}"
    base="$root/.worktrees/$branch"
    ;;
  *) base="$cwd" ;;
esac
ref=refs/task-review/last
patch="$base/.scratch/review-delta.patch"
files="$base/.scratch/review-delta.files"

# repos: "<prefix> <dir>" per git repo — the base itself ("-" = no prefix) or each nested repo
repos() {
  if git -C "$base" rev-parse --show-toplevel 2>/dev/null | grep -qxF "$base"; then
    printf '%s\n' "- $base"
  fi
  for d in "$base"/*/; do
    [ -d "$d" ] || continue
    [ "$(git -C "$d" rev-parse --show-toplevel 2>/dev/null)" = "${d%/}" ] || continue
    printf '%s\n' "$(basename "${d%/}")/ ${d%/}"
  done
}

# snap <dir>: print a commit of the dir's current working state
snap() {
  local idx; idx="$(mktemp "${TMPDIR:-/tmp}/review-idx.XXXXXX")"
  rm -f "$idx"
  GIT_INDEX_FILE="$idx" git -C "$1" read-tree HEAD &&
    GIT_INDEX_FILE="$idx" git -C "$1" add -A -- . \
      ':(exclude).scratch' ':(exclude).claude' ':(glob,exclude)**/.DS_Store' &&
    tree="$(GIT_INDEX_FILE="$idx" git -C "$1" write-tree)"
  local rc=$?
  rm -f "$idx"
  [ $rc -eq 0 ] || return 1
  git -C "$1" commit-tree "$tree" -p HEAD -m "task review snapshot"
}

tree_of() { git -C "$1" rev-parse -q --verify "$2^{tree}" 2>/dev/null; }

# repo_key: the key plannotator files this base's saved guides under. A multi-repo
# base is not a git repo: <basename>-<sha256(path)[:8]>. A single repo: its origin as
# host__owner__repo, else the same fallback on its toplevel.
repo_key() {
  local remote host path
  if git -C "$base" rev-parse --show-toplevel 2>/dev/null | grep -qxF "$base" &&
     remote="$(git -C "$base" remote get-url origin 2>/dev/null)" && [ -n "$remote" ]; then
    remote="${remote#*://}"; remote="${remote#*@}"
    case "$remote" in
      *:*) host="${remote%%:*}"; path="${remote#*:}" ;;
      *)   host="${remote%%/*}"; path="${remote#*/}" ;;
    esac
    host="${host%%:*}"; path="${path%.git}"; path="${path%/}"
    printf '%s/%s' "$host" "$path" | tr '/' '\n' | grep -v '^$' |
      sed -E 's/[^a-zA-Z0-9._-]+/-/g; s/^[.-]+//; s/[.-]+$//; s/^$/x/' | paste -sd'\n' - |
      awk 'BEGIN{ORS=""} NR>1{print "__"} {print}'
    return
  fi
  printf '%s-%s' "$(basename "$base" | sed -E 's/[^a-zA-Z0-9._-]+/-/g; s/^[.-]+//; s/[.-]+$//')" \
    "$(printf '%s' "$base" | shasum -a 256 | cut -c1-8)"
}

case "${1:-}" in
  save)
    while read -r prefix dir; do
      c="$(snap "$dir")" || { echo "SNAPSHOT_FAILED: $dir"; exit 1; }
      git -C "$dir" update-ref "$ref" "$c"
    done < <(repos)
    echo "SAVED"
    ;;
  delta)
    mkdir -p "$base/.scratch"; : > "$patch"; : > "$files"; seen=0; last_change=yes
    while read -r prefix dir; do
      [ "$prefix" = "-" ] && prefix=""
      last="$(git -C "$dir" rev-parse -q --verify "$ref^{commit}")" || continue   # never reviewed
      seen=1
      now="$(snap "$dir")" || { echo "SNAPSHOT_FAILED: $dir"; exit 1; }
      [ "$(tree_of "$dir" "$last")" = "$(tree_of "$dir" "$now")" ] && continue
      git -C "$dir" diff --no-color --src-prefix="a/$prefix" --dst-prefix="b/$prefix" \
        "$last" "$now" >> "$patch"
      git -C "$dir" diff --name-only "$last" "$now" | sed "s|^|$prefix|" >> "$files"
      git -C "$dir" diff --stat=120 "$last" "$now" | sed '$d' | sed "s|^ | $prefix|" >&3
      if [ "$(tree_of "$dir" HEAD~1)" != "$(tree_of "$dir" "$last")" ] ||
         [ "$(tree_of "$dir" HEAD)" != "$(tree_of "$dir" "$now")" ]; then
        last_change=no
      fi
    done < <(repos) 3> "$patch.stat"
    if [ $seen -eq 0 ]; then echo "NO_PREVIOUS"
    elif [ ! -s "$patch" ]; then echo "NO_DELTA"
    else echo "DELTA $patch"; cat "$patch.stat"; echo "LAST_CHANGE=$last_change"
    fi
    rm -f "$patch.stat"
    ;;
  guide)
    [ -f "${2:-}" ] || { echo "usage: $0 guide <guide.json>" >&2; exit 2; }
    [ -s "$files" ] || { echo "NO_DELTA_FILES: run delta first"; exit 1; }
    dir="${PLANNOTATOR_DATA_DIR:-$HOME/.plannotator}/guides/$(repo_key)"
    mkdir -p "$dir"
    python3 -I - "$2" "$files" "$dir" <<'PY' || exit 1
import json, os, sys, time, glob
src, files, out = sys.argv[1:4]
g = json.load(open(src))
delta = [l.strip() for l in open(files) if l.strip()]
placed = [d["file"] for s in g["sections"] for d in s.get("diffs", [])] + g.get("unplacedFiles", [])
missing = sorted(set(delta) - set(placed))
extra = sorted(set(placed) - set(delta))
dups = sorted({f for f in placed if placed.count(f) > 1})
if missing or extra or dups:
    for name, xs in (("MISSING", missing), ("NOT_IN_DELTA", extra), ("DUPLICATED", dups)):
        for f in xs:
            print(f"{name}: {f}")
    sys.exit(1)
guide = {"title": g["title"], "intent": g.get("intent", ""), "sections": g["sections"],
         "unplacedFiles": g.get("unplacedFiles", [])}
for old in glob.glob(os.path.join(out, "*-since-last-review.json")):
    os.remove(old)
now = int(time.time() * 1000)
path = os.path.join(out, f"{now}-since-last-review.json")
json.dump({"version": 1, "savedAt": now, "label": "Since last review", "title": g["title"],
           "guide": guide, "reviewed": [False] * len(g["sections"])}, open(path, "w"), indent=2)
print(f"GUIDE_SAVED {path}")
PY
    ;;
  *) echo "usage: $0 save | delta | guide <guide.json>" >&2; exit 2 ;;
esac
