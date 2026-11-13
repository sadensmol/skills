# Domain Model and Domain Boundaries

Read this when you add a value object, domain predicate or domain type, model optional or
exclusive state, or decide where a conversion or repository call belongs. Examples are
in Go; the rule is the same in any language.

## Rich Domain Models — bundle identity with its attributes (MUST FOLLOW)

When a concept has both an identity *and* intrinsic attributes, model it as **one
value object that carries both** — not a bare string/enum with the attributes
parked in a side map keyed by that string. The bare-identifier + lookup-map
split is a code smell: the attributes can drift from the identity, every reader
needs the map to make sense of a value, and "unknown key" error paths leak
everywhere.

Compose sub-value-objects rather than flattening their fields in — a size is its
own concept, so the variant *has a* `Size`, it doesn't sprout `Width`/`Height`.

```go
// Bad — identity is a bare string; attributes live in a parallel map.
type ImageVariant string
const ImageVariantSquare ImageVariant = "square"

type ImageSize struct{ Width, Height int }
var imageTargetSizes = map[ImageVariant]ImageSize{   // ❌ side map keyed by the string
    ImageVariantSquare: {575, 575},
}
func (v ImageVariant) TargetSize() (ImageSize, bool) { s, ok := imageTargetSizes[v]; return s, ok }
// every caller resizes via a map lookup + ok-check; size can drift from name.

// Good — one self-describing value object; size is a composed sub-object.
type ImageSize struct {
    Width  int
    Height int
}
type ImageVariant struct {
    Name string
    Size ImageSize          // composed, not flattened to Width/Height
}
var ImageVariantSquare = ImageVariant{Name: "square", Size: ImageSize{Width: 575, Height: 575}}

func (v ImageVariant) String() string { return v.Name }   // Name is what URLs/keys use
```

Consequences that fall out of this and are part of the pattern:

- **Validation moves to the boundary.** Parse external input into the value
  object once, with a single `ByName`/`FromString` lookup that reports validity;
  past that point every value is guaranteed well-formed, so inner logic
  (`ResizeForVariant`) needs no "unknown variant" guard.

  ```go
  func ImageVariantByName(name string) (ImageVariant, bool) {
      switch name {
      case ImageVariantSquare.Name:     return ImageVariantSquare, true
      case ImageVariantWidescreen.Name: return ImageVariantWidescreen, true
      // …
      }
      return ImageVariant{}, false
  }
  // controller: variant, ok := domain.ImageVariantByName(name); if !ok { 400 }
  ```

- **Give it a `String() string` returning the identity field** (`Name`) so the
  value still formats as its identifier in URLs, Redis keys, and logs via `%s` —
  you keep the ergonomics of the old bare string with none of the drift.
- A struct value object is comparable, so it still works in `switch v { case X: }`
  and as a map/cache key.

Rule of thumb: if you catch yourself writing `map[SomeID]SomeAttrs` to look up
fixed, intrinsic attributes of `SomeID`, fold those attributes into the type
instead.

## Domain logic lives on the domain type, not a standalone helper (MUST FOLLOW)

A predicate or derivation computed purely from a domain type's own fields belongs
as a **method on that type**, not as a package-level function in the
service/usecase layer that takes the value as a parameter. A free `foo(x, …)` that
only reads `x`'s fields is misplaced behaviour — put it on `x`.

Benefits: the call site reads as a domain statement, the logic is unit-testable in
the domain package next to the type, and the caller's cyclomatic complexity drops
(one method call instead of an inlined multi-condition expression — often the
difference that keeps a function under the cyclomatic-complexity limit).

```go
// Bad — behaviour over Order's own fields parked as a service-layer helper.
func unpaidLongerThan(order *Order, payment *Payment, maxAge time.Duration) bool {
    anchor := order.CreatedAt
    if payment != nil {
        anchor = payment.CreatedAt
    }
    return time.Since(anchor) > maxAge
}
// caller: if order.CustomerID.IsTest() && unpaidLongerThan(order, payment, maxAge) { … }

// Good — a method on the domain type; the predicate is an Order capability.
func (o *Order) IsStaleUnpaidTestOrder(payment *Payment, maxAge time.Duration) bool {
    if !o.CustomerID.IsTest() {
        return false
    }
    anchor := o.CreatedAt
    if payment != nil {
        anchor = payment.CreatedAt
    }
    return time.Since(anchor) > maxAge
}
// caller: if order.IsStaleUnpaidTestOrder(payment, maxAge) { … }
```

Rule of thumb: if a helper's parameters are a domain value plus a couple of
constants and its body only reads that value's fields, it's a method on the type.

## Repository access respects domain boundaries (MUST FOLLOW)

A service owns the repositories (DAOs/storages) of **its own domain** and may
talk to them directly. It must **not** reach into a repository owned by
**another** domain — one a different service is responsible for. Depend on that
other **service** and go through it.

