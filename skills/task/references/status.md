# `status` / `list` / `switch` — handled by the dispatcher

`list` and `switch` work on the worktrees under the workspace root `$root` (`references/worktree-helpers.md` →
*Workspace root*) and run from anywhere. `status` reports on the task this session is in, and
uses `$root` for `--all`.

## `status`

**Triggers:** "task status", "status of the task", "where am I", "what's left", "what's
blocking finish". `--all` → the fleet table at the end.

`status` answers where this task is: its phase, what its PRs and CI do, and what still
blocks `finish`. It is a read: it never commits, pushes, moves a ticket or opens a PR.

**It renders as an HTML page companion and a `.md` file, never into chat**
(step 6).

### 1. Resolve the task

Use `$WT`/`$branch` from `references/worktree-helpers.md`. No worktree → say so and offer
`--all`; never guess a task.

### 2. Show a passive panel while gathering

```bash
agtermctl session hud "collecting task status…" --spinner --position bottom-right \
  --target "$AGTERM_SESSION_ID" >/dev/null 2>&1 || true
# … gather …
agtermctl session hud close --target "$AGTERM_SESSION_ID" >/dev/null 2>&1 || true
```

### 3. Gather — every signal guarded; a missing tool is never an error

**Repos.** A git worktree has `.git` as a FILE, so never test `-d .git`:

```bash
for repo in "$WT" "$WT"/*/; do
  git -C "$repo" rev-parse --show-toplevel >/dev/null 2>&1 || continue
  git -C "$repo" status --porcelain | wc -l                                      # dirty files
  git -C "$repo" rev-list --left-right --count '@{upstream}...HEAD' 2>/dev/null  # behind<TAB>ahead
done
```

A failing `@{upstream}` means never pushed — a finish blocker, not an error.

**Ticket id.** Take the id the workspace's `new` wrote into the session context; fall back
to the branch's leading `<letters>-<digits>`:

```bash
ctx="$(agtermctl tree --json 2>/dev/null | python3 -c '
import sys, json, os
sid = os.environ.get("AGTERM_SESSION_ID", "")
d = json.load(sys.stdin)
for w in d.get("result", {}).get("tree", {}).get("workspaces", []):
    for s in w.get("sessions", []):
        if s.get("id", "").lower() == sid.lower():
            print("\t".join([s.get("context", ""), s.get("status", "idle"),
                             s.get("foreground", ""), s.get("splitForeground", "")]))
' 2>/dev/null || true)"
ticket="$(printf '%s' "$ctx" | cut -f1)"
[ -n "$ticket" ] || ticket="$(printf '%s' "$branch" | grep -oiE '^[a-z]+-[0-9]+' | tr '[:lower:]' '[:upper:]')"
```

Report the id only. Tracker state belongs to the workspace skill — this file never learns
status names, never builds a tracker link and never hardcodes a tracker host. Take the
URL from the tracker tool's issue response (`url`), or from the workspace skill's
issue-URL template. Link the id when you have a URL; never stall the report on a
tracker call.

**PRs and CI**, per repo (`gh` may be absent; no PR is not a failure):

```bash
command -v gh >/dev/null 2>&1 &&
  ( cd "$repo" && gh pr view "$branch" \
      --json number,url,state,isDraft,mergeable,reviewDecision,statusCheckRollup 2>/dev/null )
```

Keep `url` in the field list (the PR cell is a link). Name the failing job from
`statusCheckRollup`; never just "checks failing".

**What runs now.** `foreground`/`splitForeground` above are each pane's live argv. A
sibling session whose `cwd` is under `$WT` is a console this task opened (an infra
stack, a long run) — list those too.

### 4. Derive the state from evidence

| state | evidence |
|---|---|
| **planning** | a plan doc at the base and no commits yet, or plannotator/`plan` in the foreground |
| **implementation** | commits or a dirty tree, and no PR in any changed repo |
| **testing (babysitting)** | a test/build command in the foreground, a live sibling console under `$WT`, or pending PR checks |
| **code review** | an open PR with `reviewDecision` empty or `REVIEW_REQUIRED`, or plannotator `review` in the foreground |
| **finishing** | every changed repo has a PR, checks green, nothing merged |
| **blocked** | failing checks, `CHANGES_REQUESTED`, or `mergeable: CONFLICTING` |
| **done** | every PR merged, every tree clean, nothing ahead of upstream |

