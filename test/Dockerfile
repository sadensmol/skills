# Test image for the skills repository.
#
# Carries both harness CLIs and the tools this repository's own hooks need
# (git for the pre-commit hook, python3 for the secret scanner, jq for the
# assertions). Nothing repository-specific is baked in: the repository under
# test is mounted at run time, so the image is rebuilt only when a version
# below changes.
FROM node:22-slim

ARG CLAUDE_CODE_VERSION=2.1.276
ARG OPENCODE_VERSION=1.18.31
ARG SKILLS_CLI_VERSION=1.7.0

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates curl git jq python3 \
    && rm -rf /var/lib/apt/lists/*

RUN npm install -g \
        "@anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}" \
        "opencode-ai@${OPENCODE_VERSION}" \
        "skills@${SKILLS_CLI_VERSION}"

# UID 1000 is the base image's `node` user, so the agent user takes 1001.
RUN useradd -m -u 1001 agent \
    && mkdir -p /home/agent/repo /home/agent/.claude /home/agent/.agents \
                /home/agent/.config/opencode \
    && chown -R agent:agent /home/agent

USER agent
ENV HOME=/home/agent
WORKDIR /home/agent

CMD ["/home/agent/test/run-tests.sh"]
