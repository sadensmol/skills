# secret-scan (pre-commit private-info guard)

Blocks commits that add raw secrets or private local info. It scans only the
**added** lines of the staged diff — never existing history — and asks you to
generalize the value or mask it with `***`.

## What it flags

- Private key blocks (`-----BEGIN … PRIVATE KEY-----`)
- Cloud / service tokens: AWS access-key ids, GitHub (`ghp_…`), Slack (`xox…`),
  Google (`AIza…`), JWTs (`eyJ….eyJ….…`)
- Credentials embedded in URLs (`scheme://user:password@host`)
- Hardcoded `password` / `secret` / `token` / `api_key` / `client_secret` / …
  assignments with a real (non-masked) value
- Private local paths (`/Users/<you>/…`, `/home/<you>/…`)

Values that are already masked (`***`, `xxxxx`), templated (`${VAR}`, `<your-key>`,
`os.Getenv(...)`, `process.env.…`), or obvious placeholders are ignored.

## How it runs (two layers)

1. **Claude Code `PreToolUse` hook** — auto-active with the plugin. Intercepts
   `git commit` Bash calls and blocks them (exit 2) if something is found.
2. **Global git `pre-commit`** — catches your *manual* terminal commits in every
   repo. Install once:
   ```bash
   bash "${CLAUDE_PLUGIN_ROOT:-.}/hooks/install-secret-scan.sh"
   # uninstall:
   git config --global --unset core.hooksPath
   ```
   It sets `core.hooksPath` globally and chains any repo-local
   `.git/hooks/pre-commit`. Repos with their own `core.hooksPath` (husky, etc.)
   are left untouched.

## Suppressing a false positive

- Inline, on the offending line: `# pragma: allowlist secret`
  (also accepts `secret-scan: allow` / `gitleaks: allow`)
- Per-repo: add an extended-regex line to `.secret-scan-allow` at the repo root
- One-off: `SECRET_SCAN_DISABLE=1 git commit …`  (skip all)
  or `SECRET_SCAN_NO_PATHS=1 git commit …`  (skip only the local-path check)

## Not covered

Bare usernames without an adjacent secret are not flagged (too noisy). The
scanner is heuristic — treat a clean result as "nothing obvious", not a guarantee.
