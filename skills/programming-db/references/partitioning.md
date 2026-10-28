# PostgreSQL partitioning

Contents: when and how to partition · bounding every query · test data and pruning ·
foreign keys between partitioned tables · pg_partman configuration · pg_partman behaviour

## When and how to partition

- Partition a table that grows without limit, when most queries read a recent time window
  or when old data leaves together. Choose the partition key from the column that most
  queries filter on, usually `created_at`.
- The partition key is part of every primary key and unique constraint. A unique index is
  enforced per partition only, so uniqueness across partitions needs a key that already
  includes the partition key (or a globally unique id such as a UUID).
- Create the table, its partitioning and its pg_partman config in the same migration.
- Match the interval to the usual query window. A monthly interval suits month-sized
  windows. A wider window (for example, a 3-month reconciliation) suits a 3-month interval.
- Keep the partition count moderate. The planner handles a few thousand partitions when
  pruning leaves only a few, but planning time and memory grow with the partitions a
  query touches.
- Re-partitioning a large table is slow. Decide the key and the interval early.

## Bound every query by the partition key (MUST FOLLOW)

- Each `SELECT`, `UPDATE` and `DELETE` on a partitioned table filters the partition key.
  Without a bound, the query reads every partition.
- The bound is one of:
  - a fixed window that matches the use case (for example, the last month);
  - a timestamp the caller already holds (the parent row's `created_at`, the row's own
    `created_at` from an earlier read);
  - an explicit `from`/`to` from the API, with a default window.
- A derived bound is often tighter than a fixed one. A child row is never older than its
  parent, so `child.created_at >= parent.created_at` is a valid lower bound.
- When a query cannot be bounded, its repository method says why in a comment. A bound
  can be wrong on purpose: for example, a duplicate check where a missed old row would
  allow a double payment.
- A bound on a joined table does not prune this table. Each partitioned table in the
  query needs its own bound.
- An `EXISTS` subquery can turn into a semi join or a hash join that scans every partition
  of the inner table. Add a fixed partition-key bound inside the subquery. Or add
  `OFFSET 0`, which keeps it a per-row lookup on the row's own partition.
- Prove pruning with `EXPLAIN ANALYZE`. Pruned partitions show `(never executed)`, and
  init-time pruning shows `Subplans Removed`. A plain `EXPLAIN` cannot show run-time
  pruning when the bound comes from another row.
- Pruning does not replace an index. See `indexes.md`.

## Test data and pruning

Fixtures often carry an old, fixed `created_at`, older than the query window. Bypass the
window for test rows only, never for production rows.

- Mark string test ids with a fixed marker (for example, a `test_` prefix), and skip the
  window only when the id carries it.
- For UUID ids, keep a small registry of test ids, and skip the window only when the
  requested id is in the registry.
- **Never write `created_at >= $t or id in (…test ids…)`.** A matching row could have any
  `created_at`, so Postgres cannot prune, and the query scans all history. Decide in the
  code which query to send instead.

## Foreign keys between partitioned tables (MUST FOLLOW)

Design the FKs so that every old partition can still be detached. Check this when you
create the first FK between two time-partitioned tables, not when retention first runs in
production. By then, the fix needs a data rebuild.

**How Postgres blocks the detach.** The example is `orders` ← `order_items`. Both are
partitioned by month, and `order_items (order_id, order_created_at)` references
`orders (id, created_at)` on the parent table.

- `DETACH PARTITION orders_pM` fails while any row references it:
  `removing partition … violates foreign key constraint …`.
- A parent-level FK depends on every referenced partition. A `DROP` of a referenced
  partition fails too.
- A detached partition that is kept as a table (pg_partman `retention_keep_table=true`)
  keeps its FK to the live parent. So `orders_pM` can never detach while the archived
  `order_items_pM` holds rows. Detach order does not help: detaching the leaf first leaves
  the same FK on the archive.
- Cross-period references make it worse. A child row gets `created_at = now()`, but its
  parent was created earlier. If the child table is partitioned by its own `created_at`,
  `order_items_pM+1` references `orders_pM`, and no month boundary is clean.
- A row in a default partition is checked only against the referenced table's default
  partition. When no matching partition exists, inserts land in `_default` and fail the FK
  even though the referenced row exists.

