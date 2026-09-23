# Postgrespector

A standalone Postgres inspection stack: a single Postgres instance with a
JSONB-shaped `orders` table, observed through Prometheus/Grafana (CPU,
memory, disk I/O, cache/buffer stats, query timing) and Loki/Promtail for
container logs.

## Topology

Defined in `compose.yml`: `postgres` (with `pg_stat_statements` enabled), a
set of exporters scraping it (`postgres-hit-read-stats-exporter`,
`pg-table-size-exporter`, `pg-total-relation-size-exporter`,
`pg-query-time-exporter`, `cgroup-exporter` for container resource usage),
all fed into `prometheus` → `grafana`. `loki`/`promtail` ship container logs,
surfaced in the "Postgres Logs" dashboard.

There is no application layer — connect to `postgres` directly (`localhost:5432`,
user/password/db `postgres`) to run your own inserts/queries and observe the
effect on the dashboards.

## Data model

`init-db/init.sql` creates an `orders` table: typed columns for
`order_id`/`timestamp`/`status`, and `JSONB` columns for `customer`,
`financials`, `line_items`, `polymorphic_metadata`, `event_timeline`. This
shape is intentionally nested/irregular, useful for exercising JSONB path
extraction (`->>`, `#>>`, `jsonb_build_object`) rather than flat columns.
There's a btree index on `status`.

## What's being observed

- Prometheus/Grafana dashboards over `cgroup-exporter` metrics: per-container
  CPU and memory.
- Postgres-side: buffer cache hit ratio / physical reads via
  `pg_stat_database` and `pg_statio_user_tables`, table/index/relation sizes,
  and per-query timing via `pg_stat_statements`.
- Postgres container logs via Loki/Promtail.
