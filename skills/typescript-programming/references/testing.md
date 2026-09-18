# TypeScript Testing Reference

## Table of Contents
- [Framework Setup](#framework-setup)
- [Mocking Patterns](#mocking-patterns)
- [Async Testing](#async-testing)
- [Test Organization](#test-organization)
- [Snapshot Testing](#snapshot-testing)
- [Coverage and CI](#coverage-and-ci)

## Framework Setup

### Vitest Configuration

```typescript
// vitest.config.ts
import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    globals: true,
    environment: 'node', // or 'jsdom' for browser
    include: ['src/**/*.test.ts'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'lcov'],
      exclude: ['**/*.test.ts', '**/types.ts'],
    },
  },
});
```

### Jest Configuration

```typescript
// jest.config.ts
import type { Config } from 'jest';

const config: Config = {
  preset: 'ts-jest',
  testEnvironment: 'node',
  testMatch: ['**/*.test.ts'],
  collectCoverageFrom: ['src/**/*.ts', '!**/*.test.ts'],
  moduleNameMapper: {
    '^@/(.*)$': '<rootDir>/src/$1',
  },
};

export default config;
```

## Mocking Patterns

### Function Mocking

```typescript
// Vitest
import { vi, describe, it, expect } from 'vitest';

const mockFn = vi.fn<[string], number>();
mockFn.mockReturnValue(42);
mockFn.mockResolvedValue(42); // for async
mockFn.mockImplementation((s) => s.length);

// Jest
import { jest, describe, it, expect } from '@jest/globals';

const mockFn = jest.fn<(s: string) => number>();
mockFn.mockReturnValue(42);
```

### Module Mocking

```typescript
// Vitest - mock entire module
vi.mock('./database', () => ({
  getUser: vi.fn().mockResolvedValue({ id: '1', name: 'Test' }),
  saveUser: vi.fn().mockResolvedValue(true),
}));

// Vitest - partial mock
vi.mock('./utils', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./utils')>();
  return {
    ...actual,
    formatDate: vi.fn().mockReturnValue('2024-01-01'),
  };
});

// Jest equivalent
jest.mock('./database', () => ({
  getUser: jest.fn().mockResolvedValue({ id: '1', name: 'Test' }),
}));
```

### Spy on Methods

```typescript
// Vitest
const spy = vi.spyOn(console, 'log');
spy.mockImplementation(() => {}); // suppress output

// After test
spy.mockRestore();

// Jest
const spy = jest.spyOn(console, 'log');
```

### Timer Mocking

```typescript
// Vitest
vi.useFakeTimers();

it('debounces calls', async () => {
  const fn = vi.fn();
  const debounced = debounce(fn, 100);

  debounced();
  debounced();
  debounced();

  expect(fn).not.toHaveBeenCalled();

  await vi.advanceTimersByTimeAsync(100);

  expect(fn).toHaveBeenCalledTimes(1);
});

vi.useRealTimers();

// Jest
jest.useFakeTimers();
jest.advanceTimersByTime(100);
jest.useRealTimers();
```

### Dependency Injection for Testability

```typescript
// Production code - accept dependencies
interface Dependencies {
  db: Database;
  logger: Logger;
}

function createUserService({ db, logger }: Dependencies) {
  return {
    async createUser(input: CreateUserInput) {
      logger.info('Creating user', input);
      return db.users.create(input);
    },
  };
}

// Test code - inject mocks
it('creates user', async () => {
  const mockDb = { users: { create: vi.fn().mockResolvedValue({ id: '1' }) } };
  const mockLogger = { info: vi.fn() };

  const service = createUserService({ db: mockDb, logger: mockLogger });
  const result = await service.createUser({ name: 'Test' });

  expect(result.id).toBe('1');
  expect(mockLogger.info).toHaveBeenCalled();
});
```

## Async Testing

### Promise Testing

```typescript
it('resolves with data', async () => {
  const result = await fetchUser('1');

  expect(result.success).toBe(true);
  if (result.success) {
    expect(result.data.name).toBe('Test');
  }
});

it('rejects on network error', async () => {
  vi.mocked(fetch).mockRejectedValue(new Error('Network error'));

  const result = await fetchUser('1');

  expect(result.success).toBe(false);
});
```

### Testing Async Iterators

```typescript
it('paginates results', async () => {
  const mockFetcher = vi.fn()
    .mockResolvedValueOnce([1, 2, 3])
    .mockResolvedValueOnce([4, 5])
    .mockResolvedValueOnce([]);

  const results: number[] = [];
  for await (const item of paginate(mockFetcher)) {
    results.push(item);
  }

  expect(results).toEqual([1, 2, 3, 4, 5]);
  expect(mockFetcher).toHaveBeenCalledTimes(3);
});
```

### Timeout Testing

```typescript
it('times out after 5 seconds', async () => {
  vi.useFakeTimers();

  const promise = fetchWithTimeout('/slow', 5000);

  await vi.advanceTimersByTimeAsync(5000);

  const result = await promise;
  expect(result).toEqual({ success: false, error: 'TIMEOUT' });

  vi.useRealTimers();
});
```

## Test Organization

### Describe/It Structure

```typescript
describe('UserService', () => {
  // Shared setup
  let service: UserService;
  let mockDb: MockDatabase;

  beforeEach(() => {
    mockDb = createMockDb();
    service = new UserService(mockDb);
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  describe('createUser', () => {
    it('creates user with valid input', async () => { /* ... */ });
    it('returns error for invalid email', async () => { /* ... */ });
    it('returns error for duplicate email', async () => { /* ... */ });
  });

  describe('updateUser', () => {
    it('updates existing user', async () => { /* ... */ });
    it('returns not found for missing user', async () => { /* ... */ });
  });
});
```

### Test Data Builders

```typescript
// Builder for test data
function buildUser(overrides: Partial<User> = {}): User {
  return {
    id: '1',
    name: 'Test User',
    email: 'test@example.com',
    createdAt: new Date('2024-01-01'),
    ...overrides,
  };
}

// Usage
it('handles user with long name', () => {
  const user = buildUser({ name: 'A'.repeat(100) });
  // ...
});
```

### Parameterized Tests

```typescript
// Vitest
it.each([
  { input: '', expected: false },
  { input: 'invalid', expected: false },
  { input: 'test@example.com', expected: true },
])('validates email "$input" as $expected', ({ input, expected }) => {
  expect(isValidEmail(input)).toBe(expected);
});

// Jest - same syntax works
test.each`
  input              | expected
  ${''}              | ${false}
  ${'invalid'}       | ${false}
  ${'a@example.com'} | ${true}
`('validates email "$input" as $expected', ({ input, expected }) => {
  expect(isValidEmail(input)).toBe(expected);
});
```

## Snapshot Testing

### Object Snapshots

```typescript
it('generates correct user object', () => {
  const user = createUser({ name: 'Test', email: 'test@example.com' });

  expect(user).toMatchSnapshot();
});

// Inline snapshot (better for small objects)
it('generates correct user object', () => {
  const user = createUser({ name: 'Test', email: 'test@example.com' });

  expect(user).toMatchInlineSnapshot(`
    {
      "email": "test@example.com",
      "id": "generated-id",
      "name": "Test",
    }
  `);
});
```

### Snapshot Matchers for Dynamic Values

```typescript
it('creates user with generated id', () => {
  const user = createUser({ name: 'Test' });

  expect(user).toMatchSnapshot({
    id: expect.any(String),
    createdAt: expect.any(Date),
  });
});
```

## Coverage and CI

### Running Tests

```bash
# Vitest
npx vitest              # watch mode
npx vitest run          # single run
npx vitest --coverage   # with coverage

# Jest
npx jest                # single run
npx jest --watch        # watch mode
npx jest --coverage     # with coverage
```

### CI Configuration

```yaml
# GitHub Actions
- name: Run tests
  run: npm test -- --coverage

- name: Upload coverage
  uses: codecov/codecov-action@v3
```
