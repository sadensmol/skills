# Swift Modularity and Packages

Read this when you split code into SwiftPM modules, set up `Package.swift` targets, or decide where pure logic lives.

## Modularity & Packages

- Split large apps into SwiftPM modules (feature/domain libraries) with explicit `public` boundaries — improves build times and enforces layering.
- Keep a **host-free core module** (`sadensmol-programming` → *Layers*) that imports NO `AppKit`/`UIKit`/`CoreGraphics`/`Metal`. Use plain/`Double`-backed geometry and convert to platform types at the app boundary. It then compiles under Swift 6 complete concurrency checking. Put the tricky pure logic (index math, state transitions, decision resolution) here as static functions and table-test it.
- Depend on protocols across module boundaries; inject implementations (constructor injection) rather than reaching for singletons.
- Keep `Package.swift` targets focused; put tests in matching `...Tests` targets.
