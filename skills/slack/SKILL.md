---
name: sadensmol-slack
description: |
  Direct Slack Web API recipes (Lists + chat) for use from other skills. Use when
  another skill needs to read items from a Slack List, update a List item field,
  read last-modified timestamps on a List, or post a message to a Slack channel
  via curl. Caller supplies the bot token via a caller-named env var (the skill
  uses a local $SLACK_TOKEN placeholder); if no token is found, stop and ask the
  user to create one — never fall back. Trigger when a calling skill mentions
  Slack Lists, Slack channel posting, or references this skill by name.
---

# Slack — direct Web API recipes

This skill is a recipe book for **other skills**. It does not run anything on its own. Consuming skills (e.g. a `release-prep` skill) copy these recipes into their own bodies, fill in IDs, and execute them via `Bash`.

All requests go directly to `https://slack.com/api/<method>` with `curl`. No SDK, no MCP, no helper script.

## API surface uncertainty

The `slackLists.*` API family is **partially still beta**. Endpoint names and field names below are written from Slack's current public docs, but the first real call against the target workspace is the source of truth. Each recipe is annotated with `# verify on first run` where a field name or endpoint has not been validated against a live workspace. **Do not pretend a field exists if a call fails — fix the recipe in-place and tell the user what changed.**

Slack API docs root: https://api.slack.com/methods.

## Security boundary

The skill does not enforce per-resource scope. Slack does:

- **OAuth scopes** on the bot token gate which API families can be called (`lists:read`, `lists:write`, `chat:write`).
- **Lists** are reachable only if the List has been explicitly shared with the bot user.
- **Channels** are reachable only if the bot has been invited.

