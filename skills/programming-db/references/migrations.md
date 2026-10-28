# Migrations

Contents: file rules · three-phase data migrations · lock safety · safe recipes per
operation · backfills · data fixes

## File rules

- Never edit a migration or a fixture after it is applied anywhere. Add a new file with a
  new timestamp. The tool records applied files by name, so it never applies an edited
  file again.
- Write forward-only migrations when the project does not use down migrations. Leave the
  Down section empty, and undo a change with a new Up migration.
- Squash old migrations only when every environment has applied them.
- A schema change and the code that needs it ship in a safe order. Add first, deploy the
  code, and remove last. Old and new pods run side by side during a rollout, so each
  migration must work with both code versions. A change that breaks old readers or writers
  uses the three-phase data migration below.

## Three-phase data migrations (expand, migrate, contract)

Use this for any change that old code cannot read or write: a rename, a type or unit
change, a split or merge of columns, a move to a new table. The old and the new structure
live side by side until no code uses the old one. Each phase ships as its own release, and
a release can stop at any phase.

### Compatibility rules (MUST FOLLOW)

- Every schema state works with the app version before it and the version after it.
  During a rolling deploy both versions run at once, and a rollback puts the old version
  back on the current schema.
- Never combine two phases in one release. An expand and a contract in the same release
  break the pods that still run the old code.
- Run expand migrations before the new code starts. Run contract migrations only after
  every pod runs code that no longer uses the old structure.
- A new column is nullable or has a non-volatile default, and it has no constraint that
  old code can violate. Old code does not know the column, so it must keep working when
  it writes rows without it.

### Phase 1: expand

1. Add the new column or table next to the old one. Do not change or remove the old one.
2. Deploy code that **dual-writes**: every write sets the old and the new structure, in
   the same transaction. Reads still use the old structure, so behaviour does not change.
3. Old pods still write only the old structure until the rollout ends. Rows they write in
   that window have a stale or empty new value. Either backfill only after the rollout is
   complete, or add a temporary trigger that copies old to new for writers that do not
   dual-write yet. Remove the trigger in phase 3.

### Phase 2: migrate

1. **Backfill** the existing rows in batches, as in *Backfills* below. Each batch only
   touches rows whose new value is still empty (`where new_col is null`). A backfill must
   never overwrite a value that dual-write already set, because that value is newer.
2. Convert with a deterministic, exact rule. For money or other precise values, use
   `numeric` arithmetic, never floats. When a value cannot be converted, stop and report
   it. Do not write a guessed value.
3. **Verify** before anything reads the new structure:
   - Count the rows where the new value is still missing. The count must be 0.
   - Compare old and new for every row, or for a checksum per key range. Do it on a
     replica when the table is large.
   - Optionally run shadow reads: read both, serve the old result, and alert on every
     difference.
4. **Switch reads** to the new structure behind a flag, in stages. Keep dual-writing.
   While writes still reach both structures, a rollback to the old reads loses nothing.
5. Add the constraints that the new structure needs (`NOT NULL`, `CHECK`, FK) with the
   `NOT VALID` then `VALIDATE` recipes below.

### Phase 3: contract

1. Stop writing the old structure. From now on, a rollback past this release cannot
   restore the data that only the new structure holds, so wait until the new reads have
   run in production without errors.
2. Keep the old column for at least one more release, unwritten and unread. Search the
   code, reports, scheduled jobs, views and other services for every remaining reader.
3. Take a backup or snapshot of the old data, then drop the old column or table and any
   sync trigger.
4. Do not skip this phase. Two structures that hold the same fact will drift and corrupt
   reports.

### A twin column for a wider type

A common case is a value that the old type cannot hold (for example, an integer column
that overflows). Add a twin column of the wider type (`amount` → `amount_dec numeric`).
Write both: the twin holds the exact value, and the old column gets the old value or a
documented sentinel when the value does not fit. A write never fails or wraps because of
the old column. Read the twin and fall back to the old column only when the twin is NULL
(a row from before the migration). Once a twin exists, the old column is a write-only
mirror. No read, filter or aggregate may compute with it.

## Lock safety

- Every `ALTER TABLE` waits for an `ACCESS EXCLUSIVE` or another strong lock. While it
  waits in the queue, it blocks every later query on the table, including reads.
- Set `lock_timeout` in each DDL migration (for example `set lock_timeout = '5s'`), and
  retry on timeout. Use a long `statement_timeout` for the migration role, so a legal long
  build is not cut off.
- Keep one risky DDL statement per migration, so a failure is clear and a retry is safe.
- A migration does not call heavy maintenance functions for all objects (for example,
  `partman.run_maintenance()` with no argument). Run maintenance only for the set you changed.

## Safe recipes per operation

| Operation | Unsafe form | Safe form |
|---|---|---|
| Add a column | default from a volatile function (`clock_timestamp()`), which rewrites the table | Add it with no default or with a non-volatile default. That is a metadata-only change. |
| Add `NOT NULL` to an existing column | `set not null` alone, which scans under `ACCESS EXCLUSIVE` | `add constraint c check (col is not null) not valid`, then `validate constraint c` in the next migration, then `set not null` (no scan), then drop `c`. |
| Add a foreign key | plain `add constraint … foreign key` on a large table | `add constraint … not valid`, then `validate constraint` in the next migration. Validation takes only `SHARE UPDATE EXCLUSIVE`. |
| Add a `CHECK` | plain `add constraint … check` | `not valid`, then `validate constraint`. |
| Add a unique constraint (plain table) | `add constraint … unique` | `create unique index concurrently`, then `add constraint … unique using index`. Not supported on partitioned tables. |
| Add an index | `create index` on a plain table | `create index concurrently`. For a partitioned table, see `indexes.md`. |
| Change a column type | `alter column … type` that rewrites the table | Add a new column, dual-write, backfill, move the reads, stop the old writes, drop the old column. A binary-coercible change (`varchar` → `text`) needs no rewrite. |
| Rename a column or table | `rename` while old code still runs | Expand and contract, as for a type change. |
| Drop a column | drop while old code still reads it | Stop reading and writing it in the code first, deploy, then drop it. |
| Add an enum value | — | Add the value, deploy code that handles it, then use it. Postgres cannot remove an enum value. Prefer a `text` column with a `CHECK` when the value set changes often. |
| Remove an old partition | plain `DETACH PARTITION` | `detach partition … concurrently`. It cannot run in a transaction block, and it is not allowed when the table has a default partition. |
| Attach a loaded partition | plain `ATTACH PARTITION` | First add a `CHECK` that matches the partition bounds. The attach then skips its validation scan. Drop the `CHECK` after the attach. |

## Backfills

- Do not backfill in the same transaction as the DDL. Run the backfill in batches by key
  range, each batch in its own short transaction, and throttle it.
- Make the backfill restartable: each batch selects only rows that are not yet done.
- Run `ANALYZE` on the table after a large backfill or a type change, because the column
  statistics are reset.

## Data fixes

- Move rows between partitions, or out of a default partition, in one statement:
  `with moved as (delete from t_default returning *) insert into t select * from moved;`
  With FK-linked tables, move the referenced table first and the referencing table last.
- Run a data fix on a test copy first, and record the affected row count. Compare it with
  the count from the real run.
