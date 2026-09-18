---
name: sadensmol-dart-programming
description: "Expert Dart programming guidance following Effective Dart best practices. Use when (1) Writing or reviewing Dart or Flutter code, (2) Setting up Dart projects, (3) Implementing null safety, async, collections, classes, records, or pattern matching, (4) Writing unit or widget tests, (5) Refactoring Dart code, (6) Debugging Dart applications, (7) Designing APIs or data classes. Enforces idiomatic style, null safety, immutability, and modern Dart 3 features."
---

# Dart Programming

Act as a senior Dart/Flutter developer. Keep solutions idiomatic, null-safe, and concise. Prefer built-in language features over custom abstractions. Always run `dart format` and `dart analyze` before declaring work complete.

## Tooling (non-negotiable)

- `dart format .` — single source of truth for formatting. Never hand-format.
- `dart analyze` — must pass with zero warnings.
- `dart fix --apply` — apply mechanical fixes.
- Enable strict lints in `analysis_options.yaml`:

```yaml
include: package:lints/recommended.yaml   # or package:flutter_lints/flutter.yaml

analyzer:
  language:
    strict-casts: true
    strict-inference: true
    strict-raw-types: true

linter:
  rules:
    - always_declare_return_types
    - avoid_dynamic_calls
    - prefer_const_constructors
    - prefer_const_declarations
    - prefer_final_locals
    - unawaited_futures
    - use_super_parameters
```

## Imports & File Layout

Order imports into three blocks, each alphabetized, separated by a blank line:

```dart
// 1. dart: imports
import 'dart:async';
import 'dart:convert';

// 2. package: imports (third-party + your own package)
import 'package:flutter/material.dart';
import 'package:my_app/models/user.dart';

// 3. relative imports
import '../utils/logger.dart';
import 'widgets/header.dart';

// exports go after imports, in their own section
export 'src/error.dart';
```

Rules:
- **Prefer `package:` imports** for files inside your own package — not relative — once you cross `lib/`.
- **Never write `library my_name;`** — use bare `library;` only to attach library-level doc comments.
- **Don't use `part`/`part of`** except for generated code (e.g., `json_serializable`, `freezed`).
- One public class per file is the norm; the file name matches the class in `snake_case` (`UserRepository` → `user_repository.dart`).

## Naming

| Kind | Convention | Example |
|---|---|---|
| Types, extensions, enums, typedefs | `UpperCamelCase` | `HttpClient`, `UserRole` |
| Libraries, packages, files, dirs | `lowercase_with_underscores` | `user_repository.dart` |
| Import prefixes | `lowercase_with_underscores` | `import 'dart:math' as math;` |
| Variables, functions, parameters, members | `lowerCamelCase` | `itemCount`, `fetchUser()` |
| Constants | `lowerCamelCase` (not `SCREAMING_CAPS`) | `const defaultTimeout = ...` |
| Private members | leading underscore | `_cache`, `_buildBody()` |

### Acronyms

Capitalize acronyms longer than 2 letters as words: `HttpClient`, `UriParser`, `JsonEncoder` — not `HTTPClient`. Two-letter acronyms stay uppercase: `IOError`, `TVShow`, `IDProvider`.

### Method / property naming

- **Noun phrases** for non-boolean properties: `list.length`, `button.padding` — describe *what it is*.
- **Non-imperative verb phrases** for booleans: `isEmpty`, `hasElements`, `canClose`, `isVisible`. Never `getEmpty()` or `empty` alone.
- **Imperative verbs** for methods with side effects: `list.add()`, `stream.close()`, `window.refresh()`.
- **Noun phrases** for value-returning methods with no side effects: `list.elementAt(i)`, not `list.getElementAt(i)`.
- **Don't start methods with `get`** — use a getter or a precise verb (`fetch`, `load`, `compute`).
- **`toX()`** for a new copy / conversion; **`asX()`** for a view that shares storage.

