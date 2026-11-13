---
name: sadensmol-plan
description: "Use when a plan document is written, revised or reviewed: a task's PLAN.md (`task plan`, the planner subagent, a plan-gate rework), any plan .md the user asks for outside a task, a story to split into tracker tickets with assignees and estimates, or plannotator annotations on a plan that must be reworked. Also use when the user says 'make a plan', 'plan this', 'rework the plan', comments on a plan item or answers a plan's question to clarify."
---

# Plan documents

Every plan is one Markdown file with a fixed structure. The user reads it in plannotator,
comments on any item, and the planner reworks it. This skill owns the **content and
structure** of that file. The task flow (`sadensmol-task` → `references/plan.md`) owns
**when** the plan is written, gated and approved.

## Where the plan lives

| Situation | File |
|---|---|
| Inside a task | `<worktree-base>/PLAN.md` — at the base, never inside a repo, never committed |
| Outside a task | `.scratch/plans/<YYYY-MM-DD>-<slug>.md` at the session base (`sadensmol-router` → scratch dir), or the path the user names |

The plan is always `.md`. Never write the plan as HTML, and never show it as an HTML page
or a page companion.

## Structure (MUST FOLLOW)

Copy `references/template.md` and fill it. The `##` sections, in this order:

1. `## Task` — what the task is and why, the problem facts, every source, done-when,
   and `### Not requested`.
2. `## Questions to clarify` — every question the planning could not settle, each with
   the answer the plan assumes for now. The user reviews and answers them here.
3. `## Now` — how it works now, what is wrong, whether the problem is proved, and what
   must change. Diagrams first, prose second.
4. `## R&D` — real measurements (platform and test tools), options with a verdict,
   best practices, risks, related ideas.
5. `## Plan` — the exact path: repos and files, numbered steps, the story split (story
   planning only), rollout.
6. `## Testing` — how the change is proved: tests for the new behaviour and where they
   go, regression, backward compatibility, edge cases, the project's done-checks.
7. `## Docs` — the last point of planning: a short hint list of the docs the change
   likely touches. The docs are found and updated once, from the final diff, after the
   last code change and before the merge (`sadensmol-task` → `references/finish.md` →
   §6a).
8. `## Q&A` — every answered question of the planning phase (a log).
9. `## Decisions` — every decision of the planning phase, one log: what the user asked,
   what was decided and what was done according to it, and what the model or the pair
   decided alone.

No table of contents and no other content written twice: plannotator shows its own
contents panel from the headings. Each fact lives in one place; elsewhere, refer to it by
its id.

Keep the `##` heading text exactly as above. `pair.py` finds `## Q&A` and
`## Decisions` by exact text and appends to them; a renamed or numbered heading
makes it add a duplicate section at the end. Never put a `## ` line inside a section.

Read `references/template.md` before you write or rework a plan. It has every
sub-section and the rules for each one.

## Item ids — the user comments on items (MUST FOLLOW)

Every item the user may comment on carries a stable id in its heading or first cell:
`F1` problem fact, `I1` source (input), `C1` change, `M1` measurement, `O1` option, `K1` risk, `S1` plan
step, `T1` story ticket, `V1` test, `Q1` question, `U1` decision from the user's words,
`D1` model decision. `pair.py` numbers `Q<n>` and `D<n>`; the planner numbers `U<n>` and
its own `D<n>` (the next free number in the whole file). A pair decision keeps its
pair-log heading (`t<n> <status>: …`, `main: …`).

- Never renumber. A new item takes the next free id. A dropped item stays as one line:
  `~~S4 — <title>~~ dropped: <reason> (see Decisions)`.
- A reference to an item uses its id, so a comment stays valid after a rework.

## Item links — every reference is a link (MUST FOLLOW)

The user clicks a reference to jump to the item. Write it as the id in brackets:
`[S3] needs [O2]`, `([U14])`, `[S1]–[S9]`. Never write the anchor by hand.
`<plan skill dir>/scripts/plan-links.py` makes it a link to the item's plannotator heading
id: `[S3](#s3-schema-check-that-forces-the-rule)`.

