# Common Test Patterns

## Test Naming Conventions

### Test Method Names

Use descriptive names that clearly indicate what is being tested:

**Pattern 1: TestFeatureCondition**
```go
func (s *OrderTestSuite) TestRealSuccess() {...}
func (s *OrderTestSuite) TestDemoSuccess() {...}
func (s *OrderTestSuite) TestSmallOrderAmountFail() {...}
```

**Pattern 2: Test_WhenCondition_ExpectResult**
```go
func (s *OrderTestSuite) Test_WhenSessionIDIsInvalid_Return500() {...}
func (s *OrderTestSuite) Test_WhenSessionIDNotFound_Return404() {...}
func (s *OrderTestSuite) Test_WhenUserBlacklisted_ReturnForbidden() {...}
func (s *OrderTestSuite) Test_WhenIntegrationServerReturnsError_ProxifyIt() {...}
```

Choose the pattern that best expresses the test's intent. Pattern 2 is more explicit for complex conditions.

## Test Structure (Arrange-Act-Assert)

### Standard Pattern

```go
func (s *FeatureTestSuite) TestFeatureSuccess() {
    // Arrange - Set up test data
    testAmount := int64(1000)
    testCartID := uuid.New().String()
    expectedBalance := int64(9000)

    // Arrange - Configure mocks
    s.mockSrv.MethodFunc = func(ctx context.Context, req *pkg.Request) (*pkg.Response, error) {
        s.Require().Equal(testAmount, req.Amount)
        return &pkg.Response{Balance: expectedBalance}, nil
    }

    // Act - Call the API
    res, err := s.apiClient.Feature(s.Ctx, &api.Request{
        ID:     tests.TestID,
        Amount: testAmount,
    })

    // Assert - Verify results
    s.Require().NoError(err)
    s.Require().NotNil(res)
    s.Require().Equal(expectedBalance, res.Balance)
}
```

## Assertions

### Basic Assertions

```go
// Error checking
s.Require().NoError(err)
s.Require().Error(err)

// Equality
s.Require().Equal(expected, actual)
s.Require().NotEqual(unexpected, actual)

// Nil checking
s.Require().Nil(value)
s.Require().NotNil(value)

// Boolean
s.Require().True(condition)
s.Require().False(condition)

// String matching
s.Require().Contains(actualString, "substring")
s.Require().NotContains(actualString, "substring")
```

### Error Message Assertions

```go
// Exact error message
s.Require().EqualError(err, "rpc error: code = Unknown desc = [400]order amount is less than minimum order amount")

// Partial error message (more flexible)
s.Require().ErrorContains(err, "rpc error: code = Unknown desc = [404]session not found")
s.Require().ErrorContains(err, "[403|account_blocked]user is blacklisted")
```

### Custom Error Messages

Add context to failed assertions:

```go
s.Require().Equal(l.CurAmount+1, l2.CurAmount, "CurAmount should be incremented by 1")
s.Require().False(integrationCalled, "integration server should not be called in demo mode")
```

## Async Testing Patterns

### Using Eventually Instead of time.Sleep

**CRITICAL RULE:** NEVER use `time.Sleep` for async testing. Always use `require.Eventually` from testify.

**Why:**
- `time.Sleep` makes tests flaky and slow
- `Eventually` polls until condition is met or timeout occurs
- More reliable and faster in most cases

**Bad (DO NOT USE):**
```go
// ❌ DON'T DO THIS
res, err := s.apiClient.CreateAsync(s.Ctx, req)
s.Require().NoError(err)

time.Sleep(10 * time.Millisecond)  // Flaky - might be too short or too long

// Check result
record, err := s.repository.FindByID(s.Ctx, res.ID)
s.Require().NoError(err)
s.Require().Equal("completed", record.Status)
```

**Good (USE THIS):**
```go
// ✅ DO THIS
res, err := s.apiClient.CreateAsync(s.Ctx, req)
s.Require().NoError(err)

// Use Eventually to wait for async operation
s.Require().Eventually(func() bool {
    record, err := s.repository.FindByID(s.Ctx, res.ID)
    if err != nil {
        return false
    }
    return record.Status == "completed"
}, tests.EventuallyWaitTime, tests.EventuallyPollInterval, "record should be marked as completed")
```

