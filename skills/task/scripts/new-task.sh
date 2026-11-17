#!/usr/bin/env bash
set -euo pipefail

# Create (or re-open) the git worktree for a sadensmol project task and drop into it
# in a dedicated agterm session running claude.
#
# A task belongs to ONE project repo. The projects and their repos are in the
# workspace registry (.workspace/settings.json, read through sadensmol-project's
# registry.py). The project is the Linear project of the task's issue: the caller
# reads it and passes --product. A re-open finds the repo in the existing worktree,
# and a branch whose key prefix is a repo name still resolves from its key.

# Usage: new-task.sh <branch-name> --product <project> [--base <base>]... [--pair | --single]
#        new-task.sh <branch-name> --print-command
#        new-task.sh <branch-name> --open [--pair | --single]
#
# `--print-command` prints the start line that resumes the claude of an existing
# task on the model of its recorded stage and changes nothing; `pair.py stage`
# runs it (sadensmol-task SKILL.md, Stages).
#
# `--base` names the branch of the task this one builds on, never `origin/...`.
# Repeatable. Two forms:
#   --base <branch>          the base branch
#   --base <repo>:<branch>   the same, repo-qualified (kept for the shared flow)
# If the base exists on origin, the worktree is cut off it (stacked); if not, the
# repo falls back to its default branch (main). The repo is recorded in <worktree-base>/.task-base (git-config, one section per repo:
# default, base, stacked, upstream, forkPoint — see sadensmol-task
# scripts/task-base.sh), so a re-open without `--base` keeps the stack and rebases
# through forkPoint. The script never talks to GitHub — the base PRs are resolved
# per repo at `finish` time.

BRANCH_NAME=""
PRODUCT_ARG=""
BASE_ARGS=()
PAIR=0
PRINT_COMMAND=0
OPEN=0

while [ $# -gt 0 ]; do
  case "$1" in
    --base|--base-branch)
      [ $# -ge 2 ] || { echo "ERROR: $1 needs <base-branch> or <repo>:<base-branch>"; exit 1; }
      BASE_ARGS+=("$2"); shift 2 ;;
    --base=*|--base-branch=*)
      BASE_ARGS+=("${1#*=}"); shift ;;
    --product)
      [ $# -ge 2 ] || { echo "ERROR: --product needs <project>"; exit 1; }
      PRODUCT_ARG="$2"; shift 2 ;;
    --product=*)
      PRODUCT_ARG="${1#*=}"; shift ;;
    --pair)
      PAIR=1; shift ;;
    --single)
      shift ;;  # explicit single mode: the default, and the recorded mode still wins
    --print-command)
      PRINT_COMMAND=1; shift ;;
    --open)
      OPEN=1; shift ;;
    -*)
      echo "ERROR: unknown option: $1"; exit 1 ;;
    *)
      if [ -z "${BRANCH_NAME}" ]; then BRANCH_NAME="$1"; else
        echo "ERROR: unexpected argument: $1"; exit 1
      fi
      shift ;;
  esac
done

if [ -z "${BRANCH_NAME}" ]; then
  echo "Usage: new-task.sh <branch-name> --product <project> [--base <base>]... [--pair | --single]"
  echo "       new-task.sh <branch-name> --print-command"
  echo "       new-task.sh <branch-name> --open [--pair | --single]"
  exit 1
fi

# Normalize each base to `<branch>` or `<repo>:<branch>`, tolerating a pasted
# `origin/<branch>` — the base is a plain branch name.
BASE_LINES=()
for arg in ${BASE_ARGS[@]+"${BASE_ARGS[@]}"}; do
  case "${arg}" in
    *:*) repo_part="${arg%%:*}"; branch_part="${arg#*:}" ;;
    *)   repo_part="";           branch_part="${arg}" ;;
  esac
  branch_part="${branch_part#origin/}"
  if [ -z "${branch_part}" ] || [ "${branch_part}" = "${BRANCH_NAME}" ]; then
    echo "ERROR: --base ${arg} is empty or the task branch itself."
    exit 1
  fi
  BASE_LINES+=("${repo_part:+${repo_part}:}${branch_part}")
