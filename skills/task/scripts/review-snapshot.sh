#!/usr/bin/env bash
# Remember what each plannotator `review` showed, so the next one can show only what
# changed since. A snapshot is a commit of the repo's whole working state (HEAD + staged
# + unstaged + untracked; .gitignore, .scratch and .claude left out) built on a temp
# index: the working tree, the real index and the branch are never touched. `save`
# stages it as refs/task-review/pending; `promote code` moves it to refs/task-review/last.
#
# The plan gate keeps its own baseline: a copy of PLAN.md in <base>/.scratch/plan-review/
# (pending.md from `plan-save`, last.md after `promote plan`). Both plan commands run
# only in the PLAN stage: on IMPL `pair.py plan-open` refuses them, PLAN.md is closed.
#
# Usage (from the worktree, any depth):
#   bash review-snapshot.sh delta          # NO_PREVIOUS | NO_DELTA | DELTA <patch> + --stat + LAST_CHANGE=yes|no
#   bash review-snapshot.sh plan-delta     # NO_PREVIOUS | NO_DELTA | DELTA <patch> for PLAN.md
#   bash review-snapshot.sh guide [--plan] <json>   # check a Guided Review against the delta files
#   bash review-snapshot.sh save           # stage a snapshot of every repo (plannotator-session.sh runs it)
#   bash review-snapshot.sh plan-save      # stage a copy of PLAN.md (plannotator-session.sh runs it)
#   bash review-snapshot.sh promote code|plan      # pending -> last (guide-open.sh runs it)
#   bash review-snapshot.sh inject <url> <json>   # load a guide into a running plannotator
#
# A baseline moves only when plannotator opens the page: plannotator-session.sh stages it
# under the lock, and guide-open.sh promotes it after the opener returns 0. A failed start
# or a refused launch leaves last as it was.
#
# plan-delta: DELTA writes <base>/.scratch/plan-review/delta.patch, a `diff --git` header
# plus `diff -u` of last.md and PLAN.md, for `plannotator review --patch-file` (plannotator
# finds no file in a patch without that header line).
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
# --plan checks it against the plan-delta files (PLAN.md) only and saves nothing
# (GUIDE_OK): plannotator saves an injected guide by itself.
#
# inject: plannotator opens a guide by itself only for a finished `guide` job of its own
# session; a saved guide waits on its start page behind a click. So inject starts a
# guide job on the server at <url>, kills it at once (the agent does no work), and
# submits <json> as that job's output through plannotator's repair endpoint. The job
# turns `done`, the page opens the guide, and plannotator saves it with its patch, so
# this base's "Since last review" copy from `guide` is removed as a duplicate when it
# holds the same guide. A plan guide never matches it, so it never removes the code guide.
# plannotator-session.sh runs it from guide-open.sh before the page opens. Prints
# GUIDE_INJECTED <job> sections=<n> files=<n>, then a DROPPED (not in the review's diff)
# or UNPLACED (in the diff, in no section) line per file; or GUIDE_INJECT_FAILED: <why>.
# It uses plannotator's internal API (checked on 0.28.3).
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
pending_ref=refs/task-review/pending
patch="$base/.scratch/review-delta.patch"
files="$base/.scratch/review-delta.files"
plan="$base/PLAN.md"
plan_dir="$base/.scratch/plan-review"
plan_patch="$plan_dir/delta.patch"
plan_files="$plan_dir/delta.files"
self_dir="$(cd "$(dirname "$0")" && pwd)"

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
      git -C "$dir" update-ref "$pending_ref" "$c" || { echo "SNAPSHOT_FAILED: $dir"; exit 1; }
    done < <(repos)
    echo "SAVED"
    ;;
  plan-save)
    python3 "$self_dir/pair.py" plan-open --base "$base" || exit 1
    [ -f "$plan" ] || { echo "NO_PLAN: $plan"; exit 1; }
    mkdir -p "$plan_dir" && cp "$plan" "$plan_dir/pending.md" || { echo "SNAPSHOT_FAILED: $plan"; exit 1; }
    echo "SAVED"
    ;;
  promote)
    case "${2:-}" in
      code)
        moved=0
        while read -r prefix dir; do
          c="$(git -C "$dir" rev-parse -q --verify "$pending_ref^{commit}")" || continue
          git -C "$dir" update-ref "$ref" "$c" && git -C "$dir" update-ref -d "$pending_ref" ||
            { echo "PROMOTE_FAILED: $dir"; exit 1; }
          moved=1
        done < <(repos)
        [ $moved -eq 1 ] || { echo "PROMOTE_FAILED: no pending code snapshot"; exit 1; }
        ;;
      plan)
        [ -f "$plan_dir/pending.md" ] || { echo "PROMOTE_FAILED: no pending plan snapshot"; exit 1; }
        mv "$plan_dir/pending.md" "$plan_dir/last.md" || { echo "PROMOTE_FAILED: $plan_dir"; exit 1; }
        ;;
      *) echo "usage: $0 promote code|plan" >&2; exit 2 ;;
    esac
    echo "PROMOTED $2"
    ;;
  plan-delta)
    python3 "$self_dir/pair.py" plan-open --base "$base" || exit 1
    [ -f "$plan" ] || { echo "NO_PLAN: $plan"; exit 1; }
    mkdir -p "$plan_dir"; : > "$plan_patch"; : > "$plan_files"
    cp "$plan" "$plan_dir/current.md" || { echo "SNAPSHOT_FAILED: $plan"; exit 1; }
    if [ ! -f "$plan_dir/last.md" ]; then echo "NO_PREVIOUS"
    elif cmp -s "$plan_dir/last.md" "$plan_dir/current.md"; then echo "NO_DELTA"
    else
      { echo "diff --git a/PLAN.md b/PLAN.md"
        diff -u --label a/PLAN.md --label b/PLAN.md "$plan_dir/last.md" "$plan_dir/current.md"
      } > "$plan_patch"
      echo "PLAN.md" > "$plan_files"
      echo "DELTA $plan_patch"
    fi
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
    shift; mode=code
    [ "${1:-}" = "--plan" ] && { mode=plan; files="$plan_files"; shift; }
    [ -f "${1:-}" ] || { echo "usage: $0 guide [--plan] <guide.json>" >&2; exit 2; }
    [ -s "$files" ] || { echo "NO_DELTA_FILES: run $([ $mode = plan ] && echo plan-delta || echo delta) first"; exit 1; }
    dir="${PLANNOTATOR_DATA_DIR:-$HOME/.plannotator}/guides/$(repo_key)"
    [ $mode = plan ] || mkdir -p "$dir"
    python3 -I - "$1" "$files" "$dir" "$mode" <<'PY' || exit 1
