# Python Concurrency

Read this when you write `async` code, a worker, a background task, or use threads or
processes.

## Pick the model

| Work | Use |
|---|---|
| Many network / DB calls waiting on I/O | `asyncio` with async libraries |
| Blocking library you cannot replace, called from async code | `await asyncio.to_thread(fn, ...)` |
| Blocking I/O in sync code | `concurrent.futures.ThreadPoolExecutor` |
| CPU-heavy work (parsing, image, math) | `concurrent.futures.ProcessPoolExecutor` |

Do not mix: a codebase is either async at its boundary (an ASGI app, an async worker) or
sync. Do not call `asyncio.run` inside library code or inside a running loop.

## Structured concurrency — `TaskGroup`

```python
import asyncio


async def load_dashboard(user_id: UserId) -> Dashboard:
    async with asyncio.timeout(5):
        async with asyncio.TaskGroup() as tg:
            profile = tg.create_task(fetch_profile(user_id))
            orders = tg.create_task(fetch_orders(user_id))
    return Dashboard(profile=profile.result(), orders=orders.result())
```

- If one task fails, the group cancels the others and raises an `ExceptionGroup`.
  Handle a specific failure with `except* SomeError`.
- Every wait on the network has a bound: `asyncio.timeout()` or the client's own timeout.
- Prefer `TaskGroup` over `asyncio.gather`. If you keep `gather`, know that one failure
  does not cancel the rest.

## Background tasks

```python
import asyncio
from collections.abc import Coroutine
from typing import Any


class Worker:
    def __init__(self) -> None:
        self._tasks: set[asyncio.Task[None]] = set()

    def spawn(self, coro: Coroutine[Any, Any, None]) -> None:
        task = asyncio.create_task(coro)
        self._tasks.add(task)
        task.add_done_callback(self._tasks.discard)
```

- Keep a strong reference to each task; the loop holds only a weak one.
- A background task logs its own failure (`logger.exception`) — nobody awaits it.
- On shutdown, cancel the tasks and `await` them, so `finally` blocks run.

## Cancellation

- `asyncio.CancelledError` must propagate. If you catch it to clean up, re-raise it.
  Never put it under `except Exception` and swallow it.
- Put cleanup in `finally` or `async with`, so it also runs on cancel.

## Blocking calls to avoid in `async def`

| Blocking | Async replacement |
|---|---|
| `time.sleep` | `await asyncio.sleep` |
| `requests` | `httpx.AsyncClient` |
| `psycopg2`, sync SQLAlchemy session | `asyncpg`, `psycopg` async, SQLAlchemy `AsyncSession` |
| `open().read()` on a large file | `await asyncio.to_thread(path.read_bytes)` |
| `subprocess.run` | `asyncio.create_subprocess_exec` |

`ruff`'s `ASYNC` rules flag most of these.

## Shared clients

Create one `httpx.AsyncClient` / DB pool per process at start-up, pass it down, and close
it on shutdown (`async with` in the app lifespan). Never create a client per request.

## Threads and processes

- Shared mutable state between threads needs a `threading.Lock`; better, pass data
  through a `queue.Queue` and share nothing.
- Arguments to a process pool must be picklable; pass plain data, not open clients.
- Use the executor as a context manager so it shuts down and joins.
