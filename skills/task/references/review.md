# `review` — handled by the dispatcher

**Triggers:** "task review", "review task", "review changes", "review", "open
plannotator", "open plannotator please", "open annotate".

`review` opens plannotator at the worktree base and waits. Nothing else. Follow
`references/plannotator.md` for the launch, the one-instance-per-worktree rule and the
stale-`annotate` rule.

## Pick the mode by state, never blindly `review` (MUST FOLLOW)

`plannotator review` shows a raw code diff and cannot render mermaid or markdown tables.
So markdown is **always** `annotate`, even when the user typed `task review`:

- **PLAN stage** (`pair.py plan-open` exits 0) → **`annotate PLAN.md`**, always. `PLAN.md`
  wins over code changes and over other `.md` files; do not ask. No `PLAN.md` yet → say
  "nothing to open" and stop, even when code changes exist. A plain annotate — no
  `--gate`; the plan gate belongs to `plan`.
- **IMPL stage:**
  - **`review`** — only when a real code change exists: a changed or untracked non-`.md`
    file that `real_changes` keeps (task-new dep churn never counts).
  - **`annotate <file.md>`** — when the only changes are markdown (docs). One `.md` → that
    file. Several → `annotate` opens one file at a time: open them one after another in
    path order, the next one when the previous one returns. Name the order in chat. Never
    fall back to `review` to show them all. A plain annotate.
  - **Nothing to open** → say so and stop.
- **Never** generate or open an HTML page, a scratch file or a report for `review`. Never
  manufacture a diff or a plan.

Classify with `real_changes` (`references/worktree-helpers.md`), base derived the same way:

```bash
stage=IMPL
python3 <sadensmol-task-skill-dir>/scripts/pair.py plan-open --base "$base" >/dev/null 2>&1 && stage=PLAN
plan_md=0; [ -f "$base/PLAN.md" ] && plan_md=1
code=0; mds=""
for d in "$base"/*/; do
  [ -d "$d" ] || continue
  git -C "$d" rev-parse --git-dir >/dev/null 2>&1 || continue
  repo="$(basename "${d%/}")"
  while IFS= read -r f; do
    case "$f" in *.md) mds="$mds $repo/$f" ;; *) code=1 ;; esac
  done < <(real_changes "$d")
done
echo "STAGE=$stage"; echo "PLAN_MD=$plan_md"; echo "CODE=$code"; echo "MD=$mds"
```

`real_changes` uses `status --porcelain -uall`. Without `-uall`, git collapses an
untracked dir to `docs/rag/`, and a docs-only change reads as code. `plan-open` exits 1 on
IMPL, so `review` never opens a closed plan.

- `STAGE=PLAN` + `PLAN_MD=1` → `annotate PLAN.md`. `STAGE=PLAN` + `PLAN_MD=0` → nothing
  to open.
- `STAGE=IMPL`: `CODE=1` → `review`. `CODE=0` + one md → `annotate` it. `CODE=0` +
  several → annotate each in path order. `CODE=0` + none → nothing to open.

**Neither mode writes code (MUST FOLLOW).** A review or annotate request is never
"implement the plan", "make the changes then review" or "approved, so proceed". If
opening plannotator would first need a diff or a plan, you are in the wrong phase: stop
and ask.

## Drop go mod churn before a code review (MUST FOLLOW)

Before every code `review` launch (`CODE=1`), discard the `go.mod`/`go.sum` changes of each
repo that has no real change (`real_changes` prints nothing). They are `go get`/`go mod tidy`
churn with no code behind it, and must reach neither the review nor the final diff. A repo
with real changes keeps its `go.mod`/`go.sum` changes.

```bash
for d in "$base"/*/; do
  git -C "$d" rev-parse --git-dir >/dev/null 2>&1 || continue
  [ -z "$(real_changes "$d")" ] || continue
  mods=$(git -C "$d" diff --name-only HEAD -- '*go.mod' '*go.sum')
  [ -n "$mods" ] || continue
  printf '%s\n' "$mods" | xargs git -C "$d" checkout HEAD -- && echo "DISCARDED $(basename "${d%/}"): $mods"
done
```

## Launch

Check this worktree's slot first (instances of other worktrees do not count):

```bash
H=<sadensmol-task-skill-dir>/scripts/plannotator-session.sh
cd <worktree-base> && bash "$H" --status
```

- `RUNNING: …` → reuse it or follow the note-preservation rules in
  `references/plannotator.md` before replacing it.
- `NONE` → launch exactly one, in the background, from the worktree base. A
  `PLANNOTATOR_ALREADY_RUNNING` result (exit 4) means the slot was taken meanwhile:
  treat it as `RUNNING`.

```bash
cd <worktree-base> && pwd && PLANNOTATOR_PROMOTE=code bash "$H" review   # CODE=1, NO_PREVIOUS (first review) or NO_DELTA
cd <worktree-base> && pwd && PLANNOTATOR_PROMOTE=code PLANNOTATOR_GUIDE=<worktree-base>/.scratch/review-guide.json bash "$H" review --patch-file .scratch/review-delta.patch   # CODE=1, DELTA
cd <worktree-base> && pwd && bash "$H" annotate PLAN.md   # STAGE=PLAN, PLAN_MD=1
cd <worktree-base> && pwd && bash "$H" annotate <chosen.md>   # STAGE=IMPL, markdown only
```

