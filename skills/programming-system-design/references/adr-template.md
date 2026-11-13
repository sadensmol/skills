# ADR Template

Use this template for Architecture Decision Records.

```markdown
# ADR-NNN: [Title]

## Context
[Describe the situation and why a decision is needed]

## Decision
[State the decision clearly]

## Consequences

### Positive
- [benefit 1]
- [benefit 2]

### Negative
- [drawback 1]
- [drawback 2]

### Alternatives Considered
- **[Option A]**: [why rejected]
- **[Option B]**: [why rejected]

## Status
[Proposed | Accepted | Deprecated | Superseded]

## Date
[YYYY-MM-DD]
```

## Example ADR

```markdown
# ADR-001: Use Redis for Session Storage

## Context
Need fast session lookups for authenticated users. Current PostgreSQL-based sessions add 50ms latency per request.

## Decision
Use Redis for session storage with 24-hour TTL.

## Consequences

### Positive
- Sub-millisecond session lookups
- Built-in TTL support
- Simple key-value model fits sessions well

### Negative
- Additional infrastructure component
- Data loss on Redis restart (acceptable for sessions)
- Memory cost scales with active users

### Alternatives Considered
- **PostgreSQL with caching**: added complexity for marginal gains
- **JWT tokens**: larger payload size and revocation complexity

## Status
Accepted

## Date
2025-01-15
```