### Eventually Pattern with Project Constants

The project defines standard constants for Eventually assertions:

```go
// From tests/test-suite.go
const (
    EventuallyWaitTime     = 15 * time.Second  // Maximum wait time
    EventuallyPollInterval = 1 * time.Second   // Polling interval
)
```

**Always use these constants** for consistency across tests:

```go
s.Require().Eventually(func() bool {
    // Condition check
    return someConditionMet
}, tests.EventuallyWaitTime, tests.EventuallyPollInterval, "expected condition description")
```

### Common Eventually Patterns

**Waiting for Background Processing:**
```go
// After triggering async operation
s.Require().Eventually(func() bool {
    refund, err := s.refundRepository.FindByOrderID(s.Ctx, orderID)
    if err != nil {
        return false
    }
    return refund.Status == "processed"
}, tests.EventuallyWaitTime, tests.EventuallyPollInterval, "expected refund to be created and processed")
```

**Waiting for Record Creation:**
```go
s.Require().Eventually(func() bool {
    records, err := s.repository.List(s.Ctx, filter)
    if err != nil {
        return false
    }
    return len(records) > 0
}, tests.EventuallyWaitTime, tests.EventuallyPollInterval, "expected records to be created")
```

**Waiting for Status Change:**
```go
s.Require().Eventually(func() bool {
    refund, err := s.refundRepository.FindByID(s.Ctx, refundID)
    if err != nil {
        return false
    }
    return refund != nil && refund.Status == "confirmed"
}, tests.EventuallyWaitTime, tests.EventuallyPollInterval, "refund should still exist after retry processing")
```

**Waiting for Cache Population (Ristretto):**
```go
// If using Ristretto cache, prefer cache.Wait() over Eventually
cache.Wait()  // Ristretto-specific synchronization

// But if testing through API/service layer:
s.Require().Eventually(func() bool {
    result, err := s.service.GetCached(s.Ctx, key)
    return err == nil && result != nil
}, tests.EventuallyWaitTime, tests.EventuallyPollInterval, "cache should be populated")
```

### When NOT to Use Eventually

**Don't use Eventually for synchronous operations:**
```go
// ❌ WRONG - CreateUser is synchronous
s.Require().Eventually(func() bool {
    _, err := s.service.CreateUser(s.Ctx, data)
    return err == nil
}, tests.EventuallyWaitTime, tests.EventuallyPollInterval)

// ✅ CORRECT - Just call it directly
user, err := s.service.CreateUser(s.Ctx, data)
s.Require().NoError(err)
```

## Test Data Management

### Creating Test Data in SetupSuite

```go
func (s *OrderTestSuite) SetupSuite() {
    s.PrivateAPIV1TestSuite.SetupSuite()

    db, err := db.Setup(s.Cfg.Postgres)
    s.Require().NoError(err)

    s.sessionRepository = repository.NewSession(db)
    s.sessionTestRepository = repository_test.NewSession(db)
}
```

### Using Helper Methods

```go
func (s *OrderTestSuite) TestWithCustomUser() {
    testUserID := uuid.NewString()
    sessionID := s.CreateNewSessionWithUserID(testUserID)
    defer s.DeleteSession(sessionID)

    // Test using custom session
    res, err := s.prvAPIV1Client.Order(s.Ctx, &corev1.OrderRequest{
        SessionID: sessionID.String(),
        // ...
    })
}
```

### Cleanup with Defer and Test Repositories

**CRITICAL RULES:**
1. **Use test repositories** for cleanup (e.g., `repository_test.Feature`), not production repositories
2. **Delete ONLY the specific record** created by this test - NEVER delete all records
3. **Use defer immediately after creating data** - ensures cleanup even if test fails
4. **Delete by specific ID/key** - the same identifier used when creating the record

