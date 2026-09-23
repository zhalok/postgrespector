# Experiment: JSONB (Postgres) vs Document Store (MongoDB) under mixed read/write load

## Goal

Compare Postgres (`JSONB` columns) against MongoDB as the backing store for a
semi-structured "orders" document, under concurrent writes and paginated bulk
reads, and observe the resulting resource usage (CPU, disk I/O, cache
behavior) via Prometheus/Grafana.

## Topology

Two parallel stacks, defined in `compose.yml`, sharing the same Node/Express
app (`server/index.js`) and business logic (`server/services/orders.service.js`), differing
only in the storage repository selected via the `DB` env var:

- **Postgres stack**: `postgres` → `server-postgres` (port 3001) → `writer-postgres` (writer) + `reader-postgres` (reader)
- **Mongo stack**: `mongodb` → `server-mongo` (port 3002) → `writer-mongo` (writer) + `reader-mongo` (reader)

Both stacks run against the same host machine simultaneously so results are
comparable. Observability is shared: `cgroup-exporter` → `prometheus` →
`grafana`, plus `loki`/`promtail` for logs.

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
- **MongoDB** (`repositories/orders.repository.mongodb.js`): single `orders`
  collection, same shape as a native document, with an index on `timestamp`
  (ascending).

## Write path

`clients/writer/main.go` is a Go load generator: a pool of workers repeatedly POST
new orders to `/orders` (via `services/orders.service.js` →
`createOrder`/`createOrderHandler`), each stamped with `timestamp = now()` at
insert time. Because timestamps are assigned at write time, new rows are
always appended at the "newest" end of the timestamp index — the write
frontier only ever moves forward.

## Read path

`clients/reader/main.go` is a separate Go worker pool that continuously scans
the *entire* table/collection via keyset (cursor) pagination on `timestamp`,
1000 rows per page (`PAGE_SIZE` in both repositories), looping until
`next_cursor` comes back null and then restarting. Each page fetch is a real
round trip (`GET /orders?cursor=...`) so every read call touches fresh data
rather than repeatedly hitting the same cached page.

Both repositories' `getOrders` project a reduced/reshaped view of the
document rather than returning it raw:
- Postgres extracts nested JSONB paths (`customer->>'id'`,
  `financials#>>'{amounts,final_total}'`, etc.) and rebuilds them into new
  JSONB via `jsonb_build_object`, plus a numeric cast.
- Mongo does the analogous reshaping via an aggregation `$project` stage.

This means the read path isn't a trivial index-only scan — it does real
per-row JSON traversal/rebuild work in the database engine, which is part of
what's being measured (CPU cost of JSON extraction), not just I/O.

## Pagination direction (current state)

Both repositories now page **ascending** on `timestamp`:
- Postgres: `WHERE "timestamp" > $cursor ORDER BY "timestamp" ASC`, backed by
  an ascending index (`init-db/init.sql`).
- Mongo: `{ timestamp: { $gt: cursor } }`, `$sort: { timestamp: 1 }`, backed
  by an ascending index (`{ timestamp: 1 }`).

This was a deliberate change from the original descending/`$lt` pagination.
Rationale: writes always land at the *newest* end of the timestamp range, so
an ascending reader starts at the oldest data (cold, likely evicted from
buffer cache/page cache) and only reaches the actively-written "hot" end
after paging through the full history — maximizing disk reads and cache
misses during most of the scan, as opposed to a descending reader which
starts right where the writer is and stays warm for longer.

**Known gap**: the Mongo index creation (`orders.createIndex({ timestamp: 1
})`) is additive — on a database that still has the old `{ timestamp: -1 }`
index from a prior run, both indexes will coexist rather than the old one
being replaced. The old index should be dropped manually if reusing existing
Mongo data.

## What's being observed

- Prometheus/Grafana dashboards over `cgroup-exporter` metrics: per-container
  CPU and memory for each service (postgres, mongodb, server-*, client-*,
  reader-*).
- Postgres-side: buffer cache hit ratio / physical reads, visible via
  `pg_stat_database`, `pg_statio_user_tables`, or `EXPLAIN (ANALYZE,
  BUFFERS)` on the `getOrders` query.
- Expected signal: with ascending pagination against a monotonically
  increasing write frontier, disk I/O and cache-miss rate should be visibly
  higher than with descending pagination, and JSONB path extraction/rebuild
  should show up as elevated CPU time on read queries.
