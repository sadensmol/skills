# Design Patterns

Object- and function-level patterns. Every entry is: **Intent → Use when → Do
NOT use when → Cheaper alternative**. The "do NOT" line is the important one.

Announce and get approval before applying any of these (see SKILL.md gate).

## Contents

- [Varying behaviour](#varying-behaviour) — Strategy, Template Method, State, Visitor
- [Creation](#creation) — Factory, Builder, Functional options, Singleton
- [Boundaries and wrapping](#boundaries-and-wrapping) — Adapter, Facade, Decorator, Middleware, Proxy
- [Notification](#notification) — Observer, Pub/Sub
- [Absence and structure](#absence-and-structure) — Null object, Option/Result, Composite
- [Patterns that are usually a mistake](#patterns-that-are-usually-a-mistake)

---

## Varying behaviour

### Strategy

- **Intent:** swap one algorithm for another behind a stable interface.
- **Use when:** the variants are genuinely independent, chosen at runtime, and at
  least three exist (or two exist and a third is already specified).
- **Do NOT use when:** there are two variants picked at compile time, or the
  "strategies" share most of their body and differ in one line.
- **Cheaper:** pass a function value. A single-method interface and a function
  parameter are the same thing; the function needs no type, no file, no registry.

### Template Method

- **Intent:** fix the skeleton of an algorithm, let subclasses fill in steps.
- **Use when:** the sequence is truly invariant and the steps are truly variable —
  e.g. a request lifecycle every handler must follow.
- **Do NOT use when:** the language favours composition (Go, Rust), or when
  subclasses start overriding the skeleton itself. That is the signal the
  skeleton was wrong.
- **Cheaper:** a function taking hook functions, or a small pipeline of explicit
  calls the caller writes out. Explicit beats inherited.

### State

- **Intent:** an object's behaviour changes with its state, and the transitions
  are a real machine.
- **Use when:** there are ≥4 states, the legal transitions matter, and illegal
  ones must be impossible to express.
- **Do NOT use when:** there are two states — that is a boolean. Or when the
  states never constrain each other.
- **Cheaper:** an enum plus a transition table, or a typed state field with a
  single `transition()` that validates. Fewer types, same guarantee.

### Visitor

- **Intent:** add operations over a closed set of types without editing them.
- **Use when:** the type set is closed and stable (AST nodes, a spec's message
  types) and operations are added often.
- **Do NOT use when:** the type set still changes. Every new type then forces
  edits to every visitor — the exact pain you paid to avoid.
- **Cheaper:** a type switch in one place. In Go, a `switch v := x.(type)` is
  readable and localised; Visitor buys nothing until it appears 4+ times.

---

## Creation

### Factory (function, not class)

- **Intent:** centralise the decision of which concrete thing to build.
- **Use when:** the choice depends on config/input, or construction has
  invariants that must not be bypassed.
- **Do NOT use when:** it only calls one constructor. `NewX()` that returns
  `&X{}` and nothing else is noise.
- **Cheaper:** a plain constructor function. Add the factory when a second
  variant actually exists.

### Builder

- **Intent:** assemble an object step by step when construction is long or
  conditional.
- **Use when:** ≥5 fields, many optional, and partially-built states must be
  invalid to use.
- **Do NOT use when:** the struct is small, or every field is required — then a
  constructor with named parameters is clearer and the compiler checks it.
- **Cheaper:** a config struct passed by value, or functional options.

### Functional options

- **Intent:** optional, extensible configuration without breaking callers.
- **Use when:** building a long-lived component (client, server, pool) whose
  option set will grow, especially in a public API.
- **Do NOT use when:** the options never grow, or the type is internal to one
  package. It costs a variadic signature and one closure per option for nothing.
- **Cheaper:** an options struct with zero-value defaults.

### Singleton

- **Intent:** exactly one instance, globally reachable.
- **Use when:** almost never. The legitimate cases are process-wide facilities
  the runtime already owns.
- **Do NOT use when:** you want convenient access to a dependency. That is a
  global variable with better PR — it hides coupling and destroys testability.
- **Cheaper:** construct once in `main`, pass it down explicitly. Long parameter
  lists are a visible cost; hidden globals are an invisible one.

---

## Boundaries and wrapping

### Adapter

- **Intent:** make a foreign interface fit ours.
- **Use when:** a third-party or legacy API crosses into our code and its shape,
  error style, or types would otherwise spread inward.
- **Do NOT use when:** the foreign interface is already the right shape. A
  pass-through adapter is a tax with no benefit.
- **Related:** at a domain boundary, this grows into an anti-corruption layer —
  see `architecture.md`.

### Facade

- **Intent:** one simple entry point over a complicated subsystem.
- **Use when:** callers currently need to know a 5-step dance to do one thing.
- **Do NOT use when:** it just re-exports the subsystem verbatim. A facade that
  grows one method per underlying method is a rename, not a simplification.

### Decorator

- **Intent:** add behaviour around an existing implementation without changing it.
- **Use when:** the added behaviour is orthogonal (metrics, caching, logging,
  retry) and composable in any order.
- **Do NOT use when:** the wrapper needs to know the wrapped type's internals, or
  when the stack grows past ~3 layers — debugging a 6-deep wrap is misery.
- **Cheaper:** put the behaviour in the one implementation, if there is only one.

### Middleware / chain

- **Intent:** the same, for request pipelines.
- **Use when:** the framework already has the concept (HTTP handlers, gRPC
  interceptors). Use its mechanism, do not invent a parallel one.
- **Do NOT use when:** ordering between middlewares is subtle and undocumented.
  Order-dependent middleware is a bug factory; make the dependency explicit or
  merge them.

### Proxy / lazy init

- **Intent:** stand in for the real object to defer cost or control access.
- **Use when:** creation is genuinely expensive and often unused.
- **Do NOT use when:** you are guessing about the cost. Measure first — lazy init
  adds a race and a nil-check to every access.

---

## Notification

### Observer / Pub-Sub

- **Intent:** one producer, an unknown set of consumers.
- **Use when:** the consumer set really is open, and the producer must not know
  them.
- **Do NOT use when:** there are two known consumers. Call them. In-process event
  buses destroy stack traces, hide ordering, and turn "who runs this?" into a
  grep. That cost is only worth paying when the set is truly open.
- **Cheaper:** direct calls, or a slice of callbacks owned by the producer.
- **If you do use it:** define delivery semantics up front — sync or async, error
  handling, ordering, and what happens when a subscriber panics.

---

## Absence and structure

### Null object

- **Intent:** a do-nothing implementation so callers skip nil checks.
- **Use when:** the "absent" behaviour is genuinely a valid no-op (a discard
  logger, a no-op metrics sink).
- **Do NOT use when:** absence is an error. Silently doing nothing hides bugs —
  the nil check was load-bearing.

### Option / Result types

- **Intent:** make absence and failure explicit in the type.
- **Use when:** the language supports it well (Rust, Swift, TS unions, Dart).
- **Do NOT use when:** the language already has an idiom. In Go, `(T, error)` and
  `(T, bool)` are the idiom — a hand-rolled `Option[T]` fights the ecosystem.

### Composite

- **Intent:** treat a leaf and a group of leaves the same way.
- **Use when:** the structure is genuinely recursive (filesystems, UI trees,
  nested rules).
- **Do NOT use when:** the nesting is one level deep and always will be. A slice
  is a composite you do not have to explain.

---

## Patterns that are usually a mistake

| Pattern | Why it goes wrong |
|---|---|
| Singleton | Hidden global state; untestable; forces initialisation order problems. |
| Service locator | Dependencies become invisible; failures move from compile time to run time. |
| Inheritance for reuse | Couples unrelated things through a base class that becomes a junk drawer. Compose instead. |
| Generic "Manager"/"Helper"/"Util" | The name admits it has no single responsibility. Split by what it actually does. |
| Repository over an ORM that is already a repository | Two indirection layers, one purpose. Pick one. |
| Interface per struct, by default | An interface with one implementation is documentation with extra steps. Extract it when the second implementation (or the test fake that genuinely needs it) arrives. |
| Abstract factory in application code | Almost always library-scale machinery applied to a two-case problem. |
