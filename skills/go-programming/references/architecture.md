# Architecture: Domain Objects, Mappers, Domain Boundaries

Read this when you convert between API / domain / DB types, add a domain value
object or domain predicate, or wire a service's repository dependencies.

## Domain Objects and the Mapper Pattern

**Golden rule:** pass only domain objects between layers. Convert at the
boundaries: the controller converts API → domain, the repository converts
domain ↔ DB. API, domain and persistence models never cross into another layer.

### Controller: API → domain → service → API

Convert input to domain objects right after binding. Domain constructors
validate, so conversion is validation.

```go
func (c *Controller) CreateOrder(ctx echo.Context) error {
    var req api.CreateOrderRequest
    if err := ctx.Bind(&req); err != nil {
        return domain.NewBadRequestError("invalid request", err)
    }

    customerID, err := domain.NewCustomerIDFromString(req.CustomerID)
    if err != nil {
        return domain.NewBadRequestError("invalid CustomerID", err)
    }
    productID, err := domain.NewProductIDFromString(req.ProductID)
    if err != nil {
        return domain.NewBadRequestError("invalid product ID", err)
    }

    order := domain.Order{
        CustomerID: customerID,
        Amount:     decimal.NewFromFloat(req.Amount),
        ProductID:  productID,
        CartID:     domain.CartID(req.CartID),
    }

    result, err := c.service.PlaceOrder(ctx.Request().Context(), order)
    if err != nil {
        return err
    }
    return ctx.JSON(http.StatusOK, domainOrderResult(result).toAPI())
}
```

### Mapper methods on type aliases

#### Repository: DB ↔ domain

```go
type (
    repositoryOrder model.Order
    domainOrder     domain.Order
)

func (r repositoryOrder) toDomain() *domain.Order {
    return &domain.Order{
        ID:         domain.OrderID(r.ID),
        CustomerID: domain.CustomerID(r.CustomerID),
        ExternalID: domain.ExternalID(r.ExternalID),
        Amount:     decimal.NewFromInt(r.AmountCents).Div(decimal.NewFromInt(100)),
        Currency:   domain.Currency(r.Currency),
        ProductID:  domain.ProductID(r.ProductID),
        CartID:     domain.CartID(r.CartID),
        Status:     domain.OrderStatus(r.Status),
        CreatedAt:  r.CreatedAt,
    }
}

func (d domainOrder) asModel() model.Order {
    return model.Order{
        ID:          uuid.UUID(d.ID),
        CustomerID:  d.CustomerID.UUID(),
        ExternalID:  d.ExternalID.String(),
        AmountCents: d.Amount.Mul(decimal.NewFromInt(100)).IntPart(),
        Currency:    d.Currency.String(),
        ProductID:   d.ProductID.String(),
        CartID:      d.CartID.String(),
        Status:      string(d.Status),
    }
}

func (r *Repository) Save(ctx context.Context, order domain.Order) error {
    return r.db.Insert(ctx, domainOrder(order).asModel())
}

func (r *Repository) FindByID(ctx context.Context, id domain.OrderID) (*domain.Order, error) {
    var m model.Order
    if err := r.db.QueryRow(ctx, "SELECT * FROM order WHERE id = $1", id).Scan(&m); err != nil {
        return nil, fmt.Errorf("query order: %w", err)
    }
    return repositoryOrder(m).toDomain(), nil
}
```

#### Controller: domain → API (single value and collection)

```go
type domainOrderResult domain.OrderResult

func (d domainOrderResult) toAPI() api.OrderResponse {
    return api.OrderResponse{
        OrderID:   d.OrderID.String(),
        Balance:   fmt.Sprintf("%.2f", d.Balance.InexactFloat64()),
        Currency:  d.Currency.String(),
        Status:    string(d.Status),
        Timestamp: d.CreatedAt.UTC().Format(time.RFC3339),
    }
}

type domainOrderSlice []domain.Order

func (d domainOrderSlice) toAPI() []api.OrderInfo {
    result := make([]api.OrderInfo, len(d))
    for i, order := range d {
        result[i] = domainOrder(order).toAPI()
    }
    return result
}

func (d domainOrder) toAPI() api.OrderInfo {
    return api.OrderInfo{
        ID:        d.ID.String(),
        Amount:    fmt.Sprintf("%.2f", d.Amount.InexactFloat64()),
        ProductID: d.ProductID.String(),
        CartID:    d.CartID.String(),
        Status:    string(d.Status),
    }
}

// controller: return ctx.JSON(http.StatusOK, domainOrderSlice(orders).toAPI())
```

#### Optional fields (pointers)

```go
type (
    repositoryRefund model.Refund
    domainRefund     domain.Refund
)

func (r repositoryRefund) toDomain() *domain.Refund {
    return &domain.Refund{
        ID:        domain.RefundID(r.ID),
        OrderID:   domain.OrderID(r.OrderID),
        Amount:    decimal.NewFromInt(r.AmountCents).Div(decimal.NewFromInt(100)),
        PromoID:   (*domain.PromoID)(r.PrID),   // pointer type cast
        ExtTxID:   (*domain.ExtTxID)(r.ExtTxID),
        ErrorInfo: r.ErrorCode.toErrorInfo(),   // optional nested struct
    }
}

func (d domainRefund) asModel() model.Refund {
    return model.Refund{
        ID:          uuid.UUID(d.ID),
        OrderID:     uuid.UUID(d.OrderID),
        AmountCents: d.Amount.Mul(decimal.NewFromInt(100)).IntPart(),
        PrID:        d.PromoID.StringPtr(), // nil-safe helper on the domain type
        ExtTxID:     d.ExtTxID.StringPtr(),
        ErrorCode:   d.ErrorInfo.toErrorCode(),
    }
}
```

### Standard method names

- `toDomain()` — any model → domain (DB → domain, API → domain)
- `toAPI()` — domain → API response
- `asModel()` — domain → DB persistence model
- `fromDB()` — alternative to `toDomain()` when it reads clearer

### Layer rules

- **Controller:** receives API models (generated from the OpenAPI spec), converts
  to domain immediately after binding (domain constructors validate), passes only
  domain objects down, converts results with `toAPI()`.
- **Service / usecase:** accepts and returns only domain objects. No knowledge of
  API or DB models.
- **Repository:** accepts and returns only domain objects. `asModel()` before
  persisting, `toDomain()` after reading. No knowledge of API models.

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
difference that keeps a function under the `gocyclo` limit).

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
(see **Service Encapsulation** in SKILL.md) rather than reaching past it into its
repository.
