---
name: sadensmol-flutter-programming
description: "Expert Flutter programming guidance for building performant, idiomatic Flutter apps. Use when (1) Writing or reviewing Flutter widget code, (2) Designing app architecture (MVVM layers, state management), (3) Building or refactoring screens, widgets, navigation, or theming, (4) Debugging jank, rebuilds, or layout issues, (5) Writing widget, golden, or integration tests, (6) Implementing animations, forms, lists, images, or platform integrations, (7) Making adaptive/responsive UIs, (8) Setting up Flutter projects and CI. Covers widget composition, const discipline, lifecycle, keys, state management decision tree, performance pitfalls (opacity/clipping/saveLayer), testing strategy, and common antipatterns. For pure Dart language rules (null safety, records, sealed classes, naming), defer to dart-programming."
---

# Flutter Programming

Act as a senior Flutter engineer. Build for **60 fps on mid-range hardware**, not just your dev machine. Think in widgets, not screens. Compose small, immutable, `const` widgets. Put logic in ViewModels/Notifiers, not in `build()`.

**Scope**: this skill covers Flutter-specific concerns. For Dart language rules (null safety, collections, records, sealed classes, naming, `dart format`, `dart analyze`) use the `dart-programming` skill — don't duplicate here.

## Tooling & Project Setup

- `flutter analyze` — must be clean before shipping.
- `flutter format .` (alias of `dart format`) — authoritative formatting.
- `flutter test` — unit + widget tests.
- `flutter test integration_test/` — integration tests.
- `flutter run --profile` — test performance (never benchmark in debug).
- `flutter build <apk|ipa|web> --release` — production builds.
- `flutter pub outdated` / `flutter pub upgrade --major-versions` — dep hygiene.

Minimum `analysis_options.yaml`:

```yaml
include: package:flutter_lints/flutter.yaml

analyzer:
  language:
    strict-casts: true
    strict-inference: true
    strict-raw-types: true
  errors:
    missing_required_param: error
    missing_return: error

linter:
  rules:
    - avoid_print                       # use logger
    - avoid_unnecessary_containers
    - prefer_const_constructors
    - prefer_const_constructors_in_immutables
    - prefer_const_declarations
    - prefer_const_literals_to_create_immutables
    - sized_box_for_whitespace          # SizedBox over Container for spacing
    - sort_child_properties_last        # child: trailing
    - use_build_context_synchronously   # catches async + BuildContext bugs
    - use_key_in_widget_constructors
    - use_super_parameters
    - avoid_web_libraries_in_flutter
```

## App Architecture (MVVM)

Flutter's official architecture recommendation is layered MVVM. Adopt it for anything bigger than a sample:

```
┌─────────────────────────────────────────┐
│  UI LAYER                               │
│  ┌──────────┐         ┌──────────────┐  │
│  │  View    │ ←──→   │  ViewModel   │  │
│  │ (widgets)│         │ (state+cmds) │  │
│  └──────────┘         └──────┬───────┘  │
└──────────────────────────────┼──────────┘
                               │
┌──────────────────────────────┼──────────┐
│  DATA LAYER                  ↓          │
│  ┌───────────────┐   ┌──────────────┐   │
│  │  Repository   │ → │   Service    │   │
│  │ (domain data) │   │ (API/DB/FS)  │   │
│  └───────────────┘   └──────────────┘   │
└─────────────────────────────────────────┘
```

**Responsibilities:**

- **View (widget)**: layout, styling, user events. Contains only conditional rendering, routing calls, animation wiring. **No business logic, no direct service calls.**
- **ViewModel** (`ChangeNotifier`, `StateNotifier`, `Cubit`, or `AsyncNotifier`): owns UI state, exposes commands, pulls data from repositories, transforms domain → UI.
- **Repository**: single source of truth for a domain concept. Caches, retries, merges services. Returns domain models.
- **Service**: thin wrapper over one external source (REST client, SQLite, SharedPreferences, platform channel). Stateless. Returns `Future`/`Stream` of DTOs.

**Add a Domain layer (use cases)** *only* when logic is shared across ViewModels or merges several repositories. Don't add one per method.

## Project Structure (feature-first)

