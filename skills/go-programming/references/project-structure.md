# Go Project Structure

## Table of Contents
- Monorepo vs Multi-Repo
- Service Organization
- Layered Architecture
- Directory Patterns
- Database Organization
- Module System
- Build and Deployment

## Monorepo vs Multi-Repo

### Monorepo Structure

```
monorepo/
├── services/
│   ├── core/              # Core service
│   ├── product/           # Product-specific service
│   ├── integration/       # Integration service
│   └── bff/               # Backend-for-frontend
└── internal/              # Shared library
    ├── domain/
    ├── config/
    ├── app/
    ├── controller/
    ├── repository/
    └── tests/
```

### Multi-Repo Structure

```
org/
├── service-core/          # Core service
├── service-integration/   # Integration adapter
├── framework/             # Framework library
├── lib-common/            # Common library
├── lib-pricing/           # Domain library
└── e2e-tests/             # End-to-end tests
```

## Service Organization

### Full-Featured Service Structure

```
service/
├── cmd/
│   └── server/
│       └── main.go           # Entry point
├── internal/
│   ├── app.go                # App configuration and DI
│   ├── config/
│   │   └── config.go         # Service configuration
│   ├── controller/
│   │   └── v1/               # Versioned controllers
│   │       ├── public.go
│   │       ├── private.go
│   │       └── internal.go
│   ├── usecase/              # Application orchestration
│   │   ├── v1/               # Versioned usecases
│   │   ├── order.go
│   │   ├── refund.go
│   │   └── get-balance.go
│   ├── service/              # Domain business logic
│   │   ├── session.go
│   │   ├── settings.go
│   │   └── token.go
│   ├── repository/           # Data access layer
│   │   ├── session.go
│   │   ├── settings.go
│   │   └── user.go
│   └── domain/               # Domain models
│       ├── domain.go
│       ├── user.go
│       └── session.go
├── db/
│   ├── migrations/           # Database migrations
│   │   ├── 00001_create-triggers.sql
│   │   ├── 00002_create-users.sql
│   │   └── ...
│   ├── gen/                  # Generated query builders
│   │   └── main/
│   │       └── public/
│   │           ├── model/
│   │           └── table/
│   └── fixtures/             # Test data
│       └── test_data.sql
├── tests/
│   ├── service-test-suite.go   # Main integration suite
│   ├── public-api/
│   │   └── v1/
│   ├── private-api/
│   │   └── v1/
│   ├── internal-api/
│   │   └── v1/
│   └── repository/             # Test repositories
└── main.go
```

### Shared Library Structure

```
lib-common/
├── domain/
│   ├── error.go              # Domain error types
│   ├── user.go
│   └── currency.go
├── config/
│   └── config.go             # Shared config structs
├── app/
│   ├── app.go                # Application runner
│   ├── logger.go             # Logger initialization
│   └── tracer.go             # Observability setup
├── controller/
│   ├── error.go              # HTTP error handler
│   └── grpc-error.go         # gRPC error handler
├── repository/
│   ├── lock.go               # Distributed locking
│   └── cache.go
├── tests/
│   └── integration-test-suite.go  # Base test suite
└── utils/
    └── convertor.go
```

## Layered Architecture

### Clean Architecture Layers

All services follow Clean Architecture with clear separation:

```
Controller (Adapter)
    ↓
Usecase (Orchestration)
    ↓
Service (Domain Logic)
    ↓
Repository (Data Access)
```

### Layer Responsibilities

**Controller Layer** (`controller/v1/`):
- HTTP/gRPC request handling
- Request validation (before passing to usecase)
- Response mapping
- NO error wrapping (return as-is)
- NO error logging (middleware handles)

```go
// controller/v1/public.go
func (h *Handler) PostCreateItem(ctx echo.Context, params api.PostCreateItemParams) error {
    var req api.PostCreateItemJSONRequestBody
    err := ctx.Bind(&req)
    if err != nil {
        log.Error().Ctx(ctx.Request().Context()).Err(err).Msg("cannot bind request")
        return err
    }

    // Validation
    currency, err := domain.ParseCurrency(req.Currency)
    if err != nil {
        return domain.NewBadRequestError(fmt.Sprintf("currency %s not supported", req.Currency), err)
    }

    // Call usecase
    result, err := h.createItemUsecase.Execute(ctx.Request().Context(), data)
    return err  // Return as-is
}
```