```dart
// Good
final bytes = string.codeUnits;          // property, noun
final copy = list.toList();              // new copy
final view = list.asMap();               // view
if (user.isAdmin) grant();               // boolean, positive

// Bad
final bytes = string.getCodeUnits();
final view = list.toMap();               // misleading — actually a view
if (!user.isNotAdmin) grant();           // double negative
```

- **Prefer positive boolean names**: `isEnabled` over `isDisabled` so call sites avoid `!`.
- **Don't repeat parameter types in function names**: `list.add(value)`, not `list.addValue(value)`.

## Null Safety

Dart is sound null-safe. Make types non-nullable by default; opt into `?` only where `null` is a real value.

```dart
// Bad — nullable without reason
String? _name;
void greet() => print('Hi ${_name!}');      // ! hides bugs

// Good — require it
class UserCard {
  final String name;
  const UserCard({required this.name});
}

// Good — handle null explicitly
String greeting(String? name) => 'Hi ${name ?? 'stranger'}';
```

Rules:
- **Never use `!` (bang)** unless you can prove non-null from a check on the line above. Prefer `??`, `?.`, `if (x != null)`, or `late`.
- Use `late` only when the field is genuinely initialized-before-read (e.g., `initState`). Abuse of `late` defeats null safety.
- Prefer `late final` for lazy initialization.

```dart
late final _expensive = _compute();   // computed on first access
```

## Immutability & `const`

Prefer immutable data. Use `final` for locals/fields, `const` for compile-time constants. `const` constructors let the VM canonicalize objects and let Flutter skip rebuilds.

```dart
// Good
class Point {
  final double x;
  final double y;
  const Point(this.x, this.y);
}

const origin = Point(0, 0);
const points = [Point(1, 2), Point(3, 4)];
```

## Formatting & Control Flow

- `dart format` is authoritative — never hand-format. If a line reads badly after formatting, rewrite the expression (shorter names, extract a local), don't fight the formatter.
- Target **80 columns** unless a URL or literal forces longer.
- **Use curly braces** for all control-flow bodies. The one accepted exception is a single-line `if` with no `else`:

```dart
// OK
if (overflow) return;

// Good — everything else gets braces
if (overflow) {
  log.warning('overflow');
  return;
}

for (final item in items) {
  process(item);
}
```

- **Use `for`/`for-in` loops** instead of `.forEach((x) => ...)` — they read better, allow `await`, `break`, and `return`.
- **Don't use `new`** or redundant `const` inside const contexts:

```dart
// Bad
const pts = const [const Point(0, 0), const Point(1, 1)];

// Good
const pts = [Point(0, 0), Point(1, 1)];
```

## Classes & Constructors

Use initializing formals and super parameters. Skip getters for trivial fields — make the field `final` and public.

```dart
// Bad
class User {
  String _name;
  int _age;
  User(String name, int age) : _name = name, _age = age;
  String get name => _name;
  int get age => _age;
}

// Good
class User {
  final String name;
  final int age;
  const User({required this.name, required this.age});
}

// Good — super parameters (Dart 2.17+)
class AdminUser extends User {
  const AdminUser({
    required super.name,
    required super.age,
    required this.perms,
  });
  final Set<String> perms;
}
```

Named constructors express intent better than overloaded positional forms:

```dart
class Duration {
  const Duration.seconds(int s) : _ms = s * 1000;
  const Duration.minutes(int m) : _ms = m * 60 * 1000;
  final int _ms;
}
```

## Records (Dart 3)

Use records for lightweight, anonymous tuples — especially multiple return values — instead of custom classes or `Map`.

```dart
(int, int) divmod(int a, int b) => (a ~/ b, a % b);

// Named fields
({String host, int port}) parseAddr(String s) {
  final parts = s.split(':');
  return (host: parts[0], port: int.parse(parts[1]));
}

final addr = parseAddr('localhost:8080');
print('${addr.host}:${addr.port}');
```

Promote a record to a class once it gains behavior or is used across many layers.

## Pattern Matching & Sealed Classes (Dart 3)

Use `sealed` + `switch` expressions for exhaustive, type-safe state handling. This replaces fragile if/else chains.

