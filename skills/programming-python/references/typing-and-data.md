# Python Types and Data

Read this when you design a type, dataclass, enum, `Protocol`, generic or pydantic model.

## Domain values — frozen dataclasses

```python
from dataclasses import dataclass, replace
from decimal import Decimal
from typing import NewType

OrderId = NewType("OrderId", str)


@dataclass(frozen=True, slots=True, kw_only=True)
class Money:
    amount: Decimal
    currency: str

    def __post_init__(self) -> None:
        if self.amount < 0:
            raise ValueError(f"negative amount: {self.amount}")

    def add(self, other: "Money") -> "Money":
        if other.currency != self.currency:
            raise ValueError(f"currency mismatch: {self.currency} != {other.currency}")
        return replace(self, amount=self.amount + other.amount)
```

- `frozen=True` — values never change after creation; build a new one with `replace`.
- `slots=True` — less memory, and a typo in an attribute name fails at once.
- `kw_only=True` — call sites name every field, so adding a field never shifts others.
- Validate invariants in `__post_init__`; an invalid object never exists.
- Put behaviour that belongs to the value on the value (`money.add`), not in a helper
  module (`sadensmol-programming` → *Types and Domain Modelling*).

## Exclusive states — union + `match`

```python
from dataclasses import dataclass
from datetime import datetime
from typing import assert_never


@dataclass(frozen=True, slots=True)
class Pending:
    pass


@dataclass(frozen=True, slots=True)
class Paid:
    paid_at: datetime


@dataclass(frozen=True, slots=True)
class Failed:
    reason: str


type PaymentState = Pending | Paid | Failed


def describe(state: PaymentState) -> str:
    match state:
        case Pending():
            return "waiting"
        case Paid(paid_at=at):
            return f"paid at {at:%Y-%m-%d}"
        case Failed(reason=reason):
            return f"failed: {reason}"
        case _:
            assert_never(state)
```

`assert_never` (from `typing`) makes the type checker flag a new state that `describe`
does not handle. Prefer this over a class with several `bool` / `Optional` flags.

## Enums

```python
from enum import StrEnum, auto


class OrderStatus(StrEnum):
    NEW = auto()        # "new"
    SHIPPED = auto()
    CANCELLED = auto()
```

- `StrEnum` when the value is serialized as a string; `Enum` otherwise.
- Never compare to the raw string (`status == "new"`); compare to the member.
- A persisted enum value never changes once written (`sadensmol-programming` →
  *Backward-Compatible Data*). Add members; do not rename values.

## Protocols — interfaces at a boundary

```python
from typing import Protocol


class OrderRepository(Protocol):
    def get(self, order_id: OrderId) -> Order | None: ...
    def save(self, order: Order) -> None: ...


class OrderProcessing:
    def __init__(self, repository: OrderRepository) -> None:
        self._repository = repository
```

- The consumer (the service) declares the `Protocol`; the implementation does not
  inherit from it. A test passes a small fake class.
- Keep a `Protocol` to the methods its consumer calls.
- Add one only at a real boundary (I/O, another layer); see `sadensmol-programming` →
  *Code Shape* for when an interface is worth it.

## Generics (3.12 syntax)

```python
from collections.abc import Sequence


def first[T](items: Sequence[T]) -> T | None:
    return items[0] if items else None


class Page[T]:
    def __init__(self, items: list[T], next_cursor: str | None) -> None:
        self.items = items
        self.next_cursor = next_cursor
```

Use bounds when you need a method on `T`: `def total[T: Priced](items: Iterable[T])`.

## Absence

- `X | None` means "may be missing"; handle the `None` before use. The type checker
  enforces it — do not silence it with `# type: ignore` or `assert x`.
- An empty collection is not absence: return `[]`, not `None`, for "no items"
  (`sadensmol-programming` → *Types and Domain Modelling*).

## pydantic at the boundary

```python
from pydantic import BaseModel, ConfigDict, Field


class CreateOrderRequest(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    customer_id: str = Field(min_length=1)
    amount: Decimal = Field(gt=0)
    currency: str = Field(pattern=r"^[A-Z]{3}$")

    def to_domain(self) -> NewOrder:
        return NewOrder(
            customer_id=CustomerId(self.customer_id),
            total=Money(amount=self.amount, currency=self.currency),
        )
```

- `extra="forbid"` on input models so an unknown field is an error, not silently dropped.
  On models that read a third party's responses use `extra="ignore"` so their new fields
  do not break you.
- The mapper to the domain type is a method on the boundary model (`to_domain`), or a
  `from_domain` classmethod on a response model. Domain code never imports pydantic.
- Money is `Decimal` and is serialized as a string, never as a JSON float.
