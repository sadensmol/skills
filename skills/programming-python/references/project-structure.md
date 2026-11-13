# Python Project Structure

Read this when you create a project, package or service, add a dependency, or set up
`pyproject.toml`, `ruff`, the type checker, a `Makefile` or a Dockerfile.

**Pick the layout first:**

- The repository holds only one Python package or service → *Single-package layout*.
- The repository holds a product with more than one part (a Python backend plus a
  frontend, docs, examples, deploy files, shipped skills…) → *Multi-component layout*.
  Its `backend/` then uses the single-package rules inside.

## Multi-component layout

```
myproject/
├── README.md               # what it is, quick start, links into docs/
├── AGENTS.md               # agent guide for the whole repo; CLAUDE.md points here
├── CONTRIBUTING.md
├── CHANGELOG.md
├── LICENSE
├── SECURITY.md
├── Makefile                # repo-wide entry points; delegates to each component
├── .env.example            # committed template; the real .env is gitignored
├── config.example.yaml     # committed template; the real config is gitignored
├── .pre-commit-config.yaml
├── .github/
│   ├── workflows/          # one CI workflow per component, path-filtered
│   └── ISSUE_TEMPLATE/
├── backend/                # Python — one uv workspace, one lock file
│   ├── pyproject.toml      # the app package + [tool.uv.workspace]
│   ├── uv.lock
│   ├── .python-version
│   ├── Makefile            # install / lint / test / dev for the backend only
│   ├── Dockerfile
│   ├── README.md
│   ├── AGENTS.md           # backend-specific agent rules
│   ├── app/                # runnable entry points: HTTP gateway, scheduler, workers
│   │   ├── gateway/
│   │   ├── scheduler/
│   │   └── workers/
│   ├── packages/           # importable libraries, each its own pyproject.toml
│   │   ├── core/
│   │   │   ├── pyproject.toml
│   │   │   └── src/myproject_core/   # domain / service / repository layers
│   │   └── sdk/
│   │       ├── pyproject.toml
│   │       └── src/myproject_sdk/    # the stable public API that examples import
│   ├── tests/
│   │   ├── unit/
│   │   ├── integration/
│   │   ├── fixtures/       # static test data
│   │   └── support/        # shared fakes and helpers
│   ├── scripts/            # backend-only dev and ops scripts (benchmarks, migrations)
│   └── docs/               # backend design notes for contributors
├── frontend/               # web UI — its own toolchain (package.json, lock file)
│   ├── src/
│   ├── public/
│   ├── tests/              # unit/ and e2e/
│   ├── scripts/
│   └── README.md
├── contracts/              # schemas shared across components (OpenAPI, JSON Schema)
├── docs/                   # user and developer docs
│   ├── guide/
│   ├── plans/              # design specs and implementation plans
│   └── images/
├── examples/               # each example is a standalone project
│   └── weather-agent/
│       ├── pyproject.toml  # depends on myproject-sdk only
│       ├── README.md
│       ├── src/weather_agent/
│       └── tests/
├── skills/                 # agent skills the product ships to its users
│   └── public/<skill-name>/SKILL.md
├── scripts/                # repo-wide scripts: setup wizard, release, doctor
├── docker/                 # images and compose files for supporting services
│   ├── nginx/
│   └── docker-compose.yaml
├── deploy/                 # deployment manifests (helm/, terraform/)
│   └── helm/myproject/
├── tests/                  # cross-component tests (end-to-end, skill checks)
└── .agent/skills/          # skills for agents that maintain this repo, not shipped
```

Create only the directories the project needs, but use these names for them. Each
top-level directory has one job:

