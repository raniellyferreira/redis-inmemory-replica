// Package main demonstrates monitoring capabilities of the redis-inmemory-replica library
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/raniellyferreira/redis-inmemory-replica"
)

// CustomLogger implements redisreplica.Logger interface
type CustomLogger struct{}

func (l *CustomLogger) Debug(msg string, fields ...redisreplica.Field) {
	fmt.Printf("🔍 DEBUG: %s", msg)
	for _, field := range fields {
		fmt.Printf(" %s=%v", field.Key, field.Value)
	}
	fmt.Println()
}

func (l *CustomLogger) Info(msg string, fields ...redisreplica.Field) {
	fmt.Printf("ℹ️  INFO: %s", msg)
	for _, field := range fields {
		fmt.Printf(" %s=%v", field.Key, field.Value)
	}
	fmt.Println()
}

func (l *CustomLogger) Error(msg string, fields ...redisreplica.Field) {
	fmt.Printf("❌ ERROR: %s", msg)
	for _, field := range fields {
		fmt.Printf(" %s=%v", field.Key, field.Value)
	}
	fmt.Println()
}

// MetricsCollector implements redisreplica.MetricsCollector interface
type MetricsCollector struct {
	syncDurations     []time.Duration
	commandCounts     map[string]int64
	networkBytes      int64
	reconnectionCount int64
	errorCounts       map[string]int64
}

func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		commandCounts: make(map[string]int64),
		errorCounts:   make(map[string]int64),
	}
}

func (m *MetricsCollector) RecordSyncDuration(duration time.Duration) {
	m.syncDurations = append(m.syncDurations, duration)
	fmt.Printf("📊 Sync duration: %v\n", duration)
}

func (m *MetricsCollector) RecordCommandProcessed(cmd string, duration time.Duration) {
	m.commandCounts[cmd]++
	fmt.Printf("⚡ Command processed: %s (took %v)\n", cmd, duration)
}

func (m *MetricsCollector) RecordNetworkBytes(bytes int64) {
	m.networkBytes += bytes
}

func (m *MetricsCollector) RecordKeyCount(count int64) {
	fmt.Printf("🔑 Key count: %d\n", count)
}

func (m *MetricsCollector) RecordMemoryUsage(bytes int64) {
	fmt.Printf("💾 Memory usage: %d bytes\n", bytes)
}

func (m *MetricsCollector) RecordReconnection() {
	m.reconnectionCount++
	fmt.Printf("🔄 Reconnection #%d\n", m.reconnectionCount)
}

func (m *MetricsCollector) RecordError(errorType string) {
	m.errorCounts[errorType]++
	fmt.Printf("⚠️  Error recorded: %s (total: %d)\n", errorType, m.errorCounts[errorType])
}

func (m *MetricsCollector) PrintReport() {
	fmt.Printf("\n📈 Metrics Report:\n")
	fmt.Printf("   Sync durations: %v\n", m.syncDurations)
	fmt.Printf("   Command counts: %v\n", m.commandCounts)
	fmt.Printf("   Network bytes: %d\n", m.networkBytes)
	fmt.Printf("   Reconnections: %d\n", m.reconnectionCount)
	fmt.Printf("   Error counts: %v\n", m.errorCounts)
}

func main() {
	logger := &CustomLogger{}
	metrics := NewMetricsCollector()

	// Create replica with custom logger and metrics
	replica, err := redisreplica.New(
		redisreplica.WithMaster("localhost:6379"),
		redisreplica.WithReplicaAddr(":6380"),
		redisreplica.WithLogger(logger),
		redisreplica.WithMetrics(metrics),
		redisreplica.WithSyncTimeout(30*time.Second),
		redisreplica.WithMaxMemory(100*1024*1024), // 100MB limit
	)
	if err != nil {
		log.Fatal("Failed to create replica:", err)
	}
	defer func() { _ = replica.Close() }()

	// Setup monitoring callbacks
	replica.OnSyncComplete(func() {
		logger.Info("🎉 Sync completed callback triggered!")
		metrics.PrintReport()
	})

	// Start replica
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	logger.Info("Starting monitored replica...")

	if err := replica.Start(ctx); err != nil {
		log.Fatal("Failed to start replica:", err)
	}

	// Wait for initial sync with monitoring
	logger.Info("Waiting for initial synchronization...")
	if err := replica.WaitForSync(ctx); err != nil {
		log.Fatal("Failed to sync:", err)
	}

	// Monitor continuously
	logger.Info("Starting continuous monitoring...")

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			printDetailedStatus(replica, logger)
			metrics.PrintReport()

		case <-ctx.Done():
			logger.Info("Monitoring stopped")
			return
		}
	}
}

func printDetailedStatus(replica *redisreplica.Replica, logger *CustomLogger) {
	status := replica.SyncStatus()
	info := replica.GetInfo()

	logger.Info("Status Update",
		redisreplica.Field{Key: "connected", Value: status.Connected},
		redisreplica.Field{Key: "sync_completed", Value: status.InitialSyncCompleted},
		redisreplica.Field{Key: "repl_offset", Value: status.ReplicationOffset},
		redisreplica.Field{Key: "commands_processed", Value: status.CommandsProcessed},
	)

	if replInfo, ok := info["replication"].(map[string]interface{}); ok {
		logger.Info("Replication Info",
			redisreplica.Field{Key: "master", Value: replInfo["master_host"]},
			redisreplica.Field{Key: "offset", Value: replInfo["replication_offset"]},
		)
	}

	if versionInfo, ok := info["version"].(map[string]string); ok {
		logger.Info("Version Info",
			redisreplica.Field{Key: "version", Value: versionInfo["version"]},
		)
	}
}
