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

This skill is a recipe book for **other skills**. It does not run anything on its own. Consuming skills (e.g. a `release-prep` skill) copy these recipes into their own bodies, fill in IDs, and run them in a shell.

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

## Read on demand

**When a trigger below matches what you are about to do, read that file first** — before
you write, review or answer. The summary in this skill is not the full rule.

| When | Read |
|---|---|
| You read rows from a List — Recipe 2 (list items, extract `(item_id, ticket)` pairs) | `references/recipes.md` |
| You update a List item field or resolve column names → keys — Recipe 3 | `references/recipes.md` |
| You need a List's last-modified time or a staleness gate — Recipe 4 | `references/recipes.md` |
| You post a channel message — Recipe 5 | `references/recipes.md` |
| A call returned `"ok": false` and you need the error's meaning and caller-side fix | `references/errors.md` |

## Common errors

The error table (meaning and caller-side fix per Slack error) is in `references/errors.md`.

**Diagnostic one-liner** for any failed call — never just check the HTTP code; Slack returns HTTP 200 with `"ok": false`:

```bash
resp=$(curl -sS -X POST "https://slack.com/api/<method>" -H "Authorization: Bearer $SLACK_TOKEN" ... )
echo "$resp" | jq -e '.ok' >/dev/null || { echo "slack call failed: $resp" >&2; exit 1; }
```