| Directory | Holds | Never holds |
|---|---|---|
| `backend/` | all Python service code, its tests, lock file and Dockerfile | frontend code, deploy manifests |
| `frontend/` | the UI and its own toolchain | backend imports; it talks to the backend through `contracts/` |
| `contracts/` | API and data schemas that two components share | code |
| `docs/` | docs for users and contributors | generated API docs committed by hand |
| `examples/` | runnable, self-contained usage examples | code that `backend/` imports |
| `skills/` | skills the product ships | skills for maintaining this repo (→ `.agent/skills/`) |
| `scripts/` | repo-wide automation | component-only scripts (→ `<component>/scripts/`) |
| `docker/`, `deploy/` | images, compose, helm/terraform | application code |
| `tests/` | tests that span components | unit or integration tests of one component |

### The backend uv workspace

```toml
# backend/pyproject.toml
[project]
name = "myproject"
requires-python = ">=3.12"
dependencies = ["myproject-core", "myproject-sdk"]

[tool.uv.workspace]
members = ["packages/*"]

[tool.uv.sources]
myproject-core = { workspace = true }
myproject-sdk = { workspace = true }
```

- One workspace, one `uv.lock`, one `.python-version` for all backend packages. Run
  `uv sync` in `backend/` once; every package installs in editable mode.
- Each package under `packages/` has its own `pyproject.toml`, `src/<import_name>/`
  and `py.typed`. Name the distribution with hyphens (`myproject-core`) and the import
  package with underscores (`myproject_core`).
- Dependencies point one way: `app/` → `packages/`. `sdk` → `core`. A package never
  imports from `app/`, and `core` never imports `sdk`.
- `app/` holds only wiring and entry points (an app factory, a CLI, a worker loop). Logic
  lives in a package, so you can test it without starting the app.
- Shared tool config (`ruff`, `mypy`, `pytest`) lives once in `backend/pyproject.toml`
  (or `backend/ruff.toml`); packages do not repeat it.

### Examples

- Each example has its own `pyproject.toml` and depends on the public package (`sdk`)
  only — never on `core` internals. When an example needs an internal, the public API is
  missing something; add it to `sdk`.
- Each example has a `README.md` with the run command and at least one smoke test, and
  CI runs them, so an API change that breaks an example fails the build.

### Root Makefile

The root `Makefile` gives one entry point for the whole repo and delegates to the
components. It holds no build logic of its own:

```make
.PHONY: setup lint test dev

setup:
	$(MAKE) -C backend install
	cd frontend && pnpm install

lint:
	$(MAKE) -C backend lint
	cd frontend && pnpm lint

test:
	$(MAKE) -C backend test
	cd frontend && pnpm test

dev:
	docker compose -f docker/docker-compose.yaml up
```

### Agent guides

- Root `AGENTS.md`: what the repo is, the directory map above, and the commands
  (`make setup`, `make test`). `CLAUDE.md` holds one line that points to it.
- One `AGENTS.md` per component with the rules that apply only there. Keep each one
  short; link to `docs/` for long explanations.

## Single-package layout — always `src/`

```
myproject/
├── pyproject.toml          # metadata, dependencies, every tool's config
├── uv.lock                 # committed
├── .python-version         # pinned interpreter for uv
├── Makefile                # lint / test / run entry points
├── README.md
├── src/
│   └── myproject/
│       ├── __init__.py     # empty, or the public re-exports only
│       ├── __main__.py     # `python -m myproject` → calls main()
│       ├── py.typed        # ships type hints to dependants (libraries)
│       ├── config.py       # settings loaded once from env (pydantic-settings)
│       ├── domain/         # pure types and rules — no I/O imports
│       │   ├── order.py
│       │   └── errors.py
│       ├── service/        # use cases; depends on domain + Protocols
│       │   └── order_processing.py
│       ├── repository/     # DB access; implements the service's Protocols
│       │   └── order.py
│       └── api/            # HTTP / CLI boundary; pydantic models + mappers
│           ├── v1/
│           │   └── order.py
│           └── app.py      # app factory: wires dependencies
└── tests/
    ├── conftest.py
    ├── unit/
    │   └── service/
    │       └── test_order_processing.py
    └── integration/
        └── api/
            └── test_order.py
```

Why `src/`: tests then run against the installed package, not the working directory, so
a missing file in the package or a wrong import fails in tests, not in production.

