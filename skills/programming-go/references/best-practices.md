# API Standards

Read this when you design an API version or a response (status, time, money), or handle
money or validation at the API boundary.

## API Development Standards

### Versioning

Use semver for API versions: `v1.0.0`, `v2.0.0`. **Minor versions must be
backward-compatible**: the server handler supports the new version, the old
client code keeps working, and new request/response fields are optional.

### Money: cents inside, string at the boundary

API money is a **string** in the **main currency unit** (`"10.50"`, never `1050`).
Internally store **cents as `int64`** and convert only at the API boundary:

```go
type Balance struct {
    AmountCents int64
    Currency    domain.Currency
}

type BalanceResponse struct {
    Amount   string `json:"amount"`   // "10.50"
    Currency string `json:"currency"` // "USD"
}

amount := ConvertFromCurrencyCentsAsString(balance.AmountCents, balance.Currency)
response := BalanceResponse{Amount: amount, Currency: string(balance.Currency)}
```

Times: always `.UTC().Format(time.RFC3339)` — `Format(time.RFC3339)` alone may not be UTC.

### Request validation

All validation happens in the controller/handler, before the usecase is called:

```go
func (h *Handler) CreateUser(ctx echo.Context) error {
    var req api.CreateUserRequest
    if err := ctx.Bind(&req); err != nil {
        return domain.NewBadRequestError("Invalid request", err)
    }
    if req.Email == "" {
        return domain.NewBadRequestError("Email required", nil)
    }
    email := domain.Email(req.Email)
    if err := email.Validate(); err != nil {
        return domain.NewBadRequestError("Invalid email format", err)
    }
    return h.usecase.CreateUser(ctx.Request().Context(), email, req.Name)
}
```

## Naming repository finders

Language-agnostic: `sadensmol-programming` → `references/naming.md`.

## API Responses

The full text of SKILL.md → *API Responses*.

```go
// HTTP status - always 200 for success, never 201/204
return ctx.JSON(http.StatusOK, response)  // NOT http.StatusCreated
return ctx.NoContent(http.StatusOK)       // NOT http.StatusNoContent

// Times - always UTC in RFC3339
CreatedAt: user.CreatedAt.UTC().Format(time.RFC3339),

// Money - always a string in the main currency unit
Amount: "10.50",  // NOT 1050 cents
```
