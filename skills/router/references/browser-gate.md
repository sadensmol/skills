# Browser gate

Read this before the first browser tool call of a task.

Before the first browser tool call of a task — any URL, Google Sheet / Doc, dashboard, partner
docs — read the active workspace's browser policy (its router names the file) and the harness
`browser` capability. Use the server that the workspace policy names first. With no workspace
policy, use `browser.server` of the workspace's `.workspace/workspace.json` when it is set.

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

Never take a credential from a page: no token, cookie or `localStorage` / `sessionStorage`
value, and no API call made with the user's own web session. Never save an auth request or
response body (SAML, OAuth) to a file. When the configured token fails, stop and ask.

One exception: a read-only query to a site's own API, run inside that site's page by the selected
server, is allowed when the workspace's browser policy names that site and API. The session stays in
the page: never copy a credential out, and never replay the call from another client.

## A disabled button means an empty required field (MUST FOLLOW)

Before you say a form or dialog is blocked, read all of it. Scroll every scroll container to
the end, or list every control in the dialog, also the ones below the fold. A disabled
*Next*, *Submit* or *Create* button means a required field is still empty: find that field,
fill it, then go on. Example: a create-from-manifest dialog keeps *Next* disabled until you
pick the workspace, and that picker sits under the long manifest editor. Never report
"blocked" or ask the user after reading only the visible part of a form.
