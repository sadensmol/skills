# otelhttp and Error Text in Tests

Read this when you add HTTP instrumentation (otelhttp) to a client that existing tests
cover. The general rule — assert on meaning, never on exact error text — is in
`sadensmol-programming` → `references/testing.md`.

**Real gotcha — `otelhttp` changes HTTP timeout wording.** Wrapping a client's
transport with `otelhttp.NewTransport(...)` changes how `http.Client.Timeout`
surfaces: the error becomes `net/http: request canceled (Client.Timeout exceeded
while awaiting headers)` instead of `context deadline exceeded (Client.Timeout
exceeded …)`. The otelhttp wrapper is not a `*http.Transport`, so `net/http`
falls back to the legacy `req.Cancel` cancellation path instead of context
cancellation. Any test asserting the exact old string breaks the moment you add
otelhttp instrumentation — even though the timeout still fires identically and
downstream behaviour (retry/refund/etc.) is unchanged. Two takeaways:

- Adding OTel HTTP instrumentation to an existing client is a **behaviour change
  to error text** — expect brittle timeout assertions to need updating.
- Enforcing the timeout via a **context deadline** (`context.WithTimeout` on the
  request) instead of `http.Client.Timeout` keeps the `context deadline exceeded`
  wording even through otelhttp — but the durable fix is to not assert the exact
  string in the first place.
