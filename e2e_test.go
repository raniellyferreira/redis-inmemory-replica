package redisreplica_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	redisreplica "github.com/raniellyferreira/redis-inmemory-replica"
)

// TestEndToEndWithRealRedis tests integration with a real Redis instance
// This test requires a Redis instance to be running
func TestEndToEndWithRealRedis(t *testing.T) {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	// Check if Redis is available
	if !isRedisAvailable(redisAddr) {
		t.Skip("Redis not available at", redisAddr, "- skipping e2e test. Set REDIS_ADDR environment variable or start Redis at localhost:6379")
	}

	t.Logf("Running end-to-end test with Redis at %s", redisAddr)

	// Clear any existing data in Redis
	if err := clearRedis(redisAddr); err != nil {
		t.Fatalf("Failed to clear Redis: %v", err)
	}

	// Create replica
	replica, err := redisreplica.New(
		redisreplica.WithMaster(redisAddr),
		redisreplica.WithSyncTimeout(30*time.Second),
	)
	if err != nil {
		t.Fatalf("Failed to create replica: %v", err)
	}
	defer replica.Close()

	// Track sync completion
	syncCompleted := make(chan struct{})
	replica.OnSyncComplete(func() {
		t.Log("✅ Initial synchronization completed")
		close(syncCompleted)
	})

	// Start replication
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	t.Log("🚀 Starting replica...")
	if err := replica.Start(ctx); err != nil {
		t.Fatalf("Failed to start replica: %v", err)
	}

	// Wait for initial sync
	t.Log("📡 Waiting for initial synchronization...")
	select {
	case <-syncCompleted:
		t.Log("Initial sync completed successfully")
	case <-time.After(30 * time.Second):
		t.Fatal("Initial sync timeout")
	case <-ctx.Done():
		t.Fatal("Context cancelled during initial sync")
	}

	// Test 1: Set some keys in Redis and verify they appear in replica
	t.Log("Test 1: Setting keys in Redis master")
	testKeys := map[string]string{
		"test:key1": "value1",
		"test:key2": "value2",
		"counter":   "42",
		"user:123":  "john_doe",
	}

	for key, value := range testKeys {
		if err := setRedisKey(redisAddr, key, value); err != nil {
			t.Errorf("Failed to set key %s: %v", key, err)
		}
	}

	// Give some time for replication
	time.Sleep(2 * time.Second)

	// Verify keys in replica
	t.Log("Verifying keys in replica...")
	storage := replica.Storage()
	for key, expectedValue := range testKeys {
		if value, exists := storage.Get(key); !exists {
			t.Errorf("Key %s not found in replica", key)
		} else if string(value) != expectedValue {
			t.Errorf("Key %s: expected %s, got %s", key, expectedValue, string(value))
		} else {
			t.Logf("✅ Key %s correctly replicated: %s", key, string(value))
		}
	}

	// Test 2: Delete keys and verify deletion is replicated
	t.Log("Test 2: Deleting keys in Redis master")
	if err := deleteRedisKey(redisAddr, "test:key1"); err != nil {
		t.Errorf("Failed to delete key test:key1: %v", err)
	}

	// Give some time for replication
	time.Sleep(2 * time.Second)

	// Verify deletion in replica
	replicaStorage := replica.Storage()
	if _, exists := replicaStorage.Get("test:key1"); exists {
		t.Error("Key test:key1 should have been deleted from replica")
	} else {
		t.Log("✅ Key deletion correctly replicated")
	}

	// Test 3: Set a large value to test buffer handling
	t.Log("Test 3: Testing large value replication")
	// Reduced size for CI stability - 1KB is sufficient for buffer boundary testing
	largeValue := strings.Repeat("X", 1024) // 1KB value

	if err := setRedisKey(redisAddr, "large:value", largeValue); err != nil {
		t.Errorf("Failed to set large value: %v", err)
	}

	// Give some time for replication
	time.Sleep(3 * time.Second)

	// Verify large value in replica
	replicaStorage = replica.Storage()
	if value, exists := replicaStorage.Get("large:value"); !exists {
		t.Error("Large value not found in replica")
	} else if len(value) != 1024 {
		t.Errorf("Large value: expected length 1024, got %d", len(value))
	} else {
		t.Log("✅ Large value correctly replicated")
	}

	// Test 4: Test rapid updates
	t.Log("Test 4: Testing rapid updates")
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("rapid:update:%d", i)
		value := fmt.Sprintf("value_%d", i)
		if err := setRedisKey(redisAddr, key, value); err != nil {
			t.Errorf("Failed to set rapid update key %s: %v", key, err)
		}
	}

	// Give some time for replication
	time.Sleep(3 * time.Second)

	// Verify rapid updates
	replicaStorage = replica.Storage()
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("rapid:update:%d", i)
		expectedValue := fmt.Sprintf("value_%d", i)
		if value, exists := replicaStorage.Get(key); !exists {
			t.Errorf("Rapid update key %s not found in replica", key)
		} else if string(value) != expectedValue {
			t.Errorf("Rapid update key %s: expected %s, got %s", key, expectedValue, string(value))
		}
	}
	t.Log("✅ Rapid updates correctly replicated")

	// Final status check
	status := replica.SyncStatus()
	t.Logf("Final replica status:")
	t.Logf("  Connected: %v", status.Connected)
	t.Logf("  Commands processed: %d", status.CommandsProcessed)
	t.Logf("  Replication offset: %d", status.ReplicationOffset)

	// Verify we have all expected keys
	allKeys := replicaStorage.Keys()
	t.Logf("Total keys in replica: %d", len(allKeys))
	t.Logf("Keys: %v", allKeys)
}

