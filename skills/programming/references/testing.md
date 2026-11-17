# Test Oracles and Error Assertions

Read this when you write or review a test, compute an expected value, or assert on an
error. Examples are in Go; the rule is the same in any language.

## The expected-value oracle must be an INDEPENDENT implementation

A test that computes an expected value must compute it **independently** —
**never** call the production function it is checking; that is a tautology that
stays green when the formula is wrong. **Do not "de-duplicate" the oracle**, and
never export a production function just so a test can reuse it. (A unit test of
the formula asserts hand-computed literals — same principle.)

```go
// Good — re-derives the expected win itself
expectedWin := 0
if selected == resultMult {
    mult100 := int64(math.Round(float64(resultMult) * 100))
    expectedWin = int(int64(bet) * mult100 / 100)
}
s.Require().Equal(prevBalance-bet+expectedWin, gotBalance)

// Bad — oracle calls the function under test
win, _ := usecase.CalcWinAmount(selected, resultMult, bet) // ❌ tautology
```

## Don't assert on exact error-message strings

Error *text* from the standard library or third-party libraries is an implementation
detail that shifts with library version and with transport/middleware wrapping.
Asserting the full string makes tests brittle: a dependency change rewords the
message and the test fails even though behaviour is unchanged. Assert on
**semantics** (the error type / an error code / a stable substring), not the exact
message.

```go
// Bad — couples the test to stdlib phrasing
require.Equal(t, `Post "…": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`, err.Error())

// Good — assert the meaning
require.ErrorIs(t, err, context.DeadlineExceeded)
// or, when only the wrapped message is reachable, a stable fragment:
require.Contains(t, err.Error(), "Client.Timeout exceeded")
```

Instrumentation and middleware are a common cause: wrapping a client changes how its
errors read, without any change in behaviour. The language skill records the concrete
cases.

## Testing rules — full text

The full text of SKILL.md → *Testing*.

- **Test-first applies to BUG FIXES ONLY (MUST FOLLOW).** The behaviour exists and is
  wrong: write the failing test, watch it fail, then fix. **New logic is
  implementation-first:** write the code, then cover it. Never invoke
  `superpowers:test-driven-development` for new functionality. The deciding question:
  does the behaviour already exist and misbehave? Yes → test first. No → code first.
- Check the existing tests first and follow their pattern. Add a test to the existing
  file for that feature, never a parallel one-test file.
- Never change production code while you write tests. If the code must change, ask
  first.
- Use constants for test data.
- **The expected-value oracle is an independent implementation** — never call the
  production function the test checks.
- **Black-box:** drive the code through its public API and control the world around it.
  A fake of a dependency may return values, return errors or delay — within that
  dependency's real contract. Never stub the code's own internals to force a path.
- **Assert on meaning, not on library message text** — the error type, code or a stable
  fragment.
- **Reproduce a bug with the fewest entities**, and make each fake as strict as the real
  dependency.
- One behaviour per test, named by the behaviour; Arrange-Act-Assert.
- Keep tests deterministic: inject the clock, network and storage; no shared global state.
  Poll for an expected state instead of sleeping a fixed time. The one exception is
  proving that something did NOT happen.
- **Wait for the state you assert, not for a signal before it (MUST FOLLOW).** When work
  runs in the background, the poll condition must cover every value the test then
  asserts. Waiting for "the message was posted" and then asserting a field the code saves
  after the post is a race: it passes on a fast machine and fails on a busy CI runner.
- When a test fails, read the logs first. If they are not enough, add logs and run again.
  Never guess.
- **A result shown for approval names its metric and runs the real path.** Say what each
  number measures (the lowest value of many runs is not growth between runs). Check it
  through the production code path, with the user's acceptance criterion as the test.
- **Never weaken a test's oracle to reach green without the user's yes.** State the old
  rule and the new rule.
- **A green rerun is not a diagnosis.** Report the failure as unclassified until a cause
  is shown.
