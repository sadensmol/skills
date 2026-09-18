# Jira Skill (REST-only)

Natural-language Jira management for Claude, driven entirely by the **Jira Cloud
REST API via `curl`**. Nothing to install and no server to configure — the skill
tells Claude how to call the REST API with three environment variables.

## How it works

A skill is instructions, not an executable — it can't reach Jira by itself. This
skill drives `curl` (already on macOS) against the Jira Cloud REST API using
HTTP Basic auth (`email:API-token`). That's the whole mechanism.

## Setup (one-time)

1. Create an API token (Jira Cloud):
   https://id.atlassian.com/manage-profile/security/api-tokens
   (Jira Server/DC: create a Personal Access Token instead.)
2. Add these to `~/.profile` yourself — Claude never sees or writes the token:
   ```bash
   export JIRA_BASE_URL="https://<yourorg>.atlassian.net"   # no trailing slash
   export JIRA_EMAIL="you@example.com"
   export JIRA_API_TOKEN="<paste your token>"
   ```
3. zsh doesn't read `~/.profile` by default — add to `~/.zprofile`:
   ```bash
   [ -f ~/.profile ] && . ~/.profile
   ```
4. `source ~/.profile`, then verify:
   ```bash
   curl -s -u "$JIRA_EMAIL:$JIRA_API_TOKEN" "$JIRA_BASE_URL/rest/api/2/myself"
   ```

> Env vars set after Claude launched won't appear in an already-running session
> — restart Claude (or export them in the shell Claude is launched from).

## Files

- `SKILL.md` — the skill: backend check, quick reference, workflow, safety
  rules, first-time setup.
- `references/rest.md` — full curl recipes (view, JQL search, create,
  transition, comment, assign), API v2-vs-v3 notes, error handling, and the
  Jira Server/DC Bearer-auth variant.

## Notes

- Uses `/rest/api/2` for reads and plain-text comment/description bodies;
  `/rest/api/3` only when ADF rich content is needed.
- Assignment is by `accountId`, never display name.
- Always fetch valid transitions before transitioning — ids/names are
  per-workflow and depend on the current status.
- The token is never printed, echoed, or written to a file; it's passed only via
  curl's `-u`.
