---
name: sadensmol-programming
description: "Language-agnostic programming principles that every language skill builds on: naming, comments, code shape, domain types and value objects, layer and domain boundaries, error handling and logging, absence and nil handling, backward-compatible data, and test principles. Use when (1) writing, reviewing or refactoring code in any language, (2) any language skill loads (`sadensmol-programming-go`, `sadensmol-programming-typescript`, `sadensmol-programming-dart`, `sadensmol-programming-flutter`, `sadensmol-programming-swift`, `sadensmol-programming-python`) — load this one with it, (3) writing or reviewing tests, (4) deciding where logic, validation, conversion or error handling belongs, (5) naming a method, type or repository finder."
---

# Programming Principles

The rules here hold in every language. A language skill adds syntax, tooling and idioms
on top; a project skill adds its own conventions. **On conflict: project skill >
language skill > this skill.** Examples are in Go; the rule is the same in any language.

## Before Changing Code

1. Check the code hierarchy first: base types, interfaces, composition.
2. Follow explicit project rules first. For a new feature, find the closest existing
   feature and replicate its shape: schema, API, layers, naming, tests. Use general
   guidance only where project rules and existing patterns give no answer.
3. Search before you write. Never add a function that already exists.
4. Keep the layer boundaries clean (see *Layers*).
5. **Rule of three** (below) decides every "keep it raw or extract it" question.

## Rule of Three (MUST FOLLOW)

Below three, keep it plain and in place; at three, extract. It applies in every section:

- **Parameters:** up to 3 parameters (a context does not count) stay raw arguments. At 4+,
  pass one params struct (`…Data`, `…Params`) declared right above the method.
- **Reuse:** a value, condition, expression, query fragment or block used in fewer than 3
  places stays inline at each use — duplicate it for readability. At the 3rd use, extract
  it (variable, constant, helper).
- **Abstraction:** add a shared helper, generic type or shared package only at the 3rd
  concrete case or consumer. Two similar cases stay two plain copies.

Exceptions: type-to-type mapping always goes through a mapper (see *Layers*); an
interface follows its own rule (see *Code Shape*).

## Naming — Names Document the Logic

- A name says **what the code does**, in the entity's own vocabulary — not the
  caller's purpose and not invented jargon. A clear name replaces a comment.
- A finder that selects by entity state names the exact states: `XAndY` for AND,
  `XOrY` for OR. Never name by the consumer (`ForHistory`, `ForUI`), and never invent
  a state the entity does not have.
- **Never repurpose a well-named method.** Add a new, correctly named one and share the
  body.

Read `references/naming.md` when you name a repository finder or a state-dependent method.

## Comments

**Full rules:** read `references/comments.md` before you write, keep or review a comment.

Hand-written code only: never touch a comment in a generated file — fix the generator.

**Default: write NO comment.** Keep one only if you can name the concrete misunderstanding
a competent reader would have without it.

- **The #1 repeated violation:** a comment above a query/filter predicate, a struct tag, a
  migration DDL, or a single added condition. Delete it.
- No doc comment that restates the declaration, name or signature; no narration of what
  the next line does or of an error-handling decision the code already shows.
- Keep a comment only for the why the code cannot carry — that single fact.
- Test code follows the same rules.
- Delete on sight: section banners, change-log comments, finished TODOs, commented-out code.
- **A stale comment is worse than none.** Fix or delete the comment above a line you touch.
- **Never ask for a comment in review** — rename, split, or let the type say it.

## Code Shape

**Full rules:** read `references/code-shape.md` before you wrap, extract, inline or add an interface.

- **NEVER create one-line wrapper functions** that only forward, rename, re-order, or pad
  defaults onto another function. Call the underlying function directly.
- **Do not extract 1–3 line helpers — inline them** (*Rule of three*: extract only at
  **3+ consumers**). Type-to-type mapping is the exception: always a mapper (see *Layers*).
- **Do not name a value that is used once.** A condition, expression or query fragment
  used in one place stays inline at that place; never park it in a local variable or
  helper a few lines above (*Rule of three*).
- **Method signatures follow the *Rule of three*:** up to 3 parameters raw, 4+ in a
  params struct.
- **An interface must earn its place:** multiple implementations, or a unit test mocks it.
  Otherwise inject the concrete type.
- **Composition over inheritance.** Prefer pure functions.
- **Build strings with formatting, interpolation or a builder — never `+` concatenation.**

### Service encapsulation

- **No duplicate service instantiation:** create a service once and pass it around.
- **Do not expose an internal service for one method call** — add a delegating method.

## Types and Domain Modelling

**Full rules:** read `references/domain-model.md` before you add a value object, domain predicate or domain type.

- **Domain types for identifiers and values** (`UserID`, `Email`), not raw `string` /
  `uuid`. Call the type's accessor, never a raw cast.