```
lib/
├── main.dart
├── app.dart                          # MaterialApp, router, theme
├── core/
│   ├── env/                          # flavors, config, build-time constants
│   ├── error/                        # AppException, failure types
│   ├── network/                      # Dio/http client, interceptors
│   ├── routing/                      # GoRouter config, route names
│   ├── theme/                        # ThemeData, ThemeExtension, colors
│   └── widgets/                      # cross-feature widgets (AppButton, …)
├── features/
│   ├── auth/
│   │   ├── data/
│   │   │   ├── services/             # auth_api_service.dart
│   │   │   └── repositories/         # auth_repository.dart
│   │   ├── domain/                   # models: User, Credentials
│   │   └── presentation/
│   │       ├── view_models/          # login_view_model.dart
│   │       └── views/                # login_screen.dart, widgets/
│   └── memories/
│       └── ...
└── l10n/                             # arb files
```

Rules:
- **Feature-first** beats layer-first once the app has more than ~3 screens — you move, rename, and delete *features*, not "all repositories".
- **One public widget per file**, file name matches class in `snake_case` (`LoginScreen` → `login_screen.dart`).
- Never import `presentation/` from `data/`. Keep the dependency arrow pointing upward only.

## Widget Composition

Flutter is composition over inheritance. Your building block is the widget, and 90% of widgets should be `StatelessWidget` with `const` constructors.

```dart
// Good — small, const, explicit inputs
class PriceTag extends StatelessWidget {
  const PriceTag({
    required this.label,
    required this.amount,
    this.highlighted = false,
    super.key,
  });

  final String label;
  final Money amount;
  final bool highlighted;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Padding(
      padding: const EdgeInsets.all(8),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(label, style: theme.textTheme.bodyMedium),
          const SizedBox(width: 8),
          Text(
            amount.format(),
            style: theme.textTheme.titleMedium?.copyWith(
              color: highlighted ? theme.colorScheme.primary : null,
            ),
          ),
        ],
      ),
    );
  }
}
```

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

```dart
// Good
return const Padding(
  padding: EdgeInsets.all(8),
  child: Text('Ready'),
);

// Good — whole list of const children
Column(
  children: const [
    Text('One'),
    SizedBox(height: 8),
    Text('Two'),
  ],
)
```

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

```dart
// Bad
@override
Widget build(BuildContext context) {
  final user = ApiClient().fetchUser(id);        // ❌ I/O in build
  return Text('${user.name}');
}

// Good
@override
void initState() {
  super.initState();
  _userFuture = widget.api.fetchUser(widget.id);
}

@override
Widget build(BuildContext context) {
  return FutureBuilder<User>(
    future: _userFuture,
    builder: (context, snap) => Text(snap.data?.name ?? '…'),
  );
}
```

## Keys — When and Which

Most widgets don't need a key. Add one when the framework's element matching by **(position, runtimeType)** is wrong.

| Situation | Key type |
|---|---|
| Stateful items in a reorderable list | `ValueKey(item.id)` |
| Preserving state across parent rebuild that moves widget | `GlobalKey` (sparingly) |
| Form state access from outside | `GlobalKey<FormState>` |
| Tests that find by identity | `ValueKey('login-button')` |

```dart
// Good — ValueKey on list items that can reorder
return ListView(
  children: [
    for (final task in tasks)
      TaskTile(key: ValueKey(task.id), task: task),
  ],
);
```

**Don't** slap `UniqueKey()` on widgets to "force rebuild" — it throws away state and is almost always the wrong fix.

## State Management Decision Tree

There is no single answer. Pick by scope:

```
Need state?
├── Used only by this one widget?
│   └── setState (StatefulWidget) ✅
├── Used by a handful of widgets in the same subtree?
│   ├── Lift state up to a common ancestor + pass down ✅
│   └── OR expose via InheritedWidget / InheritedNotifier
├── Used across features / screens?
│   ├── Small app → Provider + ChangeNotifier ✅
│   ├── Complex async, testability, compile-safe DI → Riverpod ✅
│   └── Event-sourced, strict separation, teams → Bloc/Cubit ✅
└── Need a single primitive (int, bool, String)?
    └── ValueNotifier + ValueListenableBuilder ✅
```

**Don't mix three state libraries in one codebase.** Pick one and stick with it per repo. If you inherit a mixed codebase, normalize at feature boundaries — not file by file.

### ViewModel pattern (framework-agnostic)