**Approach 1: co-partition by the root key.** Use it when old partitions must be detached
and the FKs must stay.

- Copy the root row's partition key into every dependent table (`order_created_at`), and
  partition each dependent table by that column, not by its own `created_at`.
- Put the root key in each dependent PK: `primary key (id, order_created_at)`. A table that
  is itself referenced also needs `unique (id, order_created_at)` before the downstream FK.
- Create the FKs leaf to leaf for the same period: `order_items_pM` → `orders_pM`. Do not
  create the FK on the parent tables.
- Result: every row of one root lands in the same period, in every table. Each period then
  detaches in any order. The archived tables keep their FKs, the live FKs stay enforced,
  and orphan inserts are rejected. This was tested on PG 15 with pg_partman 5.4.3.
- Costs:
  - Existing tables must be rebuilt with a data copy.
  - PK and `ON CONFLICT` targets change.
  - Every lookup on a dependent table needs a root-key bound, or it loses pruning.
  - Do not use a default partition on the dependent tables.
- pg_partman does not create partition-level FKs. It creates a new child with
  `LIKE … INCLUDING CONSTRAINTS`, which copies no FKs, and then attaches it. Something must
  add each new leaf's FKs in the same transaction as the attach. This hook was not tested.
  Prove it before you rely on it.
- Set pg_partman `maintenance_order` from root to leaf, so the root period exists before
  its dependents.

**Approach 2: do not detach.** Use it when the archive stays in the same database.

- With `retention_keep_table=true`, a detach frees no disk and deletes no data. It only
  hides the rows from queries through the parent table. If that is the only reason to
  detach, set `retention = NULL` on every FK-linked set. pg_partman then skips the detach
  step but still creates future partitions (`premake`, `infinite_time_partitions`).
- A new table with an FK to a partitioned table starts with `retention` unset.
- The change only updates rows in `partman.part_config`. It does not rewrite tables or
  change FKs. Do not call `run_maintenance()` in the same migration, because it runs
  maintenance on all sets.
- `retention = NULL` stops new detaches only. Partitions detached before the change stay
  detached. List them, and reattach them only if a person decides to.
- Bound every query on these tables by the partition key. The table keeps growing, so plan
  real archival as a separate task.

**Do not use these fixes:**

- Dropping the FKs, or an event trigger that drops FKs from the detached tables. This loses
  referential integrity.
- A custom detach procedure or a cron job beside pg_partman. Let pg_partman own the
  partition life cycle.
- `retention_keep_table=false` alone. It deletes the data, and with FKs between sets it
  still needs pg_partman 5.5 (`detach_before_drop`).

## pg_partman configuration

```sql
select partman.create_parent(
    p_parent_table := '<schema>.<table>',
    p_control := 'created_at',
    p_interval := '1 month',
    p_premake := 3,
    p_start_partition := date_trunc('month', current_timestamp)::text
);
```

- Always set `infinite_time_partitions = true` on transactional tables. Without it,
  pg_partman stops creating partitions when no recent data exists, and later inserts fall
  into `_default`.
- `partman.part_config.parent_table` stores the **unquoted** name `schema.table`, even for
  a schema name that needs quotes in SQL (`my-schema.table`). A `WHERE parent_table =
  '"my-schema".table'` matches 0 rows and reports no error. Check the affected row count.
- Set `retention` only on sets that no other partitioned table references (see above).

## pg_partman behaviour

- `run_maintenance()` creates partitions from the last existing boundary, not from
  `now()`. It backfills every missing partition up to `now()` plus `premake` intervals.
  Call it for the one set you fixed, after a `part_config` repair.
- `run_maintenance()` never moves data out of `_default`. Move it yourself with
  `delete … returning` into the parent (see `migrations.md`), the referenced table first.
- `run_maintenance_proc` commits after each partition set. Sets that ran before an error
  stay committed.
- On 5.4.3, an error stops the remaining sets of that run. 5.5.0 continues with the next set.
- Managed Postgres services pin the pg_partman version per engine version. Check the
  version in the target environment. Do not assume it is the same as in local Docker.
- Test a retention change red-first. Seed an old partition that has a referencing row, run
  pg_partman maintenance, and assert that it no longer fails.
