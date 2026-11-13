# Swift Testing

Read this when you write or change tests.

## Testing

- Prefer **Swift Testing** (`import Testing`, `@Test`, `#expect`, `#require`) for new tests; it's the modern framework with parameterized tests and native async support. XCTest remains fine for existing suites and UI tests.
  ```swift
  @Test("scales bet to currency cents")
  func scaling() {
      #expect(CurrencyScaler.cents(1.5, .usd) == 150)
  }

  @Test(arguments: [(1.0, 100), (0.01, 1)])
  func parametrized(input: Double, cents: Int) {
      #expect(CurrencyScaler.cents(input, .usd) == cents)
  }
  ```
- Test async code directly with `async` test functions and `await`; use `#expect(throws:)` / `#require` for error paths.
- Inject dependencies (clock, network, storage) via protocols. Test principles: `sadensmol-programming` → *Testing*.
