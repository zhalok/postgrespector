package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type queryTimeStats struct {
	AvgExecTimeMs float64
	P90ExecTimeMs float64
	AvgPlanTimeMs float64
	P90PlanTimeMs float64

	AvgSharedBlksHit     float64
	AvgSharedBlksRead    float64
	AvgSharedBlksDirtied float64
	AvgSharedBlksWritten float64
}

type perQueryStats struct {
	QueryID           int64
	Query             string
	Calls             int64
	MeanExecTimeMs    float64
	MeanPlanTimeMs    float64
	Rows              int64
	SharedBlksHit     int64
	SharedBlksRead    int64
	SharedBlksDirtied int64
	SharedBlksWritten int64
}

// topQueryLimit caps the per-query table to the slowest query shapes by mean
// execution time, keeping Prometheus label cardinality bounded.
const topQueryLimit = 10

var db *sql.DB

func connect() *sql.DB {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		envOr("PGHOST", "postgres"),
		envOr("PGPORT", "5432"),
		envOr("PGUSER", "postgres"),
		envOr("PGPASSWORD", "postgres"),
		envOr("PGDATABASE", "postgres"),
	)

	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		panic(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.PingContext(ctx); err != nil {
		panic(err)
	}

	return conn
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// collectQueryTimeStats aggregates across query shapes in pg_stat_statements.
// p90 here is the 90th percentile of per-query mean times (i.e. "p90 across
// query shapes"), not a true per-execution percentile -- pg_stat_statements
// only stores aggregates (mean/stddev/min/max) per query, not raw samples.
func collectQueryTimeStats(ctx context.Context) (queryTimeStats, error) {
	var s queryTimeStats
	err := db.QueryRowContext(ctx, `
		SELECT
			coalesce(sum(total_exec_time) / nullif(sum(calls), 0), 0),
			coalesce(percentile_cont(0.9) WITHIN GROUP (ORDER BY mean_exec_time), 0),
			coalesce(sum(total_plan_time) / nullif(sum(plans), 0), 0),
			coalesce(percentile_cont(0.9) WITHIN GROUP (ORDER BY mean_plan_time) FILTER (WHERE plans > 0), 0),
			coalesce(sum(shared_blks_hit) / nullif(sum(calls), 0), 0),
			coalesce(sum(shared_blks_read) / nullif(sum(calls), 0), 0),
			coalesce(sum(shared_blks_dirtied) / nullif(sum(calls), 0), 0),
			coalesce(sum(shared_blks_written) / nullif(sum(calls), 0), 0)
		FROM pg_stat_statements
	`).Scan(
		&s.AvgExecTimeMs, &s.P90ExecTimeMs, &s.AvgPlanTimeMs, &s.P90PlanTimeMs,
		&s.AvgSharedBlksHit, &s.AvgSharedBlksRead, &s.AvgSharedBlksDirtied, &s.AvgSharedBlksWritten,
	)
	return s, err
}

// collectPerQueryStats returns the slowest query shapes by mean execution
// time, for a per-query breakdown table (as opposed to the cross-query
// aggregates in collectQueryTimeStats).
func collectPerQueryStats(ctx context.Context) ([]perQueryStats, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT
			queryid,
			query,
			calls,
			mean_exec_time,
			mean_plan_time,
			rows,
			shared_blks_hit,
			shared_blks_read,
			shared_blks_dirtied,
			shared_blks_written
		FROM pg_stat_statements
		ORDER BY mean_exec_time DESC
		LIMIT $1
	`, topQueryLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []perQueryStats
	for rows.Next() {
		var s perQueryStats
		if err := rows.Scan(
			&s.QueryID, &s.Query, &s.Calls, &s.MeanExecTimeMs, &s.MeanPlanTimeMs, &s.Rows,
			&s.SharedBlksHit, &s.SharedBlksRead, &s.SharedBlksDirtied, &s.SharedBlksWritten,
		); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// sanitizeLabel escapes a string for use as a Prometheus exposition-format
// label value and truncates it so a single query text can't blow up the
// response body.
func sanitizeLabel(s string) string {
	const maxLen = 200
	r := []rune(s)
	if len(r) > maxLen {
		s = string(r[:maxLen]) + "..."
	}
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}

func writePerQueryMetrics(w http.ResponseWriter, queries []perQueryStats) {
	metrics := []struct {
		name string
		help string
		val  func(perQueryStats) float64
	}{
		{"pg_query_stats_calls", "Number of calls for this query shape", func(s perQueryStats) float64 { return float64(s.Calls) }},
		{"pg_query_stats_mean_exec_time_ms", "Mean execution time for this query shape, in milliseconds", func(s perQueryStats) float64 { return s.MeanExecTimeMs }},
		{"pg_query_stats_mean_plan_time_ms", "Mean planning time for this query shape, in milliseconds", func(s perQueryStats) float64 { return s.MeanPlanTimeMs }},
		{"pg_query_stats_rows", "Total rows returned/affected by this query shape", func(s perQueryStats) float64 { return float64(s.Rows) }},
		{"pg_query_stats_shared_blks_hit", "Total shared buffer cache hits for this query shape", func(s perQueryStats) float64 { return float64(s.SharedBlksHit) }},
		{"pg_query_stats_shared_blks_read", "Total shared buffer disk reads for this query shape", func(s perQueryStats) float64 { return float64(s.SharedBlksRead) }},
		{"pg_query_stats_shared_blks_dirtied", "Total shared buffers dirtied for this query shape", func(s perQueryStats) float64 { return float64(s.SharedBlksDirtied) }},
		{"pg_query_stats_shared_blks_written", "Total shared buffers written for this query shape", func(s perQueryStats) float64 { return float64(s.SharedBlksWritten) }},
	}

	for _, m := range metrics {
		fmt.Fprintf(w, "# HELP %s %s\n", m.name, m.help)
		fmt.Fprintf(w, "# TYPE %s gauge\n", m.name)
		for _, q := range queries {
			fmt.Fprintf(w, "%s{queryid=\"%d\",query=\"%s\"} %f\n", m.name, q.QueryID, sanitizeLabel(q.Query), m.val(q))
		}
	}
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	stats, err := collectQueryTimeStats(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	perQuery, err := collectPerQueryStats(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	fmt.Fprintf(w, "# HELP pg_query_avg_exec_time_ms Average query execution time across all tracked statements, in milliseconds\n")
	fmt.Fprintf(w, "# TYPE pg_query_avg_exec_time_ms gauge\n")
	fmt.Fprintf(w, "pg_query_avg_exec_time_ms %f\n", stats.AvgExecTimeMs)

	fmt.Fprintf(w, "# HELP pg_query_p90_exec_time_ms 90th percentile of per-query-shape mean execution time, in milliseconds\n")
	fmt.Fprintf(w, "# TYPE pg_query_p90_exec_time_ms gauge\n")
	fmt.Fprintf(w, "pg_query_p90_exec_time_ms %f\n", stats.P90ExecTimeMs)

	fmt.Fprintf(w, "# HELP pg_query_avg_plan_time_ms Average query planning time across all tracked statements, in milliseconds\n")
	fmt.Fprintf(w, "# TYPE pg_query_avg_plan_time_ms gauge\n")
	fmt.Fprintf(w, "pg_query_avg_plan_time_ms %f\n", stats.AvgPlanTimeMs)

	fmt.Fprintf(w, "# HELP pg_query_p90_plan_time_ms 90th percentile of per-query-shape mean planning time, in milliseconds\n")
	fmt.Fprintf(w, "# TYPE pg_query_p90_plan_time_ms gauge\n")
	fmt.Fprintf(w, "pg_query_p90_plan_time_ms %f\n", stats.P90PlanTimeMs)

	fmt.Fprintf(w, "# HELP pg_query_avg_shared_blks_hit Average shared buffer cache hits per query execution, across all tracked statements\n")
	fmt.Fprintf(w, "# TYPE pg_query_avg_shared_blks_hit gauge\n")
	fmt.Fprintf(w, "pg_query_avg_shared_blks_hit %f\n", stats.AvgSharedBlksHit)

	fmt.Fprintf(w, "# HELP pg_query_avg_shared_blks_read Average shared buffer disk reads per query execution, across all tracked statements\n")
	fmt.Fprintf(w, "# TYPE pg_query_avg_shared_blks_read gauge\n")
	fmt.Fprintf(w, "pg_query_avg_shared_blks_read %f\n", stats.AvgSharedBlksRead)

	fmt.Fprintf(w, "# HELP pg_query_avg_shared_blks_dirtied Average shared buffers dirtied per query execution, across all tracked statements\n")
	fmt.Fprintf(w, "# TYPE pg_query_avg_shared_blks_dirtied gauge\n")
	fmt.Fprintf(w, "pg_query_avg_shared_blks_dirtied %f\n", stats.AvgSharedBlksDirtied)

	fmt.Fprintf(w, "# HELP pg_query_avg_shared_blks_written Average shared buffers written per query execution, across all tracked statements\n")
	fmt.Fprintf(w, "# TYPE pg_query_avg_shared_blks_written gauge\n")
	fmt.Fprintf(w, "pg_query_avg_shared_blks_written %f\n", stats.AvgSharedBlksWritten)

	writePerQueryMetrics(w, perQuery)
}

func main() {
	db = connect()
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, `CREATE EXTENSION IF NOT EXISTS pg_stat_statements`); err != nil {
		panic(err)
	}

	http.HandleFunc("/metrics", metricsHandler)
	fmt.Println("pg-query-time-exporter listening on :9107")
	if err := http.ListenAndServe(":9107", nil); err != nil {
		panic(err)
	}
}
