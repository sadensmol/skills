# Actual Code Patterns from Repository

This file contains real-world code examples showing how patterns are implemented in production. These are not theoretical - they're from actual services.

## Table of Contents
- Error Handling Patterns
- Database Query Patterns
- Testing Patterns
- Concurrency Patterns
- Logging Patterns

## Error Handling Patterns

### Domain Error Structure

```go
// lib-common/domain/error.go
type Error struct {
    HttpStatus int           // http status code
    Code       *ErrorCode    // optional domain error code
    Message    string        // public error message
    Err        error         // wrapped error (internal details)
    Action     *ClientAction // client action if needed
}

// Constructor with method chaining
func NewForbiddenError(message string, err error) *Error {
    return &Error{HttpStatus: http.StatusForbidden, Message: message, Err: err}
}

func (e *Error) WithCode(code ErrorCode) *Error {
    e.Code = &code
    return e
}
```

### Usecase Layer Error Handling

```go
// service/internal/usecase/process.go
func (u Process) Execute(ctx context.Context, d ProcessData) (domain.Result, error) {
    // Get entity
    item, err := u.itemService.GetByID(ctx, d.ItemID)
    if err != nil {
        return domain.Result{}, err  // Propagate
    }
    if item == nil {
        return domain.Result{}, domain.NewNotFoundError("item not found", nil)
    }

    // Check validation with error chaining
    isValid, err := u.validationService.Check(ctx, item.ID)
    if err != nil {
        log.Error().Ctx(ctx).Err(err).Msgf("failed to validate item %s", item.ID)
    } else if !isValid {
        return domain.Result{}, domain.NewForbiddenError("item blocked", nil).
            WithCode(domain.ErrorCodeItemBlocked)
    }

    // Business validation
    if d.Amount <= 0 {
        return domain.Result{}, domain.NewBadRequestError("amount must be positive", nil)
    }

    // Call external service
    response, err := u.externalClient.Process(metadata.NewOutgoingContext(ctx, md), req)
    if err != nil {
        return domain.Result{}, err
    }

    return domain.Result{Value: response.Value}, nil
}
```

### HTTP Error Handler

```go
// lib-common/controller/error.go
func HandleHTTPError(err error, ctx echo.Context, responseMapper func(domain.Error) (any, error)) {
    code := http.StatusInternalServerError
    er := &domain.Error{}

    if he, ok := err.(*echo.HTTPError); ok {
        code = he.Code
        if msgStr, ok2 := he.Message.(string); ok2 {
            er.Message = msgStr
        }
    } else if errors.As(err, &er) {
        if er.HttpStatus != 0 {
            code = er.HttpStatus
        }
    } else {
        // Handle wrapped errors
        intErr := errors.Unwrap(err)
        if intErr != nil {
            er.Err = intErr
        }
    }

    errEr, err := responseMapper(*er)
    if err != nil {
        log.Ctx(ctx.Request().Context()).Error().Err(err).Msg("cannot map error to response")
        code = http.StatusInternalServerError
    }

    if !ctx.Response().Committed {
        err = ctx.JSON(code, errEr)
    }
}
```

### gRPC Error Handler

```go
// lib-common/controller/grpc-error.go
func HandleGRPCError(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
    res, err := handler(ctx, req)

    if err != nil {
        // Parse proxified errors
        dErr, err2 := domain.ParseError(err.Error())
        if err2 == nil {
            err = dErr
        }

        // Ensure domain error
        switch err.(type) {
        case domain.Error, *domain.Error:
        default:
            err = domain.NewError("", err)
        }
        log.Ctx(ctx).Error().Err(err).Msg("grpc error")
    }

    return res, err
}
```

## Database Query Patterns

### Type-Safe Query Builder (go-jet)