**Usecase Layer** (`usecase/v1/`):
- Business workflow orchestration
- Coordinates multiple services
- Transaction boundaries
- Version-specific logic

```go
// usecase/process.go
func (u Process) Execute(ctx context.Context, d ProcessData) (domain.Result, error) {
    item, err := u.itemService.GetByID(ctx, d.ItemID)
    if err != nil {
        return domain.Result{}, err
    }

    // Orchestrate multiple services
    isValid, err := u.validationService.Check(ctx, item)
    if err != nil {
        log.Error().Ctx(ctx).Err(err).Msgf("validation check failed")
    }

    // Business workflow
    return result, nil
}
```

**Service Layer** (`service/`):
- Pure domain business logic
- Domain model operations
- No external dependencies

```go
// service/item.go
func (s *ItemService) GetByID(ctx context.Context, id domain.ItemID) (*domain.Item, error) {
    item, err := s.repo.FindByID(ctx, id)
    if err != nil {
        return nil, fmt.Errorf("failed to get item: %w", err)
    }
    return item, nil
}
```

**Repository Layer** (`repository/`):
- Database queries (type-safe query builders)
- Caching
- Specify root error cause

```go
// repository/item.go
func (r *ItemRepository) GetByID(ctx context.Context, id domain.ItemID) (*domain.Item, error) {
    tx, err := r.jetContext.GetDB(ctx)
    if err != nil {
        return nil, err
    }

    var items []model.Item

    // Type-safe query with partition pruning
    condition := table.Item.ID.EQ(postgres.UUID(id)).
        AND(table.Item.CreatedAt.GT(postgres.RawTimestampz("NOW() - INTERVAL '1 month'")))

    err = table.Item.SELECT(table.Item.AllColumns).
        WHERE(condition).
        Query(tx, &items)

    if err != nil {
        return nil, fmt.Errorf("failed to query item: %w", err)
    }

    if len(items) == 0 {
        return nil, nil
    }

    return items[0].toDomain(), nil
}
```

### Model Separation

Three distinct model types per layer:

```go
// API model (OpenAPI generated) - controller layer
package api
type CreateUserRequest struct {
    Name  string `json:"name"`
    Email string `json:"email"`
}

// Domain model - service/usecase layer
package domain
type User struct {
    ID        UserID
    Name      string
    Email     Email
    Balance   decimal.Decimal
    CreatedAt time.Time
}

// Persistence model - repository layer
package repository
type UserModel struct {
    ID        uuid.UUID  `db:"id"`
    Name      string     `db:"name"`
    Email     string     `db:"email"`
    Balance   int64      `db:"balance_cents"`
    CreatedAt time.Time  `db:"created_at"`
}
```

## Directory Patterns

### Naming Conventions

**File Naming**: Omit folder name from file names

```
controller/v1/user.go       # NOT user-controller.go
service/user.go             # NOT user-service.go
repository/user.go          # NOT user-repository.go

controller/v1/controller.go # Common utilities
```

**Package Naming**: Short version of folder path

```go
// controller/v1 → controllerv1
package controllerv1

// usecase/v1 → usecasev1
package usecasev1
```

### Versioning

Version controllers and usecases for API compatibility:

```
internal/
├── controller/
│   ├── v1/          # API v1.x.x
│   │   ├── user.go
│   │   └── item.go
│   └── v2/          # API v2.x.x (if needed)
└── usecase/
    ├── v1/
    └── v2/
```

## Database Organization

### Migrations and Fixtures

```
db/
├── migrations/
│   ├── 00001_create-triggers.sql  # Always first
│   ├── 00002_create-users.sql
│   ├── 00003_create-items.sql
│   └── ...
├── gen/                            # Generated query builders
│   └── main/
│       └── <schema>/
│           ├── model/              # Struct models
│           └── table/              # Query builders
└── fixtures/
    ├── users.sql
    └── items.sql
```

### Migration Rules

- **No down migrations** (leave empty)
- **First migration**: Create triggers for `updated_at`
- **Index changes on live tables: always `CONCURRENTLY`** — `CREATE INDEX` blocks
  writes, `DROP INDEX` blocks reads and writes. `CREATE/DROP INDEX CONCURRENTLY`
  is slower but takes no blocking lock. It cannot run inside a transaction, so mark
  the migration no-transaction (goose: `-- +goose NO TRANSACTION` as the first line).
  A failed concurrent build leaves an INVALID index — drop it concurrently before
  retrying; `IF NOT EXISTS` would silently skip it.