```dart
sealed class Result<T> {}
class Ok<T> extends Result<T> {
  Ok(this.value);
  final T value;
}
class Err<T> extends Result<T> {
  Err(this.error);
  final Object error;
}

String describe(Result<int> r) => switch (r) {
  Ok(:final value) => 'ok: $value',
  Err(:final error) => 'error: $error',
};
```

Destructure in `if-case`:

```dart
if (json case {'name': String name, 'age': int age}) {
  return User(name: name, age: age);
}
```

## Collections

Prefer collection literals, spread, and `if`/`for` inside literals over imperative building.

```dart
// Bad
final items = <Widget>[];
items.add(const Header());
for (final t in tasks) {
  items.add(TaskTile(t));
}
if (showFooter) items.add(const Footer());

// Good
final items = <Widget>[
  const Header(),
  for (final t in tasks) TaskTile(t),
  if (showFooter) const Footer(),
];
```

Other rules:
- Use `.isEmpty` / `.isNotEmpty`, never `length == 0`.
- Use `whereType<T>()` to filter by type; avoid `where((e) => e is T).cast<T>()`.
- Prefer `const []` / `const {}` for empty defaults to avoid allocation.
- Prefer `Iterable` in return types when you don't need random access.

## Async

Prefer `async`/`await` over raw `.then()`. Never ignore a `Future` — `await` it or explicitly mark `unawaited(...)`.

```dart
import 'dart:async';

// Good
Future<User> loadUser(String id) async {
  final json = await api.get('/users/$id');
  return User.fromJson(json);
}

// Good — fire-and-forget must be explicit
unawaited(analytics.track('screen_view'));
```

Run independent futures in parallel:

```dart
final (user, prefs) = await (loadUser(id), loadPrefs(id)).wait;
```

Use `Stream` + `await for` for sequences. Always cancel subscriptions.

## Error Handling

- Throw `Error` subtypes for programmer bugs (`ArgumentError`, `StateError`) — they shouldn't be caught.
- Throw `Exception` subtypes for runtime failures callers may handle.
- **Never catch `Exception` or `Object` broadly** unless you rethrow or log-and-continue at a true boundary (UI root, request handler).

```dart
// Good
Future<Result<User>> loadUser(String id) async {
  try {
    return Ok(await api.fetchUser(id));
  } on NotFoundException catch (e) {
    return Err(e);
  } on TimeoutException catch (e) {
    return Err(e);
  }
}
```

Validate at boundaries with specific errors:

```dart
void setAge(int age) {
  if (age < 0) throw ArgumentError.value(age, 'age', 'must be >= 0');
  _age = age;
}
```

- **Use `rethrow`** to preserve the original stack trace when you observe but don't handle:

```dart
try {
  await save(doc);
} catch (e) {
  log.error('save failed', e);
  rethrow;                  // preserves original stack
}
```

- **Never write bare `catch { }`** that swallows errors. If you truly want to ignore an error, name it and leave a comment explaining why.
- **Don't catch `Error`** (or its subclasses like `StateError`, `ArgumentError`) — they indicate bugs and should crash loudly.

## API Design & Equality

- **Default to private**. Start every class, field, and helper with `_`, then promote to public only when something outside the library needs it. Smaller surface = less to maintain.
- **Don't create classes with only static members** — use top-level functions and constants in a library instead. Dart has first-class libraries; abusing classes as namespaces is a Java-ism.
- **Avoid one-method abstract classes** — use a function type alias (`typedef Predicate<T> = bool Function(T)`) instead.
- **Use class modifiers to constrain inheritance**: `final` (no extends/implements), `base` (only extends), `interface` (only implements), `sealed` (exhaustive hierarchy for pattern matching). Default `class` is usually too permissive for library code.
- **Return empty collections, not `null`**, from methods that produce "nothing" — `const []`, `const {}`, `Stream.empty()`. Callers shouldn't have to null-check.

### Equality

When overriding `==`, also override `hashCode`, and make the class immutable (equality on mutable state is a footgun).

