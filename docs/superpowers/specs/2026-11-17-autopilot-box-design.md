# Autopilot box: autopilot on any Linux host, in one container

Status: draft for the owner's review (2026-11-17). Project, full design.

## Intent

The owner wants autopilot to run on an always-on Linux machine (first target: a Jetson Orin Nano on the
LAN) instead of the Mac, without installing anything on that machine except what Docker already
provides. The owner reaches it from the Mac's agterm as a remote window and logs in to `claude` and
`gh` there by hand, in a real terminal. One container serves every workspace the owner has on the Mac.
The owner can stop the whole thing at any time to free the machine's memory, and remove it without a
trace. Workspace data (tokens, project registry, settings) moves from the Mac in a way nobody else can
read in transit or from a stolen disk.

## Goals

1. **One setup command.** `/autopilot remote setup <ssh-host>` installs or upgrades the box on any Linux host
   that has Docker, Docker Compose and sshd, on aarch64 or x86_64. Running it again upgrades the box and
   keeps its data.
2. **Remote window.** After setup, the owner's Mac has an ssh alias for the box, agterm-server runs in
   the box, and `agtermctl window remote open <alias>` shows its sessions on the Mac.
3. **Logins in the box.** The owner runs `claude` (`/login`) and `gh auth login` in a box session. The
   logins stay in the box's encrypted storage.
4. **All workspaces.** One box holds any number of workspaces (each an agterm workspace and a workspace
   folder with `.workspace/`). Each runs its own `auto main` control room and autopilot service, as on
   the Mac.
5. **Secure migration.** `/autopilot remote migrate <ssh-host>` copies the current workspace's `.workspace/`
   from the Mac into the box: encrypted in transit, written only into encrypted storage, never onto the
   host's own disk outside the box's Docker volume.
6. **Encrypted at rest.** Everything that holds code, access or data sits on one LUKS-encrypted disk image
   inside the box's volume: workspaces, repos, logins, skills, and the box's own Docker data. It opens only
   with a passphrase the owner types after each box start, from the Mac (goal 7).
7. **Stop and start from the Mac.** `/autopilot remote stop|start|status` stops the whole box
   (every session, Claude, autopilot) or starts it again. `start` then asks for the passphrase in a
   terminal on the Mac, opens the disk, and every migrated workspace gets its `auto main` back.

## Non-goals

- No install on the host itself: no packages, no systemd units, no files in the host user's home. The
  only host state is the Docker image, the container and one named volume.
- No migration of `autopilot.db`: its runs point at Mac sessions and paths. A box workspace starts with
  no runs.
- No copy of repos from the Mac: the box clones them with its own `gh` login. Unpushed Mac work stays on
  the Mac.
- No unattended restart after a host reboot: the passphrase is needed first (goal 6). This is accepted.
- Not the task flow: the box is for autopilot only.

## Anti-goals

- A secret in the image, the Dockerfile, the compose file, a build argument, an env var of the
  container, a command line (`ps`), a log, or the skills repository. Any of these makes the design a
  failure even if it works.
- A port of the box that the LAN can reach. Only the host's sshd is exposed, as today.
- A box that needs the owner to edit files on the host.

## Constraints

- The skills repository is public: everything in `skills/autopilot/remote/` is generic. No host names,
  user names, paths or workspace names (repo `CLAUDE.md`).
- agterm-server needs a systemd user manager with lingering and an ssh login into its user. agterm's own
  test container (`scripts/server-container.sh` in the agterm repo) proves the recipe: Ubuntu 24.04,
  `systemd` as PID 1, `--privileged --cgroupns=private`, tmpfs `/run`, sshd with key login, and
  `agtermctl server install` run against that sshd.
- `agtermctl server install DESTINATION [-p PORT] [-i FILE] [-J HOST] [-F FILE]` installs the release
  that matches the Mac app's bundle, for the host's CPU. It runs on the Mac only.
- agterm-server serves every command autopilot uses (`session new/move/rename/text/type/status/context/
  info/close`, `ask`, `msg`, `tree`). It has no event hooks and marks every session `ended` after a box
  restart, so the box must recreate the `auto main` sessions itself.
- The autopilot service is pure Go (`modernc.org/sqlite`), so the box builds it with `make build`.
- The encrypted disk is opened inside the privileged box with `cryptsetup` on a loop device. The host
  kernel needs `dm_crypt` and `loop`; the Jetson has both (`dm_crypt` as a module, loaded on first open,
  in memory only). Docker's `overlay2` needs a real filesystem, which a FUSE folder (gocryptfs) is not;
  ext4 on the opened disk is.
- Board limits (Jetson, 8 GB): one image build at a time; no second heavy container.

## Approaches

**Getting the box onto the host** (owner picked 1):

1. **Build on the host.** Setup copies `box/` over ssh and runs `docker compose up -d --build` there.
   Native for any CPU, no Docker on the Mac, one source of truth in the skill. The first build on a small
   board takes minutes.
