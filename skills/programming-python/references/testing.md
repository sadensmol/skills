# Python Testing

Read this when you write or change tests.

Principles are in `sadensmol-programming` → *Testing*. This file is the pytest mechanics.

## Layout and naming

- `tests/unit/…` mirrors `src/<package>/…`; `tests/integration/…` holds tests that use a
  real DB, HTTP server or container.
- File `test_<unit>.py`, function `test_<behaviour>` that states the outcome:
  `test_cancel_refunds_paid_order`, not `test_cancel_1`.
- Test the public API of a module. Do not import `_private` names into a test.

## Table tests

```python
import pytest


@pytest.mark.parametrize(
    ("amount", "currency", "expected"),
    [
        pytest.param(Decimal("1.50"), "USD", "1.50 USD", id="two-decimals"),
        pytest.param(Decimal("0"), "EUR", "0.00 EUR", id="zero"),
    ],
)
def test_format_money(amount: Decimal, currency: str, expected: str) -> None:
    assert format_money(Money(amount=amount, currency=currency)) == expected


def test_money_rejects_negative_amount() -> None:
    with pytest.raises(ValueError, match="negative amount"):
        Money(amount=Decimal("-1"), currency="USD")
```

- Give each case an `id`, so a failure names the case.
- `pytest.raises(..., match=...)` checks the type and the message.

## Fixtures

```python
# tests/conftest.py
@pytest.fixture
def repository() -> InMemoryOrderRepository:
    return InMemoryOrderRepository()


@pytest.fixture
def processing(repository: InMemoryOrderRepository) -> OrderProcessing:
    return OrderProcessing(repository=repository)
```

- Function scope by default. Widen the scope (`session`) only for expensive, read-only
  setup such as a DB container — and reset the state between tests.
- Teardown with `yield` in the fixture, not in the test.
- Built-ins first: `tmp_path` (files), `monkeypatch` (env vars, attributes), `caplog`
  (log assertions), `capsys` (stdout).

## Fakes before mocks

- Inject dependencies through constructor arguments typed as a `Protocol`. A test passes
  a small in-memory fake (`InMemoryOrderRepository`) that behaves like the real thing.
- Use `unittest.mock` only at an external boundary you cannot fake. Always
  `create_autospec(RealClass, instance=True)` or `patch(..., autospec=True)`, so a wrong
  method name or signature fails the test.
- Patch where the name is looked up (`myproject.service.order.httpx`), not where it is
  defined.
- Assert on the outcome (returned value, saved state). Assert calls only when the call is
  the behaviour (a message sent, an email queued).

## Time, randomness, environment

- Inject a clock (`Callable[[], datetime]`) instead of calling `datetime.now` inside
  logic; tests pass a fixed one. Or use `time-machine` / `freezegun` at the boundary.
- Seed or inject random sources.
- Set env vars with `monkeypatch.setenv`; never edit `os.environ` directly.

## Async tests

With `asyncio_mode = "auto"` (pytest-asyncio), an `async def test_…` runs on its own loop:

```python
async def test_load_dashboard_times_out(slow_client: FakeClient) -> None:
    with pytest.raises(TimeoutError):
        await load_dashboard(UserId("u1"), client=slow_client)
```

## Logs

```python
def test_failed_refund_logs_error(caplog: pytest.LogCaptureFixture) -> None:
    with caplog.at_level(logging.ERROR):
        processing.refund(OrderId("o1"))
    assert "cannot refund order o1" in caplog.text
```

## Running

- `make test` / `uv run pytest`. One test: `uv run pytest tests/unit/service/test_order_processing.py::test_cancel_refunds_paid_order`.
- `filterwarnings = ["error"]` in config, so a deprecation warning fails the run.
- Coverage (`pytest --cov`) is a hint about untested code, not a target to hit.
