---
name: sadensmol-programming-patterns
description: "Language-agnostic catalog of design patterns, concurrency and resilience patterns, architectural patterns, and the anti-patterns/refactorings that undo them. Every entry carries intent, use-when, and an explicit do-NOT-use-when, so patterns are chosen deliberately instead of by reflex. Use ONLY for (1) designing NEW functionality that does not exist yet — a new feature, service, subsystem, module, or public interface, or (2) a LARGE refactoring that restructures existing code: extracting a layer, breaking up a god object, changing how components fit together, untangling a module. NEVER use it while debugging, investigating a failure, writing or running tests, fixing a bug, patching a defect, making a small local edit, or reviewing code — there the existing shape is a constraint, not a decision, and pattern advice is noise. Requires announcing every candidate pattern to the user and getting an explicit yes before applying it. Defers to the active language skill (go-programming, typescript-programming, dart-programming, flutter-programming, swift-programming) for syntax, naming, and idioms."
---

# Programming Patterns

A pattern is a named trade-off, not a goal. This skill exists to help pick the
**smallest** structure that resolves the forces in play — and, just as often, to
justify picking none.

## Hard gate — when NOT to use this skill

Stop and do not apply pattern guidance when the task is:

- Debugging, reproducing a failure, or root-causing anything → `superpowers:systematic-debugging`
- Writing, fixing, or running tests → the language skill's testing section / `sadensmol-go-integration-tests`
- Fixing a bug or patching a defect — even a structural-looking one. Fix it in place.
- A small local edit: a new field, a new branch, a new endpoint that copies the one next to it
- Reviewing code → `sadensmol-code-review`
- System topology (service decomposition, deployment, scaling, infra) → `sadensmol-system-design`

In every one of these, the existing code's shape is a **constraint you inherit**,
not a decision you get to make. Suggesting a pattern there converts a bounded
task into a rewrite.

## The three rules that override every pattern below

1. **Rule of three.** Do not abstract until the same shape appears three times.
   Two occurrences are a coincidence.
2. **YAGNI.** A pattern that only pays off "when we add X later" is not earning
   its cost today. Add it when X arrives.
3. **Name the force first.** If you cannot state the specific pressure the
   pattern relieves — "this varies per tenant", "this call fails and must not
   cascade", "these two lifecycles differ" — there is no pattern to apply. Write
   the direct code.

A direct function that a reader understands in one pass beats a correctly-named
pattern they have to trace through four files.

## ⚠️ Mandatory approval gate — never apply a pattern silently

**Every time you decide a pattern fits, you MUST tell the user and ask before
writing any code that uses it.** No exceptions, no "it is obviously right here",
no "it is only a small one". The user decides whether the abstraction lands in
their codebase.

Ask in 3–5 lines, and no more:

> I want to use **\<pattern name\>** for \<the specific place\>.
> Force it resolves: \<the one pressure, concretely\>.
> Cost: \<extra files/indirection it adds\>.
> Without it: \<what the direct code looks like instead\>.
> Use it here?

Then **stop and wait**. Rules for the gate:

- One question per pattern. Three patterns in a design → three approvals, or one
  message listing all three with a yes/no each. Do not bundle them into a
  take-it-or-leave-it design.
- Present the direct, pattern-free alternative honestly every time. If you cannot
  describe it, you have not understood the problem well enough to pattern it.
- "No" is a complete answer. Write the direct code and do not re-propose it later
  in the same task.
- Approval covers **that** use site only. The next place is a new question.
- If you only recognise a pattern **after** writing the code, say so and offer to
  remove it — do not retroactively justify it.
- This gate is in addition to any approval `superpowers:brainstorming` already
  required. A design approved as a whole does not pre-approve its patterns.

## Workflow

1. **State the forces.** What varies? What must stay stable? What can fail? What
   has an independent lifecycle? Write these down before naming anything.
