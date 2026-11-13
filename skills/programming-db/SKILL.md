---
name: sadensmol-programming-db
description: "PostgreSQL and SQL programming guidance: schema design, naming of tables, columns, indexes and constraints, data types, foreign keys, indexes, queries, transactions and locking, safe migrations, partitioning and pg_partman. Use when (1) designing or changing a schema, table, column, constraint, foreign key or index, (2) writing or reviewing a migration or any `*.sql` file, or planning a data migration (rename, type or unit change, column split, backfill, expand/contract, dual-write), (3) writing, reviewing or optimizing a SQL query or query-builder / repository code that generates SQL, (4) investigating a slow query, an EXPLAIN plan, a sequential scan, lock waits, deadlocks or idle-in-transaction connections, (5) partitioning a table, querying a partitioned one, or adding a foreign key to or from one, (6) configuring pg_partman retention, detach or maintenance, (7) choosing transaction scope, row locks, pagination, upserts or a read replica for a query."
---

# Database Programming (PostgreSQL)

**Precedence.** A project's own database guide wins over this skill. Read it first when the
project has one. This skill fills the gaps.

## Read on demand

**When a trigger below matches what you are about to do, read that file first** — before
you write, review or answer. The summary in this skill is not the full rule.

| When you… | Read |
|---|---|
| write, review or optimize a SQL query or query-builder code, or send a query to a read replica | `references/queries.md` |
| create, change or drop an index, read a plan, fix a slow query, or prove a performance change | `references/indexes.md` |
| write any migration (DDL, backfill, constraint, type change), or rename, split, move or change the type or unit of existing data | `references/migrations.md` — a change that old code cannot read or write follows its three-phase data migration |
| partition a table, query a partitioned table, add an FK to one, or touch pg_partman | `references/partitioning.md` |

## Naming

- Use lowercase `snake_case` for every identifier. Never quote identifiers to keep mixed case.
- Table names are singular (`user`, `order`). Use a plural only when one row is itself a
  collection (`settings`). A many-to-many join table joins both plural names
  (`user_roles`).
- Name constraints and indexes explicitly, never with the generated default:

  | Object | Pattern |
  |---|---|
  | Index | `<table>_<columns>_idx` |
  | Unique index or constraint | `<table>_<columns>_uq` |
  | Foreign key | `<table>_<columns>_<ref_table>_<ref_columns>_fk` |

- A covering or partial index name lists what the index holds, including the `INCLUDE`
  payload (`<table>_<keys>_<payload>_idx`). Do not rename it back to a shorter name.
- Do not use a reserved word or a `tbl_`/Hungarian prefix. Do not abbreviate unless the
  abbreviation is common (`id`, `ip_addr`).

## Schema design

- Give every table a primary key. Use existing columns, including a composite key, when
  they uniquely identify each row and no foreign key or application code needs a separate
  ID. Do not add an unused surrogate `id`.
- A key with a nullable column cannot be a PK. Use a unique index instead: one
  `nulls not distinct` index (PostgreSQL 15+) or partial unique indexes.
- When a surrogate key is needed, prefer `uuid`. For a new key, prefer a time-ordered
  UUIDv7 (`uuidv7()` on PostgreSQL 18+, or generate it in the app). It keeps the PK B-tree
  dense. Use UUIDv4 when the id is public and must not reveal its creation time.
- For an integer key, use `generated always as identity`, never `serial`.
- Every column that references another table has a foreign key. Postgres does not index
  the referencing columns, so add that index when you delete or update parent rows, or
  join on those columns.
- Choose the `ON DELETE` action on purpose. Use the default `NO ACTION` or `RESTRICT` for
  money and audit data. Use `CASCADE` only for rows that have no meaning without the parent.
- Every table has `created_at timestamptz not null default now()`.
- `updated_at timestamptz` is **nullable with no default**, and a `BEFORE UPDATE` trigger
  sets it. NULL means "never updated". A default of `now()` makes that state unreadable.
- Soft delete uses a nullable `deleted_at timestamptz`. Every query and partial index that
  must ignore deleted rows filters `deleted_at is null`.
- Denormalize when a measured query needs it. Then document which column is the copy and
  what keeps it in sync.
- Put cross-row rules in `unique`, `exclude` or `foreign key` constraints. A `CHECK` can
  read only its own row, and its expression must be immutable.

## Data types

| Use | Not | Why |
|---|---|---|
| `timestamptz` | `timestamp` | `timestamp` stores no zone, so arithmetic across zones and DST is wrong. |
| `text`, plus a `CHECK` for a real limit | `varchar(n)`, `char(n)` | An arbitrary length limit breaks later. `char(n)` pads with spaces. |
| `numeric` in main currency units, with the currency in its own column | `money`, `float`, `double precision` | `money` depends on the locale. Floats round. Integer cents overflow for currencies with many decimals. |
| `jsonb` | `json` | `jsonb` can be indexed and compared. |
| `generated always as identity` | `serial` | `serial` complicates ownership and permissions. |
| `boolean` | `0`/`1` integers or `'Y'`/`'N'` | The type states the intent. |

- Use `now()` or `current_timestamp`. Do not use `current_time` (it returns `timetz`).
- Do not use `timestamptz(0)`. It rounds, so a value can land up to half a second in the
  future. Use `date_trunc('second', …)`.

## Transactions and locking

- Hold a transaction only around the writes. Do reads, CPU work and independent lookups
  before it, and do follow-up work after the commit.
- Never hold a transaction open across a network call (RPC, HTTP, a queue). The
  connection stays `idle in transaction`, and hold time × request rate = connections in
  use. No pool size survives a slow downstream call.
- Inside an open transaction, run every query on the transaction's connection. A read on a
  plain connection checks out a second pooled connection while the first one waits.
- Lock rows in a fixed order (for example, by PK) to avoid deadlocks.
- Use `FOR NO KEY UPDATE` instead of `FOR UPDATE` when the key columns do not change. It
  does not block inserts of rows that reference the locked row.
- A work queue on a table uses `select … for update skip locked limit $n`. Use
  `SKIP LOCKED` only for queues, because it gives an inconsistent view of the data.
- Set `statement_timeout`, `lock_timeout` and `idle_in_transaction_session_timeout` for the
  app role. A query with no timeout holds its locks for as long as it runs.
- A client that disconnects cancels its request context, and the driver then rolls back
  the open transaction. Treat that as normal traffic, not as a database error.
