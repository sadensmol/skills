# `plan` — default tracker steps

Read `<sadensmol-task-skill-dir>/references/plan.md` and follow it. Its step 2 (tracker) is:

1. **Issue key.** A leading `<team>-<n>` in the branch (`abc-21-…`) → `ABC-21`. An old key
   of a renamed team still resolves: Linear redirects it.
   No key → ask the user what the task is (or plan from the branch name).
2. **Fetch it** via `sadensmol-linear` and its sadensmol defaults: title, description, comments, attachments,
   project. Server not connected → say so and ask the user to paste the issue.
3. **Mark it started**: `state` → the `implementing` status of the registry
   (`registry.py get linear.statuses.implementing`). Skip without a key or the server.

The project is the issue's Linear project (`references/defaults/project.md` → *Projects*).
Read the project repo's source-of-truth docs (its `CLAUDE.md` names them) first and cite
the sections in the plan; the plan says which sections and diagrams change. An **r&d** issue plans the research — questions, what to measure against
the running services (small calls only, never a new model process), and where the findings
are written — not code.
