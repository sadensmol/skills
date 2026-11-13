# Swift UI: SwiftUI and UIKit/AppKit

Read this when you write SwiftUI views or view models, or maintain or wrap UIKit/AppKit views.

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
