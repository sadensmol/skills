# TypeScript Project Setup Reference

## Table of Contents
- [Project Structure](#project-structure)
- [tsconfig Options](#tsconfig-options)
- [Build Tools](#build-tools)
- [Linting Setup](#linting-setup)
- [Path Aliases](#path-aliases)

## Project Structure

### Standard Node.js Project

```
project/
├── src/
│   ├── index.ts          # Entry point, exports public API
│   ├── types.ts          # Shared type definitions
│   ├── utils/            # Utility functions
│   ├── services/         # Business logic
│   └── lib/              # Core library code
├── tests/                # Or co-locate as *.test.ts
├── dist/                 # Compiled output (gitignored)
├── package.json
├── tsconfig.json
└── vitest.config.ts      # Or jest.config.ts
```

### Library Project

```
my-library/
├── src/
│   ├── index.ts          # Public exports only
│   └── internal/         # Implementation details
├── dist/
│   ├── index.js          # CJS output
│   ├── index.mjs         # ESM output
│   └── index.d.ts        # Type declarations
├── package.json
└── tsconfig.json
```

### Package.json for Libraries

```json
{
  "name": "my-library",
  "version": "1.0.0",
  "type": "module",
  "main": "./dist/index.js",
  "module": "./dist/index.mjs",
  "types": "./dist/index.d.ts",
  "exports": {
    ".": {
      "import": "./dist/index.mjs",
      "require": "./dist/index.js",
      "types": "./dist/index.d.ts"
    }
  },
  "files": ["dist"],
  "scripts": {
    "build": "tsup src/index.ts --format cjs,esm --dts",
    "test": "vitest run",
    "lint": "eslint src --ext .ts",
    "typecheck": "tsc --noEmit"
  }
}
```

## tsconfig Options

### Recommended Base Configuration

```json
{
  "compilerOptions": {
    // Target and module
    "target": "ES2022",
    "module": "NodeNext",
    "moduleResolution": "NodeNext",
    "lib": ["ES2022"],

    // Strict type checking
    "strict": true,
    "noUncheckedIndexedAccess": true,
    "noImplicitReturns": true,
    "noFallthroughCasesInSwitch": true,
    "noImplicitOverride": true,
    "exactOptionalPropertyTypes": true,

    // Module interop
    "esModuleInterop": true,
    "allowSyntheticDefaultImports": true,
    "forceConsistentCasingInFileNames": true,
    "isolatedModules": true,

    // Output
    "outDir": "dist",
    "declaration": true,
    "declarationMap": true,
    "sourceMap": true,

    // Skip checking node_modules
    "skipLibCheck": true
  },
  "include": ["src"],
  "exclude": ["node_modules", "dist"]
}
```

### Key Options Explained

| Option | Purpose |
|--------|---------|
| `strict: true` | Enables all strict type checks |
| `noUncheckedIndexedAccess` | Array/object indexing returns `T \| undefined` |
| `exactOptionalPropertyTypes` | Distinguishes `{ a?: string }` from `{ a: string \| undefined }` |
| `isolatedModules` | Ensures compatibility with single-file transpilers |
| `skipLibCheck` | Speeds up compilation by skipping .d.ts checks |

### Frontend (React) Configuration

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "lib": ["DOM", "DOM.Iterable", "ES2022"],
    "module": "ESNext",
    "moduleResolution": "bundler",
    "jsx": "react-jsx",
    "strict": true,
    "noUncheckedIndexedAccess": true,
    "noEmit": true, // Bundler handles output
    "skipLibCheck": true
  }
}
```

## Build Tools

### tsup (Recommended for Libraries)

```bash
npm install -D tsup
```

```typescript
// tsup.config.ts
import { defineConfig } from 'tsup';

export default defineConfig({
  entry: ['src/index.ts'],
  format: ['cjs', 'esm'],
  dts: true,
  clean: true,
  sourcemap: true,
  minify: false, // or true for production
});
```

### esbuild (Fast, Low-Level)

```bash
npm install -D esbuild
```

```json
{
  "scripts": {
    "build": "esbuild src/index.ts --bundle --platform=node --outfile=dist/index.js"
  }
}
```

### tsc (Type Checking Only)

```json
{
  "scripts": {
    "typecheck": "tsc --noEmit",
    "build": "tsc"
  }
}
```

## Linting Setup

### ESLint with TypeScript

```bash
npm install -D eslint @typescript-eslint/parser @typescript-eslint/eslint-plugin
```

```javascript
// eslint.config.js (flat config)
import eslint from '@eslint/js';
import tseslint from 'typescript-eslint';

export default tseslint.config(
  eslint.configs.recommended,
  ...tseslint.configs.strictTypeChecked,
  {
    languageOptions: {
      parserOptions: {
        project: true,
        tsconfigRootDir: import.meta.dirname,
      },
    },
    rules: {
      '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_' }],
      '@typescript-eslint/explicit-function-return-type': ['error', {
        allowExpressions: true,
        allowTypedFunctionExpressions: true,
      }],
      '@typescript-eslint/no-floating-promises': 'error',
      '@typescript-eslint/no-misused-promises': 'error',
      '@typescript-eslint/prefer-nullish-coalescing': 'error',
      '@typescript-eslint/prefer-optional-chain': 'error',
    },
  },
  {
    ignores: ['dist/', 'node_modules/'],
  }
);
```

### Key ESLint Rules

| Rule | Purpose |
|------|---------|
| `no-floating-promises` | Requires handling or awaiting promises |
| `no-misused-promises` | Prevents passing promises where not expected |
| `explicit-function-return-type` | Requires return types on functions |
| `strict-boolean-expressions` | Prevents implicit boolean coercion |

## Path Aliases

### tsconfig.json

```json
{
  "compilerOptions": {
    "baseUrl": ".",
    "paths": {
      "@/*": ["src/*"],
      "@/utils/*": ["src/utils/*"]
    }
  }
}
```

### Vitest Path Resolution

```typescript
// vitest.config.ts
import { defineConfig } from 'vitest/config';
import { resolve } from 'path';

export default defineConfig({
  resolve: {
    alias: {
      '@': resolve(__dirname, 'src'),
    },
  },
});
```

### Jest Path Resolution

```javascript
// jest.config.js
module.exports = {
  moduleNameMapper: {
    '^@/(.*)$': '<rootDir>/src/$1',
  },
};
```

### Node.js (runtime resolution)

For Node.js to resolve aliases at runtime, use one of:

```bash
# tsx (recommended for development)
npx tsx src/index.ts

# ts-node with tsconfig-paths
npm install -D tsconfig-paths
node -r tsconfig-paths/register -r ts-node/register src/index.ts
```

Or configure Node.js subpath imports in package.json:

```json
{
  "imports": {
    "#*": "./src/*"
  }
}
```
