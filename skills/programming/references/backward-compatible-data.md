# Backward-Compatible Data

Read this when you add, rename or change a field of a persisted model or a released
contract. This is the full text of SKILL.md → *Backward-Compatible Data*.

These rules bind released data and contracts, and data the user says must survive. A new,
unreleased change in the current task assumes fresh runs: it has no old data. Add no alias,
fallback, guard, backfill or compatibility path for what only the current task added.

Persisted models and released contracts change only in backward-compatible ways:

- Add a new field as OPTIONAL, and emit it only when it is not at its default. A value
  at its defaults then serialises the same as the old format, so old readers keep
  working.
- Decode leniently: a missing or bad field becomes its default, instead of failing the
  whole decode.
- Store the inverse when it keeps the common case absent (`collapsed` written only when
  true, rather than `expanded` written always).
- Bump an explicit version only for a break a default cannot cover, and migrate on read.

API contract rules (status codes, money, timestamps, versioning) live in
`sadensmol-programming-api`.
