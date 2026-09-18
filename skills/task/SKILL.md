---
name: sadensmol-task
description: "Single entry point for project task/worktree management across the personal plugins. Dispatches `/task <subcommand>` to the correct project's task skill (`<project>-task`) based on the active agterm workspace (authoritative) or the cwd. Use when the user says: \"task new <branch>\", \"new task\", \"task plan\", \"task list\", \"task switch\", \"task changes\", \"show changes\", \"task zed\", \"task code\", \"task console\", \"task review\", \"task finish\", \"task cleanup\", \"create worktree\", or a bare \"/task …\". Forwards all arguments verbatim to the resolved project skill — it never runs task work itself."
---

# Task (dispatcher)

`/task` is the **single visible** task entry across the personal plugins. The
per-project implementations (`<project>-task`) are hidden from the `/` menu via
`user-invocable: false` and are reached **only** through this dispatcher (or by
the model directly, since `Skill(<plugin>-task)` still works on a hidden skill).

**This skill knows NOTHING about any specific project (MUST FOLLOW).** No project
name, repo name, workspace name, path, tracker or issue prefix ever appears here —
it is published, generic, and resolves everything at runtime (Step 2). Whatever it
handles itself, it handles **generically**, from the cwd alone. Anything that needs
repository knowledge is **passed down** to the project skill, never reimplemented
here. If you find yourself wanting to special-case a project in this file, that
logic belongs in that project's plugin.

Your job: detect which project this session belongs to, then invoke that
project's `-task` skill via your harness's skill tool, **forwarding the user's arguments
unchanged**. Do NOT run any worktree/git/plan/finish work yourself — the
**project-specific** subcommands (`new`, `plan`, `finish`, `cleanup`) live in the
project skill.

**Handled here, NOT delegated:** `zed`, `code`, `console`, `review`, `changes`,
`list`, `switch`. These are project-agnostic — opening the worktree in an editor
(`zed`/`code`) or terminal (`console`), opening plannotator at the worktree base
(`review` for real code changes, `annotate` for a plan/docs `.md`), walking the
task's business-logic diffs in chat (`changes`), and
enumerating (`list`) / picking (`switch`) worktrees are
identical across projects (only the single-repo vs multi-repo shape differs, and
plannotator ≥ 0.21 auto-discovers nested repos). So this dispatcher runs them
itself — with a one-time VS Code trust preflight for `code`, and `switch`
delegating only its final "open" step back to the project's `new`. See the
"handled here" sections below.

## Step 1 — detect the project (agterm workspace decides first)

Run this single Bash call:

```bash
echo "--- agterm workspace ---"
if [ -n "${AGTERM_WORKSPACE_ID:-}" ] && command -v agtermctl >/dev/null 2>&1; then
  agtermctl tree --json 2>/dev/null | python3 -c "import sys,json,os;d=json.load(sys.stdin);w=os.environ.get('AGTERM_WORKSPACE_ID');print(next((x.get('name') for x in d.get('result',{}).get('tree',{}).get('workspaces',[]) if x.get('id')==w),'(unknown)'))" 2>/dev/null || echo "(unknown)"
else
  echo "(not in agterm)"
fi
echo "--- cwd ---"; pwd
```

The **agterm workspace name**, when present, is *authoritative* — a session lives
under a workspace named after its project, which stays correct even when the cwd
has wandered (e.g. editing the project's skill sources under a `work_<project>`
checkout rather than the project itself). Prefer it over the cwd path.

## Step 2 — resolve to a project skill

Resolution is **data-driven** — no project is named in this file. You are looking
for one thing: the **plugin namespace** `<project>`, which then gives you
`<project>-task`. Check in priority order (workspace wins; the cwd is the fallback
when the workspace is `(unknown)` / `(not in agterm)`):

**1. agterm workspace name.** A workspace is named after its project, and the
plugin carries that same name — so workspace `<name>` resolves directly to
`<name>-task`. No lookup needed.

**2. cwd, via the local routes file.** The same **local, unpublished** config the
router uses maps cwd substrings to project skills, so project names live there and
never here:

```bash
pwd | sed 's|/work_|/|g'                          # normalize a work_<project> skill-source checkout
cat ~/.agents/sadensmol-router-routes.json 2>/dev/null || cat ~/.claude/sadensmol-router-routes.json 2>/dev/null
```

It is a JSON array of `{ "cwd": "<substring>", "skill": "<plugin>:<skill>" }`. Take
the entry whose `cwd` substring appears in the **normalized** path; its plugin
namespace (the part before `:`) is `<project>`. The `sed` normalization is what
makes a `work_<project>` checkout — where you edit the project's skill sources
rather than the project — resolve to the same plugin, without this file knowing any
project's name.

**3. No match → STOP.** A plain personal session, an unrecognized workspace, a
resolved namespace with no `-task` skill (the invocation fails), or a route that
resolves to the **`sadensmol` namespace itself** (that is this dispatcher — never
dispatch to it, you would loop) all mean the same thing: `/task` needs to run inside
a project workspace, and no personal task flow exists. Say so and ask which project
— never guess one.

### Project root — derived from the cwd, never hardcoded (MUST FOLLOW)

`list` / `switch` operate on the **project root** (the directory that holds
`.worktrees/`). Derive it from the cwd — that is the only source; this skill knows
no paths and asks no one for any:

```bash
cwd="$(pwd)"; root=""
case "$cwd" in */.worktrees/*) root="${cwd%%/.worktrees/*}" ;; esac
if [ -z "$root" ]; then                      # not in a worktree: walk up to the dir holding .worktrees/
  d="$cwd"
  while [ "$d" != "/" ]; do
    [ -d "$d/.worktrees" ] && { root="$d"; break; }
    d="$(dirname "$d")"
  done
fi
echo "${root:-NO_ROOT}"
```

On `NO_ROOT` the session is simply outside the project tree: tell the user to run
`list` / `switch` from inside the project (or any of its worktrees) and stop. Never
guess a path, and never ask a skill to hand you one.

## Step 3 — delegate, or handle here

**Handled here (do NOT delegate):** `zed`, `code`, `console`, `review`,
`changes`, `list`, `switch` — see the "handled here" sections below.
`zed`/`code`/`console`/`review`/`changes`
work off the cwd (the current worktree); `list`/`switch` use the **project root**
from Step 2.

**Delegated to the project skill:** everything else — `new`, `plan`, `finish`,
`cleanup`. Invoke the resolved skill, passing the **full
argument string** the user gave `/task` (subcommand + args) **verbatim** as the
skill's args. Examples:

- `/task new ABC-XXX ABC-YYY` → invoke `<project>-task` with args `new ABC-XXX ABC-YYY`
- `/task finish` → invoke `<project>-task` with args `finish`
- bare `/task` (no args) → invoke the resolved skill with no args (it defaults to `new` per its own rules)