**Why test repositories?**
- Test repositories have delete methods that production repos may not expose
- Keeps test-only operations separate from production code
- Prevents accidental use of dangerous delete operations in production

**Pattern: Create data, then defer delete by ID**

```go
func (s *FeatureTestSuite) TestWithDynamicData() {
    // Create unique test data
    testID := uuid.New()
    expectedTxID := fmt.Sprintf("test_%s", testID)

    // Insert test record
    record := &model.OutboxRecord{
        ID:      testID,
        ExtTxID: expectedTxID,
        Status:  "pending",
    }
    err := s.outboxRepository.Insert(s.Ctx, record)
    s.Require().NoError(err)

    // IMMEDIATELY defer cleanup - delete ONLY this specific record
    defer func() {
        // Use test repository, delete by the specific ID we created
        delErr := s.outboxTestRepository.DeleteByExtTxID(s.Ctx, expectedTxID)
        s.Require().NoError(delErr)
    }()

    // Test logic here...
    res, err := s.apiClient.ProcessRecord(s.Ctx, &api.Request{ID: testID})
    s.Require().NoError(err)
}
```

**Multiple records - clean up each one:**

```go
func (s *OrderTestSuite) Test_WhenUserBlacklisted_ReturnForbidden() {
    testUserID := uuid.NewString()
    sessionID := s.CreateNewSessionWithUserID(testUserID)
    defer s.DeleteSession(sessionID)  // Cleanup #1: session

    blacklistEntry := &model.UserBlacklist{
        CustomerID: uuid.MustParse(tests2.TestCustomerID),
        ExternalID: tests2.TestExternalID,
        UserID:     testUserID,
        Reason:     "test blacklist",
    }
    err := s.userBlacklistTestRepository.Insert(s.Ctx, blacklistEntry)
    s.Require().NoError(err)

    defer func() {
        // Cleanup #2: blacklist entry - delete by specific identifiers
        delErr := s.userBlacklistTestRepository.DeleteByCustomerIDAndExternalIDAndUserID(
            s.Ctx,
            uuid.MustParse(tests2.TestCustomerID),
            tests2.TestExternalID,
            testUserID,
        )
        s.Require().NoError(delErr)
    }()

    // Test logic here
}
```

**NEVER do this:**

```go
// ❌ BAD - Deletes ALL pending records, not just the one this test created
defer func() {
    s.testRepository.DeleteAllPending(s.Ctx)
}()

// ❌ BAD - No cleanup at all
// Test creates data but doesn't clean up

// ❌ BAD - Using T().Cleanup() when defer is simpler and clearer
s.T().Cleanup(func() {
    s.testRepository.DeleteByID(s.Ctx, testID)
})
```

**Always do this:**

```go
// ✅ GOOD - Delete ONLY the specific record by its unique identifier
defer func() {
    delErr := s.testRepository.DeleteByExtTxID(s.Ctx, expectedTxID)
    s.Require().NoError(delErr)
}()
```

## Database State Verification

### Before/After Pattern

Verify state changes in the database:

```go
func (s *OrderTestSuite) TestRealSuccess() {
    // Get state before
    l, err := s.levelRepository.GetByCustomerIDAndExternalIDAndUserIDAndCurrencyAndProductAndDemo(
        s.Ctx,
        domain.NewMustCustomerIDFromString(tests2.TestCustomerID),
        domain.NewExternalIDFromString(tests2.TestExternalID),
        domain.UserID(tests2.TestUserID),
        TestCurrency,
        tests.TestProductID,
        false,
    )
    s.Require().NoError(err)
    s.Require().NotNil(l)

    // Perform action
    res, err := s.prvAPIV1Client.Order(s.Ctx, &corev1.OrderRequest{...})
    s.Require().NoError(err)

    // Get state after
    l2, err := s.levelRepository.GetByCustomerIDAndExternalIDAndUserIDAndCurrencyAndProductAndDemo(
        s.Ctx,
        domain.NewMustCustomerIDFromString(tests2.TestCustomerID),
        domain.NewExternalIDFromString(tests2.TestExternalID),
        domain.UserID(tests2.TestUserID),
        TestCurrency,
        tests.TestProductID,
        false,
    )
    s.Require().NoError(err)

    // Verify changes
    s.Require().Equal(l.CurAmount+1, l2.CurAmount, "CurAmount should be incremented by 1")
}
```

