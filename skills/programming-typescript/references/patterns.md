# TypeScript Patterns Reference

## Table of Contents
- [Generic Patterns](#generic-patterns)
- [Conditional Types](#conditional-types)
- [Mapped Types](#mapped-types)
- [Type Guards and Narrowing](#type-guards-and-narrowing)
- [Error Handling](#error-handling)
- [Async Patterns](#async-patterns)

## Generic Patterns

### Constrained Generics

```typescript
// Constrain to objects with id
function getById<T extends { id: string }>(items: T[], id: string): T | undefined {
  return items.find(item => item.id === id);
}

// Constrain to keys of object
function pick<T, K extends keyof T>(obj: T, keys: K[]): Pick<T, K> {
  const result = {} as Pick<T, K>;
  keys.forEach(key => { result[key] = obj[key]; });
  return result;
}
```

### Generic Inference

```typescript
// Infer array element type
type ElementOf<T> = T extends readonly (infer E)[] ? E : never;
type Num = ElementOf<number[]>; // number

// Infer function return type
type ReturnOf<T> = T extends (...args: unknown[]) => infer R ? R : never;

// Infer Promise value
type Awaited<T> = T extends Promise<infer V> ? Awaited<V> : T;
```

### Builder Pattern with Generics

```typescript
class QueryBuilder<T extends Record<string, unknown>> {
  private filters: Partial<T> = {};

  where<K extends keyof T>(key: K, value: T[K]): this {
    this.filters[key] = value;
    return this;
  }

  build(): Partial<T> {
    return { ...this.filters };
  }
}

// Usage: new QueryBuilder<User>().where('name', 'John').where('age', 30).build()
```

## Conditional Types

### Distributive Conditionals

```typescript
// Distributes over union
type NonNullable<T> = T extends null | undefined ? never : T;
type Result = NonNullable<string | null>; // string

// Extract/Exclude
type Extract<T, U> = T extends U ? T : never;
type Exclude<T, U> = T extends U ? never : T;
```

### Template Literal Types

```typescript
type EventName<T extends string> = `on${Capitalize<T>}`;
type ClickEvent = EventName<'click'>; // 'onClick'

// Extract parts
type ExtractEventName<T> = T extends `on${infer E}` ? Uncapitalize<E> : never;
type Event = ExtractEventName<'onClick'>; // 'click'
```

### Recursive Conditionals

```typescript
// Deep readonly
type DeepReadonly<T> = T extends (infer E)[]
  ? ReadonlyArray<DeepReadonly<E>>
  : T extends object
    ? { readonly [K in keyof T]: DeepReadonly<T[K]> }
    : T;

// Deep partial
type DeepPartial<T> = T extends object
  ? { [K in keyof T]?: DeepPartial<T[K]> }
  : T;
```

## Mapped Types

### Key Remapping

```typescript
// Prefix keys
type Prefixed<T, P extends string> = {
  [K in keyof T as `${P}${Capitalize<string & K>}`]: T[K]
};

type User = { name: string; age: number };
type PrefixedUser = Prefixed<User, 'user'>; // { userName: string; userAge: number }

// Filter keys
type OnlyStrings<T> = {
  [K in keyof T as T[K] extends string ? K : never]: T[K]
};
```

### Getters/Setters

```typescript
type Getters<T> = {
  [K in keyof T as `get${Capitalize<string & K>}`]: () => T[K]
};

type Setters<T> = {
  [K in keyof T as `set${Capitalize<string & K>}`]: (value: T[K]) => void
};
```

## Type Guards and Narrowing

### Custom Type Guards

```typescript
// Type predicate
function isString(value: unknown): value is string {
  return typeof value === 'string';
}

// Discriminated union guard
function isSuccess<T>(result: Result<T>): result is { success: true; data: T } {
  return result.success;
}

// Array type guard
function isStringArray(arr: unknown[]): arr is string[] {
  return arr.every(item => typeof item === 'string');
}
```

### Assertion Functions

```typescript
function assertDefined<T>(value: T | undefined | null, message?: string): asserts value is T {
  if (value === undefined || value === null) {
    throw new Error(message ?? 'Value is not defined');
  }
}

// Usage
function processUser(user: User | undefined) {
  assertDefined(user, 'User not found');
  // TypeScript now knows user is User
  console.log(user.name);
}
```

### Exhaustive Checking

```typescript
function assertNever(value: never): never {
  throw new Error(`Unexpected value: ${value}`);
}

type Status = 'pending' | 'active' | 'done';

function handleStatus(status: Status): string {
  switch (status) {
    case 'pending': return 'Waiting...';
    case 'active': return 'In progress';
    case 'done': return 'Complete';
    default: return assertNever(status); // Compile error if case missed
  }
}
```

## Error Handling

### Result Type Pattern

```typescript
type Result<T, E = Error> =
  | { success: true; data: T }
  | { success: false; error: E };

// Factory functions
const ok = <T>(data: T): Result<T, never> => ({ success: true, data });
const err = <E>(error: E): Result<never, E> => ({ success: false, error });

// Usage
async function fetchUser(id: string): Promise<Result<User, 'NOT_FOUND' | 'NETWORK_ERROR'>> {
  try {
    const response = await fetch(`/users/${id}`);
    if (!response.ok) return err('NOT_FOUND');
    return ok(await response.json());
  } catch {
    return err('NETWORK_ERROR');
  }
}
```

### Error Type Unions

```typescript
// Typed error variants
type AppError =
  | { type: 'validation'; field: string; message: string }
  | { type: 'network'; status: number }
  | { type: 'auth'; reason: 'expired' | 'invalid' };

function handleError(error: AppError): void {
  switch (error.type) {
    case 'validation':
      console.log(`Invalid ${error.field}: ${error.message}`);
      break;
    case 'network':
      console.log(`Network error: ${error.status}`);
      break;
    case 'auth':
      console.log(`Auth failed: ${error.reason}`);
      break;
  }
}
```

## Async Patterns

### Typed Async Functions

```typescript
// Explicit Promise return type
async function fetchData<T>(url: string): Promise<Result<T>> {
  const response = await fetch(url);
  if (!response.ok) return err(new Error(`HTTP ${response.status}`));
  return ok(await response.json() as T);
}

// Async generator
async function* paginate<T>(
  fetcher: (page: number) => Promise<T[]>
): AsyncGenerator<T, void, unknown> {
  let page = 0;
  while (true) {
    const items = await fetcher(page++);
    if (items.length === 0) return;
    yield* items;
  }
}
```

### Cancellation Pattern

```typescript
async function fetchWithTimeout<T>(
  url: string,
  timeoutMs: number
): Promise<Result<T, 'TIMEOUT' | 'NETWORK'>> {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), timeoutMs);

  try {
    const response = await fetch(url, { signal: controller.signal });
    return ok(await response.json());
  } catch (e) {
    if (e instanceof Error && e.name === 'AbortError') return err('TIMEOUT');
    return err('NETWORK');
  } finally {
    clearTimeout(timeout);
  }
}
```
