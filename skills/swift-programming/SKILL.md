---
name: sadensmol-swift-programming
description: "Expert Swift programming guidance for building idiomatic, safe, modern Swift apps and libraries across Apple platforms (iOS, iPadOS, macOS, watchOS, tvOS, visionOS) and server/CLI. Use when (1) Writing or reviewing Swift code, (2) Setting up Swift packages or Xcode projects, (3) Designing types (structs, enums, protocols, generics) and value-vs-reference decisions, (4) Working with optionals, error handling, or the Swift API Design Guidelines, (5) Writing modern concurrency (async/await, actors, structured concurrency, Sendable), (6) Building SwiftUI (mobile + desktop) or interoping with UIKit/AppKit, (7) Managing memory (ARC, weak/unowned, capture lists), (8) Writing tests (Swift Testing / XCTest), (9) Refactoring or debugging Swift. Enforces value semantics, immutability, protocol-oriented design, data-race safety, and idiomatic naming."
---

# Swift Programming

Act as a senior Swift developer. Keep solutions idiomatic, value-oriented, data-race-safe, and concise. Prefer the standard library and language features over custom abstractions. Target the latest Swift language mode (Swift 6, strict concurrency) unless the project pins an older toolchain. Always format and lint before declaring work complete.

## Tooling (non-negotiable)

