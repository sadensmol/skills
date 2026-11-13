---
name: sadensmol-programming-dart
description: "Expert Dart programming guidance following Effective Dart best practices. Use when (1) Writing or reviewing Dart or Flutter code, (2) Setting up Dart projects, (3) Implementing null safety, async, collections, classes, records, or pattern matching, (4) Writing unit or widget tests, (5) Refactoring Dart code, (6) Debugging Dart applications, (7) Designing APIs or data classes. Enforces idiomatic style, null safety, immutability, and modern Dart 3 features."
---

# Dart Programming

Act as a senior Dart/Flutter developer. Follow Effective Dart; this skill lists
only the tooling and the rules that go beyond or tighten it. **REQUIRED:** load
`sadensmol-programming` with this skill — it holds the language-agnostic rules (naming,
comments, domain types, layers, errors, tests); this skill wins where they conflict. Prefer built-in
language features over custom abstractions. Always run `dart format` and
`dart analyze` before declaring work complete.

## Tooling (non-negotiable)

- `dart format .` — single source of truth for formatting. Never hand-format; if a line reads badly after formatting, rewrite the expression instead of fighting the formatter.
- `dart analyze` — must pass with zero warnings.
- `dart fix --apply` — apply mechanical fixes.
- Enable strict lints in `analysis_options.yaml` (Flutter projects add the Flutter rules from `sadensmol-programming-flutter`): `references/setup.md`.

## Imports & File Layout

- Three import blocks, each alphabetized, separated by a blank line: `dart:` → `package:` → relative. Exports go after imports, in their own section.
- **Prefer `package:` imports** for files inside your own package — not relative — once you cross `lib/`.
- **Never write `library my_name;`** — use bare `library;` only to attach library-level doc comments.
- **Don't use `part`/`part of`** except for generated code (`json_serializable`, `freezed`).
- One public class per file; the file name matches the class in `snake_case` (`UserRepository` → `user_repository.dart`).

## Rules Beyond Effective Dart

- **Never use `!`** unless non-null is proven by a check on the line above. Prefer `??`, `?.`, `if (x != null)`. Use `late` only for genuinely initialized-before-read fields (`initState`); `late final` for lazy init.
- **Default to private** (`sadensmol-programming` → *Types and Domain Modelling*): start every class, field and helper with `_`.
- **Constrain inheritance with class modifiers** (`final`, `base`, `interface`, `sealed`) — plain `class` is usually too permissive for library code.
- **Sealed classes + `switch` expressions** are the Dart form of exhaustive state, never enum-plus-if chains.
- **Records** for multiple return values instead of a custom class or `Map`; promote to a class once it gains behaviour or crosses many layers.
- **Empty, not `null`**, for "nothing": `const []`, `const {}`, `Stream.empty()`.
- **Equality:** override `==` only on immutable types, always with `hashCode` (`Object.hash`). Many fields or deep collections → `package:equatable` or `freezed`.
- **Named parameters:** never positional booleans; `required` named parameters whenever there are 2+ parameters or any is optional.
- **No `dynamic`** unless justified — use `Object?` for "any value". Annotate public parameters and return types; return `Future<T>`, not `FutureOr<T>`.
- **Errors:** a broad `catch` on `Exception`/`Object` only with `rethrow`, or to log-and-continue at a true boundary (`sadensmol-programming` → *Errors*). Never catch `Error` subclasses. Never write a bare `catch {}` — name the error and comment why it is ignored. Use `rethrow` to keep the stack.
- **Async:** every `Future` is awaited or wrapped in `unawaited(...)`; always cancel stream subscriptions.
- **Strings:** single quotes; interpolation (never `+`); adjacent literals for long messages.
- **Docs:** `///` on every public API — describe behaviour, preconditions and failure modes, not the signature.

## Testing

- `package:test` (Dart) or `flutter_test` (Flutter); `group(...)` related tests.
- Test names are sentences: `'returns empty list when query is blank'` (principles: `sadensmol-programming` → *Testing*).
- Prefer hand-written fakes over mock frameworks for simple dependencies.

## Flutter

Flutter widget, lifecycle, `const`, `mounted`, list and disposal rules live in
`sadensmol-programming-flutter` — load it for any Flutter code.

## Read on demand

**When a trigger below matches what you are about to do, read that file first** — before
you write, review or answer. The summary in this skill is not the full rule.

| When | Read |
|---|---|
| You create a project or change `analysis_options.yaml` | `references/setup.md` |
| **Before declaring Dart work complete**, or when you review Dart code | `references/review-checklist.md` |
