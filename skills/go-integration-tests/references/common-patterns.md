# Common Test Patterns

Read this when you write a test method: naming, structure, error assertions,
async waits, multi-record cleanup, database verification, or a bug reproduction.

## Test Naming Conventions

**Pattern 1: TestFeatureCondition**
```go
func (s *OrderTestSuite) TestRealSuccess() {...}
func (s *OrderTestSuite) TestDemoSuccess() {...}
func (s *OrderTestSuite) TestSmallOrderAmountFail() {...}
```

**Pattern 2: Test_WhenCondition_ExpectResult** — more explicit for complex conditions
```go
func (s *OrderTestSuite) Test_WhenSessionIDIsInvalid_Return500() {...}
func (s *OrderTestSuite) Test_WhenSessionIDNotFound_Return404() {...}
func (s *OrderTestSuite) Test_WhenIntegrationServerReturnsError_ProxifyIt() {...}
```

## Test Structure (Arrange-Act-Assert)

Arrange, act, assert — separated by blank lines, not by narrating comments (SKILL.md rule 15).

```go
func (s *FeatureTestSuite) TestFeatureSuccess() {
    testAmount := int64(1000)
    expectedBalance := int64(9000)
    s.mockSrv.MethodFunc = func(ctx context.Context, req *pkg.Request) (*pkg.Response, error) {
        s.Require().Equal(testAmount, req.Amount)
        return &pkg.Response{Balance: expectedBalance}, nil
    }

    res, err := s.apiClient.Feature(s.Ctx, &api.Request{
        ID:     tests.TestID,
        Amount: testAmount,
    })

    s.Require().NoError(err)
    s.Require().NotNil(res)
    s.Require().Equal(expectedBalance, res.Balance)
}
```

## Assertions

**Error messages** — the service's domain error format, exact or partial:

```go
s.Require().EqualError(err, "rpc error: code = Unknown desc = [400]order amount is less than minimum order amount")
s.Require().ErrorContains(err, "rpc error: code = Unknown desc = [404]session not found")
s.Require().ErrorContains(err, "[403|account_blocked]user is blacklisted")
```

**Assertion messages** — explain why it should hold:

```go
s.Require().Equal(l.CurAmount+1, l2.CurAmount, "CurAmount should be incremented by 1")
s.Require().False(integrationCalled, "integration server should not be called in demo mode")
```

Use `Require()` (fail fast), not `Assert()`, for anything later steps depend on.

## Async Testing

**Prefer `require.Eventually` over `time.Sleep`** — poll for the expected state
instead of sleeping a fixed time. The ONLY exception is proving a background
action did NOT happen; see SKILL.md → *Proving "it did NOT happen"*.

Always use the project's standard constants:

```go
// tests/test-suite.go
const (
    EventuallyWaitTime     = 15 * time.Second
    EventuallyPollInterval = 1 * time.Second
)

s.Require().Eventually(func() bool {
    refund, err := s.refundTestRepository.FindByOrderID(s.Ctx, orderID)
    if err != nil {
        return false
    }
    return refund != nil && refund.Status == "processed"
}, tests.EventuallyWaitTime, tests.EventuallyPollInterval, "expected refund to be created and processed")
```

- Testing a Ristretto cache directly: prefer `cache.Wait()` over `Eventually`.
- **Don't use `Eventually` for synchronous calls** — call the API once and assert:

```go
// ❌ WRONG - CreateUser is synchronous
s.Require().Eventually(func() bool {
    _, err := s.apiClient.CreateUser(s.Ctx, req)
    return err == nil
}, tests.EventuallyWaitTime, tests.EventuallyPollInterval)

// ✅ CORRECT
res, err := s.apiClient.CreateUser(s.Ctx, req)
s.Require().NoError(err)
```

## Test Data Management

Feature suites wire **test repositories only** in `SetupSuite`:

```go
func (s *OrderTestSuite) SetupSuite() {
    s.PrivateAPIV1TestSuite.SetupSuite()

    db, err := db.Setup(s.Cfg.Postgres)
    s.Require().NoError(err)

    s.sessionTestRepository = repository_test.NewSession(db)
}
```

Suite helpers create dynamic data; `defer` the cleanup right after creating it:

```go
func (s *OrderTestSuite) TestWithCustomUser() {
    testUserID := uuid.NewString()
    sessionID := s.CreateNewSessionWithUserID(testUserID)
    defer s.DeleteSession(sessionID)

    res, err := s.prvAPIV1Client.Order(s.Ctx, &corev1.OrderRequest{
        SessionID: sessionID.String(),
        // ...
    })
}
```

### Multiple records — clean up each one by its own key

The cleanup rules are in SKILL.md → *Test Data*. With several records, each gets
its own `defer`, deleting by the exact identifiers it was created with:

```go
func (s *OrderTestSuite) Test_WhenUserBlacklisted_ReturnForbidden() {
    testUserID := uuid.NewString()
    sessionID := s.CreateNewSessionWithUserID(testUserID)
    defer s.DeleteSession(sessionID)

    blacklistEntry := &model.UserBlacklist{
        CustomerID: uuid.MustParse(tests2.TestCustomerID),
        ExternalID: tests2.TestExternalID,
        UserID:     testUserID,
        Reason:     "test blacklist",
    }
    err := s.userBlacklistTestRepository.Insert(s.Ctx, blacklistEntry)
    s.Require().NoError(err)
    defer func() {
        delErr := s.userBlacklistTestRepository.DeleteByCustomerIDAndExternalIDAndUserID(
            s.Ctx,
            uuid.MustParse(tests2.TestCustomerID),
            tests2.TestExternalID,
            testUserID,
        )
        s.Require().NoError(delErr)
    }()

    // ...
}
```

