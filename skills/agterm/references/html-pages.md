# Displaying an HTML page

Read this when you show the user a generated page (explainer, report, chart, diagram), a URL or a running dev server, or build an interactive page that runs agterm commands or returns a choice. File names below are relative to the skill root.

To show the user a page you generated (explainer, report, chart, diagram), write a static HTML file and
open it as a page companion of your own session: the default, and never hand the user a path instead. Page JavaScript is off unless you pass `--js`. Without `--cwd`
the page has no file access, so keep it self-contained (inline CSS, inline SVG, data URIs).

```bash
agtermctl session overlay open --html <file> --companion --target "$AGTERM_SESSION_ID" --socket "$AGTERM_SOCKET"   # prints the companion id
agtermctl session overlay reload --target <companion id> --socket "$AGTERM_SOCKET"   # after rewriting it
agtermctl session overlay open --url http://localhost:5173/ --companion --js --target "$AGTERM_SESSION_ID" --socket "$AGTERM_SOCKET"
```

A `--url` page forgets cookies on close; `--persistent` keeps logins across closes and restarts. Leave
the page up; the user closes it. Style every page you generate in the plannotator theme
(`sadensmol-task` → `references/plannotator.md` → *Theme*); this overrides the colour advice in
`reference.md` → `session overlay open --html`. The theme variables the page receives
(`--agterm-background`, `--agterm-foreground`, `--agterm-color-0`..`15`) are only for host
integration and external pages; never declare `--agterm-*` yourself. Check load state in `tree --json --window "$AGTERM_WINDOW_ID"` under
`htmlOverlays`. **Before using `--cwd`, `--navigation`, `--size-percent`, `--pane`, or a `--url` page,
read `session overlay open --html` in `reference.md`.** A companion refuses `--size-percent`, `--width`, `--height` and `--pane`; omit `--companion` only when the user asks for an overlay. Omit `--follow` so that the user's focus stays. Outside agterm, `open <file>` on the HTML page is the fallback; say so in chat.

**Documents and zoom.** To show a PDF, build its page with `sadensmol-task` →
`scripts/pdf-page.py` (*Theme* → *Embedded documents*) and open it with `--companion --js`, no
`--follow`. Zoom: the −/+/Fit buttons, Cmd = - 0, Cmd + wheel; a trackpad pinch is handled but not
verified. Internal links jump in the page; http(s) links go through agterm's open-in-browser prompt
(`reference.md` → `session overlay open --html`). Details are in the helper's docstring.

**Interactive pages.** A file page can run agterm commands without JavaScript: tag a button or form with
`data-agterm="<socket command>"` (e.g. `session.select`, `session.rename`, `session.overlay.close`), plus
`data-agterm-target`, `data-agterm-args` (JSON) and `data-agterm-into="#id"` for the reply. Untargeted
commands act on the page's own session. To get a choice back, open with `--block` and use
`session.overlay.submit` buttons; stdout is `{"outcome":"submitted","value":…}`, exit 2 means dismissed.
With `--js`, `agterm.request(cmd, {target, args})` returns the reply as a promise. Escape any outside text
you put in such a page: the page can run commands. **Read `session overlay submit` in `reference.md`
before building one.** `--url` pages get none of this.

## Native pages

Read this before you open an image, a markdown file, a source or text file, or a log natively (no HTML page of
your own). These pages cannot be `--companion` or `--page`: a report that must stay beside the session is still an
HTML page companion (above).

### Showing a markdown file rendered

Render a `.md` or `.markdown` file as a themed page with `--markdown`, at 95% here:

```bash
agtermctl session overlay open --markdown /path/to/plan.md --size-percent 95 --target "$AGTERM_SESSION_ID"
```

Pictures kept in the file's own folder show; other pictures show their alt text, raw HTML stays text, and a web
link opens in the browser after the user confirms. Add `--navigation` to let a relative link to another `.md`
file in its folder or below open in the same page with back and forward; a ⌘-clicked file asks for it. A file
directly in `/` or the home folder opens without it, and so does a page shown on an older agterm on another Mac. The user gets the same view by ⌘-clicking the file's path in a
local pane, so printing the absolute path of a plan or report you wrote is often enough there; for a user watching
from another Mac, open it with `--markdown`. Read back `markdown` in `htmlOverlays`.

