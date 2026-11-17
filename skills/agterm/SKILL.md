---
name: sadensmol-agterm
description: >
  Drive agterm, a native macOS terminal, through its agtermctl CLI and local control socket. Use when
  running inside an agterm session and asked to control the terminal — sessions, workspaces, windows,
  splits, overlays, HTML/URL pages in an overlay or as a page row (interactive, with saved logins), HUDs and
  captions, notifications, the sidebar activity line, pane messages, pickers and question dialogs, images,
  rendered markdown, source files and followed logs, session links, typing, restarting a pane, clipboard and
  scrollback, keymap/custom commands/theme/config, events and event hooks, reopening closed sessions,
  remote attach, agterm-server remote windows, installed vs running version and upgrades — or to diagnose or
  report an agterm problem.
when_to_use: >
  Trigger on: agterm, agtermctl, AGTERM_SESSION_ID, and, from inside a session, plain requests such as
  split the pane, print a link that jumps to a session, close the overlay, show a message over the session, show a question dialog, agtermctl ask,
  show an image inline, show this markdown file rendered, show this source file or log at a line, show me the
  diff of a file or the changes as links, tail or follow this log file, search the log or text page in the
  overlay, wrap long lines in the text or log page, show this HTML page or artifact, make an HTML page or explainer for this and show
  it, make a page that switches sessions or returns a choice, preview the report you generated, show this
  URL or the running dev server, keep me logged in to a page shown in an overlay, search the scrollback,
  flag every session in a workspace, tint a session or workspace row a color, show status or activity under a
  session in the sidebar, make a companion session, make this session a companion,
  keep a related program next to a session, duplicate this session with its programs as a companion,
  show a page as a companion of this session, open a page as its own session row in the sidebar, fold a
  session's companions, send a message to the other pane or another agent, read my messages, wait for a message, ask the user to type a value,
  hide the overlay for a moment, wait until that worker session finishes,
  run my custom command, tell me when the selected session changes, bring back the session I just closed,
  copy this to the clipboard, was agterm upgraded under the running app, upgrade agterm without losing sessions,
  what's new in agterm, update the agterm skill, attach a session from another Mac, attach a remote session
  with all its companions, open a remote window for my Linux server, run sessions on a Linux host with
  agterm-server, install agterm-server on a Linux host, detach or reattach a remote session, what recipes are there,
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

Every process the session's shell spawns inherits these variables, long-lived daemons included.
**Before starting a daemon, screen server or session manager from inside agterm, scrub them**
(`env -u AGTERM_ENABLED -u AGTERM_PANE -u AGTERM_PANE_ID -u AGTERM_SESSION_ID -u AGTERM_SOCKET -u AGTERM_WINDOW_ID -u AGTERM_WORKSPACE_ID <cmd>`),
or its children report to the WRONG session. Read `references/inherited-env.md` for the full rule and for
the quick terminal's reduced variable set. Claude Code background sessions (`claude --bg`) get none of the
variables; troubleshooting.md ("Claude Code background sessions have no status") passes them in through `--settings`.

## Running agtermctl

`agtermctl` must be on PATH (install it from agterm's **Help ▸ Install Command Line Tool…**). If it
is not on PATH, the user can install it, or you invoke it by absolute path.

- When `AGTERM_SOCKET` is set, pass `--socket "$AGTERM_SOCKET"` on every call: `agtermctl` never reads it
  automatically. Otherwise the socket path auto-resolves. Never write
  `${AGTERM_SOCKET:+--socket "$AGTERM_SOCKET"}`: zsh passes it as one word, and `agtermctl`
  fails with `Unknown option '--socket <path>'`.
- `--socket` and other options go **after** the subcommand: `agtermctl tree --json`, not
  `agtermctl --json tree`.
- Add `--json` to any command to get the raw JSON response (machine-readable). Without it, ordinary
  mutations print `ok`, batch close/move prints the affected session count, and `tree`/`list` print a
  human listing.
- Commands other than `events` and `session wait` make one request per invocation. Those two poll with a
  fresh connection for each request. Mutating commands return the affected/new id; batch session mutations return the
  number actually changed. Create commands (`session new`, `session duplicate`, `workspace new`,
  `window new`) print the new id.

## The model

A **window** is the top level: a named bundle rendered in its own on-screen macOS window. Each window
holds a tree of **workspaces**, each holding **sessions**. A session has a primary shell and can also
have: a **split** pane (a second shell side by side), a **scratch** terminal (a third full-coverage
shell, toggled like the split), and an ephemeral **overlay** (runs one program on top, then vanishes).
An overlay covers the whole session, or with `--pane left|right` exactly one split pane, leaving
the sibling pane visible and usable. `--html FILE` / `--url URL` puts a web page there instead of a
program (read `references/html-pages.md`). The same session-wide slot also holds a **HUD**
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
same process; needs zsh or fish as the login shell). **Before reasoning about what survives a quit, restart or
crash** — capture, the cases that fall back to a fresh shell, `backedByZmx`, reattach limits — read the
`## restore modes` section of `reference.md`.

