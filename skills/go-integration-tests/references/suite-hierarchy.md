# Test Suite Hierarchy

## Overview

Tests use a hierarchical suite structure with testify/suite, where each level adds specific functionality and dependencies.

## Hierarchy Levels

### Level 1: Base IntegrationTestSuite

Base suite (typically from a common/shared library) providing:
- App lifecycle management
- Database and cache setup (PostgreSQL, Redis, etc.)
- gRPC client connection provider (bufconn for mocking)
- Context management
- Standard test utilities

**You typically don't create this** - it's inherited from a common library or created once at the project level.

### The rule that makes the hierarchy work

Every level above the leaf is **setup and helpers only**. A `Test` method on a
shared level runs once per entry point that reaches it, so putting tests there
multiplies them silently. Concretely:

| Level | May declare `Test` methods? | `suite.Run` entry point? |
|---|---|---|
| Integration / common base | **No** | No |
| Application / service base | **No** | No |
| API-level suite | **No** | No |
| Feature suite (leaf) | **Yes** | **Exactly one** |

See "Never run the same suite twice" in SKILL.md for the failure this prevents —
83 tests that ran 249 times, tripled container time, and GC assertions that
failed only in full runs because sibling runs had seeded the shared stack.

### Level 2: Application/Service Base Suite

Application-level suite (e.g., `AppTestSuite`, `ServiceTestSuite`) providing:
- Application configuration
- App configurator setup
- Application-specific repositories
- Common test constants
- Helper methods for test data creation

**Location**: `tests/app-test-suite.go` or `tests/service-test-suite.go`

**Structure**:
```go
package tests

import (
    "yourproject/config"
    "yourproject/common/tests"  // Base test suite
    // other imports
)

const (
    // Application-specific test constants
    TestSomeValue = "test_value"
)

type AppTestSuite struct {
    tests.IntegrationTestSuite  // Base suite from common library
    Cfg           config.Config
    RequestSigner *utils.RequestSigner
    // Application-specific repositories and dependencies
}

func (s *AppTestSuite) Configure(a *app.App) error {
    // Configure ports using random unused ports
    grpcPort := testutils.GetRandomUnusedPort()
    httpPort := testutils.GetRandomUnusedPort()

    // Set up configuration
    s.Cfg = config.Config{
        GRPC: config.GRPC{Port: grpcPort},
        HTTP: config.HTTP{Port: httpPort},
        Database: config.Database{
            Host:     "localhost",
            Port:     5432,
            User:     "postgres",
            Password: "postgres",
            DBName:   "test_db",
            SSLMode:  "disable",
        },
        // ... other config
    }

    // Configure the app
    s.GRPCConfig = &s.Cfg.GRPC
    s.HTTPConfig = &s.Cfg.HTTP
    appConf := NewAppConfiguratorWithConfig(&s.Cfg)
    return appConf.ConfigureApp(a, s.GRPCClientConnectionProvider)
}

func (s *AppTestSuite) SetupSuite() {
    s.SetupSuiteWithConfigurator(s)

    // Set up application-specific dependencies
    dbConn, err := db.Setup(s.Cfg.Database)
    s.Require().NoError(err)

    // Initialize repositories
    // s.someRepository = repository.NewSomeRepository(dbConn)
}

// Helper methods
func (s *AppTestSuite) CreateTestData() {
    // Helper to create test data
}
```

### Level 3: API-Level Suite

API-level suite (e.g., `PrivateAPIV1TestSuite`, `PublicAPIV1TestSuite`, `APIV1TestSuite`) providing:
- gRPC/HTTP client setup for specific API
- Mock gRPC servers for dependencies
- API-specific test constants

**Location**: `tests/{api-type}/{version}/{api-type}_test.go` or `tests/api/v1/api_test.go`

