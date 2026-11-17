#!/usr/bin/env bash
set -euo pipefail

# Remove a sadensmol task's worktree, its local branch, and its agterm session.
# Mirrors new-task.sh: the project repo is the repo folder of the worktree (or the
# repo a branch key names). Repos come from the workspace registry.

BRANCH_NAME="${1:?Usage: cleanup-task.sh <branch-name>}"
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
# Every repo of every project, from the workspace registry (.workspace/settings.json).
ALL_REPOS="$(SADENSMOL_DIR="${SADENSMOL_DIR}" python3 -I "${REGISTRY_PY}" repos | tr '\n' ' ')" || exit 1
WORKTREE_BASE="${SADENSMOL_DIR}/.worktrees/${BRANCH_NAME}"
PRODUCT=""
for r in ${ALL_REPOS}; do
  if [ -e "${WORKTREE_BASE}/${r}/.git" ]; then PRODUCT="${r}"; break; fi
done
if [ -z "${PRODUCT}" ]; then
  PRODUCT="$(printf '%s' "${BRANCH_NAME##*/}" | grep -oiE '^[a-z0-9]+-[0-9]+' | sed -E 's/-[0-9]+$//' | tr '[:upper:]' '[:lower:]' || true)"
fi
case " ${ALL_REPOS} " in
  *" ${PRODUCT} "*) ;;
  *) echo "ERROR: no repo folder in ${WORKTREE_BASE} and the branch key names no repo; repos: ${ALL_REPOS}"; exit 1 ;;
esac
REPOS=("${PRODUCT}")

if [ ! -d "${WORKTREE_BASE}" ]; then
  echo "ERROR: Worktree directory does not exist: ${WORKTREE_BASE}"
  exit 1
fi

FORCE_CLEANUP="${FORCE_CLEANUP:-0}"
case "${2:-}" in --force|-f) FORCE_CLEANUP=1 ;; esac
CHECKLIST_FILE="${WORKTREE_BASE}/.task-cleanup-checklist"

checklist_value() {
  local key="$1"
  local count
  count="$(grep -Ec "^${key}=" "${CHECKLIST_FILE}" || true)"
  if [ "${count}" -ne 1 ]; then
    echo "ERROR: checklist must contain exactly one '${key}=...' line: ${CHECKLIST_FILE}"
    return 1
  fi
  grep -E "^${key}=" "${CHECKLIST_FILE}" | cut -d= -f2-
}

validate_checklist() {
  if [ ! -f "${CHECKLIST_FILE}" ]; then
    echo "ERROR: missing cleanup checklist: ${CHECKLIST_FILE}"
    return 1
  fi

  [ "$(checklist_value planned_work)" = "complete" ] || { echo "ERROR: planned work is not marked complete"; return 1; }
  related_work="$(checklist_value related_work)"
  case "${related_work}" in none|tracked) ;; *) echo "ERROR: related_work must be 'none' or 'tracked'"; return 1 ;; esac
  related_prs="$(checklist_value related_prs)"
  case "${related_prs}" in none|verified) ;; *) echo "ERROR: related_prs must be 'none' or 'verified'"; return 1 ;; esac
  [ "$(checklist_value commits)" = "complete" ] || { echo "ERROR: commits are not marked complete"; return 1; }
  [ "$(checklist_value tests)" = "green" ] || { echo "ERROR: tests are not marked green"; return 1; }
  problems="$(checklist_value problems)"
  case "${problems}" in none|tracked) ;; *) echo "ERROR: problems must be 'none' or 'tracked'"; return 1 ;; esac
  docs="$(checklist_value docs)"
  case "${docs}" in updated|none) ;; *) echo "ERROR: docs must be 'updated' or 'none' (finish §6a runs before the merge)"; return 1 ;; esac
  [ "$(checklist_value ticket_updates)" = "confirmed" ] || { echo "ERROR: tracker updates are not confirmed"; return 1; }
  [ "$(checklist_value tracker_state)" = "verified" ] || { echo "ERROR: tracker state/assignee not re-verified after the merge"; return 1; }
  [ "$(checklist_value infra)" = "clean" ] || { echo "ERROR: infra is not marked clean"; return 1; }
}

