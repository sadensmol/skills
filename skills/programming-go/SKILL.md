---
name: sadensmol-programming-go
description: "Expert Go programming guidance acting as a senior Go developer and architect. Use when: (1) Writing or reviewing Go code, (2) Setting up Go projects, (3) Implementing Go patterns and idioms, (4) Writing unit tests, or integration tests under `tests/` (testify/suite, gRPC bufconn mocks, WireMock, fixtures, test repositories), (5) Refactoring Go code, (6) Debugging Go applications, (7) Making architectural decisions for Go services. Provides guidance on project structure, error handling, concurrency, domain-driven design, and enforces specific coding standards including method signatures, comment formatting, mapper patterns, and tooling usage."
---

# Go Programming

**REQUIRED:** load `sadensmol-programming` with this skill. It holds the
language-agnostic rules — naming, comments, code shape, domain types, layers and
mappers, error handling, logging levels, test principles. This skill holds only the
Go mechanics and wins where the two conflict.

## Before Changing Code

1. Follow `sadensmol-programming` → *Before Changing Code*.
2. Domain types are named types: `type UserID uuid.UUID`, `type Email string`.
3. Follow the naming conventions below (files, packages, import aliases).

## Project Standards

### Method Signatures

**Full rules (this and the sections down to *Naming Conventions*):** read `references/code-style.md` before you write a signature, struct literal, string, collection operation, log line or name.

