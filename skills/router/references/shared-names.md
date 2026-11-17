# Shared skill names (Section I)

Read this when the user types a bare skill name that more than one plugin defines
(`/task`, `/linear`, `/router`, …), or loads a workspace skill that has a `sadensmol-` base.

**I. Shared skill names across the personal plugins.** `linear`, `router`, `task` (and any
future collision) exist in more than one plugin, so a bare `/<name>` or `<name> <args>` is
ambiguous and the harness may tie-break wrongly. A workspace (work_) plugin **overrides**
sadensmol for a colliding name, layered **on top of** sadensmol's base:

| Bare name | workspace session | defaults session |
|---|---|---|
| `task` | `sadensmol-task` (dispatcher) + `<workspace>-task` | `sadensmol-task` (its default flow, with the workspace registry) |
| `linear` | `<workspace>-linear` + `sadensmol-linear` base if the workspace defines one, else `sadensmol-linear` | `sadensmol-linear` |
| `router` | `<workspace>-router` | `sadensmol-router` |

- **defaults session** (only as Section A defines it: no workspace router or hook detects its
  own workspace for the session): `task` → `sadensmol-task` alone (it runs its default flow with
  the registry (`.workspace/settings.json` and `.workspace/projects/<project>.json`); load no `<workspace>-task`), `linear` →
  `sadensmol-linear` alone (never another workspace's `<workspace>-linear`, or Jira), `router` →
  `sadensmol-router`.
  In a session that a workspace detected (its agterm workspace, router or hook, or a cwd under
  `/work_<workspace>/`), that workspace's mapping wins: `linear` → `sadensmol-linear` +
  `<workspace>-linear`, never `sadensmol-linear` alone.
- Session belongs to a workspace whose plugin defines the name → that namespace wins; defer
  to its router (delivered on its own, like this one). Load the related `sadensmol-<name>`
  base **before** the workspace skill, even for a direct or fully-qualified load. Skip bases
  that are absent or unrelated; the routers themselves are independent passes, not a base pair.
- Otherwise → the `sadensmol-` version wins; override any other plugin's `<name>` the
  harness pre-loaded. The fully-qualified `/<plugin>:<name>` always targets the right one.
