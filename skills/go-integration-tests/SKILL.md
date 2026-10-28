---
name: sadensmol-go-integration-tests
description: |
  Create integration tests for Go applications using testify/suite framework. Use when the user requests: (1) creating tests for new functionality, (2) covering new code with tests, (3) adding new related tests, (4) creating integration tests for Go services or applications, (5) testing API endpoints (gRPC, HTTP), (6) testing database interactions, (7) testing with external service dependencies. Guides test creation with proper suite hierarchy, gRPC mocking via bufconn, fixtures/test data management, and testify assertion patterns.
---

# Go Integration Tests

Integration tests use testify/suite with hierarchical suites, mocked external
services, and test-repository-managed data.

## Before Writing Tests

1. **Check existing tests first** - Understand patterns used in the project
2. **Follow the same pattern** - Don't invent new test styles
3. **NEVER touch the code when writing tests** - If code changes are needed, ask first
4. **Use testify/suite** - All integration tests use testify/suite framework
5. **Fixtures for read-only tests ONLY** - If test modifies data, use dynamic data with defer cleanup
6. **Prefer `require.Eventually` over `time.Sleep`** for async operations — poll for the expected state rather than sleeping a fixed time. The ONLY exception is proving a background action did NOT happen (a negative/stability assertion), which `Eventually` cannot express: there you must let a fixed number of worker cycles pass with `time.Sleep` and then assert nothing changed. See [Proving "it did NOT happen"](#proving-it-did-not-happen) below.
7. **NEVER modify mock server files** (ledger-server.go, integration-server.go, etc.) — these are shared test infrastructure. If a nil pointer panic occurs because a mock func is nil, set up the mock expectation in the test BEFORE the action that triggers the call. For background processors, move mock setup before the action that creates data the processor picks up.
8. **NEVER set up catch-all mocks in `SetupSuite`** — multiple test suites run in parallel sharing the same DB. Catch-all mocks let one suite's background processor steal records from another suite. Set up mocks only in specific tests that need them. Use a suite helper method to avoid duplication, called explicitly per-test.
9. **NEVER call `wiremock.Reset()` (or any global mock reset) on a shared WireMock server** — delete only your own stubs and their journal entries (`DeleteStub`, `DeleteRequestsByCriteria`); never `Reset()` / `DeleteAllRequests()`. Read `references/mocking.md` → *WireMock: shared server, no global Reset* before writing any WireMock-backed test.
10. **NEVER use `TestMain` or `init()` for test setup** — put one-time setup in the suite's `SetupSuite()` (or `SetupTest()` for per-test setup), which runs before the suite's methods. Example: to capture logs, point the global logger at a buffer inside `SetupSuite()`, not in a `TestMain`. If two parallel suites both need it, each gets its own `SetupSuite()`.
11. **ALWAYS use ONLY test repositories in test code — NEVER a production repository.** Every DB read/write from a test (setup, seeding, verification, cleanup) goes through the `tests/repository` (`repository_test.*`) types. Test code MUST NOT instantiate or call a production `internal/repository` type — not for setup and not to "verify a production query." See [Never call production repositories from tests](#never-call-production-repositories-from-tests) below.
12. **Reproduce the reported bug with the FEWEST entities, and model the dependency's real guard.** Write the simplest test that triggers the *reported* failure — don't add extra records/txns/actors to fit your theory of the cause, and don't use a fake that's more lenient than the real dependency. Read `references/common-patterns.md` → *Reproduce minimally; model the real invariant* before writing a bug-reproduction test.
13. **Add methods to the EXISTING feature test file — never a parallel one-method file.** When a `*_test.go` already covers the feature (its suite embeds the API suite, has `SetupSuite`, sibling test methods), add your method to that file. A new file that re-uses the same suite type also duplicate-declares any method you copy. This is the concrete form of guideline #1.
14. **ONE `suite.Run` entry point per suite type, and a shared/base suite declares NO `Test` methods (MUST FOLLOW).** A base suite exists to carry setup and helpers for the suites that embed it. The moment a `Test` method lands on it, every entry point that runs that type executes it again — and two files each calling `suite.Run(t, new(SharedSuite))` silently doubles the whole set. Split suites by feature, not file size. Read `references/suite-hierarchy.md` → *Never run the same suite twice* before adding a suite type or an entry point.
15. **Write NO comments in test code by default (MUST FOLLOW).** The test method name + the arrange/act/assert body already say what the test does — a doc comment like `// TestFooDoesBar verifies that foo does bar` that restates the name is pure noise; delete it. Do NOT narrate the scenario ("open then close, then assert…") — the code shows it. Add a comment ONLY for a genuinely non-obvious *why* a competent reader would otherwise get wrong (a race the sleep guards, an ordering constraint, why a fixture is shaped oddly), and keep it to that one fact. Same rule as `go-programming`'s Comments section — it applies to `*_test.go` too, including per-test doc comments and inline narration.

## Black-box only: control the world, not the service's internals

Drive the service only through its public API (gRPC/HTTP client, queue) and
assert on what comes out — responses, logs (`LogBuffer`), and DB state read via
test repositories.

- **Allowed — the world AROUND the service:** external-dependency mocks (gRPC
  mock servers, WireMock) may return values, return errors, or **hang/delay**
  (e.g. block `ledgerSrv.EnsureAccountFunc` on a channel, cancel the request,
  assert the boundary logged the cancellation at warn). Preconditions come from
  real state: rows seeded via **test repositories**, a real **Redis lock/lease**
  held via a test repository/API, **test sessions**.
- **Forbidden:** reassigning or stubbing a method on the service's **own**
  usecases/services/repositories to force a path or a delay. Induce it through a
  real dependency instead.

Rule of thumb: mutating a **mock of something the service calls out to** = fine;
mutating **the service's own code** = not a black-box test.

**Keep each service's tests in its own vocabulary (MUST FOLLOW).** Test partner-specific
error handling at the adapter boundary. For a state-dependent failure inside the service
under test, arrange the state through test repositories, call its public API, and assert
the resulting state. A mocked neighbour may return errors, delay or hang, but must follow
that dependency's actual contract and guards. Never replace an internal DB precondition
with a mocked error. Proxy tests may return and assert external errors when error
propagation is the behavior under test.

## The expected-value oracle must be an INDEPENDENT implementation

A test that computes an expected value must compute it **independently** —
**never** call the production function it is checking; that is a tautology that
stays green when the formula is wrong. **Do not "de-duplicate" the oracle**, and
never export a production function just so a test can reuse it. (A unit test of
the formula asserts hand-computed literals — same principle.)

```go
// Good — re-derives the expected win itself
expectedWin := 0
if selected == resultMult {
    mult100 := int64(math.Round(float64(resultMult) * 100))
    expectedWin = int(int64(bet) * mult100 / 100)
}
s.Require().Equal(prevBalance-bet+expectedWin, gotBalance)

// Bad — oracle calls the function under test
win, _ := usecase.CalcWinAmount(selected, resultMult, bet) // ❌ tautology
```

## Never call production repositories from tests

Test code talks to the DB **only** through the `tests/repository` test repos. A production repository (`internal/repository`) is production code exercised by the service/processor under test — the test harness must never construct one (`repository.NewX(db)`) or call its methods.

**This includes verifying a production query.** It is tempting to assert a background worker's retry/selection query by calling it directly:

```go
// Bad — reaches into the production repo to check a query, and scans a shared,
// parallel-populated table with a magic limit to find one row.
rows, _ := s.refundRepository.GetFirstNotProcessedWithTestAndLimit(ctx, true, 100000) // ❌
found := false
for _, r := range rows { if r.TxID.UUID() == myTxID { found = true } }
s.Require().False(found)
```

Two things are wrong: (1) it calls a production repo from a test, and (2) checking membership in a globally-shared list with a huge `LIMIT` is non-deterministic across parallel suites. Instead, verify the **observable behavior** the query drives, read via the **test** repo (see below).

Test repositories also own the delete methods production repos don't expose, which keeps test-only operations out of production code.

## Proving "it did NOT happen"

To prove a background worker did NOT act on a row, observe a side effect the
test repo can read. The cleanest signal is **`updated_at`**: a `set_updated_at`
BEFORE-UPDATE trigger bumps it on every write, so a row processed once and then
excluded keeps a **stable** `updated_at`, while a retried row's keeps advancing.

```go
s.Require().Eventually(func() bool {
    r, e := s.fooTestRepository.GetByID(ctx, id)
    return e == nil && r != nil && r.ResponseError != nil && *r.ResponseError == domain.ErrTerminal.Error()
}, tests.RetryWaitTime, tests.EventuallyPollInterval, "expected it to be processed once")

processed, _ := s.fooTestRepository.GetByID(ctx, id)
firstUpdatedAt := *processed.UpdatedAt

// let several worker cycles pass — a retried row would bump updated_at
time.Sleep(tests.RetryWaitTime)

after, _ := s.fooTestRepository.GetByID(ctx, id)
s.Require().Equal(firstUpdatedAt, *after.UpdatedAt, "must not be retried (updated_at unchanged)")
```

This is the one place `time.Sleep` is correct — you are asserting the ABSENCE of a change over time, which `Eventually` cannot express. Reads still go through the test repo only.

## Workflow

### 1. Locate or create the suite

Look in `tests/{api-type}/{version}/` for `{feature}_test.go`. If it exists, add
your methods there (rule 13). If not, create a feature suite that:

- lives in package `{apitype}{version}_test` (e.g. `privateapiv1_test`);
- embeds the API-level suite (e.g. `PrivateAPIV1TestSuite`);
- has exactly one runner `func TestFeatureTestSuite(t *testing.T) { suite.Run(t, &FeatureTestSuite{}) }`;
- calls the parent `SetupSuite()` first, then wires the **test** repositories it needs.

Suite levels and examples: read `references/suite-hierarchy.md` when creating a suite.

### 2. Write test methods

- Names: `TestFeatureSuccess()`, `TestFeatureConditionFail()`, or `Test_WhenCondition_ExpectResult()`.
- Structure: arrange → act → assert.
- Cover the success path, validation errors, not-found, and external-service errors.
- Table tests inside a suite: `s.Run(tt.name, …)`; names are sentences with spaces, not underscores; a `name` field only — no `description` field.

Patterns, error-format assertions, async waits, DB verification: read `references/common-patterns.md`.

### 3. Test data constants

- No magic strings or numbers where you arrange then assert — amounts, IDs and other test values are constants.
- Constants used in one test only: lowercase, declared in that test. Shared across tests: uppercase, package level.
- One-time setup values (e.g. a timeout set once in `SetupTest`) may stay inline.
- Unique per-run values come from `uuid.NewString()`.

### 4. Configure mocks

Look for an existing mock server in `tests/` (e.g. `integration-server.go`,
`ledger-server.go`) and configure its `…Func` fields in the test; create a new
mock server only if none exists. Validate request parameters inside the mock
func, track calls with boolean flags, set `MethodFunc = nil` when the mock must
not be reached. gRPC (bufconn) and WireMock details: read `references/mocking.md`.

### 5. Test Data

**CRITICAL RULE: Fixtures are for read-only tests ONLY.**

- **Fixtures** (static, from `db/fixtures`) when the test only READS: filtering,
  fetching, querying; complex related data that many tests read.
- **Dynamic data** (created in the test) when the test modifies, updates, or
  deletes data, needs unique values, or tests creation/deletion. NEVER modify
  fixture data — fixtures are shared, so changing them breaks other tests and
  causes races.

```go
func (s *FeatureTestSuite) TestDeleteFeature() {
    testID := uuid.New()
    expectedTxID := fmt.Sprintf("test_%s", testID)
    err := s.featureTestRepository.Insert(s.Ctx, &model.SomeData{ID: testID, ExtTxID: expectedTxID})
    s.Require().NoError(err)
    defer func() {
        delErr := s.featureTestRepository.DeleteByExtTxID(s.Ctx, expectedTxID)
        s.Require().NoError(delErr)
    }()

    _, err = s.apiClient.DeleteFeature(s.Ctx, &api.Request{ID: testID.String()})
    s.Require().NoError(err)
}
```

**CRITICAL cleanup rules:**
1. **Use test repositories** (e.g., `repository_test.Feature`) for delete operations
2. **Delete ONLY the specific record** created by this test - NEVER delete all records
3. **Use `defer` for cleanup - NEVER use `s.T().Cleanup()`** - defer runs immediately on function exit (even on panic), while `T().Cleanup()` may not run if process is killed
4. **Delete by specific ID/key** - the same identifier used when creating the record
5. **NEVER create "DeleteAll" or bulk delete methods** - they destroy other tests' data
6. **If cleanup fails, the test fails** - don't swallow cleanup errors

**WHY this matters - orphaned test data causes:**
- Background processors pick up stale records and call mocks that aren't configured
- Nil pointer panics when mock functions aren't set
- Flaky tests that pass/fail depending on database state
- Tests interfering with each other across runs

```go
// ❌ NEVER DO THIS - destroys other tests' data
func (r *TestRepo) DeleteAllPending(ctx context.Context) error {
    _, err := table.MyTable.DELETE().WHERE(table.MyTable.Status.EQ("pending")).Exec(tx)
    return err
}

// ✅ DO THIS - delete only the specific record you created
func (r *TestRepo) DeleteByExtTxID(ctx context.Context, txID string) error {
    _, err := table.MyTable.DELETE().WHERE(table.MyTable.ExtTxID.EQ(postgres.String(txID))).Exec(tx)
    return err
}
```

Fixture files, goose, and partition-pruning workarounds: read `references/fixtures-and-test-data.md`.

### 6. Verify database changes

For operations that change state, read the record before and after through the
**test** repository and assert the delta — see `references/common-patterns.md` →
*Database State Verification*.

### 7. Run Tests

**IMPORTANT: Always use VSCode MCP tasks to run tests** - never use command line directly.

**When tests fail:**
1. Check the logs first
2. If not enough logs, add more logs to understand what's happening
3. Don't guess what's going on - add logging to verify assumptions
4. Run the test again after adding logs

**After changing existing tests:**
- Always run the test to ensure it still works
- Don't commit test changes without verifying they pass
- If test changes require code changes, ask first before modifying code

## References

- `references/suite-hierarchy.md` — read when creating a suite type or an entry point; holds the 4 suite levels and *Never run the same suite twice*.
- `references/mocking.md` — read when configuring a gRPC mock server (bufconn) or any WireMock stub.
- `references/common-patterns.md` — read when writing test methods, async waits, DB verification, or a bug reproduction.
- `references/fixtures-and-test-data.md` — read when adding/using fixtures or when partition pruning hides fixture rows.
