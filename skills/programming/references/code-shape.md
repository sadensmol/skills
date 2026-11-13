# Code Shape

Read this when you decide whether to wrap, extract, inline or add an interface, or how a
caller reaches another service. This is the full text of SKILL.md → *Code Shape*.
Examples are in Go; the rule is the same in any language.

- **NEVER create one-line wrapper functions** that only forward, rename, re-order, or pad
  defaults onto another function
  (`func badRequest(msg string) error { return NewBadRequestError(msg, nil) }`). Call the
  underlying function directly.
- **Rule of three (MUST FOLLOW):** below three, keep it plain and in place; at three,
  extract. Up to 3 parameters (a context does not count) stay raw; 4+ go in one params
  struct declared right above the method. A value, condition, expression or block used in
  fewer than 3 places stays inline and is duplicated; at the 3rd use, extract it. A shared
  helper, generic type or shared package appears only at the 3rd concrete case.

  ```go
  // Bad - 4 raw parameters
  func CreateUser(ctx context.Context, name, email, phone, address string) error

  // Good - 4+ parameters go in a params struct right above the method
  type CreateUserData struct {
      Name, Email, Phone, Address string
  }

  func CreateUser(ctx context.Context, data CreateUserData) error
  ```
- **Do not extract 1–3 line helpers — inline them.** Extract only real reusable logic
  with **3+ consumers**, or a genuine utility for a shared package. "It appears twice" is
  not enough — duplicate the two lines. When you inline, keep only the conditions that
  are reachable at that site.
  - **Exception — type-to-type mapping is architecture, not trivial code.** Always use a
    mapper (see *Layers*), never an inline format call, field copy, cast or literal. A
    value derived for a type (an absolute URL from `BaseURL` + path) is a method on that
    type or its mapper. "It is only one line" justifies inlining a *guard*, never a
    *mapping*.
- **Do not name a value that is used once.** A condition, expression or query fragment
  used in one place stays inline at that place; never park it in a local variable or
  helper a few lines above, so the reader does not jump back to find what it means. Name
  it only for real reuse.

  ```go
  // Bad - named once, read once, four lines later
  unclearOutcome := t.Status.IS_NULL().OR(t.Status.NOT_BETWEEN(Int(400), Int(499)))
  where := t.RefundRequestedAt.IS_NULL().
      AND(t.ResponseError.IS_NOT_NULL()).
      AND(unclearOutcome)

  // Good - the condition sits where it applies
  where := t.RefundRequestedAt.IS_NULL().
      AND(t.ResponseError.IS_NOT_NULL()).
      AND(t.Status.IS_NULL().OR(t.Status.NOT_BETWEEN(Int(400), Int(499))))
  ```
- **An interface (protocol, abstract type) must earn its place:** **multiple
  implementations** exist, or **a unit test mocks it**. Otherwise inject the concrete
  type; add the interface later, at the consumer that needs the test seam.
- **Composition over inheritance** — use functions and interfaces. Prefer pure functions — same input, same output, no
  side effects — wherever the logic allows.
- **Build strings with formatting, interpolation or a builder — never `+`
  concatenation.** Applies to cache keys, log messages, error messages, query fragments.

## Service encapsulation

- **No duplicate service instantiation:** create a service with the same dependencies
  once and pass it around.
- **Do not expose an internal service for one method call** — add a delegating method.

```go
// Bad - caller: service.BaseService().TxStatus(ctx, req)
func (s *Integration) BaseService() *IntegrationService { return s.baseService }

// Good - caller: service.TxStatus(ctx, req)
func (s *Integration) TxStatus(ctx context.Context, req *TxStatusRequest) (TxStatus, error) {
    return s.baseService.TxStatus(ctx, req)
}
```
