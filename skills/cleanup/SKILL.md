---
name: sadensmol-cleanup
description: "Use when the user asks to clean up, purge, prune or free disk space from old Claude Code, Claude Desktop, Codex chat or OpenCode data or unused Docker images — sessions, transcripts, subagent and tool output, file history, todos, shell snapshots, caches, logs, artifacts, stale locks, old plugin versions or config backups — older than a month or another age."
---

# Cleanup

Remove agent data older than N days (default 30) from the Claude Code, Claude Desktop, Codex
chat and OpenCode folders, and Docker images unused for N days. No other file is ever a
target: not Downloads, not Documents, not a project checkout, not the session scratch
directory. Docker containers, volumes and build cache are not targets either.

**Every deletion goes through `scripts/cleanup.sh` and a manifest the user approved.**
Never delete with `rm`, `find -delete`, SQL or the OpenCode CLI by hand, even for one item.

## Workflow

1. **Age.** Use 30 days unless the user names another age; convert weeks or months to days.
2. **Scan** (deletes nothing). Write the manifest to the session scratch directory:

   ```bash
   bash <this-skill-dir>/scripts/cleanup.sh scan --days 30 --out <scratch>/cleanup-manifest.tsv
   ```

3. **Show** the printed table and its notes verbatim. Add nothing else. Each row names
   what it deletes, the folder it deletes from (Where), the item count and the size.
4. **Ask** for permission (ask-user), with three choices: delete everything listed, exclude
   some rows, or cancel. Never delete without an explicit yes in this step.
   - **The question carries the table.** The user may see only the question, so put the
     full table (Where and Size included) and the total in the question itself, or in an
     option preview when the harness has one. A delete question without the table is
     invalid: never send one, and never re-ask with only a total.
   - **Every rescan gets a new question.** When the table changes (a rescan, an exclusion),
     ask again with the new table; an earlier yes does not cover it.
   - **Exclude:** remove that row's lines from the manifest (`grep -v` on the category
     column), print it again with `cleanup.sh table --manifest <file>`, and ask again.
     Removing lines is the only edit a manifest may get.
   - **Cancel:** stop. Leave the manifest in place.
5. **Delete** in the background; OpenCode sessions go through its CLI one at a time, so
   hundreds take minutes:

   ```bash
   bash <this-skill-dir>/scripts/cleanup.sh delete --manifest <scratch>/cleanup-manifest.tsv
   ```

6. **Report** the result table and the "Not deleted" reasons verbatim.

## What the script covers

| Tool | Targets |
|---|---|
| Claude Code | session transcripts and their subagent/tool-result folders, file history, todos, plans, jobs, shell snapshots, session env/state, paste cache, debug, telemetry, caches, backups, rotated security logs, `*.bak*` config backups, `*.tmp`, plugin versions no install record uses, the MCP log cache |
| Claude Desktop | agent-mode and code sessions, scratch workspaces, older bundled Claude Code versions, browser caches (only while the app is closed) |
| Codex | projectless chat folders (`<root>/<date>/<chat>`), only when `desktop.projectlessWorkspaceRoot` in its `config.toml` points inside `CODEX_HOME`; the default `~/Documents/Codex` is never touched |
| Docker | images in the current context that no container (running or stopped) uses and that were neither built nor tagged/pulled within N days; removed by tag without force, sizes are unique sizes. Never start Docker or switch its context for a scan |
| OpenCode | whole session trees (root and every child old), snapshots, tool output, logs, config backups, stale locks; the database is compacted afterwards when OpenCode is closed |

It never lists memory folders, settings, `CLAUDE.md`/`AGENTS.md`, prompt history, skills,
agents, commands, installed or newest plugin versions, credentials, the OpenCode database
file or the Desktop VM bundle. `delete` refuses any manifest line outside those folders, on a
protected path, or changed since the scan.

## When the user asks for more

- **A path outside these folders** (Downloads, a repo, `/tmp`): refuse in one sentence. This
  skill does not cover it, and a manifest line for it is refused anyway.
- **A target the script does not list:** say it is not covered. Do not delete it by hand.
- **Desktop caches skipped:** the app was running. Ask the user to quit it, then scan again.
- **Docker images skipped:** the daemon was not reachable. Ask the user to start Docker, then
  scan again.
- **"Database not compacted":** OpenCode was running. Pass on the printed `VACUUM` command.

## Testing

`bash scripts/cleanup.test.sh` runs every rule against a throwaway HOME. Run it after any
change to the script.