```go
// service/internal/repository/item.go
type ItemRepository struct {
    jetContext db.JetContext
}

func NewItemRepository(d db.IDB) *ItemRepository {
    return &ItemRepository{jetContext: db.NewJetContext(d)}
}

// INSERT
func (r *ItemRepository) Save(ctx context.Context, item *domain.Item) error {
    tx, err := r.jetContext.GetDB(ctx)
    if err != nil {
        return err
    }

    res, err := table.Item.INSERT(table.Item.AllColumns).
        MODEL(asModel(item)).
        Exec(tx)

    return err
}

// SELECT with partition pruning
func (r *ItemRepository) GetByID(ctx context.Context, id domain.ItemID) (*domain.Item, error) {
    tx, err := r.jetContext.GetDB(ctx)
    if err != nil {
        return nil, err
    }

    var items []model.Item

    // Type-safe condition with partition pruning
    condition := table.Item.ID.EQ(postgres.UUID(id)).
        AND(table.Item.CreatedAt.GT(postgres.RawTimestampz("NOW() - INTERVAL '1 month'")))

    err = table.Item.SELECT(table.Item.AllColumns).
        WHERE(condition).
        Query(tx, &items)

    if err != nil {
        return nil, err
    }

    if len(items) == 0 {
        return nil, nil
    }
    if len(items) > 1 {
        return nil, fmt.Errorf("GetByID returned non unique result")
    }

    return RepositoryModelItem(items[0]).toDomain(), nil
}
```

## Testing Patterns

### Suite-Based Integration Tests

```go
// service/tests/api/v1/process_test.go
type ProcessTestSuite struct {
    InternalAPIV1TestSuite  // Embedded base suite
    itemRepository     *repository.Item
    itemTestRepository *repository_test.Item
    configRepository   *repository.Config
}

func TestProcessTestSuite(t *testing.T) {
    suite.Run(t, &ProcessTestSuite{})
}

func (s *ProcessTestSuite) SetupSuite() {
    s.InternalAPIV1TestSuite.SetupSuite()

    d, err := db.Setup(s.Cfg.Postgres)
    s.Require().NoError(err)

    s.itemTestRepository = repository_test.NewItem(d)
    s.itemRepository = repository.NewItem(d)
    s.configRepository = repository.NewConfig(d)
}
```

### Table-Driven Tests

```go
// lib-common/domain/user_test.go
func TestGeneratePlayerName(t *testing.T) {
    tests := []struct {
        name              string
        userID            string
        expectedFirstName string
        expectedLastName  string
        expectedNumber    string
        expectedFullName  string
    }{
        {
            name:              "generates name with standard userID",
            userID:            "user_abc123",
            expectedFirstName: "Muscle",
            expectedLastName:  "Racer",
            expectedNumber:    "779",
            expectedFullName:  "MuscleRacer779",
        },
        {
            name:              "handles long userID",
            userID:            "very_long_user_id_with_many_characters_1234567890",
            expectedFirstName: "Brutal",
            expectedLastName:  "Engine",
            expectedNumber:    "860",
            expectedFullName:  "BrutalEngine860",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := GeneratePlayerName(tt.userID)
            assert.Equal(t, tt.expectedFirstName, result.FirstName)
            assert.Equal(t, tt.expectedLastName, result.LastName)
            assert.Equal(t, tt.expectedNumber, result.Number)
            assert.Equal(t, tt.expectedFullName, result.FullName())
        })
    }
}
```

### Integration Test with Mocking

```go
// service/tests/api/v1/process_test.go
func (s *ProcessTestSuite) TestSuccess() {
    ctx := context.Background()

    item, cleanupItem := s.CreateTestItem(ctx)
    defer cleanupItem()

    sessionID := s.CreateLocalSession(userID, false)
    defer func() {
        s.DeleteRound(sessionID)
        s.DeleteSession(sessionID)
    }()

    // Setup fixture data
    cfg := Config{
        ID:          ConfigID(uuid.New()),
        ItemID:      item.ID,
        SessionID:   SessionID(uuid.MustParse(sessionID)),
        CreatedAt:   time.Now(),
    }
    err := s.configRepository.SaveOrUpdate(ctx, cfg)
    s.Require().NoError(err)

    // Mock external service
    processCalled := false
    var capturedReq *api.ProcessRequest

    s.externalSrv.ProcessFunc = func(ctx context.Context, req *api.ProcessRequest) (*api.Response, error) {
        processCalled = true
        capturedReq = req
        return &api.Response{
            Value:  1000,
            Status: "success",
        }, nil
    }

    // Perform request
    s.PostProcess(sessionID, "1.00", targetValue, testutils.HTTPAssertion{
        Code: http.StatusOK,
    })

    // Verify
    s.Require().True(processCalled, "Process should have been called")
    s.Require().NotNil(capturedReq)
    s.Require().Equal(sessionID, capturedReq.SessionID)
}
```

