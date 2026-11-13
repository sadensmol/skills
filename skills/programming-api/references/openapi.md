# OpenAPI Specs

Read this when you write, change or review an OpenAPI / Swagger spec file (`api.yaml`,
`openapi.yaml`, `swagger.yaml`) or a spec that feeds a code generator.

A project's own API guide wins over this file.

## Descriptions: one fact, one place

Each description covers only the object it sits on.

| Object | Its description says | It never says |
|---|---|---|
| Operation (`summary`, `description`) | what the call does: its effect, side effects, preconditions, when each error happens | what a request or response field means, its range or its values |
| Response | when this status code is returned | what the fields of its body mean |
| Schema | what the object represents | what an operation that uses it does |
| Property | what this field means: units, range, the meaning of null or absent, each enum value | the operation's behaviour |

- Describe a field once, on its schema property. Every operation that uses the schema
  reaches that text through `$ref`.
- A reused object is a named schema under `components/schemas` with its own description.
  Do not inline the same object again in each operation.
- Put constraints in schema keywords (`required`, `minimum`, `maximum`, `enum`, `format`,
  `nullable`), not only in prose.
- In OpenAPI 3.0, tools ignore every key next to `$ref`, including `description`. Put
  the field's meaning on the referenced schema or property.
- Code generators such as oapi-codegen copy a property's description into the generated
  field comment. A field explained only in the operation text reaches no code.

## Example

```yaml
# Bad: the operation restates what the response fields mean
/orders/{id}/cancel:
  post:
    description: >-
      Cancels the order. The response `status` is `cancelled`, and `refund.amount`
      is the amount returned to the customer in minor units.

# Good: the operation says what it does; each field explains itself
/orders/{id}/cancel:
  post:
    description: Cancels an open order and starts the refund. A shipped order returns 409.
components:
  schemas:
    Refund:
      description: Money returned to the customer for one order.
      properties:
        amount:
          type: integer
          description: Refunded amount in minor units of the order currency.
```

## Description review check

- Read each operation and response description. Delete every sentence that explains a
  field. If the property does not say it yet, move the sentence there.
- Every new schema and property has its own description.

## Version

- Use the OpenAPI version that your code generator supports. oapi-codegen (built on
  kin-openapi) has historically supported only 3.0. Check its release notes before you
  move a spec to 3.1.
- Do not mix 3.0 and 3.1 syntax in one spec:

  | Topic | 3.0 | 3.1 |
  |---|---|---|
  | Nullable field | `nullable: true` | `type: [string, "null"]` |
  | Exclusive bound | `exclusiveMinimum: true` + `minimum: 0` | `exclusiveMinimum: 0` |
  | Schema example | `example: ...` | `examples: [...]` |
  | Keys next to `$ref` | ignored | allowed (JSON Schema 2020-12) |
  | Webhooks | not supported | top-level `webhooks` |

## Document structure

- Fill `info` (`title`, `version`, `description`) and `servers`. Bump `info.version` on
  every released change.
- Declare every tag at the root with a description. Give each operation one tag; tools
  group operations by it.
- Keep one source of truth. For a large API, split files by domain with relative `$ref`
  (`$ref: ./schemas/order.yaml`), and bundle them into one file for publishing and
  code generation.

## Operations

- Every operation has an `operationId`: unique, `camelCase`, verb plus noun (`listOrders`,
  `getOrder`, `createOrder`, `cancelOrder`). Generators use it as the function name, so a
  rename is a breaking change for generated clients.
- Every operation has a `summary` (a short title) and a `description` when it has
  behaviour to explain.
- Declare every response that the operation can return: each success code, each `4xx` it
  returns on purpose, and `default` for unexpected errors. Each one points to the shared
  error schema.
- Reuse common pieces from `components`: `parameters` (`Cursor`, `Limit`, `IdempotencyKey`),
  `responses` (`NotFound`, `Conflict`, `TooManyRequests`), `headers` (`RetryAfter`, `ETag`).
- Path parameters always have `required: true`. Give a query parameter a `default` in
  its schema when the server applies one.

## Schemas

- Name schemas in `PascalCase` nouns (`Order`, `OrderItem`). When the request and the
  response shapes differ, use separate schemas (`OrderCreate`, `OrderUpdate`, `Order`)
  rather than one schema full of optional fields.
- List every field that is always present in `required`. Generators make a required
  field a value and an optional one a pointer or an optional type, so a missing
  `required` leaks nil checks into every caller.
- Mark server-set fields `readOnly: true` (`id`, `createdAt`) and secrets
  `writeOnly: true` (`password`). Some generators ignore these keywords; then use separate
  request and response schemas.
- Give every field a `type` and, where it applies, a `format`: `int32` / `int64`,
  `date-time`, `date`, `uuid`, `email`, `uri`. Add `minLength` / `maxLength`, `minimum` /
  `maximum`, `maxItems` and an anchored `pattern` (`^[A-Z]{3}$`) for real limits.
- Money is `type: string` with a decimal `pattern`, plus a separate currency field. Never
  `type: number` (`references/rest.md` → *Field formats*).
- Use `enum` for a closed set of values. For a response enum that can grow, say in the
  description that clients must accept unknown values.
- `additionalProperties: false` rejects unknown fields. Use it on request schemas when
  the API promises strict input. Avoid it on response schemas: it makes every added
  response field a breaking change for validating clients.

## Composition and polymorphism

- Use `allOf` to extend a base schema.
- Use `oneOf` plus a `discriminator` (`propertyName` and an explicit `mapping`) for
  "one of these types". Every variant has the discriminator property in `required`.
- Avoid `anyOf`. Most generators produce weak types for it, and validation becomes
  ambiguous.
- Do not nest compositions deeply. Generated code for nested `oneOf` / `allOf` is hard to use.

## Examples

- Add an example for every request body and for the main success response. Use
  `examples` on the media type for several named cases (success, each error).
- Examples must pass validation against their schema. The linter checks this
  (see *Tooling* below).
- Use fake data only. Never put real tokens, keys or personal data in an example.

## Security

- Define each scheme once under `components/securitySchemes` (`http` bearer, `apiKey` in
  a header, `oauth2` with scopes).
- Set the default with root-level `security`. A public operation overrides it with
  `security: []`.
- Document the signature scheme of a signed API in the scheme's `description`: header
  name, algorithm, and the exact signed bytes.

## Code-generator extensions

- Keep vendor extensions (`x-...`) to what the generator needs. For oapi-codegen:
  `x-go-type` / `x-go-type-import` map a schema to an existing Go type, `x-go-name`
  renames a generated field, and `x-oapi-codegen-extra-tags` adds struct tags.
- Commit the generator config next to the spec. CI regenerates the code and fails when the
  result differs from the committed code.

## Tooling

- Lint every change: `spectral lint openapi.yaml` (Spectral) or `redocly lint openapi.yaml`
  (Redocly). Fail CI on errors.
- Check breaking changes against the released spec:
  `oasdiff breaking <released.yaml> <new.yaml> --fail-on ERR`. A breaking change goes in a
  new major version (`references/rest.md` → *Compatibility and versioning*).
- Bundle split files before publishing or generating: `redocly bundle openapi.yaml -o dist/openapi.yaml`.

## Review check

- Each operation has a unique `operationId`, a `summary`, one tag, and every response code
  it can return, including `default`.
- Each error response uses the shared error schema.
- Each list operation has pagination parameters and a maximum `limit`.
- Each always-present field is in `required`; server-set fields are `readOnly`.
- Money is a decimal string with a currency; timestamps are `format: date-time`.
- The lint is clean, and `oasdiff` reports no breaking change (or the version was bumped).
