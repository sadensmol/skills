# Go Testing Patterns

## Table of Contents
- Test Structure and Organization
- Table-Driven Tests
- Subtests
- Test Helpers
- Mocking and Interfaces
- Benchmarking
- Test Coverage
- Integration Tests

## Test Structure and Organization

### File Naming
Test files end with `_test.go`:
```
user.go
user_test.go
```

### Package Naming
```go
// Same package - test private functions
package user

func TestPrivateFunction(t *testing.T) {
    // Can access private functions
}
```

```go
// External package - test public API only
package user_test

import "myapp/user"

func TestPublicAPI(t *testing.T) {
    // Can only access exported functions
}
```

### Test Function Naming
```go
func TestFunctionName(t *testing.T) { }
func TestStructName_MethodName(t *testing.T) { }
func BenchmarkFunctionName(b *testing.B) { }
func ExampleFunctionName() { }
```

## Table-Driven Tests

### Basic Pattern
```go
func TestAdd(t *testing.T) {
    tests := []struct {
        name string
        a    int
        b    int
        want int
    }{
        {name: "positive numbers", a: 2, b: 3, want: 5},
        {name: "negative numbers", a: -2, b: -3, want: -5},
        {name: "zero", a: 0, b: 5, want: 5},
        {name: "mixed", a: -2, b: 3, want: 1},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := Add(tt.a, tt.b)
            if got != tt.want {
                t.Errorf("Add(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
            }
        })
    }
}
```