done

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
# The repo: the first repo of --product, else the repo folder of an existing worktree,
# else a branch key whose prefix is a repo name.
PRIMARY_REPO=""
if [ -n "${PRODUCT_ARG}" ]; then
  PRIMARY_REPO="$(SADENSMOL_DIR="${SADENSMOL_DIR}" python3 -I "${REGISTRY_PY}" repo-of-product "${PRODUCT_ARG}")" || exit 1
fi
if [ -z "${PRIMARY_REPO}" ] && [ -d "${WORKTREE_BASE}" ]; then
  for r in ${ALL_REPOS}; do
    if [ -e "${WORKTREE_BASE}/${r}/.git" ]; then PRIMARY_REPO="${r}"; break; fi
  done
fi
if [ -z "${PRIMARY_REPO}" ]; then
  PRIMARY_REPO="$(printf '%s' "${BRANCH_NAME##*/}" | grep -oiE '^[a-z0-9]+-[0-9]+' | sed -E 's/-[0-9]+$//' | tr '[:upper:]' '[:lower:]' || true)"
fi
case " ${ALL_REPOS} " in
  *" ${PRIMARY_REPO} "*) ;;
  *) echo "ERROR: no project repo for branch '${BRANCH_NAME}'; pass --product <project> (the Linear project of its issue); repos: ${ALL_REPOS}"; exit 1 ;;
esac
REPOS=("${PRIMARY_REPO}")
WINDOW_NAME="${BRANCH_NAME##*/}"

# --- Pre-flight checks ---

if [ -z "${AGTERM_ENABLED:-}" ] || ! command -v agtermctl >/dev/null 2>&1; then
  echo "ERROR: Not running inside agterm. Start one first."
  exit 1
fi

if [ ! -d "${SADENSMOL_DIR}/${PRIMARY_REPO}/.git" ] && [ ! -f "${SADENSMOL_DIR}/${PRIMARY_REPO}/.git" ]; then
  echo "ERROR: ${SADENSMOL_DIR}/${PRIMARY_REPO} is not a git repository."
  exit 1
fi

# --- Helper: detect a repo's default branch (master or main) ---
#
# Every new task branch is cut off the repo's own remote default tip.

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

# --- The per-repo base record (.task-base) ---
#
# tb_* helpers from the sadensmol-task skill: the record format, the stacked
# rebase through forkPoint and the re-anchor after a merged base live there.

TASK_BASE_SH=""
for d in "$(cd "$(dirname "$0")/.." && pwd -P)" "$HOME/.claude/skills/sadensmol-task" "$HOME/.agents/skills/sadensmol/task"; do
  if [ -f "$d/scripts/task-base.sh" ]; then TASK_BASE_SH="$d/scripts/task-base.sh"; break; fi
done
if [ -z "${TASK_BASE_SH}" ]; then
  echo "ERROR: needs scripts/task-base.sh from the sadensmol-task skill."
  exit 1
fi
# shellcheck source=/dev/null
. "${TASK_BASE_SH}"

# pair.py of the sadensmol-task skill: the stage record (claude-args) and pair mode.
PAIR_PY="$(dirname "${TASK_BASE_SH}")/pair.py"
if [ ! -f "${PAIR_PY}" ]; then
  echo "ERROR: needs scripts/pair.py from the sadensmol-task skill."
  exit 1
fi
SELF="$(cd "$(dirname "$0")" && pwd -P)/$(basename "$0")"

# Autopilot: every spawned claude runs with auto-approved permissions and a
# system-prompt instruction to resolve routine questions itself, asking the
# user only critical ones. No apostrophes in AUTOPILOT — the string is nested
# inside single quotes for the agterm --command.

