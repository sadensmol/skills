# Browser gate

Read this before the first browser tool call of a task.

Before the first browser tool call of a task — any URL, Google Sheet / Doc, dashboard, partner
docs — read the active project's browser policy (its router names the file) and the harness
`browser` capability. Use the server that the project policy names first.

Otherwise, read `$HOME/.agents/sadensmol-browser-routes.json` when present.
It holds a JSON array of local routes, each with `server` and either `workspace` or `cwd`.
Match `workspace` exactly against the current session's agterm workspace name.
When `AGTERM_WORKSPACE_ID` exists, load `sadensmol-agterm` to resolve that workspace;
never use the GUI's active workspace instead.
Prefer a workspace match, then the longest matching `cwd` ancestor of the session directory.
Match whole path components, not substrings. With no match, use `playwright-extension`.
Keep machine paths and workspace names in this local file, outside the skills repository.
Keep extension tokens in private MCP configuration, never in skills or route files.
Configure `PLAYWRIGHT_MCP_EXTENSION_TOKEN` on the selected server at startup.

Use only the selected server; never pick a profile or use another browser tool.
If that server is missing, stop and say so. A generic browser skill cannot override this gate.
