# Dispatching the reviewers — Claude Code

Read this only in Claude Code. The neutral instructions live in `SKILL.md`; this file is
one harness's answer to `fan-out` from the runtime contract.

Use the `Workflow` tool with the script below. Do not launch the reviewers by hand.

- **You are already opted in.** The `Workflow` tool refuses to run without explicit user
  opt-in; "the user invoked a skill whose instructions tell you to call Workflow" *is* that
  opt-in. Do not ask for permission.
- **Pass `model: 'claude-opus-5-5'` (the main model, task skill → *Stages*) and
  `effort: 'high'` on every reviewer call** (the script below,
  and each `Agent` call in the fallbacks). The six agent files in `agents/code-reviewer/`
  carry no `model`: they are **shared with OpenCode** (`~/.config/opencode/agents` symlinks
  to the same directory), and a frontmatter `model` overrides OpenCode's `opencode.jsonc`.
  Never write a `model` into those files.
- **Findings come back structured**, so the report is assembled from data rather than from
  prose blobs.
- **Security runs as its own branch**, through the upstream `security-audit` skill: two
  hunters, each owning half of its `ATTACK-CLASSES.md` blocks, in parallel with the other
  five reviewers, then one verifier over their candidates, deduplicated by fingerprint. The script sets each security finding's priority from its
  verdict and severity, so the hunters do not.
- **`security: 'audit'` in `args`** (the user asked for a full security audit, `SKILL.md` →
  *Full security audit*) skips that branch: you run the audit yourself instead.
- **`docs: 'skip'` in `args`** drops the Docs reviewer. `task finish` §4 passes it: docs
  are updated once, later, in `finish` §6a, so a docs finding here is wasted work.
- **It runs in the background.** It returns a run id immediately and a task notification
  when it finishes; `/workflows` shows live progress.
- **If the `Workflow` tool is unavailable**, launch the five general reviewers and the two
  security hunters with the `Agent` tool in a **single message**, using the same briefs.
  When both hunters return, deduplicate their findings by `fingerprint`, launch the
  verifier with `verifierBrief()`, and apply `securityReport()` by hand.
- **In pair mode (you are the pair WRITER), do not use `Workflow`.** It notifies only once,
  when every reviewer is done, so the pair reviewer could not discuss each report as it lands.
  Launch the seven agents with the `Agent` tool in a **single message**, all in the
  background. Each one's `subagent_type` is its `type` in `REVIEWERS` (`security` for the
  hunters), and its prompt is the text `brief()` or `securityBrief()` below builds, filled
  in by hand from your inputs. Add to each brief: end with the findings as a JSON object in
  the `FINDINGS_SCHEMA` (or `SECURITY_SCHEMA`) shape. Each general agent notifies you when it
  finishes: hand its findings to the pair reviewer then, as `pair.md` says. Hand the
  Security findings over only after the verifier has returned.
  An agent that fails or returns no usable JSON is recorded as failed for its area.

## The review workflow

Pass this script to the `Workflow` tool as `script`. It is used unchanged by both the
Local and the PR path — only `args` differ.