Do not re-interpret, expand, or execute a **delegated** subcommand yourself. The
project skill owns `new` / `plan` / `finish` / `cleanup`; `zed` / `code` /
`console` / `review` / `changes` / `list` / `switch` are handled here.

**A base branch on `new` is forwarded, never interpreted.** `--base <branch>` — or
the prose form ("based on `X`", "on top of `X`", "stacked on `X`") — goes to the
project skill **verbatim**, along with everything else in the argument string. It
means "start this task off another task's branch, and open a stacked PR for it".
Whether that is supported, which repos it applies to, and how the PR is stacked are
the project skill's call:

- `/task new abc-2-foo --base abc-1-foo` → invoke `<project>-task` with args
  `new abc-2-foo --base abc-1-foo`

A stacked task ends in a **registered GitHub stack**, not merely a `--base`
chain — the project's `finish` owns that step
(`POST /repos/{owner}/{repo}/stacks`, ordered bottom-to-top). See
`sadensmol-github` → "Stacked Pull Requests". Do not create or register a stack
from this dispatcher; just forward the base branch.

Do not resolve it to a PR, a SHA, or `origin/<branch>` here, and do not drop it
because it looks like stray prose.

**Issue-tracker URL as the branch argument — resolve to the tracker's REAL
branch name, never the URL slug (MUST FOLLOW).** When `/task new <url>` gets a
Linear (or similar) issue URL, do NOT derive the branch by slugifying the URL
path: issue titles use `|` separators, which the URL slug renders as `-or-`
junk (`.../PROJ-XXX/be-or-acme-integration-or-inbound-...` →
`proj-xxx-be-or-acme-integration-or-...` — wrong). Use the issue's canonical
`gitBranchName` instead: fetch the issue via the tracker skill
(`get_issue`/`save_issue` responses include `gitBranchName`, e.g.
`proj-xxx-be-acme-integration-inbound-session-auth`) and pass
THAT to the project skill. Deriving from the slug creates a second, differently
named worktree for the same ticket the moment anyone uses the real branch name.

## `zed` / `code` — handled here (not delegated)

**Triggers:** "task zed" / "open zed"; "task code" / "open code" / "open vscode".

Both just open the current worktree in an editor; the logic is identical across
projects, so the dispatcher runs it directly. `code` additionally does a
one-time **VS Code trust preflight** so worktrees never open in Restricted Mode.

First derive the worktree base **and** the trust root (the parent of
`.worktrees/`) from the cwd — this is project-agnostic:

```bash
cwd="$(pwd)"
case "$cwd" in
  */.worktrees/*)
    root="${cwd%%/.worktrees/*}"                  # the project root (dir holding .worktrees/)
    branch="${cwd#*/.worktrees/}"; branch="${branch%%/*}"
    base="$root/.worktrees/$branch"               # the worktree base dir
    ;;
  *) echo "NOT_IN_WORKTREE" ;;
esac
```

If it prints `NOT_IN_WORKTREE`, tell the user `zed`/`code` must be run from
inside a worktree, and stop.

Then choose what to open — works for single-repo and multi-repo worktrees alike
(`*/` matches only dirs, so hidden `.claude` and the `PLAN.md` file are excluded):

```bash
first=""; count=0
for d in "$base"/*/; do
  [ -d "$d" ] || continue          # guard against a literal '*/' when nothing matches
  count=$((count + 1))
  [ -n "$first" ] || first="${d%/}"
done
```

### `code` (one-time VS Code trust)

VS Code opens an untrusted folder in **Restricted Mode** (forcing a manual Trust
+ reload). Worktrees are a **sibling** of the main repo
(`<root>/.worktrees/<branch>/…`), so trusting the repo does NOT cover them — but
VS Code **trusts every subfolder of a trusted folder**, so trusting `<root>`
**once** makes every current and future worktree open trusted. A marker file
records that this was done (a proxy, not live detection; if VS Code trust is ever
reset, `rm "<root>/.vscode-parent-trusted"` to re-prompt).

```bash
MARKER="$root/.vscode-parent-trusted"             # outside every repo — never committed
if [ ! -f "$MARKER" ]; then
  echo "NEEDS_TRUST"
  code "$root"                                     # open the parent so the user can trust it
else
  echo "TRUSTED"
fi
```

- **`NEEDS_TRUST`** → VS Code just opened `<root>`. Ask the user (via
  `AskUserQuestion`) to click **"Yes, I trust the authors"** in that folder's
  trust dialog and confirm once done. Only **after** they confirm, record the
  marker (`touch "$root/.vscode-parent-trusted"`), then open the worktree.
- **`TRUSTED`** → open the worktree straight away.

Open the worktree (single dir → open it directly; multiple → open the base):

```bash
if [ "$count" -eq 1 ]; then code "$first"; else code "$base"; fi
```

### `zed`

No trust concept — just open (single dir → that repo; multiple → all top-level
dirs as a multiroot project):

```bash
if [ "$count" -eq 1 ]; then
  env -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT zed "$first"
else
  env -u CLAUDECODE -u CLAUDE_CODE_ENTRYPOINT zed "$base"/*/
fi
```

The `env -u` flags unset Claude Code env vars so Zed's own Claude integration
doesn't conflict.

## `console` — handled here (not delegated)

**Triggers:** "task console" / "open console" / "open a console" / "new console".

Open a **new agterm terminal session** whose working directory is the **current
project inside the current worktree** — the git repo the cwd is in (a service
repo for a multi-repo worktree, the monorepo root for a single-repo one). This is
project-agnostic, so the dispatcher runs it directly.