```dart
class LoginViewModel extends ChangeNotifier {
  LoginViewModel(this._auth);
  final AuthRepository _auth;

  LoginState _state = const LoginState.idle();
  LoginState get state => _state;

  Future<void> submit({required String email, required String password}) async {
    _state = const LoginState.loading();
    notifyListeners();
    try {
      final user = await _auth.login(email: email, password: password);
      _state = LoginState.success(user);
    } on AuthException catch (e) {
      _state = LoginState.error(e.message);
    }
    notifyListeners();
  }
}

// Use sealed classes (see dart-programming) for the state
sealed class LoginState {
  const LoginState();
  const factory LoginState.idle() = _Idle;
  const factory LoginState.loading() = _Loading;
  const factory LoginState.success(User user) = _Success;
  const factory LoginState.error(String message) = _Error;
}
```

The View then uses `switch` on `state` to render — exhaustive, type-safe, no forgotten branches.

### Derived state belongs in the ViewModel

If a value shown on screen is *derived* from VM state (encode a phrase to a QR, format a total, build a filtered list), compute it in the VM — not in a helper inside the widget. The View should hold only UI-local state (a toggle, a scroll offset) and calls to the VM.

A telltale sign you've put it in the wrong place: **the View memoizes the result to survive rebuilds.**

```dart
// Bad — View derives + reads services + hand-rolls a per-input cache to
// stop FutureBuilder re-firing on every rebuild.
String? _qrPhrase;
Future<String>? _qrFuture;
Future<String> _qrFor(BuildContext context, String phrase) {
  if (_qrPhrase == phrase && _qrFuture != null) return _qrFuture!;
  final secrets = context.read<AppConfig>().backupQrSecrets;
  final accountId = context.read<AuthService>().currentUser?.id ?? '';
  _qrPhrase = phrase;
  _qrFuture = QrCodec.encode(secrets: secrets, accountId: accountId, phrase: phrase);
  return _qrFuture!;
}
```

The cache only exists because the View recomputes on every `build()` and can't tell when the input changed. But the **VM knows exactly when it changed** — it changes when the VM mutates it. Move the derivation into the VM and the memoization evaporates:

```dart
// Good — VM derives once when the source phrase changes; View just reads it.
class PhraseSetupViewModel extends ChangeNotifier {
  PhraseSetupViewModel(this._service, this._secrets, this._accountId);
  String? _qrText;
  String? get qrText => _qrText;            // null while (re)generating

  Future<void> load() async {
    _qrText = null;
    notifyListeners();
    _words = await _service.generatePhrase();
    _qrText = await QrCodec.encode(
      secrets: _secrets, accountId: _accountId, phrase: _words.join(' '),
    );
    notifyListeners();
  }
}
```

Inject the config/service values at construction (where the widget already reads them) instead of reaching into `context` from a derivation helper. The View collapses to `vm.qrText == null ? spinner : card` — no `_qrFor`, no cached future, no `context.read` for business inputs.

## Lifecycle (`StatefulWidget`)

```dart
class _FeedState extends State<Feed> {
  late final StreamSubscription<FeedEvent> _sub;

  @override
  void initState() {
    super.initState();
    // One-time init. Widget is mounted but not yet laid out.
    // Safe: constructor args via widget.xxx, context (limited — no MediaQuery).
    _sub = widget.feed.events.listen(_onEvent);
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    // Runs after initState and whenever an InheritedWidget we depend on changes.
    // Safe place to read Theme.of, MediaQuery.of, Provider.of(listen: true).
  }

  @override
  void didUpdateWidget(covariant Feed old) {
    super.didUpdateWidget(old);
    // Parent rebuilt and passed new widget fields.
    // Use this to react to changed constructor args (e.g., restart a stream).
    if (widget.feedId != old.feedId) {
      _sub.cancel();
      _sub = widget.feed.events.listen(_onEvent);
    }
  }

  @override
  void dispose() {
    _sub.cancel();
    super.dispose();   // super LAST in dispose
  }
}
```

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

## Theming

Centralize theming in `core/theme`. Never hard-code colors, radii, or text styles in widgets.

