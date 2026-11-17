# Docker credentials

Read this before the first `docker` command of a workspace session, and before any test run
that builds or pulls images.

The user's default Docker config (`~/.docker/config.json`, with its `credsStore`, such as the
macOS keychain) is for the user's own projects only. A workspace session (a company context)
never uses it: every build, pull or push there asks the keychain for the logins of every
registry in that config, and a prompt that the user cancels fails the run.

In a workspace session:

1. Create a scratch Docker config under the session's `.scratch/` dir. It holds only
   `credHelpers`, one entry per cloud registry host of the workspace. Map each host to its
   cloud helper: `ecr-login` (AWS ECR, `docker-credential-ecr-login`) or `gcloud` (GCP
   Artifact Registry, `docker-credential-gcloud`). Add no `credsStore` and no `auths`.
2. Symlink `cli-plugins` in it to `~/.docker/cli-plugins`, so `compose` and `buildx` work.
3. Export `DOCKER_CONFIG=<that dir>` and the cloud login of the workspace (for AWS,
   `AWS_PROFILE=<the workspace's profile>`) in every command that runs `docker`, or tests
   that build images. Runner briefs and test-session commands carry both exports.
4. Never run `docker login` into the default config, and never read or change
   `~/.docker/config.json`. A login that the cloud helper cannot get means the cloud login
   expired: ask the user to log in again.
5. Never push an image to a workspace registry from this machine. Copy or mirror images
   through the workspace's CI workflow.

```bash
D="<session base>/.scratch/dockercfg"; mkdir -p "$D"
ln -sfn "$HOME/.docker/cli-plugins" "$D/cli-plugins"
cat > "$D/config.json" <<'EOF'
{ "credHelpers": { "<account>.dkr.ecr.<region>.amazonaws.com": "ecr-login" } }
EOF
export DOCKER_CONFIG="$D" AWS_PROFILE=<workspace profile>
```

The registry hosts, the profile and the mirror workflow are workspace facts: the workspace's
own skill names them.
