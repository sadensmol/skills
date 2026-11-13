# Flutter Architecture, State and DI

Read this when you design app layers or the folder layout, choose or change state management, write a ViewModel, or add long-lived background work (timers, polls, subscriptions).

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

// Use sealed classes (see sadensmol-programming-dart) for the state
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

## Dependency Injection

Keep DI boring:

- **Small app**: pass dependencies down through constructors + one Provider scope at the root.
- **Medium app**: `Provider` / `MultiProvider` per feature.
- **Large app / strict testing**: `Riverpod` (compile-time provider graph) or `get_it` + `injectable`.

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