```dart
final appLightTheme = ThemeData(
  useMaterial3: true,
  colorScheme: ColorScheme.fromSeed(seedColor: Colors.indigo),
  textTheme: GoogleFonts.interTextTheme(),
  extensions: const [AppSpacing(sm: 8, md: 16, lg: 24)],
);

class AppSpacing extends ThemeExtension<AppSpacing> {
  const AppSpacing({required this.sm, required this.md, required this.lg});
  final double sm, md, lg;

  @override
  AppSpacing copyWith({double? sm, double? md, double? lg}) =>
      AppSpacing(sm: sm ?? this.sm, md: md ?? this.md, lg: lg ?? this.lg);

  @override
  AppSpacing lerp(ThemeExtension<AppSpacing>? other, double t) {
    if (other is! AppSpacing) return this;
    return AppSpacing(
      sm: lerpDouble(sm, other.sm, t)!,
      md: lerpDouble(md, other.md, t)!,
      lg: lerpDouble(lg, other.lg, t)!,
    );
  }
}

// Usage
final spacing = Theme.of(context).extension<AppSpacing>()!;
SizedBox(height: spacing.md);
```

- **Use Material 3** (`useMaterial3: true`) on all new apps.
- **`ColorScheme.fromSeed`** for cohesive palettes; override only what needs overriding.
- **Put design tokens** (spacings, radii, durations) in `ThemeExtension`, not global constants, so they vary by light/dark/brand.

## Navigation

Prefer **`go_router`** for declarative, deep-link-safe navigation. Imperative `Navigator.push` is fine for modals and one-off flows but doesn't scale to deep links, web URLs, or state restoration.

```dart
final router = GoRouter(
  initialLocation: '/home',
  redirect: (ctx, state) => authGuard(ctx, state),
  routes: [
    GoRoute(
      path: '/home',
      builder: (ctx, state) => const HomeScreen(),
      routes: [
        GoRoute(
          path: 'memory/:id',
          builder: (ctx, state) => MemoryScreen(id: state.pathParameters['id']!),
        ),
      ],
    ),
    GoRoute(path: '/login', builder: (_, __) => const LoginScreen()),
  ],
);
```

Rules:
- **Name routes** as path constants in one file; never hard-code strings at call sites.
- **`context.go('/path')`** replaces the stack; **`context.push('/path')`** pushes. Don't mix them up.
- **Guard with `redirect`** for auth/feature-flag gating, not scattered `if (!loggedIn) ...` inside screens.

## Lists — Always Lazy

`ListView(children: [...])` and `Column(children: [...])` build **every child eagerly**. For anything that can grow, use builders.

```dart
// Good
ListView.builder(
  itemCount: tasks.length,
  itemBuilder: (context, i) => TaskTile(task: tasks[i]),
)

// Good — separated lists
ListView.separated(
  itemCount: tasks.length,
  separatorBuilder: (_, __) => const Divider(height: 1),
  itemBuilder: (_, i) => TaskTile(task: tasks[i]),
)

// Good — mix fixed and lazy in one scroll view
CustomScrollView(
  slivers: [
    const SliverAppBar(title: Text('Feed')),
    SliverList.builder(
      itemCount: tasks.length,
      itemBuilder: (_, i) => TaskTile(task: tasks[i]),
    ),
  ],
)
```

Rules:
- **Always provide `itemCount`** when known — allows the framework to pre-compute scroll extents.
- **Stable `ValueKey`** on items that can reorder/delete.
- **Use `AutomaticKeepAliveClientMixin`** only for expensive items (media players, complex forms); default is the right default.
- For nested scroll views, reach for **`CustomScrollView` + slivers**, not nested `ListView`s.

## Images

- **Always constrain image size** via `cacheWidth`/`cacheHeight` to the displayed logical pixel size × device pixel ratio — otherwise Flutter decodes at source resolution and trashes memory.
- **`FadeInImage`** beats `Image` + `Opacity` for progressive loads.
- **`precacheImage`** in `didChangeDependencies` for hero/above-the-fold assets.

```dart
Image.network(
  url,
  cacheWidth: 600,              // ← crucial
  fit: BoxFit.cover,
  loadingBuilder: (ctx, child, progress) => progress == null
      ? child
      : const Center(child: CircularProgressIndicator()),
  errorBuilder: (ctx, err, stack) => const Icon(Icons.broken_image),
)
```

For remote images with caching, disk cache, and retry, use `cached_network_image`.

## Performance — The Real Gotchas

### Opacity

`Opacity` forces an offscreen render target (`saveLayer`) — expensive. Alternatives, in order:

