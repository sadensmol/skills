# Fixtures and Test Data Management

## Overview

The platform uses two approaches for test data: **fixtures** for static testing and **dynamic data** for tests requiring on-the-fly creation.

## Fixtures (Static Test Data)

### What Are Fixtures?

Fixtures are pre-populated test data stored in the database, managed via goose migrations. They provide consistent, reusable test data across test runs.

**Location**: `db/fixtures/` or `testdata/fixtures/`

### When to Use Fixtures

Use fixtures for:
- **Static tests** - Tests that verify filtering, fetching, or querying against known data
- **Shared test data** - Data needed across multiple test suites
- **Complex data structures** - Relationships that are tedious to create repeatedly
- **Performance** - Avoiding repeated data creation in test setup

### Creating Fixtures

Fixtures are SQL files managed by goose or similar migration tools:

```bash
# Create a new fixture file
goose -dir db/fixtures create my_test_data sql

# Or from project root
cd db/fixtures
goose create my_test_data sql
```

Example fixture file (`db/fixtures/001_test_sessions.sql`):

```sql
-- +goose Up
INSERT INTO session (
    id,
    customer_id,
    external_id,
    user_id,
    product_id,
    currency,
    demo,
    created_at
) VALUES (
    '8eacba81-c3d4-4680-8c56-90ac841a8ec6',
    '872808ff-57fb-404e-bf23-bda822f78661',
    'test_ext_id',
    'test_user_123',
    'catch',
    'USD',
    false,
    NOW()
);

-- +goose Down
DELETE FROM session WHERE id = '8eacba81-c3d4-4680-8c56-90ac841a8ec6';
```

### Using Fixtures in Tests

Simply reference the fixture IDs in your tests:

```go
const (
    TestUnlimitedSessionID     = "8eacba81-c3d4-4680-8c56-90ac841a8ec6"  // From fixtures
    TestUnlimitedDemoSessionID = "8eacba81-c3d4-4680-8c56-90ac841a8ec7"  // From fixtures
)

func (s *OrderTestSuite) TestWithFixtureData() {
    // Use fixture session directly
    res, err := s.prvAPIV1Client.Order(s.Ctx, &corev1.OrderRequest{
        SessionID:    TestUnlimitedSessionID,  // From fixtures
        CartID: uuid.New().String(),
        Amount:  1000,
    })

    s.Require().NoError(err)
    // Assertions...
}
```

### Best Practices for Fixtures

1. **Use consistent IDs** - Hard-code well-known UUIDs for easy reference
2. **Document fixture data** - Add comments explaining what each fixture represents
3. **Keep fixtures minimal** - Only include data needed across multiple tests
4. **Version fixtures** - Use goose versioning for tracking changes
5. **Clean up properly** - Include Down migrations for rollback

## Dynamic Test Data

### When to Use Dynamic Data

Create data on-the-fly when:
- Test needs unique data (avoiding conflicts between parallel tests)
- Testing creation/deletion operations
- Data is test-specific and not reusable
- Testing edge cases with specific values

### Creating Dynamic Data

**Using helper methods:**
```go
func (s *OrderTestSuite) TestWithDynamicData() {
    // Create unique test data
    testUserID := uuid.NewString()
    sessionID := s.CreateNewSessionWithUserID(testUserID)
    defer s.DeleteSession(sessionID)  // Always clean up

    // Use in test
    res, err := s.prvAPIV1Client.Order(s.Ctx, &corev1.OrderRequest{
        SessionID: sessionID.String(),
        // ...
    })
}
```

**Using repositories directly:**
```go
func (s *FeatureTestSuite) TestWithRepositoryData() {
    // Create test record
    testRecord := &model.SomeData{
        ID:    uuid.New(),
        Field: "test_value",
        // ...
    }
    err := s.testRepository.Insert(s.Ctx, testRecord)
    s.Require().NoError(err)

    // Always clean up with defer
    defer func() {
        delErr := s.testRepository.Delete(s.Ctx, testRecord.ID)
        s.Require().NoError(delErr)
    }()

    // Test logic here
}
```

### Cleanup is Critical

**Always use defer** to ensure cleanup happens even if test fails:

```go
// Good - cleanup happens even on test failure
sessionID := s.CreateSession()
defer s.DeleteSession(sessionID)
s.RunTest(sessionID)

// Bad - cleanup might be skipped if test fails
sessionID := s.CreateSession()
s.RunTest(sessionID)
s.DeleteSession(sessionID)  // Never reached if RunTest fails
```