import json, os, sys, time, glob
src, files, out, mode = sys.argv[1:5]
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
if mode == "plan":
    print(f"GUIDE_OK {src}")
    sys.exit(0)
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
  inject)
    [ -n "${2:-}" ] && [ -f "${3:-}" ] || { echo "usage: $0 inject <plannotator url> <guide.json>" >&2; exit 2; }
    dir="${PLANNOTATOR_DATA_DIR:-$HOME/.plannotator}/guides/$(repo_key)"
    python3 -I - "$2" "$3" "$dir" <<'PY' || exit 1
import glob, json, os, sys, urllib.error, urllib.request
from urllib.parse import urlsplit
url, src, saved_dir = sys.argv[1:4]
u = urlsplit(url)
root = f"{u.scheme}://{u.netloc}"

def call(method, path, body=None):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(root + path, data=data, method=method,
                                 headers={"Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(req, timeout=15) as r:
            return r.status, json.loads(r.read() or b"{}")
    except urllib.error.HTTPError as e:
        try:
            return e.code, json.loads(e.read() or b"{}")
        except ValueError:
            return e.code, {}

def fail(why):
    print(f"GUIDE_INJECT_FAILED: {why}")
    sys.exit(1)

def same(a, b):
    keys = (("title", ""), ("intent", ""), ("sections", []), ("unplacedFiles", []))
    return all(a.get(k, d) == b.get(k, d) for k, d in keys)

try:
    payload = open(src).read()
    sent = json.loads(payload)
    wanted = [d["file"] for s in sent["sections"] for d in s.get("diffs", [])]
    st, res = call("POST", "/api/agents/jobs", {"provider": "guide", "label": "Guided Review"})
    if st != 201:
        fail(f"start guide job: HTTP {st} {res.get('error', '')}")
    job = res["job"]["id"]
    st, res = call("DELETE", f"/api/agents/jobs/{job}")
    if st != 200:
        fail(f"kill guide job {job}: HTTP {st} {res.get('error', '')}")
    st, res = call("POST", f"/api/guide/{job}/submit", {"payload": payload})
    if st != 200:
        fail(f"submit guide: HTTP {st} {res.get('error', '')}")
    st, guide = call("GET", f"/api/guide/{job}")
    if st != 200:
        fail(f"read guide back: HTTP {st}")
except (OSError, ValueError, KeyError) as e:
    fail(str(e))
placed = {d["file"] for s in guide["sections"] for d in s.get("diffs", [])}
print(f"GUIDE_INJECTED {job} sections={res['sections']} files={res['files']}")
for f in sorted(set(wanted) - placed):
    print(f"DROPPED: {f}")
for f in guide.get("unplacedFiles", []):
    print(f"UNPLACED: {f}")
for old in glob.glob(os.path.join(saved_dir, "*-since-last-review.json")):
    try:
        if same(json.load(open(old)).get("guide", {}), sent):
            os.remove(old)
    except (OSError, ValueError):
        pass
PY
    ;;
  *) echo "usage: $0 save | plan-save | promote code|plan | delta | plan-delta | guide [--plan] <guide.json> | inject <url> <guide.json>" >&2; exit 2 ;;
esac
