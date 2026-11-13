# Swift Review Checklist

Read this when before declaring Swift work complete, and when you review Swift code.

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