```dart
// Best: bake alpha into the color
Container(color: Colors.black.withValues(alpha: 0.5))  // Flutter 3.27+
Container(color: Colors.black.withOpacity(0.5))        // older

// Next: AnimatedOpacity — still uses saveLayer but only during animation
AnimatedOpacity(opacity: visible ? 1 : 0, duration: 300.ms, child: ...)

// Avoid for static alpha:
Opacity(opacity: 0.5, child: bigSubtree)               // ❌
```

For fading images specifically, use `FadeInImage`.

### Clipping

`ClipRRect`/`ClipPath` also trigger `saveLayer`. Prefer `BoxDecoration(borderRadius:)` or `DecoratedBox` when possible, and only clip the smallest subtree.

### `saveLayer` sources

These widgets **may** trigger `saveLayer` — use sparingly on large subtrees:
`Opacity`, `ShaderMask`, `ColorFilter`, `BackdropFilter`, `Chip` with alpha, `Text` with overflow shader.

Turn on **“Highlight Offscreen Layers”** in DevTools performance overlay to find them.

### `AnimatedBuilder` `child:` parameter

If only part of the subtree is animated, pass the static part via `child:` so it isn't rebuilt each frame:

```dart
AnimatedBuilder(
  animation: controller,
  child: const ExpensiveStaticHeader(),      // built once
  builder: (ctx, child) => Transform.rotate(
    angle: controller.value * 2 * pi,
    child: child,                             // reused
  ),
)
```

### Don't override widget `operator==`

Flutter's diff algorithm assumes cheap `identical` checks. Custom `==` can turn rebuild into O(N²). Let the framework canonicalize `const` instances instead.

### `setState` locality

Lift `setState` to the **smallest** subtree that actually changes. If tapping a button only changes a counter badge, that badge should be its own widget with its own state — not the whole screen.

### Profile, don't guess

- `flutter run --profile` (never benchmark debug builds).
- Open **DevTools → Performance → Timeline**. Anything > 16 ms (60 Hz) or > 8 ms (120 Hz) is a frame drop.
- **Flame chart by frame**: the two bars are *UI (build + layout + paint)* and *Raster (GPU)*. Fix whichever is red.
- Enable **“Track widget builds”** and find widgets that rebuild every frame unexpectedly.

## Animations — Pick the Lowest Power Needed

```
Tween/value needs to animate?
├── Implicit (single property, one-shot) → AnimatedContainer, AnimatedOpacity,
│                                           AnimatedAlign, AnimatedPositioned…
├── Explicit (reusable, looping, sequenced) → AnimationController + AnimatedBuilder
├── Staggered / timeline → AnimationController + Interval curves
├── Hero transitions between routes → Hero widget
└── Physics-based (spring, fling, scrollable) → SpringSimulation + AnimationController
```

Rules:
- **Always `vsync: this`** on `AnimationController`, and add `SingleTickerProviderStateMixin` (or `TickerProviderStateMixin` for multiple).
- **Dispose the controller.**
- **Use `Curves.*`** from the built-in set before writing custom curves.
- **`RepaintBoundary`** wraps subtrees that repaint independently from their parent — useful around animated widgets inside a static parent.

## Forms

```dart
final _formKey = GlobalKey<FormState>();
final _emailCtrl = TextEditingController();
final _pwdFocus = FocusNode();

@override
void dispose() {
  _emailCtrl.dispose();
  _pwdFocus.dispose();
  super.dispose();
}

// In build:
Form(
  key: _formKey,
  child: Column(
    children: [
      TextFormField(
        controller: _emailCtrl,
        keyboardType: TextInputType.emailAddress,
        textInputAction: TextInputAction.next,
        autofillHints: const [AutofillHints.email],
        validator: (v) => (v == null || !v.contains('@')) ? 'Invalid email' : null,
        onFieldSubmitted: (_) => _pwdFocus.requestFocus(),
      ),
      TextFormField(
        focusNode: _pwdFocus,
        obscureText: true,
        autofillHints: const [AutofillHints.password],
        validator: (v) => (v == null || v.length < 8) ? 'Min 8 chars' : null,
      ),
      FilledButton(
        onPressed: () {
          if (_formKey.currentState!.validate()) _submit();
        },
        child: const Text('Sign in'),
      ),
    ],
  ),
)
```