2. Build on the Mac, ship with `docker save | ssh docker load`. Faster updates on slow boards; needs Docker
   and CPU emulation on the Mac.
3. A prebuilt image from a registry, built by CI. Fastest; needs a CI workflow, and the image is public.

**Encryption at rest** (owner asked to encrypt everything, the box's Docker data included):

1. **A LUKS disk image file in the volume, opened inside the box** (picked). `cryptsetup` is in the image,
   not on the host. One ext4 filesystem holds all data, Docker's `overlay2` included. Protects a stolen disk
   and a stopped box; not the running, unlocked box from host root.
2. gocryptfs folder. No kernel module, but Docker data cannot live on FUSE, so the box's test containers
   and images would stay in clear.
3. Plain volume with file modes. Restarts unattended, but a stolen disk reads in clear.

**Moving workspace data** (my call):

1. **`tar` over ssh straight into the unlocked encrypted disk** (picked). One encrypted hop through the
   host's sshd (`-J`) into the box's sshd. Nothing lands on the host's disk outside the volume.
2. An encrypted archive copied to the host first (age/gpg), decrypted in the box. Leaves a file on the
   host and needs a second secret.

## Design

### Layout in the skill

```
skills/autopilot/remote/
  Dockerfile          Ubuntu 24.04, systemd, sshd, zsh, git, gh, go, python + uv, node + npm, make,
                      sqlite3, jq, cryptsetup, Docker engine (not started until the disk is open),
                      claude (official installer, as the box user), no secrets
  compose.yaml        one service "box": privileged, cgroupns private, tmpfs /run, /dev/fuse,
                      sshd on 127.0.0.1:<port> of the host only, one named volume for /home/<box user>
  autopilot-unlock          in-box script: open and mount the encrypted disk, start Docker, start every workspace
  autopilot-start-workspaces in-box script: recreate each migrated workspace's `auto main` session
references/remote.md     the setup, migrate, box and unlock procedures of the skill
```

### Network and identity

- The box's sshd listens only on the host's `127.0.0.1:<port>`. The Mac reaches it with the host as a
  jump host. Setup writes a Mac `~/.ssh/config` entry `<alias>` = `ProxyJump <ssh-host>`,
  `HostName 127.0.0.1`, `Port <port>`, the box user, and a box-only key.
- Setup creates that key on the Mac (ed25519, `~/.ssh/<alias>`), and puts only its public half into the
  box's `authorized_keys`, at setup time over ssh, not in the image. Password login is off.
- The box's sshd host key is created on the first box start and kept in the volume, so the Mac's
  `known_hosts` stays valid across upgrades.

### Encrypted storage

- The volume holds `disk.img`, a sparse LUKS2 image (size set at setup, default 100 GB), and nothing else
  of value. `autopilot-unlock` opens it with `cryptsetup` on a loop device and mounts its ext4 at `/secure`.
- Inside `/secure`: `home/` (workspaces under `workspaces/<workspace>/`, each with `.workspace/` and its
  repos; `.claude/`; `.config/gh/`; the skills clones) and `docker/` (the box's Docker `data-root`).
  The box user's `~/.claude`, `~/.config/gh` and `~/workspaces` are symlinks into `/secure/home`, so code and
  logins exist only while the disk is open.
- Outside the disk, in clear: the OS image, agterm-server with its state and zmx sockets, the sshd host key
  and the box's `authorized_keys`. They hold no code, data or tokens, and they let the remote window and
  ssh work while the box is locked.
- Docker in the box starts only after the disk is open, with `data-root` on it.
- The passphrase is set the first time `autopilot-unlock` runs (`cryptsetup luksFormat`). It is never stored,
  never an argument, never an env var, and never passes through Claude: the owner types it into a
  terminal prompt (below).
- **Access to the volume.** Only the box mounts it: the compose file names it, no other service uses it,
  and the box never gets the host's Docker socket. On the host, only root and the `docker` group reach it,
  and while the box is locked they see ciphertext only. While the box runs unlocked, host root can read it;
  no design prevents that.

### Passphrase entry from the Mac

`/autopilot remote start` runs `docker compose start` over ssh, then opens a program overlay in
the owner's own session running `ssh -t <alias> autopilot-unlock`. The owner types the passphrase at that
terminal prompt: keystrokes go from the Mac keyboard over ssh to `cryptsetup`, and never into an
`agtermctl ask` answer, a file or the Claude transcript. The overlay's exit code tells the skill whether
the disk opened; on success it reports which workspaces got their `auto main` back. A box session on the
remote window can run `autopilot-unlock` directly too.

### Commands of the skill (new rows in the mode table)

Every new command is a `remote` verb of `/autopilot` (owner's request): `/autopilot remote` alone, or with `setup`, `migrate`, `start`, `stop` or `status`.

| Command | Runs on | Does |
|---|---|---|
| `remote setup <ssh-host>` | Mac session | checks the host (Docker, Compose, CPU), copies `box/`, `docker compose up -d --build`, writes the ssh alias and key, runs `agtermctl server install <alias>`, opens the remote window, tells the owner to run `autopilot-unlock`, then `claude` `/login` and `gh auth login` |
| `remote migrate <ssh-host> [--force]` | Mac session, in a workspace | refuses when the box is locked; streams `.workspace/` without `*.bak` and without `autopilot/autopilot.db` into `~/workspaces/<workspace>/` on the encrypted disk, with modes 0700/0600; rewrites the Mac paths in `autopilot/config.yaml` (`service.source_dir`, `repo_paths`); clones the workspace's repos with the box's `gh`; creates the agterm workspace and its `auto main`; writes `remote.yaml` |
| `remote` | Mac session, in a workspace | start, unlock and open this workspace's autopilot in its box (*The remote of a workspace*) |
| `remote stop\|start\|status` | Mac session | `stop` closes every workspace, stops Docker, unmounts and closes the disk, then `docker compose stop`; `start` = `docker compose start`, then the passphrase overlay (above); `status` = `docker compose ps` and whether the disk is open |
| `autopilot-unlock` | box terminal, typed by the owner | opens the disk, starts Docker, runs `autopilot-start-workspaces` |

All four refuse inside a run worktree, as the control-room verbs do.

### The remote of a workspace (owner's request)

- A workspace has at most one remote box. `migrate` records it on the Mac in
  `<workspace folder>/.workspace/autopilot/remote.yaml` (mode 600, never committed, never copied into the
  box): `ssh_host` (the host's ssh destination) and `box_alias` (the Mac ssh alias of the box). A second
  `migrate` to a different host refuses until the owner removes `remote.yaml`.
- `/autopilot remote`, run from any Mac session of that workspace, reads `remote.yaml` and needs no
  other argument. In order: start the box when it is stopped; when its disk is closed, open the passphrase
  overlay (*Passphrase entry from the Mac*); make sure the workspace's `auto main` runs in the box (start
  it when missing); open or raise the remote window. It reports what it started. Without `remote.yaml` it
  stops and says to run `migrate` first.
- Plain `/autopilot` on the Mac refuses when `remote.yaml` exists, and points to `/autopilot remote`. With
  `migrate`'s own check, this keeps one autopilot per workspace from the Mac side.
- `remote stop|start|status` take the host from `remote.yaml`, so they need no argument.

### Migration rules (builder check, my calls)

- **One service per workspace.** Both copies would share the Slack Socket Mode tokens and the Linear
  workspace, so Slack would split events and both could take the same issue. `migrate` refuses while
  `autopilot ping` on the Mac answers `pong` for that workspace, and tells the owner to stop it first.
  After a migrate the workspace's autopilot lives in the box.
- **Private workspace skills.** For each installed `<workspace>-*` skill link on the Mac, `migrate` reads
  the origin URL of the repository the link points into (`git remote get-url origin`) and clones it in the
  box with the box's `gh`, then links its skills as the Mac install does. It writes the box's own
  `~/.claude/sadensmol-router-routes.json` entry for the workspace with the box path. No URL is written in
  the public repository.
- **Toolchains.** The image holds the three gate profiles' tools: Go, Python with `uv`, Node with `npm`. A
  workspace that needs more tools keeps an optional `.workspace/autopilot/box-setup.sh`; `migrate` runs it
  in the box as root after the copy. It is the owner's own script, from the owner's own data.
- **A second migrate** of a workspace the box already holds refuses without `--force`. With `--force` it
  first copies the box's `.workspace/` to `.workspace.bak-<timestamp>/` on the encrypted disk, then overwrites
  it (still never `autopilot.db`).

### After a box start

The disk is closed and every session from before reads `ended`. The owner runs `/autopilot remote start` on the Mac (or `autopilot-unlock` in a box session). After the disk opens, `autopilot-start-workspaces`
closes the ended `auto main` sessions and starts a new `auto main` in each workspace under
`~/workspaces/` that holds `.workspace/autopilot/`.
That session runs `claude /autopilot`, which builds and starts the service as today.

## Left to the builder

- The box user name, the default port, and the volume and image names (generic, overridable by flags).
- How setup finds the CPU and checks Docker and Compose on the host, and its messages.
- How `migrate` reads the repo list of a workspace (the registry's `projects/*.json` `repos`), and the
  clone URL form.
- How `autopilot-start-workspaces` names and orders sessions, as long as each workspace gets one `auto main`.
- The update path of the skills clone in the box (a `git pull` on each `setup` run is fine).
- Tests: hermetic script checks in the style of `harness/*/install.test.sh`, and one container check in
  `test/` that builds the image and runs setup against a local host container.