Derive the worktree base from the cwd, then resolve the current project as the
**git repo root clamped to the worktree** (so a monorepo resolves to its root and
a multi-repo resolves to the specific service you're in):

```bash
cwd="$(pwd)"
case "$cwd" in
  */.worktrees/*)
    root="${cwd%%/.worktrees/*}"
    branch="${cwd#*/.worktrees/}"; branch="${branch%%/*}"
    base="$root/.worktrees/$branch"
    ;;
  *) echo "NOT_IN_WORKTREE" ;;
esac

proj="$(git -C "$cwd" rev-parse --show-toplevel 2>/dev/null || true)"
case "$proj" in
  "$base"|"$base"/*) : ;;      # cwd is inside a repo under the worktree — use it directly
  *) proj="$base" ;;           # cwd is the multi-repo base (not inside any repo)
esac
```

If it prints `NOT_IN_WORKTREE`, tell the user `console` must be run from inside a
worktree, and stop.

**Multi-repo base → resolve to the working repo, don't silently open the base
(MUST FOLLOW).** When the block above leaves `proj == "$base"`, the cwd wasn't
inside any single repo — the usual case for a multi-repo worktree whose Claude
session runs at the base. A console at the bare base is rarely what the user
means; they mean the service they're working on. Enumerate the child repos and
which carry uncommitted changes:

```bash
if [ "$proj" = "$base" ]; then
  repos=(); changed=()
  for d in "$base"/*/; do
    [ -d "$d" ] || continue
    git -C "$d" rev-parse --git-dir >/dev/null 2>&1 || continue
    r="$(basename "${d%/}")"; repos+=("$r")
    [ -n "$(git -C "$d" status --porcelain 2>/dev/null)" ] && changed+=("$r")
  done
  echo "REPOS: ${repos[*]}"
  echo "CHANGED: ${changed[*]}"
fi
```

Then pick `proj` from that output:

- **0 repos** → keep `proj="$base"` (it really is a bare dir).
- **1 repo** → `proj="$base/<that repo>"` (single-repo worktree — open it, no
  prompt).
- **2+ repos, exactly 1 changed** → `proj="$base/<the changed repo>"` — that's the
  repo being worked on; open it, no prompt.
- **2+ repos, 0 or 2+ changed** → genuinely ambiguous, so **ask** with
  `AskUserQuestion` which repo to open: list the repos with the **changed** ones
  first, labelled (e.g. `platform-integration-acme (changes)`), plus a final
  `open the base (all repos)` option that maps to `proj="$base"`. Set
  `proj="$base/<pick>"` from the answer.

(If cwd was already inside a specific repo, this whole picker is skipped — open
that repo directly, no prompt.) Finally set `name="$(basename "$proj")"`.

Then open the terminal. **agterm is the primary target** (a new session);
tmux is a fallback so `console` still works there:

```bash
if [ -n "${AGTERM_ENABLED:-}" ] && command -v agtermctl >/dev/null 2>&1; then
  # --after "$AGTERM_SESSION_ID": open the new session in the CURRENT session's
  # workspace, right BELOW this session (the anchor carries its own workspace).
  agtermctl session new --cwd "$proj" --name "$name" --after "$AGTERM_SESSION_ID"   # new agterm session in this workspace, below current, selected + focused
elif [ -n "${TMUX:-}" ]; then
  tmux new-window -c "$proj" -n "$name"                # fallback: new tmux window in the project dir
else
  echo "NOT_IN_AGTERM_OR_TMUX"
fi
```

Each `task console` opens a **fresh** session/window (the user asked for a new
console) — no select-existing logic. If it prints `NOT_IN_AGTERM_OR_TMUX`, tell
the user `console` needs agterm (or tmux) and stop.

**The console ALWAYS opens next to the session that asked for it (MUST FOLLOW).**
`--after "$AGTERM_SESSION_ID"` is not optional: the anchor carries its own
workspace, so the new session lands in the current workspace directly below the
task session. Never pass `--workspace`/`--workspace-name` instead, and never omit
the anchor — a console that appears at the end of some other workspace is lost.

## Session branches — every parallel unit of work runs in a BRANCH (MUST FOLLOW)

**The operator watches the branch list to see what you are doing and whether you
are stuck.** A long stretch of work done inline in the main session is invisible
to them: one spinner, no way to tell which of five things is running, which
finished, and which wedged. So every self-contained unit of work inside a task
gets its **own session branch**, and the main session stays a thin coordinator.

**A branch is `Agent` with `subagent_type: "fork"`** — the model-callable form of
the `/branch` command. The fork inherits this conversation's full context (it
already knows the ticket, the plan, the worktree, the decisions), runs in the
background, and its tool output stays out of the main session's context.

```
Agent({
  subagent_type: "fork",
  description: "<3-5 words — the UI label>",
  prompt: "<the unit of work, end to end, incl. what 'done' means>"
})
```

### Branch it — these are MANDATORY, not judgement calls

Start a branch **before** you begin, not after it turns out slow:

- **Any new part of the task** — a second repo, a second layer (inbound vs
  outbound, migration vs handler), a distinct subsystem. Each part = one branch.
- **Running tests, and fixing what they find.** The branch owns the whole loop:
  bring the stack up, run the suite, read the failures, fix them, re-run until
  green. Targeted runs included — "this one is quick" is how the session ends up
  frozen. See *Running infra…* below for the background-Bash mechanics it uses.
- **EVERY interaction with an EXTERNAL system — categorical, no exceptions, no
  "this one call is small".** If the work leaves this machine, it runs in a
  branch:
  - **GitHub / PRs** — babysitting a PR, waiting on a review verdict, reading
    review comments and check rollups, replying to a bot, driving checks green.
  - **CI/CD actions** — dispatching a workflow (`gh workflow run`), watching a
    run (`gh run watch`), re-running failed jobs, and above all **reading the
    failure** (`gh run view --log-failed`, check-run annotations).
  - **Deploys** — an ArgoCD sync, a `kubectl rollout status`, a promotion gate.
  - **Remote test suites** — e2e against a deployed env, dev or prod, including
    the triage-and-re-run loop.

  **One branch per external thing**: per PR, per workflow run, per gate. Never
  one branch for "all the CI stuff". Two reasons, both hard: babysitting is
  never passive (the branch pushes fixes, re-runs jobs, re-triggers reviews, so
  it needs its own context), and these outputs are the worst context polluters
  in the session — a single `--log-failed` dump is thousands of lines of
  machine output that must never land in the main conversation. **A red CI job
  or a failing e2e test is diagnosed and fixed INSIDE its own branch**, and the
  main session hears only the verdict.
- **Anything that polls or waits** — a workflow run, an ArgoCD sync, a rollout.
- **A code-review pass, a research sweep, a doc-and-e2e update** that runs
  alongside implementation.

### Run them in PARALLEL