```javascript
export const meta = {
  name: 'code-review',
  description: 'Six review areas over one diff, returning structured P0/P1/P2 findings; security is hunted twice and verified',
  phases: [
    { title: 'Review', detail: 'docs, implementation, quality, simplification, testing, and two security hunters — in parallel' },
    { title: 'Verify', detail: 'one security verifier tries to refute every security candidate' },
  ],
}

const FINDINGS_SCHEMA = {
  type: 'object',
  properties: {
    findings: {
      type: 'array',
      items: {
        type: 'object',
        properties: {
          priority: { type: 'string', enum: ['P0', 'P1', 'P2'] },
          title: { type: 'string' },
          file: { type: 'string' },
          line: { type: 'integer' },
          issue: { type: 'string' },
          impact: { type: 'string' },
          fix: { type: 'string' },
        },
        required: ['priority', 'title', 'file', 'issue', 'fix'],
      },
    },
    linter: { type: 'string', description: 'Linter output for changed files; Quality reviewer only, empty otherwise' },
  },
  required: ['findings'],
}

const SECURITY_FINDING = {
  type: 'object',
  properties: {
    verdict: { type: 'string', enum: ['confirmed', 'needs_validation'] },
    severity: { type: 'string', enum: ['critical', 'high', 'medium', 'low', 'informational'], description: 'confirmed only' },
    fingerprint: { type: 'string', description: 'stable root-cause id, no line number' },
    title: { type: 'string' },
    file: { type: 'string' },
    line: { type: 'integer' },
    issue: { type: 'string' },
    impact: { type: 'string' },
    principal: { type: 'string', description: 'the lower-trust principal' },
    input: { type: 'string', description: 'the input or action it controls' },
    control: { type: 'string', description: 'the control that should stop it' },
    boundary: { type: 'string', description: 'the trust boundary crossed' },
    result: { type: 'string', description: 'the concrete effect on the affected principal or resource' },
    trace: {
      type: 'array',
      items: {
        type: 'object',
        properties: {
          kind: { type: 'string', enum: ['entrypoint', 'propagation', 'sink'] },
          location: { type: 'string', description: 'file:line' },
          note: { type: 'string' },
        },
        required: ['kind', 'location'],
      },
    },
    conditions: { type: 'string' },
    blockers: { type: 'string', description: 'needs_validation only: the missing fact and the owner check that settles it' },
    fix: { type: 'string' },
    regressionTest: { type: 'string' },
    preExisting: { type: 'boolean', description: 'true when the defect is in code the diff did not change' },
  },
  required: ['verdict', 'fingerprint', 'title', 'file', 'issue', 'principal', 'boundary', 'result', 'trace', 'fix', 'preExisting'],
}

const SECURITY_SCHEMA = {
  type: 'object',
  properties: {
    findings: { type: 'array', items: SECURITY_FINDING },
    coverage: {
      type: 'array',
      items: {
        type: 'object',
        properties: {
          boundary: { type: 'string' },
          paths: { type: 'array', items: { type: 'string' } },
          result: { type: 'string', description: 'the invariant settled on this boundary' },
        },
        required: ['boundary', 'paths', 'result'],
      },
    },
    hardening: {
      type: 'array',
      items: {
        type: 'object',
        properties: { note: { type: 'string' }, location: { type: 'string' } },
        required: ['note', 'location'],
      },
    },
  },
  required: ['findings', 'coverage', 'hardening'],
}

const VERDICT_SCHEMA = {
  type: 'object',
  properties: {
    decisions: {
      type: 'array',
      items: {
        type: 'object',
        properties: {
          fingerprint: { type: 'string' },
          decision: { type: 'string', enum: ['confirmed', 'needs_validation', 'rejected'] },
          reason: { type: 'string' },
          finding: { ...SECURITY_FINDING, description: 'the corrected candidate; omit when rejected' },
        },
        required: ['fingerprint', 'decision', 'reason'],
      },
    },
  },
  required: ['decisions'],
}

const REVIEWERS = [
  {
    area: 'Docs',
    type: 'documentation',
    focus: 'documentation that this change makes missing, stale, or now factually wrong',
    p0: 'Missing docs for a public API change, a breaking contract change with no doc update, or docs now wrong in a way that could cause a production incident.',
    p1: 'Missing internal doc updates, outdated examples, stale version references.',
    p2: 'Minor doc improvements, typos, clarifications.',
  },
  {
    area: 'Implementation',
    type: 'implementation',
    focus: 'whether the implementation actually achieves its stated goal — coverage, wiring, integration, logic flow, edge cases',
    p0: 'Logic errors producing wrong behaviour, data loss or corruption, silent failures that lose data, broken error handling on a critical path, missing wiring that stops the feature working.',
    p1: 'Backward-compatibility concerns, unhandled edge cases, architectural issues that should be addressed.',
    p2: 'Alternative approaches, minor improvements, performance considerations.',
  },
  {
    area: 'Quality',
    type: 'quality',
    focus: 'bugs and code quality. Security vulnerabilities belong to the Security reviewer — do not report them. You also own the linter — no other reviewer runs it. Find the lint command in the project docs or Makefile, run it, and report only hits in changed files, in the `linter` field. You also own junk comments — no other reviewer reports them: flag every comment this diff ADDS to a HAND-WRITTEN file that restates the code below it, narrates an obvious constructor/getter/field/const, banners a section, talks about the change itself, or is commented-out code. Generated files are exempt — skip them',
    p0: 'Linter errors, race conditions, resource leaks, crashes or panics, or a comment that is factually wrong about the code it sits on.',
    p1: 'Linter warnings, quality issues, missing error handling, compatibility risks, junk comments the diff adds.',
    p2: 'Style improvements, minor refactors, non-critical linter suggestions, merely redundant comments.',
  },
  {
    area: 'Simplification',
    type: 'simplification',
    focus: 'over-engineering — code that works but is more complex than the problem requires',
    p0: 'Abstractions that actively hide bugs or make the code unmaintainable, or complexity that introduces a correctness risk.',
    p1: 'Premature abstraction, unnecessary indirection, significant duplication that will cost maintenance.',
    p2: 'Minor simplifications, style preferences, small duplication.',
  },
  {
    area: 'Testing',
    type: 'testing',
    focus: 'test coverage and test quality, including tests that pass without verifying anything',
    p0: 'Missing tests on a critical path (payment, auth, data mutation), untested error handling that could cause an incident, or fake tests that pass regardless of the code.',
    p1: 'Missing edge-case coverage, weak assertions, test-quality issues that reduce confidence.',
    p2: 'Minor test improvements, additional assertions, test organisation.',
  },
]

const SECURITY_HUNTERS = [
  {
    key: 'boundary',
    classes: ['Injection', 'Access control', 'Resource and file handling', 'Cryptography and secrets', 'Obvious things'],
  },
  {
    key: 'logic',
    classes: ['Business logic', 'Feature abuse and data leakage', 'Chained vulnerabilities and trust boundaries', 'Wildcard'],
  },
]

const SEVERITY_PRIORITY = { critical: 'P0', high: 'P0', medium: 'P1', low: 'P2', informational: 'P2' }

const { filesCmd, diffCmd, branch, commits, skills = [], testReads = [], goal, security = 'review', docs = 'review' } = args
const ACTIVE = docs === 'skip' ? REVIEWERS.filter((r) => r.type !== 'documentation') : REVIEWERS

const loadLines = (reads) => [
  `## Load these skills first, with your harness's skill tool`,
  skills.length ? skills.join(', ') : '(none resolved — derive them yourself from the changed file extensions)',
  ...(reads.length ? [`Then read these skill references: ${reads.join(', ')}`] : []),
  `Do not judge a single line before they are loaded: their rules decide whether a finding is even valid.`,
]

