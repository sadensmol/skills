# Gate for `sadensmol-programming-patterns`

A design-time skill with a deliberately narrow trigger. The negative list wins over the
positive one whenever both look like they match.

## Load it ONLY when the stated goal is one of

1. **Designing NEW functionality that does not exist yet** — a new feature, service,
   subsystem, module, public interface, domain model, or greenfield component. Signals:
   `design`, `let's build`, `new feature`, `new service`, `how should we structure`,
   `architecture for`, `plan the implementation of`, `add <capability> from scratch`.
2. **A LARGE refactoring that restructures existing code** — extracting a layer, breaking
   up a god object, changing how components fit together, untangling a module, replacing
   an architecture. Signals: `big refactor`, `restructure`, `rewrite this module`,
   `split this up`, `extract a layer`, `this is a mess, redo it`, `clean up this architecture`.

## NEVER load it for — absolute

- Debugging, reproducing, or root-causing anything (→ `superpowers:systematic-debugging`).
- Writing, fixing, or running tests of any kind (→ the language skill / `sadensmol-go-integration-tests`).
- Fixing a bug or patching a defect, even a structural-looking one. Badly structured code
  is not a trigger while fixing it.
- Ordinary feature work on existing code: a new field, a new branch, a new endpoint
  modelled on the one beside it, a small local edit.
- Code review (→ `sadensmol-code-review`).
- Reading, exploring, or explaining existing code.
- Anything where the answer is "change these three lines".

**Tie-break:** a request that is both ("refactor this while fixing the bug") is a fix — do
not load. Load only when restructuring is the stated goal on its own.

**Scale test:** a refactor qualifies only if it moves code across type/package/layer
boundaries or changes an interface other code depends on. Renames, extracting one helper,
and tidying one function do not.

**Drift both ways:** a session that becomes design work gains the skill; one that started
as design and became debugging, test writing, or bug fixing must not, and any pattern
advice already in flight stops there.

Reason: in bounded work the existing code's shape is a constraint you inherit; pattern
advice there turns the task into a rewrite.

The skill itself requires announcing every candidate pattern and getting an explicit yes
before applying it — do not shortcut that gate. System-level topology (service
decomposition, deployment, scaling, ADRs) → `sadensmol-system-design` instead.
