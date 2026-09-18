# Anti-Patterns and Refactorings

Read this file when the honest answer to "which pattern?" is "none — something
needs removing". Each entry: **Smell → Why it hurts → The refactoring → What
makes it worse**.

Removing an abstraction still changes the codebase — announce it and get
approval like any other pattern decision (see SKILL.md gate).

## Contents

- [Too much in one place](#too-much-in-one-place)
- [Logic in the wrong place](#logic-in-the-wrong-place)
- [Too much abstraction](#too-much-abstraction)
- [Change amplification](#change-amplification)
- [Hidden behaviour](#hidden-behaviour)
- [Refactoring safely](#refactoring-safely)

---

## Too much in one place

### God object / god package

- **Smell:** one type or package everything imports; hundreds of lines of
  unrelated methods; the name is a category (`Manager`, `Service`, `Utils`,
  `common`, `helpers`).
- **Why it hurts:** every change touches it, so every change risks everything.
  It also becomes an import cycle magnet.
- **Refactoring:** group its methods by the data they actually touch. Each
  cluster is a real type. Extract clusters one at a time, moving tests with them.
  Name each by responsibility, never by layer word alone.
- **Worse:** splitting it by file while keeping one type — the coupling is
  unchanged, now spread across more files.

### Long function

- **Smell:** you have to scroll to see the whole thing; it has comment headers
  marking "sections".
- **Why it hurts:** the sections are the functions it should have been. Comment
  headers are the refactoring telling you where to cut.
- **Refactoring:** extract each section into a named function taking exactly what
  it uses. If a section needs six locals, that group of locals is probably a type.
- **Worse:** extracting into functions that take 8 parameters and mutate 3 of
  them — that hides the length without reducing the coupling.

---

## Logic in the wrong place

### Anemic domain model

- **Smell:** structs with only fields and getters; all rules live in a `Service`
  that reads the fields, decides, and writes them back.
- **Why it hurts:** invariants cannot be enforced, because anyone can build an
  invalid object and any service can apply a different rule.
- **Refactoring:** move the rule next to the data it constrains. Give the type a
  constructor that rejects invalid values and methods that are the only way to
  change it. Keep genuinely cross-entity policy in a domain service.
- **Worse:** adding a validation layer in the controller. Now the rule exists in
  two places and the second one is bypassable.

### Business logic in the controller / handler

- **Smell:** the HTTP handler queries, decides, and writes.
- **Why it hurts:** the rule is unreachable from any other transport and
  untestable without a request.
- **Refactoring:** move the decision into the usecase; leave parsing, auth
  binding, and response mapping in the handler.
- **Worse:** copying the logic into a second handler for the other transport.

### Business logic in the database

- **Smell:** rules in triggers, or in a stored procedure nobody reviews.
- **Why it hurts:** invisible to the code, invisible in review, invisible in
  tests, and it changes behaviour on deploy of a migration.
- **Refactoring:** move the rule into the domain. Keep constraints (`NOT NULL`,
  unique, foreign keys, check constraints) in the DB — those are integrity, not
  business logic, and they belong there.

---

## Too much abstraction

### Premature abstraction / speculative generality

- **Smell:** an interface with one implementation; a plugin system with one
  plugin; a `BaseX` with one subclass; parameters nothing passes.
- **Why it hurts:** every reader pays the indirection cost; the abstraction was
  shaped by one case, so the second case rarely fits and the abstraction gets
  bent rather than replaced.
- **Refactoring:** inline it. Return to the concrete code, then re-extract when
  the third case actually exists and shows the real axis of variation.
- **Worse:** adding the second implementation *just* to justify the interface.

### Config-driven everything

- **Smell:** behaviour selected by strings in a config file; a registry mapping
  names to constructors; flags that change control flow deeply.
- **Why it hurts:** the compiler stops helping. Wrong values become runtime
  failures, and "what does this do?" needs a config file plus a deploy manifest.
- **Refactoring:** make the variation explicit in code. Config should carry
  values (endpoints, limits, credentials), not choose code paths.
- **Worse:** validating the config strings at startup — that is a workaround for
  types you gave up.

### Inheritance for reuse

- **Smell:** a base class holding shared helpers; subclasses that override
  unrelated pieces; `super()` calls in the middle of methods.
- **Why it hurts:** it couples types that have nothing in common except a helper,
  and behaviour becomes a scavenger hunt across the hierarchy.
- **Refactoring:** move shared code into a plain function or a small collaborator
  and compose it. Inherit only for genuine substitutability.
- **Worse:** deepening the hierarchy to hold the next shared helper.

### Wrapper over a wrapper

- **Smell:** our client wrapping the SDK wrapping the transport; a repository
  over an ORM that already is one.
- **Why it hurts:** two layers, one purpose; errors and types get translated
  twice, losing detail each time.
- **Refactoring:** delete the layer that adds no rule. Keep the one that enforces
  something (domain types, an anti-corruption boundary, an invariant).

---

## Change amplification

### Shotgun surgery

- **Smell:** one conceptual change requires edits in many files — adding an enum
  value means touching six switches.
- **Why it hurts:** the compiler catches some sites and not others; the missed
  ones are production bugs.
- **Refactoring:** co-locate the knowledge. One table/map/type that owns the
  per-case data, and switches that the compiler forces to be exhaustive.
- **Worse:** a comment listing the six places to update.

### Copy-paste divergence

- **Smell:** three near-identical functions that drifted; a bug fixed in one.
- **Why it hurts:** every fix must be found three times, and the differences are
  now indistinguishable from intentional behaviour.
- **Refactoring:** diff them, decide which differences are real, parameterise
  only those, and unify. If the differences are not real, delete two.
- **Worse:** unifying with a boolean parameter that selects behaviour — that is
  two functions wearing one name.

### Feature envy

- **Smell:** a function that mostly reads another type's fields.
- **Refactoring:** move it onto that type, or pass a narrower value.

---

## Hidden behaviour

### Silent failure

- **Smell:** errors swallowed, caught-and-logged-and-continued, defaults returned
  on failure, empty `catch`.
- **Why it hurts:** the system reports success while doing nothing. These bugs
  surface far from their cause, often as data problems.
- **Refactoring:** propagate the error with context. Handle it only where there is
  a real decision to make. If the error is genuinely ignorable, say why in one
  comment — the comment is the design decision.
- **Worse:** logging at error level and continuing. That is a silent failure with
  a paper trail nobody reads.

### Global mutable state

- **Smell:** package-level variables mutated at runtime, singletons, init-order
  dependencies.
- **Why it hurts:** tests interfere with each other, order becomes significant,
  and coupling is invisible at the call site.
- **Refactoring:** construct in `main`, pass explicitly. Accept the longer
  signatures — they are the coupling made visible.

### Temporal coupling

- **Smell:** `Init()` must be called before `Run()`; `SetX()` before `Do()`;
  panics or zero values if the order is wrong.
- **Refactoring:** make the invalid state unrepresentable — the constructor
  returns a ready object, or each step returns the type that has the next method.
- **Worse:** documenting the required order.

### Primitive obsession

- **Smell:** ids, money, currency, and durations all passed as `string`/`int`;
  arguments swappable without a compile error.
- **Refactoring:** domain types with validating constructors. See
  `architecture.md`.

---

## Refactoring safely

Large refactors fail for procedural reasons more often than technical ones.

1. **Get the tests green first.** Refactoring on a red suite means you cannot
   tell breakage from pre-existing failure.
2. **Cover the current behaviour before changing it** — including the behaviour
   you consider wrong. Fix bugs in a separate commit, after the move.
3. **One kind of change per commit.** Move, then rename, then change behaviour —
   never together. A commit that both moves and edits a file is unreviewable.
4. **Keep it compiling at every step.** A refactor that is broken for two days is
   a rewrite with worse tooling.
5. **Prefer a bridge over a big bang.** New path beside the old, callers migrated
   one at a time, old path deleted last. The deletion commit is the one that
   proves the refactor finished.
6. **Stop when the pain is gone.** The goal is the specific problem you named at
   the start, not uniformity across the codebase.