## Launching a program in a session

**Bind it at creation.** `session new --command` (and `scratch --command`) makes the program the session
process, so no shell line is involved:

```bash
agtermctl session new --cwd ~/proj --name worker \
  --command "zsh -lc 'claude \"\$(cat ~/brief.md)\"'"   # GUI PATH: wrap a non-default binary
```

The session closes when this command exits unless `--wait` holds the final output. In Live sessions mode the
command is a create-only zmx payload, which bypasses the 1,024-byte PTY input cap. A surviving daemon ignores
the payload; a new daemon runs it. A command that ends within a moment of the pane's creation can lose its
output and leave only `agterm: session is gone`. After a clean quit, a missing daemon
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
  `zoomedSurface`, `dashboard*`, `pickPending`, `askPending`, `app`, `flaggedOrder`, `attention*`); per-node
  `parent`/`collapsed`/`content` (companions), `tint`/`tintOn`, `info`, `lastSelectedAt`, `scratchCwd`,
  `remoteSessionID`, `liveHostStale`, `sidebarDensity`, `pendingMessages` on a surface.
- **events** (`## events`) — `events [--json] [--kind …] [--run RUN --after SEQ] [--limit N]`: status,
  notify, `session.created`/`closed`/`selected`, `tree.changed`, `pane.split`/`pane.scratch`/`pane.focused`,
  `remote.opened`/`closed`, `app.launched`, `message` (mail queued for a pane; never the body). No
  terminal-output stream.
- **workspace** (`## workspace`) — `new [--collapsed] [--tint C]`, `rename`, `delete`, `select`, `go`, `move`,
  `focus`, `filter`, `flag` (every session of the workspace), `tint` (its own row only), `collapse`, `expand`.
- **session** (`## session`) — `new` (`--command`, `--wait`, `--name`, `--after`/`--before`,
  `--no-select`, `--tint`/`--tint-on`, `--parent SID` for a companion), `duplicate` (`--companion` copies each
  pane's program), `convert --companion` (make a running top-level session a companion of a fresh shell in its
  place), `collapse`/`expand` (fold a parent's companions), `close` (`--undoable`: ⌘W's undo grace), `select`, `rename`, `reveal`, `go`, `move`, `type`, `copy`,
  `paste`, `select-all`, `text`, `search`, `split`, `swap`, `scratch`, `focus`, `resize`, `status` (`--if-idle`
  sets it only on an idle row), `info` (one activity line under the sidebar row; `--markdown`, `--hide-after`),
  `go --to N` (1-based sidebar place),
  `wait --until STATUS` (block until the agent status matches), `flag` (`move --before|--after` reorders), `tint`
  (also teal, pink, brown, indigo, mint or `#rrggbb`), `context`, `seen`, `restore`, `restart --pane-id ID [--command LINE]` (start a pane's program
  over in place), `lead`, `background` (`--pane` per-pane override), `overlay` (`open` a program,
  `--html FILE` or `--url URL`, `--companion` for a page companion row or `--page` for a top-level page row
  (both only with `--html`/`--url`; page rows survive a relaunch), `--image FILE`, `--markdown FILE`,
  `--text FILE [--line N]`, `--follow-file`/`--tail N` for a log, `--width`/`--height` in points;
  `resize`/`reload`/`navigate`/`submit`/`close`/`result`/`copy`/`text`/`hide`/`show`/`toggle`, and on a text or log
  page `follow`/`search`/`wrap`/`changes`),
  `hud` (`open`/`update`/`close`, `--hide-after SECONDS`, `--sticky`/`--no-frame` for an edge caption).
