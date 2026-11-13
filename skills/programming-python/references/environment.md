# Python Environments, Dependencies and Settings

Read this when you create or repair a virtualenv, pin the interpreter, add, remove or
upgrade a dependency, install a CLI tool, write a one-off script, or add an environment
variable, a `.env` file, a setting or a secret.

**`uv` is the only tool.** Do not mix it with `pip install`, `pip-tools`, `poetry`,
`pipenv`, `conda`, `pyenv` or `virtualenv` in the same project. When the project already
uses one of them, follow the project and do not migrate it unasked.

## Interpreter

```bash
uv python install 3.12          # downloads a managed interpreter; no system Python needed
uv python pin 3.12              # writes .python-version — commit it
uv python list                  # what is installed and available
```

- `.python-version` and `requires-python` in `pyproject.toml` must agree.
- Never use the OS Python (`/usr/bin/python3`) for project work, and never `sudo pip`.

## Virtual environments

- **One `.venv/` per project**, at the root next to `pyproject.toml`. In a uv workspace
  there is one `.venv/` at the workspace root (`backend/.venv`) for all members.
- `uv sync` creates and fills it from `uv.lock`. `uv run <cmd>` syncs first, then runs
  inside it — so **you never need to activate it**. Write commands as `uv run pytest`,
  `uv run python -m myproject`, never a bare `python` or `pytest`.
- `.venv/` is gitignored. It is disposable: when it breaks, delete it and run `uv sync`.
  Never fix it by hand with `pip install`.
- Point the editor at `.venv/bin/python` (VS Code: *Python: Select Interpreter*).
- Activate (`source .venv/bin/activate`) only for an interactive shell session.
- Docker: build the venv inside the image with `uv sync --locked --no-dev` (see
  `references/project-structure.md` → *Dockerfile*). Never copy a host `.venv/` into an
  image — add it to `.dockerignore`.

## Dependencies

| Task | Command |
|---|---|
| Install everything from the lock file | `uv sync` |
| Add a runtime dependency | `uv add httpx` |
| Add a dev-only tool | `uv add --dev pytest` (the `dev` group) |
| Add to a named group | `uv add --group lint ruff` |
| Add an optional extra for users | `uv add --optional postgres asyncpg` |
| Remove a dependency | `uv remove httpx` |
| Upgrade one package | `uv lock --upgrade-package httpx && uv sync` |
| Upgrade everything | `uv lock --upgrade && uv sync` |
| See the tree / what is outdated | `uv tree`, `uv tree --outdated` |
| CI / Docker install | `uv sync --locked` (fails if `uv.lock` is stale) |
| Production install, no dev tools | `uv sync --locked --no-dev` |
| A tool needs `requirements.txt` | `uv export --format requirements-txt` — generated, never hand-edited |

Rules:

- **Always change dependencies through `uv add` / `uv remove`**, never by editing
  `uv.lock`. Edit `pyproject.toml` by hand only for a version bound, then run `uv lock`.
- **Commit `pyproject.toml` and `uv.lock` together** in the same change.
- Runtime code imports only `[project] dependencies`. Test, lint and type-check tools
  live in `[dependency-groups]`, so production images do not ship them.
- In a workspace, add a dependency to the member that imports it:
  `uv add --package myproject-core httpx`. A member that needs another member uses
  `{ workspace = true }` in `[tool.uv.sources]`.
- Before adding a package, check that the standard library cannot do it, that the package
  is maintained, and that its licence fits. One package per need — do not add a second
  HTTP client.
- Audit for known vulnerabilities in CI: `uv run --with pip-audit pip-audit`. Let
  Dependabot or Renovate open the upgrade PRs; review the changelog before you merge.
- Private index: set it in `[[tool.uv.index]]` in `pyproject.toml`; pass the credentials
  through `UV_INDEX_<NAME>_USERNAME` / `UV_INDEX_<NAME>_PASSWORD` env vars, never in the
  file.

## CLI tools and one-off scripts

- A tool the project's code or CI relies on (`ruff`, `mypy`, `pytest`) is a dev
  dependency, so every developer gets the same pinned version.
- A personal tool you use across projects: `uv tool install <tool>`. A one-shot run:
  `uvx <tool>` (for example `uvx ruff check .`). Never `pip install --user`.
- A standalone script with its own dependencies uses inline metadata (PEP 723) instead
  of a project:

  ```python
  # /// script
  # requires-python = ">=3.12"
  # dependencies = ["httpx>=0.27"]
  # ///
  import httpx
  ...
  ```

  Run it with `uv run scripts/fetch_report.py`; add dependencies with
  `uv add --script scripts/fetch_report.py rich`.

## Environment variables and settings

**Settings come from the environment (12-factor). Code reads them in exactly one place.**

```python
# src/myproject/config.py
from pydantic import PostgresDsn, SecretStr
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(
        env_prefix="MYPROJECT_",
        env_nested_delimiter="__",
        extra="forbid",
        frozen=True,
    )

    database_url: PostgresDsn
    api_key: SecretStr  # pragma: allowlist secret
    log_level: str = "INFO"
    request_timeout_s: float = 10.0
```

- `Settings()` is built once in `main()` / the app factory and passed down by
  constructor arguments. **Never call `os.environ` / `os.getenv` anywhere else** — a
  setting read deep in the code is invisible and untestable.
- A required setting has no default, so a missing one fails at start-up, not at the first
  request.
- Secrets are `SecretStr`: they print as `**********` in logs and reprs. Call
  `.get_secret_value()` only where the secret is used.
- Prefix every variable with the project name (`MYPROJECT_DATABASE_URL`), so it cannot
  clash with another program's variable. Nested settings use `__`
  (`MYPROJECT_DB__POOL_SIZE`).
- Without pydantic, use a frozen dataclass with a `from_env(environ: Mapping[str, str])`
  classmethod — same rules: one place, fail fast, no secret in `repr`
  (`field(repr=False)`).

### `.env` files

| File | Committed | Holds |
|---|---|---|
| `.env.example` | yes | every variable name, with a placeholder or safe default and a one-line comment |
| `.env` | **never** (gitignored) | the developer's real local values |
| `.env.test` | yes, if it holds no secrets | values for the test run |

- Keep `.env.example` complete: a new variable goes into it in the same change as the
  code that reads it.
- Load `.env` only in local development, from outside the code:
  `uv run --env-file .env python -m myproject`, or `direnv` with a gitignored `.envrc`.
  Do not set `env_file=".env"` in `Settings` for a service — production must never pick up
  a stray file.
- Production values come from the platform: Kubernetes Secrets / ConfigMaps, the CI
  secret store or a secret manager, injected as env vars. Never bake them into an image.
- Never print, log or commit a secret value. To check that a variable is set, print
  its name and whether it is set, not its value.
- Tests build `Settings(...)` with explicit arguments, or set variables with
  `monkeypatch.setenv`; they never read the developer's `.env`.

## `.gitignore` entries

```gitignore
.venv/
__pycache__/
*.py[cod]
.env
.envrc
.mypy_cache/
.ruff_cache/
.pytest_cache/
.coverage
htmlcov/
dist/
build/
*.egg-info/
```
