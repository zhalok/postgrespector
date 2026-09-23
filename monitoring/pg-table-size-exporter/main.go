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

type tableSizeStats struct {
	Table      string
	TableBytes int64
	IndexBytes int64
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

func collectSizeStats(ctx context.Context) ([]tableSizeStats, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT c.relname, pg_relation_size(c.oid), pg_indexes_size(c.oid)
		FROM pg_catalog.pg_class c
		JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relkind = 'r' AND n.nspname = 'public'
		ORDER BY c.relname
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []tableSizeStats
	for rows.Next() {
		var s tableSizeStats
		if err := rows.Scan(&s.Table, &s.TableBytes, &s.IndexBytes); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
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

	fmt.Fprintf(w, "# HELP pg_table_size_bytes Size of the table in bytes\n")
	fmt.Fprintf(w, "# TYPE pg_table_size_bytes gauge\n")
	for _, s := range stats {
		fmt.Fprintf(w, "pg_table_size_bytes{table=%q} %d\n", s.Table, s.TableBytes)
	}

	fmt.Fprintf(w, "# HELP pg_index_size_bytes Combined size of all indexes on the table in bytes\n")
	fmt.Fprintf(w, "# TYPE pg_index_size_bytes gauge\n")
	for _, s := range stats {
		fmt.Fprintf(w, "pg_index_size_bytes{table=%q,index=\"all\"} %d\n", s.Table, s.IndexBytes)
	}
}

func main() {
	db = connect()
	defer db.Close()

	http.HandleFunc("/metrics", metricsHandler)
	fmt.Println("pg-table-size-exporter listening on :9102")
	if err := http.ListenAndServe(":9102", nil); err != nil {
		panic(err)
	}
}
