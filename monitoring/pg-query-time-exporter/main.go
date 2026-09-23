package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type queryTimeStats struct {
	AvgExecTimeMs float64
	P90ExecTimeMs float64
	AvgPlanTimeMs float64
	P90PlanTimeMs float64
}

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
			coalesce(percentile_cont(0.9) WITHIN GROUP (ORDER BY mean_plan_time) FILTER (WHERE plans > 0), 0)
		FROM pg_stat_statements
	`).Scan(&s.AvgExecTimeMs, &s.P90ExecTimeMs, &s.AvgPlanTimeMs, &s.P90PlanTimeMs)
	return s, err
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	stats, err := collectQueryTimeStats(ctx)
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