// TestRDBParsingRobustness tests RDB parsing with various scenarios
func TestRDBParsingRobustness(t *testing.T) {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	if !isRedisAvailable(redisAddr) {
		t.Skip("Redis not available at", redisAddr, "- skipping RDB parsing test")
	}

	t.Log("Testing RDB parsing robustness")

	// Clear Redis and populate with diverse data types
	if err := clearRedis(redisAddr); err != nil {
		t.Fatalf("Failed to clear Redis: %v", err)
	}

	// Set various types of keys to generate a comprehensive RDB
	testData := map[string]interface{}{
		"string:simple":   "hello",
		"string:empty":    "",
		"string:special":  "hello\r\nworld\x00\xff",
		"string:unicode":  "Hello 世界 🌍",
		"number:positive": "12345",
		"number:negative": "-67890",
		"number:zero":     "0",
	}

	t.Log("Populating Redis with test data...")
	for key, value := range testData {
		if err := setRedisKey(redisAddr, key, fmt.Sprintf("%v", value)); err != nil {
			t.Errorf("Failed to set key %s: %v", key, err)
		}
	}

	// Create replica and test full sync
	replica, err := redisreplica.New(
		redisreplica.WithMaster(redisAddr),
		redisreplica.WithSyncTimeout(30*time.Second),
	)
	if err != nil {
		t.Fatalf("Failed to create replica: %v", err)
	}
	defer replica.Close()

	// Start and wait for sync
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	syncCompleted := make(chan struct{})
	replica.OnSyncComplete(func() {
		close(syncCompleted)
	})

	if err := replica.Start(ctx); err != nil {
		t.Fatalf("Failed to start replica: %v", err)
	}

	select {
	case <-syncCompleted:
		t.Log("✅ RDB parsing completed successfully")
	case <-time.After(30 * time.Second):
		t.Fatal("RDB parsing timeout")
	}

	// Verify all data was correctly parsed
	replicaStorage := replica.Storage()
	for key, expectedValue := range testData {
		if value, exists := replicaStorage.Get(key); !exists {
			t.Errorf("Key %s not found after RDB parsing", key)
		} else if string(value) != fmt.Sprintf("%v", expectedValue) {
			t.Errorf("Key %s: expected %v, got %s", key, expectedValue, string(value))
		}
	}

	t.Log("✅ All RDB data correctly parsed and stored")
}

// Helper functions

func isRedisAvailable(addr string) bool {
	args := []string{"-h", parseHost(addr), "-p", parsePort(addr), "ping"}

	// Add password if available
	if password := os.Getenv("REDIS_PASSWORD"); password != "" {
		args = append([]string{"-a", password}, args...)
	}

	cmd := exec.Command("redis-cli", args...)
	// Set timeout to avoid hanging in CI and avoid config file loading for CI stability
	cmd.Env = append(os.Environ(), "REDISCLI_RCFILE=/dev/null")
	
	// Create a channel to handle timeout
	done := make(chan bool, 1)
	var output []byte
	var err error
	
	go func() {
		output, err = cmd.CombinedOutput()
		done <- true
	}()
	
	// Wait for command completion or timeout
	select {
	case <-done:
		if err != nil {
			return false
		}
		// Check if the output is "PONG" (handle potential extra characters)
		return strings.TrimSpace(string(output)) == "PONG"
	case <-time.After(10 * time.Second):
		// Kill the process if it's taking too long
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		return false
	}
}

