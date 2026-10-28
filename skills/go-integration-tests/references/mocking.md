# Mocking External Services

Read this when a test needs a gRPC mock server (bufconn) or a WireMock HTTP stub.

## Overview

Integration tests use in-process gRPC mocking via bufconn to mock external service dependencies. Mock servers implement gRPC service interfaces with configurable function fields.

## Creating Mock Servers

### Mock Server Structure

**Location**: `tests/{mock-name}-server.go` or `tests/mocks/{mock-name}-server.go`

```go
package tests

import (
    "context"
    servicev1 "github.com/example/platform/shared/private-api/v1/service"
)

type ServiceNameServer struct {
    // Function fields for each RPC method
    MethodOneFunc func(context.Context, *servicev1.Request) (*servicev1.Response, error)
    MethodTwoFunc func(context.Context, *servicev1.Request) (*servicev1.Response, error)
}

func (s ServiceNameServer) MethodOne(
    ctx context.Context,
    req *servicev1.Request,
) (*servicev1.Response, error) {
    return s.MethodOneFunc(ctx, req)
}

func (s ServiceNameServer) MethodTwo(
    ctx context.Context,
    req *servicev1.Request,
) (*servicev1.Response, error) {
    return s.MethodTwoFunc(ctx, req)
}
```

### Registering Mock Servers

Register mocks in the API-level suite's SetupSuite:

```go
func (s *PrivateAPIV1TestSuite) SetupSuite() {
    s.ServiceNameTestSuite.SetupSuite()

    // ... client setup ...

    // Create mock server instances
    s.integrationSrv = &tests.IntegrationServer{}
    s.ledgerSrv = &tests.LedgerServer{}

    // Create gRPC server and register mocks
    srv := grpc.NewServer()
    integrationv1.RegisterServiceServer(srv, s.integrationSrv)
    ledgerv1.RegisterServiceServer(srv, s.ledgerSrv)

    // Start server on bufconn listener
    go func() { _ = srv.Serve(s.GRPCClientConnectionProvider.Listener) }()
    time.Sleep(1 * time.Second)
}
```

## Using Mocks in Tests

### Basic Mock Configuration

Set mock behavior before calling the API:

```go
func (s *OrderTestSuite) TestSuccess() {
    testAmount := int64(1000)
    expectedBalance := int64(9000)

    // Configure mock behavior
    s.intSrv.OrderFunc = func(ctx context.Context, req *integrationv1.OrderRequest) (*integrationv1.BalanceResponse, error) {
        // Validate request parameters
        s.Require().Equal(testAmount, req.Amount)
        s.Require().NotEmpty(req.CartID)

        // Return mock response
        return &integrationv1.BalanceResponse{
            Balance: expectedBalance,
        }, nil
    }

    // Call the API
    res, err := s.prvAPIV1Client.Order(s.Ctx, &corev1.OrderRequest{
        SessionID:    tests.TestSessionID,
        CartID: "test_cart",
        Amount:  testAmount,
    })

    // Assert results
    s.Require().NoError(err)
    s.Require().Equal(expectedBalance, res.Balance)
}
```

### Validating Metadata

Check gRPC metadata in mock functions:

```go
s.intSrv.OrderFunc = func(ctx context.Context, req *integrationv1.OrderRequest) (*integrationv1.BalanceResponse, error) {
    // Extract and validate metadata
    md, ok := metadata.FromIncomingContext(ctx)
    s.Require().True(ok)
    s.Require().Equal(tests2.TestCustomerID, md.Get("X-CustomerID")[0])

    return &integrationv1.BalanceResponse{Balance: 5000}, nil
}
```

### Returning Errors

Mock error responses:

```go
s.intSrv.OrderFunc = func(ctx context.Context, req *integrationv1.OrderRequest) (*integrationv1.BalanceResponse, error) {
    // Return domain error
    return nil, domain.NewError("insufficient funds", errors.New("balance too low"))
}

// Test error is proxied correctly
res, err := s.prvAPIV1Client.Order(s.Ctx, &corev1.OrderRequest{...})
s.Require().ErrorContains(err, "rpc error: code = Unknown desc = [500]insufficient funds(balance too low)")
```

### Tracking Mock Calls

Use boolean flags to verify mock interactions:

```go
func (s *OrderTestSuite) TestDemoMode() {
    // Track if integration is called (shouldn't be in demo mode)
    integrationCalled := false
    s.intSrv.OrderFunc = func(ctx context.Context, req *integrationv1.OrderRequest) (*integrationv1.BalanceResponse, error) {
        integrationCalled = true
        return &integrationv1.BalanceResponse{Balance: 5000}, nil
    }

    // Make request
    res, err := s.prvAPIV1Client.Order(s.Ctx, &corev1.OrderRequest{
        SessionID: tests.TestDemoSessionID,  // Demo session
        // ...
    })

    // Verify mock was NOT called
    s.Require().False(integrationCalled, "integration server should not be called in demo mode")
}
```