- An id is defined by a heading that starts with it, the first cell of a table row, or a
  struck line `~~S4 — …~~`. A heading item links to its heading. A table item (`F`, `I`,
  `C`, `M`, `O`, `K`, `T`, `V` rows) links to its section heading: only headings carry an
  id. Never bracket the id in its own definition.
- A bare id stays text and is never linked. `S3` can be AWS S3. Bracket only plan items:
  not code, paths, URLs, mermaid, `(e)`-style letters or pair threads (`t6`).
- After every write of the plan: `plan-links.py <plan> fix`, then `plan-links.py <plan>
  check`. `fix` links each `[ID]` and re-targets each `[ID](#…)` after a heading rename.
  `check` is read-only and exits 1 when a reference needs `fix`, has no definition or two,
  or when a `(#anchor)` is dead. A failed `check` blocks the gate.
- `fix` and `check` skip headings, fenced code, code spans, URLs, other links, escaped
  brackets and the user's words (`- asked:`, `- question:`, `- answer:` and their indented
  continuation lines).
- A new log entry gets its links when it is written: pipe its text through `plan-links.py
  <plan> resolve` (stdin → stdout, read-only). Old entries keep their bare ids.
- Plannotator 0.28.3 itself shows no preview and drops a link with a title; a click
  scrolls to the item. In an agterm overlay opened through `plannotator-cookie-proxy`, a
  hover or keyboard focus shows the item (a table item: its row only), and agterm Back
  returns to the scroll position. Elsewhere (a browser, or when the proxy does not start)
  promise no preview and no Back. A link in another file (an export, a review page) does
  not reach the plan's headings.

## Write simply (MUST FOLLOW)

The reader must understand what happens and why without reading code. Explain
everything in plain words. Technical detail is a reference for the implementer, not part
of the explanation.

- Each `##` section opens with one `> **In short:**` line, max 25 words, in plain words:
  no code names, paths or commands.
- Short sentences, active voice, common words. One idea per bullet.
- Name things by what they do, not by their code names: "the bet is saved twice", not
  "`SaveBet` runs in `betTx` and `retryTx`". A technical term the reader needs → explain
  it in a few words where it first appears.
- Keep paths, symbols, commands and queries out of explanation sentences. They go only in
  reference places: the `Source` / `Where` / `Command` cells and a step's `where:` and
  `check:` lines.
- Leave out detail the reader does not need to follow the plan or decide on it. The
  implementer finds the rest in the code.
- Diagram labels are plain words (`Save the bet`), never function or table names.
- A table beats a paragraph when items share attributes. A diagram beats a paragraph for a
  flow, a call sequence or a lifecycle: `flowchart` for business logic, `sequenceDiagram`
  for calls between services, `stateDiagram-v2` for a status lifecycle.
- Add a code excerpt only when words and a diagram cannot show the point. It is at most
  15 lines, with `repo/path:line` above it, and shows only the lines the point needs.
- Every fact traces to a source: a `repo/path:line`, a ticket field, a log line, a query
  result, a message. No source → find it. Still no source → a question in
  `## Questions to clarify`, not a fact.
- Before the plan goes to plannotator, a hand-over or a commit, validate every mermaid
  block (`sadensmol-task` → `references/mermaid.md`). No `%%{init}%%` theme line.

## The logs — auto-updated during planning (MUST FOLLOW)

`## Q&A` and `## Decisions` are logs. They grow while the plan is discussed, and they
are never edited by hand, except as below.

