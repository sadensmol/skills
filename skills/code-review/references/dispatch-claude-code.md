# Dispatching the reviewers — Claude Code

Read this only in Claude Code. The neutral instructions live in `SKILL.md`; this file is
one harness's answer to `fan-out` from the runtime contract.

Use the `Workflow` tool with the script below. Do not launch the five agents by hand.

- **You are already opted in.** The `Workflow` tool refuses to run without explicit user
  opt-in; "the user invoked a skill whose instructions tell you to call Workflow" *is* that
  opt-in. Do not ask for permission.
- **Findings come back structured**, so the report is assembled from data rather than from
  five prose blobs.
- **It runs in the background.** It returns a run id immediately and a task notification
  when it finishes; `/workflows` shows live progress.
- **If the `Workflow` tool is unavailable**, launch the same five agents with the `Agent`
  tool in a **single message**, using the same briefs.

## The review workflow

Pass this script to the `Workflow` tool as `script`. It is used unchanged by both the
Local and the PR path — only `args` differ.

```javascript
export const meta = {
  name: 'code-review',
  description: 'Five specialized reviewers over one diff, returning structured P0/P1/P2 findings',
  phases: [
    { title: 'Review', detail: 'docs, implementation, quality, simplification, testing — in parallel' },
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
    focus: 'bugs, security issues, and code quality. You also own the linter — no other reviewer runs it. Find the lint command in the project docs or Makefile, run it, and report only hits in changed files, in the `linter` field',
    p0: 'Security vulnerabilities, linter errors, race conditions, resource leaks, crashes or panics.',
    p1: 'Linter warnings, quality issues, missing error handling, compatibility risks.',
    p2: 'Style improvements, minor refactors, non-critical linter suggestions.',
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

const { filesCmd, diffCmd, branch, commits, skills = [], testSkills = [], goal } = args

const brief = (r) => {
  const load = r.area === 'Testing' ? skills.concat(testSkills) : skills
  return [
    `Review ONE concern only: ${r.focus}.`,
    `Report nothing outside that concern — the other concerns have their own reviewer.`,
    ``,
    `## Load these skills first, with your harness's skill tool`,
    load.length ? load.join(', ') : '(none resolved — derive them yourself from the changed file extensions)',
    `Do not judge a single line before they are loaded: their rules decide whether a finding is even valid.`,
    ``,
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
    ``,
    `## Priority of each finding`,
    `P0 — ${r.p0}`,
    `P1 — ${r.p1}`,
    `P2 — ${r.p2}`,
    ``,
    `## Output`,
    `Return findings through the schema. Problems only — no positive observations, no summary of what the change does.`,
    `Every finding needs a real repo-relative file path, a line number where one applies, the impact, and a concrete fix.`,
    `Found nothing? Return an empty findings array. Do not pad.`,
  ].join('\n')
}

phase('Review')

const raw = await parallel(
  REVIEWERS.map((r) => () =>
    agent(brief(r), {
      label: `review:${r.area}`,
      phase: 'Review',
      agentType: r.type,
      schema: FINDINGS_SCHEMA,
    }).then((res) => (res ? { area: r.area, ...res } : null)),
  ),
)

const reports = raw.map((res, i) => res || { area: REVIEWERS[i].area, findings: [], failed: true })

const counts = reports.map((r) => `${r.area}:${r.failed ? 'FAILED' : r.findings.length}`).join(' ')
log(`findings by area — ${counts}`)

return {
  reports,
  linter: reports.find((r) => r.area === 'Quality')?.linter || '',
}
```
