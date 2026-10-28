# `review` — handled by the dispatcher

**Triggers:** "task review", "review task", "review changes", "review", "open
plannotator", "open plannotator please", "open annotate".

`review` opens plannotator at the worktree base and waits. Nothing else. Follow
SKILL.md → *Plannotator binary (shared)* for the launch, the one-instance-per-worktree rule and the
stale-`annotate` rule.

## Pick the mode by state, never blindly `review` (MUST FOLLOW)

`plannotator review` shows a raw code diff and cannot render mermaid or markdown tables.
So markdown is **always** `annotate`, even when the user typed `task review`:

- **`review`** — only when a real code change exists: a changed or untracked non-`.md`
  file that `real_changes` keeps (task-new dep churn never counts).
- **`annotate <file.md>`** — when the only changes are markdown (`PLAN.md`, docs). One
  `.md` → that file. Several → `annotate` opens one file at a time, so ask the user
  (`ask-user`) which one. Never pick one silently, and never fall back to `review` to show
  them all. A plain annotate — no `--gate`; the plan gate belongs to `plan`.
- **Nothing to open** → say so and stop. Never manufacture a diff or a plan.

Classify with `real_changes` (SKILL.md → *Worktree helpers*), base derived the same way:

```bash
code=0; mds=""
[ -f "$base/PLAN.md" ] && mds="PLAN.md"
for d in "$base"/*/; do
  [ -d "$d" ] || continue
  git -C "$d" rev-parse --git-dir >/dev/null 2>&1 || continue
  repo="$(basename "${d%/}")"
  while IFS= read -r f; do
    case "$f" in *.md) mds="$mds $repo/$f" ;; *) code=1 ;; esac
  done < <(real_changes "$d")
done
echo "CODE=$code"; echo "MD=$mds"
```

`real_changes` uses `status --porcelain -uall`. Without `-uall`, git collapses an
untracked dir to `docs/rag/`, and a docs-only change reads as code.

- `CODE=1` → `review`. `CODE=0` + one md → `annotate` it. `CODE=0` + several → ask.
  `CODE=0` + none → nothing to open.

**Neither mode writes code (MUST FOLLOW).** A review or annotate request is never
"implement the plan", "make the changes then review" or "approved, so proceed". If
opening plannotator would first need a diff or a plan, you are in the wrong phase: stop
and ask.

## Launch

Check this worktree's slot first (instances of other worktrees do not count):

```bash
H=<sadensmol-task-skill-dir>/scripts/plannotator-session.sh
cd <worktree-base> && bash "$H" --status
```

- `RUNNING: …` → do not launch a second one; ask the user to force-close it or keep
  it (SKILL.md → *Plannotator binary (shared)*).
- `NONE` → launch exactly one, in the background, from the worktree base. A
  `PLANNOTATOR_ALREADY_RUNNING` result (exit 4) means the slot was taken meanwhile:
  treat it as `RUNNING`.

```bash
cd <worktree-base> && pwd && bash "$H" review                 # CODE=1, after the delta below
cd <worktree-base> && pwd && bash "$H" annotate <chosen.md>   # markdown only
```

- Run from the base (not a repo subdir), so `review` aggregates every nested repo
  (plannotator ≥ 0.21 discovers them) and `PLAN.md` resolves. Verify `pwd` first.
- `annotate` takes exactly one `.md` path, relative to the base (`PLAN.md`, or
  `<repo>/docs/…/report.md`). Remember it — the stale-`annotate` rule needs it.
- `review` shows the **uncommitted** diff only, so always review before committing.
- A completion or `failed` notification usually means the user closed the tab, not that
  the cwd was wrong. Do not relaunch.
- When it returns, address each annotation (keyed `<repo>/<path>` in a multi-repo
  `review`) and re-run if needed. No annotations = approved.
- When it finishes, the next step is `finish` or more work — see SKILL.md → *Never
  suggest cleanup while work remains*.

## Changes since the last review (MUST FOLLOW)

Every code `review` (not `annotate`, not the plan gate) explains what changed since the
previous one — after annotation fixes, local code-review fixes, CI or PR-review fixes —
as text, never as a diff, so a re-review starts from the reasons. Right before the launch:

```bash
S=<sadensmol-task-skill-dir>/scripts/review-snapshot.sh
cd <worktree-base> && bash "$S" delta
```

- `delta` compares every repo with what the last `review` showed (commits in between do
  not matter), writes `<base>/.scratch/review-delta.patch` and prints a `--stat`.
- `NO_PREVIOUS` → first review: no guide, no notice.
- `NO_DELTA` → say "No changes since the last review."
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
  JSON, never drop a file to make it pass.
- `save` right after (`bash "$S" save`), on **every** launch, the first included: it
  records what this review shows.
- Post in chat, before the launch, a short text summary (one line per section: its
  title and reason) and this notice:

  > Changes since the last review: open **Guided Review** → *Changes since the last
  > review* in plannotator.

  When `delta` printed `LAST_CHANGE=yes`, add: "The **Last change** view in the diff-type
  dropdown shows the same code." With `LAST_CHANGE=no`, do not mention it: that view
  shows each repo's last commit, which is not the delta here.
- Plannotator opens the full diff as usual; the guide links into it.
