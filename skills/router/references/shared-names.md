# Shared skill names (Section I)

Read this when the user types a bare skill name that more than one plugin defines
(`/task`, `/linear`, `/router`, …), or loads a project skill that has a `sadensmol-` base.

**I. Shared skill names across the personal plugins.** `linear`, `router`, `task` (and any
future collision) exist in more than one plugin, so a bare `/<name>` or `<name> <args>` is
ambiguous and the harness may tie-break wrongly. A project (work_) plugin **overrides**
sadensmol for a colliding name, layered **on top of** sadensmol's base:

| Bare name | project session | plain personal / other |
|---|---|---|
| `task` | `sadensmol-task` (dispatcher) + `<project>-task` | — (no project task flow) |
| `linear` | `<project>-linear` + `sadensmol-linear` base if the project defines one, else `sadensmol-linear` | `sadensmol-linear` |
| `router` | `<project>-router` | `sadensmol-router` |

- Session belongs to a project whose plugin defines the name → that namespace wins; defer
  to its router (delivered on its own, like this one). Load the related `sadensmol-<name>`
  base **before** the project skill, even for a direct or fully-qualified load. Skip bases
  that are absent or unrelated; the routers themselves are independent passes, not a base pair.
- Otherwise → the `sadensmol-` version wins; override any other plugin's `<name>` the
  harness pre-loaded. The fully-qualified `/<plugin>:<name>` always targets the right one.
