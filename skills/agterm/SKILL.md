---
name: sadensmol-agterm
description: >
  Drive agterm, a native macOS terminal, through its agtermctl CLI and local control socket. Use when
  running inside an agterm session and asked to control the terminal — sessions, workspaces, windows,
  splits, overlays, HTML/URL pages in an overlay (interactive, with saved logins), HUDs and captions,
  notifications, pickers and question dialogs, inline images, typing, restarting a pane, clipboard and
  scrollback, keymap/custom commands/theme/config, events and event hooks, reopening closed sessions,
  remote attach, installed vs running version and upgrades — or to diagnose or report an agterm problem.
when_to_use: >
  Trigger on: agterm, agtermctl, AGTERM_SESSION_ID, and, from inside a session, plain requests such as
  split the pane, close the overlay, show a message over the session, show a question dialog, agtermctl ask,
  show an image inline, show this HTML page or artifact, make an HTML page or explainer for this and show
  it, make a page that switches sessions or returns a choice, preview the report you generated, show this
  URL or the running dev server, keep me logged in to a page shown in an overlay, search the scrollback,
  run my custom command, tell me when the selected session changes, bring back the session I just closed,
  copy this to the clipboard, was agterm upgraded under the running app, upgrade agterm without losing sessions,
  what's new in agterm, update the agterm skill, attach a session from another Mac, what recipes are there,
  the keymap editor will not open.
allowed-tools: Bash(agtermctl *)
---

<!-- agterm-skill -->

# Driving agterm

agterm is a native macOS terminal. It exposes a programmatic control channel over a local unix
socket, driven by the companion CLI `agtermctl`. Use it to build and steer terminal layouts, run
programs in overlays, type into sessions, notify the user in the exact session you are working in,
and subscribe to control events. Events cover status, notifications, session lifecycle, split and
scratch pane visibility, selection changes, remote attach, structural tree changes (an overlay, page or
HUD opening or closing included) and the app's launch; `hooks.conf` runs a shell line on any of them, so
`on app.launched` is where a script recreates what a relaunch does not restore. They do not stream terminal output; use `session text` to read a buffer.

## Am I inside agterm?

Each shell agterm spawns gets these environment variables. Check `AGTERM_ENABLED` before assuming
the control channel is available:

- `AGTERM_ENABLED=1` — this shell runs inside agterm.
- `AGTERM_SESSION_ID` — the current session's UUID (the session this shell belongs to).
- `AGTERM_WINDOW_ID` / `AGTERM_WORKSPACE_ID` — the owning window / workspace UUIDs.
- `AGTERM_SOCKET` — the absolute path to the control socket this app bound.
- `AGTERM_PANE` / `AGTERM_PANE_ID`: the surface's spawn role (`left`|`right`|`scratch`) and stable
  per-surface token. The role is not rewritten after promotion or swap; the token resolves the LIVE slot.
  Prefer `--pane-id "$AGTERM_PANE_ID"` where supported, including `session status`, `session restore` and
  `session text`. The agent-status hook forwards both values for compatibility.
