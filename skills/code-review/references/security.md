# The Security area

Read this when you render the Security sections of a report, or when the user asked for a
full security audit of the change. What the hunters and the verifier do is in the
`security` agent (`agents/code-reviewer/security.md`).

## Priority

The workflow sets it (`securityPriority()` in `dispatch-claude-code.md`); a hand-run
fallback and a full audit apply the same map:

| Record | Priority |
|---|---|
| confirmed, severity critical or high | **P0** |
| confirmed, severity medium | **P1** |
| confirmed, severity low or informational | **P2** |
| needs_validation | **P1** |
| pre-existing (the defect is in code the diff did not change) | capped at **P1**, title prefixed `[pre-existing]` |

A pre-existing defect is real but older than this change, so it does not block the merge.
Say in the disposition that it needs its own ticket.

## Report

The Security rows sit in the priority tables like every other row. Below the tables, before
`### Linter`, add these sections. Omit a section only when it has nothing to show.

```
### Security details

**P0 #1 — <title>** · confirmed, high
- **Attack:** <principal> controls <input>; <control> does not stop it at <boundary> → <result>
- **Trace:** `<entrypoint file:line>` → `<propagation file:line>` → `<sink file:line>`
- **Conditions:** <conditions>
- **Fix:** <fix>
- **Test:** <regressionTest>

**P1 #4 — <title>** · needs validation
- **Attack:** …
- **Trace:** …
- **Blocker:** <blockers>
- **Fix:** <fix>

### Security coverage

| Boundary | Paths read | Result |
|---|---|---|
| operator → callback signature check | `internal/controller/v1/callback.go`, `internal/service/auth.go` | HMAC verified over the raw body before any state change |

### Security hardening (not findings)

- `internal/service/integration.go:212` — <note>

### Security verifier

Rejected 2 candidates:
- <title> — <reason>
```

- One detail block per Security row, in table order, numbered as in the tables.
- Mark a finding the verifier never decided as `· unverified`. When `verifierFailed` is
  true, say in one line that the verifier did not run and every Security finding is
  unverified. Name each hunter in `huntersFailed`: its attack classes were not covered.
- **Coverage is the useful half of a clean result.** "No Security findings" plus the
  coverage table tells the user which boundaries were checked; without it, the empty
  result says nothing.

## Full security audit (opt-in)

Run it only when the user asked for it (`SKILL.md` → *Full security audit*). The audit is
the upstream `security-audit` skill's own **full audit mode** — do not restate or adapt
its procedure here.

1. **Workflow args.** Pass `"security": "audit"`. The workflow then skips its Security
   branch, and the other five areas run as usual in the background.
2. **Run the audit yourself, meanwhile.** Load `security-audit` and follow its full audit
   mode as its parent. Tell it the run is a *scoped run* over `<BASE>...HEAD` and the
   surface that reaches it, with the profile the user named (`quick` when none). Everything
   else — setup, output directory, budget, phases, terminal states — is the skill's.
3. **Rows.** Each `confirmed` and `needs_validation` record in its `findings.json` is one
   Security row, with the priority from the table above. A record whose trace touches no
   line the diff changed is pre-existing. `rejected` records go under *Security verifier*.
4. **Report.** Render the *Security details* blocks from those records, and give the path
   of the run's `REPORT.md`. When the run ended `incomplete`, say so first, with its
   `incomplete_reason`.