AUTOPILOT="Workflow: task. Follow sadensmol-task, not the Autopilot service or its worker protocol. Work autonomously and do not stop for routine questions - resolve them yourself using the Linear issue, the codebase, and skill conventions. While planning, never stop to ask the user: write every unanswered question into PLAN.md section Questions to clarify with the answer the plan assumes, finish the plan, and ask those questions one at a time after the plan gate (task skill, references/plan.md step 6). Two gates ALWAYS require the user and are never auto-approved: (1) plan review - before approval only: after writing or changing PLAN.md, open the plannotator gate and wait for explicit user approval; once approved, start implementing immediately without asking again; (2) pre-PR diff review - reach it on your own: implement, test, run the local code review and fix its findings, then open the plannotator review and wait for the user to approve the exact diff before any commit, push, PR, or merge. The plan stage is one-way: approval ends it for good, and you never return to it. After approval PLAN.md is frozen, general direction only: no planner, no plan diff, no plan gate, no stage change, no PLAN.md edit. All feedback after approval is implementation work: fix it in the code and show it at the code review gate. For a critical decision (task skill, Autonomy), ask the user, then implement; decide every other design choice yourself and log it. User decisions after approval go to .task-user-decisions.md (task skill, User decisions log), never to PLAN.md. Beyond those gates, ask the user only for a critical decision: destructive or irreversible actions, production impact, scope changes, reversing a user decision, or skipping a guard. Everything else: pick the sensible default and proceed."

# --- The task stage: claude runs on the model of the recorded stage ---
#
# pair.py claude-args prints the stage flags (sadensmol-task SKILL.md, Stages) and
# records this script, so `pair.py stage` can call --print-command.
STAGE_ARGS=(--base "${WORKTREE_BASE}" --launcher "${SELF}" --branch "${BRANCH_NAME}")
# pair.py fresh passes TASK_CONTINUE: the first prompt of a new conversation.
STAGE_CONTINUE="${TASK_CONTINUE:-The task stage changed and you run on its model now: continue the task where you left off.}"

if [ "${PRINT_COMMAND}" = 1 ]; then
  if [ -f "${WORKTREE_BASE}/.pair/config.json" ]; then
    PAIR_BRIEF="$(python3 "${PAIR_PY}" brief --base "${WORKTREE_BASE}" --role writer)"
    echo "zsh -lc 'claude --dangerously-skip-permissions --settings \"${WORKTREE_BASE}/.pair/claude-writer-settings.json\" $(python3 "${PAIR_PY}" claude-args "${STAGE_ARGS[@]}") --append-system-prompt \"${AUTOPILOT} ${PAIR_BRIEF}\" \"${STAGE_CONTINUE}\"'"
  else
    echo "zsh -lc 'claude --dangerously-skip-permissions $(python3 "${PAIR_PY}" claude-args "${STAGE_ARGS[@]}") --append-system-prompt \"${AUTOPILOT}\" \"${STAGE_CONTINUE}\"'"
  fi
  exit 0
fi

# --- Helper: resolve Python deps in a fresh worktree ---
#
# `.venv/` is gitignored and per-worktree, so a fresh worktree cannot run pytest
# until `uv sync` runs. Non-fatal: without uv on PATH it warns.
refresh_python_deps() {
  local wt="$1"
  [ -f "${wt}/pyproject.toml" ] || return 0
  if command -v uv >/dev/null 2>&1; then
    echo "uv sync: ${wt}"
    ( cd "${wt}" && uv sync ) || echo "WARN: uv sync failed in ${wt}"
  else
    echo "WARN: uv not on PATH — run 'uv sync' in the worktree before testing."
  fi
}

# --open: `task switch` on an existing worktree. It opens the session and leaves every
# repo as it is: no stash, fetch, rebase or dep refresh.
if [ "${OPEN}" = 1 ] && [ ! -d "${WORKTREE_BASE}" ]; then
  echo "ERROR: --open needs an existing worktree: ${WORKTREE_BASE}"
  exit 1
fi

if [ "${OPEN}" = 0 ]; then

# --- Ensure the worktree base exists ---

mkdir -p "${WORKTREE_BASE}"

