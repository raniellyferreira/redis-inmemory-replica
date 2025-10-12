package main

import (
	"fmt"

	"github.com/raniellyferreira/redis-inmemory-replica/replication"
)

// Example logger to demonstrate batched logging
type exampleLogger struct{}

func (l *exampleLogger) Debug(msg string, fields ...interface{}) {
	// Silent for debug
}

func (l *exampleLogger) Info(msg string, fields ...interface{}) {
	fmt.Printf("INFO: %s", msg)
	for i := 0; i < len(fields); i += 2 {
		if i+1 < len(fields) {
			fmt.Printf(" %v=%v", fields[i], fields[i+1])
		}
	}
	fmt.Println()
}

func (l *exampleLogger) Error(msg string, fields ...interface{}) {
	fmt.Printf("ERROR: %s", msg)
	for i := 0; i < len(fields); i += 2 {
		if i+1 < len(fields) {
			fmt.Printf(" %v=%v", fields[i], fields[i+1])
		}
	}
	fmt.Println()
}

func main() {
	fmt.Println("Redis In-Memory Replica - RDB Logging Demo")
	fmt.Println("==========================================")

	logger := &exampleLogger{}

	// Create RDB stats with small batch size for demo
	stats := replication.NewRDBLoadStats()
	stats.BatchSize = 3 // Log every 3 keys for demo

	fmt.Println("\nDemonstrating batched RDB logging:")
	fmt.Println("----------------------------------")

	// Simulate processing keys
	fmt.Println("Processing keys...")
	stats.RecordKey(0, "string", logger) // Key 1
	stats.RecordKey(0, "string", logger) // Key 2
	stats.RecordKey(0, "hash", logger)   // Key 3 - triggers first log
	stats.RecordKey(1, "list", logger)   // Key 4
	stats.RecordKey(1, "string", logger) // Key 5
	stats.RecordKey(0, "string", logger) // Key 6 - triggers second log

	fmt.Println("\nFinal statistics:")
	fmt.Println("-----------------")
	stats.LogFinal(logger)

	fmt.Println("\nDemo Benefits:")
	fmt.Println("- Reduced log volume: 3 aggregate logs instead of 6 per-key logs")
	fmt.Println("- Performance tracking: Shows processing rate and type distribution")
	fmt.Println("- Database separation: Statistics separated by database number")
	fmt.Println("- Configurable batching: Can adjust batch size and time intervals")
}