If a recipe returns `list_not_found` / `channel_not_found`, the boundary is doing its job — the token is fine but the resource was not shared with this bot. See [Common errors](#common-errors).

## 1. Caller integration pattern

The skill itself does **not** know the caller's env var name. Each caller defines its own (e.g. `MYAPP_SLACK_TOKEN`) and maps it into the local placeholder `$SLACK_TOKEN` at the top of its recipe.

**Required preamble in every caller recipe:**

```bash
# Replace MYAPP_SLACK_TOKEN with the caller-defined env var name.
SLACK_TOKEN="${MYAPP_SLACK_TOKEN:-}"
if [ -z "$SLACK_TOKEN" ]; then
  echo "Slack token not set. Create a Slack bot token with the required scopes" >&2
  echo "and export MYAPP_SLACK_TOKEN before running this step."                 >&2
  exit 1
fi
```

**Hard rules — do not break:**

- **No fallback chain.** Do not try `SLACK_BOT_TOKEN`, `SLACK_TOKEN`, or any other variable if the caller's variable is empty.
- **No cached token.** Do not write the token to a file or read it from a previous run.
- **No `echo "$SLACK_TOKEN"`.** Never log or print the token.
- **No defaults for `<LIST_ID>` / `<CHANNEL_ID>`.** Every recipe takes IDs as parameters; if the caller has not supplied them, fail loudly the same way as for the token.

## 2. Recipe — list items in a List

Endpoint: `slackLists.items.list` (https://api.slack.com/methods/slackLists.items.list).

```bash
curl -sS -X GET "https://slack.com/api/slackLists.items.list?list_id=<LIST_ID>" \
  -H "Authorization: Bearer $SLACK_TOKEN"
```

Response shape (typical):

```json
{
  "ok": true,
  "items": [
    {
      "id": "Rec0123...",        // # verify on first run — row id field name
      "fields": [
        {"key": "col_ticket",  "value": "PROJ-1234"},   // # verify column-key names
        {"key": "col_done", "value": false}
      ],
      "updated_ts": "1718450400.000000"             // # verify on first run
    }
  ]
}
```

**Extract `(item_id, ticket)` pairs:**

```bash
curl -sS -X GET "https://slack.com/api/slackLists.items.list?list_id=<LIST_ID>" \
  -H "Authorization: Bearer $SLACK_TOKEN" \
  | jq -r '.items[] |
      [.id,
       (.fields[] | select(.key == "<TICKET_COLUMN_KEY>") | .value)] |
      @tsv'
```

`<TICKET_COLUMN_KEY>` is the **column key**, not the human-visible column name. See [Resolving column names → keys](#resolving-column-names--keys) below.

## 3. Recipe — update a list item field

Endpoint: `slackLists.items.update` (https://api.slack.com/methods/slackLists.items.update).

Primary use case: ticking a checkbox to mark a step as done.

```bash
curl -sS -X POST "https://slack.com/api/slackLists.items.update" \
  -H "Authorization: Bearer $SLACK_TOKEN" \
  -H "Content-Type: application/json; charset=utf-8" \
  -d '{
    "list_id": "<LIST_ID>",
    "id":      "<ITEM_ID>",
    "fields":  [
      {"key": "<COLUMN_KEY>", "value": <NEW_VALUE>}
    ]
  }'
```

- `<NEW_VALUE>` for a checkbox is JSON `true` / `false`, not `"true"` / `"false"`.
- For a status / select column, `<NEW_VALUE>` is the option's **key**, not its label.
- Multi-field update: include multiple objects inside the `fields` array.

Always check the response for `ok: true`:

```bash
resp=$(curl -sS ... )
echo "$resp" | jq -e '.ok' >/dev/null || { echo "slack update failed: $resp" >&2; exit 1; }
```

### Resolving column names → keys

The skill does not include a dedicated endpoint recipe for "list the schema of a List" because Slack's exact endpoint name here is **not validated** — `slackLists.list.get` or similar is the candidate (`# verify on first run`). When you first wire a caller, run `slackLists.items.list` once, look at any item's `fields[]`, and read off the `key` values that map to the columns you care about. Hardcode those keys as constants in the caller skill. Re-derive only if columns change.

## 4. Recipe — get list last-modified timestamp

Used by callers that need to refuse a stale plan (e.g. release-prep gate: "list must be ≤ N days old").

Endpoint candidate: `slackLists.list.get` (`# verify on first run` — Slack's actual list-metadata endpoint name may differ). If this endpoint is wrong, the practical fallback is to read `updated_ts` from the newest row via `slackLists.items.list` (Recipe 2) and treat that as the list's last activity.

```bash
curl -sS -X GET "https://slack.com/api/slackLists.list.get?list_id=<LIST_ID>" \
  -H "Authorization: Bearer $SLACK_TOKEN" \
  | jq -r '.list.updated'                          # # verify field name
```

**Caller-side staleness gate** (example, 3-day threshold):

```bash
updated_ts=$(curl -sS ... | jq -r '.list.updated')   # epoch seconds
now=$(date -u +%s)
max_age=$((3 * 24 * 3600))
if [ $((now - updated_ts)) -gt $max_age ]; then
  echo "Slack List is older than 3 days — refusing to proceed. Refresh the list first." >&2
  exit 1
fi
```

## 5. Recipe — post a channel message

Endpoint: `chat.postMessage` (https://api.slack.com/methods/chat.postMessage). This one is stable and well-tested.

```bash
curl -sS -X POST "https://slack.com/api/chat.postMessage" \
  -H "Authorization: Bearer $SLACK_TOKEN" \
  -H "Content-Type: application/json; charset=utf-8" \
  -d '{
    "channel": "<CHANNEL_ID>",
    "text":    "<plain text fallback>"
  }'
```

`<CHANNEL_ID>` is the channel's Slack ID (`C…`), not the `#name`. Use the Slack workspace UI to copy a channel link and pull the ID from the URL.

For structured output, replace `text` with `blocks` (Block Kit): https://api.slack.com/block-kit. Block Kit is out of scope for this skill — copy the JSON from Slack's Block Kit Builder and inline it.

Always check `ok`:

```bash
resp=$(curl -sS ... )
echo "$resp" | jq -e '.ok' >/dev/null || { echo "slack post failed: $resp" >&2; exit 1; }
```

## Common errors

| Slack error | Meaning | Caller-side fix |
|---|---|---|
| `not_authed` / `invalid_auth` | Token missing, malformed, or revoked | Re-create the bot token, update the caller env var. Do not retry blindly. |
| `missing_scope` | Token lacks required OAuth scope (`lists:read`, `lists:write`, `chat:write`) | Grant the scope on the Slack app, re-install to workspace, refresh the token. |
| `list_not_found` | Bot not shared with that List, **or** `list_id` is wrong | Share the List with the bot user (List → … → Share → add bot). Re-check the ID. |
| `channel_not_found` | Bot not in that channel, **or** `channel` ID is wrong | Invite the bot to the channel (`/invite @<bot>`). Re-check the ID. |
| `ratelimited` (HTTP 429) | Hitting Slack rate limits | Honor `Retry-After` header. Single retry, then bubble the error up — do not loop. |
| `ok: false` with `error: "<other>"` | Anything else | Print the raw response and stop. Do not silently continue. |

**Diagnostic one-liner** for any failed call — never just check the HTTP code; Slack returns HTTP 200 with `"ok": false`:

```bash
resp=$(curl -sS -X POST "https://slack.com/api/<method>" -H "Authorization: Bearer $SLACK_TOKEN" ... )
echo "$resp" | jq -e '.ok' >/dev/null || { echo "slack call failed: $resp" >&2; exit 1; }
```
