---
name: sadensmol-programming-swift
description: "Expert Swift guidance for idiomatic, safe, modern Swift on Apple platforms (iOS, macOS, watchOS, tvOS, visionOS) and server/CLI. Use when writing, reviewing, refactoring or debugging Swift; setting up Swift packages or Xcode projects; designing types, protocols, generics; optionals and error handling; async/await, actors, Sendable; SwiftUI or UIKit/AppKit interop; ARC and capture lists; Swift Testing / XCTest."
---

# Swift Programming

Act as a senior Swift developer. Keep solutions idiomatic, value-oriented, data-race-safe, and concise. Prefer the standard library and language features over custom abstractions. Target the latest Swift language mode (Swift 6, strict concurrency) unless the project pins an older toolchain. Always format and lint before declaring work complete.

**REQUIRED:** load `sadensmol-programming` with this skill — it holds the
language-agnostic rules (naming, comments, domain types, layers, errors, backward-compatible
data, tests); this skill holds the Swift mechanics and wins where they conflict.

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
- Don't return `Optional<[T]>` or `Optional<String>` to signal "empty" (`sadensmol-programming` → *Types and Domain Modelling*: absence is not emptiness).
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

## Documentation

- Document public API with DocC comments (`///`), using `- Parameters:`, `- Returns:`, `- Throws:`. Explain *why* and non-obvious contracts, not *what* the signature already says.
- Every other comment follows `sadensmol-programming` → *Comments*.

## Read on demand

**When a trigger below matches what you are about to do, read that file first** — before
you write, review or answer. The summary in this skill is not the full rule.

| When | Read |
|---|---|
| You add or change a persisted `Codable` type | `references/codable.md` |
| You write SwiftUI views or view models, or maintain or wrap UIKit/AppKit views | `references/ui.md` |
| You split code into SwiftPM modules, set up `Package.swift` targets, or decide where pure logic lives | `references/modules.md` |
| You write or change tests | `references/testing.md` |
| **Before declaring Swift work complete**, or when you review Swift code | `references/review-checklist.md` |
