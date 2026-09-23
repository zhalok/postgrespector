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

type bufferStats struct {
	UsedBuffers  int64
	DirtyBuffers int64
	TotalBuffers int64
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

func collectBufferStats(ctx context.Context) (bufferStats, error) {
	var s bufferStats
	err := db.QueryRowContext(ctx, `
		SELECT
			count(*) FILTER (WHERE relfilenode IS NOT NULL) AS used_buffers,
			count(*) FILTER (WHERE relfilenode IS NOT NULL AND isdirty) AS dirty_buffers,
			count(*) AS total_buffers
		FROM pg_buffercache
	`).Scan(&s.UsedBuffers, &s.DirtyBuffers, &s.TotalBuffers)
	return s, err
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	stats, err := collectBufferStats(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	fmt.Fprintf(w, "# HELP pg_buffercache_used_buffers Buffers in shared_buffers currently holding a page\n")
	fmt.Fprintf(w, "# TYPE pg_buffercache_used_buffers gauge\n")
	fmt.Fprintf(w, "pg_buffercache_used_buffers %d\n", stats.UsedBuffers)

	fmt.Fprintf(w, "# HELP pg_buffercache_dirty_buffers Buffers in shared_buffers holding a modified page not yet flushed to disk\n")
	fmt.Fprintf(w, "# TYPE pg_buffercache_dirty_buffers gauge\n")
	fmt.Fprintf(w, "pg_buffercache_dirty_buffers %d\n", stats.DirtyBuffers)

	fmt.Fprintf(w, "# HELP pg_buffercache_total_buffers Total number of buffer slots in shared_buffers\n")
	fmt.Fprintf(w, "# TYPE pg_buffercache_total_buffers gauge\n")
	fmt.Fprintf(w, "pg_buffercache_total_buffers %d\n", stats.TotalBuffers)
}

func main() {
	db = connect()
	defer db.Close()

	http.HandleFunc("/metrics", metricsHandler)
	fmt.Println("pg-buffer-usage-exporter listening on :9108")
	if err := http.ListenAndServe(":9108", nil); err != nil {
		panic(err)
	}
}