- **window** (`## window`) — `new`, `list`, `select`, `go`, `close`, `rename`, `delete`, `resize`,
  `move`, `zoom`, `fullscreen`, `minimize`, `remote open HOST` (a window mirroring a Linux host's agterm-server).
- **remote server** (`## remote server`) — what `agtermctl` on an agterm-server host serves and refuses;
  `session detach`/`reattach`, `workspace detach`.
- **server** (`## server`) — `server install HOST`: install or upgrade agterm-server on a Linux host over ssh.
- **surface** (`## surface`) — `surface zoom`, `surface cursor`.
- **dashboard** (`## dashboard`) — `dashboard <ids…>` / `--mru` / `--close`, `--font-size N` /
  `--auto-size`.
- **pick** (`## pick`) — native fuzzy picker over stdin choices; `--select ID`, `--no-block`,
  `pick result`/`cancel`.
- **ask** (`## ask`) — `ask TITLE --button ID=LABEL …`, `--input line|multiline`, `--choice ID=LABEL`, `--style terminal|gui`, `--no-block`,
  `ask result`/`cancel`.
- **quick** (`## quick`) — `quick show|hide|toggle`, `quick type`, `quick text`.
- **sidebar** (`## sidebar`) — show/hide/toggle, `mode`, `flagged-layout`, `density default|compact|toggle`,
  `expand`, `collapse`, `width`.
- **notify** (`## notify`) — `notify <body> [--title T]`. To signal that you need the user, prefer
  `session status` (`blocked`/`completed`); keep `notify` for a one-off nudge.
- **msg** (`## msg`) — `msg send TEXT|--stdin --to SESSION:PANE` or `--all`, `msg read [--ack] [--wait]`,
  `msg ack ID…`: per-pane mailboxes, in memory only, dropped when the pane closes or restarts.
- **font** (`## font`) — `font inc|dec|reset [--pane left|right|scratch]`.
- **keymap** (`## keymap`, `### keymap.conf format`) — `keymap reload`, `keymap list`, `keymap run NAME`
  (start a user custom command; the reply means it started, not that it succeeded).
- **clipboard** (`## clipboard`) — `clipboard set [TEXT]` (stdin when omitted): set the clipboard of the terminal showing
  the pane. `session copy` does NOT touch the clipboard.
- **browser** (`## browser`) — `browser clear` (drop `--persistent` page logins), `browser links [browser|overlay|overlay95|overlay75|overlay50]`
  (where a clicked terminal link opens, full overlay or a 95/75/50% panel).
- **recent** (`## recent`) — `recent list`, `recent open [ID]` (reopen a closed session or workspace), `recent clear`.
- **hooks** (`## hooks`) — `hooks reload`, `hooks list`: `hooks.conf` lines `on <kind> <shell...>` run on
  events, with the event JSON on stdin and `AGT_EVENT_*` / `AGT_SESSION_ID` / `AGT_SOCKET` in the env.
- **config** (`## config`) — `config reload` (the agterm-scoped `ghostty.conf`).
- **theme** (`## theme`) — `theme list`, `theme set [name]`, `--dark <name>|none`.
- **restore** (`## restore`) — `restore capture`, `restore clear`, `restore mode [none|rerun|live]`.
- **zmx** (`## restore`) — `zmx list`, `screen NAME` (a closed window's pane as text), `prune`, `kill`,
  `reset`, `tree [HOST]`, `attach HOST SESSION [--companions]`.
- **terminfo** (`## terminfo`) — `terminfo install DESTINATION`: install `xterm-ghostty` terminfo on a
  remote host over ssh (fixes `less`/`vim` "terminal not fully functional"). Local-only, no socket.
- **version** (`## version`) — `version` (`result.app` is the running app, `result.installed` the bundle
  on disk; they differ after an upgrade under the running app); pass `--socket "$AGTERM_SOCKET"` (or `"$AGT_SOCKET"` in a
  keymap child), since a bare call may answer for another app.

## Pages without HTML

For a file that needs no page of your own, open it natively, with `--target "$AGTERM_SESSION_ID"`:

- `session overlay open --image FILE` — a png, jpg, gif or webp picture.
- `session overlay open --markdown FILE [--navigation]` — a rendered markdown file.
- `session overlay open --text FILE [--line N]` — a source, config or text file, read-only, at a line.
- `session overlay open --text FILE --follow-file [--tail N]` — the end of a log, followed as it grows.

None of them can be `--companion` or `--page`. The user opens the same views by ⌘-clicking an absolute path
(`path:line` too), so printing the path is often enough. A diff for the user goes to one `.diff` file per changed
file, each printed as a link. Before you use one, read `references/html-pages.md` → *Native pages*.

## Session links

`agterm://session/<id>` selects that session and raises its window on a ⌘-click. In Claude Code markdown write
`[label](agterm://session/<id>)`; in a shell print OSC 8 (`printf '\e]8;;agterm://session/%s\e\\%s\e]8;;\e\\\n' "$id" label`).
A link in a HUD, quick terminal or program overlay does nothing.

## Troubleshooting and reporting

When the user hits a problem (a keymap editor that will not open, a custom action that does nothing,
notifications missing) or wants to report one, read **troubleshooting.md** first.
**Filing is opt-in and draft-first.** Never run a `gh` command without the user's explicit approval.

## Read on demand

**When a trigger below matches what you are about to do, read that file first** — before
you write, review or answer. The summary in this skill is not the full rule.

- **references/html-pages.md** — read before you open an HTML page or URL in an overlay (an explainer,
  report, chart, diagram, a PDF, dev server), build an interactive page that runs commands or returns a choice,
  or open an image, markdown file, source file or log natively (*Native pages*).
- **references/bundled-scripts.md** — read before you fall back to `scripts/show-image.sh` (when `--image` is refused) or
  wrap a long command so the session row shows its status (`scripts/agst.sh`).
- **references/inherited-env.md** — read before you start a daemon or session manager from inside a
  session, or when you run from the quick terminal.

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
- **scripts/show-image.sh** — bundled helper that displays an image inline in an overlay (see `references/bundled-scripts.md`).

Read those files when you need exact flags, return shapes, or worked examples.
