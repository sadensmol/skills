# gRPC APIs

Read this when you design, change or review a gRPC service or a `.proto` file. A
project's own API guide wins over this file. The sources are the Protocol Buffers style
guide, the Google API Improvement Proposals (AIP) and the `buf` lint rules.

## Packages and files

- Put the major version in the package: `acme.orders.v1`. The directory matches the
  package (`acme/orders/v1/orders.proto`).
- One service per file. Shared messages go in their own file in the same package.
- Set the language options once per file (`option go_package = "…/ordersv1";`).
- A breaking change gets a new package (`acme.orders.v2`). Serve both versions until no
  client uses the old one.

## Naming

| Element | Style | Example |
|---|---|---|
| Service | `PascalCase`, ends in `Service` | `OrderService` |
| RPC | `VerbNoun`, standard verbs first | `GetOrder`, `ListOrders`, `CreateOrder`, `UpdateOrder`, `DeleteOrder` |
| Message | `PascalCase` | `Order`, `GetOrderRequest` |
| Field | `lower_snake_case` | `order_id`, `create_time` |
| Enum | `PascalCase` type, `UPPER_SNAKE_CASE` values prefixed with the type | `ORDER_STATUS_OPEN` |

- Every RPC has its own `<Rpc>Request` and `<Rpc>Response` message, even when it is empty
  today. A shared or `google.protobuf.Empty` message cannot grow a field later.
- A custom action that is not CRUD is still `VerbNoun`: `CancelOrder`, `RefundPayment`.
- Repeated fields have plural names (`repeated string order_ids`).

## Enums

- The zero value is `<TYPE>_UNSPECIFIED = 0` and means "not set". Never give it business
  meaning.
- A client must handle an enum value it does not know: a newer server can send one.
- Prefer an enum to a `bool` when a third state is likely.

## Field presence and partial updates

- Use proto3 `optional` when the code must tell "not set" from the zero value.
- An update takes the resource plus a `google.protobuf.FieldMask update_mask`. The
  server changes only the masked fields. Never treat a zero value as "leave unchanged".

## Compatibility (MUST FOLLOW)

A released `.proto` changes only in backward-compatible ways.

- Never change or reuse a field number. When you delete a field, reserve both its number
  and its name: `reserved 4; reserved "legacy_id";`.
- Never change a field's type, or move it into or out of a `oneof`.
- Never rename a field, message or enum value that JSON (`protojson`, gateways) or
  reflection uses: the name is on the wire there.
- Adding a field, a message, an RPC or an enum value is safe.
- Changing the meaning of an existing field is breaking even when the type stays the same.
- Run `buf breaking` against the main branch on every change.

## Errors

- Return errors as a gRPC status, never as an error field in the response message.
- Pick the most specific canonical code:

| Code | When |
|---|---|
| `INVALID_ARGUMENT` | the request is wrong whatever the server state is |
| `FAILED_PRECONDITION` | the request is valid, but the resource state does not allow it |
| `NOT_FOUND` / `ALREADY_EXISTS` | the resource is missing / already there |
| `ABORTED` | a concurrency conflict; the client may retry the whole read-modify-write |
| `UNAUTHENTICATED` / `PERMISSION_DENIED` | no valid credentials / not allowed |
| `RESOURCE_EXHAUSTED` | a quota or rate limit is hit |
| `UNAVAILABLE` | a transient failure; safe to retry with backoff |
| `DEADLINE_EXCEEDED` | the deadline passed before the work finished |
| `UNIMPLEMENTED` | the method does not exist on this server |
| `INTERNAL` | a bug or a broken invariant; never for a client mistake |

- Attach machine-readable detail with `google.rpc.Status` details (`ErrorInfo` with a
  stable `reason`, `BadRequest` with field violations). Clients branch on the code and
  the reason, never on the message text.
- Never put stack traces, SQL or internal hostnames in a status message.

## Deadlines and cancellation

- Every client call sets a deadline. A call without one can hang forever.
- A server passes the incoming context to every downstream call, so the deadline and the
  cancellation travel with the request.
- A server stops work when the context is cancelled.

## Retries and idempotency

- Retry only idempotent RPCs, and only on `UNAVAILABLE` (and `RESOURCE_EXHAUSTED` with a
  longer backoff). Use exponential backoff with jitter and a retry limit.
- Configure retries in the channel's service config, not in hand-written loops.
- A mutating RPC that must be safe to retry takes a client-generated `request_id`. The
  server stores it and returns the first result for a repeated id.

## Lists and pagination

- A `List` RPC has `page_size` and `page_token` in the request and `next_page_token` in
  the response. An empty `next_page_token` means the last page.
- The server caps `page_size` and applies a default when it is 0.
- The page token is opaque to the client. Never let a client build one.
- Paginate from the first version. Adding pagination later is a breaking change.

## Types

- Times: `google.protobuf.Timestamp`; durations: `google.protobuf.Duration`. Never a raw
  number of seconds or a formatted string.
- Money: `google.type.Money`, or a decimal string plus an ISO 4217 currency field. Never
  `float` or `double`.
- Avoid `google.protobuf.Any` and `Struct` for data that has a known shape. They remove
  type checking and break code generation.

## Streaming

- Use unary RPCs by default. Use streaming only for a result that is large or unbounded,
  or for real push updates.
- A stream has an end condition and honours cancellation on both sides.
- Keep every message under the receive limit (4 MB by default). Send large binary data
  through object storage and pass a reference.

## Cross-cutting concerns

- Carry auth tokens, trace and correlation IDs in metadata, not in request messages.
- Put logging, metrics, auth, panic recovery and deadline checks in interceptors, once
  per server and client, not in each handler.
- Serve the standard health service (`grpc.health.v1.Health`) so load balancers and
  orchestrators can probe the server.
- Use TLS between services. Configure keepalive on long-lived connections behind proxies.

## Tooling

- `buf lint` (the `STANDARD` rule set) and `buf breaking` run in CI on every `.proto`
  change.
- Generate code with `buf generate` from a committed `buf.gen.yaml`. Never hand-edit
  generated code.

## Review check

- Every new RPC has its own request and response message.
- Deleted fields are `reserved` by number and name.
- Every new enum starts with `_UNSPECIFIED = 0`.
- Every error path returns a specific status code, not `INTERNAL` or `UNKNOWN`.
- `buf lint` and `buf breaking` pass.
