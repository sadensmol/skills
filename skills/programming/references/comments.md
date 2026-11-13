# Comments

Read this when you write, keep, delete or review a comment. This is the full text of
SKILL.md → *Comments*, with examples. Examples are in Go; the rule is the same in any
language.

**Scope: hand-written code only.** Never strip, rewrite, add, or file a finding against
a comment in a generated file — the next generator run overwrites it. Fix wrong output in
the generator, its template or its config. The language skill lists how to recognise a
generated file.

**Default: write NO comment.** A comment is the exception you must justify. Ask: "would
a competent reader get something *wrong* without this?" If you cannot name the concrete
misunderstanding, do not write it.

- **The #1 repeated violation:** a comment block above a query/filter predicate, a
  struct-tag line, a migration DDL, or a single added condition. Do not explain the WHERE
  clause or the new `AND x > 0`. Two comment lines that justify one line of code →
  delete them.
- **No doc comment** on a type whose name says what it is (`OrderRepository`, `UserID`),
  on a function whose name + signature say what it does (getters, constructors, mappers,
  `RequirePermission(p)`), on a guard or a 1–3 line block that reads plainly, or to
  restate the declaration ("X is the …", "returns a …", per-field narration of a config
  type or its tags).
- **No narration** of *what* the next line does, *why an obvious invariant holds*, what a
  route/middleware guarantees, or an error-handling decision the code already shows
  (log-and-continue vs return IS the decision).
- **Keep a comment only for the why the code cannot carry:** hard business logic,
  limitations/constraints (payment windows, ordering, races), non-obvious behaviour, or
  why a non-obvious solution was chosen. Keep it to that single fact.
- If a value object's guarantees need explaining, encode them in the type.
- Test code follows the same rules: the test name + the arrange/act/assert body already
  say what the test does. No doc comment that restates the name, no scenario narration.
  A test comment is kept only for a non-obvious why: a race the sleep guards, an ordering
  constraint, why a fixture is shaped oddly.

```go
// Bad - doc comment restates the declaration
// NewUser creates a new user
func NewUser(name string) *User { return &User{Name: name} }   // ❌

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
section banners/dividers (`// ---- helpers ----`; a file that needs signposting needs
splitting), change-log comments (`// new`, `// added for ABC-123`, `// was: oldName`),
TODOs for work this change already finished (an open TODO names who/what), and
commented-out code.

**A stale comment is worse than none.** When you touch a line, the comment above it is
part of what you touched — fix it or delete it.

**Never ask for a comment in review.** If a hunk is unreadable, the fix is *rename it*,
*split it*, or *let the type say it* — never prose above it.
