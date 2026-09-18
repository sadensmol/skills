---
name: sadensmol-router
description: "Sadensmol skill router — detects working context (cwd, project type, prompt intent) and invokes the right downstream skills (go-programming, typescript-programming, flutter-programming, dart-programming, swift-programming, go-integration-tests, programming-patterns, git, linear, jira, plus any project-specific skills configured locally). Also disambiguates ALL shared skill names (`linear`, `router`, `task`, and any future collision) across the personal plugins by workspace: a project (work_) plugin overrides sadensmol for a colliding name while layering on sadensmol's base, so defer to the owning project workspace and use the `sadensmol-` version only for a plain personal session. Use IMMEDIATELY when the UserPromptSubmit hook tells you to, or at the start of any session where personal/work skills might apply. Skips skills already loaded in the conversation."
---

# Sadensmol skill router

You were invoked because a new session started (or the user changed working context). Your job is to detect the user's situation and load the appropriate downstream skills, so project-specific conventions are in place BEFORE you do real work.

## ⚠️ ALWAYS-ON WORKTREE CONFINEMENT (applies every turn, not just at load)

Because this router text is in context on every turn, it carries the one rule that
is most often violated across ALL projects: **if the session cwd is under
`…/.worktrees/<branch>/`, then EVERY path you touch and EVERY command's working dir
for the WHOLE session MUST stay under that worktree base. NEVER `cd` to or edit a
primary checkout — a repo path with no `/.worktrees/<branch>/` segment (e.g.
`<project-root>/<repo>/…`) is a different branch (often the default
branch or a stranger's WIP), so your work silently lands in the wrong place and the
task branch/PR shows nothing.** This binds ALL work (edits, `git`, builds, `make`,
`sops`, grep), not only the `task` flow, and regardless of which downstream skill is
loaded. The harness constantly surfaces primary-checkout paths that look
authoritative — ignore them; trust the worktree cwd. Derive it once:
`case "$(pwd)" in */.worktrees/*) root="${PWD%%/.worktrees/*}"; b="${PWD#*/.worktrees/}"; b="${b%%/*}"; WT="$root/.worktrees/$b";; esac`
and prefix every repo path with `$WT`. Full mechanism + recovery live in each
project's dev skill and `:task` skill.

## ⚠️ ALWAYS-ON SCRATCH DIR — temp files go in a worktree-local `.scratch/` (applies every turn)

When you need to write a temporary/working file (generated SQL, a scratch
script, intermediate data, a report you'll hand to the user), put it in a
**`.scratch/` directory at the session base**, NOT in the harness temp dir
(`/private/tmp/claude-…/…/scratchpad`) and NOT in `/tmp`. The harness path is
deep, ephemeral, and undiscoverable — the user can't find files there.

Resolve the base once from cwd and `mkdir -p` the dir:

```bash
cwd="$(pwd)"
case "$cwd" in
  */.worktrees/*) root="${cwd%%/.worktrees/*}"; b="${cwd#*/.worktrees/}"; b="${b%%/*}"
                  base="$root/.worktrees/$b" ;;   # worktree base (a non-git dir → .scratch/ never committed)
  *) base="$cwd" ;;                                # not in a worktree: current project / agterm session cwd
esac
mkdir -p "$base/.scratch"
```

- **Worktree sessions:** `<worktree-base>/.scratch/` — the base holds the nested
  repos and is itself outside every git repo, so nothing there gets committed.
- **Non-worktree sessions:** `.scratch/` at the current project root / agterm
  session cwd. If that root IS a git repo, add `.scratch/` to its `.gitignore`
  (or `.git/info/exclude`) so scratch files never get staged.
- Give the user the **clean `.scratch/…` path** when handing off a file, never
  the `/private/tmp/claude-…` one.
- The harness may still instruct "use this scratchpad directory" — this rule
  overrides it: prefer `.scratch/` at the session base so files stay discoverable.

## How to execute

Run this routing pass at the start of the session AND re-check it on every later turn that introduces a new signal (see "Re-route on drift" below). Do NOT narrate it to the user — just do the work and proceed with their request afterwards.

### Step 1 — gather context (MANDATORY, do not shortcut)

**You MUST run the FULL bash block below in a single Bash call. Do not replace it with just `pwd`. Do not skip the `find` because cwd looks unfamiliar. Section B's project-type detection cannot fire without this output, and skipping it is the #1 source of "you forgot to load `go-programming`" bugs.**

**Concrete failures (these have happened — don't repeat):**

1. *"I recognize this cwd, I'll skip the find."* At session start, ran only `pwd` + `cat ~/.agents/sadensmol-router-routes.json`. Section A matched a project-specific skill (a `<project>-<project>` dev skill) and routing was declared done. Section B never got the `find` output it needed, so `sadensmol-go-programming` and `sadensmol-go-integration-tests` were never loaded. Eight turns of Go editing happened before the user caught the gap. **Rule:** prior knowledge of the cwd is exactly what the bash block exists to override. Familiarity is not a substitute for evidence — run the full block every time.

2. *"Some other router already ran, so I'm routed."* Another skill's router was invoked from its own hook and loaded its project skill. The main loop then treated the routing hooks as collectively satisfied and **never invoked `sadensmol-router` at all** — so Section B never ran and `sadensmol-go-programming` / `sadensmol-go-integration-tests` were never loaded. A whole task's worth of Go was written missing the language rules (`fmt.Sprintf` over `+`, comment discipline, mapper pattern) before the user caught it. **Rule:** each router reaches you independently — as a per-request hook in Claude Code, and as an always-present instructions file in OpenCode — so a *different* router having run is NOT this router having run. Check each router's rules independently and load any skill that isn't loaded. Never assume one router covers another; routers know nothing about each other.

```bash
echo "--- cwd ---"; pwd
echo "--- direct files ---"; ls -1 2>/dev/null | head -30
echo "--- project markers (up to 3 levels) ---"; find . -maxdepth 3 \( -name 'go.mod' -o -name 'tsconfig.json' -o -name 'pubspec.yaml' -o -name 'Package.swift' -o -name '*.xcodeproj' \) -not -path '*/node_modules/*' -not -path '*/vendor/*' -not -path '*/.git/*' 2>/dev/null | head -10
echo "--- *_test.go presence (up to 3 levels) ---"; find . -maxdepth 3 -name '*_test.go' -not -path '*/vendor/*' -not -path '*/.git/*' 2>/dev/null | head -3
```

If a `pubspec.yaml` was found, also check whether it pulls in Flutter:

```bash
grep -l '^\s*flutter:' <path-to-pubspec.yaml>
```

**Pre-flight checklist before moving to Step 2 — every box MUST be ticked:**

- [ ] I ran the full bash block (cwd + direct files + project markers + *_test.go presence).
- [ ] I have the literal `find` output in front of me (even if empty).
- [ ] I am now going to evaluate Section B against that output, not against my prior assumption about the project.

If you cannot tick all three, go back and run the block.

### Step 2 — apply routing rules

#### How to invoke, in whichever harness you are running

This skill and every skill it names are shared by several harnesses. Skill ids are
flat and namespace-prefixed (`sadensmol-router`, `sadensmol-task`), never
`namespace:name`.

**How to invoke, and how to do anything else mechanical, is stated once per harness
in the runtime file that was injected alongside this router** — Claude Code's arrives
with this hook, OpenCode's through its `instructions` list. It answers `load-skill`,
`ask-user`, `track-todos`, `run-background-shell`, `spawn-subagent`, `fan-out`,
`background-subagent`, `stop-subagent` and `isolate-work`. When a skill below says
"invoke", "fan out" or "run in the background", that file says what it means where you
are. If no runtime file is in context, say so rather than guessing a tool name.

One transitional exception: a namespace still installed as a Claude Code plugin keeps
its colon id there (`<namespace>:<name>`). If a flat id fails to resolve in Claude
Code, retry with the colon form before giving up.

Evaluate the rules below against the context. **For each match, invoke the listed skill — UNLESS it is already loaded in this conversation, in which case skip it.**

#### A. Project-specific skills (local config — no project names hardcoded here)

Work/project skills live in separate, private plugins. Their `cwd → skill` routes are kept in a **local, unpublished** config so no project names ever appear in this published skill.

Read the routes (skip this section if the file is absent):

```bash
cat ~/.agents/sadensmol-router-routes.json 2>/dev/null || cat ~/.claude/sadensmol-router-routes.json 2>/dev/null
```

`~/.agents/` is the canonical location because every harness can read it; the
`~/.claude/` copy is a fallback for a Claude-Code-only machine.

The file is a JSON array of `{ "cwd": "<substring>", "skill": "<skill-name>" }`. For each entry whose `cwd` substring appears in the current `pwd`, invoke its `skill` — unless already loaded.

#### B. Project-type detection

| Signal | Skill(s) to invoke |
|---|---|
| `*.go` in cwd, OR `go.mod` anywhere (cwd → 3 levels deep) | `sadensmol-go-programming` |
| `*.ts` / `*.tsx` in cwd, OR `tsconfig.json` anywhere | `sadensmol-typescript-programming` |
| `pubspec.yaml` exists AND contains a `flutter:` dependency | `sadensmol-flutter-programming` **and** `sadensmol-dart-programming` |
| `pubspec.yaml` exists WITHOUT `flutter:` | `sadensmol-dart-programming` |
| `*.dart` in cwd and no `pubspec.yaml` matched above | `sadensmol-dart-programming` |
| `*.swift` in cwd, OR `Package.swift` / `*.xcodeproj` / `*.xcworkspace` anywhere | `sadensmol-swift-programming` |

#### C. Intent-based (Go integration tests)

`sadensmol-go-integration-tests` covers **integration tests only**, which in this codebase means tests living **under a `tests/` directory**. Unit tests next to the implementation (e.g. `service/foo.go` + `service/foo_test.go`) are covered by the unit-test section of `sadensmol-go-programming` — do NOT load `go-integration-tests` for them.

If `sadensmol-go-programming` was matched in section B **AND** any of:

- The user's most recent prompt explicitly references integration tests or the `tests/` folder (look for: `integration test`, `tests/`, `_integration_test.go`, or `test`/`run test`/`fix test` in a context that points at integration tests)
- A `tests/` subdir contains any `*_test.go` (within 3 levels) — i.e. the project ships integration tests under `tests/`

→ invoke `sadensmol-go-integration-tests`

**Do NOT trigger on:**

- The presence of `*_test.go` files directly in cwd or alongside implementation (those are unit tests).
- A generic "test" mention with no integration-test signal.

#### D. Intent-based (Linear)

If the user's prompt mentions Linear in any form:

- the literal word `linear` or `Linear`
- a `linear.app/...` URL
- a Linear issue identifier matching `[A-Z]+-\d+` (e.g. `PROJ-XXX`, `ENG-XXX`)
- task-management intent in a ticketing context: `my tasks`, `assigned to me`, `in progress`, `update ticket`, `update issue`, `add subtask`, `reassign`, `change status`, `mark done`, `in review`

→ invoke `sadensmol-linear` (workspace-agnostic mechanics).

Project-specific Linear conventions (team names, status names, branch mapping, task-title format, description style) live in plugin skills, not here. The local routes file (Step A) and the per-plugin routers (`<project>-router`) handle layering those on top — `sadensmol-linear` stays workspace-agnostic.

#### E. Intent-based (Jira)

If the user's prompt mentions Jira in any form:

- the literal word `jira` or `Jira`
- an `*.atlassian.net/...` URL
- an issue key matching `[A-Z]+-\d+` in a project whose tracker is Jira (the project's own skill says which tracker it uses)
- ticketing intent (`update ticket`, `move to in progress`, `sprint`, `backlog`, JQL) in a Jira context

→ invoke `sadensmol-jira` (REST-only: drives the Jira Cloud REST API via `curl`,
no CLI or MCP).

**Why load it BEFORE touching Jira at all:**

- It defines the exact REST recipes (correct endpoints, API v2-vs-v3, JQL
  search, the two-step transition flow) — without it you'll guess the wrong
  curl calls.
- It encodes the safety rules (fetch issue before transitioning, assign by
  `accountId` not display name, show original description before editing,
  approval before any modification, never print `$JIRA_API_TOKEN`) that
  prevent irreversible Jira mistakes and token leaks.
- It carries the **First-Time Setup** flow: if the `JIRA_BASE_URL` /
  `JIRA_EMAIL` / `JIRA_API_TOKEN` env vars are missing, the skill walks the
  user through token creation, with secrets added by the user to
  `~/.profile`. So load it **even when Jira isn't set up yet** — "Jira isn't
  configured" is itself a case the skill handles; never improvise raw `curl`
  calls against the Jira REST API without loading the skill first.

If the prompt is ambiguous between Linear and Jira (a bare `[A-Z]+-\d+` identifier with no other signal), prefer the tracker the current project uses; when unknown, ask.

#### F. Intent-based (Jetson / NVIDIA Jetson)

If the user's prompt mentions a Jetson board in any form:

- the word `jetson`, `nvidia jetson`, `orin` (Orin Nano / Orin NX), or the board hostname the project skill defines
- JetPack / L4T, `tegrastats`, `runtime: nvidia` on an aarch64 board, or a Jetson iGPU (`sm_87`)
- deploying to / debugging / SSH-ing the physical board (`compose.jetson.yaml`, `make run-jetson`, "on the jetson", "the board")

→ invoke `sadensmol-jetson-orin-nano` (board connect + debug recipe, JP7.2 GPU/LLM findings, bundled NVIDIA device skills). Load it BEFORE touching any Jetson deploy/debug work — it carries the "don't wedge the 8 GB box" rules and the SSH recipe.

#### G. Intent-based (programming patterns) — NARROW TRIGGER, READ BOTH LISTS

`sadensmol-programming-patterns` is a **design-time** skill. It is the only
sadensmol skill with an explicit *negative* trigger list, and the negative list
wins over the positive one whenever both look like they match.

**Invoke it ONLY when the user's request is one of these two things:**

1. **Designing NEW functionality that does not exist yet** — a new feature, a new
   service, a new subsystem, a new module, a new public interface, a new domain
   model, a greenfield component. Signals: `design`, `let's build`, `new
   feature`, `new service`, `how should we structure`, `architecture for`, `plan
   the implementation of`, `add <capability> from scratch`.
2. **A LARGE refactoring that restructures existing code** — extracting a layer,
   breaking up a god object, changing how components fit together, untangling a
   module, replacing an architecture. Signals: `big refactor`, `restructure`,
   `rewrite this module`, `split this up`, `extract a layer`, `this is a mess,
   redo it`, `clean up this architecture`.

**NEVER invoke it for any of these — this list is absolute:**

- Debugging, reproducing, or root-causing anything (→ `superpowers:systematic-debugging`)
- Writing, fixing, or running tests of any kind (→ the language skill / `sadensmol-go-integration-tests`)
- Fixing a bug or patching a defect, even a structural-looking one
- Ordinary feature work on existing code: a new field, a new branch, a new
  endpoint modelled on the one beside it, a small local edit
- Code review (→ `sadensmol-code-review`)
- Reading, exploring, or explaining existing code
- Anything where the answer is "change these three lines"

**Tie-break rule:** if the request is *both* ("refactor this while fixing the
bug"), the fix is the task — do NOT load the skill. Load it only when
restructuring is the stated goal on its own.

**Scale test before loading:** a refactor qualifies only if it moves code across
type/package/layer boundaries, or changes an interface other code depends on.
Renames, extracting one helper, and tidying one function do not qualify.

Why the trigger is this narrow: the skill proposes structure. During debugging,
testing, and bug fixing, the existing code's shape is a **constraint you
inherit**, not a decision you get to make — pattern advice there converts a
bounded task into a rewrite. Loading it is not harmless.

Note: the skill itself requires announcing every candidate pattern to the user
and getting an explicit yes before applying it. Do not shortcut that gate.

For system-level topology (service decomposition, deployment, scaling, ADRs)
load `sadensmol-system-design` instead — that boundary is documented in both
skills.

#### H. Default-on for code work (git) — load before the FIRST git command

In any code-related project, `sadensmol-git` is **default-on**. Load it **before
the first `git` command of the session**, whatever that command is — including a
read-only one like `git status`, `git diff`, `git log`, or `git branch`. Do not
wait for a destructive command: by the time you recognise one, you may already be
mid-sequence, and the snapshot has to exist *before* the damage.

Load it before the first of any of these:

- **Any `git` invocation at all** in a code project — `status`, `diff`, `log`,
  `branch`, `switch`, `stash`, `rebase`, `reset`, `clean`, `push`, everything.
- **Any BULK automated edit**, even though it is not git: mass `sed -i`, a codemod,
  a scripted rename, a formatter/linter `--fix` across the tree, regenerating
  generated code over hand-edited files, `find … -exec` rewrites, deleting or
  moving a directory.
- **Recovering work that is already lost** — stash list, reflog, dangling commits.

**Do NOT load it when git never comes up.** A conversational turn — explaining
code, answering a question, reading a file, planning, discussing an approach —
needs no git skill. The trigger is *touching git or rewriting files in bulk*, not
*being in a repo*. No `git` command and no bulk edit → do not load it.

**Rule of thumb:** if one command touches more files than you could review in the
diff, load the skill and snapshot first.

This is orthogonal to every other section — it fires regardless of language,
project, or task type, including during debugging and bug fixing. It is also
independent of `sadensmol-github`, which covers `gh` and stacked PRs; load both
when force-pushing a stack.

#### I. Shared skill names across the personal plugins (bare-name disambiguation)

`linear`, `router` (and, in the project plugins, `task`) are skill names that
exist in more than one personal plugin, so a bare `/<name>` (or `<name> <args>`
typed as plain text) is ambiguous and the harness may tie-break to the wrong one.
This is **not `linear`-specific** — the rule below covers any current or future
shared name. Resolve by the session's workspace, writing `<project>` for the
plugin that owns it:

| Bare name | project session | plain personal / other |
|---|---|---|
| `task` | `<project>-task` | — (sadensmol has no `task`) |
| `linear` | `<project>-linear` if it defines one (+ `sadensmol-linear` base), else `sadensmol-linear` | `sadensmol-linear` |
| `router` | `<project>-router` | `sadensmol-router` |

**Rule (general — any shared name), with precedence:** a project (work_) plugin
**overrides** sadensmol for a colliding name, but builds **on top of** sadensmol's
base. So:

- If the session belongs to a project workspace whose plugin defines the name →
  that namespace's version wins; defer to its router (it is delivered on its own,
  by the same mechanism as this one). When the project skill *layers on* the sadensmol base (e.g.
  `<project>-linear` over `sadensmol-linear`), **both** load — sadensmol supplies
  base mechanics, the project skill supplies the overrides.
- Otherwise (a plain personal / sadensmol session, or the owning project plugin
  doesn't define the name) → the `sadensmol-` version wins; if the harness
  pre-loaded another plugin's `<name>`, override to `sadensmol-<name>`.

The fully-qualified `/<plugin>:<name>` always targets the right one.

### Step 2.5 — verify invocations against Step 1 output (BLOCKING GATE)

**This is a hard gate, not a "cross-check you'll get to."** You MUST NOT use ANY
non-skill tool for the user's task — no `Read`/`Grep`/`Glob`/`Bash` to explore
code, no reading a reference doc, no `Edit`/`Write`, no `Agent` — until you have
run this verification and every required skill below is loaded. Loading a
Section A project skill does **not** open the gate; only completing this list
does. (Re-running the Step 1 bash block is fine — that's part of routing.)

Cross-check the skills you invoked in Step 2 against the `find` output from Step 1. If the output shows:

- a `go.mod` (anywhere up to 3 levels deep) → `sadensmol-go-programming` MUST be in your invocation list (or already loaded).
- a `tsconfig.json` → `sadensmol-typescript-programming` MUST be loaded.
- a `pubspec.yaml` with `flutter:` → both `sadensmol-flutter-programming` and `sadensmol-dart-programming` MUST be loaded.
- a `Package.swift` / `*.xcodeproj` / `*.xcworkspace` (or `*.swift` in cwd) → `sadensmol-swift-programming` MUST be loaded.
- any `tests/.../*_test.go` AND a Go project AND Section C's intent rule fires → `sadensmol-go-integration-tests` MUST be loaded.

Say the quiet part to yourself before the first task tool-call: *"Step 1 showed
`go.mod` — is `sadensmol-go-programming` loaded? If not, load it now."* Substitute
the matching language for ts/dart/flutter projects.

**A Section A match (project-specific skill) is additive, not a substitute** — it never excuses skipping Section B/C/D. If you can't account for a missing skill against the Step 1 output, you skipped a section. Go back to Step 2 before proceeding.

**Concrete failure (this happened — don't repeat):** Step 1 ran fully and showed
`go/**/go.mod`. Section A matched a project skill (a `<project>-<project>` dev skill)
and it was loaded. Then, *without* completing Step 2.5, the next move was to read
the project skill's reference doc and `grep` across `.go` files to plan a Go
refactor — `sadensmol-go-programming` was never loaded. The user caught it. **Rule:**
loading the project skill is exactly the moment the gate feels "done" — it isn't.
The `find` output already proved a Go project; Section B is mandatory regardless
of what Section A matched, and reading/grepping `.go` files IS "real work" that
the gate blocks (see Re-route on drift).

### Step 3 — proceed with the user's request

Once matched skills are invoked (or confirmed already-loaded), continue with whatever the user actually asked for. Do not announce that routing happened.

## Re-route on drift (MUST FOLLOW)

The initial pass is not "once and forget". Re-evaluate Sections B, C, D, G, H against every later turn — at minimum, **before** any of the following actions:

- **Reading, searching, editing, or creating a `*.go` file for the user's task** — `grep`/`Glob` over Go sources, opening a `.go` file to plan a change, or any `Edit`/`Write` — confirm `sadensmol-go-programming` is loaded; load it if not. "I'm only reading to understand first" is NOT an exemption: the skill tells you *how* to read Go here and what patterns to expect, so it must be loaded before the first `.go` read, not after.
- **Editing or creating a Go test file under a `tests/` directory** (e.g. `tests/internal-api/v1/foo_test.go`, `tests/repository/bar.go`) — confirm `sadensmol-go-integration-tests` is loaded; load it if not. Unit tests next to the implementation (`service/foo_test.go`) are NOT covered by this skill — they fall under `sadensmol-go-programming`.
- **Editing or creating a `*.ts` / `*.tsx` file** — confirm `sadensmol-typescript-programming` is loaded.
- **Editing or creating a `*.dart` file (or `pubspec.yaml`)** — confirm `sadensmol-dart-programming` (and `sadensmol-flutter-programming` if Flutter) are loaded.
- **Editing or creating a `*.swift` file (or `Package.swift`)** — confirm `sadensmol-swift-programming` is loaded.
- **The user's prompt newly mentions Linear / an ENG-#### identifier / a `linear.app` URL** — confirm `sadensmol-linear` is loaded.
- **The user's prompt newly mentions Jira / an `atlassian.net` URL / a Jira issue key** — confirm `sadensmol-jira` is loaded.
- **The user's prompt newly mentions a Jetson / NVIDIA Jetson / Orin / JetPack / `compose.jetson.yaml` / the board** — confirm `sadensmol-jetson-orin-nano` is loaded; load it before any Jetson deploy/debug/SSH work.
- **The task turns into designing NEW functionality, or a LARGE restructuring refactor** — re-run Section G's two lists and load `sadensmol-programming-patterns` only if the positive list matches AND nothing on the negative list does. Drift also runs the other way: a session that started as design and became debugging, test writing, or bug fixing must NOT gain this skill, and any pattern advice already in flight stops there.
- **You are about to run your FIRST `git` command of the session** — any git command, including read-only ones (`status`, `diff`, `log`, `branch`) — **or any bulk automated edit** (mass `sed -i`, a codemod, a tree-wide `--fix`, deleting a directory) — confirm `sadensmol-git` is loaded first, per Section H. This fires in every session type, including debugging and bug fixing. It does NOT fire on a turn that never touches git.
- **The user invokes a bare colliding skill name (`/task`, `/linear`, `/router`, or any name shared across the personal plugins)** — resolve it per Section I by the session's workspace (project plugin overrides sadensmol, layering on its base) before invoking, and override any wrong-plugin version the harness pre-loaded.

You don't need to re-run the full Step 1 bash block on drift — just check whether the skill the new action would benefit from is already loaded, and load it if it isn't. **Invoke it with your harness's skill tool (never just read the SKILL.md file).** Loading a skill on drift is cheaper than producing code that violates its rules.

## Rules

- **Skip already-loaded.** Re-invoking a loaded skill is wasted tokens and noise.
- **Initial pass is mandatory; later drift checks are mandatory too.** Step 1's bash block runs at session start; Section B/C/D/G/H rules then re-fire on every drift signal listed above.
- **No matches → no action.** Silently move on to the user's request.
- **No narration.** Don't tell the user "Loaded skills X, Y, Z." Just do the work.
- **Don't ask permission.** The user has already opted into this via the hook.
- **`sadensmol-` skills are the generic layer — project detail lives in the project plugin (MUST FOLLOW).** This repository is public, so **no `sadensmol-` skill may hardcode a machine-, user- or project-specific value**: absolute checkout paths (`/Users/<name>/…`, `/home/<name>/…`), home directories, tool install locations, tracker sites, team/issue prefixes, or per-project layout. When such a value is needed, get it in this order: **(1) derive it at runtime** (from the cwd, `$HOME`, `command -v <tool>`), **(2) read it from an env var** (with an override taking precedence, e.g. `${PLANNOTATOR:-…}`), **(3) take it from the project skill** — the `<project>-*` plugin loaded above is the single place project detail is written down, so defer to it, **(4) ask the user.** Never guess a path. When you catch a hardcoded value in a `sadensmol-` skill, replace it with one of those four — do not just work around it.
- **If the user calls out a missing skill load, fix the rule that missed it.** Don't just load it and move on — edit this `SKILL.md` so the same hole doesn't reopen in the next session.
- **`sadensmol-git` is default-on for code work — load it before the FIRST git command, read-only ones included (Section H).** Waiting until a destructive command is recognised is too late; the snapshot must exist before the damage, and the reflog does not cover uncommitted or untracked files. The one exception: a turn that never touches git and never bulk-edits files (pure discussion, reading, planning) — do not load it there.
- **NEVER invoke `sadensmol-programming-patterns` outside design or big-refactor work.** Its trigger is deliberately narrow (Section G). Debugging, test work, bug fixing, small edits, and code review are hard exclusions — "the code looks badly structured" is not a trigger while fixing it. Loading it costs the user unrequested restructuring proposals in the middle of a bounded task.
- **NEVER invoke `superpowers:test-driven-development` for new logic.** TDD is for **bug fixes only** (the behaviour exists and misbehaves → failing test first). New features/methods/endpoints are **implementation-first**: write the code, then cover it with tests. "A skill exists, so I must load it" is not a reason — the skill's own trigger ("use when implementing any feature") is overridden by this rule. See the *Test-first vs implementation-first* section in `sadensmol-go-programming`.

## Adding new routing rules

Edit this file directly — it lives in the skills repository at `skills/router/SKILL.md`, and every harness reads it through a symlink, so the edit is live in the next session. To add a new skill route:

1. Pick a signal (cwd pattern, file existence, prompt keyword).
2. Add a row to the appropriate table above.
3. Save. Changes take effect in the next session that invokes this skill.
