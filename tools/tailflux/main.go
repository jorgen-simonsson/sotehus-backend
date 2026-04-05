package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	host := envOrDefault("INFLUXDB2_HOST", "localhost")
	port := envOrDefault("INFLUXDB2_PORT", "8086")
	org := envOrDefault("INFLUXDB2_ORG", "sotehus")
	bucket := envOrDefault("INFLUXDB2_BUCKET", "sotehus_bucket")
	token := os.Getenv("INFLUXDB2_TOKEN")
	if token == "" {
		user := os.Getenv("INFLUXDB2_USER")
		pass := os.Getenv("INFLUXDB2_PASSWORD")
		if user != "" && pass != "" {
			token = user + ":" + pass
		}
	}

	url := fmt.Sprintf("http://%s:%s", host, port)
	client := influxdb2.NewClient(url, token)
	defer client.Close()

	queryAPI := client.QueryAPI(org)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	fmt.Printf("Tailing InfluxDB at %s (org=%s, bucket=%s)\n", url, org, bucket)
	fmt.Println("Press Ctrl-C to exit")
	fmt.Println()

	lastTime := time.Now().Add(-10 * time.Second)

	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nExiting.")
			return
		default:
		}

		query := fmt.Sprintf(`
from(bucket: "%s")
|> range(start: %s)
|> filter(fn: (r) => r._measurement == "power_monitoring")
|> pivot(rowKey: ["_time"], columnKey: ["_field"], valueColumn: "_value")
|> sort(columns: ["_time"])
`, bucket, lastTime.UTC().Format(time.RFC3339Nano))

		result, err := queryAPI.Query(ctx, query)
		if err != nil {
			if ctx.Err() != nil {
				fmt.Println("\nExiting.")
				return
			}
			fmt.Fprintf(os.Stderr, "Query error: %v\n", err)
			time.Sleep(2 * time.Second)
			continue
		}

		for result.Next() {
			record := result.Record()
			t := record.Time()
			if !t.After(lastTime) {
				continue
			}
			lastTime = t

			skip := map[string]bool{
				"_start":       true,
				"_stop":        true,
				"_time":        true,
				"_measurement": true,
				"result":       true,
				"table":        true,
			}

			var fields []string
			for k, v := range record.Values() {
				if skip[k] {
					continue
				}
				fields = append(fields, fmt.Sprintf("%s=%.4f", k, toFloat(v)))
			}
			sort.Strings(fields)

			loc, _ := time.LoadLocation("Europe/Stockholm")
			fmt.Printf("[%s] %s\n", t.In(loc).Format("15:04:05"), strings.Join(fields, "  "))
		}

		if result.Err() != nil {
			fmt.Fprintf(os.Stderr, "Result error: %v\n", result.Err())
		}

		select {
		case <-ctx.Done():
			fmt.Println("\nExiting.")
			return
		case <-time.After(5 * time.Second):
		}
	}
}

func toFloat(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int64:
		return float64(val)
	default:
		return 0
	}
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
