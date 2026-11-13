# Planning model

Read this before you start a plan or change a `PLAN.md`.

Planning runs in the harness `planner-subagent`, never on the main session's model: a
`task plan` run, every change to a task's `PLAN.md` before the user approves it (an
addition, a refactor, a rework after review), and any plan the user asks for outside a
task. After the approval `PLAN.md` is frozen: no planner runs, and all feedback is
implementation work (task skill → *Stages*). The main session talks to
the user, then gives the planner a brief: load `sadensmol-plan` (the plan's structure, `.md`
only), the goal, the user's decisions, the files in scope and the plan file path. The planner explores and writes the whole plan without
stopping: a question it cannot settle goes to `## Questions to clarify` with the answer the
plan assumes (`sadensmol-plan` → *Questions to clarify*). The main session shows the plan
to the user and then asks those questions one at a time. In a task, the main writer and reviewer sessions also run on the
planning model until the user approves the plan (task skill → *Stages*).
