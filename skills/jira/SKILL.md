---
name: sadensmol-jira
description: Use when the user mentions Jira issues (e.g., "PROJ-123"), asks about tickets, wants to create/view/update issues, check sprint status, or manage their Jira workflow. Triggers on keywords like "jira", "issue", "ticket", "sprint", "backlog", or issue key patterns.
---

# Jira

Natural language interaction with Jira through the **Jira Cloud REST API,
driven by `curl`**. This is the only backend. A skill can't reach Jira on its
own; it drives `curl` (already on macOS) against the REST API, so nothing needs
installing — only three env vars.

## Config resolution

Three values drive every call. Resolve each in this order:

1. **Env var** (`JIRA_BASE_URL`, `JIRA_EMAIL`, `JIRA_API_TOKEN`) — highest.
2. **A loaded project skill's default** — a `<project>-<project>` dev skill may
   supply the site URL and email for its workspace, so the user only has to
   provide the token. Use those defaults when the env var is unset.
3. Otherwise unknown → **First-Time Setup**.

**`JIRA_API_TOKEN` is the only true secret and must come from the environment**
(the user's `~/.profile`) — a project skill never carries the token. Base URL
and email are not secret and may come from either source.

**Backend check.** Confirm the token is present WITHOUT printing it, and that a
base URL + email are known (from env or a project-skill default):

```bash
[ -n "$JIRA_API_TOKEN" ] && echo "token ok" || echo "missing JIRA_API_TOKEN"
```

- token set AND base URL + email resolved → use the REST recipes below (full
  patterns in `references/rest.md`).
- token missing → go to **First-Time Setup**; give the user the FULL
  instruction in one message, don't drip-feed steps.

| Var | Example | Meaning |
|-----|---------|---------|
| `JIRA_BASE_URL` | `https://yourorg.atlassian.net` | Site URL, no trailing slash (env OR project-skill default) |
| `JIRA_EMAIL` | `you@example.com` | Atlassian account email (env OR project-skill default) |
| `JIRA_API_TOKEN` | `ATATT3x...` | API token — **secret, env only** |

When base URL / email come from a project skill, substitute their literal values
into the curl calls instead of `$JIRA_BASE_URL` / `$JIRA_EMAIL` (only the token
stays a `$JIRA_API_TOKEN` reference — never inline it).

---

## Quick Reference (REST / curl)

> Full recipes + error handling in `references/rest.md`. Reusable auth prefix
> (the token is only expanded inside curl — never echo it):
> `J=(-s -u "$JIRA_EMAIL:$JIRA_API_TOKEN" -H "Accept: application/json")`.
> Use `/rest/api/2` for reads and plain-text bodies; `/rest/api/3` only when you
> need ADF rich content.

| Intent | Command |
|--------|---------|
| Who am I / verify auth | `curl "${J[@]}" "$JIRA_BASE_URL/rest/api/2/myself"` |
| View issue | `curl "${J[@]}" "$JIRA_BASE_URL/rest/api/2/issue/KEY?fields=summary,status,assignee,description"` |
| Search (JQL) | `POST "$JIRA_BASE_URL/rest/api/2/search/jql"` with `{"jql":"...","fields":["summary","status"]}` |
| Create issue | `POST "$JIRA_BASE_URL/rest/api/2/issue"` with `{"fields":{...}}` |
| List transitions | `curl "${J[@]}" "$JIRA_BASE_URL/rest/api/2/issue/KEY/transitions"` |
| Transition | `POST ".../issue/KEY/transitions"` with `{"transition":{"id":"NN"}}` |
| Add comment | `POST ".../issue/KEY/comment"` with `{"body":"text"}` |
| Assign to me | `PUT ".../issue/KEY/assignee"` with `{"accountId":"..."}` (get id from `/myself`) |

---

## Triggers

- "create a jira ticket"
- "show me PROJ-123"
- "list my tickets"
- "move ticket to done"
- "what's in the current sprint"

---

## Issue Key Detection

Issue keys follow the pattern: `[A-Z]+-[0-9]+` (e.g., PROJ-123, ABC-1).

When a user mentions an issue key, fetch it first:
`curl "${J[@]}" "$JIRA_BASE_URL/rest/api/2/issue/KEY"`.

---

## Workflow

**Creating tickets:**
1. Research context if the user references code/tickets/PRs
2. Draft ticket content
3. Show the JSON body and review with the user
4. `POST /rest/api/2/issue`

**Updating tickets:**
1. Fetch issue details first
2. Check status (careful with in-progress tickets)
3. Show current vs proposed changes
4. Get approval before updating
5. Add a comment explaining the change

---

## Before Any Operation

1. **What's the current state?** — Always fetch the issue first. Don't assume
   status, assignee, or fields are what the user thinks.
2. **Who else is affected?** — Watchers, linked issues, parent epics. A "simple
   edit" might notify many people.
3. **Is this reversible?** — Transitions may have one-way gates; some workflows
   require intermediate states. Description edits have no undo.
4. **Do I have the right identifiers?** — Issue keys, transition IDs, account
   IDs. Display names don't work for assignment.

---

## NEVER

- **NEVER transition without fetching current transitions** — the valid set
  depends on the current status, and ids/names differ per workflow. "To Do" →
  "Done" may not be a legal single hop.
- **NEVER assign using display name** — Jira Cloud assigns by `accountId` only.
  Get it from `/rest/api/2/myself` (or user search) first.
- **NEVER edit a description without showing the original** — Jira has no undo.
- **NEVER bulk-modify without explicit approval** — each change notifies
  watchers.
- **NEVER print, echo, or write `$JIRA_API_TOKEN`** — pass it only through
  curl's `-u`. Never inline the literal token or put it in a repo file.

---

## Safety

- Always show the curl command before running it
- Always get approval before modifying tickets
- Preserve original information when editing
- Verify updates after applying (re-fetch the issue)
- Surface auth errors (401/403) clearly so the user can fix email/token/scopes

---

## First-Time Setup

When `missing jira env`, print the complete instruction below in one message.
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
updates *that* shell, not Claude's Bash tool. If the vars still read as missing
after setup, the user must restart the Claude session (or export them in the
environment Claude was launched from).

> Jira Server / Data Center (self-hosted, not Cloud) uses a Personal Access
> Token as a Bearer header instead of basic auth — see `references/rest.md`.

---

## Deep Dive

Load `references/rest.md` when:
- Creating issues with complex/required fields or multi-line content
- Building JQL queries beyond simple filters
- Working with transitions, assignment, or sprints
- Troubleshooting auth (401/403) or 4xx field errors

Skip it for simple view/list/status checks — the Quick Reference above is enough.
