# Names Document the Logic — Not the Caller's Purpose

Read this when you name a repository finder or a method that selects by entity state.
Examples are in Go; the rule is the same in any language.

A name must say **what the code does** in terms of the **entity and its real
fields/states**, not the consumer's intent or invented jargon. A clear name
replaces a comment — reach for the name first.

This matters most for **repository finders that select by entity state**: name
the exact states they include, derived from the entity's own columns/flags.

```go
// the Cart entity's states come from its columns: ClosedAt, SucceededAt, FailedAt.

// Good — names the exact states the query selects, in the entity's own vocabulary
func (r *Cart) FindClosedAndSucceededByUGID(...) // ClosedAt set AND SucceededAt set
func (r *Cart) FindOpenOrSucceededByUGID(...)    // ClosedAt NULL  OR  SucceededAt set

// Bad — names the caller's purpose; hides the filter (and "history" even lies,
// since the result includes the still-live cart)
func (r *Cart) FindForHistoryByUGID(...)

// Bad — invents a state the entity does NOT have ("InFlight" is not a column;
// the entity's word for ClosedAt-NULL is "Open"), and writes "And" for an OR
func (r *Cart) FindInFlightAndSucceededByUGID(...)
```

Rules:
- Use the entity's actual state words (`Open`/`Closed`, `Succeeded`, `Failed`),
  not synonyms the codebase doesn't use (`InFlight`, `Active`, `Ongoing`).
- Encode the boolean shape: `XAndY` for AND, `XOrY` for OR. Never write `And`
  when the query is `OR`.
- Don't name by the consumer (`ForHistory`, `ForList`, `ForUI`) — the same data
  serves many callers; the name must survive a new one.
- **Never repurpose an existing well-named method.** If a caller needs different
  states, add a new correctly-named method and share the body (e.g. a private
  helper taking the state predicate) — don't widen the old one and leave its
  name lying. Remove the old method once nothing uses it.
- When the name fully describes the behaviour, **drop the doc comment** — a
  self-documenting name needs none.
