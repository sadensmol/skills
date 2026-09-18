# gRPC Mocking Patterns

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
