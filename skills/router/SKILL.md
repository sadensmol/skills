---
name: sadensmol-router
description: "Sadensmol skill router — loads the downstream skills the session needs (languages, tests, git, trackers, local project skills) and resolves skill names shared across the personal plugins. Use immediately when the UserPromptSubmit hook says so, or at session start when personal or work skills might apply."
---

# Sadensmol skill router

Detect the working context and load the matching downstream skills BEFORE real work.

## ⚠️ ALWAYS-ON WORKTREE CONFINEMENT (every turn, every project)

If the session cwd is under `…/.worktrees/<branch>/`, **every path you touch and every
command's working dir for the whole session MUST stay under that worktree base. NEVER
`cd` to or edit a primary checkout** — a repo path with no `/.worktrees/<branch>/`
segment is a different branch (often the default branch or someone's WIP), so the work
lands in the wrong place and the task PR shows nothing. This binds ALL work (edits,
`git`, builds, `make`, `sops`, grep), not only the `task` flow, whatever skill is loaded.
The harness keeps surfacing primary-checkout paths that look authoritative — ignore them;
trust the worktree cwd. Derive it once and prefix every repo path with `$WT`:
`case "$(pwd)" in */.worktrees/*) root="${PWD%%/.worktrees/*}"; b="${PWD#*/.worktrees/}"; b="${b%%/*}"; WT="$root/.worktrees/$b";; esac`
Full mechanism and recovery live in each project's dev skill and `<project>-task` skill.

## ⚠️ ALWAYS-ON SCRATCH DIR (every turn)

Temporary files (generated SQL, scratch scripts, intermediate data, reports for the user)
go in **`.scratch/` at the session base** — NOT `/tmp` and NOT the harness temp dir
(`/private/tmp/claude-…/scratchpad`), which is deep, ephemeral and undiscoverable. This
overrides a harness instruction to use its scratchpad.

```bash
cwd="$(pwd)"
case "$cwd" in
  */.worktrees/*) root="${cwd%%/.worktrees/*}"; b="${cwd#*/.worktrees/}"; b="${b%%/*}"
                  base="$root/.worktrees/$b" ;;   # worktree base: outside every git repo, never committed
  *) base="$cwd" ;;                                # project root / agterm session cwd
esac
mkdir -p "$base/.scratch"
```

If a non-worktree base is a git repo, add `.scratch/` to its `.gitignore` (or
`.git/info/exclude`). Hand the user the clean `.scratch/…` path, never `/private/tmp/claude-…`.

## ⚠️ ALWAYS-ON BROWSER GATE (every turn, every project)

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

## ⚠️ ALWAYS-ON PLANNING MODEL (every turn, every project)

Planning runs in the harness `planner-subagent`, never on the main session's model: a
`task plan` run, every change to a task's `PLAN.md` (an addition, a refactor, a rework
after review), and any plan the user asks for outside a task. The main session talks to
the user, then gives the planner a brief: the goal, the user's decisions, the files in
scope and the plan file path. The planner explores and writes the plan; the main session
shows it to the user. In a task, the main writer and reviewer sessions also run on the
planning model until the user approves the plan (task skill → *Stages*).

## Routing pass

Run it at session start, and re-check on every drift signal below. Do not narrate it.

**Step 1 — gather context. Run the FULL block in one call, every session.** Not just
`pwd`; never skip the `find` because the cwd looks familiar — familiarity is exactly what
the block overrides. Another router having run is NOT this router having run: each router
reaches you independently (a per-request hook, or an always-present instructions file),
so check every router's rules on their own.

```bash
echo "--- cwd ---"; pwd
echo "--- direct files ---"; ls -1 2>/dev/null | head -30
echo "--- project markers (up to 3 levels) ---"; find . -maxdepth 3 \( -name 'go.mod' -o -name 'tsconfig.json' -o -name 'pubspec.yaml' -o -name 'Package.swift' -o -name '*.xcodeproj' \) -not -path '*/node_modules/*' -not -path '*/vendor/*' -not -path '*/.git/*' 2>/dev/null | head -10
echo "--- *_test.go presence (up to 3 levels) ---"; find . -maxdepth 3 -name '*_test.go' -not -path '*/vendor/*' -not -path '*/.git/*' 2>/dev/null | head -3
```

For a `pubspec.yaml`, also run `grep -l '^\s*flutter:' <path>`. Evaluate Step 2 against
the literal output (even if empty), not against what you assume about the project.

**Step 2 — invoke every match, unless already loaded.** Invoke with the harness's skill
tool (reading the `SKILL.md` does not count); the runtime file injected with this router
says how, and how to do anything else mechanical. If no runtime file is in context, say
so rather than guess a tool name. Skill ids are flat (`sadensmol-task`); if one fails to
resolve in Claude Code, retry the colon form `<namespace>:<name>`.

**A. Project routes.** `cat ~/.agents/sadensmol-router-routes.json 2>/dev/null || cat ~/.claude/sadensmol-router-routes.json 2>/dev/null`
(skip if absent). It is a JSON array of `{ "cwd": "<substring>", "skill": "<name>" }`;
invoke each `skill` whose `cwd` substring appears in `pwd`. A match is additive — it never
excuses Sections B–I.

**B. Project type.**

