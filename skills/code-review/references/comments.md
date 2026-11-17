# Comments

Read this when you judge, decline or file a comment finding, or check a file for the generated-file exemption.

Working code needs no prose around it. The default for any change is **zero comments** —
a better name, a smaller function, or a clearer type says it better than a sentence above
it. This binds the review in two directions.

**SCOPE — hand-written code only. Generated files are exempt (MUST FOLLOW).** Never
file a comment finding against a generated file: its comments come from its generator, so
"delete this banner" or "this comment restates the code" is noise the author cannot act on
without a hand edit that the next regeneration wipes. Skip a changed path when it carries
the `// Code generated ... DO NOT EDIT.` header (or its language's equivalent), or when it
is a mock (`**/mocks/*`), a protobuf/gRPC stub (`*.pb.go`, `*_grpc.pb.go`, `*.pb.dart`), an
OpenAPI/codegen API package, a `gen/` package, a `*_gen.go` / `*.g.dart` / `*.freezed.dart`
file, a lockfile, or anything under `vendor/`. If the generated output is genuinely wrong,
the finding is against the **generator, template, or config** — never the emitted file. The
exemption covers the comment rules only; a generated file committed where it should be
gitignored is still a **[Hygiene]** finding.

**1. Never propose adding a comment.** No reviewer, in any area, may file a finding whose
fix is "add a comment", "add a doc comment", "document this method", or "explain this
block". It is the most common junk finding this skill produces, and the language skills
already forbid the code it asks for. If a hunk is genuinely unreadable, the finding is
*rename it* / *split it* / *make the type say it* — never *narrate it*.

**2. Flag the junk comments the diff adds.** A comment the change introduces is a defect
when it:

- restates the line below it (`// increment the counter` over `count++`);
- narrates an obvious constructor, getter, setter, field, or const;
- is a section banner (`// ---- helpers ----`) or a decorative divider;
- talks about the change rather than the code (`// new in this PR`, `// was: X`, a TODO
  about work this PR already finished);
- is commented-out code.

The **Quality** reviewer owns this — no other area reports it, so one junk comment
produces one finding, not six. Default **P1**; P2 when the comment is merely redundant
rather than actively distracting; **P0** when it is factually wrong about the code it
sits on, because a lying comment is worse than no comment.

A comment survives only when it carries what the code cannot: the **why** — a non-obvious
constraint, a workaround with its reason or link, a spec rule the reader would otherwise
break. Where a workspace mandates doc comments on an exported API, that workspace's skill
wins; otherwise the loaded language skill is the authority on the exact rule.
