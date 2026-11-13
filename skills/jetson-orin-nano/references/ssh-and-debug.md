# SSH and debugging details

Read this when the board is unreachable by IP, the SSH key is missing, a command needs
`sudo` or tricky quoting, you change a container's env, or you need the common read-only
checks.

**Host unreachable by IP? Use mDNS.** A recorded LAN IP goes stale (or the
board's routing drops it) and `ssh <user>@<ip>` just times out. The board's
`<hostname>.local` still resolves — try it before concluding the board is down,
and `hostname -I` over that session tells you the current addresses.

**⚠️ A container env change needs a RECREATE, never `docker restart`.** Compose
resolves `environment:` / `env_file` at container-CREATE time, so a restart
silently keeps the OLD values — `docker inspect <ctr> --format '{{.Config.Env}}'`
is the check that proves which the running container actually has. Re-run
`docker compose up -d --no-deps <service>` to apply. Pin `IMAGE_TAG` to the tag
already on disk; a tag the board never pulled makes compose hit the registry and
401 on a private repo.

**Password fallback (only if the key is ever missing).** A bare `ssh`/`scp` offers every agent key
first and the server drops you with **"Too many authentication failures"** before
the password. Force password-only, and drive it with `expect`:
```bash
/usr/bin/expect <<'EOF'
set timeout 60
spawn ssh -o PubkeyAuthentication=no -o PreferredAuthentications=password \
     -o IdentitiesOnly=yes -o NumberOfPasswordPrompts=1 \
     -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
     $env(JETSON_HOST) {docker ps --format "{{.Names}} {{.Status}}"; free -h}
expect { -re {[Pp]assword:} { send "<board-password>\r"; exp_continue } eof }
EOF
```
(Keep the password out of committed files — inject it, or use the key.)

**Quoting:** for `docker exec … python -c "…SQL with 'quotes'…"`, base64-encode the
payload and `echo <b64> | base64 -d | …` on the far side.

**`sudo`** needs a password: `echo '<board-password>' | sudo -S <cmd>`.

Common checks (single, read-only):
```bash
docker ps --format "{{.Names}} {{.Status}}"        # what's up
docker stats --no-stream --format "table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}"
free -h; swapon --show                              # RAM + swap
sudo tegrastats --interval 1000                     # GR3D_FREQ > 0% = GPU busy
docker exec <ollama-ctr> ollama ps                  # loaded models + PROCESSOR (want 100% GPU)
```
