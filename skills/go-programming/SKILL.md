---
name: sadensmol-go-programming
description: "Expert Go programming guidance acting as a senior Go developer and architect. Use when: (1) Writing or reviewing Go code, (2) Setting up Go projects, (3) Implementing Go patterns and idioms, (4) Writing unit tests, (5) Refactoring Go code, (6) Debugging Go applications, (7) Making architectural decisions for Go services. Provides guidance on project structure, error handling, concurrency, domain-driven design, and enforces specific coding standards including method signatures, comment formatting, mapper patterns, and tooling usage."
---

# Go Programming

## Before Changing Code

1. Check the code hierarchy (base structures, interfaces, embedding).
2. Review existing patterns in the project and follow them — don't invent new styles.
3. Rule of 3: externalize logic only after 3 duplications.
4. Don't create a function that already exists in the project.
5. Keep layer boundaries clean (controller → usecase → service → repository).
6. Use domain types for identifiers and values (`type UserID uuid.UUID`, `type Email string`) so passing a `ProductID` where a `UserID` is expected doesn't compile.
7. Pass only domain objects between layers; convert at the boundaries with mapper methods (`toDomain()`, `toAPI()`, `asModel()`).
8. Follow the naming conventions below (files, packages, import aliases).

## Project Standards

### Method Signatures

