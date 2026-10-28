# `reviewd` — handled by the dispatcher

**Triggers:** "task reviewd", "review decisions", "task review decisions", "show the
model decisions".

`reviewd` opens plannotator `annotate` on every decision the model made in this task
without the user (`SKILL.md` → *Model decisions log*), so the user can discuss, keep,
change or revert each one. User decisions (`PLAN.md` → `## Decisions`) are not shown.
It runs in any stage, and it writes no code (MUST FOLLOW).

## 1. Collect the decisions

Derive `base` and `branch` as in SKILL.md → *Worktree helpers* (`branch` is the base's
basename). The sources, in this order:

- `PLAN.md` → the `### ` entries of its `## Model decisions` section, up to the next
  `## ` heading.
- `.task-decisions.md` → every `### ` entry above its `## Reviews` section (single mode).
- `.pair/decisions.md` → every `## ` entry (pair mode).

```bash
for f in PLAN.md .task-decisions.md .pair/decisions.md; do
  [ -f "$base/$f" ] && echo "SOURCE=$f"
done
```

No entry in any source → say "No model decisions are logged for this task." and stop.

## 2. Write the review file

`mkdir -p "$base/.scratch"`, then write `<base>/.scratch/decisions-review.md`, new on
every run:

```markdown
# Model decisions — <branch>

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
- Leave out a `## From …` / `## During …` section with no entries.
- Read `references/mermaid.md` first when an entry has a mermaid block.

## 3. Open it

Follow SKILL.md → *Plannotator binary (shared)*: check the slot with `--status`, then
launch once, in the background, from the base:

```bash
cd <worktree-base> && bash <sadensmol-task-skill-dir>/scripts/plannotator-session.sh annotate .scratch/decisions-review.md
```

A plain `annotate`, never `--gate` and never `review`. An annotation belongs to the
`###` entry it sits under.

## 4. Act on the annotations

Quote each annotation before its answer (SKILL.md → *Quote the question in every
answer*). For each annotated entry:

- **A question** → answer from the code (`file:line`), or from `PLAN.md` when the
  decision is not built yet. Change nothing.
- **Change or revert** → a user decision. One planner run (`references/plan.md`, step 5)
  appends the user's words to `PLAN.md` → `## Decisions` verbatim (SKILL.md → *User
  decisions log*), and reworks every part of `PLAN.md` that carries the old choice. Then,
  when the choice is already in the code, a fixer gets the entry and the user's words in
  its brief, followed by the review checkpoint (SKILL.md → *Review checkpoint*). In pair
  mode the user's request goes through the decision loop first (`pair.md`). The user's
  replacement is not logged as a model decision.
- **Unclear** → ask one short question. Do not guess.

When every annotation has its outcome, and before the rework starts, the main session
appends one line per shown entry to `<base>/.task-decisions.md` under `## Reviews`
(created at the end of the file when absent):

```markdown
- <YYYY-MM-DD> — <entry heading> — kept | answered | changed: <what> | reverted
```

An entry without an annotation gets `kept`. `reverted` = undone with no replacement;
undone and replaced = `changed: <the replacement>`. A question that led to a change gets
the change. The next step is the rework, more work or `finish`.
