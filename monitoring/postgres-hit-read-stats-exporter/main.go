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

type dbStats struct {
	BlksHit  int64
	BlksRead int64
}

var db *sql.DB

// BlkSizeBytes is the logical page size of postgres.
var BlkSizeBytes int64

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

func fetchBlkSizeBytes(ctx context.Context) (int64, error) {
	var blockSize int64
	err := db.QueryRowContext(ctx, `SELECT current_setting('block_size')::bigint`).Scan(&blockSize)
	return blockSize, err
}

func collectDBStats(ctx context.Context) (dbStats, error) {
	var s dbStats
	err := db.QueryRowContext(ctx, `
		SELECT blks_hit, blks_read
		FROM pg_stat_database
		WHERE datname = current_database()
	`).Scan(&s.BlksHit, &s.BlksRead)
	return s, err
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	stats, err := collectDBStats(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	fmt.Fprintf(w, "# HELP pg_stat_database_blks_hit_total Buffer cache hits (memory) for the database\n")
	fmt.Fprintf(w, "# TYPE pg_stat_database_blks_hit_total counter\n")
	fmt.Fprintf(w, "pg_stat_database_blks_hit_total %d\n", stats.BlksHit)

	fmt.Fprintf(w, "# HELP pg_stat_database_blks_read_total Disk blocks read for the database\n")
	fmt.Fprintf(w, "# TYPE pg_stat_database_blks_read_total counter\n")
	fmt.Fprintf(w, "pg_stat_database_blks_read_total %d\n", stats.BlksRead)

	fmt.Fprintf(w, "# HELP pg_block_size_bytes Postgres block_size setting, used to convert block counts to bytes\n")
	fmt.Fprintf(w, "# TYPE pg_block_size_bytes gauge\n")
	fmt.Fprintf(w, "pg_block_size_bytes %d\n", BlkSizeBytes)
}

func main() {
	db = connect()
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var err error
	BlkSizeBytes, err = fetchBlkSizeBytes(ctx)
	if err != nil {
		panic(err)
	}

	http.HandleFunc("/metrics", metricsHandler)
	fmt.Println("postgres-hit-read-stats-exporter listening on :9101")
	if err := http.ListenAndServe(":9101", nil); err != nil {
		panic(err)
	}
}