verify_prs_and_worktrees() {
  command -v gh >/dev/null 2>&1 || { echo "ERROR: 'gh' is required to verify merged PRs and CI checks"; return 1; }
  command -v jq >/dev/null 2>&1 || { echo "ERROR: 'jq' is required to verify PR review and CI state"; return 1; }

  local failures=() repo_name repo_dir worktree_repo status pr_json rows url state merged_at review_decision bad_checks check_count ahead base_ref
  for repo_name in "${REPOS[@]}"; do
    repo_dir="${SADENSMOL_DIR}/${repo_name}"
    worktree_repo="${WORKTREE_BASE}/${repo_name}"
    [ -d "${worktree_repo}" ] || continue
    [ -d "${repo_dir}/.git" ] || [ -f "${repo_dir}/.git" ] || continue

    status="$(git -C "${worktree_repo}" status --porcelain 2>/dev/null || true)"
    [ -z "${status}" ] || failures+=("${repo_name}: worktree has uncommitted files")

    if ! pr_json="$(cd "${repo_dir}" && gh pr list --head "${BRANCH_NAME}" --state all --limit 100 \
      --json url,state,mergedAt,reviewDecision,statusCheckRollup 2>/dev/null)"; then
      failures+=("${repo_name}: could not query GitHub PRs")
      continue
    fi

    rows="$(printf '%s' "${pr_json}" | jq -r '.[] | [.url, .state, (.mergedAt // ""), (.reviewDecision // ""), ((.statusCheckRollup // []) | map(select((.conclusion // "") != "SUCCESS" and (.conclusion // "") != "SKIPPED")) | length), ((.statusCheckRollup // []) | length)] | @tsv')"
    while IFS=$'\t' read -r url state merged_at review_decision bad_checks check_count; do
      [ -n "${url}" ] || continue
      [ "${state}" = "MERGED" ] || failures+=("${repo_name}: PR ${url} is ${state}, not MERGED")
      [ -n "${merged_at}" ] || failures+=("${repo_name}: PR ${url} has no merge timestamp")
      [ "${review_decision}" = "APPROVED" ] || failures+=("${repo_name}: PR ${url} was not APPROVED")
      [ "${check_count}" -gt 0 ] || failures+=("${repo_name}: PR ${url} has no recorded CI checks")
      [ "${bad_checks}" -eq 0 ] || failures+=("${repo_name}: PR ${url} has failing or running CI checks")
    done <<< "${rows}"

    if [ -z "${rows}" ]; then
      base_ref="$(git -C "${worktree_repo}" symbolic-ref -q --short refs/remotes/origin/HEAD 2>/dev/null || printf '%s' 'origin/main')"
      ahead="$(git -C "${worktree_repo}" rev-list --count "${base_ref}..HEAD" 2>/dev/null || printf '%s' '0')"
      [ "${ahead}" -eq 0 ] || failures+=("${repo_name}: ${ahead} local commits have no related PR")
    fi
  done

  if [ "${#failures[@]}" -gt 0 ]; then
    echo "REFUSING cleanup for '${BRANCH_NAME}':"
    printf '  - %s\n' "${failures[@]}"
    return 1
  fi
  echo "Completion, commit, PR approval, merge, and CI checks passed."
}

if [ "${FORCE_CLEANUP}" != "1" ]; then
  validate_checklist
  verify_prs_and_worktrees
fi

# --- Resolve the task's own agterm session (closed last, at the very end) ---
#
# Computed HERE so the console sweep below can EXCLUDE it: the task session's
# cwd is the worktree base too, so a cwd match alone would close it early and
# tear down the pty this script is running in.

AGTERM_NAME=""
AGTERM_TASK_SESSION_ID=""
if [ -n "${AGTERM_ENABLED:-}" ] && command -v agtermctl >/dev/null 2>&1; then
  AGTERM_BRANCH="${BRANCH_NAME##*/}"
  TASK_ID="$(printf '%s' "${AGTERM_BRANCH}" | grep -oiE '^[a-z0-9]+-[0-9]+' | tr '[:lower:]' '[:upper:]' || true)"
  AGTERM_DESC="$(printf '%s' "${AGTERM_BRANCH}" | sed -E 's/^[a-zA-Z0-9]+-[0-9]+-//')"
  if [ -n "${TASK_ID}" ] && [ -n "${AGTERM_DESC}" ] && [ "${AGTERM_DESC}" != "${AGTERM_BRANCH}" ]; then
    AGTERM_NAME="${TASK_ID} ${AGTERM_DESC}"
  else
    AGTERM_NAME="${TASK_ID:-${AGTERM_BRANCH}}"
  fi

  if command -v jq >/dev/null 2>&1; then
    AGTERM_TASK_SESSION_ID="$(agtermctl tree --json 2>/dev/null \
      | jq -r --arg n "${AGTERM_NAME}" '.result.tree.workspaces[]?.sessions[]? | select(.name==$n) | .id' 2>/dev/null \
      | head -n1)"
  fi
fi

