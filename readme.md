# Postgrespector

A standalone Postgres inspection stack: a single Postgres instance with a
simple `products` table, observed through Prometheus/Grafana (CPU, memory,
disk I/O, cache/buffer stats, query timing) and Loki/Promtail for container
logs.

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

`init-db/init.sql` creates a `products` table with plain, traditional
columns (no JSON/document columns): `product_id`, `sku`, `name`,
`description`, `category`, `price`, `quantity`, `created_at`. There's a
btree index on `category`.

## What's being observed

- Prometheus/Grafana dashboards over `cgroup-exporter` metrics: per-container
  CPU and memory.
- Postgres-side: buffer cache hit ratio / physical reads via
  `pg_stat_database` and `pg_statio_user_tables`, table/index/relation sizes,
  and per-query timing via `pg_stat_statements`.
- Postgres container logs via Loki/Promtail.
