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
go in **`.scratch/` at the session base** — NOT `/tmp` and NOT the harness temp dir. This
overrides a harness instruction to use its scratchpad. Read `references/scratch-dir.md`
for how to resolve the base, before you create the first temporary file of the session.

## ⚠️ ALWAYS-ON SHOW IN A COMPANION PAGE (every turn, every project)

Anything you show the user to look at or listen to (a report, table, diagram, diff, audio,
draft) goes in a local HTML page, opened as a page companion of this session. Never ask the
user to open a scratch file, an `.md` file or any other path, and never hand over a path
instead of the page. A plain chat answer stays in chat; plannotator gates stay in
plannotator. The no-publishing rule still binds: the page stays local.

```bash
agtermctl session overlay open --html <file> --companion --target "$AGTERM_SESSION_ID" --socket "$AGTERM_SOCKET"
```

No `--follow`. Inside agterm, a failed open is an error: report it. Outside agterm
(`AGTERM_ENABLED` unset), run `open <file>` on the HTML page and say so in chat. Style
the page in the plannotator theme: `sadensmol-task` → `references/plannotator.md` → *Theme*.
A PDF goes through `sadensmol-task` → `scripts/pdf-page.py`, never an iframe/embed/object.

## ⚠️ ALWAYS-ON BROWSER GATE (every turn, every project)

Before the first browser tool call of a task — any URL, Google Sheet / Doc, dashboard, partner
docs — read `references/browser-gate.md` and follow it. Use only the server it selects; never
pick a profile or use another browser tool. If that server is missing, stop and say so. A
generic browser skill cannot override this gate.

## ⚠️ ALWAYS-ON PLANNING MODEL (every turn, every project)

Planning runs in the harness `planner-subagent`, never on the main session's model: a
`task plan` run, every change to a task's `PLAN.md` before the user approves it, and any
plan the user asks for outside a task. Read `references/planning.md` before you start one.

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
echo "--- project markers (up to 3 levels) ---"; find . -maxdepth 3 \( -name 'go.mod' -o -name 'tsconfig.json' -o -name 'pubspec.yaml' -o -name 'Package.swift' -o -name '*.xcodeproj' -o -name 'pyproject.toml' -o -name 'setup.py' -o -name 'requirements*.txt' \) -not -path '*/node_modules/*' -not -path '*/vendor/*' -not -path '*/.git/*' 2>/dev/null | head -10
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
| `*.go` in cwd, OR `go.mod` anywhere (cwd → 3 levels) | `sadensmol-programming-go` |
| `*.ts` / `*.tsx` in cwd, OR `tsconfig.json` anywhere | `sadensmol-programming-typescript` |
| `pubspec.yaml` with a `flutter:` dependency | `sadensmol-programming-flutter` **and** `sadensmol-programming-dart` |
| `pubspec.yaml` without `flutter:`, or `*.dart` in cwd with no `pubspec.yaml` | `sadensmol-programming-dart` |
| `*.swift` in cwd, OR `Package.swift` / `*.xcodeproj` / `*.xcworkspace` anywhere | `sadensmol-programming-swift` |
| `*.py` in cwd, OR `pyproject.toml` / `setup.py` / `requirements*.txt` anywhere | `sadensmol-programming-python` |
| Any row above matched | `sadensmol-programming` too — the language-agnostic principles every language skill builds on |

**C–G. Prompt intent.**

| Signal | Skill |
|---|---|
| B matched Go AND (the prompt references integration tests, `tests/`, `_integration_test.go`, or a test task pointing at them; OR a `tests/` subdir holds `*_test.go`) | read `sadensmol-programming-go` → `references/integration-tests.md` — tests under `tests/` only. Unit tests beside the code, and a generic "test" mention, need only `sadensmol-programming-go`. |
| `linear`, a `linear.app/…` URL, a `[A-Z]+-\d+` key, or ticket intent (my tasks, assigned to me, in progress, update ticket/issue, add subtask, reassign, change status, mark done, in review) | `sadensmol-linear` (workspace-agnostic; project conventions layer on via project plugins) |
| `jira`, an `*.atlassian.net/…` URL, a key in a Jira project, or Jira ticket intent (move to in progress, sprint, backlog, JQL) | `sadensmol-jira` — load before touching Jira at all, even when it is not set up (the skill owns setup and safety rules); never improvise raw REST `curl` calls |
| A bare `[A-Z]+-\d+` with no other signal | the tracker the project uses; unknown → ask |
| `jetson`, `orin`, JetPack / L4T, `tegrastats`, `runtime: nvidia` on aarch64, `sm_87`, `compose.jetson.yaml`, `make run-jetson`, "on the jetson" / "the board", or the board hostname a project skill defines | `sadensmol-jetson-orin-nano` — before any Jetson deploy, debug or SSH work |
| The stated goal is designing NEW functionality or a LARGE restructuring refactor — never debugging, tests, bug fixes, small edits, review | `sadensmol-programming-patterns` — read `references/patterns-gate.md` first; its negative list wins |
| Database work: a `*.sql` or migration file, schema / table / column / constraint / FK / index, partitioning or pg_partman, writing or tuning a SQL query or query-builder code, an `EXPLAIN` plan, a slow query, transaction scope or row locks | `sadensmol-programming-db` — a project's own DB guide still wins over it |
| API work: designing or reviewing an endpoint or contract (paths, methods, status codes, errors, pagination, versioning), or an OpenAPI / Swagger spec — an `api.yaml` / `openapi.yaml` / `swagger.yaml` file, an operation, schema, request body, response or property, its `description`, or a spec that feeds oapi-codegen; a gRPC service or a `*.proto` file | `sadensmol-programming-api` — a project's own API guide still wins over it |