**Structure**:
```go
package apiv1_test  // Note: separate package with _test suffix

import (
    "yourproject/tests"
    apiv1 "yourproject/api/v1"  // Your API package
    "github.com/stretchr/testify/suite"
    "google.golang.org/grpc"
)

const (
    // API-level constants
    TestExtSessionID = "test_ext_session_id"
)

type APIV1TestSuite struct {
    tests.AppTestSuite
    apiClient apiv1.ServiceClient

    // Mock servers for gRPC dependencies
    someMockSrv *tests.SomeMockServer
}

func (s *APIV1TestSuite) SetupSuite() {
    s.AppTestSuite.SetupSuite()

    // Create gRPC client
    conn, err := grpc.NewClient(
        fmt.Sprintf("localhost:%d", s.Cfg.GRPC.Port),
        grpc.WithTransportCredentials(insecure.NewCredentials()),
    )
    s.Require().NoError(err)
    s.apiClient = apiv1.NewServiceClient(conn)

    // Set up mock gRPC servers
    s.someMockSrv = &tests.SomeMockServer{}
    srv := grpc.NewServer()
    somepkgv1.RegisterServiceServer(srv, s.someMockSrv)

    go func() { _ = srv.Serve(s.GRPCClientConnectionProvider.Listener) }()
    time.Sleep(1 * time.Second)
}

func TestAPIV1TestSuite(t *testing.T) {
    suite.Run(t, &APIV1TestSuite{})
}
```

### Level 4: Feature Test Suite

Individual feature test suite (e.g., `BetTestSuite`, `WinTestSuite`, `UserTestSuite`) providing:
- Tests for a specific feature/endpoint
- Feature-specific repositories
- Feature-specific test data

**Location**: `tests/{api-type}/{version}/{feature}_test.go` or `tests/api/v1/{feature}_test.go`

**Structure**:
```go
package apiv1_test

import (
    "testing"
    "github.com/stretchr/testify/suite"
)

type FeatureTestSuite struct {
    APIV1TestSuite

    // Feature-specific repositories
    featureRepository *repository.Feature
}

func TestFeatureTestSuite(t *testing.T) {
    suite.Run(t, &FeatureTestSuite{})
}

func (s *FeatureTestSuite) SetupSuite() {
    s.APIV1TestSuite.SetupSuite()

    // Set up feature-specific dependencies
    db, err := db.Setup(s.Cfg.Database)
    s.Require().NoError(err)

    s.featureRepository = repository.NewFeature(db)
}

func (s *FeatureTestSuite) TestFeatureSuccess() {
    // Arrange
    testData := setupTestData()

    // Configure mock behavior
    s.someMockSrv.SomeMethodFunc = func(ctx context.Context, req *pkg.Request) (*pkg.Response, error) {
        // Validate request
        s.Require().Equal(expected, req.Field)
        return &pkg.Response{Data: "result"}, nil
    }

    // Act
    res, err := s.apiClient.Feature(s.Ctx, &apiv1.Request{
        Field: testData,
    })

    // Assert
    s.Require().NoError(err)
    s.Require().NotNil(res)
    s.Require().Equal(expected, res.Field)
}

func (s *FeatureTestSuite) TestFeatureError() {
    // Test error cases
}
```

## Key Patterns

### Package Naming
- Application base suite: `package tests`
- API/Feature suites: `package {apitype}{version}_test` (e.g., `apiv1_test`, `privateapiv1_test`, `publicapiv1_test`)

### Test Runner
Always include a test runner function at the API level:
```go
func TestSuiteNameHere(t *testing.T) {
    suite.Run(t, &SuiteNameHere{})
}
```

### SetupSuite Chain
Always call parent SetupSuite first:
```go
func (s *ChildSuite) SetupSuite() {
    s.ParentSuite.SetupSuite()
    // Child-specific setup
}
```

### Random Ports
Always use random ports for test services to avoid conflicts:
```go
grpcPort := testutils.GetRandomUnusedPort()
```