# The record lives at the worktree BASE (outside the git repo, so it is never
# staged or committed). `--base` replaces every repo's requested base; without it
# the recorded bases stay. `finish` reads it and looks up, per repo, whether that
# branch still has an open PR there — that is what turns into
# `gh pr create --base <upstream>`.
tb_migrate "${WORKTREE_BASE}"
if [ ${#BASE_LINES[@]} -gt 0 ]; then
  for repo in "${REPOS[@]}"; do
    tb_unset "${WORKTREE_BASE}" "${repo}" base
    tb_unset "${WORKTREE_BASE}" "${repo}" basePr
    tb_unset "${WORKTREE_BASE}" "${repo}" baseTask
    b="$(tb_pick_base "${repo}" "${BASE_LINES[@]}")"
    if [ -n "${b}" ]; then tb_set "${WORKTREE_BASE}" "${repo}" base "${b}"; fi
  done
  echo "Recorded base branches in ${WORKTREE_BASE}/.task-base: ${BASE_LINES[*]}"
fi

# Copy Claude permissions template to the worktree base
PERMISSIONS_SRC="${SADENSMOL_DIR}/.workspace/task/settings.local.json"
if [ -f "${PERMISSIONS_SRC}" ]; then
  mkdir -p "${WORKTREE_BASE}/.claude"
  cp "${PERMISSIONS_SRC}" "${WORKTREE_BASE}/.claude/settings.local.json"
  echo "Copied Claude permissions to ${WORKTREE_BASE}/.claude/settings.local.json"
else
  echo "WARN: no permissions file ${PERMISSIONS_SRC}; the worktree gets no .claude/settings.local.json"
fi

# --- Ensure ONE repo's worktree exists ---
#
# Creates the worktree (cutting the branch off the repo's upstream when it does
# not exist yet) or refreshes an existing one. Returns non-zero only when the
# worktree could not be made at all. Whether planning auto-starts is decided
# AFTER this loop, from the task's own state — not from what this run created.

ensure_worktree() {
  local REPO_NAME="$1"
  local REPO_DIR="${SADENSMOL_DIR}/${REPO_NAME}"
  local WORKTREE_PATH="${WORKTREE_BASE}/${REPO_NAME}"

  if [ ! -d "${REPO_DIR}/.git" ] && [ ! -f "${REPO_DIR}/.git" ]; then
    echo "WARN: ${REPO_DIR} is not cloned — skipping ${REPO_NAME}."
    return 0
  fi

  local DEFAULT_BRANCH
  DEFAULT_BRANCH="$(get_default_branch "${REPO_DIR}")"

  git -C "${REPO_DIR}" worktree prune 2>/dev/null || true

  echo ""
  echo "── ${REPO_NAME} ──"
  # The branch this repo follows: its base when stacked (the base is on origin),
  # else its default branch. tb_plan prints a NOTE for a missing or merged base.
  local UPSTREAM_BRANCH
  UPSTREAM_BRANCH="$(tb_plan "${WORKTREE_BASE}" "${REPO_NAME}" "${REPO_DIR}" "${DEFAULT_BRANCH}")"
  if [ "$(tb_get "${WORKTREE_BASE}" "${REPO_NAME}" stacked)" = true ]; then
    echo "STACKED on origin/${UPSTREAM_BRANCH}"
  fi

if [ -d "${WORKTREE_PATH}" ]; then
  # Worktree already exists — update to latest while preserving local changes.
  echo "Worktree already exists: ${WORKTREE_PATH}"

  has_changes=false
  if [ -n "$(git -C "${WORKTREE_PATH}" status --porcelain 2>/dev/null)" ]; then
    has_changes=true
    git -C "${WORKTREE_PATH}" stash push -m "task-script-auto-stash" --include-untracked 2>/dev/null || true
  fi

  # A stacked repo follows its BASE branch — rebasing it onto master/main would
  # drop it off the stack and replay the base task's commits as its own. tb_sync
  # rebases --onto the upstream tip from forkPoint, so a force-pushed or merged
  # base never replays the base task's commits either.
  git -C "${WORKTREE_PATH}" fetch origin 2>/dev/null || true
  if ! tb_sync "${WORKTREE_BASE}" "${REPO_NAME}" "${WORKTREE_PATH}"; then
    git -C "${WORKTREE_PATH}" rebase "origin/${BRANCH_NAME}" 2>/dev/null ||
      git -C "${WORKTREE_PATH}" rebase --abort 2>/dev/null || true
  fi

  if [ "${has_changes}" = true ]; then
    git -C "${WORKTREE_PATH}" stash pop 2>/dev/null || true
  fi
else
  # Fresh worktree — branch off the REMOTE tip of the upstream branch so it never
  # starts on a stale local ref or inherits uncommitted changes from the source
  # working tree. Upstream is this repo's default branch (main), or the
  # requested base branch when this repo actually carries it (stacked task).
  # Prefer origin/<upstream>; fall back to the local ref.
  echo "Creating worktree: ${WORKTREE_PATH} (base: ${UPSTREAM_BRANCH})"
  git -C "${REPO_DIR}" fetch origin "${UPSTREAM_BRANCH}" 2>/dev/null || true
  if git -C "${REPO_DIR}" show-ref --verify --quiet "refs/remotes/origin/${UPSTREAM_BRANCH}"; then
    BASE_REF="origin/${UPSTREAM_BRANCH}"
  else
    BASE_REF="${UPSTREAM_BRANCH}"
  fi

  if ! git -C "${REPO_DIR}" worktree add -b "${BRANCH_NAME}" "${WORKTREE_PATH}" "${BASE_REF}" 2>/dev/null; then
    # Branch already exists — attach a worktree onto it (remote tip preferred,
    # then the local ref) instead of creating a new branch.
    git -C "${REPO_DIR}" fetch origin "${BRANCH_NAME}" 2>/dev/null || true
    # Attach the worktree to a real LOCAL branch — never a detached HEAD.
    # `worktree add <path> origin/<branch>` would check out the remote-tracking
    # ref in DETACHED HEAD state, so branch off it explicitly instead.
    if git -C "${REPO_DIR}" show-ref --verify --quiet "refs/heads/${BRANCH_NAME}"; then
      # Local branch already exists → check it out in the worktree.
      git -C "${REPO_DIR}" worktree add "${WORKTREE_PATH}" "${BRANCH_NAME}"
    elif git -C "${REPO_DIR}" show-ref --verify --quiet "refs/remotes/origin/${BRANCH_NAME}"; then
      # Only the remote branch exists → create a local tracking branch and attach.
      git -C "${REPO_DIR}" worktree add -b "${BRANCH_NAME}" "${WORKTREE_PATH}" "origin/${BRANCH_NAME}"
    else
      echo "ERROR: Failed to create worktree for ${BRANCH_NAME} in ${REPO_NAME}"
      return 1
    fi
  fi
  tb_mark_fork "${WORKTREE_BASE}" "${REPO_NAME}" "${WORKTREE_PATH}"

  # Copy gitignored local config the worktree needs to run (.env files). They are
  # not tracked, so a fresh worktree won't have them.
  while IFS= read -r -d '' f; do
    rel="${f#${REPO_DIR}/}"
    target="${WORKTREE_PATH}/${rel}"
    mkdir -p "$(dirname "${target}")"
    cp "${f}" "${target}"
  done < <(find "${REPO_DIR}" \
    -name '.env' \
    -not -path '*/.worktrees/*' -not -path '*/.git/*' -not -path '*/.venv/*' \
    -not -path '*/node_modules/*' -not -path '*/build/*' -print0 2>/dev/null)

  # Fresh worktree → resolve Python deps so pytest runs immediately.
  refresh_python_deps "${WORKTREE_PATH}"
fi

  echo "Worktree ready: ${WORKTREE_PATH} ($(git -C "${WORKTREE_PATH}" rev-parse --abbrev-ref HEAD 2>/dev/null))"
}

# --- Create/refresh the project's worktree ---

ensure_worktree "${PRIMARY_REPO}" || exit 1

fi # OPEN = 0

# --- Does this task still need planning? ---
#
# The signal is the TASK's state, NOT whether a worktree happened to be created
# in THIS run. Keying off "was a worktree just created" is fragile: an
# interrupted earlier run, a re-run, or a closed session all leave the worktrees
# in place, and the task then never gets planned at all.
#
# A task still needs planning when it has produced NOTHING yet:
#   * no PLAN.md at the worktree base, AND
#   * no uncommitted changes, AND
#   * no commits ahead of the upstream branch.
# Anything else means work exists (planned or in progress) → plain claude, so a
# fresh `task plan` can never clobber WIP. Pair mode follows the same rule: the
# writer is seeded with `task plan` exactly like a solo session.
#
# Dep churn that this script produces itself (`uv sync` rewriting uv.lock) and
# harness/OS files are NOT work — counting them made a
# brand-new worktree look "in progress" and skipped planning.
has_real_changes() {
  git -C "$1" status --porcelain -uall 2>/dev/null | while IFS= read -r line; do
    f="${line:3}"
    case "$f" in
      uv.lock|*/uv.lock) ;;
      .claude/*|*/.claude/*) ;;
      .DS_Store|*/.DS_Store) ;;
      *) echo x; break ;;
    esac
  done | grep -q x
}