Layer rules (`sadensmol-programming` → *Layers*):

- `domain/` imports nothing from `service/`, `repository/` or `api/`, and no framework,
  DB driver or HTTP library.
- `service/` declares the `Protocol`s it needs (`class OrderRepository(Protocol)`);
  `repository/` implements them. Wire concrete types in one place (`app.py` /
  `main()`), by constructor arguments — no service locator, no import-time globals.
- `api/` holds pydantic request/response models and maps them to domain types at once.

A small script or tool is fine as one module; add the layers when there is something
to separate.

## `pyproject.toml`

```toml
[project]
name = "myproject"
version = "0.1.0"
requires-python = ">=3.12"
dependencies = [
    "httpx>=0.27",
    "pydantic>=2.7",
]

[project.scripts]
myproject = "myproject.__main__:main"

[dependency-groups]
dev = [
    "mypy>=1.10",
    "pytest>=8",
    "pytest-asyncio>=0.23",
    "pytest-cov>=5",
    "ruff>=0.5",
]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"

[tool.ruff]
line-length = 100
target-version = "py312"

[tool.ruff.lint]
select = [
    "E", "W", "F",   # pycodestyle, pyflakes
    "I",             # isort
    "N",             # pep8-naming
    "UP",            # pyupgrade — modern syntax
    "B",             # bugbear
    "A",             # builtins shadowing
    "C4",            # comprehensions
    "SIM",           # simplify
    "RET",           # return
    "PTH",           # pathlib over os.path
    "DTZ",           # naive datetime
    "BLE",           # blind except
    "TRY",           # exception anti-patterns
    "LOG", "G",      # logging
    "T20",           # print
    "ASYNC",         # blocking calls in async
    "PT",            # pytest style
    "RUF",
]

[tool.mypy]
strict = true
warn_unreachable = true
files = ["src", "tests"]

[tool.pytest.ini_options]
testpaths = ["tests"]
addopts = ["--strict-markers", "--strict-config", "-ra"]
asyncio_mode = "auto"
filterwarnings = ["error"]
```

- Pin lower bounds in `dependencies`; the exact versions live in `uv.lock`.
- A library never pins upper bounds without a known break.
- Dev-only tools go in `[dependency-groups] dev`, never in `dependencies`.

## Makefile

```make
.PHONY: install lint fmt typecheck test

install:
	uv sync

fmt:
	uv run ruff format .
	uv run ruff check --fix .

lint:
	uv run ruff format --check .
	uv run ruff check .
	uv run mypy

test:
	uv run pytest
```

CI runs `make lint` and `make test`; it never formats.

## Configuration

- Read settings once at start-up into a typed object (`pydantic-settings` `BaseSettings`,
  or a frozen dataclass filled from `os.environ`). Pass it down; never read `os.environ`
  deep inside the code.
- Secrets come from the environment or a secret manager — never from a committed file.
  Fail at start-up when a required setting is missing.

## Entry point

```python
# src/myproject/__main__.py
import logging
import sys

from myproject.app import run
from myproject.config import Settings


def main() -> int:
    logging.basicConfig(level=logging.INFO)
    settings = Settings()
    return run(settings)


if __name__ == "__main__":
    sys.exit(main())
```

## Dockerfile (service)

```dockerfile
FROM python:3.12-slim AS build
COPY --from=ghcr.io/astral-sh/uv:latest /uv /usr/local/bin/uv
WORKDIR /app
ENV UV_COMPILE_BYTECODE=1 UV_LINK_MODE=copy
COPY pyproject.toml uv.lock ./
RUN uv sync --locked --no-dev --no-install-project
COPY src ./src
RUN uv sync --locked --no-dev

FROM python:3.12-slim
WORKDIR /app
COPY --from=build /app /app
ENV PATH="/app/.venv/bin:$PATH" PYTHONUNBUFFERED=1
USER nobody
CMD ["myproject"]
```

Pin the `uv` image tag to a version in a real project. Copy the lock file before the
source so the dependency layer stays cached.
