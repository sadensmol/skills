---
name: sadensmol-typescript-programming
description: Expert TypeScript programming guidance for writing idiomatic, type-safe code. Use when (1) Writing or reviewing TypeScript code, (2) Setting up TypeScript projects, (3) Implementing type patterns (generics, discriminated unions, utility types), (4) Writing tests with Vitest or Jest, (5) Refactoring TypeScript code, (6) Debugging type errors, (7) Designing type-safe APIs or data structures. Provides guidance on strict typing, functional patterns, modern idioms, and project configuration.
---

# TypeScript Programming

Expert guidance for writing idiomatic, type-safe TypeScript code with functional patterns and modern idioms.

## Core Principles

### Strict Typing

- Enable `strict: true` in tsconfig.json - never disable strict checks
- Avoid `any` - use `unknown` for truly unknown types, then narrow
- Prefer explicit return types on exported functions
- Use `as const` for literal types, avoid type assertions (`as`)

### Functional Patterns

- Prefer immutability: `readonly` arrays/objects, `Readonly<T>`, `ReadonlyArray<T>`
- Pure functions: same input → same output, no side effects
- Composition over inheritance: use functions and interfaces
- Avoid classes unless modeling stateful entities with clear lifecycles

### Modern Idioms

- Discriminated unions for state modeling
- Template literal types for string manipulation
- Satisfies operator for type checking without widening
- Optional chaining (`?.`) and nullish coalescing (`??`)

## Type Patterns

### Discriminated Unions (Preferred for State)

```typescript
// Good: Exhaustive, self-documenting, narrowable
type Result<T, E = Error> =
  | { success: true; data: T }
  | { success: false; error: E };

function handleResult<T>(result: Result<T>): T {
  if (result.success) {
    return result.data; // TypeScript knows data exists
  }
  throw result.error; // TypeScript knows error exists
}
```

### Branded Types (Type-Safe IDs)

```typescript
type UserId = string & { readonly __brand: unique symbol };
type OrderId = string & { readonly __brand: unique symbol };

const createUserId = (id: string): UserId => id as UserId;

// Prevents: assignOrderToUser(orderId, orderId) - compile error
function assignOrderToUser(userId: UserId, orderId: OrderId): void { }
```

### Utility Type Patterns

```typescript
// Pick only what you need
type CreateUserInput = Pick<User, 'name' | 'email'>;

// Make fields optional for updates
type UpdateUserInput = Partial<Pick<User, 'name' | 'email'>>;

// Require specific fields
type UserWithId = Required<Pick<User, 'id'>> & Partial<User>;
```

### See [references/patterns.md](references/patterns.md) for:
- Generic constraints and inference
- Conditional types
- Mapped types
- Type guards and narrowing
- Error handling patterns

## Testing

### Framework Setup

Both Vitest and Jest work well. Vitest is recommended for new projects (faster, native ESM).

```typescript
// Vitest
import { describe, it, expect, vi } from 'vitest';

// Jest
import { describe, it, expect, jest } from '@jest/globals';
```

### Test Structure

```typescript
describe('UserService', () => {
  describe('createUser', () => {
    it('creates user with valid input', async () => {
      const input = { name: 'Test', email: 'test@example.com' };
      const result = await userService.createUser(input);

      expect(result.success).toBe(true);
      if (result.success) {
        expect(result.data.name).toBe(input.name);
      }
    });

    it('returns error for duplicate email', async () => {
      // Arrange
      await userService.createUser({ name: 'First', email: 'dup@example.com' });

      // Act
      const result = await userService.createUser({ name: 'Second', email: 'dup@example.com' });

      // Assert
      expect(result.success).toBe(false);
    });
  });
});
```

### See [references/testing.md](references/testing.md) for:
- Mocking patterns (functions, modules, timers)
- Async testing
- Snapshot testing
- Test organization

## Project Setup

### Recommended tsconfig.json

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "NodeNext",
    "moduleResolution": "NodeNext",
    "strict": true,
    "noUncheckedIndexedAccess": true,
    "noImplicitReturns": true,
    "noFallthroughCasesInSwitch": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "outDir": "dist",
    "declaration": true
  },
  "include": ["src"],
  "exclude": ["node_modules", "dist"]
}
```

### See [references/project-setup.md](references/project-setup.md) for:
- Project structure patterns
- Build tool configuration (tsup, esbuild, tsc)
- Linting setup (ESLint + TypeScript)
- Path aliases

## Code Review Checklist

When reviewing TypeScript code, verify:

- [ ] No `any` types (use `unknown` and narrow)
- [ ] Explicit return types on exported functions
- [ ] Discriminated unions for complex state
- [ ] Immutable by default (`readonly`, `Readonly<T>`)
- [ ] Proper error handling (Result types or explicit throws)
- [ ] No type assertions (`as`) unless unavoidable
- [ ] Tests cover edge cases and error paths