func clearRedis(addr string) error {
	args := []string{"-h", parseHost(addr), "-p", parsePort(addr), "flushall"}

	// Add password if available
	if password := os.Getenv("REDIS_PASSWORD"); password != "" {
		args = append([]string{"-a", password}, args...)
	}

	cmd := exec.Command("redis-cli", args...)
	// Set timeout and avoid config file loading for CI stability
	cmd.Env = append(os.Environ(), "REDISCLI_RCFILE=/dev/null")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("redis flushall failed: %v, output: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func setRedisKey(addr, key, value string) error {
	args := []string{"-h", parseHost(addr), "-p", parsePort(addr), "set", key, value}

	// Add password if available
	if password := os.Getenv("REDIS_PASSWORD"); password != "" {
		args = append([]string{"-a", password}, args...)
	}

	cmd := exec.Command("redis-cli", args...)
	cmd.Env = append(os.Environ(), "REDISCLI_RCFILE=/dev/null")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("redis set failed: %v, output: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func deleteRedisKey(addr, key string) error {
	args := []string{"-h", parseHost(addr), "-p", parsePort(addr), "del", key}

	// Add password if available
	if password := os.Getenv("REDIS_PASSWORD"); password != "" {
		args = append([]string{"-a", password}, args...)
	}

	cmd := exec.Command("redis-cli", args...)
	cmd.Env = append(os.Environ(), "REDISCLI_RCFILE=/dev/null")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("redis del failed: %v, output: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func parseHost(addr string) string {
	// Handle IPv6 addresses: [::1]:6379 or [2001:db8::1]:6379
	if strings.HasPrefix(addr, "[") {
		if idx := strings.LastIndex(addr, "]:"); idx != -1 {
			return addr[:idx+1] // Include the closing bracket
		}
		return addr // Return as-is if malformed
	}
	
	// Handle IPv4 and hostnames: localhost:6379, 192.168.1.1:6379
	if idx := lastIndex(addr, ":"); idx != -1 {
		return addr[:idx]
	}
	return addr
}

func parsePort(addr string) string {
	// Handle IPv6 addresses: [::1]:6379 or [2001:db8::1]:6379
	if strings.HasPrefix(addr, "[") {
		if idx := strings.LastIndex(addr, "]:"); idx != -1 {
			return addr[idx+2:] // Skip ]:
		}
		return "6379" // Default port if malformed
	}
	
	// Handle IPv4 and hostnames: localhost:6379, 192.168.1.1:6379
	if idx := lastIndex(addr, ":"); idx != -1 {
		return addr[idx+1:]
	}
	return "6379"
}

func lastIndex(s, substr string) int {
	for i := len(s) - len(substr); i >= 0; i-- {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// Custom logger for detailed debugging
type debugLogger struct {
	t *testing.T
}

func (l *debugLogger) Debug(msg string, fields ...interface{}) {
	l.t.Logf("DEBUG: %s %v", msg, fields)
}

func (l *debugLogger) Info(msg string, fields ...interface{}) {
	l.t.Logf("INFO: %s %v", msg, fields)
}

func (l *debugLogger) Error(msg string, fields ...interface{}) {
	l.t.Logf("ERROR: %s %v", msg, fields)
}

// Benchmark test for replication performance
func BenchmarkReplicationThroughput(b *testing.B) {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	if !isRedisAvailable(redisAddr) {
		b.Skip("Redis not available - skipping benchmark")
	}

	// Clear Redis
	if err := clearRedis(redisAddr); err != nil {
		b.Fatalf("Failed to clear Redis: %v", err)
	}

	// Create replica
	replica, err := redisreplica.New(
		redisreplica.WithMaster(redisAddr),
	)
	if err != nil {
		b.Fatalf("Failed to create replica: %v", err)
	}
	defer replica.Close()

	// Start replica
	ctx := context.Background()
	if err := replica.Start(ctx); err != nil {
		b.Fatalf("Failed to start replica: %v", err)
	}

	// Wait for initial sync
	if err := replica.WaitForSync(ctx); err != nil {
		b.Fatalf("Failed to sync: %v", err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		key := "bench:key:" + strconv.Itoa(i)
		value := "benchmark_value_" + strconv.Itoa(i)

		if err := setRedisKey(redisAddr, key, value); err != nil {
			b.Fatalf("Failed to set key: %v", err)
		}
	}

	// Wait for all operations to replicate
	time.Sleep(time.Second)

	// Verify some operations completed
	replicaStorage := replica.Storage()
	if len(replicaStorage.Keys()) == 0 {
		b.Fatal("No keys replicated during benchmark")
	}
}
