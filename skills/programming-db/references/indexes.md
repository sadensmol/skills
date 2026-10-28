# Indexes and query plans

Contents: design rules · composite key order · partial indexes · covering indexes ·
range and expression keys · safe creation · checks after a change

## Design rules

- Design each index for one query shape. Write down the query it serves.
- Create the indexes a new table needs in the migration that creates it.
- Partition pruning chooses *which* partitions a query reads. It does nothing inside a
  partition. A lookup on a non-key column still needs its own index, or each surviving
  partition gets a sequential scan.
- An index on the parent of a partitioned table propagates to every existing and future
  partition.
- A new index on a hot table can change the plans of other queries on that table. Before
  you add one, run the plan tests of every query on the table.

## Composite key order

A B-tree index serves a query only when the query filters its **leading** column. An
index that holds the right columns in the wrong order looks like coverage but gets a
sequential scan.

Order the key columns like this:

1. Equality columns (`owner_id = $1 and currency = $2`).
2. The `ORDER BY` column. The scan then returns rows already sorted, and the `Sort` node
   goes away.
3. A range column, only when it does not break the order you need.

Example: `(owner_id, currency, created_at)` for
`where owner_id = $1 and currency = $2 order by created_at`.

## Partial indexes

- Make the index `WHERE` match the query's constant, low-selectivity filters exactly
  (`deleted_at is null`, `demo = false`, `amount > 0`).
- A poll that filters `… order by created_at limit n` needs **every** constant filter in
  the index predicate. Without one, rows the poll never wants stay in the index, and the
  ordered scan skips a growing pile of them. The poll gets slower over time.
- A partial index shows no `Filter:` line in the plan for the predicate it holds.

## Covering indexes (`INCLUDE`)

- Each row that a query reads costs one random heap fetch for every selected column that
  is not in the index. Over a wide window, that is thousands of reads.
- Add the read-only columns that the query selects or aggregates as `INCLUDE` payload. The
  plan then becomes an `Index Only Scan`.
- `INCLUDE` exactly the columns that the query reads. A column name that does not exist on
  the table aborts the migration.
- `INCLUDE` does not help a plain `Index Scan`. That scan checks non-key columns only after
  it fetches the heap row.
- An index-only scan needs pages marked all-visible. On an append-heavy table that has not
  been vacuumed, the planner picks a `Bitmap Heap Scan`, and the payload is unused. After
  the deploy, check `Heap Fetches`. If it stays high, run a one-off `VACUUM` on the recent
  partitions. Do not run it from a migration.
- Child-partition index names come from the key columns only. A plain index and a partial
  index on the same keys get the same child-name pattern.

## Range and expression keys

- "Low selectivity" describes today's data, not the query. A range filter left out of the
  key costs one heap fetch for each rejected row, and rejected rows can grow without limit
  (for example, expired rows that are never deleted).
- `col is null or col >= $x` cannot be an index range condition. Rewrite it as
  `coalesce(col, 'infinity'::timestamptz) >= $x`, and index that same expression.
- The query must repeat the index expression exactly, with constants inlined. A constant
  sent as a bind parameter is a different expression, and the planner ignores the index.

## Creating and dropping indexes safely

- On a plain table, use `create index concurrently if not exists` and
  `drop index concurrently if exists`. `CONCURRENTLY` cannot run in a transaction, so mark
  the migration as non-transactional for your tool.
- A failed concurrent build leaves an INVALID index. `IF NOT EXISTS` then keeps it without
  a warning. Find it with `select indexrelid::regclass from pg_index where not indisvalid;`,
  drop it, and retry.
- A partitioned table does not support `CREATE INDEX CONCURRENTLY` or
  `DROP INDEX CONCURRENTLY`. A plain `CREATE INDEX` takes `SHARE` (it blocks writes and
  allows reads). `DROP INDEX` takes `ACCESS EXCLUSIVE` (it blocks reads too).
- To replace an index on a partitioned table, **create the new one first, then drop the
  old one**, in one transaction. The long build then runs under `SHARE`, and only the
  short drop takes `ACCESS EXCLUSIVE`. Dropping first blocks every read for the whole build.
- If a parent build blocks writes for too long, build each partition's index concurrently
  first, as a one-off step against the real database. Then the parent `CREATE INDEX`
  (or `create index … on only parent` plus `alter index … attach partition`) only attaches
  them. The child indexes must match the parent definition exactly: columns, `INCLUDE`
  and opclasses. Do not hard-code the partition list in a migration. Other environments
  have a different partition set.

## Checks after a change

- Run `EXPLAIN (ANALYZE, BUFFERS)` with the SQL that the code generates, not a hand-written
  copy.
- Seed enough rows that the target key is a small slice of a large table, and run
  `ANALYZE` before the plan check.
- A plan test asserts that the plan names the index and has no `Seq Scan` (or no
  `SubPlan`). A plan test that cannot fail proves nothing. Watch it fail once without the
  index.
