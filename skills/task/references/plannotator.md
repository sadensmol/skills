# Plannotator — launch, one instance per worktree, stale annotate

Read this before every `plannotator …` launch (`review`, `annotate`, the `PLAN.md` gate,
`reviewd`, `reviewq`), and when you edit a `.md` or `.html` that an open `annotate` shows.

Every `plannotator …` launch — `review`, `annotate`, `annotate PLAN.md --gate` — goes
through the helper with completion delivery (MUST FOLLOW). Use a notifying background
run when the harness provides one; otherwise use the runner below:

```bash
cd <worktree-base> && bash <sadensmol-task-skill-dir>/scripts/plannotator-session.sh <plannotator args…>
```

`<helper>` below = `<sadensmol-task-skill-dir>/scripts/plannotator-session.sh`.

- The helper resolves the binary (`PLANNOTATOR` env, else `command -v plannotator`, else
  `~/.local/bin/plannotator`; never type a bare `plannotator`). On
  `PLANNOTATOR_NOT_FOUND`, tell the user to install it or export `PLANNOTATOR`, and stop.
- It opens plannotator in its own agterm session `review <task ID>`, a companion of the
  task session (`--no-select`, through `child-session.sh`), runs it at the worktree base, prints its output, exits
   with its status and closes that session. A notifying harness returns the result to
   the main session; a plain detached shell does not. Outside agterm it runs directly.
- The page opens as an agterm HTML overlay inside that review session (via
  `PLANNOTATOR_BROWSER`, default `~/.local/bin/plannotator-agterm-open`) — never on the
  task session, never in a browser. `PLANNOTATOR_OVERLAY_OPENER_NOT_FOUND` or
  `OVERLAY_OPEN_FAILED` → report it; never fall back to a browser.
- Never use a bare binary launch. Never leave `&`, `nohup`, or `disown` without a live
  result-delivery path. Do not wait in a foreground shell in the main session; use the
  runner fallback when notifications are unavailable. Launch once and do not relaunch
  while it runs. `REVIEW_SESSION_CLOSED` means closure, not approval; do not relaunch.
- **One live instance per worktree (MUST FOLLOW).** `review` and `annotate` share one
  slot per worktree base; the helper holds it as `<base>/.scratch/plannotator.lock`.
  Other worktrees (other tasks) run their own instance and review session in parallel —
  never wait on them. Check this worktree's slot with
  `cd <worktree-base> && bash <helper> --status` (`RUNNING: <args> (session <id>)` or
  `NONE`). A launch while the slot is held prints `PLANNOTATOR_ALREADY_RUNNING` and
  exits 4 without launching. Never `pgrep`/`pkill` plannotator by name: that matches
  other tasks' instances.
- **Two review baselines.** Code `review` compares with `refs/task-review/last` in each
  repo (`references/review.md`). The `PLAN.md` gate compares with
  `<base>/.scratch/plan-review/last.md` (`references/plan.md`, step 6). Each one's first
  review and an unchanged one open without a guide. `PLANNOTATOR_PROMOTE=code|plan` makes
  the helper stage the matching baseline under the lock; `guide-open.sh` promotes it only
  when the opener returns 0. A failed start or `PLANNOTATOR_ALREADY_RUNNING` moves
  nothing. A launch without it (plain `annotate`, the plan diff view) moves nothing.
- **Reuse a current instance; protect review notes.** Reuse the live instance when it
  shows the required unchanged content. A different file does not make its review
  disposable. Close a superseded task-owned instance with `bash <helper> --close` only
  after preserving submitted annotations and unsent drafts. If preservation cannot be
  proved, ask before closing it. Losing notes is a critical destructive action. Never
  treat a forced close as approval.
- **Stale annotate.** Finish the edits, then refresh the same file under the rule above.
  Do not change review mode to refresh. No open annotate means no refresh.

## Own the gate through its result (MUST FOLLOW)

Opening the page is not completion. Before launch, choose a delivery method the harness
actually supports. Never assume a background shell sends a completion notification.

- **With notifications:** retain the run id and output location. Handle its completion
  as task work, even when the user has sent another question meanwhile.
- **Without notifications:** use one `cheap-subagent` runner in the foreground. It runs
  the helper, waits for its result, and returns the full approval or annotation output.
  The main session keeps the approval decision; the runner only transports the result.
  This runner exception replaces the usual background launch, not the review helper.
- **Already detached without a notifier:** attach one runner to that exact helper
  process and output file. It waits for process completion and reads the final output.
  Do not start a second review. A command timeout means keep watching the same run,
  not approval, cancellation, or permission to go idle.

On return, read the entire result. An explicit approval advances the owning workflow
immediately: plan → implementation; diff → the authorized next step. Handle annotations
through that workflow. A closed page, exit code, or empty result alone never means approval.
Do not end with an acknowledgement when the next authorized action can run.

After a side request or interrupted wait, inspect this task's saved result before going
idle. If the gate completed, process it now. Never make the user announce their approval
again in chat. Keep exactly one result consumer to avoid duplicate transitions.

## Theme (MUST FOLLOW)

Every user-facing HTML page you generate follows the plannotator theme: a page opened in
plannotator, a page companion, or a standalone page. Never hardcode a light or a dark
page: take the mode from the helper.

- Get the mode from the helper:
  `bash <sadensmol-task-skill-dir>/scripts/plannotator-theme.sh` prints `dark`, `light`
  or `system`. It reads `theme.mode` from
  `${PLANNOTATOR_CONFIG:-$HOME/.plannotator/config.json}`. A missing file, malformed
  JSON or unknown value prints `dark`.
- Do not style a generated page from the agterm `--agterm-*` variables. They are for host
  integration and external pages only.
- **Generated HTML** sets its colors for that mode only, with
  `<meta name="color-scheme" content="<mode>">`. Only `system` uses
  `prefers-color-scheme`. Use plannotator's palettes (`.theme-plannotator` and
  `.theme-plannotator.light`): inline `scripts/plannotator-palette.css` and set
  `<html data-theme="<mode>">`, as `scripts/qa-export.py` does.
  `qa-export.py render` takes the mode as `--theme`.
- **Embedded documents.** The theme covers the whole viewing surface: toolbar, scroll
  area, nested HTML and embedded viewer. `color-scheme` is only a hint; it recolors no PDF
  or image content.
  - A PDF shown to the user goes through `scripts/pdf-page.py`, which gives the theme,
    zoom and links. Never use a bare `<iframe>`, `<embed>` or `<object>`: in agterm that
    gave white pages, no zoom and no links.
    `python3 -I <sadensmol-task-skill-dir>/scripts/pdf-page.py <pdf> <out.html> --theme "$(bash <sadensmol-task-skill-dir>/scripts/plannotator-theme.sh)"`,
    then open the page with `--js`. In dark mode the PDF canvas is inverted, images
    included. For original colors, rebuild with `--theme light`.
  - Do not invert arbitrary images in a generated page. Use a dark reading treatment
    (`filter: invert(1) hue-rotate(180deg)`) only for a known light-only image of a text
    document. Keep the original image bytes. Never recolor photos or charts.
  - Run the theme helper before every page, one-off wrappers too. Never pick colors by
    hand.
- **Generated Markdown** holds no raw HTML colors (`style="color:…"`, `<font>`,
  `bgcolor`) and no mermaid `%%{init: {'theme': …}}%%`. Plannotator renders Markdown in
  its own theme.
