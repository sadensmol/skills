---
name: sadensmol-programming-api
description: "API design guidance: REST resources and paths, HTTP methods, status codes, error bodies, pagination, filtering, idempotency, concurrency, versioning and backward compatibility, plus OpenAPI / Swagger spec authoring and gRPC / protobuf service design. Use when (1) designing, changing or reviewing an API endpoint or contract, REST or gRPC, (2) writing or changing an OpenAPI or Swagger spec (`api.yaml`, `openapi.yaml`, `swagger.yaml`, `*.openapi.yaml`, `*.swagger.json`), (3) adding or editing an operation, request body, response, schema or property, or its `summary` / `description`, (4) reviewing an API or spec change, (5) changing a spec that feeds oapi-codegen or another client/server code generator, (6) writing or changing a gRPC service or a `.proto` file: RPCs, messages, enums, field numbers, status codes, deadlines, streaming, `buf` lint and breaking checks."
---

# API Design

**Precedence.** A project's own API guide wins over this skill. Read it first when the
project has one. This skill fills the gaps.

## Read on demand

**When a trigger below matches what you are about to do, read that file first** — before
you write, review or answer. The summary in this skill is not the full rule.

| When you… | Read |
|---|---|
| design or change a REST endpoint: path, method, status codes, errors, pagination, filters, idempotency, concurrency, versioning | `references/rest.md` |
| write, change or review an OpenAPI / Swagger spec: descriptions (one fact, one place), version, `operationId`, components, schemas, polymorphism, security, linting, breaking-change checks | `references/openapi.md` |
| design, change or review a gRPC service or a `.proto` file: naming, field numbers, enums, status codes, deadlines, retries, pagination, streaming, `buf` | `references/grpc.md` |

## Core rules

- **Any REST endpoint change: read `references/rest.md` first.** It holds paths,
  methods, status codes, errors, pagination and versioning.
- **Any gRPC service or `.proto` change: read `references/grpc.md` first.** It holds
  naming, compatibility, status codes, deadlines and `buf` checks.
- **Any OpenAPI / Swagger spec change: read `references/openapi.md` first.** It holds where
  each description goes (one fact, one place) and the spec tooling.

- Design the contract in the spec (OpenAPI or `.proto`) first. Generate the server and
  client code from it. Never hand-edit generated code.
- No field that is derivable from sibling fields of the same object (`available` when it
  equals `progress == maxProgress`). The client computes it.
- No request parameter that only echoes server-owned state back for a sync check. The
  server owns the state and decides alone.
- A released contract changes only in backward-compatible ways. A breaking change needs a
  new version (`references/rest.md` → *Compatibility and versioning* /
  `references/grpc.md` → *Compatibility*).
- Lint every spec change and diff it against the released spec for breaking changes
  (`references/openapi.md` → *Tooling*, `references/grpc.md` → *Tooling*).
