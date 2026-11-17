# Linear protocol

The issue is the run's record. The service holds the only Linear key, so every Linear read
and write of a run goes through the `autopilot` CLI.

**In a run, never use a tracker tool or a tracker server (MUST FOLLOW).** Load
`sadensmol-linear` and the workspace skill `<workspace>-<workspace>` for the status names and
the comment style only. Their steps that call a tracker server do not apply in a run.

## Phases

`autopilot status <KEY> <phase>` records the phase, sets the issue status that the
workspace registry maps to it (`linear.statuses` in `<registry>/settings.json`). Only `blocked`
posts a line to the thread: the stuck line (`feed.md`). The same phase twice changes nothing.

| Phase | The run session sets it | Typical status |
|---|---|---|
| `planning` | at the start, and before the contract and QA plan waves of each repo | Planning |
| `implementing` | right after the first implementer wave of a repo starts, and on each loop back to implement | In Progress |
| `qa` | before each `qa-verify` wave | In QA |
| `review` | when every changed repo passed the gate | In Review |
| `pr` | when the PRs are open, and after each pushed PR round | PR Open |

The status names come from the workspace registry, never from this table.

Never set these phases:

- `todo` — the service sets it at intake.
- `blocked` — the service shows this status while an ask is open, and sets this phase
  when a loop passes the cap.
- `done` — the service sets it when the PR is merged.
- `canceled` — the service sets it on **Cancel** or `autopilot cancel`.

## Loops and the cap

The service counts each loop when the phase goes back: `qa` → `implementing` (gate),
`review` → `implementing` or `qa` (review), `pr` → `implementing` (PR, not capped). When
the gate or review loop passes `loop_cap` (default 3), the run moves to `blocked`. Then
every status move is refused until the owner runs `resume` from the control room. A run
never resumes itself (`run.md` → *Loop-backs*).

## Asks

While an `autopilot ask` is open, the issue shows the blocked status. When the ask is
answered, the service restores the status of the run's phase. Do not set a phase to show
that a run waits.

## Comments

`autopilot comment <KEY> --file <md>` adds a comment to the issue. Write it in the comment
style of `sadensmol-linear` (or of the workspace skill when it says more): short, plain
English, facts first.

- At the end of planning: the summary of `PLAN.md` (what changes, the criteria `AC<n>`, the
  parts and waves per repo).
- When the PRs are open: one line per PR with its link.

A comment is a record, not a gate. When `autopilot comment` fails, write in the run log
that the comment failed and why, then go on.

## Rules

- Never put a secret, a token or an environment value into a comment.
- Never edit the issue title, description, labels, assignee or project. The service owns
  them.
- Never create, move or close an issue by yourself.
