# `status` / `list` / `switch` — handled by the dispatcher

`list` and `switch` work on the worktrees under the project root `$root` (SKILL.md →
Step 2) and run from anywhere. `status` reports on the task this session is in, and
uses `$root` for `--all`.

## `status`

**Triggers:** "task status", "status of the task", "where am I", "what's left", "what's
blocking finish". `--all` → the fleet table at the end.

`status` answers where this task is: its phase, what its PRs and CI do, and what still
blocks `finish`. It is a read: it never commits, pushes, moves a ticket or opens a PR.

**It renders into a floating agterm overlay, not into chat.** Outside agterm
(`AGTERM_ENABLED` unset), print the same report in chat as a plain markdown table, no
escapes.

### 1. Resolve the task

Use `$WT`/`$branch` from SKILL.md → *Worktree helpers*. No worktree → say so and offer
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

**Ticket id.** Take the id the project's `new` wrote into the session context; fall back
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

Report the id only. Tracker state belongs to the project skill — this file never learns
status names, never builds a tracker link and never hardcodes a tracker host. Take the
URL from the tracker tool's issue response (`url`), or from the project skill's
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
- ticket `<id>` still needs its tracker move — the project skill owns it; name the id
  and the owner, never invent a status name

An empty list means `finish` would go through. Deferred review findings that should
become follow-up tickets are invisible to git and `gh`, so end with the question "any
deferred review findings still to file as follow-ups?".

### 6. Render — a floating, coloured, tabular overlay

Write the gathered facts to a small JSON file, then let the overlay run a renderer that
reads its own terminal size (the panel width is unknown until it exists):

```bash
agtermctl session overlay open \
  "zsh -lc 'python3 \"$render\" \"$data\" | less -R'" \
  --target "$AGTERM_SESSION_ID" --size-percent 75 --background-color "#11131a"
```

- `--size-percent 75`, never full-pane, no `surface zoom`, no `--full`: a full-pane
  overlay looks like output printed into the session.
- `--target "$AGTERM_SESSION_ID"` is required: the default is whatever session the
  user has selected. `zsh -lc` is there because `overlay open` runs with the GUI `PATH`.
  `less -R` passes escapes through, gives search and scroll, and quitting closes it.
- **Then say ONE line in chat, never the report again:** "Status is up in the overlay
  (`q` to dismiss) — implementation, 3 repos dirty, no PRs yet." Answer a follow-up
  about one row with that row only.

Renderer rules (python3 stdlib + SGR escapes; never depend on `gum`, `glow`, `bat`,
`rich`):

- **Width.** Piping into `less` makes `shutil.get_terminal_size()` fall back to 80, so
  read the tty:

  ```python
  def term_cols(default=100):
      for fd in (sys.stdout.fileno(), sys.stderr.fileno()):
          try:    return os.get_terminal_size(fd).columns
          except OSError: pass
      try:
          fd = os.open("/dev/tty", os.O_RDONLY)
          try:    return os.get_terminal_size(fd).columns
          finally: os.close(fd)
      except OSError: return default
  ```

  `WIDTH = max(48, cols - 4)`. Every rule spans it, and the name column absorbs the
  slack (`flex = WIDTH - (fixed columns + gaps)`). Truncate only against the real
  width — an ellipsis next to blank columns means you truncated to a guess.
- **Links: OSC 8, not bare URLs.** Link the ticket id, each PR cell, and a blocker line
  that names one of them — nothing else. The terminator is ESC + ONE backslash:

  ```python
  def link(url, label):
      return label if not url else "\033]8;;" + url + "\033\\" + label + "\033]8;;\033\\"
  OSC8 = re.compile("\033\\]8;;[^\033]*\033\\\\")
  def w(s): return len(SGR.sub("", OSC8.sub("", s)))   # measure without SGR and OSC 8
  ```

  `clip()` must strip both before slicing too.
- **No outer box** — agterm frames the panel. A bold title line, a dim rule, then
  sections.
- **Colour: the 16 ANSI colours** (they follow the user's theme); truecolor only as an
  accent; no colour when the stream is not a TTY. green = done / green checks / in sync;
  red = blocked, failing job, `CHANGES_REQUESTED`, `CONFLICTING`; yellow = code review,
  pending, draft; cyan = testing, a live foreground command; default + bold state word =
  implementation, planning; dim = repo and branch names, PR urls. Never encode a state
  in colour alone — print the word.
- **Three sections, plain columns, one rule each.** Compute every column width from the
  content (`max(len(header), max(len(cell)))`), never a hardcoded field width. One
  width for the whole board, rules included. Number columns hold digits only
  (annotations go beside the name, dim). Pad by display width (strip escapes first, or
  colour after padding; keep the glyph set narrow or measure with
  `unicodedata.east_asian_width`). Right-align numbers, left-align names.
  Truncate, never wrap. No `gh` → `—` in the `pr`/`checks` columns.

```
  ABC-123 · be-add-health-checks
  ────────────────────────────────────────────────────────────────────
  state   code review — PR #1501 open, no review yet

  repo              dirty  ahead  behind   pr      checks
  ────────────────────────────────────────────────────────────────────
  api                   3      2       0   #1501   2✓ 1✗ lint
  e2e-tests             0      1       0   —       —

  blockers
  ────────────────────────────────────────────────────────────────────
  ✗  api: 3 uncommitted files
  ✗  api: PR #1501 check `lint` failing
  ✗  e2e-tests: 1 commit on this branch and no PR
  ?  deferred review findings still to file as follow-ups?
```

### `status --all`

One row per worktree under `$root/.worktrees/`, most recently modified first, in chat:

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

1. List worktrees as in `list`. None → "No worktrees found." and stop.
2. Offer them through `ask-user` (max 4 — the 4 most recently modified, plus "Other").
   Without `ask-user`, a numbered list in chat. Never pick silently.
3. Delegate the open to the project skill so its wiring runs (dep refresh, session
   naming): args `new <branch>`, `new <branch> --pair` or `new <branch> --single`. Every
   project's `new` treats an existing branch as a switch, so `new <existing>` IS the open.
   Never run a project's `new-task.sh` from here.
4. **Plain `switch` resumes in the recorded mode (MUST FOLLOW).** Do not ask single or
   pair, and add no mode flag. Use `--pair`/`--single` only when the user asks for it.
