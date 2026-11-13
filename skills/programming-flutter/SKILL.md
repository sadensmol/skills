---
name: sadensmol-programming-flutter
description: "Expert Flutter guidance for performant, idiomatic Flutter apps. Use when (1) writing or reviewing Flutter widget code, (2) designing app architecture (MVVM, state management, DI), (3) building screens, navigation, theming, forms, lists, images or platform channels, (4) debugging jank, rebuilds or layout issues, (5) writing widget, golden or integration tests, (6) setting up Flutter projects. For pure Dart language rules, defer to sadensmol-programming-dart."
---

# Flutter Programming

Act as a senior Flutter engineer. Build for **60 fps on mid-range hardware**, not just your dev machine. Think in widgets, not screens. Compose small, immutable, `const` widgets. Put logic in ViewModels/Notifiers, not in `build()`.

**Scope**: this skill covers Flutter-specific concerns. For Dart language rules (null safety, collections, records, sealed classes, naming, `dart format`, `dart analyze`) use the `sadensmol-programming-dart` skill — don't duplicate here. Language-agnostic rules (naming, comments, domain types, layers, errors, test principles) live in `sadensmol-programming` — load it too.

## Tooling & Project Setup

- `flutter analyze` — must be clean before shipping.
- `dart format .` — authoritative formatting.
- `flutter test` — unit + widget tests.
- `flutter test integration_test/` — integration tests.
- `flutter run --profile` — test performance (never benchmark in debug).
- `flutter build <apk|ipa|web> --release` — production builds.
- `flutter pub outdated` / `flutter pub upgrade --major-versions` — dep hygiene.

`analysis_options.yaml`: the Flutter lint block is in `references/setup.md`.

## App Architecture (MVVM)

Flutter's official architecture recommendation is layered MVVM. Adopt it for anything bigger than a sample (layer diagram: `references/architecture.md`).

**Responsibilities:**

- **View (widget)**: layout, styling, user events. Contains only conditional rendering, routing calls, animation wiring. **No business logic, no direct service calls.**
- **ViewModel** (`ChangeNotifier`, `StateNotifier`, `Cubit`, or `AsyncNotifier`): owns UI state, exposes commands, pulls data from repositories, transforms domain → UI.
- **Repository**: single source of truth for a domain concept. Caches, retries, merges services. Returns domain models.
- **Service**: thin wrapper over one external source (REST client, SQLite, SharedPreferences, platform channel). Stateless. Returns `Future`/`Stream` of DTOs.

**Add a Domain layer (use cases)** *only* when logic is shared across ViewModels or merges several repositories. Don't add one per method.

## Project Structure (feature-first)

Folder tree: `references/architecture.md`.

Rules:
- **Feature-first** beats layer-first once the app has more than ~3 screens — you move, rename, and delete *features*, not "all repositories".
- **One public widget per file**, file name matches class in `snake_case` (`LoginScreen` → `login_screen.dart`).
- Never import `presentation/` from `data/`. Keep the dependency arrow pointing upward only.

## Widget Composition

Flutter is composition over inheritance. Your building block is the widget, and 90% of widgets should be `StatelessWidget` with `const` constructors.

Example: `references/widget-examples.md`.

Rules:
- **Widget fields are always `final`.** (Enforced by the framework.)
- **Constructor is always `const`** unless a field prevents it. Pair with `prefer_const_constructors_in_immutables` lint.
- **Accept `super.key`** on every public widget — it's the idiomatic way to forward keys for list diffing.
- **Keep widgets small.** If `build()` is taller than ~50 lines, extract a subwidget.

## `StatelessWidget` vs `StatefulWidget`

Default to `StatelessWidget`. Reach for `StatefulWidget` only when the widget owns **ephemeral** state that no one else needs (local animation progress, expansion toggle, text field focus).