- **A re-review with changes shows ONLY the delta (MUST FOLLOW).** `DELTA` launches
  `review --patch-file` on the delta patch. `NO_DELTA` opens the whole worktree diff.
  Never use `--diff-type last-commit` for it.

- `PLANNOTATOR_PROMOTE=code` on **every** code `review`: the helper stages a snapshot of
  every repo, and it becomes the baseline of the next `delta` only when the page opens. A
  failed start or `PLANNOTATOR_ALREADY_RUNNING` keeps the old baseline. Never run `save`
  by hand.

- Run from the base (not a repo subdir), so `review` aggregates every nested repo
  (plannotator ≥ 0.21 discovers them) and `PLAN.md` resolves. Verify `pwd` first.
- `annotate` takes exactly one `.md` path, relative to the base (`PLAN.md`, or
  `<repo>/docs/…/report.md`). Remember it — the stale-`annotate` rule needs it.
- `review` shows the **uncommitted** diff only, so always review before committing.
- A completion or `failed` notification usually means the user closed the tab, not that
  the cwd was wrong. Do not relaunch.
- When it returns, address each annotation (keyed `<repo>/<path>` in a multi-repo
  `review`) and re-run if needed. At the pre-PR gate, use the explicit approval rule in
  finish.md §4b. An empty annotation set alone is not approval.
- When it finishes, the next step is `finish` or more work — see SKILL.md → *Never
  suggest cleanup while work remains*.

## Changes since the last review (MUST FOLLOW)

A code `review` (not `annotate`, not the plan gate) that follows changes since the
previous one explains them — after annotation fixes, local code-review fixes, CI or
PR-review fixes — as text, never as a diff, so a re-review starts from the reasons. The
first review and an unchanged review open without a guide. Right before every code
`review` launch:

```bash
S=<sadensmol-task-skill-dir>/scripts/review-snapshot.sh
cd <worktree-base> && bash "$S" delta
```

- `delta` compares every repo with what the last code `review` showed (commits in between
  do not matter), writes `<base>/.scratch/review-delta.patch` and prints a `--stat`. The
  plan gate keeps its own baseline (`references/plan.md`, step 6).
- The baseline is the last code review that **opened**, not proof of approval.
- Before a `DELTA` launch: if any repo has `refs/task-review/last`, every changed repo must
  have its own — `delta` silently skips a repo without one. A repo without it → name it
  and stop. Never fall back to the full diff.
- `NO_PREVIOUS` → first review: no guide, no notice. Launch the full diff without
  `PLANNOTATOR_GUIDE`. `NO_PREVIOUS` after a known earlier code review → report the lost
  baseline and stop.
- `NO_DELTA` → say "No changes since the last review. Plannotator shows the whole worktree
  diff." Launch plain `review` with `PLANNOTATOR_PROMOTE=code`: no `PLANNOTATOR_GUIDE`, no
  `--patch-file`, no notice.
- `DELTA` → read the patch, then write the guide to `<base>/.scratch/review-guide.json`
  (plannotator's Guided Review schema) and install it with `bash "$S" guide
  <base>/.scratch/review-guide.json` (expect `GUIDE_SAVED`):

  ```json
  {"title": "Changes since the last review (2 annotations, 1 code-review P1)",
   "intent": "What changed after your last review, and why.",
   "sections": [{"title": "Return the save error instead of logging it",
     "overview": "**Your annotation on `core/internal/usecase/bet.go`:** the usecase logged and returned, so a cancelled request produced an error line. It now wraps with `%w` and returns; the boundary logs it once, at warn.",
     "diffs": [{"file": "core/internal/usecase/bet.go", "summary": "Drops the log line; returns the wrapped error."}]}],
   "unplacedFiles": []}
  ```

  One section per reason (an annotation, a finding, a CI failure, a PR comment, or a
  change nobody asked for). `overview`: 2–6 sentences of markdown on what was wrong,
  what changed and why. Paths exactly as the `--stat` prints them; every delta file in
  exactly one section. `guide` refuses a missing, extra or duplicated file — fix the
  JSON, never drop a file to make it pass. Launch with `PLANNOTATOR_GUIDE` (see *Launch*).
- Post in chat, before the launch, a short text summary (one line per section: its
  title and reason) and this notice:

  > Changes since the last review: plannotator shows only these changes, with the Guided
  > Review *Changes since the last review*.

- Plannotator shows only the delta patch; the guide explains it.
- `plannotator-session.sh` wraps the opener with `guide-open.sh`, which loads the guide
  into the server before the page opens (`review-snapshot.sh inject`). After the launch,
  read `<base>/.scratch/review-guide.status` (it appears within seconds):
  - `GUIDE_INJECTED …` → done. Each `DROPPED: <file>` (not in the review's diff) or
    `UNPLACED: <file>` (in the diff, in no section) is a guide mistake: say so in chat.
  - `GUIDE_INJECT_FAILED: <why>` → report it in one line. The page opened without a
    guide. The guide is still in plannotator's saved guides (Guided Review → saved list).
  - `PROMOTE_FAILED: …` → report it in one line: the next `delta` compares with the old
    baseline.
