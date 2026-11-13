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
