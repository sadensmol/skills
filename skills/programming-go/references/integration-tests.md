# Go Integration Tests

Read this when you create, change or run Go tests under `tests/`.

Integration tests use testify/suite with hierarchical suites, mocked external
services, and test-repository-managed data.

## Before Writing Tests

General test principles are in `sadensmol-programming` → *Testing* — load it with this
file. Go integration specifics (full text: `references/integration-tests/test-rules.md`):

1. **Check existing tests first** — see `sadensmol-programming` → *Testing*.
2. **Follow the same pattern** — see `sadensmol-programming` → *Testing*.
3. **NEVER touch the code when writing tests** — see `sadensmol-programming` → *Testing*.
4. **Use testify/suite** - All integration tests use testify/suite framework
5. **Fixtures for read-only tests ONLY** - If test modifies data, use dynamic data with defer cleanup
6. **Prefer `require.Eventually` over `time.Sleep`** — the ONLY exception is proving a
   background action did NOT happen (`references/integration-tests/common-patterns.md` → *Proving "it did
   NOT happen"*).
7. **NEVER modify mock server files** (shared test infrastructure). A nil mock func →
   set up the expectation in the test BEFORE the action that triggers the call.
8. **NEVER set up catch-all mocks in `SetupSuite`** — parallel suites share the DB. Set
   mocks up per test, through an explicitly called suite helper.
9. **NEVER call `wiremock.Reset()` (or any global mock reset) on a shared WireMock
   server** — delete only your own stubs. Read `references/integration-tests/mocking.md` → *WireMock:
   shared server, no global Reset* before writing any WireMock-backed test.
10. **NEVER declare `TestMain` in integration tests, for any purpose: not for setup, not
    for flags. NEVER use `init()` for test setup.** Setup goes in `SetupSuite()` or
    `SetupTest()`. A test flag such as `-parallel` goes on the `go test` command in the
    Makefile.
11. **ALWAYS use ONLY test repositories in test code — NEVER a production repository**,
    for setup, seeding, verification and cleanup (`references/integration-tests/common-patterns.md` →
    *Never call production repositories from tests*).
12. **Reproduce the reported bug with the FEWEST entities, and model the dependency's
    real guard.** Read `references/integration-tests/common-patterns.md` → *Reproduce minimally; model the
    real invariant* before writing a bug-reproduction test.
13. **Add methods to the EXISTING feature test file — never a parallel one-method file.**
14. **ONE `suite.Run` entry point per suite type, and a shared/base suite declares NO
    `Test` methods (MUST FOLLOW).** Read `references/integration-tests/suite-hierarchy.md` → *Never run the
    same suite twice* before adding a suite type or an entry point.
15. **Write NO comments in test code by default (MUST FOLLOW)** — `sadensmol-programming`
    → *Comments* applies to `*_test.go` too.

## Black-box only: control the world, not the service's internals

Drive the service only through its public API and assert on responses, logs
(`LogBuffer`) and DB state read via test repositories. Mocks of external dependencies
may return values, errors or hang/delay, within that dependency's real contract. Never
stub the service's **own** usecases/services/repositories. **Keep each service's tests in
its own vocabulary (MUST FOLLOW):** arrange internal preconditions through test
repositories, never as a mocked error. The expected-value oracle must be an independent
implementation. Full text: `references/integration-tests/test-rules.md`.

## Workflow

### 1. Locate or create the suite

Look in `tests/{api-type}/{version}/` for `{feature}_test.go`. If it exists, add
your methods there (rule 13). If not, create a feature suite that:

- lives in package `{apitype}{version}_test` (e.g. `privateapiv1_test`);
- embeds the API-level suite (e.g. `PrivateAPIV1TestSuite`);
- has exactly one runner `func TestFeatureTestSuite(t *testing.T) { suite.Run(t, &FeatureTestSuite{}) }`;
- calls the parent `SetupSuite()` first, then wires the **test** repositories it needs.

Suite levels and examples: read `references/integration-tests/suite-hierarchy.md` when creating a suite.

### 2. Write test methods

- Names: `TestFeatureSuccess()`, `TestFeatureConditionFail()`, or `Test_WhenCondition_ExpectResult()`.
- Structure: arrange → act → assert.
- Cover the success path, validation errors, not-found, and external-service errors.
- Table tests inside a suite: `s.Run(tt.name, …)`; names are sentences with spaces, not underscores; a `name` field only — no `description` field.

Patterns, error-format assertions, async waits, DB verification: read `references/integration-tests/common-patterns.md`.

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
not be reached. gRPC (bufconn) and WireMock details: read `references/integration-tests/mocking.md`.

### 5. Test Data

**CRITICAL RULE: Fixtures are for read-only tests ONLY.**

- **Fixtures** (static, from `db/fixtures`) when the test only READS: filtering,
  fetching, querying; complex related data that many tests read.
- **Dynamic data** (created in the test) when the test modifies, updates, or
  deletes data, needs unique values, or tests creation/deletion. NEVER modify
  fixture data — fixtures are shared, so changing them breaks other tests and
  causes races.

**CRITICAL cleanup rules:**
1. **Use test repositories** (e.g., `repository_test.Feature`) for delete operations
2. **Delete ONLY the specific record** created by this test - NEVER delete all records
3. **Use `defer` for cleanup - NEVER use `s.T().Cleanup()`**
4. **Delete by specific ID/key** - the same identifier used when creating the record
5. **NEVER create "DeleteAll" or bulk delete methods** - they destroy other tests' data
6. **If cleanup fails, the test fails** - don't swallow cleanup errors

Examples, why orphaned data breaks other suites, fixture files, goose, and
partition-pruning workarounds: read `references/integration-tests/fixtures-and-test-data.md`.

### 6. Verify database changes

For operations that change state, read the record before and after through the
**test** repository and assert the delta — see `references/integration-tests/common-patterns.md` →
*Database State Verification*.

### 7. Run Tests

**IMPORTANT: Always use VSCode MCP tasks to run tests** - never use command line directly.

**When tests fail:** read the logs first — `sadensmol-programming` → *Testing*.

**After changing existing tests:**
- Always run the test to ensure it still works
- Don't commit test changes without verifying they pass
- If test changes require code changes, ask first before modifying code

## Read on demand

- `references/integration-tests/test-rules.md` — read when a rule above is unclear or you are about to break
  one.
- `references/integration-tests/suite-hierarchy.md` — read when creating a suite type or an entry point; holds the 4 suite levels and *Never run the same suite twice*.
- `references/integration-tests/mocking.md` — read when configuring a gRPC mock server (bufconn) or any WireMock stub.
- `references/integration-tests/common-patterns.md` — read when writing test methods, async waits, DB verification, a bug reproduction, or a check that something did NOT happen.
- `references/integration-tests/fixtures-and-test-data.md` — read when adding/using fixtures, creating dynamic data and its cleanup, or when partition pruning hides fixture rows.