const changeLines = [
  `## Get the change yourself — it is NOT inlined here`,
  `changed files: ${filesCmd}`,
  `diff:          ${diffCmd}`,
  `Read the whole file around a hunk whenever the diff alone cannot settle a question.`,
  ``,
  `## Context`,
  `branch: ${branch}`,
  goal ? `goal of this change: ${goal}` : `goal of this change: not stated — infer it from the commits.`,
  `commits:`,
  commits,
]

const brief = (r) => [
  `Review ONE concern only: ${r.focus}.`,
  `Report nothing outside that concern — the other concerns have their own reviewer.`,
  ``,
  ...loadLines(r.area === 'Testing' ? testReads : []),
  ``,
  ...changeLines,
  ``,
  `## Priority of each finding`,
  `P0 — ${r.p0}`,
  `P1 — ${r.p1}`,
  `P2 — ${r.p2}`,
  ``,
  `## Comments`,
  `NEVER file a finding whose fix is "add a comment", "add a doc comment", "document this", or "explain this block". Code is self-explanatory by default; the fix for an unreadable hunk is a better name, a smaller function, or a clearer type — never a sentence about it.`,
  `GENERATED FILES ARE EXEMPT from every comment rule: never file a comment finding against a file carrying "Code generated ... DO NOT EDIT." (or its language's equivalent), a mock (**/mocks/*), a protobuf/gRPC stub (*.pb.go, *_grpc.pb.go, *.pb.dart), an OpenAPI/codegen API package, a gen/ package, *_gen.go / *.g.dart / *.freezed.dart, a lockfile, or anything under vendor/. Their comments come from the generator; a hand edit there is wiped on the next regeneration. If the output is wrong, the finding is against the generator, template, or config.`,
  r.area === 'Quality'
    ? `You alone report junk comments the diff ADDS in hand-written files: restating the code, narrating an obvious declaration, section banners, notes about the change itself, commented-out code. P1 by default, P2 when merely redundant, P0 when the comment is wrong about its code.`
    : `Junk comments the diff adds belong to the Quality reviewer. Do not report them.`,
  ``,
  `## Output`,
  `Return findings through the schema. Problems only — no positive observations, no summary of what the change does.`,
  `Every finding needs a real repo-relative file path, a line number where one applies, the impact, and a concrete fix.`,
  `Found nothing? Return an empty findings array. Do not pad.`,
].join('\n')