Rules:
- **Always dispose controllers and focus nodes.**
- **Set `keyboardType`, `textInputAction`, `autofillHints`** — free accessibility + UX wins.
- **Validators return a message or `null`**, never throw.
- **For big forms**, use a form library (`reactive_forms`, `flutter_form_builder`) — but only once you actually have >5 fields with cross-field rules.

## Error Handling & Crash Reporting

```dart
void main() {
  FlutterError.onError = (details) {
    FlutterError.presentError(details);
    logger.e('FlutterError', details.exception, details.stack);
    CrashReporter.record(details.exception, details.stack);
  };

  PlatformDispatcher.instance.onError = (error, stack) {
    logger.e('PlatformDispatcher', error, stack);
    CrashReporter.record(error, stack);
    return true;
  };

  ErrorWidget.builder = (details) => const _FriendlyError();

  runApp(const MyApp());
}
```

- **`FlutterError.onError`** catches errors in the framework callback stack (build, layout, paint).
- **`PlatformDispatcher.instance.onError`** catches uncaught async errors in the root zone.
- **`ErrorWidget.builder`** replaces the red error screen in release.
- **Never swallow errors silently** — log + report, then fall back to a visible error state.

## Accessibility

Flutter's accessibility is good *if you help it*:

- **Every interactive thing** needs a `Semantics` label or an underlying widget that provides one (`IconButton(tooltip:)`, `TextButton(child: Text(...))`). Bare `GestureDetector` on an `Icon` is invisible to screen readers.
- **Minimum tap target 48×48 logical pixels** (Material guideline). Wrap small icons with `IconButton` or pad them.
- **Color contrast ≥ 4.5:1** for body text, 3:1 for large text. Test with `Theme.of(context).colorScheme` combinations.
- **Respect text scaling**: `MediaQuery.textScalerOf(context).scale(16)` — never hard-cap.
- **`excludeSemantics: true`** on decorative images.
- **Focus traversal**: use `FocusTraversalGroup` + `OrderedTraversalPolicy` for keyboard-heavy screens.

Test with the **Accessibility Scanner** on Android and **Accessibility Inspector** on iOS/macOS.

## Localization / i18n

Use the built-in `flutter_localizations` + `gen_l10n` pipeline. Add to `pubspec.yaml`:

```yaml
flutter:
  generate: true

# l10n.yaml
arb-dir: lib/l10n
template-arb-file: app_en.arb
output-localization-file: app_localizations.dart
```

Then `AppLocalizations.of(context)!.welcome` is type-safe, autocompleted, and extracted at build time. **Don't** roll your own `Map<String,String>` translation system — you lose pluralization, gender, and ICU format.

## Responsive & Adaptive

- **`LayoutBuilder`** when you need *parent* constraints (e.g. switch from list to grid above 600 px).
- **`MediaQuery.sizeOf(context)`** when you need *window* size (dialogs, full-screen routing).
- **Breakpoints** (Material 3 guidelines): compact < 600, medium 600–839, expanded 840–1199, large 1200–1599, extra-large 1600+.
- **`NavigationBar` on phone, `NavigationRail` on tablet, `NavigationDrawer` on desktop** — the Material 3 adaptive scaffold pattern.
- **`Theme.of(context).platform`** to branch on TargetPlatform (overridable in tests) — not `dart:io`'s `Platform.isIOS` which crashes on web.
- **`SafeArea`** around root content; never assume status/nav bar insets.

```dart
@override
Widget build(BuildContext context) {
  final width = MediaQuery.sizeOf(context).width;
  if (width >= 840) return const _ExpandedLayout();
  if (width >= 600) return const _MediumLayout();
  return const _CompactLayout();
}
```

## Testing

Pyramid:

```
       /\
      /  \   Few integration tests (happy-path E2E)
     /----\
    /      \  Some widget tests (screen-level)
   /--------\
  /          \ Many unit tests (VMs, repositories, utils)
 /____________\
```

### Unit tests — ViewModels, repositories, pure logic