- **Same domain → repositories directly.** A service composing several
  repositories/storages that all belong to its own domain is fine and
  expected — that's what the service layer is for. Don't invent a wrapper
  service just to "go through a service"; same-domain direct access is correct.
- **Different domain → inject the owning service, never its repository.** When
  the data is managed by another service, that service is the only thing allowed
  to touch its repositories. Reaching past it into its DAO couples you to a
  schema you don't own, duplicates its invariants and error mapping, and rots
  silently when the owner changes its storage. Inject the service, call its
  methods.

Why: the owning service is where that domain's rules, error translation, and
invariants live. Bypassing it to hit the DAO scatters those rules across
callers and breaks the moment the owner changes how it persists.

```go
// Bad - BackupService (backup domain) reaches into repositories owned by
// the media and account-limit domains.
type BackupService struct {
    passRepo      PassRepository       // own domain  - ok
    deletionRepo  DeletionRepository   // own domain  - ok
    attachmentDAO MediaAttachmentDAO   // ❌ media domain  - owned by MediaService
    limitDAO      AccountLimitDAO      // ❌ limits domain - owned by AccountLimitService
}

// Good - own-domain repos stay direct; other domains come through their service.
type BackupService struct {
    passRepo     PassRepository       // own domain  - direct
    deletionRepo DeletionRepository   // own domain  - direct
    mediaService MediaService         // ✅ other domain - via its service
    limitService AccountLimitService  // ✅ other domain - via its service
}
```

If the owning service doesn't expose what you need, add a delegating method to it
(see *Service encapsulation* in SKILL.md) rather than reaching past it into its
repository.

## Types and Domain Modelling — full rules

The full text of SKILL.md → *Types and Domain Modelling*.

- **Domain types for identifiers and values** (`UserID`, `Email`, not raw `string` /
  `uuid`), so passing a `ProductID` where a `UserID` is expected does not compile. Call
  the type's accessor, never a raw cast; a missing accessor is added to the type.
- **Rich domain models (MUST FOLLOW):** a concept with identity + intrinsic attributes is
  one value object that carries both — not a bare identifier plus a `map[ID]Attrs` side
  table. Parse once at the boundary (`ByName`); give it a string form that returns the
  identity.
- **Domain logic on the domain type (MUST FOLLOW):** a predicate or derivation computed
  only from a type's own fields is a method on that type, not a service-layer helper.
- **Model mutually exclusive states as one sum type** (enum with associated values,
  sealed class, discriminated union), not a set of booleans and optionals. Switch over a
  closed set exhaustively, with no `default`, so a new case is a compile error.
- **Immutable by default.** Make a value mutable only when the code must mutate it.
- **Absence is not emptiness.** Return an empty collection or string for "nothing", never
  `null`/`nil`. An optional means the value is absent.
- **A value that is always present is non-optional** (a value type, not a pointer), so
  nil is impossible by construction and no guard is needed.
- **Check an optional before you dereference it (MUST FOLLOW).** Never assume "the caller
  always sets it" or "the middleware guarantees it". The function that dereferences owns
  the guard, at the point of access, not only upstream. Applies to pointer fields,
  elements of collections of pointers, and optional scalars (or use a nil-safe accessor).
- **Validate at the boundary.** Parse external input into domain types once, where it
  enters; inner code trusts the types and needs no "unknown value" guard.
- **Narrowest visibility first.** Start private; widen only when a caller outside needs
  it. A library exposes the minimum public surface.

## Layers — full rules

The full text of SKILL.md → *Layers*.

- Calls go one way: transport (controller/handler) → usecase (optional) → service →
  repository; domain models are shared by all. Never skip a layer, never call upward.
- **API, domain and persistence models never cross into another layer.** The controller
  converts API → domain right after it reads the request (domain constructors validate);
  services and repositories accept and return only domain objects.
- **Every type-to-type conversion goes through a named mapper** — `toDomain()`,
  `toAPI()`, `asModel()` (`fromDB()` as an alternative to `toDomain()`) — even a
  one-line conversion with one call site.
- **Repository access respects domain boundaries (MUST FOLLOW):** a service uses its own
  domain's repositories directly; another domain's data goes through that domain's
  service, never its repository.
- **A service knows nothing about its caller (MUST FOLLOW).** A service exposes plain
  actions on its own domain (`Get`, `Inc`, `Reset`). The usecase that owns the flow
  decides which action to call and keeps the case as a local value.

  ```text
  // Bad — the service switches on its caller's cases
  counterService.ApplyOrder(ctx, id, kind /* free | regular | promo */, paid)

  // Good — the usecase decides; the service only acts
  if kind == promo { counterService.Reset(ctx, id) } else if paid { counterService.Inc(ctx, id) }
  ```
- **Keep pure logic free of the host framework.** Model, parsing, validation and state
  transitions, routing live in a core that imports no UI or platform framework and is
  unit-tested without an app host. The app or transport layer stays a thin side-effect
  adapter: it only maps ids and applies effects.
