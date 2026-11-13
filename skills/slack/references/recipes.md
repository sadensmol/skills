# Slack recipes — Lists and chat

Read this when you write or run a caller recipe that lists List items, updates a List item field, reads a List's last-modified time, or posts a channel message. Recipe 1 (the caller preamble and hard rules) stays in `SKILL.md`.

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