```dart
void main() {
  group('LoginViewModel', () {
    late FakeAuthRepo repo;
    late LoginViewModel vm;

    setUp(() {
      repo = FakeAuthRepo();
      vm = LoginViewModel(repo);
    });

    test('emits loading then success on valid credentials', () async {
      repo.nextUser = const User(id: '1', name: 'Ada');

      final states = <LoginState>[];
      vm.addListener(() => states.add(vm.state));

      await vm.submit(email: 'a@b', password: 'pw12345678');

      expect(states.first, isA<_Loading>());
      expect(states.last, isA<_Success>());
    });
  });
}
```

### Widget tests — screens and complex widgets

```dart
void main() {
  testWidgets('shows error when login fails', (tester) async {
    final repo = FakeAuthRepo()..nextError = const AuthException('bad creds');
    await tester.pumpWidget(
      MaterialApp(
        home: ChangeNotifierProvider(
          create: (_) => LoginViewModel(repo),
          child: const LoginScreen(),
        ),
      ),
    );

    await tester.enterText(find.byKey(const ValueKey('email')), 'a@b');
    await tester.enterText(find.byKey(const ValueKey('password')), 'pw12345678');
    await tester.tap(find.byKey(const ValueKey('submit')));
    await tester.pump();                 // loading frame
    await tester.pump(const Duration(seconds: 1));

    expect(find.text('bad creds'), findsOneWidget);
  });
}
```

Rules:
- **`pumpWidget`** → **`pump`** after every async → **`pumpAndSettle`** *only* for finite animations. Don't use `pumpAndSettle` on indeterminate progress spinners — it hangs.
- **Use `ValueKey` on things you test** so tests don't depend on visual text.
- **Wrap with `MaterialApp`** (or `WidgetsApp`) when the widget reads `Theme`/`Directionality`/`MediaQuery`.
- **Fake at the repository boundary**, not deep inside services — tests stay realistic.

### Golden tests

Catch visual regressions. Run with `flutter test --update-goldens` to refresh when intentional.

```dart
testWidgets('PriceTag matches golden', (tester) async {
  await tester.pumpWidget(const MaterialApp(
    home: Scaffold(body: Center(child: PriceTag(label: 'Total', amount: Money(42)))),
  ));
  await expectLater(find.byType(PriceTag), matchesGoldenFile('price_tag.png'));
});
```

CI: run goldens on a **single fixed host** (Linux or a specific CI image) — font rendering differs by OS and will false-positive otherwise.

### Integration tests

One per critical user journey (login → main screen → key action). Run on CI with `flutter test integration_test/`. Keep them few; they're slow and flaky compared to widget tests.

## Dependency Injection

Keep DI boring:

- **Small app**: pass dependencies down through constructors + one Provider scope at the root.
- **Medium app**: `Provider` / `MultiProvider` per feature.
- **Large app / strict testing**: `Riverpod` (compile-time provider graph) or `get_it` + `injectable`.

Rules:
- **Never `GetIt.instance.get<X>()` inside `build()`** — inject via constructor or a Provider scope so widgets are testable with overrides.
- **Repositories and services are singletons** (created once at app start). ViewModels are **created per screen** (dispose on route pop).
- **Long-lived background work (Timers, periodic polls, stream subscriptions, pending Futures) belongs in a service, not a ViewModel.** `go_router` and any `Navigator` that rebuilds routes can re-invoke a `GoRoute.builder` when the user pops back from a pushed child — producing a **new** VM instance. If the Timer lives on the old VM, it fires on an orphaned object; `notifyListeners()` reaches no one because the tree now listens to the new VM. Put the Timer in a `ChangeNotifier` service in DI; ephemeral VMs subscribe in their constructor, unsubscribe in `dispose`.

### Pattern: polling service + ephemeral VM

```dart
// lib/core/services/recognition_polling_service.dart — lifetime-scoped
class RecognitionPollingService extends ChangeNotifier {
  final MyService _backend;
  final Set<String> _pending = {};
  Timer? _timer;

  RecognitionPollingService(this._backend);

  void observe(String id) {
    _pending.add(id);
    _timer ??= Timer(const Duration(seconds: 20), _tick);
  }

  Future<void> _tick() async {
    _timer = null;
    for (final id in _pending.toList()) {
      final result = await _backend.getStatus(id);
      if (result.done) _pending.remove(id);
    }
    notifyListeners();                             // ← consumers reload
    if (_pending.isNotEmpty) {
      _timer = Timer(const Duration(seconds: 10), _tick);
    }
  }

  @override
  void dispose() { _timer?.cancel(); super.dispose(); }
}

// lib/ui/screens/memories/memories_view_model.dart — ephemeral
class MemoriesViewModel extends ChangeNotifier {
  final RecognitionPollingService _polling;
  MemoriesViewModel(this._polling) {
    _polling.addListener(_onUpdate);
  }
  void scheduleRefresh(String id) => _polling.observe(id);
  void _onUpdate() => loadMemories();              // same path as pull-to-refresh
  @override
  void dispose() {
    _polling.removeListener(_onUpdate);
    super.dispose();
  }
}
```

