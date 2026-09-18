# Go Concurrency Patterns

## Table of Contents
- Goroutines
- Channels
- Select Statement
- Common Patterns
- Synchronization Primitives
- Context for Cancellation
- Error Handling in Concurrent Code
- Best Practices

## Goroutines

### Basic Usage
```go
func main() {
    // Start goroutine
    go doWork()

    // Anonymous function
    go func() {
        fmt.Println("Anonymous goroutine")
    }()

    // With parameters
    go processData(data)

    time.Sleep(time.Second) // Wait for goroutines
}
```

### Don't Use Sleep to Wait
```go
// Bad
go doWork()
time.Sleep(time.Second) // Arbitrary wait

// Good - use WaitGroup
var wg sync.WaitGroup
wg.Add(1)
go func() {
    defer wg.Done()
    doWork()
}()
wg.Wait()
```

## Channels

### Creating Channels
```go
// Unbuffered channel
ch := make(chan int)

// Buffered channel
ch := make(chan int, 10)

// Receive-only
func reader(ch <-chan int) {
    val := <-ch
}

// Send-only
func writer(ch chan<- int) {
    ch <- 42
}
```

### Channel Operations
```go
// Send
ch <- value

// Receive
value := <-ch

// Receive with ok check
value, ok := <-ch
if !ok {
    // Channel closed
}

// Close channel
close(ch)

// Iterate over channel
for value := range ch {
    // Process value
    // Loop exits when channel closes
}
```

### Buffered vs Unbuffered
```go
// Unbuffered - blocks until receiver ready
ch := make(chan int)
ch <- 1 // Blocks until someone receives

// Buffered - blocks only when full
ch := make(chan int, 3)
ch <- 1 // Doesn't block
ch <- 2 // Doesn't block
ch <- 3 // Doesn't block
ch <- 4 // Blocks until someone receives
```

## Select Statement

### Basic Select
```go
select {
case msg := <-ch1:
    fmt.Println("Received from ch1:", msg)
case msg := <-ch2:
    fmt.Println("Received from ch2:", msg)
case ch3 <- value:
    fmt.Println("Sent to ch3")
}
```

### Select with Default
```go
select {
case msg := <-ch:
    fmt.Println("Received:", msg)
default:
    fmt.Println("No message received")
    // Non-blocking
}
```

### Select with Timeout
```go
select {
case result := <-ch:
    fmt.Println("Got result:", result)
case <-time.After(5 * time.Second):
    fmt.Println("Timeout!")
}
```

### Select with Context
```go
select {
case result := <-ch:
    return result, nil
case <-ctx.Done():
    return nil, ctx.Err()
}
```

## Common Patterns

### Worker Pool
```go
func workerPool(jobs <-chan Job, results chan<- Result, numWorkers int) {
    var wg sync.WaitGroup

    // Start workers
    for i := 0; i < numWorkers; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            for job := range jobs {
                result := process(job)
                results <- result
            }
        }(i)
    }

    // Close results when all workers done
    go func() {
        wg.Wait()
        close(results)
    }()
}

// Usage
func main() {
    jobs := make(chan Job, 100)
    results := make(chan Result, 100)

    workerPool(jobs, results, 5)

    // Send jobs
    go func() {
        for _, job := range allJobs {
            jobs <- job
        }
        close(jobs)
    }()

    // Collect results
    for result := range results {
        fmt.Println(result)
    }
}
```

### Fan-Out, Fan-In
```go
// Fan-out: distribute work to multiple goroutines
func fanOut(input <-chan int, numWorkers int) []<-chan int {
    outputs := make([]<-chan int, numWorkers)

    for i := 0; i < numWorkers; i++ {
        outputs[i] = worker(input)
    }

    return outputs
}

func worker(input <-chan int) <-chan int {
    output := make(chan int)
    go func() {
        defer close(output)
        for val := range input {
            output <- process(val)
        }
    }()
    return output
}

// Fan-in: combine multiple channels into one
func fanIn(channels ...<-chan int) <-chan int {
    var wg sync.WaitGroup
    output := make(chan int)

    // Start goroutine for each channel
    for _, ch := range channels {
        wg.Add(1)
        go func(c <-chan int) {
            defer wg.Done()
            for val := range c {
                output <- val
            }
        }(ch)
    }

    // Close output when all inputs done
    go func() {
        wg.Wait()
        close(output)
    }()

    return output
}
```