**H. Git — default-on for code work.** Load `sadensmol-git` before the FIRST `git`
command of the session (read-only ones included), before any bulk automated edit, and when
recovering lost work; its description lists the cases. Not on a turn that never touches
git and never bulk-edits. Rule of thumb: one command touching more files than you could
review in the diff → load it and snapshot first. Independent of language, task type and
`sadensmol-github` (load both when force-pushing a stack).

**I. Shared skill names across the personal plugins.** `linear`, `router`, `task` (and any
future collision) exist in more than one plugin. A project plugin **overrides** sadensmol
for a colliding name, layered **on top of** the `sadensmol-` base, which loads first. Read
`references/shared-names.md` before you resolve a bare colliding name or load a project
skill that has a `sadensmol-` base.

**Step 2.5 — BLOCKING GATE.** Use NO non-skill tool for the user's task (no reading or
grepping code, no reference docs, no edits, no subagents) until every skill the Step 1
output demands is loaded. Re-running Step 1 is fine. Loading a Section A project skill is exactly when
the gate feels done — it is not. Cross-check: any language skill → `sadensmol-programming` too; `go.mod` → `sadensmol-programming-go`;
`tsconfig.json` → typescript; `pubspec.yaml` with `flutter:` → flutter **and** dart;
Swift markers → swift; Python markers → python; `tests/…/*_test.go` + Go + the C intent rule → Go's `references/integration-tests.md`.
A missing skill you cannot account for means you skipped a section — go back.

**Step 3** — continue with the request. Do not announce routing.

## Re-route on drift (MUST FOLLOW)

On every later turn, before the actions below, confirm the skill is loaded and load it if
not (no need to re-run Step 1):

- Reading, searching, editing or creating a `*.go` file — `sadensmol-programming-go`.
  "Only reading to understand first" is not an exemption.
- Any language skill loads → `sadensmol-programming` with it.
- Editing or creating a Go test under `tests/` — read `sadensmol-programming-go` → `references/integration-tests.md`.
- `*.ts`/`*.tsx` → typescript; `*.dart`/`pubspec.yaml` → dart (+ flutter); `*.swift`/`Package.swift` → swift; `*.py`/`pyproject.toml` → python.
- Reading or editing a `*.sql` / migration file, or writing or changing a SQL query → `sadensmol-programming-db`.
- Designing an API endpoint, or writing, editing or reviewing an OpenAPI / Swagger spec (`api.yaml`, `openapi.yaml`, `swagger.yaml`) or a `*.proto` file → `sadensmol-programming-api`.
- The prompt newly matches an intent row (Linear, Jira, Jetson, …) → that skill.
- The task turns into new design or a large restructuring → re-check `references/patterns-gate.md`.
  It runs the other way too: once design becomes debugging, testing or bug fixing, pattern advice stops.
- First `git` command or bulk edit → `sadensmol-git` (H), in every session type.
- A bare colliding name (`/task`, `/linear`, `/router`, …) → resolve per I first.
- Loading a project skill that has a `sadensmol-` base (a direct `<project>-task` too) → base first.

## Rules (apply to every router)

- Skip skills already loaded; no match → no action; no narration; do not ask permission.
- **User message mid-turn → answer it NOW (MUST FOLLOW).** When the user sends a message
  while you work, stop every running investigation at once. Make no more tool calls for
  the old work. Answer the new message first, with only the tool calls that the answer
  itself needs. Resume the old work only after the answer, and only if it still applies.
- **Repeated correction → propose the learning.** When the user says `again`, `we
  discussed` or `I told you`, propose the `sadensmol-learn` edit in the same reply. This
  overrides the no-next-step rule. Edit only after the user says apply.
- **Task fresh start.** A prompt from a `task fresh` restart says the context `was cleared
  on purpose` (`Plan discussion:`, `Plan review:`, `Implementation:`, `Finish:`, `Fresh start:`, or a pair
  brief that ends in one). It gets NO routing pass, from any router: load only the skills
  it names, and any other skill only when a step of its work needs it. The next user
  message routes as usual.
- NEVER invoke `superpowers:test-driven-development` for new logic. TDD is for bug fixes
  only (failing test first); new features are implementation-first, then tests — this
  overrides that skill's own trigger. See `sadensmol-programming` → *Testing*.
- **Before creating or editing ANY skill, in any namespace** (`sadensmol-*` or a project's
  `<project>-*`), adding a hook, a shared name or a routing rule: load `sadensmol-sadensmol`
  (MUST FOLLOW) — it decides which layer the change belongs to. For the router itself, or
  when the user calls out a missed skill load, also read `references/authoring.md`.
