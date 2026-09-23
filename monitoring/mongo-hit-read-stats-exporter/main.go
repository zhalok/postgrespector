package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type cacheStats struct {
	Requested int64
	ReadInto  int64
}

var leafPageMaxRe = regexp.MustCompile(`leaf_page_max=(\d+)(KB|MB|GB)?`)

var client *mongo.Client
var dbName string
var collName string

// pageSizeBytes is the WiredTiger leaf_page_max for the collection, fixed at collection-creation time.
var pageSizeBytes int64

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

func collectCacheStats(ctx context.Context) (cacheStats, error) {
	var s cacheStats

	var result bson.M
	err := client.Database("admin").RunCommand(ctx, bson.D{
		{Key: "serverStatus", Value: 1},
	}).Decode(&result)
	if err != nil {
		return s, err
	}

	wiredTiger, ok := docToM(result["wiredTiger"])
	if !ok {
		return s, fmt.Errorf("wiredTiger stats not present (requires WiredTiger storage engine)")
	}
	cache, ok := docToM(wiredTiger["cache"])
	if !ok {
		return s, fmt.Errorf("wiredTiger.cache stats not present")
	}

	s.Requested = toInt64(cache["pages requested from the cache"])
	s.ReadInto = toInt64(cache["pages read into cache"])

	return s, nil
}

func collectPageSizeBytes(ctx context.Context) (int64, error) {
	var result bson.M
	err := client.Database(dbName).RunCommand(ctx, bson.D{
		{Key: "collStats", Value: collName},
	}).Decode(&result)
	if err != nil {
		return 0, err
	}

	wiredTiger, ok := docToM(result["wiredTiger"])
	if !ok {
		return 0, fmt.Errorf("wiredTiger stats not present for collection %q", collName)
	}
	creationString, _ := wiredTiger["creationString"].(string)

	match := leafPageMaxRe.FindStringSubmatch(creationString)
	if match == nil {
		return 0, fmt.Errorf("leaf_page_max not found in creationString for collection %q", collName)
	}

	n, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil {
		return 0, err
	}

	switch match[2] {
	case "KB":
		n *= 1024
	case "MB":
		n *= 1024 * 1024
	case "GB":
		n *= 1024 * 1024 * 1024
	}

	return n, nil
}

func docToM(v interface{}) (bson.M, bool) {
	switch d := v.(type) {
	case bson.M:
		return d, true
	case bson.D:
		m := make(bson.M, len(d))
		for _, e := range d {
			m[e.Key] = e.Value
		}
		return m, true
	default:
		return nil, false
	}
}

func toInt64(v interface{}) int64 {
	switch n := v.(type) {
	case int32:
		return int64(n)
	case int64:
		return n
	case float64:
		return int64(n)
	default:
		return 0
	}
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	stats, err := collectCacheStats(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// pages served from cache without going to disk
	hits := stats.Requested - stats.ReadInto

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	fmt.Fprintf(w, "# HELP mongo_wt_cache_hit_pages_total WiredTiger cache pages served from memory without a disk read\n")
	fmt.Fprintf(w, "# TYPE mongo_wt_cache_hit_pages_total counter\n")
	fmt.Fprintf(w, "mongo_wt_cache_hit_pages_total %d\n", hits)

	fmt.Fprintf(w, "# HELP mongo_wt_cache_read_pages_total WiredTiger cache pages read from disk into cache\n")
	fmt.Fprintf(w, "# TYPE mongo_wt_cache_read_pages_total counter\n")
	fmt.Fprintf(w, "mongo_wt_cache_read_pages_total %d\n", stats.ReadInto)

	fmt.Fprintf(w, "# HELP mongo_wt_cache_requested_pages_total WiredTiger cache pages requested from the cache\n")
	fmt.Fprintf(w, "# TYPE mongo_wt_cache_requested_pages_total counter\n")
	fmt.Fprintf(w, "mongo_wt_cache_requested_pages_total %d\n", stats.Requested)

	fmt.Fprintf(w, "# HELP mongo_wt_leaf_page_max_bytes WiredTiger leaf_page_max for collection %q, used to convert page counts to bytes\n", collName)
	fmt.Fprintf(w, "# TYPE mongo_wt_leaf_page_max_bytes gauge\n")
	fmt.Fprintf(w, "mongo_wt_leaf_page_max_bytes %d\n", pageSizeBytes)
}

func main() {
	client = connect()
	defer client.Disconnect(context.Background())

	dbName = envOr("MONGO_DB", "jsonb_experiments")
	collName = envOr("MONGO_COLLECTION", "orders")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var err error
	pageSizeBytes, err = collectPageSizeBytes(ctx)
	if err != nil {
		panic(err)
	}

	http.HandleFunc("/metrics", metricsHandler)
	fmt.Println("mongo-hit-read-stats-exporter listening on :9104")
	if err := http.ListenAndServe(":9104", nil); err != nil {
		panic(err)
	}
}
