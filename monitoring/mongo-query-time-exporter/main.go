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

type queryTimeStats struct {
	AvgExecTimeMs float64
	P90ExecTimeMs float64
	AvgPlanTimeMs float64
	P90PlanTimeMs float64
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

// collectQueryTimeStats aggregates across query shapes recorded in
// system.profile. Like pg_stat_statements, p90 here is the 90th percentile
// of per-query-shape (grouped by namespace) mean times, not a true
// per-execution percentile -- the profiler collection is a capped buffer of
// individual ops, which we roll up ourselves.
func collectQueryTimeStats(ctx context.Context) (queryTimeStats, error) {
	var s queryTimeStats
	profile := client.Database(dbName).Collection("system.profile")

	overall, err := aggregateOverall(ctx, profile)
	if err != nil {
		return s, err
	}
	s.AvgExecTimeMs = overall.avgMillis
	s.AvgPlanTimeMs = overall.avgPlanMs

	perShape, err := aggregatePerShapeP90(ctx, profile)
	if err != nil {
		return s, err
	}
	s.P90ExecTimeMs = perShape.p90Millis
	s.P90PlanTimeMs = perShape.p90PlanMs

	return s, nil
}

type overallStats struct {
	avgMillis float64
	avgPlanMs float64
}

func aggregateOverall(ctx context.Context, coll *mongo.Collection) (overallStats, error) {
	var s overallStats

	cursor, err := coll.Aggregate(ctx, bson.A{
		bson.D{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: nil},
			{Key: "avgMillis", Value: bson.D{{Key: "$avg", Value: "$millis"}}},
			{Key: "avgPlanMicros", Value: bson.D{{Key: "$avg", Value: "$planningTimeMicros"}}},
		}}},
	})
	if err != nil {
		return s, err
	}
	defer cursor.Close(ctx)

	var results []bson.M
	if err := cursor.All(ctx, &results); err != nil {
		return s, err
	}
	if len(results) == 0 {
		return s, nil
	}

	s.avgMillis = toFloat64(results[0]["avgMillis"])
	s.avgPlanMs = toFloat64(results[0]["avgPlanMicros"]) / 1000

	return s, nil
}

type shapeP90Stats struct {
	p90Millis float64
	p90PlanMs float64
}

func aggregatePerShapeP90(ctx context.Context, coll *mongo.Collection) (shapeP90Stats, error) {
	var s shapeP90Stats

	cursor, err := coll.Aggregate(ctx, bson.A{
		bson.D{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$ns"},
			{Key: "avgMillis", Value: bson.D{{Key: "$avg", Value: "$millis"}}},
			{Key: "avgPlanMicros", Value: bson.D{{Key: "$avg", Value: "$planningTimeMicros"}}},
		}}},
		bson.D{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: nil},
			{Key: "p90Millis", Value: bson.D{{Key: "$percentile", Value: bson.D{
				{Key: "input", Value: "$avgMillis"},
				{Key: "p", Value: bson.A{0.9}},
				{Key: "method", Value: "approximate"},
			}}}},
			{Key: "p90PlanMicros", Value: bson.D{{Key: "$percentile", Value: bson.D{
				{Key: "input", Value: "$avgPlanMicros"},
				{Key: "p", Value: bson.A{0.9}},
				{Key: "method", Value: "approximate"},
			}}}},
		}}},
	})
	if err != nil {
		return s, err
	}
	defer cursor.Close(ctx)

	var results []bson.M
	if err := cursor.All(ctx, &results); err != nil {
		return s, err
	}
	if len(results) == 0 {
		return s, nil
	}

	s.p90Millis = firstOf(results[0]["p90Millis"])
	s.p90PlanMs = firstOf(results[0]["p90PlanMicros"]) / 1000

	return s, nil
}

func firstOf(v interface{}) float64 {
	arr, ok := v.(bson.A)
	if !ok || len(arr) == 0 {
		return 0
	}
	return toFloat64(arr[0])
}

func toFloat64(v interface{}) float64 {
	switch n := v.(type) {
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	case float64:
		return n
	default:
		return 0
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

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	fmt.Fprintf(w, "# HELP mongo_query_avg_exec_time_ms Average query execution time across all profiled operations, in milliseconds\n")
	fmt.Fprintf(w, "# TYPE mongo_query_avg_exec_time_ms gauge\n")
	fmt.Fprintf(w, "mongo_query_avg_exec_time_ms %f\n", stats.AvgExecTimeMs)

	fmt.Fprintf(w, "# HELP mongo_query_p90_exec_time_ms 90th percentile of per-namespace mean execution time, in milliseconds\n")
	fmt.Fprintf(w, "# TYPE mongo_query_p90_exec_time_ms gauge\n")
	fmt.Fprintf(w, "mongo_query_p90_exec_time_ms %f\n", stats.P90ExecTimeMs)

	fmt.Fprintf(w, "# HELP mongo_query_avg_plan_time_ms Average query planning time across all profiled operations, in milliseconds\n")
	fmt.Fprintf(w, "# TYPE mongo_query_avg_plan_time_ms gauge\n")
	fmt.Fprintf(w, "mongo_query_avg_plan_time_ms %f\n", stats.AvgPlanTimeMs)

	fmt.Fprintf(w, "# HELP mongo_query_p90_plan_time_ms 90th percentile of per-namespace mean planning time, in milliseconds\n")
	fmt.Fprintf(w, "# TYPE mongo_query_p90_plan_time_ms gauge\n")
	fmt.Fprintf(w, "mongo_query_p90_plan_time_ms %f\n", stats.P90PlanTimeMs)
}

func main() {
	client = connect()
	defer client.Disconnect(context.Background())

	dbName = envOr("MONGO_DB", "jsonb_experiments")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Database(dbName).RunCommand(ctx, bson.D{
		{Key: "profile", Value: 1},
		{Key: "slowms", Value: 0},
	}).Err(); err != nil {
		panic(err)
	}

	http.HandleFunc("/metrics", metricsHandler)
	fmt.Println("mongo-query-time-exporter listening on :9108")
	if err := http.ListenAndServe(":9108", nil); err != nil {
		panic(err)
	}
}
