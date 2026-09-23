# Postgrespector

A Postgres inspection/load-testing stack: a JSONB-backed "orders" store under
concurrent writes and paginated bulk reads, with Prometheus/Grafana observing
the resulting resource usage (CPU, disk I/O, cache behavior).

## Topology

Defined in `compose.yml`: `postgres` → `writer-postgres` (write load) +
`reader-postgres` (read load), observed via `cgroup-exporter` → `prometheus`
→ `grafana`, plus `loki`/`promtail` for logs.

## Data model

An "order" document with deeply nested, variable-shape fields — customer
info, geo/device context, financials with nested discounts, line items,
polymorphic metadata, and an append-only `event_timeline`. This shape is
intentionally nested/irregular to stress JSON(B) extraction rather than flat
columns.

- **Postgres** (`init-db/init.sql`): single `orders` table, typed columns for
  `order_id`/`timestamp`/`status`, and `JSONB` columns for `customer`,
  `financials`, `line_items`, `polymorphic_metadata`, `event_timeline`. One
  btree index on `"timestamp"` (ascending).

## Write path

`clients/writer/main.go` is a Go load generator: a pool of workers repeatedly POST
new orders to `/orders`, each stamped with `timestamp = now()` at
insert time. Because timestamps are assigned at write time, new rows are
always appended at the "newest" end of the timestamp index — the write
frontier only ever moves forward.

## Read path

`clients/reader/main.go` is a separate Go worker pool that continuously scans
the *entire* table via keyset (cursor) pagination on `timestamp`, 1000 rows
per page (`PAGE_SIZE`), looping until `next_cursor` comes back null and then
restarting. Each page fetch is a real round trip (`GET /orders?cursor=...`)
so every read call touches fresh data rather than repeatedly hitting the same
cached page.

`getOrders` projects a reduced/reshaped view of the document rather than
returning it raw: it extracts nested JSONB paths (`customer->>'id'`,
`financials#>>'{amounts,final_total}'`, etc.) and rebuilds them into new
JSONB via `jsonb_build_object`, plus a numeric cast. This means the read path
isn't a trivial index-only scan — it does real per-row JSON traversal/rebuild
work in the database engine, which is part of what's being measured (CPU
cost of JSON extraction), not just I/O.

## Pagination direction (current state)

Reads page **ascending** on `timestamp`: `WHERE "timestamp" > $cursor ORDER
BY "timestamp" ASC`, backed by an ascending index (`init-db/init.sql`).

This was a deliberate change from the original descending/`$lt` pagination.
Rationale: writes always land at the *newest* end of the timestamp range, so
an ascending reader starts at the oldest data (cold, likely evicted from
buffer cache/page cache) and only reaches the actively-written "hot" end
after paging through the full history — maximizing disk reads and cache
misses during most of the scan, as opposed to a descending reader which
starts right where the writer is and stays warm for longer.

## What's being observed

- Prometheus/Grafana dashboards over `cgroup-exporter` metrics: per-container
  CPU and memory for each service (postgres, client-*, reader-*).
- Buffer cache hit ratio / physical reads, visible via `pg_stat_database`,
  `pg_statio_user_tables`, or `EXPLAIN (ANALYZE, BUFFERS)` on the
  `getOrders` query.
- Expected signal: with ascending pagination against a monotonically
  increasing write frontier, disk I/O and cache-miss rate should be visibly
  higher than with descending pagination, and JSONB path extraction/rebuild
  should show up as elevated CPU time on read queries.
