# Concurrency and Resilience Patterns

Patterns for work that runs in parallel, and for dependencies that fail. Same
format: **Intent → Use when → Do NOT use when → Cheaper alternative**.

Announce and get approval before applying any of these (see SKILL.md gate).

## Contents

- [Before any concurrency](#before-any-concurrency)
- [Parallel work](#parallel-work) — Worker pool, Fan-out/fan-in, Pipeline, Semaphore
- [State sharing](#state-sharing) — Ownership, Immutable snapshot, Mutex
- [Failure of dependencies](#failure-of-dependencies) — Timeout, Retry, Circuit breaker, Bulkhead, Fallback
- [Flow control](#flow-control) — Bounded queue, Backpressure, Load shedding, Rate limit
- [Correctness under retry](#correctness-under-retry) — Idempotency, Dedup, Exactly-once myth
- [Lifecycle](#lifecycle) — Cancellation, Graceful shutdown, Durable work

---

## Before any concurrency

Ask, in order:

1. **Is it actually slow?** Measure. Concurrency added to fast code buys nothing
   and costs every future reader.
2. **Is the work independent?** If items share mutable state, the parallel
   version is a different, harder program.
3. **What bounds it?** Unbounded goroutines/tasks are the single most common
   production failure in this family. Every spawn site needs a bound.
4. **Who cancels it?** Work with no cancellation path leaks on shutdown.

If any answer is unclear, write the sequential version. It is correct, and it is
the baseline the concurrent version must beat.

---

## Parallel work

### Worker pool

- **Intent:** process N items with a fixed number of concurrent workers.
- **Use when:** N is large or unknown, and the downstream resource (DB, API, CPU)
  has a limit you must respect.
- **Do NOT use when:** N is small and bounded (say ≤10 known items) — spawn them
  directly with a wait group and skip the pool machinery.
- **Watch:** the pool size must come from the *constraint* (connection pool size,
  API rate limit), not from a round number. Document which.

### Fan-out / fan-in

- **Intent:** split one input across workers, then merge results.
- **Use when:** results must be collected and the merge is trivial.
- **Do NOT use when:** results must stay ordered — merging then re-sorting often
  costs more than the parallelism saved. Index the results instead of streaming
  them if order matters.
- **Watch:** the merge channel must be closed exactly once, by the party that
  knows all producers finished.

### Pipeline

- **Intent:** stages, each transforming a stream, running concurrently.
- **Use when:** stages have genuinely different costs (I/O then CPU), so
  overlapping them helps.
- **Do NOT use when:** it is three cheap map operations. A single loop is faster
  and debuggable.
- **Watch:** every stage needs the same cancellation signal, and every stage must
  drain on cancel or the upstream blocks forever.

### Semaphore

- **Intent:** cap concurrency without owning the spawn loop.
- **Use when:** callers spawn work you do not control, but a shared resource
  needs a ceiling.
- **Do NOT use when:** a worker pool already bounds the same resource — two
  independent limits on one resource is a misconfiguration waiting to happen.

---

## State sharing

Preference order, best first:

1. **Do not share.** Give each worker its own data; merge at the end.
2. **Ownership.** One goroutine/actor owns the state; everyone else sends it
   messages. All mutation is in one place, so all invariants are in one place.
3. **Immutable snapshot.** Publish a new value; readers hold the old one safely.
   Good for config and lookup tables read far more than written.
4. **Mutex.** Correct, but it puts the invariant in the programmer's head instead
   of the type system. Keep the critical section tiny, never call out to unknown
   code inside it, and never hold two locks without a documented order.

**Do NOT** reach for atomics to "optimise" a mutex without a benchmark showing
the mutex is the bottleneck. Atomic code is much easier to get subtly wrong.

---

## Failure of dependencies

### Timeout

- **Intent:** bound how long a call can take.
- **Use when:** always, on every network call, every lock acquisition that can
  block, every external process.
- **Do NOT use when:** — there is no "do not". A call without a deadline is a
  hang waiting for traffic.
- **Watch:** the deadline must be propagated (context/cancellation token), not
  re-invented per layer. Inner timeouts must be shorter than outer ones.

### Retry with backoff and jitter

- **Intent:** survive transient failure.
- **Use when:** the failure is genuinely transient (network blip, 503, deadlock
  retry) **and** the operation is idempotent.
- **Do NOT use when:** the operation is not idempotent, or the error is a client
  error (4xx, validation, auth). Retrying a 400 just multiplies the failure.
  Never retry without a cap.
- **Watch:** exponential backoff **with jitter**, always. Synchronised retries
  from many clients are how a recovering service is knocked back down. Budget the
  total retry time against the caller's deadline.

### Circuit breaker

- **Intent:** stop calling a dependency that is clearly down; fail fast instead.
- **Use when:** the dependency is remote, failures are correlated, and hammering
  it delays its recovery or exhausts our own threads/connections.
- **Do NOT use when:** there is exactly one caller and one call site with a short
  timeout — the timeout already bounds the damage. A breaker adds state,
  configuration, and a new failure mode (flapping) that must be tuned.
- **Watch:** define all three states (closed/open/half-open), what counts as a
  failure, and what callers get while open. An unmonitored breaker is worse than
  none, because outages become silent.

### Bulkhead

- **Intent:** isolate resource pools so one slow dependency cannot consume all
  capacity.
- **Use when:** several dependencies share a pool and one of them is known to be
  slow or unreliable.
- **Do NOT use when:** there is one dependency. Partitioning one thing into one
  partition is ceremony.

### Fallback / degraded response

- **Intent:** return something useful when the primary path fails.
- **Use when:** a stale or partial answer is genuinely acceptable to the caller —
  and you can say who decided that.
- **Do NOT use when:** correctness matters (money, auth, writes). A silent
  fallback that returns wrong data is worse than an error the caller can handle.
  Never fall back without logging and a metric.

---

## Flow control

### Bounded queue and backpressure

- **Intent:** make a fast producer slow down instead of exhausting memory.
- **Use when:** producer and consumer rates can diverge — which is always, given
  enough traffic.
- **Do NOT use when:** — unbounded queues are not an alternative, they are a
  deferred out-of-memory. Choose the bound deliberately and decide what happens
  when it is hit: block, drop, or reject.

### Load shedding

- **Intent:** reject work early when overloaded, to keep the rest healthy.
- **Use when:** the service must stay responsive for some traffic rather than
  fail for all of it.
- **Do NOT use when:** the work is not sheddable (a payment, a write). Shed reads
  before writes, and say so in the design.

### Rate limiting

- **Intent:** stay inside a downstream limit, or protect our own.
- **Use when:** a partner API has a documented quota, or an endpoint is abusable.
- **Watch:** limiting per-process when the limit is per-account silently breaks
  as soon as there are two replicas. Decide local vs distributed explicitly.

---

## Correctness under retry

Retries, at-least-once queues, and client double-submits all produce duplicates.
Design for it rather than hoping.

- **Idempotency key.** The caller supplies a key; the server stores the outcome
  against it and replays that outcome on a repeat. This is the general answer for
  externally-triggered writes.
- **Natural idempotency.** Prefer operations that are idempotent by construction:
  `SET status = x` over `INCREMENT`, upsert over insert, absolute values over
  deltas.
- **Dedup window.** Store seen ids for a bounded period. Cheaper than full
  idempotency, but only correct within the window — state the window.
- **Exactly-once delivery does not exist** across a network. What exists is
  at-least-once delivery plus idempotent processing. Any design that claims
  otherwise is hiding the dedup step.

---

## Lifecycle

### Cancellation

Propagate one cancellation signal from the entry point down. Every blocking
operation must select on it. Work that ignores cancellation is work that survives
its own request.

### Graceful shutdown

- Stop accepting new work first, then drain in-flight work with a deadline, then
  force-stop.
- The drain deadline must be shorter than the orchestrator's kill timeout, or the
  graceful path never runs.
- Flush what must not be lost (buffered writes, metrics, outbox) before exit.

### Durable work

- **Outbox:** write the DB row and the intent-to-publish in one transaction; a
  separate process publishes. This is the standard fix for "the DB commit
  succeeded but the event was never sent". See `architecture.md`.
- **Checkpointing:** long jobs record progress so a restart resumes rather than
  redoes. Only worth it when redoing is expensive or unsafe.
- **Do NOT** hold important work only in memory or only in an in-process channel
  if losing it on restart is unacceptable. That is the entire question to ask.