A ⌘-clicked `.html` or `.htm` path shows as a page with scripts off, no tagged controls, no access to other
files and no network loads, read back as `document`. A page that needs its scripts, its controls or files beside it opens with `--html`.

### Showing a source file or log at a line

Show a source, config, text or log file read-only with `--text`, scrolled to and marking `--line`:

```bash
agtermctl session overlay open --text /path/to/main.go --line 42 --size-percent 95 --target "$AGTERM_SESSION_ID"
```

It is highlighted when its name maps to a known language, up to 256 KB, and plain above; a file over 4 MB is
refused. FILE is taken literally,
never as `path:line`. The page runs nothing, loads nothing and has no links. Markdown, HTML and SVG are not text
pages. The user gets the same view by ⌘-clicking a path or a `path:line` location, and reveals a file on their own Mac
in Finder with ⌘⌥-click. A click in a session attached from another Mac opens the page from the Mac the session runs
on when both run an agterm that forwards clicks; `--text` shows it there with older ones. Read back `text`
(`{file, language?, line?}`) in `htmlOverlays`.

To watch a log, open its end and follow it as it grows:

```bash
agtermctl session overlay open --text /path/to/app.log --follow-file --size-percent 95 --target "$AGTERM_SESSION_ID"
agtermctl session overlay follow off --target "$AGTERM_SESSION_ID"   # pause; on resumes, toggle flips
```

`--follow-file` shows the last 5,000 lines, read from the end so a log of any size opens; `--tail N` sets the count
(1 to 100,000) and alone shows them once without following. It is not `--follow`, which selects the session. A
⌘-click on a `.log` or `.out` path opens the same following page; `app.log:120` still opens the plain page at that
line. The user pauses by scrolling up and resumes at the bottom or with Resume. Read back `text.tail`, `following`,
`lines`, `clipped`, `waiting` (file missing) and `unreadable`. On a session another Mac presents, the log page and
`follow`, `search`, `wrap` and `changes` work on that Mac when its agterm shows live text pages; an older one refuses
them.

Search a text or log page, not the terminal under it:

```bash
agtermctl session overlay search ERROR --target "$AGTERM_SESSION_ID"   # highlights every match
agtermctl session overlay search --next --target "$AGTERM_SESSION_ID"  # prints "2 of 7"; --prev, --close
```

Matching is case-insensitive plain text. A step pauses a following log page, so the match stays in view; setting the
query does not. Read back `text.search` (`{query, matches, current}`). The user has the same search in the page's
bar: `/` or ⌘F focuses it, `n`/`N` step, Esc clears; on a log page `F` toggles follow, `G`/End goes to the bottom and
follows, `g`/Home to the top. A step before the page loads is refused `page not loaded`.

Long lines of a text, log or diff page run off its right edge until wrapped:

```bash
agtermctl session overlay wrap on --target "$AGTERM_SESSION_ID"   # off unwraps, toggle flips; prints the state
```

The user wraps with the bar's Wrap button or `w`. A page opens unwrapped and keeps its choice until it closes. Read
back `text.wrap`, true while wrapped and omitted otherwise.

To show the user changes, write each changed file's diff to its own `.diff` file and list them as links; a ⌘-click
opens it as a read-only text page:

```bash
git diff --no-color HEAD~3 HEAD -- src/app.go > /tmp/src_app.go.diff
```

Then print `[src/app.go diff](/tmp/src_app.go.diff)`, one link per file, since a multi-file patch shows as one stream
with no file list. Use absolute, space-free output paths so the printed paths are clickable without OSC 8.
The page steps between changes with `]`/`[` or `agtermctl session overlay changes --next|--prev`, which prints
`N of M`; read back `text.changes` (`{count, current}`).


### Displaying an image inline

Show a png, jpg, gif or webp picture with `--image`; the panel takes the picture's shape and size, up to
80% of the session:

```bash
agtermctl session overlay open --image /tmp/shot.png --target "$AGTERM_SESSION_ID"
```

The user can open the same view by ⌘-clicking a picture's path, so printing the absolute path of an image you
produced is often enough. When the user watches your session from another Mac, that click works only when both
Macs run an agterm that forwards clicks; `--image` shows it on that Mac either way, as `--markdown` and `--text`
pages do. When `--image` is rejected, by an older CLI as an unknown option, by an older app, or
by a presenting Mac too old for these pages (`a viewer presents this session`), fall back to `scripts/show-image.sh` (`references/bundled-scripts.md`).