should_plan=1
plan_reason="no PLAN.md and no work yet"
if [ -f "${WORKTREE_BASE}/PLAN.md" ]; then
  should_plan=0
  plan_reason="PLAN.md already exists"
else
  for repo in "${REPOS[@]}"; do
    wt="${WORKTREE_BASE}/${repo}"
    [ -d "${wt}" ] || continue
    git -C "${wt}" rev-parse --git-dir >/dev/null 2>&1 || continue

    if has_real_changes "${wt}"; then
      should_plan=0
      plan_reason="${repo} has uncommitted changes"
      break
    fi

    wt_upstream_branch="$(tb_get "${WORKTREE_BASE}" "${repo}" upstream)"
    wt_upstream="origin/${wt_upstream_branch:-$(get_default_branch "${wt}")}"
    if git -C "${wt}" rev-parse --verify -q "${wt_upstream}" >/dev/null 2>&1 &&
       [ "$(git -C "${wt}" rev-list --count "${wt_upstream}..HEAD" 2>/dev/null || echo 0)" -gt 0 ]; then
      should_plan=0
      plan_reason="${repo} has commits ahead of ${wt_upstream}"
      break
    fi
  done
fi

if [ "${should_plan}" -eq 1 ]; then
  echo "Planning: will seed 'task plan' (${plan_reason})"