## Database State Verification

Don't stop at the API response — verify persistence, before and after, through
the **test** repository:

```go
func (s *OrderTestSuite) TestRealSuccess() {
    l, err := s.levelTestRepository.GetByUserAndCurrency(s.Ctx, tests2.TestUserID, TestCurrency)
    s.Require().NoError(err)
    s.Require().NotNil(l)

    res, err := s.prvAPIV1Client.Order(s.Ctx, &corev1.OrderRequest{...})
    s.Require().NoError(err)

    l2, err := s.levelTestRepository.GetByUserAndCurrency(s.Ctx, tests2.TestUserID, TestCurrency)
    s.Require().NoError(err)
    s.Require().Equal(l.CurAmount+1, l2.CurAmount, "CurAmount should be incremented by 1")
}
```

## Error Paths

Cover success and failure. A mock set to `nil` documents that the request must
fail before reaching it.

**Validation errors:**
```go
func (s *OrderTestSuite) TestSmallOrderAmountFail() {
    s.intSrv.OrderFunc = nil

    res, err := s.prvAPIV1Client.Order(s.Ctx, &corev1.OrderRequest{
        SessionID: tests.TestUnlimitedSessionID,
        CartID:    uuid.New().String(),
        Amount:    int64(1),
    })

    s.Require().EqualError(err, "rpc error: code = Unknown desc = [400]order amount is less than minimum order amount")
    s.Nil(res)
}
```

**Not found:**
```go
func (s *OrderTestSuite) Test_WhenSessionIDNotFound_Return404() {
    s.intSrv.OrderFunc = nil

    res, err := s.prvAPIV1Client.Order(s.Ctx, &corev1.OrderRequest{
        SessionID: "0c0066f9-a383-4a46-bd4b-8be5b04dc354",
        CartID:    uuid.New().String(),
        Amount:    1000,
    })

    s.Require().ErrorContains(err, "rpc error: code = Unknown desc = [404]session not found")
    s.Require().Nil(res)
}
```

**External service errors are proxied:**
```go
func (s *OrderTestSuite) Test_WhenIntegrationServerReturnsError_ProxifyIt() {
    s.intSrv.OrderFunc = func(ctx context.Context, req *integrationv1.OrderRequest) (*integrationv1.BalanceResponse, error) {
        return nil, domain.NewError("some message 123", errors.New("some error 456"))
    }

    res, err := s.prvAPIV1Client.Order(s.Ctx, &corev1.OrderRequest{...})

    s.Require().ErrorContains(err, "rpc error: code = Unknown desc = [500]some message 123(some error 456)")
    s.Require().Nil(res)
}
```

**Modes** (e.g. demo vs real): test each mode separately and assert which
dependency is (or is not) called.

## Reproduce minimally; model the real invariant

When writing a test that reproduces a bug (especially a red test before a fix), two failure modes waste everyone's time:

**1. Over-engineering the scenario to fit your theory of the cause.** Reproduce the *reported* behavior with the fewest entities. If the report is "one operation fails and the account is stuck," the test is **one** record where **one** call fails — not two records where the first blocks the second. Reaching for extra entities (a second txn/actor/record) to force the failure is a smell that you're testing your mental model of the cause, not the reported bug. Build the simplest thing that triggers it; add an entity only when the simplest form genuinely can't reproduce it.

**2. A fake that's more lenient than the real dependency — it hides the bug (false green).** A red test only means something if the fake enforces the same guard the real dependency does. When the behavior under test depends on a dependency's **lock / reservation / uniqueness / gate / constraint**, the per-test fake func MUST model that guard, or the test passes for the wrong reason and you "prove" a bug that's still there.

```go
// Bad — fake always lets the reserve succeed, so the retry never hits the real
// gate. The deadlock is invisible; the test is falsely green.
s.ledgerSrv.WithdrawBonusBalanceStartFunc = func(ctx, req) (*Resp, error) {
    return &Resp{...}, nil   // ❌ no reservation state, no "already in progress"
}

// Good — model the real single-slot guard in the closure (minimal state).
var reserved int64
s.ledgerSrv.WithdrawBonusBalanceStartFunc = func(ctx, req) (*Resp, error) {
    if reserved > 0 {                                   // the real gate
        return nil, fmt.Errorf("already in progress: reserved %d", reserved)
    }
    reserved = req.Amount
    return &Resp{...}, nil
}
s.ledgerSrv.WithdrawBonusBalanceFinishFunc = func(ctx, req) (*Resp, error) {
    reserved = 0
    return &Resp{...}, nil
}
```

Keep the modeled state minimal — plain `int64`/`bool` vars, scoped to the one account/key the test owns (filter other keys with `"not found"`, like the sibling tests). No `sync.Mutex` by default: a background processor handles records sequentially in one goroutine, so the fake funcs never run concurrently, and sibling tests use plain flags. Add synchronization only if the TEST goroutine also reads the fake's state (e.g. polling it in `Eventually`) — and prefer asserting on DB state via the test repo instead, which removes that need.

**Assert the DESIRED outcome, not the current broken behavior.** The red test asserts what *should* happen (the operation recovers / completes), so it fails now and flips green once the fix lands. To make the flip automatic, arrange the transient failure to eventually succeed (e.g. fake fails the first call, succeeds after) — pre-fix the guard stays stuck so it never reaches the success; post-fix it recovers and completes.