Why this is better than hoisting the VM:

- **Separation of concerns** — VMs describe view state; services own async work.
- **Multiple consumers for free** — list VM and detail VM can both subscribe to the same poll stream without duplicating Timers.
- **Testability** — service is a pure unit testable with a fake clock.
- **VMs stay ephemeral** — matches the DI convention for other screens; no special-casing in the Provider tree.
- **Tab / account switches clean up naturally** — lifetime services can observe auth/connection changes in one place.

### Debugging route-rebuild VM orphaning

Symptom: backend state is correct (verified by pull-to-refresh), but the screen doesn't auto-update even though your VM's `notifyListeners()` clearly fires. Often: "refresh works but polling doesn't".

Proof pattern — add hashCode-tagged `debugPrint`s on the VM's constructor, dispose, and the async handler:

```dart
MyViewModel(...) {
  debugPrint('[vm] CREATED id=$hashCode');
}

void _tick() {
  debugPrint('[vm] tick vm=$hashCode');
}

@override
void dispose() {
  debugPrint('[vm] DISPOSED id=$hashCode');
  super.dispose();
}
```

If the log shows **multiple `CREATED` lines** from a single screen navigation, or a `tick vm=<old-id>` firing *after* a later `CREATED` line, the VM is being recreated under the running work. Move the work to a service (see pattern above).

## Platform Channels — Principles

- Wrap every channel in a typed Dart class with a `Future`-returning API. Never expose raw `MethodChannel` to the rest of the app.
- **Version your method names** (`"v1/getBattery"`) — platform code ships on a different cadence from Dart.
- **Handle `MissingPluginException`** — happens on hot restart and on platforms that don't implement the channel.
- Prefer **Pigeon** for anything non-trivial: generates typed host/Dart code from one IDL, eliminates stringly-typed bugs.

## Common Antipatterns

| Antipattern | Why it's bad | Fix |
|---|---|---|
| `Widget _buildFoo()` helpers | No `const`, no DevTools name, rebuilds with parent | Extract `StatelessWidget` |
| `GlobalKey` everywhere | Defeats widget tree invariants, leaks, perf hit | Use `ValueKey` / lift state up |
| Storing `BuildContext` in fields | Stale after rebuild/dispose → crash | Pass `context` into method calls |
| API call in `build()` | Hammers network, causes rebuild loops | Move to `initState` / VM |
| Rebuilding whole screen on keystroke | Jank on low-end devices | Split into smaller widgets + local state |
| `FutureBuilder` with inline future | New future each build → refetch each rebuild | Compute in the VM (it knows when the input changed); View reads the result |
| View memoizes a derived value to survive rebuilds | Derivation + service reads leaked into the View; cache only compensates for that | Move the derivation to the VM — the cache then disappears |
| `dart:io Platform.isXXX` | Crashes on web | `Theme.of(context).platform` or `kIsWeb` |
| `setState` after `await` w/o `mounted` | `setState() called after dispose` crash | Guard with `if (!mounted) return;` |
| `print` in production | Strips PII to logs, slow | `logger` package or `debugPrint` |
| Hardcoded colors / sizes in widgets | Inconsistent, no dark-mode | `Theme.of(context)` + `ThemeExtension` |
| `Opacity` for static alpha | Forces `saveLayer` | Bake alpha into color |
| Eager `ListView(children: [...])` | Builds every child | `ListView.builder` |
| Custom `operator==` on widgets | Breaks framework diff optimization | Delete it; rely on `const` |
| VM owns a Timer / poll / subscription | Pop-back can rebuild the route → new VM; Timer fires on the orphaned old VM, UI listens to the new one, `notifyListeners` reaches no one | Move the work to a lifetime-scoped `ChangeNotifier` service; VM subscribes on construct, unsubscribes on dispose |

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