# --- Close the task's OTHER console sessions (test runs, infra stacks) ---
#
# `task` opens extra agterm console sessions for work that must not block the
# task session — a `task dev` stack, a long `task test:integration` run, a repo
# shell. They are named after their directory (`<repo>`),
# NOT after the task, so the name-matched close at the
# end of this script never saw them: they outlived every cleanup, kept holding
# the stack's host ports and piled up in the workspace.
#
# Find them by cwd instead — every such console was opened with its cwd inside
# this worktree — then stop whatever stack each one left running and close it.
#
# ORDER MATTERS: this runs BEFORE the worktrees are deleted, because
# `task dev:stop:clean` needs the repo's compose file that the rm -rf removes.

if [ -n "${AGTERM_ENABLED:-}" ] && command -v agtermctl >/dev/null 2>&1; then
  if ! command -v jq >/dev/null 2>&1; then
    if [ "${FORCE_CLEANUP}" = "1" ]; then
      echo "WARN: jq not found — cannot verify task console sessions because --force was explicit."
    else
      echo "ERROR: jq is required to verify and close task console sessions."
      exit 1
    fi
  else
    console_rows="$(agtermctl tree --json 2>/dev/null \
      | jq -r --arg base "${WORKTREE_BASE}" '
          .result.tree.workspaces[]?.sessions[]?
          | select((.cwd // "") == $base or ((.cwd // "") | startswith($base + "/")))
          | [.id, (.name // ""), (.cwd // "")] | @tsv' 2>/dev/null || true)"

    while IFS=$'\t' read -r sid sname scwd; do
      [ -n "${sid}" ] || continue
      if [ "${sid}" = "${AGTERM_TASK_SESSION_ID}" ]; then continue; fi   # the task's own session
      if [ "${sid}" = "${AGTERM_SESSION_ID:-}" ]; then continue; fi      # the session we run in

      # Closing a console kills its pty but NOT the containers it started, so a
      # stack would survive as orphaned. Stop it first when the console sits in a
      # repo that has the stop target. The marker is split so the shell prints it
      # only as OUTPUT — the echoed command line reads `echo AGTERM_"DONE"_MARKER`
      # and never false-matches the poll below.
      if [ -f "${scwd}/Taskfile.yml" ] && grep -qE '^  dev:stop:clean:' "${scwd}/Taskfile.yml" 2>/dev/null; then
        echo "Stopping stack in console '${sname}' (${scwd})..."
        agtermctl session type --target "${sid}" "$(printf '\003')" 2>/dev/null || true
        sleep 1
        printf 'task dev:stop:clean; echo AGTERM_"DONE"_MARKER\n' \
          | agtermctl session type --target "${sid}" --stdin 2>/dev/null || true
        for _ in $(seq 1 60); do
          if agtermctl session text --target "${sid}" --all --lines 40 2>/dev/null \
               | grep -q '^AGTERM_DONE_MARKER'; then
            break
          fi
          sleep 2
        done
      fi

      agtermctl session close --target "${sid}" 2>/dev/null || true
      echo "Closed console session: ${sname:-${sid}} (${scwd})"
    done <<< "${console_rows}"

    remaining_console_rows="$(agtermctl tree --json 2>/dev/null \
      | jq -r --arg base "${WORKTREE_BASE}" --arg task "${AGTERM_TASK_SESSION_ID}" --arg current "${AGTERM_SESSION_ID:-}" '
          .result.tree.workspaces[]?.sessions[]?
          | select((.cwd // "") == $base or ((.cwd // "") | startswith($base + "/")))
          | select(.id != $task and .id != $current)
          | [.id, (.name // ""), (.cwd // "")] | @tsv' 2>/dev/null || true)"
    if [ -n "${remaining_console_rows}" ]; then
      if [ "${FORCE_CLEANUP}" = "1" ]; then
        echo "WARN: task console sessions remain because --force was explicit:"
        printf '  %s\n' "${remaining_console_rows}"
      else
        echo "ERROR: task console sessions remain after cleanup:"
        printf '  %s\n' "${remaining_console_rows}"
        exit 1
      fi
    fi
  fi
fi

# --- Stop and remove Docker resources owned by this worktree ---

cleanup_docker_resources() {
  if ! command -v docker >/dev/null 2>&1; then
    echo "ERROR: 'docker' is required to verify task-owned resources"
    return 1
  fi
  if ! docker info >/dev/null 2>&1; then
    echo "ERROR: Docker is unavailable; task-owned resources cannot be verified"
    return 1
  fi

  local resource_dir ids networks volumes remaining
  local resource_dirs=("${WORKTREE_BASE}")
  for resource_dir in "${WORKTREE_BASE}"/*/; do
    [ -d "${resource_dir}" ] || continue
    resource_dirs+=("${resource_dir%/}")
  done

  for resource_dir in "${resource_dirs[@]}"; do
    ids="$(docker ps -q --filter "label=com.docker.compose.project.working_dir=${resource_dir}" 2>/dev/null || true)"
    if [ -n "${ids}" ]; then
      echo "Stopping Docker containers for ${resource_dir}:"
      docker ps --filter "label=com.docker.compose.project.working_dir=${resource_dir}" --format '  {{.Names}}' || true
      docker rm -f ${ids} >/dev/null || true
    fi

    networks="$(docker network ls -q --filter "label=com.docker.compose.project.working_dir=${resource_dir}" 2>/dev/null || true)"
    [ -z "${networks}" ] || docker network rm ${networks} >/dev/null || true

    volumes="$(docker volume ls -q --filter "label=com.docker.compose.project.working_dir=${resource_dir}" 2>/dev/null || true)"
    [ -z "${volumes}" ] || docker volume rm ${volumes} >/dev/null || true
  done

  for resource_dir in "${resource_dirs[@]}"; do
    remaining="$(docker ps -q --filter "label=com.docker.compose.project.working_dir=${resource_dir}" 2>/dev/null || true)"
    [ -z "${remaining}" ] || { echo "ERROR: Docker containers still run for ${resource_dir}: ${remaining}"; return 1; }
  done
  echo "Docker resource check passed."
}

if ! cleanup_docker_resources; then
  if [ "${FORCE_CLEANUP}" = "1" ]; then
    echo "WARN: Docker cleanup verification failed; continuing because --force was explicit."
  else
    exit 1
  fi
fi

# --- Release the worktree infra lock ---
# The lock (<worktree-base>/.infra.lock) serializes stack bring-ups across this
# task's branches and consoles. The rm -rf below would take it with the worktree,
# but release it here, right after the containers are confirmed gone, so an early
# exit further down cannot leave a lock standing over a stack that no longer runs.

release_infra_lock() {
  local lock="${WORKTREE_BASE}/.infra.lock"
  [ -d "${lock}" ] || return 0
  if [ -f "${lock}/owner" ]; then
    echo "Releasing infra lock held by:"
    sed 's/^/  /' "${lock}/owner"
  else
    echo "Releasing infra lock (no owner file)."
  fi
  rm -rf "${lock}"
}

release_infra_lock

# --- Remove each repo's git worktree + local branch ---

for REPO_NAME in "${REPOS[@]}"; do
  REPO_DIR="${SADENSMOL_DIR}/${REPO_NAME}"
  WORKTREE_PATH="${WORKTREE_BASE}/${REPO_NAME}"

  if [ ! -d "${REPO_DIR}/.git" ] && [ ! -f "${REPO_DIR}/.git" ]; then
    continue   # repo not cloned on this machine — nothing to clean
  fi

  if [ -d "${WORKTREE_PATH}" ]; then
    echo "Removing worktree: ${WORKTREE_PATH}"
    git -C "${REPO_DIR}" worktree remove --force "${WORKTREE_PATH}" 2>/dev/null || true
  fi

  if git -C "${REPO_DIR}" rev-parse --verify "${BRANCH_NAME}" &>/dev/null; then
    git -C "${REPO_DIR}" branch -D "${BRANCH_NAME}" 2>/dev/null || true
  fi

  git -C "${REPO_DIR}" worktree prune 2>/dev/null || true
done

# --- Remove the worktree base directory ---

if [ -d "${WORKTREE_BASE}" ]; then
  rm -rf "${WORKTREE_BASE}"
  echo "Removed directory: ${WORKTREE_BASE}"
fi

# --- Close the task's agterm session (when running inside agterm) ---
#
# agterm sessions are named "<task id> <descriptor>" (ABC-21 style) when the
# branch carries a <KEY>-<n> id — mirror new-task.sh EXACTLY or the session won't be
# found. The lookup itself happened earlier (AGTERM_TASK_SESSION_ID), so the
# console sweep could skip it. Closing the session you're in doesn't end agterm — the app
# selects a neighboring session itself. When we ARE that session, close it delayed
# + backgrounded so this script can finish reporting before its pty is torn down.

if [ -n "${AGTERM_ENABLED:-}" ] && command -v agtermctl >/dev/null 2>&1; then
  agterm_id="${AGTERM_TASK_SESSION_ID}"
  if [ -n "${agterm_id}" ]; then
    if [ "${AGTERM_SESSION_ID:-}" = "${agterm_id}" ]; then
      nohup bash -c "sleep 2; agtermctl session close --target '${agterm_id}'" >/dev/null 2>&1 &
      echo "Closing agterm session: ${AGTERM_NAME} (agterm will select another session)"
    else
      agtermctl session close --target "${agterm_id}" 2>/dev/null || true
      echo "Closed agterm session: ${AGTERM_NAME}"
    fi
  fi
fi

echo ""
echo "Cleanup complete for: ${BRANCH_NAME}"
