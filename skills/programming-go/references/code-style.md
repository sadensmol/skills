# Go Code Style — full rules and examples

Read this when you write a function signature, a struct literal, a string, a collection
operation, a log line, a file/type/package name, or named returns. This is the full text
of the matching SKILL.md → *Project Standards* sections.

## Method Signatures

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

## Struct Initialization

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

## Embedded fields — keep the qualifier, silence the linter

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

## String Operations

No `+` concatenation (`sadensmol-programming` → *Code Shape*). Use `fmt.Sprintf` for fixed formats and
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

## Collection Operations

Use `pie2` for filter/map/reduce/find/any when the standard library lacks it:

```go
import pie2 "github.com/elliotchance/pie/v2"

active := pie2.Filter(users, func(u User) bool { return u.Active })
ids := pie2.Map(users, func(u User) string { return u.ID })
idx := pie2.FindFirstUsing(users, func(u User) bool { return u.Email == target })
total := pie2.Reduce(amounts, func(acc, v int64) int64 { return acc + v })
```

Use the standard library instead for iteration with side effects (`for`), `append`, `len`, and basic sorting.

## Logging

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
- Levels, and never log and return: `sadensmol-programming` → *Logging*, *Errors*.

## Naming Conventions

**Files:** hyphens for multi-word names, never underscores; lowercase; don't repeat the package name.

```
service/core.go               // NOT service/core-service.go
service/order-processing.go   // NOT service/order_processing.go
controller/v1/user.go         // NOT controller/v1/user_controller.go
```

**Types:** don't repeat the package name — `service.Core` not `service.CoreService`, `repository.Order` not `repository.OrderRepository`.

**Interfaces:** always prefix with `I` (`ICore`, `IRepository`, `IProductService`).
When to add one at all: `sadensmol-programming` → *Code Shape*; without one, inject the
concrete struct pointer.

**Packages:** short form of the path — `controller/v1` → `package controllerv1`, `usecase/v1` → `usecasev1`.

**Import aliases:** the current service's package has no suffix; the shared common library gets `2`; further clashes `3`, `4`…

```go
import (
    domain2 "github.com/acme/platform-common/domain"  // common lib
    domain "myservice/internal/domain"                // this service
)
```

**Getters:** no `Get` prefix — `Name()`, not `GetName()`.

## Code Shape

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

- Wrappers, helper extraction, the mapping exception and service encapsulation:
  `sadensmol-programming` → *Code Shape*.