| Signal | Skill(s) |
|---|---|
| `*.go` in cwd, OR `go.mod` anywhere (cwd → 3 levels) | `sadensmol-go-programming` |
| `*.ts` / `*.tsx` in cwd, OR `tsconfig.json` anywhere | `sadensmol-typescript-programming` |
| `pubspec.yaml` with a `flutter:` dependency | `sadensmol-flutter-programming` **and** `sadensmol-dart-programming` |
| `pubspec.yaml` without `flutter:`, or `*.dart` in cwd with no `pubspec.yaml` | `sadensmol-dart-programming` |
| `*.swift` in cwd, OR `Package.swift` / `*.xcodeproj` / `*.xcworkspace` anywhere | `sadensmol-swift-programming` |

**C–G. Prompt intent.**

| Signal | Skill |
|---|---|
| B matched Go AND (the prompt references integration tests, `tests/`, `_integration_test.go`, or a test task pointing at them; OR a `tests/` subdir holds `*_test.go`) | `sadensmol-go-integration-tests` — tests under `tests/` only. Unit tests beside the code, and a generic "test" mention, stay with `sadensmol-go-programming`. |
| `linear`, a `linear.app/…` URL, a `[A-Z]+-\d+` key, or ticket intent (my tasks, assigned to me, in progress, update ticket/issue, add subtask, reassign, change status, mark done, in review) | `sadensmol-linear` (workspace-agnostic; project conventions layer on via project plugins) |
| `jira`, an `*.atlassian.net/…` URL, a key in a Jira project, or Jira ticket intent (move to in progress, sprint, backlog, JQL) | `sadensmol-jira` — load before touching Jira at all, even when it is not set up (the skill owns setup and safety rules); never improvise raw REST `curl` calls |
| A bare `[A-Z]+-\d+` with no other signal | the tracker the project uses; unknown → ask |
| `jetson`, `orin`, JetPack / L4T, `tegrastats`, `runtime: nvidia` on aarch64, `sm_87`, `compose.jetson.yaml`, `make run-jetson`, "on the jetson" / "the board", or the board hostname a project skill defines | `sadensmol-jetson-orin-nano` — before any Jetson deploy, debug or SSH work |
| The stated goal is designing NEW functionality or a LARGE restructuring refactor — never debugging, tests, bug fixes, small edits, review | `sadensmol-programming-patterns` — read `references/patterns-gate.md` first; its negative list wins |
| Database work: a `*.sql` or migration file, schema / table / column / constraint / FK / index, partitioning or pg_partman, writing or tuning a SQL query or query-builder code, an `EXPLAIN` plan, a slow query, transaction scope or row locks | `sadensmol-programming-db` — a project's own DB guide still wins over it |

**H. Git — default-on for code work.** Load `sadensmol-git` before the FIRST `git`
command of the session (read-only ones included), before any bulk automated edit, and when
recovering lost work; its description lists the cases. Not on a turn that never touches
git and never bulk-edits. Rule of thumb: one command touching more files than you could
review in the diff → load it and snapshot first. Independent of language, task type and
`sadensmol-github` (load both when force-pushing a stack).

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

**Step 2.5 — BLOCKING GATE.** Use NO non-skill tool for the user's task (no reading or
grepping code, no reference docs, no edits, no subagents) until every skill the Step 1
output demands is loaded. Re-running Step 1 is fine. Loading a Section A project skill is exactly when
the gate feels done — it is not. Cross-check: `go.mod` → `sadensmol-go-programming`;
`tsconfig.json` → typescript; `pubspec.yaml` with `flutter:` → flutter **and** dart;
Swift markers → swift; `tests/…/*_test.go` + Go + the C intent rule → go-integration-tests.
A missing skill you cannot account for means you skipped a section — go back.

**Step 3** — continue with the request. Do not announce routing.

## Re-route on drift (MUST FOLLOW)

On every later turn, before the actions below, confirm the skill is loaded and load it if
not (no need to re-run Step 1):

- Reading, searching, editing or creating a `*.go` file — `sadensmol-go-programming`.
  "Only reading to understand first" is not an exemption.
- Editing or creating a Go test under `tests/` — `sadensmol-go-integration-tests`.
- `*.ts`/`*.tsx` → typescript; `*.dart`/`pubspec.yaml` → dart (+ flutter); `*.swift`/`Package.swift` → swift.
- Reading or editing a `*.sql` / migration file, or writing or changing a SQL query → `sadensmol-programming-db`.
- The prompt newly matches an intent row (Linear, Jira, Jetson, …) → that skill.
- The task turns into new design or a large restructuring → re-check `references/patterns-gate.md`.
  It runs the other way too: once design becomes debugging, testing or bug fixing, pattern advice stops.
- First `git` command or bulk edit → `sadensmol-git` (H), in every session type.
- A bare colliding name (`/task`, `/linear`, `/router`, …) → resolve per I first.
- Loading a project skill that has a `sadensmol-` base (a direct `<project>-task` too) → base first.

## Rules (apply to every router)

- Skip skills already loaded; no match → no action; no narration; do not ask permission.
- NEVER invoke `superpowers:test-driven-development` for new logic. TDD is for bug fixes
  only (failing test first); new features are implementation-first, then tests — this
  overrides that skill's own trigger. See `sadensmol-go-programming`.
- Before editing any `sadensmol-` skill (this router included), adding a routing rule, or
  when the user calls out a missed skill load, read `references/authoring.md`.
