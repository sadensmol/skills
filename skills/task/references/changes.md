# `changes` — handled by the dispatcher

**Triggers:** "task changes", "show changes", "show me the changes", "show the diffs",
"changes".

Walk the user through **this task's business-logic diffs, in chat**, a screenful at a
time, so each fix can be approved, questioned or reworked on the spot. No plannotator,
no annotation round-trip — the exact patch, a static test-reference count, and a pause.

**Scope — this task's own changes only (MUST FOLLOW).** The branch's own commits plus
the uncommitted tree, inside the current worktree. Never diff a primary checkout, never
show the default branch's history, and never rebuild a diff from memory or `PLAN.md`.
Every hunk you print is verbatim output of a `git` command run in this session.

## Step 1 — each repo's base ref

Derive `$base` (SKILL.md → *Worktree helpers*). `.task-base` is the per-repo base
record the project's `new` writes (`references/new.md` → *`.task-base`*); `tb_diff_base`
gives the commit each repo's own changes start from — the forkPoint in a stacked repo,
else the merge-base with its default branch:

```bash
. <sadensmol-task-skill-dir>/scripts/task-base.sh
tb_migrate "$base"
for d in "$base"/*/; do
  [ -d "$d" ] || continue
  git -C "$d" rev-parse --git-dir >/dev/null 2>&1 || continue
  r="$(basename "${d%/}")"
  mb="$(tb_diff_base "$base" "$r" "$d")"
  echo "REPO=$r UPSTREAM=$(tb_get "$base" "$r" upstream) MERGE_BASE=$mb"
  git -C "$d" diff --numstat "$mb"                     # committed + staged + unstaged
  git -C "$d" status --porcelain -uall | grep '^??'    # untracked (new files)
done
```

- `git diff "$mb"` already covers committed + staged + unstaged — one diff, not three.
- Render an untracked file with `git -C "$d" diff --no-index -- /dev/null "<file>"`.
- Skip a repo with no changes silently. No changed repo at all → "no changes in this
  task", and stop.

## Step 2 — keep business logic, name the rest once

Noise (named once, never printed):

- **Tests** — `*_test.go`, `*_test.py`, `test_*.py`, `*.test.*`, `*.spec.*`,
  `*_test.dart`, `*Test.swift` / `*Tests.swift`, and any `test/`, `tests/`,
  `__tests__/`, `testdata/`, `fixtures/` path segment.
- **Generated / mocks** — `*.pb.go`, `*_gen.go`, `*.gen.*`, `mock_*.go`, `*_mock.go`,
  `mocks/`, `generated/`, `*.g.dart`, `*.freezed.dart`.
- **Dependency + lock churn** — `go.mod`, `go.sum`, `package-lock.json`,
  `pnpm-lock.yaml`, `yarn.lock`, `pubspec.lock`, `Podfile.lock`, `poetry.lock`,
  `uv.lock`, `Cargo.lock`.
- **Docs and junk** — `*.md` (incl. `PLAN.md`), `.DS_Store`, `local.properties`.

Everything else is business logic, including SQL migrations, config/values files and
API/schema definitions. Print the noise as one line (`Not shown: 4 test files, go.sum,
PLAN.md`), then never mention it again.

## Step 3 — batch by screen

- Up to **3 files** per batch while the batch's diff lines (added + removed) stay
  **≤ ~60**. A file over ~60 lines is shown alone. A file over ~150 lines is split by
  hunk across screens of ~80 lines, each followed by a pause.
- Order: repo by repo, files in `git` order.
- One-line header per batch: `Batch 2/5 — api: internal/wallet/service.go, internal/wallet/mapper.go`.

Each file: `#### <repo>/<path>` (plus `(new file)` / `(deleted)`), then the exact diff
in a `diff` fence, verbatim from `git` (never re-typed, summarised or trimmed beyond the
hunk split), then the `Tests: N` line.

**No commentary unless asked.** Do not explain, review, praise, or propose. Show and wait.

## Step 4 — the `Tests: N` line

Take the symbols the hunks touch (names in `@@ … @@` headers and added/removed
declarations) and count the test cases that reference them:

```bash
grep -rnw --include='*_test.go' --include='*_test.py' --include='test_*.py' \
  --include='*.test.ts' --include='*.test.tsx' --include='*.spec.ts' \
  --include='*_test.dart' --include='*Test.swift' --include='*Tests.swift' \
  "<Symbol>" "$base" | head -20
```

Report `Tests: N` (distinct test functions, this task's new ones included) and name up
to 3: `Tests: 3 (wallet_service_test.go: TestDebit_InsufficientFunds, …)`. No match →
`Tests: 0 — no test references this change`.

**It is a static reference count, not coverage (MUST FOLLOW).** Never call it coverage,
never give a percentage, and never run a test for it — `changes` runs no tests.

## Step 5 — the pause

After each batch, ask through `ask-user`: one `Discuss <file>` option per file in the
batch, plus `Next batch` (`Done` on the last batch) — 4 options at most.

- **Next / Done** → approved; show the next batch, or the closing summary.
- **Discuss `<file>`** → **the walkthrough stops here (MUST FOLLOW).** Switch to plain
  discussion of that one change: answer short and factual, traced to the diff and the
  code around it, and apply the fix the user asks for. Do not print the next batch, do
  not re-print remaining diffs, and do not ask "shall we continue?".
  - Reworks are allowed only here, only on the file the user named, exactly as asked.
    Never fix anything "while you are in there".
  - After a rework, re-print that file's diff and its refreshed `Tests:` line. Still no
    next batch.
  - Resume only on an explicit "continue" / "next" / `task changes`: re-derive the
    diffs and resume at the batch after the interrupted one; say which batch.

`changes` never commits, pushes, runs `finish`, or opens plannotator. Its only writes are
the reworks the user asks for while discussing.

## Closing summary

Only when the user answered `Done` on the last batch — never after a stop for
discussion. At most 5 lines: files approved, files reworked, files with an open
question. No next-step offers and no `cleanup` suggestion.
