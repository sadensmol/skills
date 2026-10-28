# Authoring the router and the `sadensmol-` skills

## The generic layer (MUST FOLLOW)

`sadensmol-` skills are the generic layer; project detail lives in the project plugin.
This repository is public, so **no `sadensmol-` skill may hardcode a machine-, user- or
project-specific value**: absolute checkout paths (`/Users/<name>/…`, `/home/<name>/…`),
home directories, tool install locations, tracker sites, team/issue prefixes, or
per-project layout. When such a value is needed, get it in this order:

1. **Derive it at runtime** — from the cwd, `$HOME`, `command -v <tool>`.
2. **Read it from an env var**, with an override taking precedence (e.g. `${PLANNOTATOR:-…}`).
3. **Take it from the project skill** — the `<project>-*` plugin is the single place
   project detail is written down, so defer to it.
4. **Ask the user.** Never guess a path.

When you catch a hardcoded value in a `sadensmol-` skill, replace it with one of those
four — do not just work around it.

## When the user calls out a missed skill load

Fix the rule that missed it. Do not just load the skill and move on — edit the router's
`SKILL.md` so the same hole does not reopen in the next session.

## Adding a routing rule

Edit `skills/router/SKILL.md` in the skills repository directly. Every harness reads it
through a symlink, so the edit is live in the next session.

1. Pick a signal (cwd pattern, file existence, prompt keyword).
2. Add a row to the matching table in the router (Section B for project type, the intent
   table for prompt signals).
3. Save. The change takes effect in the next session that invokes the router.

Keep the router short: it is injected on every prompt. Put a rule's rationale, examples
and long trigger lists in the downstream skill's own description, not here.
