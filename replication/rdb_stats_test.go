package replication

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/raniellyferreira/redis-inmemory-replica/storage"
)

// TestRDBLoadStats tests the RDB loading statistics and batched logging
func TestRDBLoadStats(t *testing.T) {
	stats := NewRDBLoadStats()
	logger := &mockLogger{}

	// Test recording keys with different types
	stats.RecordKey(0, "string", logger)
	stats.RecordKey(0, "string", logger)
	stats.RecordKey(0, "hash", logger)
	stats.RecordKey(1, "list", logger)

	// Check statistics
	if stats.ProcessedKeys != 4 {
		t.Errorf("Expected 4 processed keys, got %d", stats.ProcessedKeys)
	}

	db0Stats := stats.DatabaseStats[0]
	if db0Stats == nil {
		t.Fatal("Database 0 stats not found")
	}

	if db0Stats.Keys != 3 {
		t.Errorf("Expected 3 keys in DB 0, got %d", db0Stats.Keys)
	}

	if db0Stats.TypeCounts["string"] != 2 {
		t.Errorf("Expected 2 string keys in DB 0, got %d", db0Stats.TypeCounts["string"])
	}

	if db0Stats.TypeCounts["hash"] != 1 {
		t.Errorf("Expected 1 hash key in DB 0, got %d", db0Stats.TypeCounts["hash"])
	}

	db1Stats := stats.DatabaseStats[1]
	if db1Stats == nil {
		t.Fatal("Database 1 stats not found")
	}

	if db1Stats.Keys != 1 {
		t.Errorf("Expected 1 key in DB 1, got %d", db1Stats.Keys)
	}

	if db1Stats.TypeCounts["list"] != 1 {
		t.Errorf("Expected 1 list key in DB 1, got %d", db1Stats.TypeCounts["list"])
	}
}

// TestRDBLoadStatsBatchedLogging tests that logging happens at the right intervals
func TestRDBLoadStatsBatchedLogging(t *testing.T) {
	stats := NewRDBLoadStats()
	stats.BatchSize = 3 // Log every 3 keys
	logger := &mockLogger{}

	// Add keys one by one - should log on the 3rd key
	stats.RecordKey(0, "string", logger)
	if len(logger.logs) != 0 {
		t.Error("Should not log on first key")
	}

	stats.RecordKey(0, "string", logger)
	if len(logger.logs) != 0 {
		t.Error("Should not log on second key")
	}

	stats.RecordKey(0, "string", logger)
	if len(logger.logs) != 1 {
		t.Error("Should log on third key")
	}

	// Check log content
	logMsg := logger.logs[0]
	if !strings.Contains(logMsg, "RDB load progress") {
		t.Errorf("Log should contain progress message, got: %s", logMsg)
	}

	if !strings.Contains(logMsg, "db=0") {
		t.Errorf("Log should contain database info, got: %s", logMsg)
	}

	if !strings.Contains(logMsg, "keys=3") {
		t.Errorf("Log should contain key count, got: %s", logMsg)
	}

	if !strings.Contains(logMsg, "string:3") {
		t.Errorf("Log should contain type counts, got: %s", logMsg)
	}
}

// TestRDBLoadStatsTimeBasedLogging tests that logging happens based on time intervals
func TestRDBLoadStatsTimeBasedLogging(t *testing.T) {
	stats := NewRDBLoadStats()
	stats.BatchSize = 100 // High batch size so time-based logging triggers first
	stats.LogInterval = 50 * time.Millisecond
	logger := &mockLogger{}

	// Add one key
	stats.RecordKey(0, "string", logger)
	if len(logger.logs) != 0 {
		t.Error("Should not log immediately")
	}

	// Wait for the time interval to pass
	time.Sleep(60 * time.Millisecond)

	// Add another key - should trigger time-based logging
	stats.RecordKey(0, "string", logger)
	if len(logger.logs) != 1 {
		t.Error("Should log based on time interval")
	}
}

// TestRDBStorageHandlerWithStats tests the updated RDB storage handler
func TestRDBStorageHandlerWithStats(t *testing.T) {
	stor := storage.NewMemory()
	logger := &mockLogger{}
	stats := NewRDBLoadStats()
	stats.BatchSize = 2 // Log every 2 keys for testing

	handler := &rdbStorageHandler{
		storage:   stor,
		logger:    logger,
		databases: make(map[int]struct{}),
		stats:     stats,
	}

	// Test processing keys
	err := handler.OnDatabase(0)
	if err != nil {
		t.Fatalf("OnDatabase failed: %v", err)
	}

	// Process some keys
	testKey1 := []byte("test1")
	testValue1 := []byte("value1")
	err = handler.OnKey(testKey1, testValue1, nil)
	if err != nil {
		t.Fatalf("OnKey failed: %v", err)
	}

	testKey2 := []byte("test2")
	testValue2 := []byte("value2")
	err = handler.OnKey(testKey2, testValue2, nil)
	if err != nil {
		t.Fatalf("OnKey failed: %v", err)
	}

	// Should have logged after 2 keys
	if len(logger.logs) == 0 {
		t.Error("Should have logged progress after processing keys")
	}

	// Check that keys were stored
	value, exists := stor.Get("test1")
	if !exists {
		t.Error("Key test1 should exist in storage")
	}
	if !bytes.Equal(value, testValue1) {
		t.Error("Stored value doesn't match")
	}

	// Test OnEnd logs final stats
	logger.logs = nil // Clear previous logs
	err = handler.OnEnd()
	if err != nil {
		t.Fatalf("OnEnd failed: %v", err)
	}

	if len(logger.logs) == 0 {
		t.Error("OnEnd should log final statistics")
	}
}

// TestRDBLoadStatsErrorRecording tests error recording functionality
func TestRDBLoadStatsErrorRecording(t *testing.T) {
	stats := NewRDBLoadStats()

	// Initialize database stats by recording some keys first
	mockLogger := &mockLogger{}
	stats.RecordKey(0, "string", mockLogger)
	stats.RecordKey(1, "string", mockLogger)

	// Record some errors
	stats.RecordError(0)
	stats.RecordError(1)
	stats.RecordError(0)

	if stats.ErrorCount != 3 {
		t.Errorf("Expected 3 total errors, got %d", stats.ErrorCount)
	}

	if stats.DatabaseStats[0].ErrorCount != 2 {
		t.Errorf("Expected 2 errors in DB 0, got %d", stats.DatabaseStats[0].ErrorCount)
	}

	if stats.DatabaseStats[1].ErrorCount != 1 {
		t.Errorf("Expected 1 error in DB 1, got %d", stats.DatabaseStats[1].ErrorCount)
	}
}

// mockLogger captures log messages for testing
type mockLogger struct {
	logs []string
}

func (l *mockLogger) Debug(msg string, fields ...interface{}) {
	// Silent for debug
}

func (l *mockLogger) Info(msg string, fields ...interface{}) {
	logStr := msg
	for i := 0; i < len(fields); i += 2 {
		if i+1 < len(fields) {
			key := getString(fields[i])
			value := getString(fields[i+1])
			logStr += " " + key + "=" + value
		}
	}
	l.logs = append(l.logs, logStr)
}

func (l *mockLogger) Error(msg string, fields ...interface{}) {
	logStr := "ERROR: " + msg
	for i := 0; i < len(fields); i += 2 {
		if i+1 < len(fields) {
			key := getString(fields[i])
			value := getString(fields[i+1])
			logStr += " " + key + "=" + value
		}
	}
	l.logs = append(l.logs, logStr)
}

func getString(v interface{}) string {
	return fmt.Sprintf("%v", v)
}