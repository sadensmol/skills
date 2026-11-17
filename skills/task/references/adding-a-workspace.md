# Adding a new workspace

Read this when you add a new workspace's `<workspace>-task` skill to the dispatcher.

1. Give the workspace's plugin a `task` skill with `user-invocable: false`. It implements
   only `new` / `plan` / `finish` / `cleanup` (reading the shared `references/` files for
   each), `pull` (its source checkouts fast-forwarded to their default branches, refused
   inside a worktree), plus its layout, preflight and definition of done. Its `new` writes the ticket id
   into the agterm session context; `status` reports that id without interpreting it.
2. Name the agterm workspace after the plugin — `SKILL.md` → *Step 2* then resolves it with no edit there.
3. For cwd resolution outside agterm, add `{ "cwd": "<substring>", "skill":
   "<plugin>:<skill>" }` to the local, unpublished `~/.agents/sadensmol-router-routes.json`.
   Nothing is added to `SKILL.md`.

The default workspace, sadensmol, has no workspace task skill: its flow is in `SKILL.md` →
*Subcommands*, `references/defaults/` and this skill's `scripts/`.
