# Reviewing Design Documents

Read this when the user asks to review the design documents you produced.

When the user asks to **review** the design document(s) you produced
(requirements, plan, supporting docs, ADRs), do NOT just paste them as text —
open them for inline annotation with **plannotator annotate**:

```bash
# <skill-dir> = this skill's base directory; pwd -P resolves the install symlink
bash "$(cd <skill-dir> && pwd -P)/../task/scripts/plannotator-session.sh" annotate <file-or-folder>
```

- A doc you generate follows `sadensmol-task` → `references/plannotator.md` → *Theme*.

- Pass a single doc (`docs/plans/{date}.md`) or the whole `docs/` folder to
  review several at once.
- The helper opens plannotator in its own agterm session `└─ review <task ID>`
  directly below the current one (outside agterm it runs plannotator directly).
  See `sadensmol-task` → `references/plannotator.md`.
- Run it in the background; it blocks until the user submits annotations, then
  returns them. Address each annotation and re-open if needed. No annotations =
  approved.