## Testing Different Scenarios

### Success Path

```go
func (s *FeatureTestSuite) TestSuccess() {
    // Configure successful mock response
    s.mockSrv.MethodFunc = func(ctx context.Context, req *pkg.Request) (*pkg.Response, error) {
        return &pkg.Response{Status: "success"}, nil
    }

    res, err := s.apiClient.Feature(s.Ctx, &api.Request{...})

    s.Require().NoError(err)
    s.Require().NotNil(res)
}
```

### Error Paths

**Validation Errors:**
```go
func (s *OrderTestSuite) TestSmallOrderAmountFail() {
    s.intSrv.OrderFunc = nil  // Won't reach mock

    res, err := s.prvAPIV1Client.Order(s.Ctx, &corev1.OrderRequest{
        SessionID:    tests.TestUnlimitedSessionID,
        CartID: uuid.New().String(),
        Amount:  int64(1),  // Too small
    })

    s.Require().EqualError(err, "rpc error: code = Unknown desc = [400]order amount is less than minimum order amount")
    s.Nil(res)
}
```

**Not Found Errors:**
```go
func (s *OrderTestSuite) Test_WhenSessionIDNotFound_Return404() {
    s.intSrv.OrderFunc = nil

    res, err := s.prvAPIV1Client.Order(s.Ctx, &corev1.OrderRequest{
        SessionID:    "0c0066f9-a383-4a46-bd4b-8be5b04dc354", // Non-existent
        CartID: uuid.New().String(),
        Amount:  1000,
    })

    s.Require().ErrorContains(err, "rpc error: code = Unknown desc = [404]session not found")
    s.Require().Nil(res)
}
```

**External Service Errors:**
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

### Conditional Behavior

```go
func (s *OrderTestSuite) TestDemoVsRealMode() {
    // Test demo mode
    res1, err1 := s.prvAPIV1Client.Order(s.Ctx, &corev1.OrderRequest{
        SessionID: tests.TestDemoSessionID,  // Demo session
        // ...
    })
    s.Require().NoError(err1)

    // Test real mode
    res2, err2 := s.prvAPIV1Client.Order(s.Ctx, &corev1.OrderRequest{
        SessionID: tests.TestRealSessionID,  // Real session
        // ...
    })
    s.Require().NoError(err2)
}
```

## Constants and Test Data

### Defining Test Constants

```go
const (
    // API-level constants
    TestExtSessionID = "test_ext_session_id"
    TestQuantity     = 11

    // User test data
    TestCurrency      = "USD"
    TestPlatform      = "desktop"
    TestReturnURL     = "https://example.com/return"
    TestUserPrefix    = "test_user_"
    TestUserFirstName = "test_user"
    TestUserLastName  = "test_user"
    TestUserNickName  = "test_user_nickname"
    TestUserCountry   = "Georgia"
)
```

### Dynamic Test Data

```go
func (s *FeatureTestSuite) TestFeature() {
    // Generate unique IDs for each test run
    testCartID := uuid.New().String()
    testUserID := uuid.NewString()

    // Use in test
}
```

## Best Practices

1. **Always use Require for setup** - Tests should fail fast if setup fails
2. **Use defer for cleanup** - Ensure resources are cleaned up even if test fails
3. **Test one thing per test** - Keep tests focused on a single scenario
4. **Use descriptive variable names** - testAmount, expectedBalance, not a, b, c
5. **Add assertion messages** - Explain why an assertion should pass
6. **Verify database changes** - Don't just check API response, verify persistence
7. **Reset mocks between tests** - Prevent test interference
8. **Use constants for test data** - Reuse common test values
9. **Test both success and failure paths** - Don't just test happy path
10. **Clean up test data** - Always delete created records with defer
11. **NEVER use time.Sleep** - Always use `require.Eventually` for async operations