```dart
class Point {
  final double x;
  final double y;
  const Point(this.x, this.y);

  @override
  bool operator ==(Object other) =>
      other is Point && other.x == x && other.y == y;

  @override
  int get hashCode => Object.hash(x, y);
}
```

Rules:
- `==` must be **reflexive, symmetric, transitive**, and never throw.
- The `other` parameter is `Object` (non-nullable) — don't declare it `Object?`.
- Use `Object.hash(...)` / `Object.hashAll(...)` instead of hand-rolled XOR.
- For value types with many fields or deep collections, reach for `package:equatable` or `freezed` rather than hand-writing.

## Strings

- Use single quotes `'...'` by default.
- Use interpolation `'$name'` / `'${user.name}'`, never `+` concatenation.
- Use raw strings `r'...'` for regex/paths.
- Use triple-quoted strings for multi-line.

```dart
final msg = 'User ${user.name} has ${user.items.length} items';
final pathRe = RegExp(r'^/users/(\d+)$');
```

## Types & Inference

Let inference do the work — but annotate at boundaries where readers can't see the initializer.

```dart
// Good — obvious from initializer
final count = 0;
final names = <String>[];
final user = User(name: 'Ada', age: 36);

// Good — annotate when there's no initializer
int count;
List<User> users;

// Good — annotate public API return types and parameters always
List<User> activeUsers(Iterable<User> all) => all.where((u) => u.isActive).toList();

// Bad — redundant annotation on initialized local
final int count = 0;

// Bad — missing return type on public function
activeUsers(all) => ...;
```

Rules:
- **Always annotate** public function parameters and return types, and fields/top-levels whose type isn't obvious.
- **Don't annotate** initialized locals, lambda parameters, or initializing formals (`this.x`) — inference handles them.
- **Avoid `dynamic`**. Use `Object?` to mean "any value" — it forces type checks at use sites.
- **Avoid `FutureOr<T>`** as a return type. Return `Future<T>` so callers always `await`.
- **Use `Future<void>`** for async methods that return nothing, never `Future` or `Future<Null>`.
- **Don't use raw generics**. `List` → `List<Object?>` or a concrete type. Turn on `strict-raw-types`.
- **Prefer full function-type signatures** over bare `Function`: `void Function(User)` not `Function`.

## Members (Getters, Setters, Fields)

