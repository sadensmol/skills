---
name: sadensmol-programming-python
description: "Expert Python guidance for idiomatic, typed, modern Python (3.12+). Use when writing, reviewing, refactoring or debugging Python code (`*.py`, `*.pyi`); setting up a Python project, package or service (`pyproject.toml`, `uv`, `uvx`, `src/` layout, uv workspace, virtualenv `.venv`, `.python-version`, adding or upgrading dependencies, lock file); laying out a multi-component repository (`backend/`, `frontend/`, `docs/`, `examples/`, `scripts/`, `skills/`, `contracts/`, `deploy/`); environment variables, `.env` / `.env.example`, settings and secrets (`pydantic-settings`); configuring `ruff`, `mypy` or `pyright`; type hints, `Protocol`, generics, dataclasses, `pydantic`, enums, `match`; exceptions and logging; `asyncio`, `TaskGroup`, threads or processes; context managers and resources; writing tests with `pytest` (fixtures, parametrize, monkeypatch, mocks)."
---

# Python Programming

Act as a senior Python developer. Keep code typed, explicit, flat and small. Prefer the
standard library and plain functions over frameworks and class hierarchies. Target the
newest Python the project pins (`requires-python`); for new projects use 3.12+.

**REQUIRED:** load `sadensmol-programming` with this skill — it holds the
language-agnostic rules (naming, comments, domain types, layers, errors, backward-compatible
data, tests); this skill holds the Python mechanics and wins where they conflict.

**A project's own conventions win.** When the repository has its own `Makefile`, `ruff`
/ `mypy` config, layout or style guide, follow it over this skill.

## Tooling (non-negotiable)

- **`uv`** manages the interpreter, the virtualenv, dependencies and the lock file:
  `uv sync`, `uv add <pkg>`, `uv add --dev <pkg>`, `uv run <cmd>`. Commit `uv.lock`. Never
  `pip install` into the system or a global interpreter.
- **Settings come from env vars, read in one place** (a `pydantic-settings` class built in
  `main()`). `.env.example` is committed; `.env` never is.
- **`ruff format`** is the formatter and **`ruff check`** the linter. Never hand-format;
  autofix with `ruff check --fix`. Both must be clean before work is done.
- **A type checker in strict mode** — `mypy --strict` or `pyright` (`"strict"`). Use the
  one the project already uses. New code type-checks with zero errors and no blanket
  `# type: ignore`.
- All config lives in `pyproject.toml` — no `setup.py`, `setup.cfg`, `requirements.txt`
  or `.flake8` in new projects.
- Run everything through the project's entry point (`make lint`, `make test`, or
  `uv run …`), never through a bare global `python`.

## Naming (PEP 8)

- Modules and packages: short `snake_case` (`order_service.py`). Functions, methods,
  variables: `snake_case`. Classes, type aliases, `TypeVar`s: `PascalCase`. Constants:
  `UPPER_SNAKE_CASE` at module level.
- `_leading_underscore` marks a module- or class-private name. Never use `__dunder__`
  names for your own attributes; `__name_mangling` only to avoid a subclass clash.
- Booleans read as assertions: `is_active`, `has_items`, `can_retry`.
- No `get_` prefix on a cheap attribute — use a `@property` (`order.total`, not
  `order.get_total()`). Keep `get_`/`fetch_`/`load_` for calls that do I/O.
- Never shadow builtins (`id`, `type`, `list`, `input`, `filter`); use `order_id`,
  `kind`, `items`.

## Type Hints

**Every function signature is fully annotated, public and private alike.** Locals are
annotated only when the checker cannot infer them.

- Modern syntax only: `list[int]`, `dict[str, User]`, `X | None`, `type UserId = int`
  (3.12) — never `List`, `Optional`, `Union` from `typing`.
- Abstract parameter types, concrete return types: accept `Iterable[T]` / `Sequence[T]` /
  `Mapping[K, V]`, return `list[T]` / `dict[K, V]`.
- `Any` is banned outside a boundary that truly takes anything; use `object` and narrow
  with `isinstance`. `cast()` needs a comment that says why it is safe.
- Domain identifiers are distinct types: `UserId = NewType("UserId", int)` or a frozen
  value object — never a bare `int` / `str` that can be swapped by mistake.
- `Protocol` for structural interfaces at a boundary; `abc.ABC` only when you share
  implementation. `typing.Self`, `@override` (3.12) and `Final` where they apply.
- Use `TypedDict` only for dicts you do not own (JSON you pass through); otherwise a
  dataclass.

**Full rules:** read `references/typing-and-data.md` before you design a type, a
dataclass, an enum or a pydantic model.

## Data Modelling

- **Default to `@dataclass(frozen=True, slots=True, kw_only=True)`** for domain values and
  records. Mutate by `dataclasses.replace(obj, field=...)`.
- **`pydantic` only at the boundary** (HTTP request/response, config, external JSON) where
  you need parsing and validation. Map to a domain dataclass at once; do not pass the
  pydantic model through the domain.
- Closed sets of values are `enum.StrEnum` / `enum.Enum`, never bare string literals.
  Model exclusive states as a union of small dataclasses and branch with `match`.
