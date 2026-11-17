#!/usr/bin/env bash
# remote-setup.sh <ssh-host> [--port N] [--alias NAME] — install or upgrade the autopilot remote box on a
# Linux host with Docker, from a Mac running agterm. The image is built on the host from a context streamed
# over ssh; the compose file is streamed too, so nothing is written on the host outside Docker.
# It ends with the box up and agterm-server installed; remote.sh then opens the disk (SIZE for a new one,
# default 100G) and runs the sign-ins in a new agterm session.
# Prints the box's ssh alias on the last line: `ALIAS=<alias>`.
set -euo pipefail

die() { echo "remote-setup: $*" >&2; exit 1; }

host="" port=2222 alias=""
while [ $# -gt 0 ]; do
  case "$1" in
    --port) port="${2:?--port needs a value}"; shift 2 ;;
    --alias) alias="${2:?--alias needs a value}"; shift 2 ;;
    -*) die "unknown option $1" ;;
    *) [ -z "$host" ] || die "one ssh host only"; host="$1"; shift ;;
  esac
done
[ -n "$host" ] || die "usage: remote-setup.sh <ssh-host> [--port N] [--alias NAME]"
[[ "$port" =~ ^[0-9]+$ ]] || die "the port must be a number"
if [ -z "$alias" ]; then
  short="${host#*@}"; short="${short%%.*}"
  alias="autopilot-$short"
fi
[[ "$alias" =~ ^[A-Za-z0-9._-]+$ ]] || die "the alias may hold letters, digits, dot, dash and underscore only"

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
box="$here/../remote"
ctl="${AGTERMCTL:-agtermctl}"
command -v "$ctl" >/dev/null || die "agtermctl is not on PATH"

H() { ssh -o BatchMode=yes -o ConnectTimeout=8 -o ServerAliveInterval=5 -o ServerAliveCountMax=3 "$host" "$@"; }

echo "== checking $host"
H 'uname -m' | grep -qxE 'aarch64|x86_64' || die "$host is not an aarch64 or x86_64 Linux host"
H 'docker version --format "{{.Server.Version}}" >/dev/null' || die "Docker does not answer on $host for this user"
H 'docker compose version >/dev/null' || die "Docker Compose is missing on $host"
if H "ss -ltnH 'sport = :$port' | grep -q ." && ! H 'docker ps -q --filter name=^autopilot-remote$ | grep -q .'; then
  die "port $port is taken on $host: pass --port"
fi

echo "== building the image on $host (the first build takes minutes)"
tar -C "$box" -cf - Dockerfile autopilot-disk autopilot-lock \
  autopilot-ssh-keys.service autopilot-disk-close.service | H 'timeout 3600 docker build -q -t autopilot-remote:local - >/dev/null'

echo "== starting the box"
# a running box closes its disk first: recreating it must not leave the disk open in the host kernel
if H 'docker ps -q --filter name=^autopilot-remote$ | grep -q .'; then
  H 'docker exec autopilot-remote /usr/local/sbin/autopilot-disk close' >/dev/null || die "cannot close the disk of the running box"
fi
H "AUTOPILOT_REMOTE_PORT=$port docker compose -f - up -d --remove-orphans" < "$box/compose.yaml"
for _ in $(seq 60); do
  H 'docker exec autopilot-remote test -f /var/lib/autopilot-remote/ssh/authorized_keys' 2>/dev/null && break
  sleep 2
done
H 'docker exec autopilot-remote test -f /var/lib/autopilot-remote/ssh/authorized_keys' \
  || die "the box did not come up: run: ssh $host docker logs --tail 40 autopilot-remote"

echo "== ssh access from this Mac"
key="$HOME/.ssh/$alias"
[ -f "$key" ] || ssh-keygen -q -t ed25519 -N '' -C "$alias" -f "$key"
H 'docker exec -i autopilot-remote sh -c "cat > /var/lib/autopilot-remote/ssh/authorized_keys"' < "$key.pub"
cfg="$HOME/.ssh/config"
touch "$cfg"; chmod 600 "$cfg"
tmp="$(mktemp)"
awk -v a="$alias" '
  $0 == "# autopilot-remote " a " begin" { skip = 1; next }
  $0 == "# autopilot-remote " a " end" { skip = 0; next }
  !skip' "$cfg" > "$tmp"
{
  printf '# autopilot-remote %s begin\n' "$alias"
  printf 'Host %s\n  HostName 127.0.0.1\n  Port %s\n  User agent\n  ProxyJump %s\n' "$alias" "$port" "$host"
  printf '  HostKeyAlias %s\n  IdentityFile %s\n  IdentitiesOnly yes\n  StrictHostKeyChecking accept-new\n' "$alias" "$key"
  printf '# autopilot-remote %s end\n\n' "$alias"
  cat "$tmp"
} > "$cfg"
rm -f "$tmp"
for _ in $(seq 30); do
  ssh -o BatchMode=yes -o ConnectTimeout=8 "$alias" true 2>/dev/null && break
  sleep 2
done
ssh -o BatchMode=yes -o ConnectTimeout=8 "$alias" true || die "cannot ssh into the box as $alias"

echo "== installing agterm-server in the box"
set +e
"$ctl" server install "$alias"
rc=$?
set -e
case "$rc" in
  0) ;;
  3) echo "remote-setup: agterm-server installed; its agent hooks wait for the open disk" >&2 ;;
  *) die "agtermctl server install failed (exit $rc)" ;;
esac

echo "ALIAS=$alias"
