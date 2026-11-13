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

**Follow-up / related tickets created during a task (MUST FOLLOW):** when you create
a ticket from work on an origin ticket, fetch the origin first, assign the new ticket to
the current user, give it the origin's parent, link it to the origin, give the context a
reader needs, then re-fetch and verify. Read `references/follow-up-tickets.md` for the
full steps and JSON before you create such a ticket.

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

When the Backend check prints `missing JIRA_API_TOKEN`, read `references/setup.md` and
print its complete instruction in one message.
**Secrets live in `~/.profile` and are added by the USER, never by Claude** —
never write, echo, or edit a token, never print `$JIRA_API_TOKEN`, never put it
in a repo file.

---

## Read on demand

**When a trigger below matches what you are about to do, read that file first** — before
you write, review or answer. The summary in this skill is not the full rule.

| When | Read |
|---|---|
| The Backend check prints `missing JIRA_API_TOKEN` | `references/setup.md` |
| You create a ticket from work on another ticket | `references/follow-up-tickets.md` |
| Complex create, JQL, transitions, assignment, sprints, or auth/4xx errors | `references/rest.md` (see *Deep Dive*) |

---

## Deep Dive

Load `references/rest.md` when:
- Creating issues with complex/required fields or multi-line content
- Building JQL queries beyond simple filters
- Working with transitions, assignment, or sprints
- Troubleshooting auth (401/403) or 4xx field errors

Skip it for simple view/list/status checks — the Quick Reference above is enough.
