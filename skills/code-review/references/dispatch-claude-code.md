# Dispatching the reviewers — Claude Code

Read this only in Claude Code. The neutral instructions live in `SKILL.md`; this file is
one harness's answer to `fan-out` from the runtime contract.

Use the `Workflow` tool with the script below. Do not launch the five agents by hand.

- **You are already opted in.** The `Workflow` tool refuses to run without explicit user
  opt-in; "the user invoked a skill whose instructions tell you to call Workflow" *is* that
  opt-in. Do not ask for permission.
- **The model comes from the agent frontmatter — do NOT pass one per call.** The five agent
  files in `agents/code-reviewer/` carry `model: sonnet` + `effort: high`, which is what this
  harness reviews on. Those files are **shared with OpenCode** (`~/.config/opencode/agents`
  symlinks to the same directory), so an `openai/*` model must never be written into them: it
  belongs in OpenCode's own `opencode.jsonc`, whose per-agent `model`/`variant` overrides the
  frontmatter anyway. A fan-out that fails with "There's an issue with the selected model"
  means that rule was broken — fix the frontmatter rather than overriding `model` in the
  script.
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
    focus: 'bugs, security issues, and code quality. You also own the linter — no other reviewer runs it. Find the lint command in the project docs or Makefile, run it, and report only hits in changed files, in the `linter` field. You also own junk comments — no other reviewer reports them: flag every comment this diff ADDS to a HAND-WRITTEN file that restates the code below it, narrates an obvious constructor/getter/field/const, banners a section, talks about the change itself, or is commented-out code. Generated files are exempt — skip them',
    p0: 'Security vulnerabilities, linter errors, race conditions, resource leaks, crashes or panics, or a comment that is factually wrong about the code it sits on.',
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
