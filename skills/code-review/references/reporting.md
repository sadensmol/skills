# Reporting notes

Read this when you assemble a large report, or when you are tempted to summarize findings instead of listing them.

The findings ARE the deliverable — running the reviewers and then describing
them in prose is the single most common way this skill fails the user. A
sentence like "the review found 1 P0 and 15 P1s, I fixed the ones that were
mine" is **not** a report: the user cannot see what was found, cannot judge a
verdict they never read, and cannot tell a finding you skipped from one that
does not exist.

When the run is large, generating the tables from the workflow's returned JSON
beats retyping them: the return value is `{reports: [{area, findings: [...]}]}`,
so a short script that sorts by `priority` and prints the rows cannot drop or
paraphrase a finding the way hand-transcription does.

## Common mistakes

| Mistake | Why it hurts |
|---------|--------------|
| Inlining the diff into the agent prompts | Sends the diff 7× and overflows on a large branch. Pass `diffCmd`. |
| Hardcoding `main` as the base | Repos on `master`/`develop` silently produce an empty or wrong diff. Resolve the base. |
| Letting each reviewer re-derive the skill list | Six duplicated routing passes. Derive once, pass it in. |
| Reporting CRITICAL/IMPORTANT/SUGGESTED | The scale is P0/P1/P2. Nothing else is a priority. |
| Dropping a reviewer that returned `failed` | A silent gap reads as "clean". Name the area that did not run. |
| Filing "add a doc comment" as a finding | The language skills forbid the comment. The fix for an unreadable hunk is a better name, not a sentence about it. |
| Reviewing in your own context instead of fanning out | One context cannot hold six concerns and the diff. Dispatch. |
