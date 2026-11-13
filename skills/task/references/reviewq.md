# `reviewq` — handled by the dispatcher

**Triggers:** "task reviewq", "review questions", "task review questions".

`reviewq` opens plannotator `annotate` on an HTML page with every question of this task
and its answers, so the user can comment, discuss and send updates back. The Q&A log
(`SKILL.md` → *Q&A log*) holds every logged question of every session and stage, with
the writer, reviewer and merged answers in pair mode. The current session adds what is
not logged yet: a user prompt (typed, or sent while a turn ran), an `AskUserQuestion`
item, or a question the agent asked in its reply text.
It runs in any stage, and it writes no code (MUST FOLLOW).

## 0. PLAN stage — the plan's `.md`, never HTML (MUST FOLLOW)

On PLAN (`pair.py plan-open --base <worktree-base>` exits 0) the questions live in
`PLAN.md` → `## Q&A` (`sadensmol-plan`), so `reviewq` opens `PLAN.md` itself:

1. Log every question of this session that is not in `## Q&A` yet, with `pair.py qa`
   (`SKILL.md` → *Q&A log*).
2. Run `pair.py plan-log-flush --base <worktree-base>`.
3. Open a plain `annotate PLAN.md` (no `--gate`), as `references/review.md` does on PLAN.
   No `PLAN.md` yet → say "nothing to open" and stop.
4. Act on the annotations as in step 4 below.

Steps 1–3 below (the HTML page) run on IMPL only.

## 1. Collect the logs and the session

Derive `base` and `branch` as in `references/worktree-helpers.md`, and run
`mkdir -p "$base/.scratch"`. Collect the logs first:

```bash
python3 <sadensmol-task-skill-dir>/scripts/pair.py plan-log-flush --base <worktree-base>
for f in PLAN.md .task-qa.md; do [ -f "$base/$f" ] && echo "LOG=$f"; done
```

Each `LOG=` file is a `--log` of step 3: `PLAN.md` holds the PLAN-stage Q&A, `.task-qa.md`
the IMPL one. When the flush prints `"held"` above 0 (`PLAN.md` does not exist yet),
write the `entry` of each `"kind": "qa"` item of `.scratch/plan-log-pending.json` to
`<base>/.scratch/qa-held.md`, one after the other, and pass it as one more `--log`.

Then get the transcript through the harness capability
`session-transcript` (`harness/README.md`; your harness file says where it is):

- **A transcript file** → normalize it:

  ```bash
  python3 <sadensmol-task-skill-dir>/scripts/qa-export.py normalize --transcript <transcript> --out "$base/.scratch/questions-review.json"
  ```

  Read that JSON. `items` are in time order, on the active branch of the session only:
  `prompt` (a human prompt; `via` is `typed` or `queued`; `command` marks a slash
  command), `ask` (one `AskUserQuestion` item with its chosen `answer`) and `reply` (one
  agent message, split into `parts` at its `> ` quote lines). `complete: false` means
  the session chain is broken: the page is marked partial with `partial_reason`.
- **A blocker** (the harness file names it) → say it in one line and stop.
- **A partial snapshot only** → write the entries of step 2 from your own context. The
  page is labelled partial.

## 2. Associate the answers

Write `<base>/.scratch/questions-review-entries.json`, new on every run:

```json
{"entries": [
  {"speaker": "user",
   "question": {"source": "<item id>", "text": "<verbatim text>"},
   "answers": [{"source": "<item id>", "text": "<verbatim excerpt>"}]}
]}
```

- One entry for each human prompt, each `ask` item, and each question the agent asked
  in a reply (`speaker: agent`; the user's reply prompt is its answer and stays its own
  entry too). Put the entries in the order of their questions.
- Leave out a question that a log entry of step 1 already holds verbatim: the page shows
  the log entry, with its pair split. `{"entries": []}` is valid when the logs hold all.
- Leave out the prompt that started this `reviewq`, and a slash command whose arguments
  hold no question or request.
- Copy every text verbatim from its source. An excerpt is a cut, never a rewrite. Never
  shorten, merge or reword.
- A reply part's `candidates` are the earlier prompts that contain its quote. They are
  an aid, not the answer. Tie a part to an entry first by a unique full quote, then by
  what the part says. A part you cannot tie to one entry for sure goes in as an
  unassociated entry (`"question": null`). Never tie it to the prompt of the current
  turn by default.
- An entry with no answer gets the mark "No answer yet." on the page.

## 3. Render and open it

```bash
python3 <sadensmol-task-skill-dir>/scripts/qa-export.py render --transcript <transcript> --entries "$base/.scratch/questions-review-entries.json" --out "$base/.scratch/questions-review.html" --theme "$(bash <sadensmol-task-skill-dir>/scripts/plannotator-theme.sh)" --log "$base/PLAN.md" --log "$base/.task-qa.md"
```

Pass one `--log` per `LOG=` file of step 1 (and `qa-held.md`), and only those.

`--theme` is required: the page follows the plannotator theme (`references/plannotator.md`
→ *Theme*).

For a partial snapshot, pass `--partial "<reason>"` instead of `--transcript`. The render
fails when an excerpt is not found verbatim in its source: fix the entry, never the
check. The page is a snapshot, new on every run: one `<section id="lN">` per logged
entry first, then one `<section id="qN">` per session entry.

Follow `references/plannotator.md`: check the slot with `--status`, then launch once, in
the background, from the base:

```bash
cd <worktree-base> && bash <sadensmol-task-skill-dir>/scripts/plannotator-session.sh annotate .scratch/questions-review.html
```

A plain `annotate`: never `--gate`, never `review`, never `PLANNOTATOR_PROMOTE`. An
annotation belongs to the entry whose `lN` or `qN` is in its element context, else to the
one entry whose text holds its quote. Ambiguous → ask one short question.

## 4. Act on the annotations

Quote each annotation before its answer (SKILL.md → *Quote the question in every
answer*). For each annotated entry:

- **A question** → answer from the code (`file:line`), `PLAN.md` or the docs. Change
  nothing.
- **A change** → a user decision (SKILL.md → *User decisions log*), handled as in
  `references/reviewd.md` → *4. Act on the annotations*: the decision loop first in
  pair mode, then one planner run on PLAN, or `pair.py user-decided` and a fixer on IMPL.
- **Unclear** → ask one short question. Do not guess.

Closing with no annotation changes nothing. `reviewq` writes no review log. The next
step is the change, more work or `finish`.
