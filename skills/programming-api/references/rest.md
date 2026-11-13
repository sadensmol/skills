# REST APIs

Read this when you design, change or review a REST endpoint. A project's own API guide
wins over this file.

## Core rules

- Paths name resources with plural nouns (`/orders/{orderId}`). The HTTP method is the verb.
- Use the most specific status code. Never return `200` with an error in the body.
- Every error response uses one shared error schema (`application/problem+json`,
  RFC 9457).
- Every list endpoint is paginated from its first version, with a maximum page size.

## Resources and paths

- A path names a resource, never an action. Use plural nouns: `/orders`, `/orders/{orderId}`.
- Use lowercase `kebab-case` for path segments (`/payment-methods`). Use one case style for
  parameter and field names across the whole API (`camelCase` or `snake_case`, never both).
- Nest only for real ownership, and at most one level: `/orders/{orderId}/items`. Reach a
  resource that has its own identity through its own top-level path.
- An action that is not CRUD is a sub-resource with `POST`: `POST /orders/{orderId}/cancel`.
  Use it only when no state change on the resource expresses the action well.
- Never put a trailing slash, a file extension (`.json`) or a version suffix on a resource path.

## Methods

| Method | Use for | Safe | Idempotent | Success |
|---|---|---|---|---|
| `GET` | read one resource or a list | yes | yes | `200` |
| `POST` | create in a collection, or run an action | no | no (make it so with a key) | `201` + `Location`, or `200` / `202` |
| `PUT` | replace the whole resource at a known id | no | yes | `200`, or `204` |
| `PATCH` | change some fields | no | not by default | `200` with the new state |
| `DELETE` | remove | no | yes | `204`; a repeat returns `204` or `404`, by a documented choice |

- `GET` never changes state and never takes a request body.
- `PATCH` uses JSON Merge Patch (`application/merge-patch+json`, RFC 7396): an absent
  field stays unchanged, `null` clears it. Use JSON Patch (RFC 6902) only when clients need
  array operations.

## Idempotency

- A `POST` that creates something or moves money accepts an `Idempotency-Key` header.
- The server stores the key with the request fingerprint and the response. A repeat with
  the same key and body returns the stored response. The same key with a different body
  returns `422` (or `409`).
- Keep keys for a documented time (for example 24 h). Scope them to the caller.
- An id that the client sends (a transaction id) is also an idempotency key. Enforce it
  with a unique constraint, not with a read before the write.

## Status codes

| Code | Use when |
|---|---|
| `200` | the call succeeded and the body holds the result |
| `201` | a resource was created; set `Location` to its URL and return it in the body |
| `202` | the work was accepted and runs later; return a status resource to poll |
| `204` | the call succeeded with no body |
| `304` | the conditional `GET` matched (`If-None-Match`) |
| `400` | the request is malformed: bad JSON, wrong type, failed schema validation |
| `401` | no credentials, or bad ones; send `WWW-Authenticate` |
| `403` | the caller is known but not allowed |
| `404` | the resource does not exist, or the caller must not learn that it exists |
| `405` | the method is not supported on this path; send `Allow` |
| `409` | the request conflicts with the current state (duplicate, wrong state transition) |
| `412` | an `If-Match` precondition failed |
| `415` | the `Content-Type` is not supported |
| `422` | the request is well-formed but breaks a business rule |
| `428` | the endpoint requires a precondition header that the request does not have |
| `429` | rate limit exceeded; send `Retry-After` |
| `500` | an unexpected server fault; never for a client mistake |
| `502` / `503` / `504` | an upstream failure / temporary unavailability (`Retry-After`) / an upstream timeout |

- Never return `200` with an error inside the body.
- Pick the `400` vs `422` split once per API and document it.
- A client can retry a `5xx`, `429` or `408` safely only on an idempotent call. Say which
  calls are safe to retry in the operation description.

## Errors

- Use one error schema for every error: Problem Details (`application/problem+json`,
  RFC 9457).

  ```json
  {
    "type": "https://api.example.com/problems/insufficient-funds",
    "title": "Insufficient funds",
    "status": 422,
    "detail": "Balance 10.00 is less than the bet 25.00.",
    "instance": "/bets/7f3c",
    "code": "INSUFFICIENT_FUNDS",
    "errors": [{ "field": "amount", "message": "must be at most 10.00" }]
  }
  ```

- Add a stable, machine-readable `code`. Clients branch on `code` (or `type`), never on
  `title` or `detail` text.
