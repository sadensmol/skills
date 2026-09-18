# Architectural Patterns

Patterns for how components and boundaries inside a service fit together.
Service topology, deployment, and scaling belong to `sadensmol-system-design`.

Announce and get approval before applying any of these (see SKILL.md gate) —
these are the most expensive patterns to reverse.

## Contents

- [Layering](#layering) — Layered, Hexagonal, Anti-corruption layer
- [Modelling the domain](#modelling-the-domain) — Value objects, Entities, Aggregates, Domain services
- [Reads vs writes](#reads-vs-writes) — CQRS, Read models
- [History](#history) — Event sourcing, Audit log
- [Consistency across boundaries](#consistency-across-boundaries) — Outbox, Saga, Idempotent consumers
- [Cost table](#cost-table)

---

## Layering

### Layered (controller → usecase → service → repository)

- **Intent:** separate transport, orchestration, business rules, and persistence.
- **Use when:** essentially always for a service of non-trivial size. This is the
  default, not a pattern decision.
- **Do NOT:** let a layer skip another (controller reaching into repository), or
  let a lower layer import a higher one. Both defeat the whole point.
- **Watch:** if a layer only forwards calls with no logic, it is a pass-through
  tax — either give it a job or remove it. That judgement is per-project; follow
  the surrounding code and the project skill.

### Hexagonal / ports and adapters

- **Intent:** the domain defines interfaces (ports); infrastructure implements
  them (adapters). Dependencies point inward, always.
- **Use when:** the business rules are substantial, must be testable without
  infrastructure, or the infrastructure is expected to change (swap providers,
  multiple transports for one usecase).
- **Do NOT use when:** the "domain" is a CRUD pass-through. Wrapping a table in
  ports and adapters produces four files that together do what one did, with no
  rule to protect.
- **Watch:** the port belongs to the **consumer** side, named for what the domain
  needs (`UserStore`), not for what the implementation is (`PostgresClient`). A
  port that mirrors the driver's API is an adapter in disguise.

### Anti-corruption layer

- **Intent:** stop a foreign model (partner API, legacy system, aggregator) from
  leaking into ours.
- **Use when:** the external model is meaningfully different from ours, or
  unstable, or we do not control it.
- **Do NOT use when:** the external model is small and stable and we are a thin
  proxy over it. Then a translation layer just doubles the maintenance.
- **Watch:** translation happens **at the boundary**, once. External types must
  not appear past it — a partner's `Status` string reaching the usecase means the
  layer failed.

---

## Modelling the domain

### Value objects and domain types

- **Intent:** a type per concept, so wrong values cannot be constructed and wrong
  arguments cannot be passed.
- **Use when:** a primitive carries meaning and constraints — money, currency,
  ids, percentages, external references.
- **Do NOT use when:** it is genuinely a raw number with no rules and no risk of
  mix-up.
- **Watch:** validate in the constructor and make the raw field unexported, or the
  type is only a naming convention. Money as a float is a defect, not a style
  choice — use minor units in an integer, or a decimal type.

### Entities and aggregates

- **Intent:** an entity has identity and a lifecycle; an aggregate is the
  consistency boundary around a group of them, with one root.
- **Use when:** invariants span several objects and must hold at every commit
  ("a session's total bet never exceeds its limit").
- **Do NOT use when:** each record stands alone. Then the aggregate boundary is
  just the row.
- **Watch:** one transaction per aggregate. An operation that must atomically
  change two aggregates means the boundary is drawn wrong — redraw it, or accept
  eventual consistency deliberately (see Saga).

### Domain services

- **Intent:** behaviour that belongs to the domain but not to any one entity.
- **Use when:** the operation genuinely involves several aggregates or external
  policy.
- **Do NOT use when:** the logic belongs on the entity and was put in a service
  because the entity is anemic. See `anti-patterns.md` — that is the smell, not
  the fix.

---

## Reads vs writes

### CQRS (separate read model)

- **Intent:** reads and writes get different models, and often different stores.
- **Use when:** the read shape is irreconcilable with the write shape (a
  dashboard joining eight tables), or read and write load differ by orders of
  magnitude.
- **Do NOT use when:** the shapes are similar. Full CQRS with two stores adds
  synchronisation, lag, and a second source of truth — a permanent cost for a
  problem an index or a view usually solves.
- **Cheaper, in order:** an index → a database view → a denormalised column → a
  cached projection → a separate read store. Go down this list, not straight to
  the bottom.
- **Watch:** the moment reads are eventually consistent, the UI must handle
  "wrote it, cannot see it yet". Design that before choosing the pattern.

### Read models / projections

- **Intent:** precomputed, query-shaped copies of data.
- **Use when:** the query is hot and the source data changes far less often than
  it is read.
- **Watch:** every projection needs a documented rebuild path. A projection that
  cannot be rebuilt from source is a second source of truth.

---

## History

### Event sourcing

- **Intent:** store the sequence of events as the source of truth; current state
  is a fold over them.
- **Use when:** history *is* the requirement — audit, temporal queries,
  reconstructing past state, regulatory replay — and the domain is genuinely
  event-shaped.
- **Do NOT use when:** you only need "who changed what and when". That is an
  audit log, and it costs one table. Event sourcing changes how every query,
  migration, and schema change works, forever; versioning old events is a
  permanent tax.
- **Cheaper:** an append-only audit/history table alongside normal state. This is
  the right answer the large majority of the time.

---

## Consistency across boundaries

### Transactional outbox

- **Intent:** guarantee that a state change and its published event both happen.
- **Use when:** a DB write must reliably produce a message. This is the standard
  fix for dual-write failures, and it is usually worth it.
- **Do NOT use when:** losing the message is genuinely acceptable (best-effort
  notification) — then say so explicitly in the design rather than implying
  reliability you do not have.
- **Watch:** consumers get at-least-once delivery, so they must be idempotent
  (see `concurrency.md`). The relay needs monitoring and a backlog alert.

### Saga (compensating transactions)

- **Intent:** a multi-step operation across boundaries, where each step has a
  compensating action for rollback.
- **Use when:** the steps genuinely cannot share a transaction (separate services
  or separate systems) and partial completion is unacceptable.
- **Do NOT use when:** the steps could share one transaction. Distributed
  transaction logic to avoid a join is a large, self-inflicted cost.
- **Watch:** compensation is not rollback — a refund is not an un-charge, and some
  steps (an email sent) cannot be compensated at all. Enumerate every failure
  point and its compensator before writing any of it. Sagas need explicit state
  persistence; an in-memory saga loses its position on restart.

### Idempotent consumers

Any consumer of an at-least-once stream must be idempotent. Treat this as part of
the pattern, not as a later hardening step. See `concurrency.md`.

---

## Cost table

Reversibility is the real decision criterion. Prefer the top of this table.

| Pattern | Cost to add | Cost to remove later |
|---|---|---|
| Domain types / value objects | Low | Low |
| Layered structure | Low | Medium |
| Anti-corruption layer | Low–medium | Low |
| Hexagonal ports | Medium | Medium |
| Audit log | Low | Low |
| Read model / projection | Medium | Medium |
| Transactional outbox | Medium | Medium |
| Full CQRS (two stores) | High | High |
| Saga | High | High |
| Event sourcing | High | Very high — effectively permanent |

For anything in the bottom four rows, write down the alternative that was
rejected and why, and get explicit approval. An ADR is worth it there
(`sadensmol-system-design` has the template).
