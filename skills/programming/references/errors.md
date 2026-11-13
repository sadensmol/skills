# Errors

Read this when you return, wrap, log, swallow or catch an error, or two errors meet on
one path. This is the full text of SKILL.md → *Errors*.

- **Every error is returned XOR logged — never both, never neither (MUST FOLLOW).** Never
  discard an error silently.
- **Never log and return.** The boundary (error middleware, interceptor, UI root) logs
  once; a layer that logs and returns produces two lines for one event.
- **Best-effort work you deliberately do not return** (cache writes, fire-and-forget,
  cleanup) is logged at the call site. A shared helper that swallows errors internally is
  a smell — make it return the error and let each caller decide.
- **Every propagated error carries context** — "cannot X: cause" — and keeps the cause
  inspectable (type, code), so the boundary can recognise and classify it (for example a
  cancellation it downgrades).
- **Layer roles:** the repository names the root cause; the service/usecase adds context;
  the controller returns the error as-is and the boundary formats and logs it.
- **Two errors must never meet in one return (MUST FOLLOW).** Never weld one error's text
  into another's message. Fix the control flow — usually exit early on the failing path,
  so the second operation never runs. First verify the skipped work has nothing left to
  persist on that path. When the second operation genuinely must run, pick **which single
  error owns the return** and describe the other situation in text.
- **Catch only what you handle.** A catch-all belongs only at a true boundary (request
  handler, UI root), where it logs and continues, or to rethrow. Do not use errors for control flow that is not
  exceptional — return a result type, enum or optional.