- Validation errors list every bad field in `errors[]`, not only the first one.
- Never leak stack traces, SQL, internal hostnames or secrets in an error body.
- Add a request or trace id to every error (body field or `traceparent` / `X-Request-Id`
  header), so that a support ticket can find the log.

## Lists: pagination, filtering, sorting

- Paginate every list from its first version. Adding pagination later is a breaking change.
- Prefer cursor pagination: `?limit=50&cursor=<opaque>`, response
  `{ "items": [...], "nextCursor": "..." }`. A `null` or absent `nextCursor` means the last
  page. The cursor is opaque to the client.
- Use offset pagination only for small, stable sets. It skips or repeats rows when data
  changes, and a deep offset is slow.
- Set a default `limit` and a maximum; reject or clamp a larger value, by a documented rule.
- Wrap a list in an object, never a bare top-level array, so that fields can be added later.
- Filter with plain query parameters (`?status=open&createdFrom=...&createdTo=...`). Time
  ranges are half-open: `from` included, `to` excluded.
- Sort with one parameter: `?sort=-createdAt,id` (`-` = descending). Allow sort only on
  indexed fields. Always end with a unique field, so that the order is stable.
- Return a total count only when it is cheap or the client really needs it.

## Field formats

| Data | Format |
|---|---|
| Timestamp | RFC 3339 string in UTC with a `Z` (`2026-10-29T12:00:00Z`); name it `...At` |
| Date only | `YYYY-MM-DD` string |
| Duration | integer with the unit in the name (`timeoutMs`), or ISO 8601 (`PT30S`) |
| Money | decimal **string** plus an ISO 4217 currency field (`{"amount":"12.50","currency":"EUR"}`); never a float |
| Id | string, even when the store uses integers; clients must not do arithmetic on it |
| 64-bit integer | string, or document it: JavaScript loses precision above 2^53 |
| Enum | `UPPER_SNAKE` or `lower_snake` string values, one style per API; clients must accept unknown values |
| Boolean | `true` / `false`; name it as a statement (`isActive`, `hasBalance`) |
| Country / language | ISO 3166-1 alpha-2 / BCP 47 tags |

- Absent and `null` mean different things only when the docs say so. Otherwise do not
  send `null` fields.
- Never reuse a field for two meanings.

## Concurrency and caching

- Return an `ETag` on a resource that clients update. Require `If-Match` on `PUT` /
  `PATCH` / `DELETE` when lost updates matter, and return `412` on a mismatch.
- Set `Cache-Control` on purpose: `no-store` for private or money data, `max-age` for
  stable reference data.

## Long-running work

- Return `202 Accepted` with a `Location` header to a status resource
  (`/operations/{id}`) that reports `PENDING` / `RUNNING` / `SUCCEEDED` / `FAILED` and the
  result or the error.
- Offer a webhook when clients would otherwise poll a lot. Sign webhook payloads, send an
  event id for deduplication, and retry with backoff.

## Compatibility and versioning

Backward-compatible (allowed in place):

- add an optional request field, query parameter or header
- add a response field
- add an endpoint
- add an enum value to a **response**, when the docs say clients must accept unknown values
- relax a constraint (a larger maximum, a field becomes optional in the request)

Breaking (needs a new version):

- remove or rename a field, parameter, endpoint or enum value
- change a type, format, unit or meaning
- make an optional request field required, or tighten a constraint
- add an enum value to a **request** field that old servers or clients reject
- change a status code, the error format, the auth scheme or the default page size

Rules:

- Version in the path with a major number only (`/v1/...`). Bump it only for a breaking change.
- Mark an old operation or field `deprecated: true` before removal. Send `Deprecation` (RFC 9745)
  and `Sunset` (RFC 8594) headers, and publish the removal date.
- Support the old version until its clients have moved.
- Clients must ignore unknown response fields (the tolerant reader rule). Say so in the docs.

## Security

- Use HTTPS only. Send credentials in the `Authorization` header, never in the URL or
  query string, because URLs land in logs.
- Check authorization on every resource access, not only on the route (prevent IDOR /
  BOLA): the caller may read or change only its own objects.
- Validate every input against the schema: type, length, range, pattern. Reject unknown
  fields where the API promises strict input.
- Limit request body size and page size. Rate-limit per caller, and return `429` with
  `Retry-After` (plus `RateLimit-*` headers when the API publishes limits).
- For a signed request (HMAC), sign the method, path, a timestamp and the raw body.
  Reject an old timestamp, so that a captured request cannot be replayed.
