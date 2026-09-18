# REST (curl) Reference

The **only** backend: talk to the **Jira Cloud REST API** directly with `curl`.
Nothing to install — `curl` ships with macOS.

## Auth model

Jira Cloud uses **HTTP Basic auth with `email:API-token`** (the token is NOT a
password — create one at
https://id.atlassian.com/manage-profile/security/api-tokens).

Three values come from the environment (user adds them to `~/.profile`):

| Var | Example | Meaning |
|-----|---------|---------|
| `JIRA_BASE_URL` | `https://yourorg.atlassian.net` | Site URL, no trailing slash |
| `JIRA_EMAIL` | `you@example.com` | Atlassian account email |
| `JIRA_API_TOKEN` | `ATATT3x...` | API token (secret) |

**Never print `$JIRA_API_TOKEN`.** Always pass it through curl's `-u` from the
env var — never inline the literal token, never echo it, never write it to a
file. Check presence without revealing it:
`[ -n "$JIRA_BASE_URL" ] && [ -n "$JIRA_EMAIL" ] && [ -n "$JIRA_API_TOKEN" ] && echo ready || echo "missing jira env"`.

A reusable auth prefix (safe — the token is only expanded inside curl):

```bash
J=(-s -u "$JIRA_EMAIL:$JIRA_API_TOKEN" -H "Accept: application/json")
```

## API version choice

- Use **`/rest/api/2`** for reads and for **plain-text** comment/description
  bodies — v2 accepts plain strings, which keeps curl simple.
- **`/rest/api/3`** requires **ADF** (Atlassian Document Format, a JSON
  document) for `body`/`description`. Only reach for v3 when you specifically
  need ADF/rich content; otherwise v2 is less error-prone.

## Recipes

### Who am I / verify auth
```bash
curl "${J[@]}" "$JIRA_BASE_URL/rest/api/2/myself" | jq '{accountId, displayName, emailAddress}'
```
A 200 with your account = auth works. A 401 = bad email/token. A 403 = token
lacks scope. Surface these to the user verbatim.

### View an issue
```bash
curl "${J[@]}" "$JIRA_BASE_URL/rest/api/2/issue/PROJ-XXX?fields=summary,status,assignee,description,issuetype,priority" \
  | jq '{key, summary: .fields.summary, status: .fields.status.name, assignee: .fields.assignee.displayName, type: .fields.issuetype.name}'
```

### Search with JQL
Newer Jira Cloud replaced `GET /search` with `POST /search/jql`:
```bash
curl "${J[@]}" -H "Content-Type: application/json" -X POST \
  "$JIRA_BASE_URL/rest/api/2/search/jql" \
  -d '{"jql":"assignee = currentUser() AND statusCategory != Done ORDER BY updated DESC","maxResults":50,"fields":["summary","status"]}' \
  | jq '.issues[] | {key, summary: .fields.summary, status: .fields.status.name}'
```
If a site still serves the old endpoint, fall back to
`GET /rest/api/2/search?jql=...&fields=summary,status` (URL-encode the JQL).

Common JQL:
- My open work: `assignee = currentUser() AND statusCategory != Done`
- One project's board: `project = PROJ AND sprint in openSprints()`
- Recently updated: `updated >= -7d ORDER BY updated DESC`

### Create an issue (plain-text description, v2)
```bash
curl "${J[@]}" -H "Content-Type: application/json" -X POST \
  "$JIRA_BASE_URL/rest/api/2/issue" \
  -d '{"fields":{"project":{"key":"PROJ"},"issuetype":{"name":"Task"},"summary":"<summary>","description":"<plain text>"}}' \
  | jq '{key, self}'
```
Show the user the JSON body and get approval before POSTing. Required fields
vary per project — a 400 lists the offending fields; read and fix them.

### Transition an issue (two steps — IDs are per-project)
```bash
# 1. list valid transitions FROM the current status
curl "${J[@]}" "$JIRA_BASE_URL/rest/api/2/issue/PROJ-XXX/transitions" \
  | jq '.transitions[] | {id, name, to: .to.name}'

# 2. apply one by id
curl "${J[@]}" -H "Content-Type: application/json" -X POST \
  "$JIRA_BASE_URL/rest/api/2/issue/PROJ-XXX/transitions" \
  -d '{"transition":{"id":"31"}}'
```
Always list first — transition names AND ids differ per workflow, and the set
depends on the current state (a status may not be reachable in one hop). A
successful transition returns HTTP 204 with no body.

### Add a comment (plain text, v2)
```bash
curl "${J[@]}" -H "Content-Type: application/json" -X POST \
  "$JIRA_BASE_URL/rest/api/2/issue/PROJ-XXX/comment" \
  -d '{"body":"Opened PR: https://github.com/..."}'
```

### Assign / unassign
```bash
# assign to me: first get my accountId from /myself, then:
curl "${J[@]}" -H "Content-Type: application/json" -X PUT \
  "$JIRA_BASE_URL/rest/api/2/issue/PROJ-XXX/assignee" -d '{"accountId":"<id>"}'
# unassign:
curl "${J[@]}" -H "Content-Type: application/json" -X PUT \
  "$JIRA_BASE_URL/rest/api/2/issue/PROJ-XXX/assignee" -d '{"accountId":null}'
```
Jira Cloud assigns by **accountId**, never username/displayName.

## Response & error handling

- Add `-w '\n%{http_code}\n'` (or `-i`) when you need the status code; 2xx =
  ok, 4xx = client error with a JSON `errorMessages`/`errors` body worth
  showing the user, 5xx = Jira-side.
- Pipe through `jq` for readable output; without `jq`, the raw JSON is fine but
  noisy.
- On 401/403, stop and tell the user to check `JIRA_EMAIL` / `JIRA_API_TOKEN` /
  token scopes — don't retry blindly.

## Jira Server / Data Center (not Cloud)

Self-hosted Jira uses a **Personal Access Token** as a Bearer header instead of
basic auth:
```bash
J=(-s -H "Authorization: Bearer $JIRA_API_TOKEN" -H "Accept: application/json")
```
Everything else (endpoints, JQL, transitions) is the same, and v2 is the safe
default there too.
