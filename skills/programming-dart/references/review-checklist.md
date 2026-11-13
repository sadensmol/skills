# Dart Review Checklist

Read this before declaring Dart work complete, and when you review Dart code.

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
16. Flutter code: the `sadensmol-programming-flutter` review checklist (its `references/review-checklist.md`) also passes
