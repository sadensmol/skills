# Test Rules — full text

Read this when a rule in `references/integration-tests.md` → *Before Writing Tests* or *Black-box only* is
unclear, or you are about to break one. This is the full text of those sections.

## Before Writing Tests

General test principles (check existing tests and follow their pattern, never touch the
code while writing tests, independent oracle, black-box, assert on meaning, no comments)
are in `sadensmol-programming` → *Testing* — load it with this skill. Go integration
specifics:

1. **Check existing tests first** — see `sadensmol-programming` → *Testing*.
2. **Follow the same pattern** — see `sadensmol-programming` → *Testing*.
3. **NEVER touch the code when writing tests** — see `sadensmol-programming` → *Testing*.
4. **Use testify/suite** - All integration tests use testify/suite framework
5. **Fixtures for read-only tests ONLY** - If test modifies data, use dynamic data with defer cleanup
6. **Prefer `require.Eventually` over `time.Sleep`** for async operations — poll for the expected state rather than sleeping a fixed time. The ONLY exception is proving a background action did NOT happen (a negative/stability assertion), which `Eventually` cannot express: there you must let a fixed number of worker cycles pass with `time.Sleep` and then assert nothing changed. See `common-patterns.md` → *Proving "it did NOT happen"*.
7. **NEVER modify mock server files** (ledger-server.go, integration-server.go, etc.) — these are shared test infrastructure. If a nil pointer panic occurs because a mock func is nil, set up the mock expectation in the test BEFORE the action that triggers the call. For background processors, move mock setup before the action that creates data the processor picks up.
8. **NEVER set up catch-all mocks in `SetupSuite`** — multiple test suites run in parallel sharing the same DB. Catch-all mocks let one suite's background processor steal records from another suite. Set up mocks only in specific tests that need them. Use a suite helper method to avoid duplication, called explicitly per-test.
9. **NEVER call `wiremock.Reset()` (or any global mock reset) on a shared WireMock server** — delete only your own stubs and their journal entries (`DeleteStub`, `DeleteRequestsByCriteria`); never `Reset()` / `DeleteAllRequests()`. Read `references/mocking.md` → *WireMock: shared server, no global Reset* before writing any WireMock-backed test.
10. **NEVER declare `TestMain` in integration tests, for any purpose: not for setup, not for flags. NEVER use `init()` for test setup.** Put one-time setup in the suite's `SetupSuite()` (or `SetupTest()` for per-test setup), which runs before the suite's methods. A test flag such as `-parallel` goes on the `go test` command in the Makefile. Example: to capture logs, point the global logger at a buffer inside `SetupSuite()`, not in a `TestMain`. If two parallel suites both need it, each gets its own `SetupSuite()`.
11. **ALWAYS use ONLY test repositories in test code — NEVER a production repository.** Every DB read/write from a test (setup, seeding, verification, cleanup) goes through the `tests/repository` (`repository_test.*`) types. Test code MUST NOT instantiate or call a production `internal/repository` type — not for setup and not to "verify a production query." See `common-patterns.md` → *Never call production repositories from tests*.
12. **Reproduce the reported bug with the FEWEST entities, and model the dependency's real guard.** Write the simplest test that triggers the *reported* failure — don't add extra records/txns/actors to fit your theory of the cause, and don't use a fake that's more lenient than the real dependency. Read `references/common-patterns.md` → *Reproduce minimally; model the real invariant* before writing a bug-reproduction test.
13. **Add methods to the EXISTING feature test file — never a parallel one-method file.** When a `*_test.go` already covers the feature (its suite embeds the API suite, has `SetupSuite`, sibling test methods), add your method to that file. A new file that re-uses the same suite type also duplicate-declares any method you copy. This is the concrete form of guideline #1.
14. **ONE `suite.Run` entry point per suite type, and a shared/base suite declares NO `Test` methods (MUST FOLLOW).** A base suite exists to carry setup and helpers for the suites that embed it. The moment a `Test` method lands on it, every entry point that runs that type executes it again — and two files each calling `suite.Run(t, new(SharedSuite))` silently doubles the whole set. Split suites by feature, not file size. Read `references/suite-hierarchy.md` → *Never run the same suite twice* before adding a suite type or an entry point.
15. **Write NO comments in test code by default (MUST FOLLOW)** — `sadensmol-programming` → *Comments* applies to `*_test.go` too, including per-test doc comments and inline narration.

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

See `sadensmol-programming` → `references/testing.md`.
