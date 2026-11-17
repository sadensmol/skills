# Remote box

Autopilot can run in one Docker container, the box, on any Linux host with Docker and Docker Compose that
the owner reaches over ssh. The Mac's agterm shows each box session it needs as a new session right after
the calling one, attached to agterm-server in the box. The box image is
built from `remote/` in this skill; it holds no secret. Logins, workspaces and the box's own Docker data
live on an encrypted disk inside the box, opened with a passphrase that only the owner types.

These verbs run in a Mac session inside agterm (`sadensmol-agterm` for every `agtermctl` call).

**Everything runs in one agterm session of the box (MUST FOLLOW).** `/autopilot remote` creates a session
in the box's agterm-server, attaches it here as a NEW session with `agtermctl zmx attach`, and moves it right
after the calling session (`session move --after`). Every box step runs in that session, from
`scripts/box-session.sh` of this skill, sent as the session's command: nothing of it is stored in the box.
The owner types the passphrase and the sign-ins there, and Claude starts there at the end. Never run a box
step over a plain `ssh -t`, never copy a script into the box, never open an overlay on an existing session,
and never open a remote window (`window remote open`) or any other window: the calling Claude session stays
as it is. Plain ssh carries only data the box cannot read from the Mac, and the script's status reads.

## Remote

`remote [--size SIZE]` is the one verb the owner needs, the first time and every time after. Run it from a
Mac session whose cwd is inside the workspace folder.

**A workspace has one remote, so `remote` takes no host (MUST FOLLOW).** Never ask for one and never pass
one. The box is the `box_alias` of `<registry>/autopilot/remote.yaml`; before the first run, it is the one
`# autopilot-remote` block that `remote setup <ssh-host>` wrote to `~/.ssh/config`. No block: tell the user
to run `/autopilot remote setup <ssh-host>` first. Several blocks and no `remote.yaml`: ask the user which
box this workspace uses, then pass `--alias NAME`. `--size` sizes a new disk only.

What one run does:

1. On the Mac (`scripts/remote.sh`): the box answers over ssh, else `remote-setup.sh` installs it again
   with the host and port of its `~/.ssh/config` block; agterm-server runs in it. It refuses the first
   move while autopilot runs for this workspace on the Mac (one service per workspace).
2. On the Mac: it lists what the box must have, the same as this Mac:
   - every skill of `~/.claude/skills` (with a `SKILL.md`): a git-backed one as a link into a mirror of its
     repo, any other one as a copy;
   - `~/.claude/agents` and `~/.claude/CLAUDE.md` when they link into a repo;
   - every plugin marketplace and installed plugin; a directory marketplace from its repo's mirror;
   - the autopilot source repo, and every project repo of the registry;
   - `.workspace/` (without backups, the service state and `remote.yaml`), and the router routes files.
   Each mirrored repo with uncommitted or unpushed work prints `COMMIT AND PUSH: <repo>`: the box clones
   from GitHub, so it gets only what is pushed.
3. In the box session (`scripts/box-session.sh`), named `auto main` in the box workspace named like this
   one: open the disk (the owner types the passphrase; a new disk asks a new one twice, and losing it loses
   the disk) and link the logins and the workspaces into it; sign in to Claude and GitHub when signed out.
   Then it takes the Mac's data, moves `.workspace/` in when the box lacks it, clones or updates every
   mirror (reset to origin's copy of the Mac's branch) and clones missing project repos, makes
   `~/.claude/skills` exactly the Mac's set, adds the marketplaces and plugins, rewrites Mac paths in
   `.workspace/`, checks the registry, the settings, the secret keys (names only) and the source folder,
   and trusts the workspace folder for Claude Code. Each failed check prints `NOT READY: <what>`.
4. All checks pass: `claude /autopilot` runs in that same session. When `auto main` already runs, the
   session is a short `autopilot sync` instead; it closes when it passes, and `auto main` is attached here.
   A failed check: the session stops with a shell open, and the owner reads what is NOT READY there.

Run it like this:

1. When the box does not exist yet and the host is a board or machine that a workspace skill guards, ask
   the user first (*Setup* step 1).
2. Run the script as a `run-background-shell` and wait for its exit; it waits for the owner in the session:

   ```bash
   bash <this-skill-dir>/scripts/remote.sh [--size SIZE] [--force] [--alias NAME]
   ```

3. **When the output shows the box session, tell the owner in one line:** the session `auto main` (or
   `autopilot sync`) is open right after this one; type the disk passphrase there when it asks, then
   follow the Claude and GitHub sign-ins.
4. A failure prints `remote: <reason>` (or a `remote-setup:` line). Show it and stop. When it refuses
   because autopilot runs for this workspace on the Mac, ask the user before you stop that service.
5. **Report** in two lines: `auto main` runs in the box, shown here as the session of the last line
   `AUTO_MAIN=<id>` (write it as an `agterm://session/<id>` link); and every `COMMIT AND PUSH` repo, so the
   user commits and pushes it and runs `remote` again.

Never type the passphrase, read it, or ask for it: the owner types it at the prompt of the box session.

## Setup

`remote setup <ssh-host> [--port N] [--alias NAME]` installs the box, or upgrades it when it
exists. It runs from the Mac. An upgrade keeps the box's data and restarts the box, which closes the disk:
the next `remote` opens it again.

1. **Ask the user first** when the host is a board or machine that a workspace skill guards, and say what
   setup does there: build an image, start one privileged container with two named volumes, and listen on
   the host's loopback port (default 2222). Nothing else is written on the host.
2. Run the script as a `run-background-shell` and wait for its exit; the first build takes minutes:

   ```bash
   bash <this-skill-dir>/scripts/remote-setup.sh <ssh-host> [--port N] [--alias NAME]
   ```

   It checks the host (aarch64 or x86_64, Docker, Compose, a free port), streams the build context and
   the compose file over ssh, creates the box-only key `~/.ssh/<alias>`, writes the `# autopilot-remote
   <alias>` block of `~/.ssh/config` (through the host as a jump host), installs agterm-server in the box
   with `agtermctl server install <alias>`. It opens no window.
3. A failure prints `remote-setup: <reason>`. Show that line and stop.
4. **Report** in one line: the alias (last line `ALIAS=<alias>`), and that `/autopilot remote` from the
   workspace folder now brings the workspace into the box.

## Migrate

`remote migrate [--force]` is *Remote*: its first run moves the workspace in by itself. With `--force` it
moves `.workspace/` in again from the Mac, and the box keeps its old one as a `.workspace.bak-*` folder. Run
`bash <this-skill-dir>/scripts/remote.sh --force` as *Remote* says.

## In the box

The image holds only what needs root or a system service; every step of the owner's flow comes from
`scripts/box-session.sh`.

- `autopilot-disk` (root, through sudo) — open, close and report the encrypted disk.
- `autopilot-lock` — close every session that works on the disk, stop the box's Docker, close the disk.