### With Error Cases
```go
func TestDivide(t *testing.T) {
    tests := []struct {
        name    string
        a, b    float64
        want    float64
        wantErr bool
    }{
        {"normal", 10, 2, 5, false},
        {"divide by zero", 10, 0, 0, true},
        {"negative", -10, 2, -5, false},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := Divide(tt.a, tt.b)
            if (err != nil) != tt.wantErr {
                t.Errorf("Divide() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if got != tt.want {
                t.Errorf("Divide() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

### Complex Structs
```go
func TestUserService_CreateUser(t *testing.T) {
    tests := []struct {
        name    string
        input   *CreateUserRequest
        want    *User
        wantErr error
    }{
        {
            name: "valid user",
            input: &CreateUserRequest{
                Name:  "John Doe",
                Email: "john@example.com",
            },
            want: &User{
                ID:    "123",
                Name:  "John Doe",
                Email: "john@example.com",
            },
            wantErr: nil,
        },
        {
            name: "invalid email",
            input: &CreateUserRequest{
                Name:  "John Doe",
                Email: "invalid",
            },
            want:    nil,
            wantErr: ErrInvalidEmail,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            svc := NewUserService()
            got, err := svc.CreateUser(tt.input)

            if !errors.Is(err, tt.wantErr) {
                t.Errorf("CreateUser() error = %v, wantErr %v", err, tt.wantErr)
                return
            }

            if !reflect.DeepEqual(got, tt.want) {
                t.Errorf("CreateUser() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

## Subtests

### Sequential Subtests
```go
func TestUser(t *testing.T) {
    t.Run("creation", func(t *testing.T) {
        user := NewUser("john@example.com")
        if user.Email != "john@example.com" {
            t.Errorf("unexpected email: %s", user.Email)
        }
    })

    t.Run("validation", func(t *testing.T) {
        user := NewUser("invalid")
        if err := user.Validate(); err == nil {
            t.Error("expected validation error")
        }
    })
}
```

### Parallel Subtests
```go
func TestExpensiveOperations(t *testing.T) {
    t.Run("op1", func(t *testing.T) {
        t.Parallel() // Runs in parallel with other subtests
        // Long-running test
    })

    t.Run("op2", func(t *testing.T) {
        t.Parallel()
        // Another long-running test
    })
}
```

## Test Helpers

### Helper Functions
Mark helper functions with `t.Helper()`:
```go
func assertUser(t *testing.T, got, want *User) {
    t.Helper() // Test failures report caller's line number

    if got.ID != want.ID {
        t.Errorf("ID: got %s, want %s", got.ID, want.ID)
    }
    if got.Name != want.Name {
        t.Errorf("Name: got %s, want %s", got.Name, want.Name)
    }
}

func TestCreateUser(t *testing.T) {
    got := CreateUser("John")
    want := &User{ID: "1", Name: "John"}
    assertUser(t, got, want) // Failure points to this line
}
```

### Setup and Teardown
```go
func TestMain(m *testing.M) {
    // Setup
    setup()

    // Run tests
    code := m.Run()

    // Teardown
    teardown()

    os.Exit(code)
}

func setup() {
    // Initialize database, start services, etc.
}

func teardown() {
    // Clean up resources
}
```

### Per-Test Cleanup
```go
func TestWithCleanup(t *testing.T) {
    tmpFile, err := os.CreateTemp("", "test")
    if err != nil {
        t.Fatal(err)
    }

    t.Cleanup(func() {
        os.Remove(tmpFile.Name()) // Runs after test completes
    })

    // Use tmpFile
}
```

## Mocking and Interfaces

### Interface-Based Mocking
```go
// Production code
type UserRepository interface {
    FindByID(id string) (*User, error)
    Save(user *User) error
}

type UserService struct {
    repo UserRepository
}

// Test code
type mockUserRepository struct {
    users map[string]*User
}

func (m *mockUserRepository) FindByID(id string) (*User, error) {
    user, ok := m.users[id]
    if !ok {
        return nil, ErrNotFound
    }
    return user, nil
}

func (m *mockUserRepository) Save(user *User) error {
    m.users[user.ID] = user
    return nil
}

func TestUserService_GetUser(t *testing.T) {
    mockRepo := &mockUserRepository{
        users: map[string]*User{
            "1": {ID: "1", Name: "John"},
        },
    }

    svc := &UserService{repo: mockRepo}
    user, err := svc.GetUser("1")

    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if user.Name != "John" {
        t.Errorf("got name %s, want John", user.Name)
    }
}
```

### Mock with Call Tracking
```go
type mockRepository struct {
    findByIDCalled bool
    saveCalled     bool
    users          map[string]*User
}

func (m *mockRepository) FindByID(id string) (*User, error) {
    m.findByIDCalled = true
    return m.users[id], nil
}

func (m *mockRepository) Save(user *User) error {
    m.saveCalled = true
    m.users[user.ID] = user
    return nil
}

func TestUserService_UpdateUser(t *testing.T) {
    mock := &mockRepository{
        users: map[string]*User{
            "1": {ID: "1", Name: "John"},
        },
    }

    svc := &UserService{repo: mock}
    err := svc.UpdateUser("1", "Jane")

    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    if !mock.findByIDCalled {
        t.Error("expected FindByID to be called")
    }
    if !mock.saveCalled {
        t.Error("expected Save to be called")
    }
}
```

### Dependency Injection
```go
// Constructor injection
func NewUserService(repo UserRepository, logger *log.Logger) *UserService {
    return &UserService{
        repo:   repo,
        logger: logger,
    }
}

// Test
func TestUserService(t *testing.T) {
    mockRepo := &mockUserRepository{}
    testLogger := log.New(io.Discard, "", 0)

    svc := NewUserService(mockRepo, testLogger)
    // Test svc
}
```

## Benchmarking

### Basic Benchmark
```go
func BenchmarkAdd(b *testing.B) {
    for i := 0; i < b.N; i++ {
        Add(2, 3)
    }
}
```

### With Setup
```go
func BenchmarkMapAccess(b *testing.B) {
    m := make(map[string]int)
    for i := 0; i < 1000; i++ {
        m[fmt.Sprintf("key%d", i)] = i
    }

    b.ResetTimer() // Reset timer after setup

    for i := 0; i < b.N; i++ {
        _ = m["key500"]
    }
}
```

### Sub-Benchmarks
```go
func BenchmarkString(b *testing.B) {
    b.Run("concat", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            _ = "hello" + "world"
        }
    })

    b.Run("sprintf", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            _ = fmt.Sprintf("%s%s", "hello", "world")
        }
    })

    b.Run("builder", func(b *testing.B) {
        for i := 0; i < b.N; i++ {
            var sb strings.Builder
            sb.WriteString("hello")
            sb.WriteString("world")
            _ = sb.String()
        }
    })
}
```

### Memory Benchmarks
```go
func BenchmarkAllocation(b *testing.B) {
    b.ReportAllocs() // Report allocations

    for i := 0; i < b.N; i++ {
        s := make([]int, 100)
        _ = s
    }
}
```

Run with: `go test -bench=. -benchmem`

## Test Coverage

### Running Coverage
```bash
# Generate coverage
go test -coverprofile=coverage.out

# View coverage by function
go tool cover -func=coverage.out

# Generate HTML report
go tool cover -html=coverage.out
```

### Coverage in CI
```bash
# Fail if coverage below threshold
go test -cover -coverprofile=coverage.out
go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//' | \
    awk '{if ($1 < 80) exit 1}'
```

## Integration Tests

### Build Tags
```go
//go:build integration
// +build integration

package myapp_test

func TestDatabaseIntegration(t *testing.T) {
    // Integration test
}
```

Run with: `go test -tags=integration`

### Test Fixtures
```go
func setupTestDB(t *testing.T) *sql.DB {
    t.Helper()

    db, err := sql.Open("postgres", "postgres://localhost/testdb")
    if err != nil {
        t.Fatal(err)
    }

    t.Cleanup(func() {
        db.Close()
    })

    // Run migrations
    if err := runMigrations(db); err != nil {
        t.Fatal(err)
    }

    return db
}

func TestUserRepository_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test")
    }

    db := setupTestDB(t)
    repo := NewUserRepository(db)

    // Test with real database
}
```

Run without integration tests: `go test -short`

### Golden Files
```go
func TestGenerateHTML(t *testing.T) {
    got := GenerateHTML(&User{Name: "John"})

    goldenPath := "testdata/user.golden"

    if *update {
        os.WriteFile(goldenPath, []byte(got), 0644)
    }

    want, err := os.ReadFile(goldenPath)
    if err != nil {
        t.Fatal(err)
    }

    if got != string(want) {
        t.Errorf("output mismatch:\ngot:\n%s\nwant:\n%s", got, want)
    }
}
```

Run with: `go test -update` to update golden files

## Common Assertions

### Custom Assertion Helpers
```go
func assertEqual(t *testing.T, got, want interface{}) {
    t.Helper()
    if !reflect.DeepEqual(got, want) {
        t.Errorf("\ngot:  %+v\nwant: %+v", got, want)
    }
}

func assertError(t *testing.T, got, want error) {
    t.Helper()
    if !errors.Is(got, want) {
        t.Errorf("got error %v, want %v", got, want)
    }
}

func assertNoError(t *testing.T, err error) {
    t.Helper()
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
}
```

## Don't assert on exact error-message strings

Error *text* from the stdlib or third-party libraries is an implementation
detail that shifts with library version and with transport/middleware wrapping.
Asserting the full string makes tests brittle: a dependency change reworths the
message and the test fails even though behaviour is unchanged. Assert on
**semantics** (`errors.Is` / an error code / a stable substring), not the exact
message.

```go
// Bad — couples the test to stdlib phrasing
require.Equal(t, `Post "…": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`, err.Error())

// Good — assert the meaning
require.ErrorIs(t, err, context.DeadlineExceeded)
// or, when only the wrapped message is reachable, a stable fragment:
require.Contains(t, err.Error(), "Client.Timeout exceeded")
```

**Real gotcha — `otelhttp` changes HTTP timeout wording.** Wrapping a client's
transport with `otelhttp.NewTransport(...)` changes how `http.Client.Timeout`
surfaces: the error becomes `net/http: request canceled (Client.Timeout exceeded
while awaiting headers)` instead of `context deadline exceeded (Client.Timeout
exceeded …)`. The otelhttp wrapper is not a `*http.Transport`, so `net/http`
falls back to the legacy `req.Cancel` cancellation path instead of context
cancellation. Any test asserting the exact old string breaks the moment you add
otelhttp instrumentation — even though the timeout still fires identically and
downstream behaviour (retry/refund/etc.) is unchanged. Two takeaways:

- Adding OTel HTTP instrumentation to an existing client is a **behaviour change
  to error text** — expect brittle timeout assertions to need updating.
- Enforcing the timeout via a **context deadline** (`context.WithTimeout` on the
  request) instead of `http.Client.Timeout` keeps the `context deadline exceeded`
  wording even through otelhttp — but the durable fix is to not assert the exact
  string in the first place.
