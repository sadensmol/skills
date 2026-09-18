---
name: sadensmol-go-integration-tests
description: |
  Create integration tests for Go applications using testify/suite framework. Use when the user requests: (1) creating tests for new functionality, (2) covering new code with tests, (3) adding new related tests, (4) creating integration tests for Go services or applications, (5) testing API endpoints (gRPC, HTTP), (6) testing database interactions, (7) testing with external service dependencies. Guides test creation with proper suite hierarchy, gRPC mocking via bufconn, fixtures/test data management, and testify assertion patterns.
---

# Go Integration Tests

Create integration tests for Go applications using the testify/suite framework with hierarchical test suites, gRPC mocking, and comprehensive test data management.

## Before Writing Tests

Critical guidelines before creating any tests:

1. **Check existing tests first** - Understand patterns used in the project
2. **Follow the same pattern** - Don't invent new test styles
3. **NEVER touch the code when writing tests** - If code changes are needed, ask first
4. **Use testify/suite** - All integration tests use testify/suite framework
5. **Fixtures for read-only tests ONLY** - If test modifies data, use dynamic data with defer cleanup
6. **Prefer `require.Eventually` over `time.Sleep`** for async operations — poll for the expected state rather than sleeping a fixed time. The ONLY exception is proving a background action did NOT happen (a negative/stability assertion), which `Eventually` cannot express: there you must let a fixed number of worker cycles pass with `time.Sleep` and then assert nothing changed. See [Proving "it did NOT happen"](#proving-it-did-not-happen) below.
7. **NEVER modify mock server files** (ledger-server.go, integration-server.go, etc.) — these are shared test infrastructure. If a nil pointer panic occurs because a mock func is nil, set up the mock expectation in the test BEFORE the action that triggers the call. For background processors, move mock setup before the action that creates data the processor picks up.
8. **NEVER set up catch-all mocks in `SetupSuite`** — multiple test suites run in parallel sharing the same DB. Catch-all mocks let one suite's background processor steal records from another suite. Set up mocks only in specific tests that need them. Use a suite helper method to avoid duplication, called explicitly per-test.
9. **NEVER call `wiremock.Reset()` (or any global mock reset) on a shared WireMock server** — see [WireMock: shared server, no global Reset](#wiremock-shared-server-no-global-reset) below.
10. **NEVER use `TestMain` or `init()` for test setup** — put one-time setup in the suite's `SetupSuite()` (or `SetupTest()` for per-test setup), which runs before the suite's methods. Example: to capture logs, point the global logger at a buffer inside `SetupSuite()`, not in a `TestMain`. If two parallel suites both need it, each gets its own `SetupSuite()`.
11. **ALWAYS use ONLY test repositories in test code — NEVER a production repository.** Every DB read/write from a test (setup, seeding, verification, cleanup) goes through the `tests/repository` (`repository_test.*`) types. Test code MUST NOT instantiate or call a production `internal/repository` type — not for setup and not to "verify a production query." See [Never call production repositories from tests](#never-call-production-repositories-from-tests) below.
12. **Reproduce the reported bug with the FEWEST entities, and model the dependency's real guard.** Write the simplest test that triggers the *reported* failure — don't add extra records/txns/actors to fit your theory of the cause, and don't use a fake that's more lenient than the real dependency. See [Reproduce minimally; model the real invariant](#reproduce-minimally-model-the-real-invariant) below.
13. **Add methods to the EXISTING feature test file — never a parallel one-method file.** When a `*_test.go` already covers the feature (its suite embeds the API suite, has `SetupSuite`, sibling test methods), add your method to that file. A new file that re-uses the same suite type also duplicate-declares any method you copy. This is the concrete form of guideline #1.
14. **ONE `suite.Run` entry point per suite type, and a shared/base suite declares NO `Test` methods (MUST FOLLOW).** A base suite exists to carry setup and helpers for the suites that embed it. The moment a `Test` method lands on it, every entry point that runs that type executes it again — and two files each calling `suite.Run(t, new(SharedSuite))` silently doubles the whole set. See [Never run the same suite twice](#never-run-the-same-suite-twice) below.
15. **Write NO comments in test code by default (MUST FOLLOW).** The test method name + the arrange/act/assert body already say what the test does — a doc comment like `// TestFooDoesBar verifies that foo does bar` that restates the name is pure noise; delete it. Do NOT narrate the scenario ("open then close, then assert…") — the code shows it. Add a comment ONLY for a genuinely non-obvious *why* a competent reader would otherwise get wrong (a race the sleep guards, an ordering constraint, why a fixture is shaped oddly), and keep it to that one fact. Same rule as `go-programming`'s Comments section — it applies to `*_test.go` too, including per-test doc comments and inline narration.

## Never run the same suite twice

`suite.Run(t, new(X))` runs **every** `Test*` method on `X`, including the ones it
inherits from an embedded suite. So the number of times a test executes is the
number of entry points that reach its receiver type — not the number of times you
wrote it.

Two rules keep that at one:

1. **A shared/base suite declares no `Test` methods.** It holds `SetupSuite` /
   `SetupTest` and helpers. Tests live on the feature suites that embed it.
2. **Each suite type has exactly one `func TestXxxSuite(t *testing.T)`.**

**The failure mode, from a real repo.** One `BackupSuite` carried all 83 test
methods, and three files each ended with `suite.Run(t, new(BackupSuite))` — one
per "area" someone wanted to name. Result:

- **249 executions instead of 83.** Every assertion ran three times.
- **Triple the container time**, because each entry point runs `SetupSuite` and
  starts its own Postgres + LocalStack.
- **Cross-suite interference that looked like a product bug.** Each run seeded
  more rows into its stack and left background workers (a GC loop) sweeping
  shared storage. A GC test asserting "this orphan chunk was swept" failed
  depending on what a sibling run had seeded — passing in isolation, failing in
  a full run. Hours went into the "flaky GC" before the duplication was spotted.

```go
// Bad — three entry points on one type: every method runs three times
// backup_test.go
func TestBackupSuite(t *testing.T)          { suite.Run(t, new(BackupSuite)) }
// key_dao_test.go
func TestBackupKeyDAOSuite(t *testing.T)    { suite.Run(t, new(BackupSuite)) }   // ❌
// key_endpoints_test.go
func TestBackupKeyEndpointsSuite(t *testing.T) { suite.Run(t, new(BackupSuite)) } // ❌

// Good — base holds setup + helpers only; one suite and one entry point per feature
// suite.go
type BaseSuite struct{ tests.CloudSuite }                 // no Test methods, never run directly
func (s *BaseSuite) SetupTest()          { /* seed */ }
func (s *BaseSuite) startPass() string   { /* helper */ } // helpers on the base

// gc_test.go
type GCSuite struct{ BaseSuite }
func TestGCSuite(t *testing.T) { suite.Run(t, new(GCSuite)) }
func (s *GCSuite) TestGC_SweepsOrphans() { ... }

// key_dao_test.go
type KeyDAOSuite struct{ BaseSuite }
func TestKeyDAOSuite(t *testing.T) { suite.Run(t, new(KeyDAOSuite)) }
```

**Split by feature, not by file size.** One suite per role / feature / part of the
surface — `EndpointsSuite`, `PassLifecycleSuite`, `DeletionSuite`, `GCSuite`,
`StorageLimitSuite`, `KeyDAOSuite`, `KeyEndpointsSuite`, … A test belongs to the
suite for the thing it tests, not the file it was easiest to paste into.

**Check it cheaply.** These two greps should print one entry point per type and
nothing for the base:

```bash
grep -rn "^func Test.*Suite(t \*testing.T)" *_test.go   # one line per suite type
grep -c "func (s \*BaseSuite) Test" *.go                 # must be 0 everywhere
```

**The cost of doing it right.** Each suite runs its own `SetupSuite`, so N suites
means N container sets — in that repo, 3 suites at ~40s became 9 suites at ~125s,
of which ~11s per suite is container startup. That is the price of isolation and
it is usually worth paying: the shared-stack interference above is exactly what
per-suite containers prevent. If the wall-clock matters more than isolation,
share the containers behind a package-level `sync.Once` **inside `SetupSuite`**
(never a `TestMain` — see rule 10), and accept that suites then see each other's
rows.

## Black-box only: control the world, not the service's internals

Integration tests are **black-box**: drive the service-under-test only through its
public API (gRPC/HTTP client, queue) and assert on what comes out — responses,
logs (`LogBuffer`), and DB state read via test repositories. Never reach inside the
service to steer its control flow.

**Allowed — you own the world AROUND the service:**
- **External-dependency mocks** (gRPC mock servers like `ledgerSrv`/`intSrv`,
  WireMock): set a return value, return an error, or make them **hang/delay**.
  They're doubles for *external* services, so blocking one just simulates a slow/
  failing downstream — still black-box. E.g. a cancellation test blocks
  `ledgerSrv.EnsureAccountFunc` on a channel, cancels the request, and asserts the
  boundary logged the cancellation at warn — legitimate.
- **Preconditions via real state:** seed rows through **test repositories**, hold a
  real **Redis lock/lease** via a test repository/API, create **test sessions**.

**Forbidden — breaks black-box:**
- Reassigning or stubbing a method on the service's **own** internals (its
  usecases/services/repositories) to force a path or emulate a delay. If the test
  needs the service to block/branch, induce it through a real dependency (a hung
  external mock, a held redis lock, seeded data) — never by patching the service.

Rule of thumb: mutating a **mock of something the service calls out to** = fine;
mutating **the service's own code** = not a black-box test.

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

## The expected-value oracle must be an INDEPENDENT implementation

When a test computes an expected value to assert against (an oracle), that
computation MUST be written independently in the test — **never** call the
production function/formula it is checking. The point of the oracle is to catch a
regression in the production logic; if the test derives its expectation from the
same production code, it's a tautology that stays green even when the production
formula is wrong. **Do not "de-duplicate" the oracle** — and never export a
production function just so a test can reuse it for this. Duplication between the
test oracle and production is deliberate and correct here.

```go
// Good — the integration test re-derives the expected win itself, so a broken
// production calcWinAmount makes the balance assertion fail.
expectedWin := 0
if selected == resultMult {
    mult100 := int64(math.Round(float64(resultMult) * 100))
    expectedWin = int(int64(bet) * mult100 / 100)
}
expectedBalance := prevBalance - bet + expectedWin
s.Require().Equal(expectedBalance, gotBalance)

// Bad — oracle calls the very function under test; can never catch its bug.
win, _ := usecase.CalcWinAmount(selected, resultMult, bet) // ❌ tautology
s.Require().Equal(prevBalance-bet+win, gotBalance)
```

This is distinct from the DRY rule for production code: production stays DRY, but
a verification oracle is intentionally a second, independent implementation.
(A unit test of the formula itself still asserts hand-computed literals — same
principle: the expected numbers are written by hand, not produced by the code.)

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

## Proving "it did NOT happen"

To prove a background worker did NOT act on a row (e.g. a refund that must NOT be retried), don't call the selection query. Observe a side effect that the test repo can read. The cleanest signal is **`updated_at`**: every table with a `set_updated_at` BEFORE-UPDATE trigger bumps `updated_at` on every write, so each processing attempt advances it. A row that is processed once and then excluded keeps a **stable** `updated_at`; a retried row's `updated_at` keeps advancing.

```go
// process once, then record updated_at via the TEST repo
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

## Workflow

### Step 1: Understand What to Test

Identify what needs testing:

**For new endpoints/features:**
- What is the endpoint/method being tested?
- What are the success scenarios?
- What are the error scenarios? (validation errors, not found, external service errors)
- What external dependencies need mocking?

**For existing code coverage:**
- What functionality is currently untested?
- What edge cases are missing?
- What error paths are uncovered?

### Step 2: Locate or Create the Test Suite

**Decision: Where does this test belong?**

Check if a test suite already exists for the feature:
- Look in `tests/{api-type}/{version}/` or project root test directory
- Pattern: `{feature}_test.go` (e.g., `order_test.go`, `refund_test.go`)

**If suite exists:**
- Add new test methods to the existing suite
- Jump to Step 4

**If suite doesn't exist:**
- Create a new test suite file
- Continue to Step 3

### Step 3: Create Test Suite Structure

**For detailed suite hierarchy patterns, see [suite-hierarchy.md](references/suite-hierarchy.md)**

Create a new feature test suite:

```go
package privateapiv1_test  // Note: Use {apitype}{version}_test pattern

import (
    "testing"
    "github.com/stretchr/testify/suite"
    // Import repositories and dependencies
)

type FeatureTestSuite struct {
    PrivateAPIV1TestSuite  // Embed the API-level suite

    // Add feature-specific repositories
    featureRepository     *repository.Feature
    featureTestRepository *repository_test.Feature
}

func TestFeatureTestSuite(t *testing.T) {
    suite.Run(t, &FeatureTestSuite{})
}

func (s *FeatureTestSuite) SetupSuite() {
    s.PrivateAPIV1TestSuite.SetupSuite()

    // Set up feature-specific dependencies
    db, err := db.Setup(s.Cfg.Postgres)
    s.Require().NoError(err)

    s.featureRepository = repository.NewFeature(db)
    s.featureTestRepository = repository_test.NewFeature(db)
}
```

**Key points:**
- Package name must have `_test` suffix (e.g., `privateapiv1_test`)
- Embed the API-level suite (e.g., `PrivateAPIV1TestSuite`)
- Always include test runner function: `func TestFeatureTestSuite(t *testing.T)`
- Call parent `SetupSuite()` first
- Initialize repositories needed for this feature

### Step 4: Write Test Methods

**For common test patterns and examples, see [common-patterns.md](references/common-patterns.md)**

For each scenario, create a test method following these patterns:

**Test naming:**
- `TestFeatureSuccess()` - for success cases
- `TestFeatureConditionFail()` - for specific failures
- `Test_WhenCondition_ExpectResult()` - for complex conditions

**Test structure (Arrange-Act-Assert):**

```go
func (s *FeatureTestSuite) TestFeatureSuccess() {
    // Arrange - Set up test data
    testAmount := int64(1000)
    testID := uuid.New().String()
    expectedResult := "expected_value"

    // Arrange - Configure mock behavior (if needed)
    s.mockSrv.MethodFunc = func(ctx context.Context, req *pkg.Request) (*pkg.Response, error) {
        // Validate incoming request
        s.Require().Equal(testAmount, req.Amount)
        s.Require().NotEmpty(req.ID)

        // Return mock response
        return &pkg.Response{
            Result: expectedResult,
        }, nil
    }

    // Act - Call the API
    res, err := s.apiClient.Feature(s.Ctx, &api.Request{
        ID:     testID,
        Amount: testAmount,
    })

    // Assert - Verify results
    s.Require().NoError(err)
    s.Require().NotNil(res)
    s.Require().Equal(expectedResult, res.Result)
}
```

**For error scenarios:**

```go
func (s *FeatureTestSuite) Test_WhenIDNotFound_Return404() {
    s.mockSrv.MethodFunc = nil  // Won't reach mock

    res, err := s.apiClient.Feature(s.Ctx, &api.Request{
        ID: "non-existent-id",
    })

    s.Require().ErrorContains(err, "[404]not found")
    s.Require().Nil(res)
}
```

### Step 4b: Table-Driven Tests (Alternative Pattern)

For testing multiple scenarios of the same functionality, use table-driven tests:

```go
func (s *FeatureTestSuite) TestFeatureVariousInputs() {
    tests := []struct {
        name    string  // Use descriptive names with spaces
        input   string
        amount  int64
        wantErr bool
        errMsg  string
    }{
        {name: "processes valid input successfully", input: "valid", amount: 1000, wantErr: false},
        {name: "returns error when amount is zero", input: "valid", amount: 0, wantErr: true, errMsg: "amount required"},
        {name: "returns error when input is empty", input: "", amount: 1000, wantErr: true, errMsg: "input required"},
    }

    for _, tt := range tests {
        s.Run(tt.name, func() {
            res, err := s.apiClient.Feature(s.Ctx, &api.Request{
                Input:  tt.input,
                Amount: tt.amount,
            })

            if tt.wantErr {
                s.Require().Error(err)
                s.Require().ErrorContains(err, tt.errMsg)
                s.Require().Nil(res)
            } else {
                s.Require().NoError(err)
                s.Require().NotNil(res)
            }
        })
    }
}
```

**Table test guidelines:**
- Don't use underscores in test names - use spaces
- Name should describe what the test does (arrange-act-assert style)
- **Don't use description field** - just use name
- Use `s.Run()` for subtests within suite methods

### Step 4c: Test Data Constants

Use constants instead of magic strings or numbers in tests:

```go
// Package-level constants (uppercase) - shared across tests
const (
    TestExternalID = "test_ext_id"
    TestProductID = "test_product"
    TestAmount = int64(1000)
)

// Function-level constants (lowercase) - used in single test
func (s *FeatureTestSuite) TestFeature() {
    const (
        testEmail = "test@example.com"
        testName  = "Test User"
    )

    // Use constants in test
    res, err := s.apiClient.Feature(s.Ctx, &api.Request{
        Email: testEmail,
        Name:  testName,
    })

    s.Require().NoError(err)
    s.Require().Equal(testEmail, res.Email)
}

// Exception - one-time setup values can be inline
func (s *FeatureTestSuite) SetupTest() {
    s.timeout = 30 // OK - used once for setup
}
```

**Guidelines:**
- If constants are used in current test only, use lowercase (package private)
- If used across multiple tests, use uppercase (package exported)
- No magic strings or numbers where you arrange then assert
- Always use constants for amounts, IDs, and other test values

### Step 5: Configure Mocks (if needed)

**For detailed mocking patterns, see [mocking.md](references/mocking.md)**

If the feature calls external services:

**Check if mock server exists:**
- Look in `tests/` directory for mock server files (e.g., `integration-server.go`, `ledger-server.go`)
- If it exists, configure its behavior in your test
- If not, create a new mock server

**Configure mock in test:**

```go
s.integrationSrv.OrderFunc = func(ctx context.Context, req *integrationv1.OrderRequest) (*integrationv1.BalanceResponse, error) {
    // Validate request parameters
    s.Require().Equal(testAmount, req.Amount)

    // Return mock response
    return &integrationv1.BalanceResponse{
        Balance: expectedBalance,
    }, nil
}
```

**Key patterns:**
- Set `MockSrv.MethodFunc = nil` when mock shouldn't be called
- Validate request parameters in mock functions
- Use boolean flags to track if mocks were called
- Extract and validate gRPC metadata when needed

### Step 6: Handle Test Data

**For detailed fixture and test data patterns, see [fixtures-and-test-data.md](references/fixtures-and-test-data.md)**

**CRITICAL RULE: Fixtures are for read-only tests ONLY**

**Use fixtures (static test data)** when:
- **Test only READS data** (no modifications)
- Testing filtering, fetching, or querying operations
- Data structure is complex with relationships
- Multiple tests need the same read-only data

**Use dynamic data (on-the-fly creation)** when:
- **Test modifies, updates, or deletes data** (NEVER use fixtures for this)
- Tests run in parallel (avoid conflicts)
- Testing creation/deletion operations
- Data is test-specific or unique
- Testing edge cases with specific values

**Why this matters:**
- Fixtures are shared across all tests
- Modifying fixture data breaks other tests
- Creates race conditions and flaky tests
- Violates test isolation principles

**Using fixtures (READ-ONLY tests):**
```go
const (
    TestUnlimitedSessionID = "8eacba81-c3d4-4680-8c56-90ac841a8ec6"  // From fixtures
)

// ✅ GOOD - Read-only test using fixture
func (s *FeatureTestSuite) TestGetFeature() {
    res, err := s.apiClient.GetFeature(s.Ctx, &api.Request{
        ID: TestUnlimitedSessionID,  // From db/fixtures - only reading
    })
    s.Require().NoError(err)
    s.Require().NotNil(res)
}

// ❌ BAD - Modifying fixture data
func (s *FeatureTestSuite) TestUpdateFeature() {
    res, err := s.apiClient.UpdateFeature(s.Ctx, &api.Request{
        ID:    TestUnlimitedSessionID,  // ❌ DON'T modify fixture data
        Field: "new_value",
    })
}
```

**Creating dynamic data with helpers (for tests that modify data):**
```go
// ✅ GOOD - Dynamic data for update test
func (s *FeatureTestSuite) TestUpdateFeature() {
    testUserID := uuid.NewString()
    sessionID := s.CreateNewSessionWithUserID(testUserID)
    defer s.DeleteSession(sessionID)  // Always clean up in defer

    // Now safe to modify
    res, err := s.apiClient.UpdateFeature(s.Ctx, &api.Request{
        ID:    sessionID,
        Field: "new_value",
    })
    s.Require().NoError(err)
}
```

**Creating dynamic data with test repositories:**
```go
func (s *FeatureTestSuite) TestDeleteFeature() {
    // Create unique test data
    testID := uuid.New()
    expectedTxID := fmt.Sprintf("test_%s", testID)

    testData := &model.SomeData{
        ID:      testID,
        ExtTxID: expectedTxID,
        Field:   "value",
    }
    err := s.repository.Insert(s.Ctx, testData)
    s.Require().NoError(err)

    // IMMEDIATELY defer cleanup - use TEST repository, delete ONLY this record
    defer func() {
        delErr := s.testRepository.DeleteByExtTxID(s.Ctx, expectedTxID)
        s.Require().NoError(delErr)
    }()

    // Test logic
    res, err := s.apiClient.DeleteFeature(s.Ctx, &api.Request{
        ID: testData.ID,
    })
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

### Step 7: Verify Database Changes (when applicable)

For operations that modify database state:

```go
// Get state before
before, err := s.repository.GetByID(s.Ctx, id)
s.Require().NoError(err)

// Perform action
res, err := s.apiClient.Feature(s.Ctx, &api.Request{...})
s.Require().NoError(err)

// Get state after
after, err := s.repository.GetByID(s.Ctx, id)
s.Require().NoError(err)

// Verify changes
s.Require().Equal(before.Count+1, after.Count, "Count should increment")
```

### Step 8: Run Tests

**IMPORTANT: Always use VSCode MCP tasks to run tests** - never use command line directly.

**Standard Go test commands** (reference only, prefer VSCode MCP):

```bash
# Run all tests
go test ./...

# Run tests in specific package
go test -v ./tests/private-api/v1

# Run specific test suite or test method
go test -v ./tests/private-api/v1 -run TestOrderTestSuite
go test -v ./tests/private-api/v1 -run TestOrderTestSuite/TestRealSuccess

# Run with coverage
go test -v -cover ./...
```

**When tests fail:**
1. Check the logs first
2. If not enough logs, add more logs to understand what's happening
3. Don't guess what's going on - add logging to verify assumptions
4. Run the test again after adding logs

**After changing existing tests:**
- Always run the test to ensure it still works
- Don't commit test changes without verifying they pass
- If test changes require code changes, ask first before modifying code

## Coverage Guidelines

Ensure comprehensive coverage:

**For each endpoint, test:**
1. ✅ Success case (happy path)
2. ✅ Validation errors (invalid input)
3. ✅ Not found errors (missing resources)
4. ✅ External service errors (mock returning errors)
5. ✅ Authorization/permission errors (if applicable)
6. ✅ Edge cases specific to the feature

**For features with modes (demo vs real):**
- Test both modes separately
- Verify correct service is called (or not called)

**For financial operations:**
- Verify balance changes
- Verify database state changes
- Test rollback scenarios

## Quick Reference

**Common imports:**
```go
import (
    "context"
    "testing"

    "github.com/google/uuid"
    "github.com/stretchr/testify/suite"
    "google.golang.org/grpc/metadata"

    // Your project's packages
    "yourproject/tests"
    "yourproject/internal/domain"
)
```

**Common assertions:**
```go
s.Require().NoError(err)
s.Require().Equal(expected, actual)
s.Require().ErrorContains(err, "substring")
s.Require().NotNil(value)
s.Require().True(condition, "explanation")
```

**Async testing (CRITICAL - NEVER use time.Sleep):**
```go
// ✅ DO THIS - Use Eventually
s.Require().Eventually(func() bool {
    record, err := s.repository.FindByID(s.Ctx, id)
    if err != nil {
        return false
    }
    return record.Status == "completed"
}, tests.EventuallyWaitTime, tests.EventuallyPollInterval, "expected status to be completed")

// ❌ DON'T DO THIS - Never use time.Sleep
time.Sleep(10 * time.Millisecond)  // Flaky and unreliable
```

**Test data generation:**
```go
testID := uuid.New().String()
testUserID := uuid.NewString()
testAmount := int64(1000)
```

## Resources

### references/suite-hierarchy.md
Detailed guide to the test suite hierarchy (4 levels: IntegrationTestSuite → AppTestSuite → APITestSuite → FeatureTestSuite). Includes complete examples and package naming conventions for organizing tests.

### references/mocking.md
Complete guide to gRPC mocking with bufconn. Covers creating mock servers, registering mocks, configuring behavior, validating requests, and tracking mock calls.

### references/common-patterns.md
Common test patterns including test naming conventions, Arrange-Act-Assert structure, assertion examples, async testing with Eventually (NEVER use time.Sleep), test data management, cleanup patterns, and database verification.

### references/fixtures-and-test-data.md
Comprehensive guide to fixtures (static test data) vs dynamic test data, partition pruning considerations, goose migration management, and best practices for test data lifecycle.
