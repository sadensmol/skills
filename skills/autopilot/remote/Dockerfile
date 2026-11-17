# The autopilot remote box: agterm-server, Claude Code, gh and the project toolchains under systemd,
# reached over ssh. It holds no secret: logins and workspaces live on the encrypted disk that the  # pragma: allowlist secret
# `/autopilot remote` session opens after each start; its steps come from the skill, never from this image.
FROM ubuntu:24.04

ARG GO_VERSION=1.27.4
ARG GH_VERSION=2.104.0
ARG NODE_MAJOR=22
ARG BOX_USER=agent

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update -qq \
 && apt-get install -y -qq --no-install-recommends \
      systemd systemd-sysv dbus dbus-user-session openssh-server sudo \
      zsh bash procps less vim-tiny jq git make gcc libc6-dev sqlite3 curl ca-certificates gnupg \
      cryptsetup dmsetup e2fsprogs util-linux psmisc python3 python3-venv docker.io \
 && rm -rf /var/lib/apt/lists/* /etc/ssh/ssh_host_*

RUN arch="$(dpkg --print-architecture)" \
 && curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${arch}.tar.gz" | tar -xz -C /usr/local \
 && curl -fsSL "https://github.com/cli/cli/releases/download/v${GH_VERSION}/gh_${GH_VERSION}_linux_${arch}.tar.gz" \
    | tar -xz -C /tmp \
 && install -m 0755 "/tmp/gh_${GH_VERSION}_linux_${arch}/bin/gh" /usr/local/bin/gh \
 && rm -rf /tmp/gh_* \
 && curl -fsSL "https://deb.nodesource.com/setup_${NODE_MAJOR}.x" | bash - >/dev/null \
 && apt-get install -y -qq --no-install-recommends nodejs \
 && rm -rf /var/lib/apt/lists/* \
 && curl -fsSL https://astral.sh/uv/install.sh | env UV_INSTALL_DIR=/usr/local/bin INSTALLER_NO_MODIFY_PATH=1 sh \
 && npm install -g --silent @anthropic-ai/claude-code \
 && npm cache clean --force
ENV PATH=/usr/local/go/bin:$PATH
RUN printf 'export PATH="/usr/local/go/bin:$HOME/go/bin:$HOME/.local/bin:$PATH"\nexport DISABLE_AUTOUPDATER=1\nexport CLAUDE_CONFIG_DIR="$HOME/.claude"\n' \
    | tee /etc/profile.d/autopilot.sh >> /etc/zsh/zshenv

# The box user: lingering for its systemd user manager (agterm-server), docker access for its own engine,
# and root only for opening and closing the disk.
RUN useradd -m -s /bin/zsh "$BOX_USER" \
 && usermod -aG docker "$BOX_USER" \
 && mkdir -p /var/lib/systemd/linger && touch "/var/lib/systemd/linger/$BOX_USER" \
 && printf '%s ALL=(root) NOPASSWD: /usr/local/sbin/autopilot-disk\n' "$BOX_USER" > /etc/sudoers.d/autopilot \
 && chmod 0440 /etc/sudoers.d/autopilot \
 && touch "/home/$BOX_USER/.zshrc" && chown "$BOX_USER:$BOX_USER" "/home/$BOX_USER/.zshrc"

# sshd: key login only; the host key and the authorized key live in the state volume.
RUN printf '%s\n' \
      'HostKey /var/lib/autopilot-remote/ssh/ssh_host_ed25519_key' \
      'AuthorizedKeysFile /var/lib/autopilot-remote/ssh/authorized_keys' \
      'PasswordAuthentication no' 'KbdInteractiveAuthentication no' 'PermitRootLogin no' \
      "AllowUsers $BOX_USER" > /etc/ssh/sshd_config.d/autopilot.conf

# Docker in the box starts only after the disk is open, with its data on the disk.
RUN mkdir -p /etc/docker && printf '{"data-root": "/secure/docker"}\n' > /etc/docker/daemon.json \
 && systemctl disable docker.service docker.socket containerd.service >/dev/null 2>&1 || true

COPY autopilot-disk /usr/local/sbin/autopilot-disk
COPY autopilot-lock /usr/local/bin/
COPY autopilot-ssh-keys.service autopilot-disk-close.service /etc/systemd/system/
RUN chmod 0755 /usr/local/sbin/autopilot-disk /usr/local/bin/autopilot-lock \
 && systemctl enable autopilot-ssh-keys.service autopilot-disk-close.service ssh.service >/dev/null 2>&1

STOPSIGNAL SIGRTMIN+3
CMD ["/sbin/init"]
