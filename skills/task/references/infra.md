# Running infra — ONE standalone agterm session, ONE test run against it

Read this before the main session brings up a local stack. The project skill adds its
own stack commands, ports and teardown command.

**The main session owns every stack (MUST FOLLOW).** Bring-up is one command: the main
session runs it itself, health-checks it, hands a test lane the healthy stack, and tears
it down after the lane reports. No subagent ever starts, stops or restarts a stack or
touches the lock. Use the project skill's bring-up command exactly as written (own
agterm session, `--no-select`, status wrapper, health check); a hand-rolled
`bash -c '… | tee … && sleep N'`, or a fixed sleep instead of a health check, is a defect.

**Never ask the user to start a stack (MUST FOLLOW).** A repo's `CLAUDE.md` or README line
such as "do not run make up, the developer keeps services running, ask the developer"
does not apply in a task worktree: when the stack a test needs is down, the main session
starts it itself per this file. The gates below still hold — the runtime check (0a), the
lock (0), the health check, and asking first for anything destructive or shared.

The shape: the **stack** runs in its own agterm session; the **test** runs from the
main session (a `run-background-shell` call) or a session branch, pointed at it; never
the suite inside the infra session, never the stack inside the main session.

```
take lock → open infra session → wait until healthy → run THE test (-p 1)
  → read its output → teardown + release lock + close session → only then the next stack
```

## 0a. Check the container runtime — before the lock (MUST FOLLOW)

```bash
docker info >/dev/null 2>&1 || echo RUNTIME_DOWN
AGST="$(cd <sadensmol-task-skill-dir> && pwd -P)/../agterm/scripts/agst.sh"
[ -f "$AGST" ] || echo AGST_NOT_FOUND
```

`RUNTIME_DOWN` → stop and report the failed preflight to the user. Do not take the lock,
do not run the bring-up, do not start the runtime yourself. `AGST_NOT_FOUND` → stop and
report it (the `sadensmol-agterm` skill is missing next to this one); do not start the stack
without the status wrapper.

## 0. Take the lock first — no stack starts without it (MUST FOLLOW)

Two stacks — this task's or another worktree's — fight over the same host ports, and
neither test run works. The lock makes the
second attempt fail loudly.