- **Rich domain models (MUST FOLLOW):** identity + intrinsic attributes form one value
  object, not an identifier plus a side table. Parse once at the boundary.
- **Domain logic on the domain type (MUST FOLLOW):** a predicate computed only from a
  type's own fields is a method on that type.
- **Mutually exclusive states are one sum type**; switch over it exhaustively, no `default`.
- **Immutable by default.**
- **Absence is not emptiness:** return an empty collection or string, never `null`/`nil`.
- **A value that is always present is non-optional.**
- **Check an optional before you dereference it (MUST FOLLOW)** — the function that
  dereferences owns the guard.
- **Validate at the boundary**; inner code trusts the types.
- **A shared or generic type appears only at the 3rd concrete case** (*Rule of three*).
- **Narrowest visibility first.**

## Layers

**Full rules:** read `references/domain-model.md` before you decide where a conversion or repository call belongs.

- Calls go one way: transport → usecase (optional) → service → repository. Never skip a
  layer, never call upward.
- **API, domain and persistence models never cross into another layer.** The controller
  converts API → domain on read; services and repositories take only domain objects.
- **Every type-to-type conversion goes through a named mapper** (`toDomain()`, `toAPI()`,
  `asModel()`), even a one-line one.
- **Move code into a shared package only at its 3rd consumer** (*Rule of three*).
- **Repository access respects domain boundaries (MUST FOLLOW):** another domain's data
  goes through that domain's service, never its repository.
- **A service knows nothing about its caller (MUST FOLLOW):** it exposes plain actions on
  its own domain (`Get`, `Inc`, `Reset`). The usecase that owns the flow picks the action.
  Never pass a caller-specific enum or flag into a service to pick its behaviour.
- **Keep pure logic free of the host framework**; the app layer is a thin adapter.

## Errors

**Full rules:** read `references/errors.md` before you handle, wrap, swallow or catch an error.

- **Every error is returned XOR logged — never both, never neither (MUST FOLLOW).**
- **Never log and return.** The boundary logs once.
- **Best-effort work you deliberately do not return** is logged at the call site.
- **Every propagated error carries context** ("cannot X: cause") and keeps the cause
  inspectable.
- **Layer roles:** repository names the root cause; service/usecase adds context;
  controller returns it as-is.
- **Two errors must never meet in one return (MUST FOLLOW).** Exit early on the failing
  path; never weld one error's text into another.
- **Catch only what you handle.** No errors for non-exceptional control flow.

## Logging

- Product and money operations log at `Info` with every relevant ID.
- Flow tracing logs at `Debug` (omitted in production).
- Never log a secret or an API key.

## Backward-Compatible Data

**Full rules:** read `references/backward-compatible-data.md` before you change a persisted model or a released contract.

Persisted models and released contracts change only in backward-compatible ways: a new
field is optional and emitted only when not at its default, decoding is lenient, and an
explicit version bump is only for a break a default cannot cover. What only the current
unreleased task added has no old data, so it needs no compatibility path. API contract
rules live in `sadensmol-programming-api`.

## Testing

**Full rules:** read `references/testing.md` before you write a test, compute an expected value, or assert on an error.

- **Test-first applies to BUG FIXES ONLY (MUST FOLLOW).** Does the behaviour already exist
  and misbehave? Yes → test first. No → code first. Never invoke
  `superpowers:test-driven-development` for new functionality.
- Follow the existing tests' pattern; add to the feature's existing test file.
- Never change production code while you write tests. If it must change, ask first.
- Use constants for test data.
- **The expected-value oracle is an independent implementation.**
- **Black-box:** drive the public API; fake only dependencies, within their real contract.
- **Assert on meaning, not on library message text.**
- **Reproduce a bug with the fewest entities**, with fakes as strict as the real dependency.
- One behaviour per test, named by the behaviour; Arrange-Act-Assert.
- Deterministic: inject clock, network, storage; poll instead of sleep, except to prove
  something did NOT happen.
- When a test fails, read the logs first. Never guess.

## Read on demand

**When a trigger below matches what you are about to do, read that file first** — before
you write, review or answer. The summary in this skill is not the full rule.
Each section above states its rules briefly; the reference holds the full text and examples.

- `references/comments.md` — read before you write or keep a comment, or review one.
- `references/code-shape.md` — read when you decide to wrap, extract, inline or add an
  interface.
- `references/naming.md` — read when you name a repository finder or a method that
  selects by entity state.
- `references/domain-model.md` — read when you add a value object, domain predicate or
  domain type, or decide where a conversion or repository call belongs.
- `references/backward-compatible-data.md` — read when you change a persisted model or a
  released contract.
- `references/errors.md` — read when you handle, wrap, swallow or catch an error.
- `references/testing.md` — read when you write a test, compute an expected value, or
  assert on an error.