- **Expose fields directly as `final`** — don't wrap trivial state in getters/setters.
- **Use getters** for derived, idempotent, side-effect-free values; **use `=>`** for simple ones.
- **Never write a setter without a getter**, and vice versa (unless there's a very good reason).
- **Initialize at declaration** when possible — it's less noise than constructor init lists.
- **Don't use `this.`** except in initializing formals or to disambiguate shadowed names.
- **Don't return `this`** to fake fluent APIs — use *cascades* instead.

```dart
// Good
class Rectangle {
  final double width;
  final double height;
  const Rectangle(this.width, this.height);

  double get area => width * height;        // derived, no state
  bool get isSquare => width == height;
}

// Good — cascades at call sites
final button = Button()
  ..label = 'OK'
  ..onPressed = submit
  ..enabled = true;
```

## Parameters

- **Avoid positional boolean parameters** — call sites become opaque. Use named.
- **Avoid optional positional parameters** when users would skip earlier ones — use named.
- **Use `required` named** for mandatory args when any param is optional or there are 2+ params.
- **Half-open ranges**: inclusive start, exclusive end. `substring(start, end)` means `[start, end)`.

```dart
// Bad
showDialog('Delete?', true, false, null);    // what are those?

// Good
showDialog(
  title: 'Delete?',
  barrierDismissible: true,
  useRootNavigator: false,
);
```

## Functions

- Always declare return types on public APIs; prefer them on private ones.
- Use arrow syntax `=>` only for single expressions.
- Prefer named parameters with `required` when there are 2+ params or any are optional — positional parameters become unreadable at call sites.
- **Use tear-offs** instead of trivial lambdas:

```dart
// Bad
names.forEach((n) => print(n));
users.map((u) => u.name).toList();

// Good
names.forEach(print);
users.map((u) => u.name).toList();   // lambda OK — extracts a field
```

- **Adjacent string literals concatenate** — use them for long messages instead of `+`:

```dart
throw StateError(
  'Cannot complete the transaction because the account '
  'is frozen and no override token was provided.',
);
```

```dart
// Bad
Rect makeRect(double x, double y, double w, double h, [double r = 0]) => ...;
makeRect(10, 20, 100, 50, 4);  // what is 4?

// Good
Rect makeRect({
  required double x,
  required double y,
  required double width,
  required double height,
  double radius = 0,
}) => ...;

makeRect(x: 10, y: 20, width: 100, height: 50, radius: 4);
```

## Extensions

Use extensions to add ergonomic helpers to types you don't own. Keep them focused and non-surprising.

```dart
extension StringX on String {
  bool get isBlank => trim().isEmpty;
  String? get nullIfBlank => isBlank ? null : this;
}
```

Don't use extensions to hide heavy logic — it makes call sites mysterious.

## Documentation

- Use `///` doc comments on every public API.
- First sentence is a noun phrase or third-person verb ("Returns..."), ending with a period.
- Reference identifiers in square brackets: `[User]`, `[fetch]`.
- Don't repeat the signature in prose — describe behavior, preconditions, and failure modes.

```dart
/// Fetches the user with [id] from the remote API.
///
/// Throws [NotFoundException] if no such user exists.
/// Returns a cached value if called within [cacheWindow].
Future<User> fetchUser(String id) async { ... }
```

## Testing

- Use `package:test` (Dart) or `flutter_test` (Flutter).
- One behavior per test. Name tests as sentences: `'returns empty list when query is blank'`.
- Group related tests with `group(...)`.
- Prefer real fakes over mock frameworks for simple dependencies.

```dart
void main() {
  group('UserRepository', () {
    late FakeApi api;
    late UserRepository repo;

    setUp(() {
      api = FakeApi();
      repo = UserRepository(api);
    });

    test('returns cached user on second call', () async {
      api.users['1'] = const User(name: 'Ada', age: 36);

      await repo.get('1');
      await repo.get('1');

      expect(api.callCount, 1);
    });
  });
}
```

## Flutter-Specific

- **Const widgets everywhere possible** — `const SizedBox(height: 8)` avoids rebuilds.
- Prefer `StatelessWidget` over `StatefulWidget` until you need local state.
- Keep `build` methods pure and fast. Extract subtrees into named widget classes, not private `_buildX()` methods — classes get `const` and better DevTools names.
- Always `dispose()` controllers, focus nodes, stream subscriptions, animation controllers.
- Never call `setState` after `dispose`; check `mounted` after an `await` in a `State`.

```dart
Future<void> _load() async {
  final data = await api.fetch();
  if (!mounted) return;
  setState(() => _data = data);
}
```

- For lists use `ListView.builder` with a stable `key` when items reorder.
- Don't do heavy work in widget constructors — they run on every rebuild. Put it in `initState` or a provider.

## Review Checklist

Before declaring Dart/Flutter work complete:

1. `dart format .` applied
2. `dart analyze` clean (zero issues)
3. No `!` bangs without a proven non-null check
4. No broad `catch` without `rethrow`; no catching `Error` subclasses
5. All `Future`s awaited or `unawaited()`
6. Public APIs have `///` docs and explicit return/parameter types
7. Fields are `final` / constructors are `const` where possible
8. Sealed classes used for exhaustive state, not enum-plus-if chains
9. Imports ordered: `dart:` → `package:` → relative, each alphabetized
10. Private-by-default; public API is the minimum surface actually needed
11. Named parameters used instead of positional booleans / 3+ positionals
12. No `new` keyword; no redundant `const` inside const contexts
13. No `dynamic` unless justified — use `Object?` for "any value"
14. `==` override paired with `hashCode`; only on immutable types
15. Tests pass; new behavior has a test
16. Flutter: controllers disposed, `mounted` checked after await, const widgets used