- **Every table must have**:
  - `created_at` timestamp
  - `updated_at` timestamp (auto-updated by trigger)
  - `deleted_at` timestamp (if soft delete needed)
  - Primary key (prefer `uuid`)
  - Foreign keys for relationships

### Table Naming

- Singular form: `user`, `item`, `transaction`
- Plural for collections: `promotions`
- Join tables: `user_roles`, `item_categories`

### Constraint Naming

```sql
-- Indexes
<table-name>_<row_names>_idx

-- Unique indexes
<table-name>_<row_names>_uq

-- Foreign keys
<table-name>_<row_names>_<ext_table_name>_<ext_row_name>_fk
```

### Partitioning

Heavy tables use partitioning with 3-month intervals:

```sql
select partman.create_parent(
    p_parent_table := 'public.transaction',
    p_control := 'created_at',
    p_interval := '3 months',
    p_premake := 3,
    p_start_partition := date_trunc('month', CURRENT_TIMESTAMP)::text
);
```

**Partition pruning strategy**:
- High-volume transactions: 1 month filter
- Session data: 1 day filter
- Historical data: 3 months filter
- Integration data: 3 months filter

Always add `created_at` filter for partition pruning:

```go
query := SELECT(...).
    WHERE(Transaction.ID.EQ(uuid(id))).
    AND(Transaction.CreatedAt.GT(time.Now().AddDate(0, -1, 0)))  // Partition pruning
```

## Module System

### Import Aliases

When importing similar packages, use numbered suffix for shared library:

```go
import (
    domain2 "github.com/org/lib-common/domain"
    config2 "github.com/org/lib-common/config"
    tests2 "github.com/org/lib-common/tests"

    domain "github.com/org/service-core/internal/domain"
    config "github.com/org/service-core/internal/config"
)

// Usage
user := domain.User{}                // Service domain
err := domain2.NewError("error", nil)    // Common lib
```

**Suffix rules**:
- Current service package: no suffix
- Common library: `2` suffix
- Additional imports: `3`, `4`, etc.

### Go Module Structure

```go
// go.mod
module github.com/org/service-core

go 1.21

require (
    github.com/org/lib-common v0.1.0
    github.com/labstack/echo/v4 v4.11.0
    github.com/go-jet/jet/v2 v2.11.0
    github.com/rs/zerolog v1.31.0
    github.com/shopspring/decimal v1.3.1
)
```

## Build and Deployment

### Makefile

```makefile
# Makefile
.PHONY: build test run clean

BINARY_NAME=service
MAIN_PATH=./cmd/server

build:
	go build -o bin/$(BINARY_NAME) $(MAIN_PATH)

test:
	go test -v ./...

test-integration:
	go test -v ./tests/...

run:
	go run $(MAIN_PATH)

clean:
	go clean
	rm -rf bin/

migrate:
	goose -dir db/migrations postgres "$(DB_URL)" up

fixtures:
	goose -dir db/fixtures postgres "$(DB_URL)" up
```

### Dockerfile (Multi-stage)

```dockerfile
# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /server ./cmd/server

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /server .

EXPOSE 8080

CMD ["./server"]
```

### Configuration

Environment-based configuration with struct tags:

```go
// config/config.go
type Config struct {
    HTTP          HTTPConfig
    PrivateHTTP   PrivateHTTPConfig
    GRPC          GRPCConfig
    Postgres      PostgresConfig
    PostgresRead  PostgresConfig
    Redis         RedisConfig
    OpenTelemetry OtelConfig
}

type PostgresConfig struct {
    Host           string `env:"POSTGRES_HOST"`
    Port           int    `env:"POSTGRES_PORT"`
    User           string `env:"POSTGRES_USER"`
    Password       string `env:"POSTGRES_PASSWORD"`
    DBName         string `env:"POSTGRES_DB_NAME"`
    MaxConnections int    `env:"POSTGRES_MAX_CONNECTIONS,default=10"`
}
```

## Test Organization

```
tests/
├── service-test-suite.go     # Main integration suite
├── public-api/
│   └── v1/
│       ├── create_test.go
│       └── get_test.go
├── internal-api/
│   └── v1/
│       ├── process_test.go
│       └── validate_test.go
└── repository/
    ├── item_test.go
    └── user_test.go
```

Each test suite extends base `IntegrationTestSuite` from shared library.