else
  echo "Planning: skipped, plain claude (${plan_reason})"
fi

echo ""
echo "Worktrees ready under: ${WORKTREE_BASE}"

# --- Open a new agterm session and switch to it ---
#
# `should_plan` (computed above from the TASK's state, not from what this run
# happened to create) decides how claude starts: seeded with "task plan" when the
# task has produced nothing yet, plain otherwise so WIP is never clobbered.
# AUTOPILOT is defined above, before --print-command. A single session takes the
# stage flags here (the model of the task stage); a pair gets them below.
WRITER_STAGE_ARGS=""
if [ "${PAIR}" = 0 ] && [ ! -f "${WORKTREE_BASE}/.pair/config.json" ]; then
  WRITER_STAGE_ARGS="$(python3 "${PAIR_PY}" claude-args "${STAGE_ARGS[@]}")"
fi
CLAUDE_CMD="claude --dangerously-skip-permissions ${WRITER_STAGE_ARGS} --append-system-prompt \"${AUTOPILOT}\""

# agterm sessions are named "<task id> <descriptor>" (e.g.
# "ABC-21 add-health-endpoint") when the branch carries a <KEY>-<n> id —
# the id for quick scanning plus the descriptor so you can tell what the worktree
# is about. Falls back to the whole branch name when there's no id prefix.
# cleanup-task.sh MUST mirror this exact naming to find & close the session.
TASK_ID="$(printf '%s' "${WINDOW_NAME}" | grep -oiE '^[a-z0-9]+-[0-9]+' | tr '[:lower:]' '[:upper:]' || true)"
AGTERM_DESC="$(printf '%s' "${WINDOW_NAME}" | sed -E 's/^[a-zA-Z0-9]+-[0-9]+-//')"
if [ -n "${TASK_ID}" ] && [ -n "${AGTERM_DESC}" ] && [ "${AGTERM_DESC}" != "${WINDOW_NAME}" ]; then
  AGTERM_NAME="${TASK_ID} ${AGTERM_DESC}"
