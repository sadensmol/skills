# Asserting on Errors in Tests

Read this when a test asserts on an error, or when you add HTTP instrumentation
(otelhttp) to a client that existing tests cover.

## Don't assert on exact error-message strings

Error *text* from the stdlib or third-party libraries is an implementation
detail that shifts with library version and with transport/middleware wrapping.
Asserting the full string makes tests brittle: a dependency change rewords the
message and the test fails even though behaviour is unchanged. Assert on
**semantics** (`errors.Is` / an error code / a stable substring), not the exact
message.

```go
// Bad — couples the test to stdlib phrasing
require.Equal(t, `Post "…": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`, err.Error())

// Good — assert the meaning
require.ErrorIs(t, err, context.DeadlineExceeded)
// or, when only the wrapped message is reachable, a stable fragment:
require.Contains(t, err.Error(), "Client.Timeout exceeded")
```

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
