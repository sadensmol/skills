# `reviewd` — handled by the dispatcher

**Triggers:** "task reviewd", "review decisions", "task review decisions", "show the
model decisions".

`reviewd` opens plannotator `annotate` on every decision logged in this task, with what
was decided and what was done: the model's and the pair's own (`SKILL.md` → *Model
decisions log*), and on PLAN what was decided from the user's words (the `U<n>` entries
of `PLAN.md` → `## Decisions`), so the user can discuss, keep, change or revert each
one. `.task-user-decisions.md` holds only the user's words and is not shown.
It runs in any stage, and it writes no code (MUST FOLLOW).

## 1. Collect the decisions

Derive `base` and `branch` as in `references/worktree-helpers.md` (`branch` is the base's
basename). The sources, in this order:

- `PLAN.md` → the `### ` entries of its `## Decisions` section, up to the next `## `
  heading: the `U<n>`, `D<n>` and pair entries logged on PLAN, in both modes (`SKILL.md`
  → *Q&A log*). Skip a `U<n>` entry without a `decided:` line: it holds only the user's
  words. A `PLAN.md` written before the one-section format also has a
  `## Model decisions` section: read its `### ` entries too.
- `.scratch/plan-log-pending.json` → the `entry` of each `"kind": "decision"` item: a
  PLAN-stage decision still held for `PLAN.md`.
- `.task-decisions.md` → every `### ` entry above its `## Reviews` section (single mode).
- `.pair/decisions.md` → every `## ` entry (pair mode). Skip an entry whose heading text
  is already shown from `PLAN.md` or the held entries: on PLAN `pair.py` shows it there too.

```bash
for f in PLAN.md .scratch/plan-log-pending.json .task-decisions.md .pair/decisions.md; do
  [ -f "$base/$f" ] && echo "SOURCE=$f"
done
```

No entry in any source → say "No decisions are logged for this task." and stop.

## 2. Write the review file

`mkdir -p "$base/.scratch"`, then write `<base>/.scratch/decisions-review.md`, new on
every run:

```markdown
# Decisions — <branch>

Annotate an entry to ask about it, change it or revert it. An entry without an
annotation is kept.

## From PLAN.md
### <entry heading>
<entry body>
- last review: <its latest line from `## Reviews`, if any>

## During the work
<the same for each entry of .task-decisions.md or .pair/decisions.md>
```

- Every entry gets a `###` heading; keep the heading text and the body verbatim. Never
  shorten, merge or reword them.
- Put this line under the page title: "This export does not validate plan-reference
  targets; use PLAN.md for reliable item navigation." Item links (`[S3](#…)`) stay as
  they are: they jump only to an entry shown on this page.
- Leave out a `## From …` / `## During …` section with no entries.
- Read `references/mermaid.md` first when an entry has a mermaid block.

## 3. Open it

Follow `references/plannotator.md`: check the slot with `--status`, then
launch once, in the background, from the base:

```bash
cd <worktree-base> && bash <sadensmol-task-skill-dir>/scripts/plannotator-session.sh annotate .scratch/decisions-review.md
```

A plain `annotate`, never `--gate` and never `review`. An annotation belongs to the
`###` entry it sits under. The file follows `references/plannotator.md` → *Theme*.

## 4. Act on the annotations

Quote each annotation before its answer (SKILL.md → *Quote the question in every
answer*). For each annotated entry:

- **A question** → answer from the code (`file:line`), or from `PLAN.md` when the
  decision is not built yet. Change nothing.
- **Change or revert** → a user decision (SKILL.md → *User decisions log*). In pair mode
  the user's request goes through the decision loop first (`pair.md`). The user's
  replacement is not logged as a model decision.
  - **On PLAN:** one planner run (`references/plan.md`, step 5) appends a `U<n>` entry
    to `PLAN.md` → `## Decisions` (the user's words verbatim, what it decided and what it
    changed), and changes every part of `PLAN.md` that
    carries the old choice. When the choice is already in the code, a fixer follows, as
    on IMPL.
  - **On IMPL:** `PLAN.md` stays frozen, and no planner runs. Log the user's words with
    `pair.py user-decided`. A fixer gets the entry and the words in its brief, followed
    by the review checkpoint (SKILL.md → *Review checkpoint*).
- **Unclear** → ask one short question. Do not guess.

When every annotation has its outcome, and before the change starts, the main session
appends one line per shown entry to `<base>/.task-decisions.md` under `## Reviews`
(created at the end of the file when absent):

```markdown
- <YYYY-MM-DD> — <entry heading> — kept | answered | changed: <what> | reverted
```

An entry without an annotation gets `kept`. `reverted` = undone with no replacement;
undone and replaced = `changed: <the replacement>`. A question that led to a change gets
the change. The next step is the change, more work or `finish`.