### Resetting Mocks

Reset mock behavior between tests:

```go
func (s *OrderTestSuite) TestInvalidRequest() {
    // Clear any previous mock setup
    s.intSrv.OrderFunc = nil

    // Test should fail before reaching integration service
    res, err := s.prvAPIV1Client.Order(s.Ctx, &corev1.OrderRequest{
        SessionID:   "invalid",
        Amount: -100,
    })

    s.Require().Error(err)
}
```

## Multiple Mock Servers

When testing services that depend on multiple external services:

```go
type PrivateAPIV1TestSuite struct {
    tests.CoreTestSuite
    prvAPIV1Client corev1.ServiceClient

    // Multiple mock servers
    intSrv    *tests.IntegrationServer
    ledgerSrv *tests.LedgerServer
    coreSrv   *tests.CoreServer
}

func (s *PrivateAPIV1TestSuite) SetupSuite() {
    s.CoreTestSuite.SetupSuite()

    // ... client setup ...

    // Initialize all mocks
    s.intSrv = &tests.IntegrationServer{}
    s.ledgerSrv = &tests.LedgerServer{}
    s.coreSrv = &tests.CoreServer{}

    // Register all on same server
    srv := grpc.NewServer()
    integrationv1.RegisterServiceServer(srv, s.intSrv)
    ledgerv1.RegisterServiceServer(srv, s.ledgerSrv)
    corev1.RegisterServiceServer(srv, s.coreSrv)

    go func() { _ = srv.Serve(s.GRPCClientConnectionProvider.Listener) }()
    time.Sleep(1 * time.Second)
}

func (s *SomeTestSuite) TestWithMultipleMocks() {
    // Configure multiple mocks
    s.intSrv.OrderFunc = func(...) {...}
    s.ledgerSrv.GetBalanceFunc = func(...) {...}
    s.coreSrv.ValidateFunc = func(...) {...}

    // Test will use all configured mocks
}
```

## Best Practices

1. **Always validate request parameters** in mock functions to ensure API calls match expectations
2. **Use descriptive variable names** for expected values in tests
3. **Reset mocks** between tests to prevent side effects (set to nil if not needed)
4. **Track mock calls** with boolean flags when testing conditional behavior
5. **Extract metadata** when testing authentication/authorization flows
6. **Return realistic responses** - use appropriate status codes and error messages
7. **Test both success and error paths** with different mock configurations

## WireMock: shared server, no global Reset

WireMock runs as **one long-lived server process per port** (e.g. core mock `:8081`, integration mock `:8082`), started once for the whole `go test ./...` job. **Every test in every package is a client of the same server** — the stub registry is global mutable state *inside the server*, not inside your Go test. Go runs different **packages** in parallel (no `-p 1`), so suites in `tests/api/v1`, `tests/privateapi/v1`, `tests/service`, … all hit the same WireMock concurrently.

`wc.Reset()` in `go-wiremock` is `POST /__admin/mappings/reset` — it **wipes the ENTIRE server's stub registry** (and reloads only file-based mappings from `mappings/`). It does NOT delete "this test's stubs". So one suite's `Reset()` deletes the stubs another parallel suite just registered → that suite's request hits no stub → WireMock returns a plain-text `404 Not Found` / `500`.