| You need… | Use |
|---|---|
| Render props / theme lookup | `StatelessWidget` |
| Local, throwaway state (`_expanded`, `_hovered`) | `StatefulWidget` |
| App-wide data (auth, cart, settings) | ViewModel + DI (Provider/Riverpod/Bloc) |
| Async lifecycle (timers, subscriptions, controllers) | `StatefulWidget` or disposable VM |

**Never use functions that return widgets** (`Widget _buildHeader()`) — they defeat `const` caching, don't show up in DevTools, and rebuild on every parent rebuild. Extract a `StatelessWidget` subclass instead.

```dart
// Bad
Widget _buildHeader() => const Text('Hello');

// Good
class _Header extends StatelessWidget {
  const _Header();
  @override
  Widget build(BuildContext context) => const Text('Hello');
}
```

## `const` Discipline

`const` widgets are canonicalized: Flutter skips rebuilding subtrees whose `const` instance is identical. This is the single biggest free performance win.

Examples: `references/widget-examples.md`.

Rules:
- Turn on `prefer_const_constructors` + `prefer_const_literals_to_create_immutables`.
- Extract widgets that depend on runtime values into a subwidget so their **siblings** can stay `const`.
- Never pass a non-`const` lambda or object to a `const` constructor — you'll silently lose the `const`.

## `build()` Method Discipline

`build()` may run 60+ times per second. Treat it like a pure function of `(widget fields + State fields + InheritedWidgets)`.

- **No side effects** in `build()`: no API calls, no `setState`, no navigation, no `SharedPreferences.getInstance()`.
- **No allocation of heavy objects**: no `Dio()`, no `HttpClient()`, no regex compilation. Move to `initState` or inject via constructor.
- **Read `Theme.of` / `MediaQuery.of` once** at the top and reuse — each call re-looks-up the inherited widget.
- **Short-circuit expensive children** with `const` or extracted widgets.
- **Keep `setState` local:** lift it to the smallest subtree that changes — a counter badge gets its own widget, not a whole-screen rebuild per keystroke.

Bad/Good example: `references/widget-examples.md`.

## Keys — When and Which

Most widgets don't need a key. Add one when the framework's element matching by **(position, runtimeType)** is wrong.

| Situation | Key type |
|---|---|
| Stateful items in a reorderable list | `ValueKey(item.id)` |
| Preserving state across parent rebuild that moves widget | `GlobalKey` (sparingly) |
| Form state access from outside | `GlobalKey<FormState>` |
| Tests that find by identity | `ValueKey('login-button')` |

**Don't** slap `UniqueKey()` on widgets to "force rebuild" — it throws away state and is almost always the wrong fix.

## State Management

Pick by scope — the decision tree and the ViewModel pattern are in `references/architecture.md`.

**Don't mix three state libraries in one codebase.** Pick one and stick with it per repo. If you inherit a mixed codebase, normalize at feature boundaries — not file by file.

**Derived state belongs in the ViewModel.** If a value shown on screen is *derived* from VM state (encode a phrase to a QR, format a total, build a filtered list), compute it in the VM — not in a helper inside the widget. The View should hold only UI-local state (a toggle, a scroll offset) and calls to the VM.

## Lifecycle (`StatefulWidget`)

Full lifecycle example: `references/widget-examples.md`.

Rules:
- **Always cancel/close** in `dispose`: `AnimationController`, `TextEditingController`, `FocusNode`, `ScrollController`, `StreamSubscription`, `Timer`.
- **`super.initState()` first**, **`super.dispose()` last**.
- **Don't call `setState` in `dispose` or before `mount`.**

## `BuildContext` and `async`

After an `await`, the widget may have been disposed. **Always check `mounted`** before touching `context` or calling `setState`.

```dart
Future<void> _load() async {
  final data = await api.fetch();
  if (!mounted) return;                         // 1) State.mounted
  setState(() => _data = data);
}

Future<void> _confirm(BuildContext context) async {
  final ok = await showDialog<bool>(...);
  if (!context.mounted) return;                 // 2) BuildContext.mounted
  Navigator.of(context).pop(ok);
}
```