More than 3 parameters (context doesn't count) → a `Data` struct, placed
immediately above the method, not at the top of the file.

```go
// Bad
func CreateUser(ctx context.Context, name, email, phone, address string) error

// Good
type CreateUserData struct {
    Name    string
    Email   string
    Phone   string
    Address string
}

func CreateUser(ctx context.Context, data CreateUserData) error
```

### Struct Initialization

**Use named-field struct literals and OMIT zero-value fields (MUST FOLLOW).**
Never use positional literals beyond a tiny, stable 1–2 field type where every
field is always set (`image.Point{x, y}`). Never spell out a field equal to its
zero value (`nil`, `0`, `""`, `false`) — it hides the fields that carry
information and forces every literal to change when a field is added. A pointer
field whose `nil` means "not applicable" is left out entirely.

```go
// Bad - positional literal, every field spelled out incl. zero values
"AED":  {"United Arab Emirates Dirham", "د.إ", 2, nil, false, false, nil},
"COP#": {"Milli Colombia Peso", "$", 2, conv.Pointer("COP"), false, false, conv.Pointer(1000)},

// Good - named fields, zero values omitted
"AED":  {Name: "United Arab Emirates Dirham", Symbol: "د.إ", DecimalPlaces: 2},
"COP#": {Name: "Milli Colombia Peso", Symbol: "$", DecimalPlaces: 2,
         IsFractionalFor: conv.Pointer("COP"), FractionalValue: conv.Pointer(1000)},
```

Applies everywhere a struct is built: map-of-struct tables, test fixtures, config literals.

### Comments

**Scope: hand-written Go only.** Never strip, rewrite, add, or file a finding
against a comment in a generated file — the next generator run overwrites it.
A file is generated when it carries `// Code generated ... DO NOT EDIT.`
(authoritative: `grep -l '^// Code generated .* DO NOT EDIT\.'`), is a
moq/mockery mock (`**/mocks/*.go`), a protobuf/gRPC stub (`*.pb.go`,
`*_grpc.pb.go`), an `oapi-codegen` API package, a `gen/` query-builder package, a
`*_gen.go` / `*_generated.go` file, or lives under `vendor/`. Fix wrong output in
the generator, its template, or its config.

**Default: write NO comment.** A comment is the exception you must justify. Ask:
"would a competent Go reader get something *wrong* without this?" If you can't
name the concrete misunderstanding, don't write it.

- **The #1 repeated violation:** a comment block above a query/filter predicate,
  a struct-tag line, a migration DDL, or a single added condition. Don't explain
  the WHERE clause or the new `AND x > 0`. Two comment lines justifying one line
  of code → delete them.
- **No doc comment** on a type whose name says what it is (`OrderRepository`,
  `UserID`), on a function whose name + signature say what it does (getters,
  constructors, mappers, `RequirePermission(p)`), on a guard or a 1–3 line block
  that reads plainly, or to restate the declaration ("X is the …", "returns a …",
  per-field narration of a config struct or its `env:"…"` tags).
- **No narration** of *what* the next line does, *why an obvious invariant holds*,
  what a route/middleware guarantees, or an error-handling decision the code
  already shows (log-and-continue vs `return err` IS the decision).
- **Keep a comment only for the why the code cannot carry:** hard business logic,
  limitations/constraints (payment windows, ordering, races), non-obvious
  behaviour or why a non-obvious solution was chosen. Keep it to that single fact.
- **Lowercase**: comments start with a small letter.
- If a value object's guarantees need explaining, encode them in the type.

```go
// Bad - doc comments restating the declaration
// NewUser creates a new user
func NewUser(name string) *User { return &User{Name: name} }   // ❌

// Bad - capitalized
// Process handles the request
func process() {}                                                // ❌

// Bad - restates the type and narrates every field and env tag
// Config holds the HTTP server configuration. ListenAddr is the address the
// server binds to. ReadTimeout is the per-request read timeout.
type Config struct {                                             // ❌
    ListenAddr  string        `env:"LISTEN_ADDR,required"`
    ReadTimeout time.Duration `env:"READ_TIMEOUT,required"`
}

// Bad - narrates a decision the log + continue already show
if err := h.images.EnsureCached(ctx, items); err != nil {
    // the list is the primary payload, so a failure here must not fail the
    // request — but it can't be silent either.                 ❌
    log.Error().Ctx(ctx).Err(err).Msg("cannot record image sources for list")
}

// Good - only the one non-obvious fact survives
// refund is only possible within 24 hours due to payment processor limitations
func (s *Service) ProcessRefund(ctx context.Context, txID string) error

// Good - a multi-param constructor keeps only the ctx-lifetime fact
// ctx is the base context for token refresh — pass the app lifecycle context
// so refresh stops on shutdown.
func NewHTTPClient(ctx context.Context, tokenURL, clientID, ...) *http.Client
```

**Comments that must never exist** — delete on sight in hand-written code you touch:
section banners/dividers (`// ---- helpers ----`; a file that needs signposting
needs splitting), change-log comments (`// new`, `// added for ABC-123`,
`// was: oldName`), TODOs for work this change already finished (an open TODO
names who/what), and commented-out code.

**A stale comment is worse than none.** When you touch a line, the comment above
it is part of what you touched — fix it or delete it.

**Never ask for a comment in review.** If a hunk is unreadable, the fix is
*rename it*, *split it*, or *let the type say it* — never prose above it.

### Formatting

**NEVER format code manually.** Never run gofmt or any formatting tool — rely on IDE formatting.

### Embedded fields — keep the qualifier, silence the linter

**NEVER collapse an embedded-struct qualifier out of a selector to satisfy
staticcheck's `QF1008`.** The qualifier says *which* embedded config the field
came from. Keep it and suppress with a specific, explained directive — `nolintlint`
is usually configured with `require-specific` and `require-explanation`, so a bare
`//nolint` is itself a lint failure.

```go
// Bad — QF1008 "satisfied", but Addr no longer says which config it is
client := redis.NewClient(&redis.Options{Addr: cfg.Addr})

// Good
//nolint:staticcheck // QF1008: keep the Redis. qualifier — a bare Addr does not
// say which config's address it is
client := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr})
```

### String Operations

NEVER use `+` for string concatenation. Use `fmt.Sprintf` for fixed formats and
`strings.Builder` for dynamic/loop construction. `%s` calls `String()` on
Stringers, so no `.String()` noise. Applies to cache keys, log messages, error
messages, query fragments.

```go
// Bad
key := k.CustomerID.String() + ":" + k.ExternalID.String() + ":" + k.UserID.String()

// Good
key := fmt.Sprintf("%s:%s:%s", k.CustomerID, k.ExternalID, k.UserID)

var b strings.Builder
for i, v := range parts {
    if i > 0 {
        b.WriteByte(',')
    }
    b.WriteString(v)
}
```

### Collection Operations

Use `pie2` for filter/map/reduce/find/any when the standard library lacks it:

```go
import pie2 "github.com/elliotchance/pie/v2"

active := pie2.Filter(users, func(u User) bool { return u.Active })
ids := pie2.Map(users, func(u User) string { return u.ID })
user, found := pie2.FindFirstUsing(users, func(u User) bool { return u.Email == target })
total := pie2.Reduce(amounts, func(acc, v int64) int64 { return acc + v }, 0)
```

Use the standard library instead for iteration with side effects (`for`), `append`, `len`, and basic sorting.

### Logging

zerolog, short form only: every value goes into `.Msgf()`'s format string. NEVER
use `.Str()`, `.Int()`, `.Int64()`, `.Bool()`, etc. `.Err(err)` is the only
exception.

```go
// Good
log.Info().Ctx(ctx).Msgf("order placed for user %s, session %s, amount %d", userID, sessionID, amount)
log.Error().Ctx(ctx).Err(err).Msgf("failed to process: %s", id)

// Bad
log.Info().Ctx(ctx).Str("session_id", sessionID).Int64("order_cents", amount).Msgf("order placed for user %s", userID)
```

- `.Ctx(ctx)` for trace-ID injection.
- Product/money operations at `Info` with all relevant IDs.
- `Debug` for flow tracing (omitted in production).
- Never log and return — see Error Handling.

### API Responses

```go
// HTTP status - always 200 for success, never 201/204
return ctx.JSON(http.StatusOK, response)  // NOT http.StatusCreated
return ctx.NoContent(http.StatusOK)       // NOT http.StatusNoContent

// Times - always UTC in RFC3339
CreatedAt: user.CreatedAt.UTC().Format(time.RFC3339),

// Money - always a string in the main currency unit
Amount: "10.50",  // NOT 1050 cents
```

### Naming Conventions

**Files:** hyphens for multi-word names, never underscores; lowercase; don't repeat the package name.

```
service/core.go               // NOT service/core-service.go
service/order-processing.go   // NOT service/order_processing.go
controller/v1/user.go         // NOT controller/v1/user_controller.go
```

**Types:** don't repeat the package name — `service.Core` not `service.CoreService`, `repository.Order` not `repository.OrderRepository`.

**Interfaces:** always prefix with `I` (`ICore`, `IRepository`, `IProductService`).
Add an interface only when it earns it: **multiple implementations** exist, or **a
unit test mocks it**. Otherwise inject the concrete struct pointer; introduce the
interface later, at the consumer that needs the test seam.

**Packages:** short form of the path — `controller/v1` → `package controllerv1`, `usecase/v1` → `usecasev1`.

**Import aliases:** the current service's package has no suffix; the shared common library gets `2`; further clashes `3`, `4`…

```go
import (
    domain2 "github.com/acme/platform-common/domain"  // common lib
    domain "myservice/internal/domain"                // this service
)
```

**Getters:** no `Get` prefix — `Name()`, not `GetName()`.

### Error Handling

Use `domain.Error` with status-specific constructors and optional codes:

```go
domain.NewBadRequestError("Invalid input", err)
domain.NewNotFoundError("Product unavailable", err).WithCode(domain.ErrorCodeProductNotFound)
```

**Layer rules:**

```go
// Repository - name the root cause
return nil, fmt.Errorf("failed to query user: %w", dbErr)

// Service / usecase - wrap with context
return nil, fmt.Errorf("failed to get user: %w", err)

// Controller - NO wrapping, return as-is; the boundary middleware formats and logs it
return err
```

**ALWAYS wrap — every propagated error carries context (MUST FOLLOW).** Outside the
controller, an error crossing a call boundary gets `fmt.Errorf("<what failed>: %w", err)`.
A bare `return err` reaches the boundary as a context-free string ("context
canceled", "sql: no rows"). Always `%w`, never `%v`, so `errors.Is` / `errors.As`
still work at the boundary — that is what lets a cancellation be recognised and
downgraded.

**Wrap — do NOT use `errors.Join` (MUST FOLLOW).** Its `Unwrap() []error` form
breaks the single-cause chain (`errors.Unwrap` returns nil for it), so the
boundary cannot split message from cause. One error, one `%w` chain.

**Two errors must never meet in one return — restructure so only one can exist
(MUST FOLLOW).** Never interpolate one error's text into the other's message
(`fmt.Errorf("... %s: %w", first.Error(), second)`; `errorlint` rejects the `%v`
form). Fix the control flow — usually return early when already failing, so the
second operation never runs:

```go
// Bad — two live errors, welded together by string interpolation
defer func() {
    if saveErr := repo.Save(txCtx, order); saveErr != nil {
        rollback()
        if err != nil {
            err = fmt.Errorf("cannot save order %s: %w", saveErr.Error(), err)  // ❌
            return
        }
        err = fmt.Errorf("cannot save order: %w", saveErr)
        return
    }
    ...
}()

// Good — the failing path exits first, so there is only ever one error
defer func() {
    if err != nil {
        rollback()
        return
    }
    if saveErr := repo.Save(txCtx, order); saveErr != nil {
        rollback()
        err = fmt.Errorf("cannot save order: %w", saveErr)
        return
    }
    if commitErr := commit(); commitErr != nil {
        err = fmt.Errorf("cannot commit order: %w", commitErr)
    }
}()
```

Before restructuring, verify the skipped work has nothing left to persist on the
failing path — do not assume it. When the second operation genuinely must run,
pick **which single error owns the return** and describe the situation in text:
`fmt.Errorf("cannot close order %s after payment error: %w", id, saveErr)`.

**Never wrap a nil error.** `fmt.Errorf("…: %w", nil)` is a NON-nil error. Check
`if saveErr != nil` before wrapping.

**NEVER log and return.** The boundary (error middleware / interceptor) logs it
once; a layer that logs and returns produces two lines for one event.

**NEVER silently discard an error with `_ =` (MUST FOLLOW).** Every error is
**returned XOR logged** — never both, never neither. Best-effort work you
deliberately don't return (cache writes, fire-and-forget, cleanup) is logged at
the call site. A shared helper that swallows errors internally is
a smell — make it return the error and let each caller decide.

```go
// Bad
_ = s.cache.Save(ctx, key, &v)

// Good — best-effort, not returned, so logged
if err := s.cache.Save(ctx, key, &v); err != nil {
    log.Error().Ctx(ctx).Err(err).Msgf("cannot cache value for %s", key)
}
```

### Nil Safety — validate pointers before accessing (MUST FOLLOW)

**Nil-check a pointer before dereferencing it or calling a method on it** — never
assume "the caller always sets it" or "the middleware guarantees it". The
function that dereferences owns the guard; guard at the point of access, not
only upstream. Applies to struct pointer fields, map/slice-of-pointer elements,
and `*string`/`*int`/`*bool` optionals (or use a nil-safe accessor like `StringPtr()`).

```go
func (m *AuthMiddleware) validateClaims(claims *Claims) error {
    if claims == nil {
        return errors.New("nil claims")
    }
    if len(claims.GetSubject()) == 0 { ... }
}
```

If a value is genuinely always present, model it as a **value type** (`T`, not
`*T`) so nil is impossible by construction and no guard is needed.

### Domain Model and Mappers

Read `references/architecture.md` when you convert between API/domain/DB types,
add a value object or domain predicate, or wire a service's repositories. Rules:

- **API, domain and persistence models never mix across layers.** Controllers
  convert API → domain right after binding (domain constructors validate);
  services and repositories accept and return only domain objects.
- **Every type-to-type conversion goes through a mapper method on a type alias** —
  `toDomain()`, `toAPI()`, `asModel()` (`fromDB()` as an alternative to
  `toDomain()`) — even a one-line, single-call-site conversion.
- **Rich domain models (MUST FOLLOW):** a concept with identity + intrinsic
  attributes is one value object carrying both, not a bare string plus a
  `map[ID]Attrs` side table. Parse once at the boundary (`ByName`); give it
  `String()` returning the identity.
- **Domain logic on the domain type (MUST FOLLOW):** a predicate computed only
  from a type's own fields is a method on that type, not a service-layer helper.
- **Repository access respects domain boundaries (MUST FOLLOW):** a service uses
  its own domain's repositories directly; another domain's data goes through
  that domain's service, never its repository.

### Code Shape

- **Named returns — avoid by default (MUST FOLLOW).** The one legitimate reason
  is letting a `defer` set the return value (tx commit/rollback). Then:
  - **All or none** — Go forbids mixing.
  - **Name them ALL meaningfully** — never `_` placeholders like
    `(_ *Order, _ Balance, err error)`, no single-letter names.
  - **Always return explicitly** in every branch — never a naked `return`.
  - **Compose, don't clobber:** `if commitErr := commit(); commitErr != nil && err == nil { err = commitErr }` —
    an unconditional `err = commit()` turns a failing return into a false success.
  - The body's first assignment becomes `=`, not `:=` (`shadow`/`NoNewVar` flag misses).

  ```go
  func (u Checkout) Process(ctx context.Context, ...) (order *domain.Order, balance domain2.Balance, err error) {
      order, err = u.orderService.ByCartID(ctx, cart.ID)
      if err != nil {
          return nil, domain2.Balance{}, err
      }
      defer func() {
          if commitErr := commit(); commitErr != nil && err == nil {
              err = commitErr
          }
      }()
      ...
      return order, balance, nil
  }
  ```

- **NEVER create one-line wrapper functions** that only forward, rename, re-order,
  or pad defaults onto another function (`func badRequest(msg string) error { return domain2.NewBadRequestError(msg, nil) }`).
  Call the underlying function directly.
- **Don't extract 1–3 line helpers — inline them.** Extract only real reusable
  logic with **3+ consumers**, or a genuine utility for a shared package. "It
  appears twice" is not enough — duplicate the two lines. When inlining, keep
  only the conditions reachable at that site.
  - **Exception — type-to-type mapping is architecture, not trivial code.** Always
    use a mapper method (see above), never an inline `fmt.Sprintf` / field copy /
    `string(x)` / struct literal. A value derived for a type (an absolute URL from
    `BaseURL` + path) is a method on that type or its mapper. "It's only one line"
    justifies inlining a *guard*, never a *mapping*.

### Service Encapsulation

- **No duplicate service instantiation:** create a service with the same
  dependencies once and pass it around.
- **Don't expose an internal service for one method call** — add a delegating method:

```go
// Bad - caller: service.BaseService().TxStatus(ctx, req)
func (s *Integration) BaseService() *IntegrationService { return s.baseService }

// Good - caller: service.TxStatus(ctx, req)
func (s *Integration) TxStatus(ctx context.Context, req *TxStatusRequest) (TxStatus, error) {
    return s.baseService.TxStatus(ctx, req)
}
```

## Testing

### Test-first vs implementation-first (MUST FOLLOW)

**TDD applies to BUG FIXES ONLY.** The behaviour exists and is wrong: write the
failing test first, watch it fail, then fix.

**New logic (features, methods, endpoints) is implementation-first.** Write the
code, then cover it with tests. Do NOT invoke `superpowers:test-driven-development`
for new functionality, and don't stall behind a red-test ritual. The deciding
question: **does the behaviour already exist and misbehave?** Yes → test first.
No → code first.

### Mocking with moq

Generate mocks with [matryer/moq](https://github.com/matryer/moq) via `go run` (no global install):

```
service/
  core.go
  tx-status_test.go          # unit tests next to the code
  mocks/
    order-repository.go      # one mock per file, interface name without -mock suffix
```

```makefile
MOQ_RUN:=go run github.com/matryer/moq@latest

.PHONY:gen-mock
gen-mock:
	$(MOQ_RUN) -out service/mocks/order-repository.go -pkg mocks ./service IOrderRepository
```

```go
package service_test

func TestService_Method(t *testing.T) {
    orderRepo := &mocks.IOrderRepositoryMock{
        FindByIDFunc: func(ctx context.Context, id domain.OrderID) (*domain.Order, error) {
            return &domain.Order{ID: id}, nil
        },
    }
    svc := service.NewService(orderRepo, nil, nil)
    // ...
}
```

- Mocks live in `<package>/mocks/`, package `mocks`.
- Tests use `package <name>_test` (black-box).
- Regenerate with `make gen-mock` after interface changes.

### Test rules shared with integration tests

Unit tests follow the same rules as `sadensmol-go-integration-tests` → *Before
Writing Tests* and *Running Tests*: check existing tests and follow their
pattern; never change production code while writing tests without asking first;
constants for test data (lowercase when local to one test, uppercase when
shared); run tests via the editor's test tasks, never the command line; on
failure read the logs, add logs if needed, never guess. Integration tests
(under `tests/`) → load that skill.

## References

- `references/architecture.md` — read when mapping API/domain/DB types, adding a
  value object or domain method, or deciding which repositories a service may use.
- `references/best-practices.md` — read when versioning an API, handling money or
  request validation at the boundary, or naming a repository finder.
- `references/testing.md` — read when a test asserts on an error message or you
  add otelhttp to an HTTP client.
- `references/actual-patterns.md` — read when you need a worked example of domain
  errors, go-jet queries, graceful shutdown, distributed locking, or the
  zerolog/OTel logger setup.
- `references/project-structure.md` — read when creating a service or package
  layout, a migration, a table/constraint name, partitioning, or the Makefile.