More than 3 parameters (context doesn't count) → a `Data` struct, placed
immediately above the method, not at the top of the file.

### Struct Initialization

**Use named-field struct literals and OMIT zero-value fields (MUST FOLLOW).** No
positional literals beyond a tiny, stable 1–2 field type. Never spell out a field equal
to its zero value. Applies to map-of-struct tables, test fixtures and config literals.

### Comments

The comment rules are in `sadensmol-programming` → *Comments* (default: no comment).
Go additions:

- **Lowercase**: comments start with a small letter — `// process handles the request`,
  never `// Process handles the request`.
- **Generated Go files** — never touch their comments. A file is generated when it
  carries `// Code generated ... DO NOT EDIT.` (authoritative:
  `grep -l '^// Code generated .* DO NOT EDIT\.'`), is a moq/mockery mock
  (`**/mocks/*.go`), a protobuf/gRPC stub (`*.pb.go`, `*_grpc.pb.go`), an
  `oapi-codegen` API package, a `gen/` query-builder package, a `*_gen.go` /
  `*_generated.go` file, or lives under `vendor/`.

### Formatting

**NEVER format code manually.** Never run gofmt or any formatting tool — rely on IDE formatting.

### Embedded fields — keep the qualifier, silence the linter

**NEVER collapse an embedded-struct qualifier out of a selector to satisfy
staticcheck's `QF1008`.** Keep it and suppress with a specific, explained `//nolint`
directive — a bare `//nolint` is itself a lint failure.

### String Operations

No `+` concatenation. Use `fmt.Sprintf` for fixed formats and `strings.Builder` for
dynamic/loop construction. `%s` calls `String()`, so no `.String()` noise.

### Collection Operations

Use `pie2` (`github.com/elliotchance/pie/v2`) for filter/map/reduce/find/any when the
standard library lacks it; the standard library for side-effect loops, `append`, `len`
and basic sorting.

### Logging

zerolog, short form only: every value goes into `.Msgf()`'s format string. NEVER
use `.Str()`, `.Int()`, `.Int64()`, `.Bool()`, etc. `.Err(err)` is the only
exception. Always `.Ctx(ctx)` for trace-ID injection. Levels, and never log and return:
`sadensmol-programming` → *Logging*, *Errors*.

### API Responses

**Full rules:** read `references/best-practices.md` before you shape an API response.

HTTP success is always `200` (never 201/204); times are UTC RFC3339; money is a string
in the main currency unit (`"10.50"`, not cents).

### Naming Conventions

- **Files:** hyphens for multi-word names, never underscores; lowercase; don't repeat the
  package name (`service/order-processing.go`, `controller/v1/user.go`).
- **Types:** don't repeat the package name — `service.Core`, not `service.CoreService`.
- **Interfaces:** always prefix with `I` (`ICore`). When to add one at all:
  `sadensmol-programming` → *Code Shape*; without one, inject the concrete struct pointer.
- **Packages:** short form of the path — `controller/v1` → `package controllerv1`.
- **Import aliases:** the current service's package has no suffix; the shared common
  library gets `2`; further clashes `3`, `4`…
- **Getters:** no `Get` prefix — `Name()`, not `GetName()`.

### Error Handling

**Full rules (with *Nil Safety*):** read `references/errors.md` before you return, wrap or discard an error, write a deferred commit/rollback, or nil-check a pointer.

The rules are in `sadensmol-programming` → *Errors*. Go mechanics:

- Use `domain.Error` with status-specific constructors and optional codes
  (`domain.NewNotFoundError(msg, err).WithCode(...)`).
- Repository names the root cause, service/usecase wraps with context, controller
  returns `err` as-is.
- **ALWAYS wrap (MUST FOLLOW).** Outside the controller: `fmt.Errorf("<what failed>: %w",
  err)`. Always `%w`, never `%v`.
- **Wrap — do NOT use `errors.Join` (MUST FOLLOW).** One error, one `%w` chain.
- **Two errors never meet in one return** — no `fmt.Errorf("... %s: %w", first.Error(),
  second)`. Return early when already failing.
- **Never wrap a nil error** — `fmt.Errorf("…: %w", nil)` is non-nil.
- **NEVER silently discard an error with `_ =` (MUST FOLLOW)** — log best-effort work at
  the call site.

### Nil Safety — validate pointers before accessing (MUST FOLLOW)

Nil-check a pointer before you dereference it or call a method on it — struct pointer
fields, map/slice-of-pointer elements, and `*string`/`*int`/`*bool` optionals (or use a
nil-safe accessor). A value that is always present is a value type (`T`, not `*T`).

### Domain Model and Mappers

The rules (model separation, mappers, rich domain models, domain logic on the type,
repository boundaries) are in `sadensmol-programming` → *Types and Domain Modelling*,
*Layers*. In Go:

- A mapper is a **method on a type alias** of the source type
  (`type repositoryOrder model.Order` → `func (r repositoryOrder) toDomain()`).
- A value object's identity formats via `String()`, so `%s` prints it in keys and logs.

Read `references/architecture.md` when you write a mapper between API/domain/DB types.

### Code Shape

- **Named returns — avoid by default (MUST FOLLOW).** Use them only to let a `defer` set
  the return value. Then: all or none, all named meaningfully, explicit `return` in every
  branch, and compose — never clobber — `err` in the defer.
- Wrappers, helper extraction, the mapping exception and service encapsulation:
  `sadensmol-programming` → *Code Shape*.

## Testing

Test principles (test-first for bug fixes only, independent oracle, black-box, assert
on meaning) are in `sadensmol-programming` → *Testing*.

### Mocking with moq

**Full rules:** read `references/mocking.md` before you write a unit test that needs a moq mock.

- Generate mocks with `matryer/moq` via `go run` (no global install), one mock per file.
- Mocks live in `<package>/mocks/`, package `mocks`.
- Tests use `package <name>_test` (black-box).
- Regenerate with `make gen-mock` after interface changes.

### Go test rules

- Test-data constants: lowercase when local to one test, uppercase when shared.
- Run tests the way the project defines (`make test`, a script, the editor's test task
  named in the project docs). Only when the project defines no way, use `go test`.
- Integration tests (under `tests/`) → read `references/integration-tests.md` (only then).

## Read on demand

**When a trigger below matches what you are about to do, read that file first** — before
you write, review or answer. The summary in this skill is not the full rule.
Each section above states its rules briefly; the reference holds the full text and examples.

- `references/code-style.md` — read when you write a signature, struct literal, string,
  collection operation, log line, file/type/package name, or named returns.
- `references/errors.md` — read when you return, wrap or discard an error, write a
  deferred commit/rollback, or nil-check a pointer.
- `references/mocking.md` — read when you write a unit test that needs a moq mock.
- `references/integration-tests.md` — read when you create, change or run Go tests under
  `tests/` (integration tests). It indexes `references/integration-tests/*.md`.
- `references/architecture.md` — read when you write a mapper between API/domain/DB
  types.
- `references/best-practices.md` — read when you design an API version or response, or
  handle money or request validation at the boundary.
- `references/testing.md` — read when you add otelhttp to an HTTP client that tests
  cover.
- `references/actual-patterns.md` — read when you need a worked example of domain
  errors, go-jet queries, graceful shutdown, distributed locking, or the
  zerolog/OTel logger setup.
- `references/project-structure.md` — read when creating a service or package
  layout, a migration, a table/constraint name, partitioning, or the Makefile.