2. **Check the index below** — symptom → candidate → reference file.
3. **Read only the matching reference file.** Not all four.
4. **Apply the do-NOT-use-when test** in that entry. If it fires, take the
   cheaper alternative the entry names.
5. **Ask the user** — run the approval gate above and wait for a yes.
6. **Write it in the project's own idiom.** The reference files are neutral;
   syntax, naming, error style, and file layout come from the active language
   skill and from the surrounding code. Never introduce a pattern name the
   codebase does not already use without saying so.
7. **Leave an escape hatch.** A pattern you can delete in one commit is cheap. A
   pattern threaded through every call site is not.

## Index — symptom to candidate

### Behaviour varies, or objects need wiring → `references/design-patterns.md`

| Symptom | Candidates |
|---|---|
| One algorithm with several interchangeable variants | Strategy, Template Method |
| `switch`/`if` chain on a type tag, repeated in several places | Polymorphism, Visitor, State |
| Construction is complex, conditional, or has many optional parts | Factory, Builder, Functional options |
| Third-party or legacy interface does not match ours | Adapter, Facade, Anti-corruption layer |
| Need to add behaviour without touching the original | Decorator, Middleware/chain |
| One change must notify several unknown listeners | Observer, Pub/Sub, Event bus |
| Expensive or lazily-created collaborator | Proxy, Lazy init, Object pool |
| Absent value handled with null checks everywhere | Null object, Option/Result type |
| Tree of parts and wholes treated uniformly | Composite |
| Cross-cutting concern (auth, logging, retry) around a call | Middleware, Decorator |

### Work runs in parallel, or a dependency can fail → `references/concurrency.md`

| Symptom | Candidates |
|---|---|
| N independent items to process, bounded resources | Worker pool, Semaphore |
| Multi-stage transformation over a stream | Pipeline, Fan-out/fan-in |
| A remote call can be slow, flaky, or hang | Timeout, Retry with backoff + jitter, Circuit breaker |
| Producer outruns consumer | Bounded queue, Backpressure, Load shedding |
| The same request may arrive twice | Idempotency key, Dedup store |
| Shared state mutated from several places | Ownership by one goroutine/actor, Immutable snapshot, Mutex (last resort) |
| Work must survive process restart | Outbox, Durable queue, Checkpointing |
| Shutdown drops in-flight work | Graceful shutdown, Context cancellation |

### Components and boundaries → `references/architecture.md`

| Symptom | Candidates |
|---|---|
| Business logic entangled with HTTP/DB/SDK code | Hexagonal (ports and adapters), Layered |
| Domain concepts scattered as bare primitives and maps | Value objects, Entities, Aggregates, Domain types |
| Reads and writes have irreconcilable shapes or load | CQRS (read model), separate query path |
| Need the full history of what happened, not just current state | Event sourcing, Audit log (usually the cheaper answer) |
| DB write and message publish must both happen or neither | Transactional outbox |
| A multi-service operation needs compensation on failure | Saga, or redesign so it is one transaction |
| An external model is leaking into ours | Anti-corruption layer, Mapper at the boundary |

### The code works but is wrong-shaped → `references/anti-patterns.md`

Read this file when the honest answer to "which pattern?" is "none — this needs
removing". It maps each smell (god object, anemic domain, shotgun surgery,
premature abstraction, config-driven everything, inheritance for reuse) to the
specific refactoring that fixes it, and to the ones that make it worse.

## Relationship to other skills

- **Language idioms** — `sadensmol-go-programming`, `typescript-programming`,
  `dart-programming`, `flutter-programming`, `swift-programming`. They own
  syntax, naming, error handling, project layout, and testing. This skill never
  overrides them.
- **System-level topology** — `sadensmol-system-design` owns service
  decomposition, deployment, scaling, and ADRs. This skill stops at the process
  boundary.
- **Design conversation** — `superpowers:brainstorming` owns getting to a design
  and its approval. Use this skill to inform the options, not to skip the gate.