The lock is a directory at the worktree base — outside every repo, so never committed,
and visible to every lane of the task. `$root`, `$b`, `$WT` come from SKILL.md →
*Worktree helpers* (no worktree → no lock, no stack). Scan sibling worktrees first (only
this scan sees another task's stack), then `mkdir` (atomic — one caller wins a race):

```bash
LOCK="$WT/.infra.lock"; other_held=
for other in "$root"/.worktrees/*/.infra.lock; do
  [ -d "$other" ] && [ "$other" != "$LOCK" ] || continue
  other_held=1; echo "OTHER_TASK_HOLDS_A_STACK:"; sed 's/^/  /' "$other/owner" 2>/dev/null
done
if [ -n "$other_held" ]; then
  :   # another task owns the ports — do not take this lock
elif mkdir "$LOCK" 2>/dev/null; then
  printf 'task=%s\nworktree=%s\nstack=%s\nrepo=%s\nsession=%s\npid=%s\nstarted=%s\n' \
    "$b" "$WT" "<stack>" "<repo>" "${AGTERM_SESSION_ID:-}" "$$" "$(date -u +%FT%TZ)" > "$LOCK/owner"
  echo ACQUIRED
else
  echo HELD; cat "$LOCK/owner" 2>/dev/null
fi
```

- `OTHER_TASK_HOLDS_A_STACK` → do not start. Say which task (`task=`) and worktree hold
  it, and wait for its release.
- `ACQUIRED` → bring the stack up.
- `HELD` → do not start a second stack. `owner` names the task, worktree, stack, repo
  and session. Wait for release, or say which stack holds it and what is queued. Never
  `rm -rf` a live lock.

**Release in the SAME command as the teardown:** `make down && rm -rf "$LOCK"` (or the
project's teardown). A teardown without the release blocks every later stack; a release
without the teardown hands the next caller live containers.

**Stale only when BOTH hold:** the `owner` session no longer exists, and no container of
that stack runs (`docker ps`). Then `rm -rf "$LOCK"`, retake it, and say so in chat.

**Only the main session takes and releases the lock.** A lane's brief never contains the
acquire, the release, the bring-up or the teardown. One lock per worktree: it serializes
this task's stacks; the sibling scan shows other tasks. A stack started without a lock does not show up — check
`docker ps` when a bring-up fails on a busy port.

## 1. The stack runs in its OWN agterm session

```bash
id="$(printf '%s' "$b" | grep -oiE '^[a-z]+-[0-9]+' | tr '[:lower:]' '[:upper:]')"
pfx="$(bash <sadensmol-task-skill-dir>/scripts/session-tree-prefix.sh)"
AGST="$(cd <sadensmol-task-skill-dir> && pwd -P)/../agterm/scripts/agst.sh"
sid="$(agtermctl session new --cwd "<repo>" --name "$pfx infra <service> ${id:-$b}" --after "$AGTERM_SESSION_ID" \
  --no-select --command "zsh -lc 'sh \"$AGST\" make up <service>'" --json | jq -r '.result.id')"
agtermctl session text --target "$sid" --lines 5
```

- **Check the new session is not blocked by a startup prompt (MUST FOLLOW).** A login
  shell in a dir with `.env` can stop on
  `dotenv: found '.env' file. Source it? ([y]es/[N]o/[a]lways/n[e]ver)`, and the stack
  never starts. Read the screen right after `session new`. On such a prompt, decline it:
  `agtermctl session type --target "$sid" $'n\n'`. Read the screen again, then go on to
  the health check.

- **Name it as a tree child of the task session:** `└─ infra <service> <task ID>` when
  it is the last child below the task session, `├─ infra …` when another child is
  already below. The helper decides; never hardcode the prefix.
- `--after "$AGTERM_SESSION_ID"` keeps it directly below this session. **`--no-select`
  is required** — the operator stays in the main session. The stack may run in the
  foreground there.
- **`agst`** (the agterm cookbook's `long-commands-status` wrapper, bundled as
  `sadensmol-agterm` → `scripts/agst.sh`; `$AGST` from step 0a) sets
  the sidebar row **active** while the command runs, **completed** on exit 0, **blocked**
  on non-zero, so a stack that died on startup is visible. Prefix any long command with it
  (`sh "$AGST" make test`, `sh "$AGST" go test ./...`). It passes the exit code through and, outside
  agterm, just `exec`s the command. A hang reads as active; Ctrl-C leaves the row active.
  Always call it by its quoted path through `sh` (`sh "$AGST" …`), never a bare `agst`:
  it is not on `PATH`, and `sh` works even when an install dropped the `+x` bit.

## 2. One infra session at a time, serving ONE related test run

A service's stack provisions only that service's schema and binds the shared host
ports, so only that service's tests may run against it, one run at a time — pass
`-p 1` to `go test ./...` (Go runs packages in parallel by default). **Never two infra
sessions in parallel**: they fight over ports, and the second one's migrations land on
the first one's database.

## Rules for the loop

- **Never swallow the bring-up output.** Read the session's output and confirm health on
  a port the stack alone owns, or with `docker ps`. `nc -z localhost 5432` also passes
  against a host-installed postgres.
- **Nothing long-running blocks the task session.** Suites, builds and lint sweeps go in
  the background or a branch. Poll by reading the output file or waiting for the
  notification; never re-run a command to see progress.
- **Tear down the moment the run ends — pass or fail.** No "I might still need it"; the
  next run gets a fresh stack. **One run, one stack (MUST FOLLOW):** another service, a
  re-verify after a fix — each gets a fresh session. A carried-over stack means a
  half-migrated database and failures that look like regressions.
- **Say in chat what you started and when you tore it down.**
- **Destructive infra is still a question.** Up/down of a local stack is routine; wiping
  a volume that holds work, dropping a database, or touching anything shared or remote
  is not — ask first.
- **`cleanup` closes the task's infra sessions and branches** before deleting the
  worktree (`make down` needs the compose file): teardown first, then close each session.
  The `rm -rf` takes `.infra.lock` with the worktree; a `cleanup` that stops early
  releases it explicitly.