- `TERM_PROGRAM=agterm` / `TERM_PROGRAM_VERSION` (agterm's version): the terminal identity, replacing
  the `ghostty` pair embedded libghostty would set. A tool that decides a capability from a list of
  terminal names (Claude Code's OSC 8 hyperlinks) needs its own override; see troubleshooting.md.

The quick terminal is scratch (not in the tree) and belongs to no window, so of the `AGTERM_*` variables
it only gets `AGTERM_ENABLED` and `AGTERM_SOCKET` (no session/workspace/window ids). An untargeted `agtermctl` run
from it therefore resolves the active window like any other caller.

These variables are inherited by every process the session's shell spawns — including long-lived
daemons that outlive the shell. A screen server, a session manager (agent-deck and the like), or
any background service started from inside a session captures the spawning session's `AGTERM_*` and
passes it to every child it ever creates, so status hooks running in those children resolve
`$AGTERM_SESSION_ID` to the session that happened to start the daemon and report to the WRONG session.
Before starting such a process from inside agterm, scrub the variables
(`env -u AGTERM_ENABLED -u AGTERM_PANE -u AGTERM_PANE_ID -u AGTERM_SESSION_ID -u AGTERM_SOCKET -u AGTERM_WINDOW_ID -u AGTERM_WORKSPACE_ID <cmd>`);
see troubleshooting.md ("agent-status glyph updates the wrong session") for diagnosing and fixing an
already-poisoned daemon.

## Running agtermctl

`agtermctl` must be on PATH (install it from agterm's **Help ▸ Install Command Line Tool…**). If it
is not on PATH, the user can install it, or you invoke it by absolute path.

- When `AGTERM_SOCKET` is set, pass `--socket "$AGTERM_SOCKET"` on every call: `agtermctl` never reads it
  automatically. Otherwise the socket path auto-resolves.
- `--socket` and other options go **after** the subcommand: `agtermctl tree --json`, not
  `agtermctl --json tree`.
- Add `--json` to any command to get the raw JSON response (machine-readable). Without it, ordinary
  mutations print `ok`, batch close/move prints the affected session count, and `tree`/`list` print a
  human listing.
- Commands other than `events` make one request per invocation. `events` polls with a fresh connection
  for each request. Mutating commands return the affected/new id; batch session mutations return the
  number actually changed. Create commands (`session new`, `session duplicate`, `workspace new`,
  `window new`) print the new id.

## The model

A **window** is the top level: a named bundle rendered in its own on-screen macOS window. Each window
holds a tree of **workspaces**, each holding **sessions**. A session has a primary shell and can also
have: a **split** pane (a second shell side by side), a **scratch** terminal (a third full-coverage
shell, toggled like the split), and an ephemeral **overlay** (runs one program on top, then vanishes).
An overlay covers the whole session, or with `--pane left|right` exactly one split pane, leaving
the sibling pane visible and usable. `--html FILE` / `--url URL` puts a web page there instead of a
program (see "Displaying an HTML page" below). The same session-wide slot also holds a **HUD**
(`session hud`), a small passive panel carrying a message instead of a program. A HUD can use the
whole session or one pane as its placement bounds. The session keeps focus and stays typable
under it.
One slot, so a session shows either a HUD or a program overlay, never both. Separately, the app has one
**quick terminal** (a scratch shell in a floating panel at 90% of the focused screen capped at 1100x700,
or whatever share Settings sets instead; not part of the tree and not owned by a window).

Inspect the live tree any time with `agtermctl tree --json` (workspaces → sessions, each with
`id`, `name`, `cwd`, `splitCwd`, `title`, `active`, `split`, `overlay`, `hud`, `ask`, `scratch`, `status`,
`background`, `surfaces`, plus read-only top-level window state). `title` is the raw OSC terminal title —
read it when a session's local `cwd` is stale because it is connected to a remote. List windows with
`agtermctl window list --json` (`geometry`, `fullscreen`/`zoomed`/`minimized`, for record-then-restore).
**Before relying on a field's exact meaning or omission rule, read `## tree` / `## window` in
`reference.md`.**

## Addressing

Commands that target a session or workspace take `--target` (default `active`):

- `active` — the selected session / current workspace.
- a full UUID (case-insensitive), or a unique **prefix** of one (git-style). Zero matches → `notFound`
  error; two or more → `ambiguous` error listing candidates.

`window.*` commands take the window id/prefix/`active` as a positional argument. Other commands accept
a global `--window <id|prefix|active>` to operate on a specific window's tree (default: the frontmost).

Scripts rarely type ids: create with `*.new` (capture the returned id), or act on `active`.

**Agents: `active` is almost never your own session.** `active` is the session the USER has selected in
the GUI; your shell runs in `$AGTERM_SESSION_ID`, and the user is usually on a different session while
you work. For any session-scoped command meant to act on *this* session — `session overlay open`,
`session scratch`, `session type`, `session text`, `session background`, `session status`, `session copy`,
… — pass `--target "$AGTERM_SESSION_ID"`. Omit it and
you open overlays / type into whatever the user has selected, not your own session.

## Restore modes

**Settings ▸ General ▸ Restore sessions** (global, applies after restarting agterm) picks **Fresh shells**,
**Re-run commands**, or **Live sessions** (primary and split panes run through zmx and reattach to the
same process; needs zsh as the login shell). **Before reasoning about what survives a quit, restart or
crash** — capture, the cases that fall back to a fresh shell, `backedByZmx`, reattach limits — read the
`## restore modes` section of `reference.md`.

## Launching a program in a session

**Bind it at creation.** `session new --command` (and `scratch --command`) makes the program the session
process, so no shell line is involved:

```bash
agtermctl session new --cwd ~/proj --name worker \
  --command "zsh -lc 'claude \"\$(cat ~/brief.md)\"'"   # GUI PATH: wrap a non-default binary
```

In Fresh shells and Re-run commands modes, the session closes when this command exits unless `--wait` holds
the final output. In Live sessions mode the command is a create-only zmx payload, which bypasses the 1,024-byte
PTY input cap. A surviving daemon ignores the payload; a new daemon runs it, then starts the persistent shell.
The shell stays open after it exits and `--wait` adds no hold prompt. After a clean quit, a missing daemon
replays the captured running command inside a new persistent shell, except in the fresh-shell cases
listed under `## restore modes` in `reference.md`.

`session type` drives an ALREADY-RUNNING program — it is not a launcher. Its keystrokes land in a line
buffer you do not own: a newline submits (a multi-line brief becomes N premature Enters), and the user
or a concurrent agent writes to that same buffer. An untargeted `session type` from another agent hits
whatever is `active`, and `session new` focuses — so a just-created session is briefly `active`, a stray
prompt concatenates with yours, and the program starts on the merged line. (`--no-select` skips the
focus, but the newline and shared-buffer hazards of `type`-as-launcher remain — `--command` is still the
rule.) After `--command`, confirm in `tree --json` that the new node's `foreground` shows your program running, not a bare shell prompt.

## Command summary

Run `agtermctl <area> <cmd> --help` for exact flags. **Before using a command whose flags, return shape,
read-back field, or refusals you do not already know, read its section in `reference.md`** — each
section is named after the area (`## tree`, `## session`, …). Worked examples are in `examples.md`;
installable community workflows in `cookbook.md`.

- **tree** (`## tree`) — `tree [--json]`: per-session panes, `foreground` argv, status, split / overlay /
  HUD / ask state, `surfaces`; top-level window state (`idleMs`, `sidebar*`, `quickVisible`,
  `zoomedSurface`, `dashboard*`, `pickPending`, `askPending`, `app`).
- **events** (`## events`) — `events [--json] [--kind …] [--run RUN --after SEQ] [--limit N]`: status,
  notify, `session.created`/`closed`/`selected`, `tree.changed`, `pane.split`/`pane.scratch`,
  `remote.opened`/`closed`, `app.launched`. No terminal-output stream.
- **workspace** (`## workspace`) — `new [--collapsed]`, `rename`, `delete`, `select`, `go`, `move`,
  `focus`, `filter`, `collapse`, `expand`.
- **session** (`## session`) — `new` (`--command`, `--wait`, `--name`, `--after`/`--before`,
  `--no-select`), `duplicate`, `close`, `select`, `rename`, `reveal`, `go`, `move`, `type`, `copy`,
  `paste`, `select-all`, `text`, `search`, `split`, `swap`, `scratch`, `focus`, `resize`, `status`,
  `flag`, `context`, `seen`, `restore`, `restart --pane-id ID [--command LINE]` (start a pane's program
  over in place), `lead`, `background` (`--pane` per-pane override), `overlay` (`open` a program,
  `--html FILE` or `--url URL`; `resize`/`reload`/`navigate`/`submit`/`close`/`result`/`copy`/`text`),
  `hud` (`open`/`update`/`close`, `--hide-after SECONDS`, `--sticky`/`--no-frame` for an edge caption).
- **window** (`## window`) — `new`, `list`, `select`, `go`, `close`, `rename`, `delete`, `resize`,
  `move`, `zoom`, `fullscreen`, `minimize`.
- **surface** (`## surface`) — `surface zoom`, `surface cursor`.
- **dashboard** (`## dashboard`) — `dashboard <ids…>` / `--mru` / `--close`, `--font-size N` /
  `--auto-size`.
- **pick** (`## pick`) — native fuzzy picker over stdin choices; `--select ID`, `--no-block`,
  `pick result`/`cancel`.
- **ask** (`## ask`) — `ask TITLE --button ID=LABEL …`, `--style terminal|gui`, `--no-block`,
  `ask result`/`cancel`.
- **quick** (`## quick`) — `quick show|hide|toggle`, `quick type`, `quick text`.
- **sidebar** (`## sidebar`) — show/hide/toggle, `mode`, `flagged-layout`, `expand`, `collapse`, `width`.
- **notify** (`## notify`) — `notify <body> [--title T]`. To signal that you need the user, prefer
  `session status` (`blocked`/`completed`); keep `notify` for a one-off nudge.
- **font** (`## font`) — `font inc|dec|reset [--pane left|right|scratch]`.
- **keymap** (`## keymap`, `### keymap.conf format`) — `keymap reload`, `keymap list`, `keymap run NAME`
  (start a user custom command; the reply means it started, not that it succeeded).
- **clipboard** (`## clipboard`) — `clipboard set [TEXT]` (stdin when omitted): set the clipboard of the terminal showing
  the pane. `session copy` does NOT touch the clipboard.
- **browser** (`## browser`) — `browser clear` (drop `--persistent` page logins), `browser links [browser|overlay]`
  (where a clicked terminal link opens).
- **recent** (`## recent`) — `recent list`, `recent open [ID]` (reopen a closed session or workspace), `recent clear`.
- **hooks** (`## hooks`) — `hooks reload`, `hooks list`: `hooks.conf` lines `on <kind> <shell...>` run on
  events, with the event JSON on stdin and `AGT_EVENT_*` / `AGT_SESSION_ID` / `AGT_SOCKET` in the env.
- **config** (`## config`) — `config reload` (the agterm-scoped `ghostty.conf`).
- **theme** (`## theme`) — `theme list`, `theme set [name]`, `--dark <name>|none`.
- **restore** (`## restore`) — `restore capture`, `restore clear`, `restore mode [none|rerun|live]`.
- **zmx** (`## restore`) — `zmx list`, `screen NAME` (a closed window's pane as text), `prune`, `kill`,
  `reset`, `tree [HOST]`, `attach HOST SESSION`.
- **terminfo** (`## terminfo`) — `terminfo install DESTINATION`: install `xterm-ghostty` terminfo on a
  remote host over ssh (fixes `less`/`vim` "terminal not fully functional"). Local-only, no socket.
- **version** (`## version`) — `version` (`result.app` is the running app, `result.installed` the bundle
  on disk; they differ after an upgrade under the running app); pass `--socket "$AGTERM_SOCKET"` (or `"$AGT_SOCKET"` in a
  keymap child), since a bare call may answer for another app.

## Displaying an HTML page

To show the user a page you generated (explainer, report, chart, diagram), write a static HTML file and
open it in your own session's overlay. Page JavaScript is off unless you pass `--js`. Without `--cwd`
the page has no file access, so keep it self-contained (inline CSS, inline SVG, data URIs).

```bash
agtermctl session overlay open --html <file> --target "$AGTERM_SESSION_ID" --follow --socket "$AGTERM_SOCKET"
agtermctl session overlay reload --target "$AGTERM_SESSION_ID" --socket "$AGTERM_SOCKET"   # after rewriting it
agtermctl session overlay open --url http://localhost:5173/ --js --target "$AGTERM_SESSION_ID" --follow --socket "$AGTERM_SOCKET"
```

A `--url` page forgets cookies on close; `--persistent` keeps logins across closes and restarts. Leave
the page up; the user closes it with ⌘W. Style it from the theme variables the page receives
(`--agterm-background`, `--agterm-foreground`, `--agterm-color-0`..`15`) with fallbacks, and never
declare `--agterm-*` yourself. Check load state in `tree --json --window "$AGTERM_WINDOW_ID"` under
`htmlOverlays`. **Before using `--cwd`, `--navigation`, `--size-percent`, `--pane`, or a `--url` page,
read `session overlay open --html` in `reference.md`.** Outside agterm, `open <file>` is the fallback.

**Interactive pages.** A file page can run agterm commands without JavaScript: tag a button or form with
`data-agterm="<socket command>"` (e.g. `session.select`, `session.rename`, `session.overlay.close`), plus
`data-agterm-target`, `data-agterm-args` (JSON) and `data-agterm-into="#id"` for the reply. Untargeted
commands act on the page's own session. To get a choice back, open with `--block` and use
`session.overlay.submit` buttons; stdout is `{"outcome":"submitted","value":…}`, exit 2 means dismissed.
With `--js`, `agterm.request(cmd, {target, args})` returns the reply as a promise. Escape any outside text
you put in such a page: the page can run commands. **Read `session overlay submit` in `reference.md`
before building one.** `--url` pages get none of this.

## Displaying an image inline

This skill bundles `scripts/show-image.sh`. It opens an overlay (a real terminal) and renders the
image there via the kitty graphics protocol, which ghostty draws natively — no kitty binary and no
external image tool, just `base64` + `printf`. Run it with the image path (optional size percent,
default 60):

The script sits next to this file, so resolve `scripts/show-image.sh` against the directory this
`SKILL.md` was loaded from — not against your working directory. That one form is correct wherever
the skill came from: a plugin install (`~/.claude/plugins/cache/…`, `~/.codex/plugins/cache/…`) or the
app's own **Help ▸ Install Agent Skill…** copy (`~/.claude/skills/agterm/`, `~/.codex/skills/agterm/`).

```bash
bash <this-skill-directory>/scripts/show-image.sh <image> [size-percent]
```

The size percent sizes the overlay PANEL; the image itself is scaled to fit that panel and centered
in it, so a large screenshot needs no resizing beforehand.

Do NOT print graphics escapes to your own tool stdout (the agent harness escapes the control bytes)
and do NOT run an image viewer in your tool shell (no controlling terminal). The overlay is what makes
it render. Outside agterm (`AGTERM_ENABLED` unset) there is no overlay — fall back to `open <image>`.

## Long-command status wrapper (`agst`)

This skill bundles `scripts/agst.sh`, the cookbook's `long-commands-status` wrapper, so nothing is
copied to `~/bin`. It sets the session's row `active` while a command runs, then `completed` on exit 0
or `blocked` on non-zero; the exit code passes through, and outside agterm it just `exec`s the command.
Resolve it against this skill's directory, physically (`pwd -P`), and call it through `sh`:

```bash
AGST="$(cd <this-skill-directory> && pwd -P)/scripts/agst.sh"
sh "$AGST" make test
agtermctl session new … --command "zsh -lc 'sh \"$AGST\" task dev'"
```

`whats-new.md` step 2 refreshes it when the recipe changes upstream.

## Troubleshooting and reporting

When the user hits a problem (a keymap editor that will not open, a custom action that does nothing,
notifications missing), diagnose it from inside the session first: inspect `agtermctl tree --json`,
run `agtermctl keymap reload` for the parse-diagnostic count, and read the unified logs under
subsystem `com.umputun.agterm`. If it turns out to be a bug, offer to help file it.

**Filing is opt-in and draft-first.** Never run a `gh` command without the user's explicit approval.
Decide first whether it is a bug (a supported feature misbehaving → a GitHub **issue**) or something
not supported / a question / an idea (→ a GitHub **Discussion**, category `Ideas` or `Q&A`). Draft the
title and body, show it to the user, scrub anything private (tokens, hostnames, usernames in paths,
selection/clipboard text), and only post after an explicit go-ahead. If `gh` is missing or not
authenticated, hand the user the prefilled text plus the new-issue / new-discussion URL instead.

Full detail, templates, and the exact `gh` commands are in **troubleshooting.md**.

## Reference files

- **whats-new.md** — run it for "what's new in agterm" or "update the agterm skill": sync this skill from
  the app's bundle, check the cookbook for new recipes, refresh the bundled `scripts/agst.sh`, and map
  both onto the current workflow.

- **reference.md** — full per-command detail: every flag, the JSON return shapes
  (`result.id`/`text`/`exitCode`/`count`/`affected`/`tree`/`windows`/`app`/`restore`/`zmx`/`remote`), error strings, the scratch/overlay/split
  lifecycle, and the keymap.conf format (`map` / `command`, chords, leaders, `|` alternatives,
  `{AGT_X}` tokens).
- **examples.md** — copy-paste agtermctl examples for common tasks (build a layout, run a program in a
  blocking overlay and read its status, type into a fresh session, notify, inspect the tree).
- **cookbook.md** — how to list, acquire and install the repository's cookbook recipes, and how to read
  one as reference for a tricky workflow. The recipe list lives in the repo, not here; fetch it.
- **troubleshooting.md** — diagnosing common problems (keymap editor, custom actions, logs) and the
  bug-issue / feature-Discussion reporting workflow (draft-first, scrub, never post without approval).
- **scripts/show-image.sh** — bundled helper that displays an image inline in an overlay (see above).

Read those files when you need exact flags, return shapes, or worked examples.
