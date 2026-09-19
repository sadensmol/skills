# Container checks

Installs this repository into a throwaway container exactly the way `README.md`
tells a human to install it, then asserts that both harnesses resolve what it
ships — and that an edit made through either harness's install path lands back
in the repository.

```bash
docker compose -f test/compose.yaml run --rm test     # the checks
docker compose -f test/compose.yaml run --rm shell    # a shell in the same image
docker compose -f test/compose.yaml build             # after a version bump below
```

## Safety

The repository is mounted **read-only** at `/src` and copied to `$HOME/repo` inside
the container before anything installs. Every check — including the ones that
deliberately edit a skill — writes only to that copy. A failed or interrupted
run cannot touch your working tree.

## No credentials, no model calls

Both CLIs are real, and both are driven through their offline introspection
commands: `claude plugin list`, `claude plugin validate`, the plugin's
`UserPromptSubmit` hook, `opencode debug skill`, `opencode debug config`. None
of these needs a login or spends tokens, so the run is repeatable offline and in
CI.

The one thing this cannot prove is that a live session received the router text.
That needs `claude -p` / `opencode run`, which needs a login — deliberately out
of scope here.

## What is checked

| Group | Asserts |
| --- | --- |
| Claude Code | every skill is linked under its frontmatter name and resolves into the repository; `agents/` linked; `CLAUDE.md` → `instructions/AGENTS.md`; the plugin installs; the hook injects the router **and** the harness runtime file |
| OpenCode | the namespace link, `AGENTS.md`, `agents/`; `opencode debug skill` resolves every skill from this repository; no name is registered twice; the router and the runtime file are in `instructions`; `harness/opencode/install.test.sh` passes |
| Validity | the skills CLI lists every skill (a skipped one has broken frontmatter); both manifests validate; agent frontmatter carries `name`, `description`, `mode: subagent` and no named `color:` |
| Edits sync | an edit through `~/.claude/skills`, through `~/.agents/skills`, or to the instructions file lands in the repository; both harnesses see a repository edit with no re-install; reverts clear everywhere |
| A new skill | re-running the install links it and OpenCode picks it up; no dangling links are left behind |

## Versions

The CLI versions are build arguments in `Dockerfile`, pinned so a run fails for
your reasons rather than an upstream release:

```bash
docker compose -f test/compose.yaml build \
  --build-arg CLAUDE_CODE_VERSION=<v> --build-arg OPENCODE_VERSION=<v>
```

## Testing more than one repository together

`install.sh` and `run-tests.sh` take repository paths as arguments, and a
repository contributes whatever it has (`skills/`, `agents/`,
`instructions/AGENTS.md`, `harness/opencode/opencode.md`, a `plugin/`). The namespace is
read from `.claude-plugin/marketplace.json`, so nothing is hardcoded. Mount a
second repository and pass both paths to check that two namespaces install side
by side without colliding.
