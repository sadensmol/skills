# Python Review Checklist

Read this before declaring Python work complete, and when you review Python code.

## Review Checklist

- [ ] `ruff format --check` and `ruff check` clean; the type checker (`mypy --strict` /
      `pyright` strict) passes with no new `# type: ignore` or `cast` without a reason.
- [ ] Every function signature is annotated; modern syntax (`list[int]`, `X | None`); no
      `Any` outside a real boundary.
- [ ] Domain IDs are `NewType` / value objects, not bare `int` / `str`.
- [ ] Domain values are frozen, slotted, keyword-only dataclasses; pydantic stays at the
      boundary and is mapped at once.
- [ ] Closed value sets are enums; exclusive states are a union + `match` with
      `assert_never`.
- [ ] No mutable default arguments; no shadowed builtins.
- [ ] Specific exception types; every translation is `raise … from err`; no bare
      `except:` or swallowed `except Exception`; `assert` not used for input validation.
- [ ] `logging.getLogger(__name__)` with %-style arguments; no `print` outside CLI output;
      no log-and-re-raise.
- [ ] Resources opened with `with`; `pathlib` for paths; `encoding="utf-8"` on text I/O.
- [ ] No I/O or global state at import time; wiring happens in `main()` / the app factory.
- [ ] Timezone-aware `datetime`; `Decimal` for money.
- [ ] Async code never blocks the loop; parallel work uses `TaskGroup`; every network
      wait has a timeout; background tasks are referenced and their errors logged;
      `CancelledError` is not swallowed.
- [ ] Layers respected: `domain/` imports no framework, driver or HTTP library.
- [ ] Dependencies added with `uv add` (dev tools in the `dev` group); `uv.lock` updated.
- [ ] Tests use pytest, parametrize table cases with ids, prefer fakes over mocks, mocks
      use `autospec`; bug fixes have a reproducing test.