- Never use a mutable default argument (`def f(items=[])`); use `None` and build inside,
  or `field(default_factory=list)` in a dataclass.

## Errors

The rules are in `sadensmol-programming` → *Errors*. Python mechanics:

- Raise a specific exception type. Define a small hierarchy per package
  (`class OrderError(Exception)`, `class OrderNotFound(OrderError)`). Never raise bare
  `Exception`.
- **Chain every translated exception (MUST FOLLOW):** `raise OrderNotFound(order_id) from
  err`. Use `from None` only to hide a meaningless internal cause, and say why.
- **Never `except:` or `except Exception: pass` (MUST FOLLOW).** Catch the narrowest type
  that you can handle. A broad catch is allowed only at the top-level boundary (request
  handler, worker loop, `main`), and it logs with the traceback.
- Keep the `try` body to the one call that can fail; put the rest in `else`.
- `assert` is for invariants in tests and internal sanity checks only — `python -O`
  strips it. Validate input with an explicit `if` + `raise`.
- Use `ExceptionGroup` / `except*` only for errors that come from a `TaskGroup` or
  other concurrent work.

## Logging

- `logger = logging.getLogger(__name__)` per module. Never `print()` in library or
  service code; `print` is for a CLI's real output only.
- Pass values as arguments, not f-strings: `logger.info("order %s paid", order_id)`.
- `logger.exception(...)` inside an `except` block to keep the traceback.
- Configure handlers once, in the entry point (`main`), never at import time in a library.
- Levels and "never log and re-raise": `sadensmol-programming` → *Logging*, *Errors*.

## Resources and Code Shape

- Manage every resource with `with` (files, locks, connections, sessions). Write your
  own with `@contextlib.contextmanager` or `contextlib.ExitStack`.
- `pathlib.Path` for file paths, never `os.path` string joins. Read text with
  `Path.read_text(encoding="utf-8")` — always pass `encoding`.
- Comprehensions for a simple map/filter; a plain `for` loop when the body has side
  effects or more than one condition. No nested comprehension deeper than two levels.
- Return early; keep the happy path unindented.
- Module-level code only defines things. No I/O, network or global state on import;
  start work in `def main()` under `if __name__ == "__main__":`.
- No `from module import *`. Absolute imports; relative imports only inside a package.
- Prefer functions and modules over classes. A class earns its place when it holds
  state with a lifecycle or implements a `Protocol`. No `utils.py` / `helpers.py`
  catch-all modules — name the module after what it does.
- Use `datetime.now(tz=UTC)`; never a naive `datetime`. Use `decimal.Decimal` for money,
  never `float`.

## Concurrency

- I/O-bound concurrency → `asyncio`. Run parallel tasks with `asyncio.TaskGroup` (3.11+),
  not loose `create_task` calls or `gather` without error handling. Bound waits with
  `asyncio.timeout()`.
- **Never block the event loop** — no `time.sleep`, `requests`, sync DB drivers or heavy
  CPU in `async def`. Use async libraries (`httpx`, `asyncpg`) or
  `asyncio.to_thread(...)`.
- Keep a reference to every task you create; a dropped task can be garbage-collected
  mid-run.
- CPU-bound work → `concurrent.futures.ProcessPoolExecutor`. Blocking I/O in sync code →
  `ThreadPoolExecutor`. Protect shared mutable state with a lock or avoid sharing it.

Read `references/async.md` before you write `async` code, a worker or a background task.

## Testing

Test principles (test-first for bug fixes only, independent oracle, black-box, assert on
meaning) are in `sadensmol-programming` → *Testing*. Python mechanics:

- `pytest` only — plain `assert`, plain functions, no `unittest.TestCase` in new code.
- Tests live in `tests/`, mirror the package layout, and are named `test_<unit>.py`.
- `@pytest.mark.parametrize` for table tests; fixtures in `conftest.py` for shared setup;
  `tmp_path`, `monkeypatch` and `caplog` before any hand-rolled helper.
- Fake dependencies through injected `Protocol` implementations first; `unittest.mock`
  only at an external boundary, always with `autospec=True`.

Read `references/testing.md` before you write or change tests.

## Read on demand

**When a trigger below matches what you are about to do, read that file first** — before
you write, review or answer. The summary in this skill is not the full rule.

| When | Read |
|---|---|
| You create a project, package or service, lay out a multi-component repository (`backend/`, `frontend/`, `docs/`, `examples/`, `skills/` …), add a dependency, or set up `pyproject.toml`, a uv workspace, `ruff`, the type checker, a `Makefile` or a Dockerfile | `references/project-structure.md` |
| You create or repair a virtualenv, pin the interpreter, add / remove / upgrade a dependency, install a CLI tool, write a standalone script, or add an env var, a `.env` file, a setting or a secret | `references/environment.md` |
| You design a type, dataclass, enum, `Protocol`, generic or pydantic model | `references/typing-and-data.md` |
| You write `async` code, a worker, a background task, or use threads or processes | `references/async.md` |
| You write or change tests | `references/testing.md` |
| **Before declaring Python work complete**, or when you review Python code | `references/review-checklist.md` |