## Partition Pruning and Test Data

### Background

Many tables use partitioning by `created_at` for performance. Queries include `created_at` filters to enable partition pruning:

```go
// Typical query includes created_at filter
sessions, err := repo.GetByCustomerIDSince(ctx, customerID, time.Now().Add(-24*time.Hour))
```

### Problem with Old Fixtures

Fixtures might have old `created_at` timestamps, causing partition pruning to skip them:

```sql
-- Fixture created long ago
INSERT INTO session (..., created_at) VALUES (..., '2024-01-01');

-- Query only looks at recent partitions
SELECT * FROM session WHERE customer_id = ? AND created_at > NOW() - INTERVAL '1 day'
-- Won't find the fixture!
```

### Solution: Test Data ID List

Maintain a list of test/fixture data IDs in the repository layer. When querying by ID from this list, bypass partition pruning:

```go
type Repository struct {
    db          *sql.DB
    testDataIDs map[string]bool  // Track fixture/test IDs
}

func (r *Repository) GetByID(ctx context.Context, id string) (*Session, error) {
    // Check if this is test/fixture data
    if r.testDataIDs[id] {
        // Bypass partition pruning - don't add created_at filter
        return r.queryWithoutDateFilter(ctx, id)
    }

    // Production query with partition pruning
    return r.queryWithDateFilter(ctx, id, time.Now().Add(-24*time.Hour))
}
```

### Partition Pruning Guidelines by Table

Different tables have different partition pruning windows:

- **Ledger transactions**: 1 month (bonus transactions queried within a month)
- **Sessions**: 1 day (sessions typically last 4 hours, except e2e test sessions)
- **Carts and configs**: 3 months (historical data access)
- **Integration adapters**: 3 months (reconciliation queries)

### Working with Fixtures and Partitions

**Option 1: Use recent timestamps in fixtures**
```sql
-- Update fixture to use recent timestamp
INSERT INTO session (..., created_at) VALUES (..., NOW() - INTERVAL '1 hour');
```

**Option 2: Add fixture IDs to test data list**
```go
// In repository setup
testDataIDs := map[string]bool{
    "8eacba81-c3d4-4680-8c56-90ac841a8ec6": true,  // TestUnlimitedSessionID
    "8eacba81-c3d4-4680-8c56-90ac841a8ec7": true,  // TestUnlimitedDemoSessionID
}
```

**Option 3: Query without date filter in tests**
```go
// Test repository method that bypasses partition pruning
func (r *TestRepository) GetByIDWithoutPruning(ctx context.Context, id string) (*Session, error) {
    // Query without created_at filter
}
```

## Fixtures vs Dynamic Data Decision Matrix

| Scenario | Use Fixtures | Use Dynamic Data |
|----------|-------------|------------------|
| **Parallel test execution** | ❌ May conflict | ✅ Isolated |
| **Reusable across suites** | ✅ Yes | ❌ Test-specific |
| **Complex relationships** | ✅ Pre-built | ⚠️ Tedious to build |
| **Testing creation** | ❌ Already exists | ✅ Test creates it |
| **Testing deletion** | ⚠️ Affects other tests | ✅ Safe to delete |
| **Edge case values** | ⚠️ Limited variety | ✅ Any value |
| **Partition pruning** | ⚠️ Requires workaround | ✅ Recent data |

## Migration and Fixture Management

### Aggregating Old Migrations

Once migrations/fixtures are applied to all environments, they can be aggregated:

```bash
# Combine old migrations into single file
cat db/fixtures/001_*.sql db/fixtures/002_*.sql > db/fixtures/001_initial.sql
rm db/fixtures/001_*.sql db/fixtures/002_*.sql
```

This keeps the number of migration files manageable.

### Fixture Lifecycle

1. **Development**: Create fixture via goose
2. **Testing**: Use fixture in integration tests
3. **Applied to all envs**: Consider aggregating with other fixtures
4. **Deprecated**: Remove via Down migration if no longer needed

## Best Practices Summary

1. **Prefer fixtures** for shared, static test data
2. **Prefer dynamic data** for test-specific or unique data
3. **Always clean up** dynamic data with defer
4. **Document fixtures** with comments in SQL files
5. **Consider partition pruning** when working with old fixtures
6. **Use consistent IDs** in fixtures for easy reference
7. **Keep fixtures minimal** - only what's needed across tests
8. **Aggregate old migrations** to keep files manageable