Print the state and its evidence on one line ("code review — PR #1501 open, no review
yet"). When two states have evidence, report the later one and list the earlier one's
leftovers as blockers.

### 5. The `finish` gate — one line per blocker

- uncommitted changes in `<repo>` (N files)
- N commits not pushed in `<repo>` — or no upstream at all
- `<repo>` has changes on this branch and no PR
- PR #N is a draft
- PR #N checks: `<job>` failing / N pending
- PR #N `CHANGES_REQUESTED`
- PR #N `CONFLICTING` — needs a rebase
- PR #N open and unmerged
- ticket `<id>` still needs its tracker move — the workspace skill owns it; name the id
  and the owner, never invent a status name

An empty list means `finish` would go through. Deferred review findings that should
become follow-up tickets are invisible to git and `gh`, so end with the question "any
deferred review findings still to file as follow-ups?".

### 6. Render — a `.md` report plus an HTML page companion (MUST FOLLOW)

The report is ALWAYS two files and ONE page companion — never a terminal renderer, never
the report in chat. Write both under the session's scratch dir (`sadensmol-router` →
scratch dir; inside a task, `$WT/.scratch/`):

- **`task-status.md`** — the report itself, the file the user keeps: a title line (the
  ticket id linked when you have its URL, then the branch), the state line, a repos
  table (`repo | dirty | ahead | behind | PR | checks`), the blockers list, what runs
  now, and the closing deferred-findings question.
- **`task-status.html`** — the same content as a page, built from the same facts (never
  a second, diverging gather).

**Diagrams when they carry information** — a mermaid block in the `.md` (and the same
diagram in the page) only when there is a structure to show:

| Show | When |
|---|---|
| `flowchart LR` of the PR chain, each node `repo #N state` | stacked PRs, or a merge order across repos |
| `flowchart LR` of the phases, the current one highlighted | the state is not obvious from one line (two states had evidence) |

One repo with one PR, or no PRs → no diagram. Validate every block with
`references/mermaid.md` before you open the page.

**The page:**

- Self-contained HTML: inline CSS, `<table>`s, the state word bold and coloured; ticket
  id, PR numbers and a blocker naming one of them are `<a href>` links; nothing else is
  a link. Escape every value that came from git, `gh` or the tracker.
- Colours from the plannotator theme (`references/plannotator.md` → *Theme*): background,
  text and borders from its palette (`--bg`, `--fg`, `--line`); red = `--partial`, yellow =
  `--mark`, plus a fixed green and cyan that read well on that background. No ANSI or
  `--agterm-*` slots. green = done / green checks / in sync; red = blocked, failing job,
  `CHANGES_REQUESTED`, `CONFLICTING`; yellow = code review, pending, draft; cyan =
  testing, a live command. Never encode a state in colour alone — print the word.
- Mermaid renders in the page from the LOCAL bundle, never a CDN: copy it next to the page
  and load it relatively:

  ```bash
  mjs="$(npm root -g 2>/dev/null)/@mermaid-js/mermaid-cli/node_modules/mermaid/dist/mermaid.min.js"
  [ -f "$mjs" ] && cp "$mjs" "$WT/.scratch/mermaid.min.js"
  ```

  `<pre class="mermaid">…</pre>` plus `<script src="mermaid.min.js"></script><script>mermaid.initialize({startOnLoad:true, theme:"dark"})</script>`.
  No bundle → leave the diagram out of the page and say so in the chat line (the `.md`
  keeps it).

**Open it — the page is the deliverable:**

```bash
agtermctl session overlay open --html "$WT/.scratch/task-status.html" --companion --cwd "$WT/.scratch" --js \
  --target "$AGTERM_SESSION_ID" --socket "$AGTERM_SOCKET"
```

