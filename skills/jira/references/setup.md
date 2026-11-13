# First-Time Setup

Read this when the Backend check in `SKILL.md` prints `missing JIRA_API_TOKEN`, or the base URL / email cannot be resolved.

When the Backend check prints `missing JIRA_API_TOKEN`, print the complete instruction below in one message.
**Secrets live in `~/.profile` and are added by the USER, never by Claude** —
never write, echo, or edit a token, never print `$JIRA_API_TOKEN`, never put it
in a repo file.

### Creating a Jira API token (Cloud)

1. Sign in to Atlassian, then open
   **https://id.atlassian.com/manage-profile/security/api-tokens**
   (Atlassian account → Security → "Create and manage API tokens").
2. Click **Create API token**. (For scoped tokens, choose "Create API token
   with scopes" and grant Jira read/write — classic unscoped tokens work too.)
3. Give it a label (e.g. `claude-cli`) and an expiry, click **Create**.
4. **Copy the token immediately** — Atlassian shows it only once. If lost,
   revoke it and create a new one.
5. Put it in `~/.profile` yourself as `JIRA_API_TOKEN` (see below). It pairs
   with your account email for HTTP Basic auth (`email:token`).

> Jira **Server / Data Center** (self-hosted) has no id.atlassian.com — create a
> **Personal Access Token** under your Jira profile → Personal Access Tokens,
> and it's sent as a Bearer header instead of basic auth (see `references/rest.md`).

```
Jira works with zero installs — I'll call the Jira Cloud REST API with curl
(already on your Mac). One-time setup:

1. Create an API token (Jira Cloud) — see "Creating a Jira API token" above:
     https://id.atlassian.com/manage-profile/security/api-tokens
   (Jira Server/DC: create a Personal Access Token in your profile instead.)

2. Add these to ~/.profile YOURSELF (I must never see or write the token):
     export JIRA_BASE_URL="https://<yourorg>.atlassian.net"   # no trailing slash
     export JIRA_EMAIL="you@example.com"
     export JIRA_API_TOKEN="<paste your token>"

   Your shell is zsh, which does NOT read ~/.profile by default. Add this
   line to ~/.zprofile so the vars load in new terminals:
     [ -f ~/.profile ] && . ~/.profile

3. Apply to the CURRENT terminal:
     source ~/.profile

4. Tell me when done — I'll verify with:
     curl -s -u "$JIRA_EMAIL:$JIRA_API_TOKEN" "$JIRA_BASE_URL/rest/api/2/myself"
```

After the user confirms, re-run the Backend check, then hit
`/rest/api/2/myself`. A 200 with the account = ready; a 401/403 = fix
email/token/scopes. Then continue with the user's original request.

**Env vars won't appear in an already-running Claude session.** This process's
env was captured at launch, so `source ~/.profile` in an interactive `!` shell
updates *that* shell, not the agent's shell. If the vars still read as missing
after setup, the user must restart the Claude session (or export them in the
environment Claude was launched from).

> Jira Server / Data Center (self-hosted, not Cloud) uses a Personal Access
> Token as a Bearer header instead of basic auth — see `references/rest.md`.
