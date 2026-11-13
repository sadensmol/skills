# Architecture: Domain Objects and Mappers

Read this when you convert between API / domain / DB types in Go.

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

## Rich domain models, domain logic, repository boundaries

These are language-agnostic: `sadensmol-programming` → `references/domain-model.md`.