- `--target "$AGTERM_SESSION_ID"` is required (the default is whatever session the user
  selected). `--cwd` lets the page load `mermaid.min.js`; `--js` runs it. No `--follow`
  (*Never move the user's focus*). Save the printed id in `$WT/.scratch/task-status.page`.
- A status page from this task is still open (the saved id is in `tree --json`) →
  `session overlay reload --target <saved id>`, never a second page.
- Outside agterm (`AGTERM_ENABLED` unset) → `open "$WT/.scratch/task-status.html"`.
- **Then say ONE line in chat, never the report again and never a file path:** "Status
  is up in the companion page — implementation, 3 repos dirty, no PRs yet." Answer a follow-up about one row with that row only.

The `.md` for the example task:

```markdown
# [ABC-123](<tracker url>) · be-add-health-checks

**state:** code review — PR #1501 open, no review yet

| repo      | dirty | ahead | behind | PR                 | checks      |
|-----------|------:|------:|-------:|--------------------|-------------|
| api       |     3 |     2 |      0 | [#1501](<pr url>)  | 2✓ 1✗ lint  |
| e2e-tests |     0 |     1 |      0 | —                  | —           |

## Blockers
- ✗ api: 3 uncommitted files
- ✗ api: PR [#1501](<pr url>) check `lint` failing
- ✗ e2e-tests: 1 commit on this branch and no PR

Any deferred review findings still to file as follow-ups?
```

### `status --all`

One row per worktree under `$root/.worktrees/`, most recently modified first, rendered
like step 6 (`task-status-all.md` + `task-status-all.html` in the scratch dir, one page companion):

```
branch                          session              git
abc-123-be-add-health-checks    ABC-123 · active     2 ahead · 3 dirty
abc-98-fix-settings-cache       ABC-98 · idle        clean · in sync
abc-75-docs                     — no session         5 ahead · no upstream
```

Match worktree to session by **cwd prefix**, not name. Call out `— no session` and
dirty/ahead (work that exists nowhere else). A session whose cwd is under `.worktrees/`
but whose directory is gone is an **orphan session** — list those separately as
sessions to close.

## `list`

**Triggers:** "task list", "list tasks", "show worktrees".

```bash
ls -1d "$root"/.worktrees/*/ 2>/dev/null | xargs -I{} basename {}
```

A numbered list of bare names; none → "No worktrees found." State questions are
`status`.

## `switch`

**Triggers:** "task switch", "switch task", "task switch --pair", "task switch
--single", "switch to <task> in pair mode".

**It works on worktrees (MUST FOLLOW).** The choice is an existing worktree. A live session
is never a condition: after a reboot or a closed session there is none, and `switch` opens
it. Never delegate to `new`: on an existing worktree `new` stashes, fetches and rebases every
repo. Do not create worktrees. Do not run Git commands, take snapshots, fetch, pull, rebase,
stash, refresh dependencies or inspect task status.

1. Resolve `$root` through `references/worktree-helpers.md` and list its existing worktrees.
   None → "No worktrees found." and stop.
2. Use the worktree the user names when it matches exactly one existing directory.
   Otherwise offer the four most recently modified through `ask-user`, with custom input
   for another worktree. Without `ask-user`, show a numbered list. Never pick silently.
3. Outside agterm, use the selected worktree as the working directory for subsequent
   commands and stop.
4. Read `agtermctl tree --json` and locate the task session by its `cwd` under the selected
   worktree. Use path boundaries, not a bare name prefix; exclude test, console, and review
   companion sessions (every node with a `parent` field). Ask only if several task sessions match.
5. No matching session, or `--pair` / `--single` given → run the workspace's script, which
   opens or converts the session and leaves every repo as it is. It keeps the mode the
   worktree recorded:

   ```bash
   bash <workspace-skill-dir>/scripts/new-task.sh <branch> --open [--pair | --single]
   ```

   Then read `agtermctl tree --json` again for the session id.
6. Select the session with `agtermctl session select --target <session-id>`. Pass
   `--socket "$AGTERM_SOCKET"` when set. This explicit switch request permits moving the
   user's focus. Confirm the selected session through `tree --json`, then stop.

**Mode.** Plain `switch` asks no single/pair question and changes no mode or stage record.
`--pair` / `--single` follow `references/pair-mode.md` → *Changing a task's mode*.
