#!/usr/bin/env bash
set -euo pipefail

# Usage: pull-main.sh
#
# Brings every project checkout in <workspace folder> up to date with its
# remote default branch (main): `git fetch --prune`, then a fast-forward-only pull.
# Same project list as new-task.sh.
#
# Runs only from the workspace folder, never from a task worktree: a worktree is on
# its task branch, and `task new` already cuts fresh worktrees from origin.
#
# A repo is never forced: not on its default branch, or a pull that is not a
# fast-forward (local commits, or local changes in the way) → reported as FAILED
# and left untouched.

# The workspace folder: SADENSMOL_DIR when set, else the first folder that holds
# .workspace/, walking up from the cwd, then from this script's own folder.
products_dir() {
  local d
  for d in "${PWD}" "$(cd "$(dirname "$0")" && pwd -P)"; do
    while [ "${d}" != "/" ]; do
      if [ -d "${d}/.workspace" ]; then echo "${d}"; return 0; fi
      d="$(dirname "${d}")"
    done
  done
  echo "ERROR: no .workspace/ found; set SADENSMOL_DIR" >&2
  return 1
}
SADENSMOL_DIR="${SADENSMOL_DIR:-$(products_dir)}"
REGISTRY_PY="$(cd "$(dirname "$0")" && pwd -P)/../../project/scripts/registry.py"
REPOS=()
while IFS= read -r r; do REPOS+=("${r}"); done < <(SADENSMOL_DIR="${SADENSMOL_DIR}" python3 -I "${REGISTRY_PY}" repos)
[ "${#REPOS[@]}" -gt 0 ] || { echo "ERROR: the workspace registry lists no repo"; exit 1; }

case "${PWD}/" in
  */.worktrees/*)
    echo "ERROR: task pull runs only in the workspace folder (${SADENSMOL_DIR}), not in a worktree (${PWD})."
    exit 1
    ;;
esac

get_default_branch() {
  local repo_dir="$1"
  if git -C "${repo_dir}" show-ref --verify --quiet refs/heads/master 2>/dev/null; then
    echo "master"
  elif git -C "${repo_dir}" show-ref --verify --quiet refs/heads/main 2>/dev/null; then
    echo "main"
  else
    git -C "${repo_dir}" symbolic-ref --short HEAD 2>/dev/null || echo "master"
  fi
}

updated=0
current=0
declare -a failed=()

for repo_name in "${REPOS[@]}"; do
  repo_dir="${SADENSMOL_DIR}/${repo_name}"

  if [ ! -d "${repo_dir}/.git" ] && [ ! -f "${repo_dir}/.git" ]; then
    echo "  FAILED  ${repo_name}: ${repo_dir} is not a git repository"
    failed+=("${repo_name}")
    continue
  fi

  default_branch="$(get_default_branch "${repo_dir}")"
  branch="$(git -C "${repo_dir}" symbolic-ref --short HEAD 2>/dev/null || echo "(detached)")"

  if ! git -C "${repo_dir}" fetch --prune origin >/dev/null 2>&1; then
    echo "  FAILED  ${repo_name}: git fetch failed"
    failed+=("${repo_name}")
    continue
  fi

  if [ "${branch}" != "${default_branch}" ]; then
    echo "  FAILED  ${repo_name}: on '${branch}', not '${default_branch}' — left untouched"
    failed+=("${repo_name}")
    continue
  fi

  before="$(git -C "${repo_dir}" rev-parse --short HEAD)"
  if ! out="$(git -C "${repo_dir}" merge --ff-only "origin/${default_branch}" 2>&1)"; then
    echo "  FAILED  ${repo_name}: not a fast-forward onto origin/${default_branch}: $(printf '%s\n' "${out}" | grep -m1 -E '^(error|fatal):' || printf '%s\n' "${out}" | head -n1)"
    failed+=("${repo_name}")
    continue
  fi
  after="$(git -C "${repo_dir}" rev-parse --short HEAD)"

  if [ "${before}" = "${after}" ]; then
    echo "  ok      ${repo_name} (${default_branch} @ ${after}, already current)"
    current=$((current + 1))
  else
    echo "  pulled  ${repo_name} (${default_branch} ${before} → ${after})"
    updated=$((updated + 1))
  fi
done

echo "Pull: ${updated} updated, ${current} already current, ${#failed[@]} failed"
if [ ${#failed[@]} -gt 0 ]; then
  printf '  - %s\n' "${failed[@]}"
  exit 2
fi