else
  AGTERM_NAME="${TASK_ID:-${WINDOW_NAME}}"
fi

# Find an existing session with this name. jq is NOT assumed: without a
# fallback a missing jq leaves existing_id empty and a DUPLICATE session is
# created every time. python3 ships with macOS, so it is the fallback.
existing_id=""
if command -v jq >/dev/null 2>&1; then
  existing_id="$(agtermctl tree --json 2>/dev/null \
    | jq -r --arg n "${AGTERM_NAME}" '.result.tree.workspaces[]?.sessions[]? | select(.name==$n) | .id' 2>/dev/null \
    | head -n1)"
elif command -v python3 >/dev/null 2>&1; then
  existing_id="$(agtermctl tree --json 2>/dev/null \
    | AGT_NAME="${AGTERM_NAME}" python3 -c 'import sys,json,os
name = os.environ["AGT_NAME"]
try:
  tree = json.load(sys.stdin)["result"]["tree"]
except Exception:
  sys.exit(0)
for ws in tree.get("workspaces", []):
  for sess in ws.get("sessions", []):
      if sess.get("name") == name:
          print(sess.get("id", ""))
          sys.exit(0)' 2>/dev/null)"
fi

# Place the new session in the CURRENT session's workspace, directly BELOW it.
# `--after <anchor>` positions the new session right after the anchor AND makes
# it inherit the anchor's workspace (replacing --workspace). AGTERM_SESSION_ID
# is the session running this script. The ${arr[@]+...} expansion is empty-array
# safe under `set -u` (macOS bash 3.2) when AGTERM_SESSION_ID is somehow unset.
after_args=()
[ -n "${AGTERM_SESSION_ID:-}" ] && after_args=(--after "${AGTERM_SESSION_ID}")

# --- Pair mode: writer (left) + read-only reviewer (right); see sadensmol-task pair.md ---
#
# A new session starts in pair mode. A live session becomes one only on an explicit
# --pair: it is replaced by a new session whose claude resumes its previous
# conversation (pair.py claude-args). A live session that is already paired is
# just selected.
PAIR_REQUESTED="${PAIR}"
PAIR_REOPEN=0
PAIR_RESPAWN=0
if [ -f "${WORKTREE_BASE}/.pair/config.json" ]; then
  PAIR=1
  PAIR_REOPEN=1
fi
# the writer's stage flags on respawn: resume its conversation
RESUME_ARGS=()
if [ "${PAIR_REQUESTED}" = 1 ] && [ -n "${existing_id}" ] \
   && [ "$(python3 "${PAIR_PY}" inspect --base "${WORKTREE_BASE}" --session "${existing_id}" --get paired)" != 1 ]; then
  if [ "$(python3 "${PAIR_PY}" inspect --session "${existing_id}" --get writer)" = claude ] \
     && [ "${AGTERM_SESSION_ID:-}" = "${existing_id}" ] && [ -n "${CLAUDE_CODE_SESSION_ID:-}" ]; then
    # that claude runs this script: its conversation is the one to resume
    RESUME_ARGS=(--resume-id "${CLAUDE_CODE_SESSION_ID}")
  fi
  # the agent running this script may be that session's own writer: it asked for this
  if [ "${AGTERM_SESSION_ID:-}" != "${existing_id}" ] \
     && [ "$(python3 "${PAIR_PY}" inspect --session "${existing_id}" --get status)" = active ]; then
    echo "ERROR: the claude agent in '${AGTERM_NAME}' is busy. Run --pair again when it is idle."
    exit 1
  fi
  PAIR_RESPAWN=1
fi
if [ "${PAIR}" = 1 ] && { [ -z "${existing_id}" ] || [ "${PAIR_RESPAWN}" = 1 ]; }; then
  python3 "${PAIR_PY}" init --base "${WORKTREE_BASE}" --writer claude >/dev/null
  PAIR_BRIEF="$(python3 "${PAIR_PY}" brief --base "${WORKTREE_BASE}" --role writer)"
  # --settings: the pair hook forwards every user message to the reviewer
  CLAUDE_CMD="claude $(python3 "${PAIR_PY}" claude-args "${STAGE_ARGS[@]}" ${RESUME_ARGS[@]+"${RESUME_ARGS[@]}"}) --dangerously-skip-permissions --settings \"${WORKTREE_BASE}/.pair/claude-writer-settings.json\" --append-system-prompt \"${AUTOPILOT} ${PAIR_BRIEF}\""