Symptoms of this race (they **move between runs**, same root cause — the tell-tale sign of the shared-Reset bug):
- `[404]<provider> order http error: 404 Not Found` (file-based stub briefly gone during another suite's reset)
- `failed to parse core error response: invalid character 'N'/'R'` (parser choking on WireMock's plain-text `Not Found` / request-not-matched body where JSON was expected)
- Passes locally (packages effectively serialize) but fails in CI (packages run parallel).

**Rule: delete only your own stub, never reset the server.** Capture the `*StubRule` returned by `wiremock.Get(...)/Post(...)`, register with `StubFor(stub)`, and clean up with `DeleteStub(stub)` (deletes by the rule's auto-generated UUID). `NewStubRule` assigns the UUID at build time, so the handle is valid for deletion.

```go
// Bad — wipes the whole shared server; races with every parallel package
func (s *Suite) stubCoreProducts(externalID string, products []coreapiv1.ProductInfo) func() {
    wc := wiremock.NewClient(tests.TestCoreWireMockURL)
    err := wc.StubFor(wiremock.Get(wiremock.URLPathEqualTo("/api/v1/products")).
        WithQueryParam("externalID", wiremock.EqualTo(externalID)).
        WillReturnResponse(wiremock.NewResponse().WithStatus(200).WithJSONBody(products)))
    s.Require().NoError(err)
    return func() { s.Require().NoError(wc.Reset()) }   // ❌ global reset
}

// Good — capture the stub, delete only it
func (s *Suite) stubCoreProducts(externalID string, products []coreapiv1.ProductInfo) func() {
    wc := wiremock.NewClient(tests.TestCoreWireMockURL)
    stub := wiremock.Get(wiremock.URLPathEqualTo("/api/v1/products")).
        WithQueryParam("externalID", wiremock.EqualTo(externalID)).
        WillReturnResponse(wiremock.NewResponse().WithStatus(200).WithJSONBody(products))
    s.Require().NoError(wc.StubFor(stub))
    return func() { s.Require().NoError(wc.DeleteStub(stub)) }   // ✅ scoped delete
}
// call site: defer s.stubCoreProducts(externalID, products)()
```

Corollaries:
- **No `Reset()` in `SetupTest`/`SetupSuite` either.** If a suite re-stubbed a fixed path each test via `SetupTest` + `Reset()`, register the shared stub **once** in `SetupSuite` and remove it in `TearDownSuite` (`DeleteStub`); per-test stubs get a per-test `defer DeleteStub`.
- **Make stubs naturally non-colliding** — match on a unique key per test (unique `externalID`, `session_id`, cart UUID) so leftover stubs from other tests never satisfy your request and you never *need* a global reset.
- **Request-journal cleanup is the same** — if you assert call counts, scope with `DeleteRequestsByCriteria(matcher)` for your exact key, never `DeleteAllRequests()`.

### WireMock lifecycle: setup → call → verify → clean up

Every WireMock-backed test follows the same four steps. Skipping the cleanup of the **request journal** (not just the stub) is the usual cause of flaky call-count assertions.

1. **Setup** — register the stub(s) the test needs (`StubFor`).
2. **Call** — exercise the code that hits the mocked endpoint.
3. **Verify** — assert the call count if relevant (`Verify` / `GetCountRequests`).
4. **Clean up** — delete **both** the stub **and** its recorded requests, scoped to this test.

**`Verify` checks an ABSOLUTE count, not "this test's calls".** `wc.Verify(r, n)` is literally `GetCountRequests(r) == n`, counting **every** journal entry matching `r` on the shared server. The journal is **never reset globally** (see above), so without per-stub journal cleanup the count accumulates across sibling tests and across reruns — `Verify(r, 1)` then passes only for the very first matching request ever and fails (count 2, 3, …) afterwards. The fix is **not** a delta hack; it's cleaning up the journal each test so the count genuinely reflects this test's one call:

```go
ws := wiremock.Post(wiremock.URLPathEqualTo("/api/v1/create-new-game")).
    WithHeader("X-REQUEST-SIGN", wiremock.EqualTo(sign)).
    WithBodyPattern(wiremock.EqualToJson(cBody)).
    WillReturnResponse(wiremock.NewResponse().WithStatus(200).WithJSONBody(resp))
s.Require().NoError(wc.StubFor(ws))             // setup
defer func() {                                  // clean up: journal first, then stub
    _, _ = wc.DeleteRequestsByCriteria(ws.Request())
    _ = wc.DeleteStub(ws)
}()

// ... make the one call ...

ok, err := wc.Verify(ws.Request(), 1)           // verify: now genuinely 1
s.Require().NoError(err)
s.Require().True(ok)
```

**Same endpoint, different behaviours → same suite, run in sequence.** When several tests stub the *same* endpoint (often the *same* request matcher) with *different* responses — e.g. `create-new-game` returning success / `account_blocked` / `500` — they must live in **one testify suite** so they run sequentially. testify runs a suite's methods one at a time, so each test's setup → call → verify → cleanup completes before the next begins; conflicting stubs/journal for the same matcher never coexist. Splitting them across packages (which Go runs in parallel) makes the stubs collide on the shared server.

**Helper: a `StubScope` that owns register + scoped cleanup.** Wrap the client so cleanup of both stub and journal can't be forgotten. Never call `Reset()`/`DeleteAllRequests()` inside it.

```go
type StubScope struct {
    *wiremock.Client
    stubs []*wiremock.StubRule
}

func NewStubScope(c *wiremock.Client) *StubScope { return &StubScope{Client: c} }

func (s *StubScope) StubFor(rule *wiremock.StubRule) error {
    if err := s.Client.StubFor(rule); err != nil {
        return err
    }
    s.stubs = append(s.stubs, rule)
    return nil
}

// Cleanup removes each registered stub AND its journal requests, scoped to this
// scope's matchers — other parallel packages' stubs/requests are untouched.
func (s *StubScope) Cleanup() error {
    var firstErr error
    for _, st := range s.stubs {
        if _, err := s.DeleteRequestsByCriteria(st.Request()); err != nil && firstErr == nil {
            firstErr = err
        }
        if err := s.DeleteStub(st); err != nil && firstErr == nil {
            firstErr = err
        }
    }
    s.stubs = nil
    return firstErr
}
// usage: wc := NewStubScope(wiremock.NewClient(url)); defer func(){ s.Require().NoError(wc.Cleanup()) }()
```