| Log | What goes in | Who writes it |
|---|---|---|
| `## Q&A` | every answered question: user → agent, agent → user (with the user's answer), and in pair mode the writer, reviewer and merged answers | `pair.py qa` (`sadensmol-task` → *Q&A log*); the planner never writes here — its open questions go to `## Questions to clarify` |
| `## Decisions` | every decision, in the order made. `U<n>`: the user started it (an answer, a gate annotation, an approved suggestion, a settled dispute). `D<n>`: a normal or architectural choice no ticket or user settled (the planner's, the main session's). `t<n>` / `main:`: a pair decision | the planner for `U<n>` (on the rework that applies it) and its own `D<n>`; `pair.py model-decided` / the pair log for the rest |

Every entry says what was decided and what was done according to it:

```markdown
### U<n> — <topic> (<YYYY-MM-DD>)

- asked: <YYYY-MM-DD> (<chat | annotation on <id> | ask-user, answer to Q<n>>): “<the user's words, verbatim>”
- decided: <first person: "I decided to …"; "We decided to …" when the pair settled it>
- done: <what was done according to it>
- where: <the plan ids or file:symbol it affects, and the D/t entries with the detail>

### D<n> — <topic> (<YYYY-MM-DD>, <PLAN|IMPL>, <by>)

- decided: <first person: "I decided to …">
- done: <what was done according to it>
- why: <the reason, with ids or `path:line`>
- rejected: <the alternative not taken, when there was one>
- where: <the plan ids or file:symbol it affects>
```

`done:` names only work actually done, with its ids: on PLAN the plan items written or
changed ("Updated S5 to require the O2 proof; no implementation recorded"), on IMPL the
file:symbol, ticket or command. A planned step is not done work. Nothing done yet →
`not done yet: <what waits>`. A past outcome without a source → `not recorded`.

- Keep every entry verbatim. Never rewrite, merge, reorder or drop one. A decision that
  replaces an earlier one is a new entry that names it ("replaces U12").
- The one exception is `done:`. On the rework that carries a decision out (any entry,
  pair entries too), the planner updates its `done:` line to name the change.
- The second exception is a link target. The planner's `plan-links.py fix` may change the
  `(#…)` after an existing `[ID]` in an entry, when the item's heading moved or was
  renamed. Words and ids stay byte-identical; it adds and removes no link.
- An entry from before this format (a `U<n>` with only the user's words, a `D<n>` with
  `what:`, a pair entry in prose) gets its missing `decided:` / `done:` / `where:` lines
  on the next rework; `what:` and pair prose count as `decided:`. Its other lines stay
  byte-identical.
- Outside a task (no `pair.py` base), the main session appends the entries itself in the
  same formats: `### Q<n> — <topic> (<YYYY-MM-DD>)` with the `asked by` / `question` /
  `answer` lines, and the `U<n>` / `D<n>` entries above (`D<n>` without the stage).
- An empty log holds the single line `none yet`.

## Questions to clarify — finish the plan, ask later (MUST FOLLOW)

**Planning never stops for a question.** The planner finishes the whole plan in one
autonomous run, also when facts are missing or a choice is not its to make. Each such gap
becomes one entry in `## Questions to clarify`, and the plan is written on the answer that
entry assumes. The user reviews the questions together with the plan and answers them
after the plan is ready, one at a time; the plan is then reworked on the answers.

Which gaps become a question:

1. Look for the missing fact in every source first (the ticket, its comments and
   attachments, the code, the docs, the logs). Found → it is a fact, not a question.
2. A choice that is not critical (`sadensmol-task` → *Autonomy*) → decide it yourself and
   log a `D<n>`. It is not a question.
3. Everything else is a question: a fact still unknown that changes what is built or how
   it is accepted, or a critical choice (scope beyond the ticket, a production, data,
   money, security or compliance trade-off, a reversal of a user decision).

```markdown
### Q<n> — <topic> (<YYYY-MM-DD>, PLAN)

- question: <one concrete question the user can answer in one line>
- options: <O-ids or short options, each with its effect on the plan>
- recommendation: <the option you advise> — <the reason, with ids or `path:line`>
- assumed: <the option this plan is written on — normally the recommendation>
- affects: <the plan ids that change when the answer differs from `assumed`>
- checked: <the sources read, and any access blocker>
- answer: <empty — the user's answer goes here>
```

- `Q<n>` takes the next free number in the whole file (`pair.py` counts these headings
  too). Never write a bare `answer: open` or a question without `recommendation:` and
  `assumed:`.
- Every plan item that rests on an assumption names it: `(assumes Q<n>)` in its line.
- Order the questions by impact: the one that changes the most plan items first.
- An answered question becomes one line, `~~Q<n> — <topic>~~ answered: see Q<m>, U<k>`.
  The answer is logged in `## Q&A` (`resolves: Q<n>`) and applied as a `U<n>` decision
  (*Rework on comments*). An answer equal to `assumed:` changes no plan item; the `U<n>`
  says so (`done: none — the plan already assumed it`).
- No questions → the section holds the single line `none`.
- Name every question in the reply to the main session.
- The planner's recommendation is not the pair's view and not the user's answer.

**Who asks.** The planner cannot ask the user. The main session handles the questions
after the plan is written (`sadensmol-task` → `references/plan.md`, step 6; outside a task,
the same way): first it settles every question it can (a source found, or a noncritical
choice), then it opens the plan with the rest for the user's review, then it asks each
question still open **one at a time**, waits for the answer, and gives all the answers to
the planner for one rework.

## Rework on comments (MUST FOLLOW)

The user comments in plannotator (gate or plain `annotate`) or in chat. Each comment is
handled on its own, by its id:

1. **A question** → answer it from the code, the sources or the plan. Log it in `## Q&A`.
   It changes no plan item unless the user then asks for the change.
   **An answer to a `Q<n>` of `## Questions to clarify`** (a comment on it, or its
   `answer:` line filled in) is a change: handle it as step 2, and close the question
   (*Questions to clarify*).
2. **A change** → a user decision. The planner appends a `U<n>` entry to `## Decisions`
   (the user's words, what it decided from them and what it changed), then changes
   **every** part of the plan that carries the old choice (Task, Now, R&D, Plan), not
   only the commented line.
3. **Unclear** → when its readings lead to different deliverables, ask one short
   question. Otherwise take the reading that fits the ticket and the plan, log it as a
   `D<n>` with that reading, and name it in the rework summary.

After the rework: update the `Updated:` line, re-validate mermaid,
and re-open plannotator through the task flow (`sadensmol-task` → `references/plan.md`,
step 6: a change summary in chat, one line per comment, then the gate). Outside a task, open
it with the same helper:

```bash
cd <session-base> && bash <sadensmol-task-skill-dir>/scripts/plannotator-session.sh annotate <plan.md> --gate
```

Run it in the background (`sadensmol-task` → `references/plannotator.md`).

## Who writes the plan

The plan is written and reworked by the `planner-subagent` (`sadensmol-router` →
`references/planning.md`). Its brief names this skill, the plan path, the sources, the
user's decisions verbatim and the comments to apply. The main session never edits the
plan, except the log appends of `pair.py`.

## Common mistakes

| Mistake | Fix |
|---|---|
| A wall of prose in `## Now` | A diagram, a before/after table, one short excerpt |
| An explanation full of function names, paths and jargon | Plain words; technical detail only in the `Source` / `Where` cells and `where:` / `check:` lines |
| "The bug is X" with no evidence | Mark the problem `PROVED`, `NOT REPRODUCED` or `NOT A PROBLEM`, with the evidence |
| A plan step without a file or a check | Every `S<n>` names `where` and `check` |
| Story tickets without assignee or estimate | Take both from the project tracker skill's rules; unknown → ask |
| A user question turned into a plan step | It is a `## Q&A` entry until the user asks for the change |
| Planning stopped to ask the user | Finish the plan; the gap is a `## Questions to clarify` entry with `assumed:` |
| A question with no `assumed:` answer, or plan items that hide the assumption | Write the plan on one answer and tag each item `(assumes Q<n>)` |
| A noncritical choice left as a question | Decide it, log a `D<n>` |
| Renamed `## Q&A` / `## Decisions` heading | Keep the exact text; `pair.py` matches it |
| A decision entry without `decided:` or `done:` | Add what was decided and what was done according to it (`not done yet: …` when nothing is) |
| Renumbered items after a rework | Ids are stable; drop with strike-through |
| A bare `(U14)` or a hand-written `(#u14-…)` anchor | Write `[U14]`, run `plan-links.py fix`, then `check` |