fi

session_id="${existing_id}"
if [ "${PAIR_RESPAWN}" = 1 ]; then
  echo "Restarting '${AGTERM_NAME}' in pair mode; claude continues its conversation..."
  session_id="$(python3 "${PAIR_PY}" respawn --old "${existing_id}" --name "${AGTERM_NAME}" \
    --cwd "${WORKTREE_BASE}" \
    --command "zsh -lc '${CLAUDE_CMD} \"Pair mode is now on for this task: continue the task where you left off.\"'")"
elif [ -n "${existing_id}" ]; then
  # Never move the user's focus: the existing session stays where it is.
  echo "agterm session '${AGTERM_NAME}' already exists; focus is not moved to it."
  # The session is already running a plain claude, so --command can no longer
  # seed it. Type the subcommand in instead — otherwise an unplanned task
  # silently stays unplanned and the user has to run `task plan` by hand.
  if [ "${should_plan}" -eq 1 ]; then
    echo "Seeding 'task plan' into the existing session..."
    agtermctl session type "task plan" --target "${existing_id}"
  fi
else
  # zsh -lc so claude resolves via the login PATH (agterm --command uses the GUI PATH).
  # --no-select opens the session in the background, so the user's focus never moves to it.
  if [ "${should_plan}" -eq 1 ]; then
    new_out="$(agtermctl session new --cwd "${WORKTREE_BASE}" --name "${AGTERM_NAME}" ${after_args[@]+"${after_args[@]}"} \
      --no-select --command "zsh -lc '${CLAUDE_CMD} \"task plan\"'")"
  else
    new_out="$(agtermctl session new --cwd "${WORKTREE_BASE}" --name "${AGTERM_NAME}" ${after_args[@]+"${after_args[@]}"} \
      --no-select --command "zsh -lc '${CLAUDE_CMD}'")"
  fi
  # `session new` prints the new session id — capture it so the context/flag
  # calls below can target the session just created. `|| true` because pipefail
  # turns a non-matching grep into a fatal error.
  session_id="$(printf '%s' "${new_out}" | grep -oiE '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}' | head -n1 || true)"
  echo "Opened agterm session: ${AGTERM_NAME}"
fi

# Put the task id in the session's title bar and add the session to the flagged
# working set, so every live task is one flagged-view keystroke away. Both need
# agterm 0.26.0+; on an older build they fail and the task still opens. The row
# tint (agterm 0.39+) marks the workspace's tasks in the sidebar.
if [ -n "${session_id}" ]; then
  agtermctl session context "${TASK_ID:-${WINDOW_NAME}}" --target "${session_id}" >/dev/null 2>&1 || true
  agtermctl session flag on --target "${session_id}" >/dev/null 2>&1 || true
  agtermctl session tint blue --target "${session_id}" >/dev/null 2>&1 || true
fi

if [ "${PAIR}" = 1 ] && [ -n "${session_id}" ] \
   && { [ -z "${existing_id}" ] || [ "${PAIR_RESPAWN}" = 1 ]; }; then
  launch_args=(--base "${WORKTREE_BASE}" --session "${session_id}")
  # a respawned reviewer joins work already in progress, so it gets the catch-up too
  if [ "${PAIR_REOPEN}" = 1 ] || [ "${PAIR_RESPAWN}" = 1 ]; then launch_args+=(--reopen); fi
  python3 "${PAIR_PY}" launch "${launch_args[@]}" \
    || echo "WARNING: pair launch failed (see above); the writer runs without a reviewer."
elif [ "${PAIR}" = 1 ] && [ -n "${existing_id}" ]; then
  echo "Pair mode: the existing session is already paired (or --pair was not given); selected it."
fi
