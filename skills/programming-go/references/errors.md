# Go Error Handling and Nil Safety — full rules and examples

Read this when you return, wrap, log or discard an error, write a deferred
commit/rollback, or dereference a pointer. This is the full text of SKILL.md →
*Error Handling* and *Nil Safety*.

## Error Handling

The rules are in `sadensmol-programming` → *Errors* (returned XOR logged, never log and
return, one error per return, layer roles). Go mechanics below.

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

**ALWAYS wrap (MUST FOLLOW).** Outside the controller, an error crossing a call
boundary gets `fmt.Errorf("<what failed>: %w", err)`. A bare `return err` reaches the
boundary as a context-free string ("context canceled", "sql: no rows"). Always `%w`,
never `%v`, so `errors.Is` / `errors.As` still work at the boundary — that is what lets
a cancellation be recognised and downgraded.

**Wrap — do NOT use `errors.Join` (MUST FOLLOW).** Its `Unwrap() []error` form
breaks the single-cause chain (`errors.Unwrap` returns nil for it), so the
boundary cannot split message from cause. One error, one `%w` chain.

**Two errors never meet in one return.** In Go the violation is
`fmt.Errorf("... %s: %w", first.Error(), second)` (`errorlint` rejects the `%v` form).
Return early when already failing, so the second operation never runs:

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

When the second operation genuinely must run, the single owning error describes the
other situation in text:
`fmt.Errorf("cannot close order %s after payment error: %w", id, saveErr)`.

**Never wrap a nil error.** `fmt.Errorf("…: %w", nil)` is a NON-nil error. Check
`if saveErr != nil` before wrapping.

**NEVER silently discard an error with `_ =` (MUST FOLLOW)** — log best-effort work at
the call site:

```go
// Bad
_ = s.cache.Save(ctx, key, &v)

// Good — best-effort, not returned, so logged
if err := s.cache.Save(ctx, key, &v); err != nil {
    log.Error().Ctx(ctx).Err(err).Msgf("cannot cache value for %s", key)
}
```

## Nil Safety — validate pointers before accessing (MUST FOLLOW)

The rule is `sadensmol-programming` → *Types and Domain Modelling* (check an optional
before you dereference it). In Go: nil-check a pointer before you dereference it or call
a method on it — struct pointer fields, map/slice-of-pointer elements, and
`*string`/`*int`/`*bool` optionals (or use a nil-safe accessor like `StringPtr()`).

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