### Error Parsing Tests

```go
// lib-common/domain/error_test.go
func (s *ErrorTestSuite) TestParseError() {
    tests := []struct {
        name    string
        errStr  string
        want    *Error
        wantErr bool
    }{
        {
            name:   "valid error string",
            errStr: "[500]test message(test error)",
            want: &Error{
                HttpStatus: http.StatusInternalServerError,
                Message:    "test message",
                Err:        errors.New("test error"),
            },
            wantErr: false,
        },
        {
            name:   "valid error string with code",
            errStr: "[500|item_not_found]test message(test error)",
            want: &Error{
                HttpStatus: http.StatusInternalServerError,
                Code:       NewErrorCodePtr("item_not_found"),
                Message:    "test message",
                Err:        errors.New("test error"),
            },
            wantErr: false,
        },
        {
            name:   "full with action",
            errStr: "[400]some test message(some details)|some action type|some action data",
            want: &Error{
                HttpStatus: 400,
                Message:    "some test message",
                Err:        errors.New("some details"),
                Action: &ClientAction{
                    Type: ClientActionType("some action type"),
                    Data: conv.Pointer("some action data"),
                },
            },
            wantErr: false,
        },
    }

    for _, tt := range tests {
        s.Run(tt.name, func() {
            got, err := ParseError(tt.errStr)
            if (err != nil) != tt.wantErr {
                s.T().Errorf("ParseError() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if !got.Equal(tt.want) {
                s.T().Errorf("ParseError() got = %v, want %v", got, tt.want)
            }
        })
    }
}
```

## Concurrency Patterns

### WaitGroup and Goroutine Coordination

```go
// lib-common/app/app.go
func (a *App) start() {
    numToStart := len(a.servers)
    a.startErrCh = make(chan error, numToStart)

    for _, server := range a.servers {
        a.wg.Add(1)

        go func(server IServer) {
            defer a.wg.Done()
            a.startErrCh <- server.start()
        }(server)
    }
}
```

### Context Cancellation

```go
// lib-common/app/app.go
func (a *App) wait() error {
    sig := make(chan os.Signal, 1)
    signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

    wgDoneCh := make(chan struct{})
    go func() {
        a.wg.Wait()
        close(wgDoneCh)
    }()

    select {
    case <-sig:
        log.Info().Msg("received signal, shutting down...")
    case <-wgDoneCh:
        log.Info().Msg("all servers stopped")
    case err := <-a.startErrCh:
        log.Error().Err(err).Msg("server stopped with error")
        return err
    case <-a.Ctx.Done():
        log.Info().Msg("context done, shutting down...")
    }

    return nil
}
```

### Distributed Locking with Polling

```go
// lib-common/repository/lock.go
func (r *Lock) TryLockForDuration(
    ctx context.Context,
    key string,
    ttl, tryFor time.Duration,
    pollInterval ...time.Duration,
) (*redsync.Mutex, error) {
    interval := defaultLockPollInterval
    if len(pollInterval) > 0 && pollInterval[0] > 0 {
        interval = pollInterval[0]
    }

    m := r.rs.NewMutex(key, redsync.WithExpiry(ttl))

    ticker := time.NewTicker(interval)
    defer ticker.Stop()
    timeout := time.NewTimer(tryFor)
    defer timeout.Stop()

    for {
        if err := m.TryLockContext(ctx); err == nil {
            if ctx.Err() != nil {
                _, _ = m.Unlock()
                return nil, ctx.Err()
            }
            return m, nil
        }

        select {
        case <-ctx.Done():
            return nil, ctx.Err()
        case <-timeout.C:
            return nil, redsync.ErrFailed
        case <-ticker.C:
            // Continue
        }
    }
}
```

### Background Service with Panic Recovery

