# Common Architecture Patterns

## Frontend Patterns

**Component Composition**: build complex UI from simple composable components

**Container/Presenter**: separate data fetching/logic from presentation

**Custom Hooks**: extract and reuse stateful logic

**Context for Global State**: avoid prop drilling for truly global state

**Code Splitting**: lazy load routes and heavy components

## Backend Patterns

**Repository Pattern**: abstract data access behind interfaces

**Service Layer**: encapsulate business logic separate from controllers

**Middleware Pattern**: chain request/response processing

**Event-Driven Architecture**: decouple components with async events

**CQRS**: separate read and write models for different optimization

## Retrieval / Search Patterns

**Retrieve then Rerank**: reranking only reorders candidates the retriever returned — it can never add one the retriever missed. A signal a different modality can supply (e.g. a caption naming an object the image classifier missed) must be a RETRIEVAL source, not just a reranker.

**Zero-shot via per-dimension softmax**: for sigmoid-contrastive zero-shot classifiers (e.g. SigLIP), pick tags with a softmax over each dimension's own labels, not an absolute probability threshold — absolute probs are tiny (~1e-4) and threshold nothing.

## Data Patterns

**Load-once heavy objects**: cache expensive-to-construct objects (ML model weights, connection pools) once and reuse — never rebuild per request; a per-request model reload can add seconds of latency.

**Normalized Database**: reduce redundancy with proper foreign keys

**Denormalized for Read Performance**: strategic duplication for query speed

**Event Sourcing**: store events not state for audit trail and replay

**Caching Layers**: Redis for hot data, CDN for static assets

**Eventual Consistency**: accept temporary inconsistency for availability

## API Patterns

**REST**: resource-oriented with HTTP verbs and status codes

**GraphQL**: flexible queries, single endpoint, schema-driven

**gRPC**: binary protocol for internal service communication

**Webhooks**: push-based notifications for async events

## Integration Patterns

**API Gateway**: single entry point with routing, auth, rate limiting

**Circuit Breaker**: fail fast when downstream services are unhealthy

**Retry with Backoff**: handle transient failures gracefully

**Saga Pattern**: manage distributed transactions across services

**Outbox Pattern**: reliable event publishing with database transactions
