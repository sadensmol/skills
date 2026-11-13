# Flutter Review Checklist

Read this before declaring Flutter work complete, and when you review Flutter code.

## Flutter Review Checklist

Before declaring Flutter work complete:

1. `flutter analyze` clean — zero warnings
2. `flutter test` green (unit + widget)
3. `flutter run --profile` on a real mid-range device; no dropped frames on the golden path
4. All widget constructors `const` where possible; `prefer_const_constructors` not suppressed
5. No `Widget _buildX()` helpers — extracted to `StatelessWidget` subclasses
6. `StatelessWidget` by default; `StatefulWidget` only where ephemeral state demands it
7. Every controller / focus node / subscription / ticker disposed in `dispose`
8. `if (!mounted) return;` (or `context.mounted`) after every `await` that touches `context` / `setState`
9. No API calls, heavy allocations, or side effects in `build()`
10. Lists use `ListView.builder` / `SliverList.builder` with `itemCount` and stable `ValueKey`s
11. Images set `cacheWidth`/`cacheHeight`; remote images use a cache package
12. `Theme.of(context)` + `ThemeExtension` for all colors/sizes — no hardcoded values
13. Navigation via `go_router` (or one chosen router), with named routes
14. `FlutterError.onError` and `PlatformDispatcher.onError` wired to the crash reporter
15. Strings extracted via `gen_l10n` — no raw user-visible text
16. Tap targets ≥ 48×48; `Semantics` labels on interactive icons; respects text scaling
17. `SafeArea` at root; adaptive layout at ≥ 600/840 breakpoints
18. Widget tests cover screen states (loading/empty/error/success); at least one integration test per critical flow
19. No `dart:io` `Platform.isXXX` in shared code — use `Theme.of(context).platform` / `kIsWeb`
20. No `print` — `logger` / `debugPrint` only
