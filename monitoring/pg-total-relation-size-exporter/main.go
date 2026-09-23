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

type sizeStats struct {
	TotalBytes int64
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

func collectSizeStats(ctx context.Context) (sizeStats, error) {
	var s sizeStats

	if err := db.QueryRowContext(ctx, `SELECT pg_total_relation_size('orders')`).Scan(&s.TotalBytes); err != nil {
		return s, err
	}

	return s, nil
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	stats, err := collectSizeStats(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	fmt.Fprintf(w, "# HELP pg_total_relation_size_bytes Total on-disk size of the orders table, including indexes and TOAST, in bytes\n")
	fmt.Fprintf(w, "# TYPE pg_total_relation_size_bytes gauge\n")
	fmt.Fprintf(w, "pg_total_relation_size_bytes{table=\"orders\"} %d\n", stats.TotalBytes)
}

func main() {
	db = connect()
	defer db.Close()

	http.HandleFunc("/metrics", metricsHandler)
	fmt.Println("pg-total-relation-size-exporter listening on :9105")
	if err := http.ListenAndServe(":9105", nil); err != nil {
		panic(err)
	}
}