### Pipeline
```go
func generator(nums ...int) <-chan int {
    out := make(chan int)
    go func() {
        defer close(out)
        for _, n := range nums {
            out <- n
        }
    }()
    return out
}

func square(in <-chan int) <-chan int {
    out := make(chan int)
    go func() {
        defer close(out)
        for n := range in {
            out <- n * n
        }
    }()
    return out
}

func main() {
    // Build pipeline
    c := generator(2, 3, 4, 5)
    out := square(c)

    // Consume output
    for result := range out {
        fmt.Println(result)
    }
}
```

### Done Channel
```go
func worker(done <-chan struct{}) <-chan Result {
    results := make(chan Result)

    go func() {
        defer close(results)
        for {
            select {
            case <-done:
                return
            default:
                result := doWork()
                results <- result
            }
        }
    }()

    return results
}

func main() {
    done := make(chan struct{})
    results := worker(done)

    // Stop worker after 5 seconds
    time.AfterFunc(5*time.Second, func() {
        close(done)
    })

    for result := range results {
        fmt.Println(result)
    }
}
```

### Rate Limiting
```go
// Simple rate limiter using time.Ticker
func rateLimiter() {
    requests := make(chan Request, 100)
    limiter := time.Tick(100 * time.Millisecond) // 10 requests/second

    for req := range requests {
        <-limiter // Wait for rate limiter
        go handleRequest(req)
    }
}

// Bursty rate limiter
func burstyRateLimiter() {
    requests := make(chan Request, 100)
    limiter := make(chan struct{}, 3) // Allow burst of 3

    // Fill limiter
    for i := 0; i < 3; i++ {
        limiter <- struct{}{}
    }

    // Refill limiter
    go func() {
        ticker := time.NewTicker(200 * time.Millisecond)
        defer ticker.Stop()
        for range ticker.C {
            select {
            case limiter <- struct{}{}:
            default:
            }
        }
    }()

    for req := range requests {
        <-limiter
        go handleRequest(req)
    }
}
```

## Synchronization Primitives

### sync.Mutex
```go
type SafeCounter struct {
    mu    sync.Mutex
    count int
}

func (c *SafeCounter) Inc() {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.count++
}

func (c *SafeCounter) Value() int {
    c.mu.Lock()
    defer c.mu.Unlock()
    return c.count
}
```

### sync.RWMutex
```go
type Cache struct {
    mu    sync.RWMutex
    data  map[string]string
}

func (c *Cache) Get(key string) (string, bool) {
    c.mu.RLock() // Multiple readers allowed
    defer c.mu.RUnlock()
    val, ok := c.data[key]
    return val, ok
}

func (c *Cache) Set(key, value string) {
    c.mu.Lock() // Exclusive lock for writing
    defer c.mu.Unlock()
    c.data[key] = value
}
```

### sync.WaitGroup
```go
func processItems(items []Item) {
    var wg sync.WaitGroup

    for _, item := range items {
        wg.Add(1)
        go func(i Item) {
            defer wg.Done()
            process(i)
        }(item)
    }

    wg.Wait() // Block until all done
}
```

### sync.Once
```go
var (
    instance *Database
    once     sync.Once
)

func GetDatabase() *Database {
    once.Do(func() {
        instance = &Database{
            // Initialize once
        }
    })
    return instance
}
```

### sync.Cond
```go
type Queue struct {
    mu    sync.Mutex
    cond  *sync.Cond
    items []Item
}

func NewQueue() *Queue {
    q := &Queue{}
    q.cond = sync.NewCond(&q.mu)
    return q
}

func (q *Queue) Enqueue(item Item) {
    q.mu.Lock()
    defer q.mu.Unlock()

    q.items = append(q.items, item)
    q.cond.Signal() // Wake one waiter
}

func (q *Queue) Dequeue() Item {
    q.mu.Lock()
    defer q.mu.Unlock()

    for len(q.items) == 0 {
        q.cond.Wait() // Release lock and wait
    }

    item := q.items[0]
    q.items = q.items[1:]
    return item
}
```

### sync.Map
```go
var cache sync.Map

// Store
cache.Store("key", value)

// Load
if val, ok := cache.Load("key"); ok {
    // Use val
}

// LoadOrStore
actual, loaded := cache.LoadOrStore("key", value)

// Delete
cache.Delete("key")

// Range
cache.Range(func(key, value interface{}) bool {
    fmt.Println(key, value)
    return true // Continue iteration
})
```

## Context for Cancellation

