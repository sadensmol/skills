---
name: sadensmol-dart-programming
description: "Expert Dart programming guidance following Effective Dart best practices. Use when (1) Writing or reviewing Dart or Flutter code, (2) Setting up Dart projects, (3) Implementing null safety, async, collections, classes, records, or pattern matching, (4) Writing unit or widget tests, (5) Refactoring Dart code, (6) Debugging Dart applications, (7) Designing APIs or data classes. Enforces idiomatic style, null safety, immutability, and modern Dart 3 features."
---

# Dart Programming

Act as a senior Dart/Flutter developer. Follow Effective Dart; this skill lists
only the tooling and the rules that go beyond or tighten it. Prefer built-in
language features over custom abstractions. Always run `dart format` and
`dart analyze` before declaring work complete.

## Tooling (non-negotiable)

- `dart format .` — single source of truth for formatting. Never hand-format; if a line reads badly after formatting, rewrite the expression instead of fighting the formatter.
- `dart analyze` — must pass with zero warnings.
- `dart fix --apply` — apply mechanical fixes.
- Enable strict lints in `analysis_options.yaml` (Flutter projects add the Flutter rules from `sadensmol-flutter-programming`):

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

- Three import blocks, each alphabetized, separated by a blank line: `dart:` → `package:` → relative. Exports go after imports, in their own section.
- **Prefer `package:` imports** for files inside your own package — not relative — once you cross `lib/`.
- **Never write `library my_name;`** — use bare `library;` only to attach library-level doc comments.
- **Don't use `part`/`part of`** except for generated code (`json_serializable`, `freezed`).
- One public class per file; the file name matches the class in `snake_case` (`UserRepository` → `user_repository.dart`).

## Rules Beyond Effective Dart

- **Never use `!`** unless non-null is proven by a check on the line above. Prefer `??`, `?.`, `if (x != null)`. Use `late` only for genuinely initialized-before-read fields (`initState`); `late final` for lazy init.
- **Default to private.** Start every class, field and helper with `_`; promote to public only when something outside the library needs it.
- **Constrain inheritance with class modifiers** (`final`, `base`, `interface`, `sealed`) — plain `class` is usually too permissive for library code.
- **Sealed classes + `switch` expressions** for exhaustive state, never enum-plus-if chains.
- **Records** for multiple return values instead of a custom class or `Map`; promote to a class once it gains behaviour or crosses many layers.
- **Return empty collections, not `null`**, for "nothing" (`const []`, `const {}`, `Stream.empty()`).
- **Equality:** override `==` only on immutable types, always with `hashCode` (`Object.hash`). Many fields or deep collections → `package:equatable` or `freezed`.
- **Named parameters:** never positional booleans; `required` named parameters whenever there are 2+ parameters or any is optional.
- **No `dynamic`** unless justified — use `Object?` for "any value". Annotate public parameters and return types; return `Future<T>`, not `FutureOr<T>`.
- **Errors:** never catch `Exception`/`Object` broadly unless you `rethrow`, or log-and-continue at a true boundary (UI root, request handler). Never catch `Error` subclasses. Never write a bare `catch {}` — name the error and comment why it is ignored. Use `rethrow` to keep the stack.
- **Async:** every `Future` is awaited or wrapped in `unawaited(...)`; always cancel stream subscriptions.
- **Strings:** single quotes; interpolation, never `+`; adjacent literals for long messages.
- **Docs:** `///` on every public API — describe behaviour, preconditions and failure modes, not the signature.

## Testing

- `package:test` (Dart) or `flutter_test` (Flutter); `group(...)` related tests.
- One behaviour per test, named as a sentence: `'returns empty list when query is blank'`.
- Prefer hand-written fakes over mock frameworks for simple dependencies.

## Flutter

Flutter widget, lifecycle, `const`, `mounted`, list and disposal rules live in
`sadensmol-flutter-programming` — load it for any Flutter code.

## Review Checklist

Before declaring Dart work complete:

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
16. Flutter code: the `sadensmol-flutter-programming` review checklist also passes
