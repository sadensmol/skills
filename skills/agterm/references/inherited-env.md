# Inherited AGTERM_* variables

Read this when you run from the quick terminal, or before you start a long-lived daemon, screen server or session manager from inside an agterm session. File names below are relative to the skill root.

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
