package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type sizeStats struct {
	TotalBytes int64
}

type collStatsResult struct {
	TotalSize int64 `bson:"totalSize"`
}

var client *mongo.Client
var dbName string

func connect() *mongo.Client {
	uri := envOr("MONGO_URI", "mongodb://mongo:mongo@mongodb:27017")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		panic(err)
	}
	if err := c.Ping(ctx, nil); err != nil {
		panic(err)
	}

	return c
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func collectSizeStats(ctx context.Context) (sizeStats, error) {
	var s sizeStats

	var result collStatsResult
	err := client.Database(dbName).RunCommand(ctx, bson.D{
		{Key: "collStats", Value: "orders"},
	}).Decode(&result)
	if err != nil {
		return s, err
	}

	s.TotalBytes = result.TotalSize

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

	fmt.Fprintf(w, "# HELP mongo_total_size_bytes Total on-disk size of the orders collection, including indexes, in bytes\n")
	fmt.Fprintf(w, "# TYPE mongo_total_size_bytes gauge\n")
	fmt.Fprintf(w, "mongo_total_size_bytes{collection=\"orders\"} %d\n", stats.TotalBytes)
}

func main() {
	client = connect()
	defer client.Disconnect(context.Background())
	dbName = envOr("MONGO_DB", "jsonb_experiments")

	http.HandleFunc("/metrics", metricsHandler)
	fmt.Println("mongo-total-size-exporter listening on :9106")
	if err := http.ListenAndServe(":9106", nil); err != nil {
		panic(err)
	}
}