### Basic Context Usage
```go
func doWork(ctx context.Context) error {
    for {
        select {
        case <-ctx.Done():
            return ctx.Err() // context.Canceled or context.DeadlineExceeded
        default:
            // Do work
        }
    }
}

func main() {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    go doWork(ctx)

    time.Sleep(5 * time.Second)
    cancel() // Signal cancellation
}
```

### Context with Timeout
```go
func fetchData(ctx context.Context) (Data, error) {
    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()

    resultCh := make(chan Data)
    errCh := make(chan error)

    go func() {
        data, err := fetch()
        if err != nil {
            errCh <- err
            return
        }
        resultCh <- data
    }()

    select {
    case data := <-resultCh:
        return data, nil
    case err := <-errCh:
        return Data{}, err
    case <-ctx.Done():
        return Data{}, ctx.Err()
    }
}
```

### Context with Deadline
```go
func process(ctx context.Context) error {
    deadline := time.Now().Add(10 * time.Second)
    ctx, cancel := context.WithDeadline(ctx, deadline)
    defer cancel()

    return doWork(ctx)
}
```

### Context with Values
```go
type contextKey string

const requestIDKey contextKey = "requestID"

func withRequestID(ctx context.Context, requestID string) context.Context {
    return context.WithValue(ctx, requestIDKey, requestID)
}

func getRequestID(ctx context.Context) string {
    if id, ok := ctx.Value(requestIDKey).(string); ok {
        return id
    }
    return ""
}
```

## Error Handling in Concurrent Code

### errgroup Package
```go
import "golang.org/x/sync/errgroup"

func processFiles(files []string) error {
    g, ctx := errgroup.WithContext(context.Background())

    for _, file := range files {
        file := file // Capture for goroutine
        g.Go(func() error {
            return processFile(ctx, file)
        })
    }

    // Wait for all goroutines, return first error
    return g.Wait()
}
```

### Collecting All Errors
```go
func processAll(items []Item) []error {
    var (
        mu     sync.Mutex
        errors []error
        wg     sync.WaitGroup
    )

    for _, item := range items {
        wg.Add(1)
        go func(i Item) {
            defer wg.Done()
            if err := process(i); err != nil {
                mu.Lock()
                errors = append(errors, err)
                mu.Unlock()
            }
        }(item)
    }

    wg.Wait()
    return errors
}
```

## Best Practices

### Avoid Goroutine Leaks
```go
// Bad - goroutine leak
func leak() <-chan int {
    ch := make(chan int)
    go func() {
        val := compute() // Long operation
        ch <- val // Blocks forever if nobody receives
    }()
    return ch
}

// Good - use buffered channel or context
func noLeak(ctx context.Context) <-chan int {
    ch := make(chan int, 1) // Buffered
    go func() {
        defer close(ch)
        val := compute()
        select {
        case ch <- val:
        case <-ctx.Done():
        }
    }()
    return ch
}
```

### Keep Critical Sections Small
```go
// Bad - holding lock too long
func (c *Cache) Process(key string) Result {
    c.mu.Lock()
    defer c.mu.Unlock()

    val := c.data[key]
    result := expensiveOperation(val) // Holding lock during expensive operation
    return result
}

// Good - minimize lock time
func (c *Cache) Process(key string) Result {
    c.mu.RLock()
    val := c.data[key]
    c.mu.RUnlock()

    result := expensiveOperation(val) // Lock released
    return result
}
```

### Close Channels from Sender
```go
// Good - sender closes
func producer() <-chan int {
    ch := make(chan int)
    go func() {
        defer close(ch) // Sender closes
        for i := 0; i < 10; i++ {
            ch <- i
        }
    }()
    return ch
}

// Consumer just receives
func consumer(ch <-chan int) {
    for val := range ch {
        process(val)
    }
}
```

### Use Buffered Channels Appropriately
```go
// Unbuffered - synchronization point
ch := make(chan int)

// Buffered - avoid goroutine blocking
ch := make(chan int, 100)

// Buffer size = number of workers (common pattern)
results := make(chan Result, numWorkers)
```

### Test Concurrent Code
```go
func TestConcurrentAccess(t *testing.T) {
    cache := NewCache()
    var wg sync.WaitGroup

    // Multiple concurrent writers
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func(val int) {
            defer wg.Done()
            cache.Set(fmt.Sprintf("key%d", val), val)
        }(i)
    }

    // Multiple concurrent readers
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func(val int) {
            defer wg.Done()
            _, _ = cache.Get(fmt.Sprintf("key%d", val))
        }(i)
    }

    wg.Wait()
}
```

### Race Detector
```bash
# Run with race detector
go test -race ./...
go run -race main.go
```
