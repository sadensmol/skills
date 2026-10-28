# Test Suite Hierarchy

Read this when you create a new suite type, an entry point (`suite.Run`), or change which suite a test lives on.

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

See "Never run the same suite twice" below for the failure this prevents.

### Level 2: Application/Service Base Suite

Application-level suite (e.g., `AppTestSuite`, `ServiceTestSuite`) providing:
- Application configuration
- App configurator setup
- Application-specific test repositories
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

    // Initialize TEST repositories only
    // s.someTestRepository = repository_test.NewSome(dbConn)
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

// no Test methods and no suite.Run here — only feature suites run
```

### Level 4: Feature Test Suite

Individual feature test suite (e.g., `BetTestSuite`, `WinTestSuite`, `UserTestSuite`) providing:
- Tests for a specific feature/endpoint
- Feature-specific test repositories
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

    // Feature-specific TEST repositories — never production ones
    featureTestRepository *repository_test.Feature
}

func TestFeatureTestSuite(t *testing.T) {
    suite.Run(t, &FeatureTestSuite{})
}

func (s *FeatureTestSuite) SetupSuite() {
    s.APIV1TestSuite.SetupSuite()

    // Set up feature-specific dependencies
    db, err := db.Setup(s.Cfg.Database)
    s.Require().NoError(err)

    s.featureTestRepository = repository_test.NewFeature(db)
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
Exactly one test runner function per feature (leaf) suite — never on a base or API-level suite:
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

## Never run the same suite twice

`suite.Run(t, new(X))` runs **every** `Test*` method on `X`, including the ones it
inherits from an embedded suite. So the number of times a test executes is the
number of entry points that reach its receiver type — not the number of times you
wrote it.

Two rules keep that at one:

1. **A shared/base suite declares no `Test` methods.** It holds `SetupSuite` /
   `SetupTest` and helpers. Tests live on the feature suites that embed it.
2. **Each suite type has exactly one `func TestXxxSuite(t *testing.T)`.**

**The failure mode, from a real repo.** One `BackupSuite` carried all 83 test
methods, and three files each ended with `suite.Run(t, new(BackupSuite))` — one
per "area" someone wanted to name. Result:

- **249 executions instead of 83.** Every assertion ran three times.
- **Triple the container time**, because each entry point runs `SetupSuite` and
  starts its own Postgres + LocalStack.
- **Cross-suite interference that looked like a product bug.** Each run seeded
  more rows into its stack and left background workers (a GC loop) sweeping
  shared storage. A GC test asserting "this orphan chunk was swept" failed
  depending on what a sibling run had seeded — passing in isolation, failing in
  a full run. Hours went into the "flaky GC" before the duplication was spotted.

```go
// Bad — three entry points on one type: every method runs three times
// backup_test.go
func TestBackupSuite(t *testing.T)          { suite.Run(t, new(BackupSuite)) }
// key_dao_test.go
func TestBackupKeyDAOSuite(t *testing.T)    { suite.Run(t, new(BackupSuite)) }   // ❌
// key_endpoints_test.go
func TestBackupKeyEndpointsSuite(t *testing.T) { suite.Run(t, new(BackupSuite)) } // ❌

// Good — base holds setup + helpers only; one suite and one entry point per feature
// suite.go
type BaseSuite struct{ tests.CloudSuite }                 // no Test methods, never run directly
func (s *BaseSuite) SetupTest()          { /* seed */ }
func (s *BaseSuite) startPass() string   { /* helper */ } // helpers on the base

// gc_test.go
type GCSuite struct{ BaseSuite }
func TestGCSuite(t *testing.T) { suite.Run(t, new(GCSuite)) }
func (s *GCSuite) TestGC_SweepsOrphans() { ... }

// key_dao_test.go
type KeyDAOSuite struct{ BaseSuite }
func TestKeyDAOSuite(t *testing.T) { suite.Run(t, new(KeyDAOSuite)) }
```

**Split by feature, not by file size.** One suite per role / feature / part of the
surface — `EndpointsSuite`, `PassLifecycleSuite`, `DeletionSuite`, `GCSuite`,
`StorageLimitSuite`, `KeyDAOSuite`, `KeyEndpointsSuite`, … A test belongs to the
suite for the thing it tests, not the file it was easiest to paste into.

**Check it cheaply.** These two greps should print one entry point per type and
nothing for the base:

```bash
grep -rn "^func Test.*Suite(t \*testing.T)" *_test.go   # one line per suite type
grep -c "func (s \*BaseSuite) Test" *.go                 # must be 0 everywhere
```

**The cost of doing it right.** Each suite runs its own `SetupSuite`, so N suites
means N container sets — in that repo, 3 suites at ~40s became 9 suites at ~125s,
of which ~11s per suite is container startup. That is the price of isolation and
it is usually worth paying: the shared-stack interference above is exactly what
per-suite containers prevent. If the wall-clock matters more than isolation,
share the containers behind a package-level `sync.Once` **inside `SetupSuite`**
(never a `TestMain` — see SKILL.md rule 10), and accept that suites then see each other's
rows.