Independent branches go out **in a single message, as multiple `Agent` calls** —
that is the whole point. Five repos' suites is five branches at once, not five in
sequence. Serialize only true dependencies (a dependency's branch finishes before
its dependent's starts).

### Name them so the list reads at a glance

`description` is the UI label. Make it say **what** and **where**:
`tests platform-lib-common`, `ci platform PR 1465`, `e2e dev regression`,
`impl inbound bet`. Never a generic `work`, `fix`, or `task`.

### Check on them, and merge back

- **`ListAgents`** — what is live right now.
- **`SendMessage({to: "<name>", …})`** — steer a running branch, or resume a
  finished one with its context intact. Never re-spawn to ask a follow-up.
- A branch completing arrives as a `<task-notification>`; its final report is
  **not shown to the operator** — relay what matters.
- **Merge back when the branch is done**: read its report, apply/keep whatever
  belongs to the task (its file edits are already on disk in the shared
  worktree), and say in chat what it produced. If it delivered nothing that the
  main session needs, say that too — do not silently drop a branch's result.

### Close every branch you opened (MUST FOLLOW)

A branch is finished when its unit of work is finished — **close it then**, pass
or fail:

```
TaskStop({ task_id: "<branch name or id>" })
```

Never leave a branch running because "it might still be useful": a stuck fork
still shows as live in the operator's UI and makes the session look busy when it
is not. **`cleanup` is the backstop** — before a project's `cleanup` deletes the
worktree, `ListAgents` and `TaskStop` **every** branch that belongs to this task.
Branches are in-process agents, so a `cleanup-task.sh` shell script cannot reach
them: closing them is the agent's job, and it happens *before* the script runs.

### What stays in the MAIN session — never branched

- **The two approval gates** — plan approval and the pre-commit code review.
  Those belong to the operator, in the conversation they are reading.
- **Any question to the operator** (`AskUserQuestion`). A branch that needs a
  decision reports back; it does not prompt the user.
- **`git commit` / `push` / `gh pr merge`** — the main session owns the
  irreversible steps, on work the operator approved.
- Quick reads, greps, and single-file edits. A branch has a startup cost; do not
  wrap a ten-second lookup in one.
- **The one-line external STATUS read** that decides whether to spawn a branch
  or to close one — `gh pr view <pr> --json state,mergedAt`, a single
  `gh run list -L 1`. That is a lookup, not work. The moment the answer is "not
  green", the diagnosis and the fix move into a branch — reading the log in the
  main session is the exact thing this rule forbids.

### Branch ≠ console

A **branch** is a parallel *agent* lane (`Agent` fork) that the main session
opens for work it wants to run alongside the conversation. A **console** is an
agterm session — a shell for the **operator** to type in, opened only by the
`console` subcommand when they ask for one.

They are not two halves of the same mechanism. A branch is where the **test** runs;
the **infra** it points at lives in its own agterm session — see the section below.

## Running infra — ONE standalone agterm session, ONE test against it (MUST FOLLOW)

Infrastructure and the tests that use it are two different things in two different
places. Getting this wrong is what produces invisible hangs, half-migrated
databases, and stacks nobody can find.

**1. The infra runs in its OWN standalone agterm session.** One session, holding
one stack, for the one test run that needs it:

```bash
agtermctl session new --cwd "<repo>" --name "<service>-infra" --after "$AGTERM_SESSION_ID" \
  --no-select --command "zsh -lc 'make up <service>'"
```

`--after "$AGTERM_SESSION_ID"` keeps it in the current workspace, directly below
this session, so it is visible and findable. **`--no-select` is REQUIRED: the
operator stays in the main session.** Infra is background furniture — yanking
focus to a session that just prints container logs interrupts whatever they were
reading. `make up` may run in the foreground **there**; that is what a session is
for, and it never blocks the conversation.

**2. The TEST runs from the MAIN session**, in its own context — a backgrounded
`Bash` call (`run_in_background: true`) or a session **branch** / agent — and is
pointed at that infra. Never run the suite inside the infra session, and never run
the stack inside the main session.

**3. ONE infra session at a time, serving ONE related test run.** The stack a
service brings up provisions only that service's schema and binds the shared host
ports, so only that service's tests may run against it, and only one such run at a
time (`go test ./... -p 1` — Go runs packages in parallel by default, which puts
several packages on one stack at once).

**4. When the test is done, STOP the infra** — `make down` (`docker compose down -v`)
and close that session — **before** any other infra session is opened. **NEVER two
infra agterm sessions in parallel.** It makes no sense: they fight over the same
ports, and the second one's migrations land on the first one's database.

The whole loop:

```
open infra session  →  wait until healthy  →  run THE test (main session, background/branch)
      →  read its output  →  make down + close the infra session  →  only then the next service
```

Rules for this loop:

- **Never swallow the bring-up's output.** A failed `up` that was redirected to
  `/dev/null` looks exactly like a slow test: the readiness check spins forever and
  nothing says why. Read the session's output, and confirm the stack is actually
  healthy (the ports from that service's compose file) before starting the test.
- **A readiness check that can pass without the stack is not a check.** `nc -z
  localhost 5432` also succeeds against a host-installed postgres — check a port
  the stack alone owns, or check the containers.
- **Nothing long-running blocks the task session (MUST FOLLOW).** The suite, the
  build, the lint sweep all go in the background or in a branch.
- **Poll a background run by READING ITS OUTPUT FILE**, or wait for the completion
  notification. Do not re-run the command to see where it got to.
- **TEAR THE STACK DOWN the moment the run ENDS — pass or fail, no exception.**
  Red is not a reason to leave it up. There is no "I might still need it" — the
  next run gets a fresh stack.
- **One run, one stack (MUST FOLLOW).** Another service, another repo, a re-verify
  after a fix — each gets its own fresh session. Carrying a stack across services
  means reusing a half-migrated database, which produces failures that look like
  code regressions but are pure leftover state.
- **Say what you started, in chat**, and say when you tore it down. The operator
  must never have to ask where the infra is.
- **Destructive infra is still a question.** Bringing a local stack up or down is
  routine; wiping a volume, dropping a database that holds work, or touching
  anything shared/remote is not — ask first.
- **`cleanup` closes the task's infra sessions AND its session branches** —
  `ListAgents` → `TaskStop` each branch, and for every infra session `make down`
  first, then close it. Do it **before** the worktree is deleted (a `make down`
  needs the compose file).

## Plannotator binary (shared)

`plannotator` may not be on PATH in spawned sessions, so **never type a bare
`plannotator`** — resolve it inside the same command, with an env override first
and `~/.local/bin` as the conventional fallback:

```bash
PLANNOTATOR="${PLANNOTATOR:-$(command -v plannotator || echo "$HOME/.local/bin/plannotator")}"
[ -x "$PLANNOTATOR" ] || echo "PLANNOTATOR_NOT_FOUND"
```

Every `plannotator …` command below (and in the project skills' `plan` step)
means `"$PLANNOTATOR" …` with that resolution inlined — shell state does not
survive between `Bash` calls, so repeat the assignment in each command. On
`PLANNOTATOR_NOT_FOUND`, tell the user to install it or to export `PLANNOTATOR`,
and stop — do not guess another path.

**ALWAYS launch plannotator in the BACKGROUND (MUST FOLLOW).** Every plannotator
invocation — `review`, `annotate`, `annotate … --gate` — runs with
`run_in_background: true` on the `Bash` tool call. plannotator blocks until the
user submits, so a foreground call freezes the session: the user cannot type, and
you cannot answer a question about the diff or plan they are reading. The run
returns later as a `<task-notification>` carrying the annotations. Never use a
trailing `&`, `nohup`, or `disown` to imitate this — the notification would never
reach you and the annotations would be lost. This rule holds in this dispatcher
and in every project skill.

## Test scope — targeted tests while working, FULL suite only at `finish` (MUST FOLLOW)

This rule is project-agnostic and **overrides any project skill's wording that
asks for the whole suite earlier.**

- **While the functionality is still in progress** (writing code, iterating,
  fixing a review finding, reworking after `review`): run **only the tests that
  cover the code you are writing or changing** — the specific test file,
  package, suite, or `-run`/`--name` filter for that behaviour, plus its direct
  callers. Nothing wider. A full-repo or all-repo suite run at this stage is
  **waste** — do not do it, and do not ask the operator to stage shared
  infrastructure for it.
- **The FULL suite runs only after the user explicitly says `task finish`.**
  That is the single point where every changed repo/folder runs all of its
  tests + linters (unit, integration, frontend, e2e — whatever the project skill
  lists). Before that command, a full run is out of scope.
- **`review` does NOT trigger a full suite.** The `review` checkpoint shows the
  diff; it requires only the targeted tests for the changed behaviour to be
  green.
- **"Never commit red" still holds** — but it is enforced at `finish`, which is
  where committing happens anyway. In-progress code may have untouched parts of
  the repo unverified; it may NOT have the tests for *its own* behaviour failing.
- **If a targeted run is impossible** (no isolated suite exists, the behaviour is
  only reachable through a broad e2e suite), run the narrowest thing that does
  cover it, and say in chat which scope you used.
- **Every run — targeted or full — happens in a session BRANCH**, which owns the
  run *and* the fixes for whatever it turns red. See *Session branches* above.
  This rule narrows the *scope* of what is run; it never moves the run back into
  the main session.

## Run the task to COMPLETION — never stop in the middle (MUST FOLLOW)

**A task runs from start to finish in one continuous push. Do NOT halt partway
to report progress, to describe what you are about to do, or to ask permission
to keep going.** Stopping mid-task with nothing delivered is the single most
expensive failure mode in this flow: the work is left half-applied, the user has
to re-prompt to get the rest, and the context that made the next step obvious is
gone by the time you resume.

**These are NOT reasons to stop — keep working:**

- A step finished and the next one is "big" or slow.
- You want to tell the user what you found, what you changed, or what is next.
- A command failed, a test went red, a linter complained, or CI is still running.
  **Diagnose it and fix it** — that IS the task, not a reason to hand back.
- A dependency PR is not merged yet, a review has not come back, or a deploy is
  pending. Do the work that does not depend on it, then drive the blocker.
- You are unsure which of two reasonable implementations to pick, and the ticket
  or surrounding code implies an answer. Pick the one that matches the existing
  code and say so in one line when you report at the end.
- The task turned out larger than expected. Scaling it down is not your call —
  finish it.

**Stop ONLY when the user genuinely has to decide.** Those cases are:

- **The two named gates** — plan approval (after writing/reworking `PLAN.md`) and
  the code review before commit/push/merge. These always require the user.
- **A destructive or irreversible action** — dropping data, force-pushing over
  someone else's work, deleting a branch/env, rewriting shared history.
- **Production impact** — anything that changes prod behaviour, config, or data.
- **A real scope change** — the task as written turns out to need work the user
  never asked for. Report it in one or two lines and ask; do not silently widen.
- **A genuine ambiguity that changes the deliverable** and that the ticket, the
  code, and the conventions cannot settle. Ask ONE short question.

If none of those apply, you do not have permission to stop — you have an
obligation to continue. When you finally do report, report the finished result,
not a status update from the middle.

## Understand the task BEFORE planning it (MUST FOLLOW)

`plan` is delegated to the project skill, but this rule binds **every** project,
including new ones, and applies just as much to the `plan` that `new`
auto-starts in a fresh worktree.

**Never write a plan — and never open the plannotator gate — for a task you do
not fully understand.** A plan invented from a one-line ticket title gets
approved, implemented, and reviewed against the wrong problem; that costs far
more than one clarifying question.

Before any brainstorming, any `PLAN.md`, and any `annotate`/`review` gate, you
must be able to answer all of:

- **Observed behaviour** — the concrete symptom: failing request/response, exact
  error text, stack trace or log lines, env, and the affected user/tenant/entity.
- **Expected behaviour** — acceptance criteria, or the exact desired output.
- **Where** — the repo/service/screen/flow, precise enough to point at the code.
- **How to reproduce or verify** — steps, a request, a test, or real data.

Close the gaps yourself first: read the whole ticket, **every comment and every
attachment/image**, then search the worktree's repos. Never ask for something the
ticket already contains, and never ask the user to fetch what you can fetch.

**If anything is still missing — STOP.** Do not write `PLAN.md`, do not open
plannotator, do not touch code. Ask the user, in chat, for the specific missing
input: the raw log excerpt / stack trace the ticket was cut from, the real
request+response payloads and failing ids, the env / build / partner / device it
happens on, screenshots or the alert that produced the ticket, what "done" means
when acceptance criteria are absent, or which of two readings is intended. Ask
ONE short, concrete list (use `AskUserQuestion` when the options are enumerable),
then **WAIT** for the answer before planning.

## Review checkpoint — stop after code changes (MUST FOLLOW)

Every time you finish a batch of code changes, **STOP before committing / before
`finish`** and put the diff in front of the user. Do one of:

- **Ask the user to review** and wait, OR
- **Run the `changes` subcommand** — walk the task's business-logic diffs in
  chat, a screenful at a time, and stop on any one the user wants to discuss or
  rework (see the `changes` section), OR
- **Run the `review` subcommand** (the default) — it **picks the mode by state**:
  a `plannotator review` of the worktree diff when a real (non-markdown) code
  change exists, or a `plannotator annotate <file.md>` when the change is only a
  plan/docs (`.md`) — because `review` can't render mermaid/tables (see the
  `review` section). plannotator ≥ 0.21 auto-discovers every nested repo, so this
  is the native path for single-repo AND multi-repo worktrees alike — no per-repo
  loop, no external diff tool. **A docs-only task (e.g.
  an R&D report) is therefore checkpointed with `annotate`, not `review`.**

Then wait for the user, or address the annotations, before continuing. Going
straight from writing code to `finish` without a review checkpoint is not
allowed. A review that returns no annotations = approved.

**Plus a LOCAL code-review before any commit (MUST FOLLOW).** The operator
checkpoint above is not the only gate: before the first `git add`/`commit` of a
task, the `sadensmol-code-review` skill MUST be run over the working changes.
**Fix every P0 and every P1**; **report every P2** to the user and fix it only
when it is critical or trivially easy. Re-run it after fixes until no P0/P1
remains. Each project's `finish` owns the exact placement of this gate, but the
rule holds for every project — green tests/linters, a clean plannotator review,
or an agentic PR review never substitute for it.

## `review` / plannotator — handled here (not delegated)

**Triggers:** "task review", "review task", "review changes", "review", "open
plannotator", "open plannotator please", "open annotate".

Opening plannotator at the worktree base and waiting is the ONLY thing this does.
But **which plannotator mode you open depends on what actually exists — pick it by
state, NEVER blindly `review` (MUST FOLLOW):**

**Markdown is ALWAYS annotate, NEVER review (MUST FOLLOW).** `plannotator review`
renders a raw code diff — it does **not** render mermaid diagrams or markdown
tables. So `.md` files (a `PLAN.md`, an R&D report / design doc under `docs/`,
any `*.md`) are **always** opened with `annotate`, never `review` — **even when
the user literally typed `task review`**. Until a **real, non-markdown code
change** exists, if the only changes are a plan and/or docs you MUST use
`annotate`. Classify the changes, then pick:

- **`plannotator review`** — opened **only when a real CODE change exists**: a
  changed/added tracked-or-untracked **non-`.md`** source/test/config file,
  beyond mechanical `go.mod`/`go.sum`/dep churn from `task new`. `.md` files
  never count as a code change.
- **`plannotator annotate <file.md>`** — opened when the only changes are
  **markdown** (`PLAN.md` and/or `*.md` docs), with no real code change:
  - **Exactly one `.md`** changed → annotate that file.
  - **Multiple `.md` files** changed → `annotate` opens **one file at a time**,
    so **ASK the user (`AskUserQuestion`) which single file to open** (list them,
    e.g. `PLAN.md`, `<repo>/docs/rag/rag-rnd.md`). Never silently pick one, and
    never fall back to `review` just to show them all at once.
  - This is a plain annotate — **no `--gate`**; the formal plan-approval gate
    belongs to the `plan` subcommand.
- **Neither a code change nor any `.md`** → there is nothing to open. Say so and
  stop. Do NOT manufacture a diff or a plan to give plannotator something to show.

Decide the mode by **classifying the actual changes** across the nested repos
(untracked included) — markdown is annotate-only, everything else is code:

```bash
# (worktree base already derived as $base, exactly as the zed/code block does)
code=0; mds=()
[ -f "$base/PLAN.md" ] && mds+=("PLAN.md")          # plan at the base is annotatable md
for d in "$base"/*/; do
  [ -d "$d" ] || continue
  git -C "$d" rev-parse --git-dir >/dev/null 2>&1 || continue
  repo="$(basename "${d%/}")"
  # -uall is REQUIRED: without it git collapses an untracked dir to "docs/rag/"
  # (a non-.md path) and a new-doc-only change gets misread as a code change.
  while IFS= read -r line; do
    [ -n "$line" ] || continue
    f="${line:3}"                                    # strip the "XY " status prefix
    case "$f" in
      # machine-generated / dependency churn — NOT deliberate code work; ignore:
      go.mod|go.sum|*/go.mod|*/go.sum) : ;;          # Go dep pins (from `task new`)
      local.properties|*/local.properties) : ;;      # Flutter/Android generated
      .DS_Store|*/.DS_Store) : ;;
      *.md) mds+=("$repo/$f") ;;                      # markdown → annotate
      *) code=1 ;;                                    # real code → review
    esac
  done < <(git -C "$d" status --porcelain -uall 2>/dev/null)
done
echo "CODE=$code"; printf 'MD=%s\n' "${mds[@]}"
```

- **`CODE=1`** → `plannotator review` (real code changed).
- **`CODE=0` and exactly one `MD=`** → `plannotator annotate <that file>`.
- **`CODE=0` and multiple `MD=`** → **ask** which one, then `annotate` it.
- **`CODE=0` and no `MD=`** → nothing to open.

**Neither mode writes code (MUST FOLLOW).** Never read a review/annotate request
as "implement the plan", "start coding", "make the changes then review", or
"approved, so proceed". Writing/editing/reverting ANY file in response is a hard
violation. If opening plannotator would require you to first create a diff or a
plan, you are in the wrong phase: **STOP and ask**, don't implement.

**NEVER suggest / offer / ask about `cleanup` while work remains (MUST FOLLOW).**
When `review` finishes — approved, no annotations, or annotations addressed — do
NOT end by proposing `cleanup` (or "want me to remove the worktree?") as a next
step. `cleanup` is appropriate ONLY once the task is fully done: every PR for the
branch **merged** (not just approved) — including whatever extra gate the project
skill defines as part of its definition-of-done (e.g. an end-to-end test PR and
its regression run being green) — and no outstanding work. If any PR is still open or any work is left (implementation, `finish`,
merges, e2e), cleanup must not be mentioned at all — the natural next step is
`finish` (or continuing the work), never cleanup. Only surface `cleanup` when
nothing is left to merge and nothing is left to do, or when the user asks for it
explicitly.

**SINGLE INSTANCE — plannotator allows only ONE live instance at a time (MUST FOLLOW).**
plannotator supports a **single** running instance **total** — `review` and
`annotate` **share that one slot**. Launching a second while one is live
**silently breaks the first** (the already-open tab stops working and any
in-progress annotations in it are LOST) — a hard plannotator constraint, not just
tab clutter. So never have two open at once: not review+review, not
annotate+annotate, and **not review+annotate together**.

**Force-closing another plannotator instance requires explicit USER PERMISSION (MUST FOLLOW).**
Do NOT `pkill` / kill / `session close` a running `review` or `annotate` on your
own — the user may have unsubmitted annotations in it, and force-closing
**discards that work**. When one is already open and you need a different one (or
the same one refreshed), use `AskUserQuestion` to let the user decide:
- **Force-close & open new** — the user authorizes it (e.g. the open tab is
  stale/orphaned or belongs to another session). Only then may you
  `pkill -f "plannotator (review|annotate)"`, confirm it is closed (`pgrep` shows
  none), and launch the new one.
- **Keep existing** — do NOT close it; the user finishes/inspects the current
  review, tells you when it is closed, and you launch then.
Default: never force-close unprompted; act only on the user's explicit choice.
This holds even on an explicit "reopen"/"restart"/"refresh".

The launch blocks until submit — a completion/`failed` notification usually just
means the user closed the tab, NOT that the cwd was wrong; do not react by
relaunching.

Check whether one is already running before launching (either mode):

```bash
pgrep -f "plannotator (review|annotate)" >/dev/null 2>&1 && echo "ALREADY_RUNNING" || echo "NONE"
```

- **`ALREADY_RUNNING`** → do NOT launch a second one. Ask the user
  (`AskUserQuestion`) whether to **force-close the existing** review/annotation
  and open a new one, or **keep** it and wait until they close it themselves.
  `pkill` ONLY after the user picks force-close; then confirm `pgrep` shows none
  and launch exactly one. Never force-close on your own initiative.
- **`NONE`** → launch exactly one instance, in the **mode chosen above**:

```bash
PLANNOTATOR="${PLANNOTATOR:-$(command -v plannotator || echo "$HOME/.local/bin/plannotator")}"
# CODE=1 (real, non-markdown code changed) → code-diff review:
cd <worktree-base> && pwd && "$PLANNOTATOR" review
# OR — markdown only → annotate the chosen .md (PLAN.md, or the file the user picked):
cd <worktree-base> && pwd && "$PLANNOTATOR" annotate <chosen.md>
```

**Launch it in the BACKGROUND — always (MUST FOLLOW).** Set
`run_in_background: true` on the `Bash` tool call. plannotator blocks until the
user submits. A foreground call therefore freezes the whole session: the user
cannot type, and you cannot answer a question about the very diff they are
reading. Background keeps the conversation usable while the tab is open; the run
returns later as a `<task-notification>` carrying the annotations.

- This applies to **every** plannotator launch — `review`, `annotate`, and
  `annotate … --gate` in a project's `plan` step.
- Do NOT fake it with a trailing `&`, `nohup`, or `disown`. Use the tool's
  background flag, or the notification never reaches you and the annotations are
  lost.
- One launch is enough. Do not poll and do not relaunch while it runs.

`annotate` takes exactly **one** `.md` path (resolved relative to the worktree
base — so `PLAN.md`, or a nested `<repo>/docs/…/report.md`). It cannot open
several at once; that is why multiple changed `.md` files force the
`AskUserQuestion` pick above rather than a `review`.

- Derive `<worktree-base>` from the cwd (the `.worktrees/<branch>/` root, NOT a
  repo subdir) exactly as the `zed`/`code` block does. Run it from the base so
  `review` aggregates every nested repo into one view, and `PLAN.md` resolves (it
  lives at the base).
- Verify `pwd` is the base BEFORE launching; the session cwd persists, so it may
  already be correct.
- `review` shows the **uncommitted** working-tree diff — it shows NOTHING once
  work is committed, so always review **before** committing (see each project's
  finish review gate).
- When it returns, address each annotation (keyed by `<repo>/<path>` in
  multi-repo for `review`), re-run if needed. No annotations = approved.

### `annotate` goes stale after a doc edit — ASK the user, never force-refresh (MUST FOLLOW)

`plannotator annotate` reads the file **once at launch** — so any edit you make to
that `.md` afterward leaves the open tab **stale**, showing the pre-edit content.
But you must **never force-close the open `annotate` to refresh it** — the user may
have unsubmitted annotations there, and killing it **loses that work** (see the
SINGLE INSTANCE rule).

- **Trigger:** you (via `Edit`/`Write`) changed the exact `.md` file the open
  `annotate` was launched on. Remember that path when you launch (`annotate
  <chosen.md>`), so you know which edits are relevant.
- **Debounce:** make **all** the doc edits for this turn first, then handle it
  once at the end — not after each individual `Edit`.
- **What to do — tell the user, don't touch their tab:** report that you edited
  `<file>`, so the open `annotate` is now showing stale content, and **ask them to
  submit/close it when ready** so you can reopen it with the fresh content.
  **Wait** until they've closed it (`pgrep -f "plannotator annotate"` shows none),
  then relaunch `annotate` on the **same** file.
- If your edits were to a doc that ISN'T the open one, don't touch the user's tab —
  just tell them which file changed.
- Only refresh a `plannotator **annotate**`; never convert an open annotate into a
  `review` (or vice-versa) as part of a refresh — mode is chosen by state, not by
  the refresh.
- If NO `annotate` is running (the user closed it), a doc edit does **not** auto-open
  one — there's nothing to refresh; mention the doc changed and offer to reopen.

## `changes` — handled here (not delegated)

**Triggers:** "task changes", "show changes", "show me the changes", "show the
diffs", "changes".

Walks the user through **this task's business-logic diffs, in chat**, a screenful
at a time, so every individual fix can be approved, questioned, or reworked on
the spot. It is the conversational counterpart of `review`: no plannotator tab,
no single-instance constraint, no annotation round-trip — just the exact patch, a
static test-reference count, and a pause.

**Goal:** the user reads each real diff hunk, decides whether the fix looks good,
raises a question or a discussion about that exact fix, and (later, if they say
so) has it reworked. Nothing else.

**Scope — THIS task's own changes ONLY (MUST FOLLOW).** Show only what this
task/session actually changed: the branch's own commits plus the uncommitted
working tree, inside the current worktree. Never diff a primary checkout, never
show the default branch's history, and never reconstruct a diff from memory, from
`PLAN.md`, or from what you believe you edited. **Every hunk you print MUST be
verbatim output of a `git` command run in this session** — if you did not run the
command, you do not have the diff.

### Step 1 — worktree base, and each repo's base ref

Derive `$base` (the worktree base) from the cwd exactly as the `zed`/`code` block
does. On `NOT_IN_WORKTREE`, tell the user `changes` must run inside a worktree,
and stop.

Then, per nested repo, resolve what to diff against. `.task-base` is the
stacked-task marker written at the worktree base by the project's `new`; absent →
the repo's own default branch:

```bash
# ($base already derived exactly as the zed/code block does)
base_branch="$(cat "$base/.task-base" 2>/dev/null)"
for d in "$base"/*/; do
  [ -d "$d" ] || continue
  git -C "$d" rev-parse --git-dir >/dev/null 2>&1 || continue
  def="$(git -C "$d" symbolic-ref -q --short refs/remotes/origin/HEAD 2>/dev/null | sed 's|^origin/||')"
  ref="origin/${base_branch:-${def:-main}}"
  git -C "$d" rev-parse --verify -q "$ref" >/dev/null 2>&1 || ref="origin/${def:-main}"
  mb="$(git -C "$d" merge-base "$ref" HEAD 2>/dev/null)"
  echo "REPO=$(basename "${d%/}") BASE=$ref MERGE_BASE=$mb"
  git -C "$d" diff --numstat "$mb"            # committed + staged + unstaged, in one shot
  git -C "$d" status --porcelain -uall | grep '^??'   # untracked (new files)
done
```

- `git diff "$mb"` already covers **committed + staged + unstaged** — do not run
  three separate diffs.
- Untracked files are rendered with
  `git -C "$d" diff --no-index -- /dev/null "<file>"`.
- A repo with no changes is skipped silently; **no changed repo at all** → say
  "no changes in this task" and stop.

### Step 2 — keep business logic, list the rest

Split the changed files into **business logic** (shown) and **noise** (named
once, never printed). Noise is:

- **Tests** — `*_test.go`, `*_test.py`, `test_*.py`, `*.test.*`, `*.spec.*`,
  `*_test.dart`, `*Test.swift` / `*Tests.swift`, and anything under a
  `test/`, `tests/`, `__tests__/`, `testdata/`, `fixtures/` path segment (a
  whole test repo therefore drops out).
- **Generated / mocks** — `*.pb.go`, `*_gen.go`, `*.gen.*`, `mock_*.go`,
  `*_mock.go`, `mocks/`, `generated/`, `*.g.dart`, `*.freezed.dart`.
- **Dependency + lock churn** — `go.mod`, `go.sum`, `package-lock.json`,
  `pnpm-lock.yaml`, `yarn.lock`, `pubspec.lock`, `Podfile.lock`, `poetry.lock`,
  `uv.lock`, `Cargo.lock`.
- **Docs and junk** — `*.md` (incl. `PLAN.md`), `.DS_Store`,
  `local.properties`.

Everything else is business logic — including SQL migrations, config/values
files, and API/schema definitions. Print the noise as a single line
(`Not shown: 4 test files, go.sum, PLAN.md`) so nothing is hidden silently, then
never mention it again.

### Step 3 — batch by screen, pause after each batch

Batch so the user sees **about one screen** and scrolls as little as possible:

- Up to **3 files** per batch, while the batch's total diff lines (added +
  removed) stay **≤ ~60**.
- A file whose diff is larger than ~60 lines is shown **alone**.
- A file larger than ~150 diff lines is split **by hunk** across several screens
  (~80 lines each), each screen still followed by a pause.
- Order: repo by repo, and inside a repo the way `git` lists them — stable and
  predictable, so the user can tell how far along they are.

Print a one-line progress header for every batch:
`Batch 2/5 — platform: internal/wallet/service.go, internal/wallet/mapper.go`.

Each file in a batch is shown as:

1. `#### <repo>/<path>` — plus `(new file)` / `(deleted)` when applicable.
2. The **exact diff**, in a ```diff fence, verbatim from `git diff` (never
   re-typed, re-indented, summarised, or trimmed apart from the hunk-splitting
   above).
3. One line: `Tests: N` — see Step 4.

**No commentary unless asked.** Do not explain what the diff does, do not review
it, do not praise it, do not propose improvements. The user reads the diff; your
job is to show it and wait. Answer only what they ask.

### Step 4 — the `Tests: N` line (static reference count)

For each shown file, take the symbols its hunks touch (function/method/class
names from the `@@ … @@` context headers and from added/removed declaration
lines), and count the test cases that reference them:

```bash
grep -rnw --include='*_test.go' --include='*_test.py' --include='test_*.py' \
  --include='*.test.ts' --include='*.test.tsx' --include='*.spec.ts' \
  --include='*_test.dart' --include='*Test.swift' --include='*Tests.swift' \
  "<Symbol>" "$base" | head -20
```

Report `Tests: N` where N is the number of distinct test functions/cases that hit
those symbols (this task's newly added tests included), and name up to 3 of them:
`Tests: 3 (wallet_service_test.go: TestDebit_InsufficientFunds, …)`. No match →
`Tests: 0 — no test references this change`.

**This is a static reference count, not a coverage measurement (MUST FOLLOW).**
Never call it coverage, never state a percentage, and never run the suite to
produce it — `changes` runs **no** tests (the targeted-tests rule above is about
writing code, not about this walkthrough).

### Step 5 — the pause

After each batch, stop and ask with `AskUserQuestion` — one option per file in
the batch plus the continue option (3 files + continue = 4, the maximum):

- `Discuss <file>` — one per file shown in this batch.
- `Next batch` — approved as-is, go on. On the final batch this reads `Done`.

Then act on the answer:

- **Next / Done** → those diffs are approved; continue with the next batch, or
  print the closing summary.
- **Discuss `<file>`** → **the walkthrough STOPS here (MUST FOLLOW).** Leave the
  batch loop entirely and switch to plain discussion of that one change: answer
  the user's questions about that exact fix, short and factual, tracing to the
  diff and the code around it, and apply whatever fix they ask for. Do NOT print
  the next batch, do NOT re-print the remaining diffs, and do NOT ask "shall we
  continue?" — the discussion owns the session now.
  - **Reworks are allowed here, and only here.** `changes` may edit the file the
    user is discussing, exactly as they asked. Never touch a file they did not
    name, and never fix anything "while you are in there".
  - After a rework, re-print **that file's** diff and its refreshed `Tests:`
    line, and keep discussing. Still no next batch.
  - **Resume only on an explicit request** — the user says "continue", "next",
    or runs `task changes` again. Then pick up at the batch after the one that
    was interrupted (re-deriving the diffs, since a rework changed them), and
    say which batch you are resuming at.

**`changes` never commits, never pushes, never runs `finish`, and never opens
plannotator.** It is a read-and-discuss loop; the only writes it may perform are
the reworks the user explicitly asks for while discussing.

### Closing summary

Print this only when the walkthrough actually reaches the end (the user answered
`Done` on the last batch) — never when it stopped for a discussion. At most 5
lines: files approved, files reworked in this pass, files left with an open
question. Nothing else — no next-step offers, and (per the rule above) never a
`cleanup` suggestion.

## `list` / `switch` — handled here (not delegated)

Both operate on the worktrees under the **project root** from Step 2 (`$root`).
They run from anywhere — you need not be inside a worktree.

### `list`

**Triggers:** "task list", "list tasks", "show worktrees".

```bash
ls -1d "$root"/.worktrees/*/ 2>/dev/null | xargs -I{} basename {}
```

Display as a numbered list. If none, say "No worktrees found."

### `switch`

**Triggers:** "task switch", "switch task".

1. List worktrees (as in `list`, using `$root`). If none → "No worktrees found."
   and stop.
2. Present them with `AskUserQuestion` (max 4 — if more, show the 4 most recently
   modified and let the user type "Other").
3. On selection, **delegate the open to the project** so its worktree wiring runs
   (dep refresh, session naming): invoke the resolved project `:task` skill with
   args `new <selected-branch>`. Every project's `new` treats an already-existing
   branch as a switch (reuse it, plain claude, no new branch, no auto-plan) — so
   `new <existing>` IS the open. Do not run a project's `new-task.sh` directly
   from here; go through the project skill.

## Adding a new project

1. Give the project's plugin a `task` skill with `user-invocable: false` in its
   frontmatter (so it stays out of the `/` menu but remains `Skill()`-callable).
   It implements only `new` / `plan` / `finish` / `cleanup` (+ its own preflight,
   layout, definition-of-done); `review` / `changes` / `list` / `switch` / `zed` /
   `code` / `console` are owned here and need nothing from it.
2. Name the agterm workspace after the plugin — that alone makes Step 2 resolve
   it, with no edit to this file.
3. For cwd-based resolution outside agterm, add an entry to the **local, unpublished**
   `~/.agents/sadensmol-router-routes.json` (`{ "cwd": "<substring>", "skill":
   "<plugin>:<skill>" }`). **Nothing is added to this file** — no project name, no
   path, no row. This skill is published and stays generic; the routes file and the
   project's own plugin hold every project-specific value.

That's the whole contract — one visible `/task`, N hidden per-project impls, with
the project-agnostic subcommands centralized here and zero project knowledge in
this file.