The lint `use_build_context_synchronously` catches most cases — keep it on.

Other `BuildContext` rules:
- **Don't store `context`** in fields. It becomes stale.
- **Read `Theme.of`/`MediaQuery.of` in `build`, not `initState`** — `initState` has no ancestor-lookup guarantee.
- Prefer `MediaQuery.sizeOf(context)` / `Theme.of(context)` over capturing the whole `MediaQueryData`/`ThemeData` if you only need one slice (Flutter 3.10+ added granular `.sizeOf`/`.textScalerOf`/etc. that rebuild less).

## App-Wide Conventions

- **Theming:** centralize in `core/theme`; never hard-code colors, radii, or text styles in widgets. Material 3 + `ColorScheme.fromSeed`; design tokens (spacings, radii, durations) in a `ThemeExtension`, not global constants.
- **Navigation:** `go_router`; route paths as constants in one file, never strings at call sites; gate auth/feature flags with `redirect`, not `if (!loggedIn)` inside screens. `context.go` replaces the stack, `context.push` pushes.
- **Lists:** anything that can grow uses a lazy builder (`ListView.builder`, `SliverList.builder`) with `itemCount` and a stable `ValueKey` on items; nested scrolling uses `CustomScrollView` + slivers.
- **Images:** always set `cacheWidth`/`cacheHeight` to the displayed size; remote images use `cached_network_image`.
- **Forms:** dispose every controller and focus node; set `keyboardType`, `textInputAction`, `autofillHints`; validators return a message or `null`, never throw; add a form library only past ~5 fields with cross-field rules.
- **Errors:** wire `FlutterError.onError`, `PlatformDispatcher.instance.onError`, and a release `ErrorWidget.builder` to the logger and crash reporter; never swallow an error — log, report, show a visible error state.
- **Accessibility:** tap targets ≥ 48×48; `Semantics`/`tooltip` on interactive icons (a bare `GestureDetector` on an `Icon` is invisible to screen readers); never cap text scaling.
- **i18n:** `flutter_localizations` + `gen_l10n`; no hand-rolled translation maps, no raw user-visible strings.
- **Adaptive:** breakpoints at 600 / 840; branch on `Theme.of(context).platform` or `kIsWeb`, never `dart:io` `Platform.isXXX` (crashes on web); `SafeArea` at the root.
- **Logging:** no `print` — `logger` or `debugPrint`.

## Dependency Injection

Choose the DI tool by app size: `references/architecture.md`.

Rules:
- **Never `GetIt.instance.get<X>()` inside `build()`** — inject via constructor or a Provider scope so widgets are testable with overrides.
- **Repositories and services are singletons** (created once at app start). ViewModels are **created per screen** (dispose on route pop).
- **Long-lived background work (Timers, periodic polls, stream subscriptions, pending Futures) belongs in a service, not a ViewModel.** Put the Timer in a `ChangeNotifier` service in DI; ephemeral VMs subscribe in their constructor, unsubscribe in `dispose`. Why (route rebuilds orphan the VM): `references/architecture.md`.

Pattern (polling service + ephemeral VM) and how to debug VM orphaning: `references/architecture.md`.

## Read on demand

**When a trigger below matches what you are about to do, read that file first** — before
you write, review or answer. The summary in this skill is not the full rule.

| When | Read |
|---|---|
| You create a project or change `analysis_options.yaml` | `references/setup.md` |
| You design layers or folders, choose state management, write a ViewModel, or add long-lived background work | `references/architecture.md` |
| You need a worked widget, `const`, `build()` or lifecycle example | `references/widget-examples.md` |
| You write or change unit, widget, golden or integration tests | `references/testing.md` |
| You add or change a platform channel | `references/platform-channels.md` |
| **Before declaring Flutter work complete**, or when you review Flutter code | `references/review-checklist.md` |
