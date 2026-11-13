# Slack — common errors

Read this when a Slack call returns `"ok": false` and you need the meaning of the error and the caller-side fix.

## Common errors

| Slack error | Meaning | Caller-side fix |
|---|---|---|
| `not_authed` / `invalid_auth` | Token missing, malformed, or revoked | Re-create the bot token, update the caller env var. Do not retry blindly. |
| `missing_scope` | Token lacks required OAuth scope (`lists:read`, `lists:write`, `chat:write`) | Grant the scope on the Slack app, re-install to workspace, refresh the token. |
| `list_not_found` | Bot not shared with that List, **or** `list_id` is wrong | Share the List with the bot user (List → … → Share → add bot). Re-check the ID. |
| `channel_not_found` | Bot not in that channel, **or** `channel` ID is wrong | Invite the bot to the channel (`/invite @<bot>`). Re-check the ID. |
| `ratelimited` (HTTP 429) | Hitting Slack rate limits | Honor `Retry-After` header. Single retry, then bubble the error up — do not loop. |
| `ok: false` with `error: "<other>"` | Anything else | Print the raw response and stop. Do not silently continue. |
