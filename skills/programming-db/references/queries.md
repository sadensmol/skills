# Queries and read replicas

Read this when you write, review or optimize a SQL query or query-builder code, or
route a query to a read replica.

## Queries

- Select only the columns you use. Every extra column that is not in the index costs one
  heap fetch per row.
- Use half-open ranges for time: `created_at >= $from and created_at < $to`. Do not use
  `BETWEEN` on timestamps, because it includes both ends.
- Use `NOT EXISTS`, not `NOT IN (subquery)`. A NULL in the list makes `NOT IN` return no rows.
- A `LIMIT` needs an `ORDER BY` that gives a unique order. Add the PK as the last sort key.
- Paginate deep lists with a keyset, not `OFFSET`:
  `where (created_at, id) < ($last_created_at, $last_id) order by created_at desc, id desc limit $n`.
  `OFFSET` reads and drops every skipped row, and rows move between pages under inserts.
- An `EXISTS` inside an `OR` runs once per row as a SubPlan. Postgres turns a sublink into
  a semi join or an anti join only when it is a top-level `AND` term. Split the `OR` into
  disjoint branches joined by `UNION ALL`, and put `ORDER BY`/`LIMIT` outside the union.
- A claim or one-shot transition is one conditional `UPDATE`:
  `update … set token = $t where id = $id and token is null`. Check the affected row
  count. Zero rows means another caller won. Do not read and then write.
- Make writes idempotent with a natural unique key and `insert … on conflict (…) do …`.
  A retry must reuse the stored row's key values, including the partition key.
- A query builder that binds every constant as a parameter can break plans. An index
  expression only matches when the query repeats it exactly, with the constant inlined.
  A bound parameter in a select list can also get the type `text`.
- Use `jsonb` operators that an index supports (`@>`, `?`) when the column is indexed.

## Read replicas

- Send background polls, reports and read-only aggregates that tolerate lag to the read
  replica. Keep writes and read-after-write paths on the primary.
- When one component reads and writes, give it two repositories: one on the reader and one
  on the writer.
- You cannot add an index to a physical replica only. An index built on the primary exists
  on every replica.
- Dev and staging often point the reader at the writer. Replica lag then shows up only in
  production.