```go
// lib-common/service/background-processor.go
type BackgroundProcessor struct {
    ctx          context.Context
    cancel       context.CancelFunc
    repository   IRepository
    client       ServiceClient
    locker       ILocker
    ticker       *time.Ticker
    requestCh    chan Request
}

func NewBackgroundProcessor(
    ctx context.Context,
    client ServiceClient,
    repository IRepository,
    locker ILocker,
) *BackgroundProcessor {
    serviceCtx, cancel := context.WithCancel(ctx)
    return &BackgroundProcessor{
        ctx:        serviceCtx,
        cancel:     cancel,
        repository: repository,
        client:     client,
        ticker:     time.NewTicker(processPeriod),
        requestCh:  make(chan Request, maxRequests),
        locker:     locker,
    }
}

func (s BackgroundProcessor) Start() error {
    go func() {
        defer func() {
            if r := recover(); r != nil {
                log.Error().Msgf("recovered from panic: %v (%s)", r, debug.Stack())
                err := s.Start()
                log.Error().Err(err).Msg("cannot restart service")
            }
        }()

        for {
            select {
            case <-s.ctx.Done():
                s.ticker.Stop()
                return
            case req := <-s.requestCh:
                s.processRequest(s.ctx, req)
            case <-s.ticker.C:
                s.process(s.ctx)
            }
        }
    }()
    return nil
}

func (s BackgroundProcessor) Stop() error {
    s.cancel()
    return nil
}
```

### Graceful Shutdown

```go
// lib-common/app/app.go
func (a *App) gracefulStop() error {
    gsCtx, cancel := context.WithTimeout(context.Background(), gracefulShutdownDelay)
    defer cancel()

    // Parallel server shutdown
    var serverWg sync.WaitGroup
    for _, server := range a.servers {
        serverWg.Add(1)
        go func(server IServer) {
            defer serverWg.Done()
            server.gracefulStop(gsCtx)
        }(server)
    }

    // Wait with timeout
    serverDone := make(chan struct{})
    go func() {
        serverWg.Wait()
        close(serverDone)
    }()

    select {
    case <-serverDone:
    case <-gsCtx.Done():
        return errors.New("graceful shutdown timeout")
    }

    return nil
}
```

## Logging Patterns

### Logger Initialization with OpenTelemetry

```go
// lib-common/app/logger.go
type otelLoggerHook struct{}

func (h otelLoggerHook) Run(e *zerolog.Event, level zerolog.Level, msg string) {
    ctx := e.GetCtx()
    if ctx == nil {
        return
    }

    span := trace.SpanFromContext(ctx)
    if span.SpanContext().IsValid() {
        e.Str("traceID", span.SpanContext().TraceID().String())
    }
}

func InitLogger(level zerolog.Level) {
    zerolog.SetGlobalLevel(level)
    logger := zerolog.New(os.Stdout).
        Hook(otelLoggerHook{}).
        With().
        Timestamp().
        Logger()
    log.Logger = logger
    log.Info().Msgf("logger initialized with level %s", level.String())
}
```

### Structured Logging Usage

```go
// Error logging with context
log.Error().Ctx(ctx).Err(err).Msgf("failed to validate item for user %s", userID)

// Info logging with structured fields
log.Info().Ctx(ctx).Msgf("got item: %s", itemID)

// Debug logging for flow
log.Debug().Ctx(ctx).Msgf("processing request for session %s", sessionID)
```

## Currency Handling

### Decimal Operations

```go
// lib-common/utils/currency.go
import "github.com/shopspring/decimal"

func ConvertStringToCents(strAmount string, currency Currency) (int64, error) {
    ci := GetCurrencyInfo(currency)
    if ci == nil {
        return 0, fmt.Errorf("cannot get currency information %s", currency)
    }

    amount, err := decimal.NewFromString(strAmount)
    if err != nil {
        return 0, fmt.Errorf("cannot convert string to decimal %s", strAmount)
    }

    amountCents := ci.ConvertDecimalToCents(amount)
    return amountCents, nil
}

func ConvertFromCentsAsString(amount int64, currency Currency) (string, error) {
    ci := GetCurrencyInfo(currency)
    if ci == nil {
        return "", fmt.Errorf("cannot get currency information %s", currency)
    }

    decPlaces := ci.DecimalPlaces
    if !ci.IsCrypto && decPlaces > 2 {
        decPlaces = 2
    }

    return decimal.NewFromInt(amount).
        Div(decimal.New(1, int32(decPlaces))).
        StringFixedBank(int32(decPlaces)), nil
}
```