- `swift-format` (Apple's, bundled with the toolchain) — the source of truth for formatting: `swift format -i -r Sources Tests`. Never hand-format. A `.swift-format` config lives at repo root.
- `swiftlint` — style/anti-pattern linter; must pass with zero warnings (`swiftlint --strict`). Autofix mechanical issues with `swiftlint --fix`.
- `swift build` / `swift test` for SwiftPM; `xcodebuild`/Xcode for app targets. Build must be warning-clean.
- Enable the strictest settings the project can bear: Swift 6 language mode (`swiftLanguageMode: .v6`) or, when migrating, `.enableUpcomingFeature("StrictConcurrency")` and treat warnings as errors in CI.
- Prefer SwiftPM (`Package.swift`) for libraries and modular code; reserve `.xcodeproj` for app/UI targets.

## Naming & API Design (Swift API Design Guidelines)

**Clarity at the point of use is the goal.** Read call sites aloud — they should form grammatical English phrases.

- Types, protocols: `UpperCamelCase`. Everything else (functions, methods, properties, cases, vars): `lowerCamelCase`. No `snake_case`, no Hungarian/`m_` prefixes.
- Include all words needed to avoid ambiguity; omit needless words that repeat type information.
  ```swift
  // ✗ redundant — the parameter type already says it's a String
  func remove(element: String)
  // ✓ reads at the call site: employees.remove(at: index)
  func remove(at index: Int) -> Element
  ```
- Name by role, not type (`greeting`, not `string`). Booleans read as assertions: `isEmpty`, `hasChanges`, `canEdit`.
- Methods with side effects read as imperative verbs (`list.sort()`, `queue.append(x)`); their non-mutating counterparts use the `-ed`/`-ing` rule (`sorted()`, `appending(x)`).
- Protocols describing *what something is* are nouns (`Collection`); protocols describing a *capability* end in `-able`/`-ible`/`-ing` (`Equatable`, `ProgressReporting`).
- First argument label reads into the method name; use `_` only when the first arg is truly the subject.
  ```swift
  func move(from start: Point, to end: Point)   // move(from: a, to: b)
  func contains(_ member: Element) -> Bool       // set.contains(x)
  ```
- Don't abbreviate (`viewController`, not `vc`) except for universally-known acronyms, which are all-caps or all-lower: `urlString`, `parseHTML`, `id`.

## Value Types First

**Default to `struct` and `enum`. Reach for `class` only when you need reference semantics.**

- Use a `class` when: identity matters (two instances with equal contents are still distinct), you need inheritance from an Objective-C/AppKit/UIKit type, shared mutable state with controlled lifetime, or deinitialization side effects.
- Use `struct`/`enum` for models, DTOs, view state, and most everything else — cheap copies, no aliasing bugs, free `Sendable` conformance when members are `Sendable`.
- Mark every class you don't intend to subclass `final` (better performance, clearer intent). Prefer composition + protocols over class inheritance.
- Model exhaustive, mutually-exclusive states as an `enum` with associated values rather than a struct full of optional/boolean flags.
  ```swift
  enum LoadState<Value> { case idle, loading, loaded(Value), failed(Error) }
  ```

## Optionals

- **Never force-unwrap (`!`) or force-cast (`as!`) in production paths.** Acceptable only for genuinely-impossible-to-fail cases with a comment, or `@IBOutlet`s.
- Use `guard let`/`if let` shorthand (Swift 5.7+) to bind and narrow early:
  ```swift
  guard let user else { return }          // shorthand — no `= user`
  ```
- `guard` for early exit at the top of a function (keeps the happy path un-indented); `if let` for a local branch.
- Nil-coalesce for defaults (`name ?? "Anonymous"`), optional-chain for reaching through (`user?.profile?.avatarURL`).
- Don't return `Optional<[T]>` or `Optional<String>` just to signal "empty" — return an empty collection/string. Optional means *absence*, not *emptiness*.
- Avoid `Optional` of `Optional`; flatten with `flatMap`.

## Immutability

- `let` by default; switch to `var` only when mutation is required. `swiftlint` will flag a `var` that's never mutated.
- Prefer immutable stored properties; expose `let` publicly and mutate through explicit `mutating` methods on value types.
- For arrays/dictionaries built up then never changed, build with `map`/`reduce` and bind to `let` rather than mutating a `var`.

## Error Handling

- Use `throws` for recoverable, caller-relevant failures; conform error types to `Error` (usually an `enum`, `LocalizedError` for user-facing messages).
- Use typed throws (`throws(MyError)`, Swift 6) when the error domain is closed and callers switch on it; untyped `throws` when it can propagate arbitrary errors.
- `Result<Success, Failure>` for stored/deferred outcomes and completion-handler boundaries — but prefer `async`/`throws` over completion handlers in new code.
- `try?` to convert to optional when the specific error is irrelevant; `try!` only when failure is a programmer error. Never swallow errors silently (`try? foo()` discarding a real failure is a smell — log or handle).
- `defer` for cleanup that must run on every exit path (close handles, balance locks).
  ```swift
  let handle = try FileHandle(forReadingFrom: url)
  defer { try? handle.close() }
  ```
- Don't use errors for control flow that isn't exceptional; return an enum/optional instead.

## Enums & Pattern Matching

- Exhaustive `switch` — avoid `default` when matching a closed enum so new cases surface as compile errors.
- Bind associated values and use `where` clauses:
  ```swift
  switch event {
  case let .tap(point) where point.y < 0: handleAbove(point)
  case .tap, .swipe: break
  case .longPress(let duration): handleHold(duration)
  }
  ```
- `if case`/`guard case` for single-case matches. Use tuple patterns, `for case let ... where`, and `is`/`as?` patterns to flatten nested conditionals.
- Give enums raw values only when you need the mapping (serialization); otherwise leave them raw-value-less.

## Protocols & Generics (Protocol-Oriented)

- Program to protocols; put shared default behaviour in protocol extensions rather than base classes.
- Constrain generics with `where` clauses; prefer generics (`some`/`<T>`) over type-erased existentials for performance.
- **`some` vs `any`:** use `some P` (opaque type — one concrete type, static dispatch, no boxing) for parameters and returns whenever the concrete type is fixed at the call site. Use `any P` (existential — dynamic, boxed) only when you genuinely need heterogeneous storage (`[any Shape]`). Swift now requires the explicit `any` keyword — that syntactic weight is a hint to reconsider.
  ```swift
  func total(of shapes: some Sequence<Shape>) -> Double   // generic, fast
  var layers: [any Renderable]                            // heterogeneous, needs existential
  ```
- Keep protocols small and focused (one capability). Compose with `&` (`Codable & Sendable`).
- Prefer conditional conformances (`extension Array: Drawable where Element: Drawable`) over wrapper types.

## Closures

- Use trailing-closure syntax; for multiple trailing closures use labeled syntax.
- Always specify capture semantics for closures that outlive the current scope — `[weak self]` (then `guard let self`) to break retain cycles, `[unowned self]` only when self is guaranteed alive.
- Mark long-lived stored closures `@escaping`; non-escaping is the default and cheaper.
- Prefer `map`/`filter`/`reduce`/`compactMap`/`flatMap` over manual loops for transformations; use a `for` loop when side effects or early exit dominate.

## Concurrency (async/await, actors, structured)

**Use Swift structured concurrency. Do NOT reach for GCD (`DispatchQueue`) or completion handlers in new code.**

- `async`/`await` for asynchronous work; `throws` composes with `async` (`try await`).
- Structured concurrency for parallelism: `async let` for a fixed number of children, `withTaskGroup`/`withThrowingTaskGroup` for a dynamic set. Children are awaited/cancelled with the parent — no leaks.
  ```swift
  async let a = fetchProfile()
  async let b = fetchSettings()
  let (profile, settings) = try await (a, b)
  ```
- Protect mutable shared state with an `actor` instead of locks; actors serialize access and prevent data races.
- UI and `@Observable`/view-model state belong on the main actor — annotate types/methods `@MainActor`. Never mutate UI off the main actor.
- Make types crossing concurrency boundaries `Sendable`. Value types of `Sendable` members are automatically `Sendable`; for a reference type that's safe, use `final class ... : Sendable` with immutable state, or `@unchecked Sendable` **only** with a documented locking invariant.
- Honor cancellation: check `Task.isCancelled` / call `try Task.checkCancellation()` in loops; propagate rather than swallow `CancellationError`.
- Bridge callbacks with `withCheckedThrowingContinuation` (resume **exactly once**). Consume async streams with `for await` over `AsyncSequence`/`AsyncStream`.
- Fix strict-concurrency warnings — do not silence them with unaudited `@unchecked Sendable` or `nonisolated(unsafe)`.

## Memory Management (ARC)

- ARC is automatic; retain cycles are the failure mode. Break them with `weak` (optional, becomes `nil`) or `unowned` (non-optional, must outlive) references.
- Classic cycles: parent↔child (child holds parent `weak`), delegates (`weak var delegate`), and closures capturing `self` (`[weak self]`).
- Value types don't participate in ARC — another reason to prefer them.
- `deinit` is for resource cleanup only; don't rely on ordering across objects.

## Collections & Functional Style

- Reach for standard-library algorithms: `map`, `compactMap`, `filter`, `reduce`, `first(where:)`, `contains(where:)`, `sorted(by:)`, `prefix`, `zip`, `enumerated`.
- Use `lazy` when chaining transforms over large sequences to avoid intermediate allocations.
- Prefer `for element in collection` over index-based loops; use `enumerated()` only when you truly need the index.
- Use `Set`/`Dictionary` for membership/lookup instead of linear `contains` over arrays.
- Prefer `isEmpty` over `count == 0`.

## Access Control

- Default to the most restrictive level that works: `private` → `fileprivate` → `internal` (default) → `package` (Swift 5.9, same package) → `public`/`open`.
- Libraries: expose the minimal `public` surface; mark classes `final` and only make them `open` when subclassing is an intended extension point.
- Use `private(set)` to expose read-only state while keeping mutation internal.

## Codable & Persistence

- Evolve persisted `Codable` models backward-compatibly: add new fields as OPTIONAL, and emit them only when non-default (synthesized `encodeIfPresent`, or a custom `encode(to:)`). A value at its defaults then serializes byte-identically to the old format — old readers keep working and no schema version bump is needed.
- Decode leniently: a custom `init(from:)` that treats a missing/garbage field as the default — `(try? c.decodeIfPresent(T.self, forKey: .x)) ?? default` — lets legacy and forward-compat payloads load instead of failing the whole decode on one bad key.
- Store the inverse when it keeps the common case absent (e.g. `collapsed: Bool?` encoded only when `true` rather than `expanded` encoded always), so the default state carries no key.
- Bump an explicit `version` only for a breaking change a default can't cover, and migrate on read.

## SwiftUI (mobile + desktop)

Prefer SwiftUI for new UI on all platforms; it's the shared UI layer across iOS, macOS, watchOS, tvOS, and visionOS.

- Views are value types — keep them small, composable, and cheap to recreate. Extract subviews and computed `var body` fragments rather than giant view bodies.
- **State ownership (modern):** use the `@Observable` macro for models/view-models (iOS 17+/macOS 14+); reference them with `@State` where owned and `@Environment`/plain `let` where injected. Reserve `@State` for view-local value state, `@Binding` for two-way child bindings. (Legacy: `ObservableObject` + `@Published` + `@StateObject`/`@ObservedObject` — use only when supporting older OS versions.)
- Never store view models in a way that recreates them each render — own them with `@State`/`@StateObject`, pass them down by reference.
- Keep side effects in `.task {}`/`.onChange`/model methods, not in `body`. `body` must be pure.
- Adapt to platform and size instead of branching per-OS: use `NavigationSplitView`, `.frame(minWidth:)`, `ControlGroup`, `@Environment(\.horizontalSizeClass)`, and multiplatform scene types (`WindowGroup`, `Settings`, `MenuBarExtra` on macOS).
- Drop to UIKit/AppKit only for gaps (`UIViewRepresentable`/`NSViewRepresentable`).

## UIKit / AppKit interop

- When maintaining UIKit/AppKit code: `weak var delegate`, dequeue/reuse cells, do layout in the documented lifecycle methods, and marshal all UI updates to `@MainActor`/main thread.
- Wrap platform views for SwiftUI via `UIViewRepresentable`/`NSViewRepresentable`; keep the `Coordinator` for delegate callbacks. Annotate the `Coordinator` `@MainActor` under strict concurrency — its delegate callbacks are all main-thread — so it can touch main-actor model state without warnings.
- **Observation dependency tracking:** for an `@Observable`-driven representable to re-render, READ the observed properties inside `updateNSView` — that read is what registers the representable as an observer. A read inside a delegate method (e.g. `outlineView(_:viewFor:)`) does NOT register the dependency, so mutations won't re-invoke `updateNSView`. Touch every property the view depends on there (even just `_ = model.x`).
- **Stable item identity:** `NSOutlineView`/`NSTableView` key item identity, expansion, and selection by object identity (`===`). Back rows with cached reference-type nodes — one instance per model id, children repopulated on reload — never hand back fresh structs each reload, or expansion/selection silently resets.
- Isolate `#if os(macOS)` / `#if os(iOS)` platform differences behind small typealiases or wrapper types instead of scattering conditionals through logic.

## Modularity & Packages

- Split large apps into SwiftPM modules (feature/domain libraries) with explicit `public` boundaries — improves build times and enforces layering.
- Keep a **host-free core module** — pure model, logic, parsing, validation, routing — that imports NO UI framework (`AppKit`/`UIKit`/`CoreGraphics`/`Metal`). Use plain/`Double`-backed geometry and convert to platform types at the app boundary. It then compiles under Swift 6 complete concurrency checking and unit-tests with no app host (fast, deterministic); the app target stays a thin side-effect adapter over it. Put the tricky pure logic (index math, state transitions, decision resolution) here as static functions and table-test it; the app only maps ids and applies effects.
- Depend on protocols across module boundaries; inject implementations (constructor injection) rather than reaching for singletons.
- Keep `Package.swift` targets focused; put tests in matching `...Tests` targets.

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
- Follow Arrange-Act-Assert; one behavior per test; name tests by behavior, not method.
- Inject dependencies (clock, network, storage) via protocols so tests are deterministic — no real network, no `sleep`, no shared global state.
- Write a failing test first **only when fixing a bug** (reproduce, then fix). New features are implementation-first, then covered.

## Documentation

- Document public API with DocC comments (`///`), using `- Parameters:`, `- Returns:`, `- Throws:`. Explain *why* and non-obvious contracts, not *what* the signature already says.
- Keep comments truthful and current; delete stale ones. Prefer self-documenting names over explanatory comments.

## Review Checklist

- [ ] `swift format` applied; `swiftlint --strict` clean; build warning-free (Swift 6 mode where possible).
- [ ] Value types (`struct`/`enum`) used unless reference semantics are truly needed; non-subclassed classes are `final`.
- [ ] No force-unwrap/`try!`/`as!` on production paths; optionals handled with `guard`/`if let`/`??`.
- [ ] `let` over `var`; the minimal `public`/`open` surface; `private(set)` where appropriate.
- [ ] Errors are `throws`/`Result`, never silently swallowed; `defer` balances resources.
- [ ] Exhaustive `switch` (no `default`) over closed enums; states modeled as enums, not flag soup.
- [ ] `some` used over `any` unless heterogeneity is required.
- [ ] Structured concurrency (async/await, actors, task groups) — no new GCD/completion handlers; UI on `@MainActor`; boundary-crossing types are `Sendable`; strict-concurrency warnings resolved (not suppressed).
- [ ] Retain cycles broken with `[weak self]`/`weak delegate`; capture lists explicit on escaping closures.
- [ ] SwiftUI views small and pure; state owned correctly (`@Observable`/`@State`/`@Binding`); side effects out of `body`.
- [ ] AppKit-in-SwiftUI: observed properties read in `updateNSView`, `Coordinator` is `@MainActor`, outline/table rows use cached identity nodes.
- [ ] Persisted `Codable` changes stay backward-compatible: new fields optional and emitted only when non-default; lenient decode; no gratuitous version bump.
- [ ] Pure logic lives in a host-free (no UI-framework) core module, unit-tested without an app host; the app target is a thin side-effect adapter.
- [ ] Tests are deterministic, async-native, dependency-injected; bug fixes have a reproducing test.
- [ ] Public API carries DocC docs; names read as grammatical phrases at the call site.
