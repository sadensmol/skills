# Forwarding `new` and `plan`

Read this before you forward `new` or `plan` to the workspace skill.

**OpenCode mode for `plan`:** When a new OpenCode session starts `task plan`, use the
write-enabled `task` mode. Do not use OpenCode's read-only `plan` mode. The `plan`
subcommand writes `<worktree-base>/PLAN.md` and opens its approval gate; it does not edit
application code until approval.

- **A base on `new` is forwarded, never interpreted.** `--base <branch>`, a base task
  (key or tracker URL), or the prose form ("based on `X`", "on top of `X`", "stacked on
  `X`") goes down verbatim: `/task new abc-2-foo --base abc-1-foo` → args `new abc-2-foo
  --base abc-1-foo`; `/task new abc-2-foo based on ABC-1` → args `new abc-2-foo based on
  ABC-1`. Do not resolve it to a PR, SHA or `origin/<branch>`, and do not drop it as stray
  prose. The workspace's `new` resolves a base task to its PRs per repo
  (`references/new.md` → *Based on a task*); its `finish` opens the stacked PRs and
  registers the GitHub stack (`sadensmol-github` → "Stacked Pull Requests"); never do
  either here.
- **An issue-tracker URL as the branch → the tracker's real branch name.**
  Never slugify the URL path (`|` in titles becomes `-or-` junk). Fetch the issue through
  the tracker skill and pass its canonical branch name (Linear's `gitBranchName`, e.g.
  `proj-12-be-acme-inbound-session-auth`). A slug-derived name creates a second worktree
  for the same ticket.