const securityBrief = (h) => [
  `Your role: hunter. Your ATTACK-CLASSES.md blocks: ${h.classes.join(', ')}.`,
  `The other security hunter owns the remaining blocks; other concerns have their own reviewer.`,
  ``,
  ...loadLines([]),
  `Then load security-audit and follow your agent instructions for the hunter role.`,
  ``,
  ...changeLines,
  ``,
  `## Scope`,
  `A scoped run over the change: the diff plus the entry surfaces, controls, sinks and parallel paths that reach it, unchanged code included. Mark a defect in unchanged code preExisting. Static only.`,
  ``,
  `## Output`,
  `Return findings, coverage and hardening through the schema. Do not set a priority — the workflow derives it from verdict and severity.`,
  `A finding whose fix is "add a comment" is never valid. Junk comments belong to the Quality reviewer.`,
  `Found nothing? Return an empty findings array, and still list the boundaries you checked in coverage.`,
].join('\n')

const verifierBrief = (candidates) => [
  `Your role: verifier. Try to refute each security candidate below. You did not write them.`,
  ``,
  ...loadLines([]),
  `Then load security-audit and follow your agent instructions for the verifier role.`,
  ``,
  ...changeLines,
  ``,
  `## Candidates`,
  JSON.stringify(candidates, null, 2),
  ``,
  `## Output`,
  `Return exactly one decision per candidate fingerprint through the schema. For confirmed and needs_validation, return the corrected candidate in finding.`,
].join('\n')

const securityPriority = (f) => {
  const priority = f.verdict === 'needs_validation' ? 'P1' : SEVERITY_PRIORITY[f.severity] || 'P1'
  return f.preExisting && priority === 'P0' ? 'P1' : priority
}

const securityReport = (found, verdicts) => {
  const decided = new Map((verdicts?.decisions || []).map((d) => [d.fingerprint, d]))
  const kept = []
  const rejected = []
  for (const candidate of found) {
    const d = decided.get(candidate.fingerprint)
    if (d?.decision === 'rejected') {
      rejected.push({ title: candidate.title, reason: d.reason })
      continue
    }
    const finding = d ? { ...candidate, ...(d.finding || {}), verdict: d.decision } : { ...candidate, unverified: true }
    kept.push({
      ...finding,
      priority: securityPriority(finding),
      title: finding.preExisting ? `[pre-existing] ${finding.title}` : finding.title,
    })
  }
  return { kept, rejected }
}

const securityBranch = async () => {
  if (security === 'audit') return null
  const hunts = await parallel(
    SECURITY_HUNTERS.map((h) => () =>
      agent(securityBrief(h), {
        label: `review:Security-${h.key}`,
        phase: 'Review',
        agentType: 'security',
        model: 'claude-opus-5-5',
        effort: 'high',
        schema: SECURITY_SCHEMA,
      }),
    ),
  )
  const huntersFailed = SECURITY_HUNTERS.filter((h, i) => !hunts[i]).map((h) => h.key)
  const done = hunts.filter(Boolean)
  if (!done.length) return { area: 'Security', findings: [], failed: true }

  const byFingerprint = new Map()
  done.flatMap((h) => h.findings).forEach((f) => byFingerprint.has(f.fingerprint) || byFingerprint.set(f.fingerprint, f))
  const candidates = [...byFingerprint.values()]

  const verdicts = candidates.length
    ? await agent(verifierBrief(candidates), {
        label: 'verify:Security',
        phase: 'Verify',
        agentType: 'security',
        model: 'claude-opus-5-5',
        effort: 'high',
        schema: VERDICT_SCHEMA,
      })
    : { decisions: [] }
  const { kept, rejected } = securityReport(candidates, verdicts)

  return {
    area: 'Security',
    findings: kept,
    coverage: done.flatMap((h) => h.coverage),
    hardening: done.flatMap((h) => h.hardening),
    rejected,
    huntersFailed,
    verifierFailed: !verdicts,
  }
}

phase('Review')

const [general, securityResult] = await Promise.all([
  parallel(
    ACTIVE.map((r) => () =>
      agent(brief(r), {
        label: `review:${r.area}`,
        phase: 'Review',
        agentType: r.type,
        model: 'claude-opus-5-5',
        effort: 'high',
        schema: FINDINGS_SCHEMA,
      }).then((res) => (res ? { area: r.area, ...res } : null)),
    ),
  ),
  securityBranch(),
])

const reports = general.map((res, i) => res || { area: ACTIVE[i].area, findings: [], failed: true })
if (securityResult) reports.splice(3, 0, securityResult)

const counts = reports.map((r) => `${r.area}:${r.failed ? 'FAILED' : r.findings.length}`).join(' ')
log(`findings by area — ${counts}`)
if (securityResult?.rejected?.length) log(`security verifier rejected ${securityResult.rejected.length} candidate(s)`)

return {
  reports,
  linter: reports.find((r) => r.area === 'Quality')?.linter || '',
}
```
