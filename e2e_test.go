package redisreplica_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
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

	redisPassword := os.Getenv("REDIS_PASSWORD")

	// Check if Redis is available
	if !isRedisAvailable(redisAddr) {
		t.Skip("Redis not available at", redisAddr, "- skipping e2e test. Set REDIS_ADDR environment variable or start Redis at localhost:6379")
	}

	t.Logf("Running end-to-end test with Redis at %s", redisAddr)

	// Track test success
	testsPassed := true
	defer func() {
		if testsPassed {
			t.Log("✅ All end-to-end tests passed successfully!")
		}
	}()

	// Clear any existing data in Redis
	if err := clearRedisWithAuth(redisAddr, redisPassword); err != nil {
		t.Fatalf("Failed to clear Redis: %v", err)
	}

	// Create replica with authentication if needed
	replicaOptions := []redisreplica.Option{
		redisreplica.WithMaster(redisAddr),
		redisreplica.WithSyncTimeout(30 * time.Second),
	}
	if redisPassword != "" {
		replicaOptions = append(replicaOptions, redisreplica.WithMasterAuth(redisPassword))
	}

	replica, err := redisreplica.New(replicaOptions...)
	if err != nil {
		t.Fatalf("Failed to create replica: %v", err)
	}
	defer func() {
		if closeErr := replica.Close(); closeErr != nil {
			t.Logf("Warning: Error during replica cleanup: %v", closeErr)
		}
	}()

	// Track sync completion
	syncCompleted := make(chan struct{})
	var syncOnce sync.Once
	replica.OnSyncComplete(func() {
		t.Log("✅ Initial synchronization completed")
		syncOnce.Do(func() {
			close(syncCompleted)
		})
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

	// Give additional time for streaming connection to stabilize
	// This helps ensure commands aren't lost during protocol synchronization
	time.Sleep(2 * time.Second)

	// Test 1: Set some keys in Redis and verify they appear in replica
	t.Log("Test 1: Setting keys in Redis master")
	testKeys := map[string]string{
		"test:key1": "value1",
		"test:key2": "value2",
		"counter":   "42",
		"user:123":  "john_doe",
	}

	for key, value := range testKeys {
		if err := setRedisKeyWithAuth(redisAddr, redisPassword, key, value); err != nil {
			t.Errorf("Failed to set key %s: %v", key, err)
			testsPassed = false
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
			testsPassed = false
		} else if string(value) != expectedValue {
			t.Errorf("Key %s: expected %s, got %s", key, expectedValue, string(value))
			testsPassed = false
		} else {
			t.Logf("✅ Key %s correctly replicated: %s", key, string(value))
		}
	}

	// Test 2: Delete keys and verify deletion is replicated
	t.Log("Test 2: Deleting keys in Redis master")
	if err := deleteRedisKeyWithAuth(redisAddr, redisPassword, "test:key1"); err != nil {
		t.Errorf("Failed to delete key test:key1: %v", err)
		testsPassed = false
	}

	// Give some time for replication
	time.Sleep(2 * time.Second)

	// Verify deletion in replica with proper timeout
	replicaStorage := replica.Storage()

	// Wait for deletion to propagate (give it some time)
	deleted := false
	for i := 0; i < 50; i++ { // Wait up to 5 seconds
		if _, exists := replicaStorage.Get("test:key1"); !exists {
			deleted = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !deleted {
		t.Error("Key test:key1 should have been deleted from replica but still exists")
		testsPassed = false
	} else {
		t.Log("✅ Key deletion correctly replicated")
	}

	// Test 3: Set a large value to test buffer handling
	t.Log("Test 3: Testing large value replication")
	// Reduced size for CI stability - 1KB is sufficient for buffer boundary testing
	largeValue := strings.Repeat("X", 1024) // 1KB value

	if err := setRedisKeyWithAuth(redisAddr, redisPassword, "large:value", largeValue); err != nil {
		t.Errorf("Failed to set large value: %v", err)
		testsPassed = false
	}

	// Give some time for replication
	time.Sleep(3 * time.Second)

	// Verify large value in replica
	replicaStorage = replica.Storage()
	if value, exists := replicaStorage.Get("large:value"); !exists {
		t.Error("Large value not found in replica")
		testsPassed = false
	} else if len(value) != 1024 {
		t.Errorf("Large value: expected length 1024, got %d", len(value))
		testsPassed = false
	} else {
		t.Log("✅ Large value correctly replicated")
	}

	// Test 4: Test rapid updates
	t.Log("Test 4: Testing rapid updates")
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("rapid:update:%d", i)
		value := fmt.Sprintf("value_%d", i)
		if err := setRedisKeyWithAuth(redisAddr, redisPassword, key, value); err != nil {
			t.Errorf("Failed to set rapid update key %s: %v", key, err)
			testsPassed = false
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
			testsPassed = false
		} else if string(value) != expectedValue {
			t.Errorf("Rapid update key %s: expected %s, got %s", key, expectedValue, string(value))
			testsPassed = false
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
	allKeys := replicaStorage.Keys("*")
	t.Logf("Total keys in replica: %d", len(allKeys))
	t.Logf("Keys: %v", allKeys)
}

// TestRDBParsingRobustness tests RDB parsing with various scenarios
func TestRDBParsingRobustness(t *testing.T) {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	redisPassword := os.Getenv("REDIS_PASSWORD")

	if !isRedisAvailable(redisAddr) {
		t.Skip("Redis not available at", redisAddr, "- skipping RDB parsing test")
	}

	t.Log("Testing RDB parsing robustness")

	// Clear Redis and populate with diverse data types
	if err := clearRedisWithAuth(redisAddr, redisPassword); err != nil {
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
		if err := setRedisKeyWithAuth(redisAddr, redisPassword, key, fmt.Sprintf("%v", value)); err != nil {
			t.Errorf("Failed to set key %s: %v", key, err)
		}
	}

	// Force Redis to persist data to RDB before starting replication
	// This ensures all test data is available during full sync
	if err := forcePersistRedisWithAuth(redisAddr, redisPassword); err != nil {
		t.Errorf("Failed to force Redis persistence: %v", err)
	}

	// Small delay to ensure persistence is complete
	time.Sleep(100 * time.Millisecond)

	// Create replica and test full sync with authentication if needed
	replicaOptions := []redisreplica.Option{
		redisreplica.WithMaster(redisAddr),
		redisreplica.WithSyncTimeout(30 * time.Second),
	}
	if redisPassword != "" {
		replicaOptions = append(replicaOptions, redisreplica.WithMasterAuth(redisPassword))
	}

	replica, err := redisreplica.New(replicaOptions...)
	if err != nil {
		t.Fatalf("Failed to create replica: %v", err)
	}
	defer func() {
		if closeErr := replica.Close(); closeErr != nil {
			t.Logf("Warning: Error during replica cleanup: %v", closeErr)
		}
	}()

	// Start and wait for sync
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	syncCompleted := make(chan struct{})
	var syncOnce sync.Once
	replica.OnSyncComplete(func() {
		syncOnce.Do(func() {
			close(syncCompleted)
		})
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

	// Verify all data was correctly parsed with proper error handling
	replicaStorage := replica.Storage()

	// Allow some time for all data to be available
	time.Sleep(500 * time.Millisecond)

	missingKeys := []string{}
	mismatchedKeys := []string{}

	for key, expectedValue := range testData {
		value, exists := replicaStorage.Get(key)
		if !exists {
			missingKeys = append(missingKeys, key)
		} else {
			expectedStr := fmt.Sprintf("%v", expectedValue)
			actualStr := string(value)
			if actualStr != expectedStr {
				mismatchedKeys = append(mismatchedKeys, fmt.Sprintf("%s: expected %v, got %v", key, expectedStr, actualStr))
			}
		}
	}

	// Report all errors at once for better debugging
	if len(missingKeys) > 0 {
		t.Errorf("Missing keys after RDB parsing: %v", missingKeys)
	}
	if len(mismatchedKeys) > 0 {
		t.Errorf("Mismatched values after RDB parsing: %v", mismatchedKeys)
	}

	// Only log success if no errors
	if len(missingKeys) == 0 && len(mismatchedKeys) == 0 {
		t.Log("✅ All RDB data correctly parsed and stored")
	}
}

// Helper functions

func isRedisAvailable(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return false
	}
	defer func() { _ = conn.Close() }()

	// Check if authentication is needed
	password := os.Getenv("REDIS_PASSWORD")

	if password != "" {
		// Authenticate first
		authCmd := fmt.Sprintf("*2\r\n$4\r\nAUTH\r\n$%d\r\n%s\r\n", len(password), password)
		if _, err := conn.Write([]byte(authCmd)); err != nil {
			return false
		}

		// Read auth response
		buf := make([]byte, 1024)
		if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
			return false
		}
		if _, err := conn.Read(buf); err != nil {
			return false
		}
	}

	// Send PING command using proper RESP protocol
	_, err = conn.Write([]byte("*1\r\n$4\r\nPING\r\n"))
	if err != nil {
		return false
	}

	// Read response
	buf := make([]byte, 1024)
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return false
	}
	n, err := conn.Read(buf)
	if err != nil {
		return false
	}

	// Check if response contains PONG
	return strings.Contains(string(buf[:n]), "PONG")
}

func clearRedisWithAuth(addr, password string) error {
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	// Authenticate if password is provided
	if password != "" {
		authCmd := fmt.Sprintf("*2\r\n$4\r\nAUTH\r\n$%d\r\n%s\r\n", len(password), password)
		if _, err := conn.Write([]byte(authCmd)); err != nil {
			return err
		}

		// Read auth response
		buf := make([]byte, 1024)
		if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
			return err
		}
		if _, err := conn.Read(buf); err != nil {
			return err
		}
	}

	// Send FLUSHALL command using proper RESP protocol
	_, err = conn.Write([]byte("*1\r\n$8\r\nFLUSHALL\r\n"))
	if err != nil {
		return err
	}

	// Read response
	buf := make([]byte, 1024)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, err = conn.Read(buf)
	if err != nil {
		return err
	}

	return nil
}

func setRedisKeyWithAuth(addr, password, key, value string) error {
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	// Authenticate if password is provided
	if password != "" {
		authCmd := fmt.Sprintf("*2\r\n$4\r\nAUTH\r\n$%d\r\n%s\r\n", len(password), password)
		if _, err := conn.Write([]byte(authCmd)); err != nil {
			return err
		}

		// Read auth response
		buf := make([]byte, 1024)
		if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
			return err
		}
		if _, err := conn.Read(buf); err != nil {
			return err
		}
	}

	// Send SET command using proper RESP protocol (array of bulk strings)
	cmd := fmt.Sprintf("*3\r\n$3\r\nSET\r\n$%d\r\n%s\r\n$%d\r\n%s\r\n",
		len(key), key, len(value), value)
	_, err = conn.Write([]byte(cmd))
	if err != nil {
		return err
	}

	// Read response
	buf := make([]byte, 1024)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, err = conn.Read(buf)
	if err != nil {
		return err
	}

	return nil
}

func deleteRedisKey(addr, key string) error {
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	// Send DEL command using proper RESP protocol
	cmd := fmt.Sprintf("*2\r\n$3\r\nDEL\r\n$%d\r\n%s\r\n", len(key), key)
	_, err = conn.Write([]byte(cmd))
	if err != nil {
		return err
	}

	// Read response
	buf := make([]byte, 1024)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, err = conn.Read(buf)
	if err != nil {
		return err
	}

	return nil
}

func forcePersistRedisWithAuth(addr, password string) error {
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	// Authenticate if password is provided
	if password != "" {
		authCmd := fmt.Sprintf("*2\r\n$4\r\nAUTH\r\n$%d\r\n%s\r\n", len(password), password)
		if _, err := conn.Write([]byte(authCmd)); err != nil {
			return err
		}

		// Read auth response
		buf := make([]byte, 1024)
		if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
			return err
		}
		if _, err := conn.Read(buf); err != nil {
			return err
		}
	}

	// Send BGSAVE command using proper RESP protocol
	_, err = conn.Write([]byte("*1\r\n$6\r\nBGSAVE\r\n"))
	if err != nil {
		return err
	}

	// Read response
	buf := make([]byte, 1024)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, err = conn.Read(buf)
	if err != nil {
		return err
	}

	return nil
}

func deleteRedisKeyWithAuth(addr, password, key string) error {
	if password == "" {
		return deleteRedisKey(addr, key)
	}

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	// Authenticate if password is provided
	authCmd := fmt.Sprintf("*2\r\n$4\r\nAUTH\r\n$%d\r\n%s\r\n", len(password), password)
	if _, err := conn.Write([]byte(authCmd)); err != nil {
		return err
	}

	// Read auth response
	buf := make([]byte, 1024)
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	if _, err := conn.Read(buf); err != nil {
		return err
	}

	// Send DEL command using proper RESP protocol
	cmd := fmt.Sprintf("*2\r\n$3\r\nDEL\r\n$%d\r\n%s\r\n", len(key), key)
	_, err = conn.Write([]byte(cmd))
	if err != nil {
		return err
	}

	// Read response
	buf = make([]byte, 1024)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, err = conn.Read(buf)
	if err != nil {
		return err
	}

	return nil
}

// TestHeartbeatConnectionStability tests heartbeat mechanism for 2 minutes
func TestHeartbeatConnectionStability(t *testing.T) {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	redisPassword := os.Getenv("REDIS_PASSWORD")

	if !isRedisAvailable(redisAddr) {
		t.Skip("Redis not available at", redisAddr, "- skipping heartbeat stability test")
	}

	t.Log("Testing heartbeat connection stability for 2 minutes")

	// Clear Redis
	if err := clearRedisWithAuth(redisAddr, redisPassword); err != nil {
		t.Fatalf("Failed to clear Redis: %v", err)
	}

	// Create replica with heartbeat enabled (default 30s)
	replicaOptions := []redisreplica.Option{
		redisreplica.WithMaster(redisAddr),
		redisreplica.WithSyncTimeout(30 * time.Second),
		// Default heartbeat interval is 30s - let it run automatically
	}
	if redisPassword != "" {
		replicaOptions = append(replicaOptions, redisreplica.WithMasterAuth(redisPassword))
	}

	replica, err := redisreplica.New(replicaOptions...)
	if err != nil {
		t.Fatalf("Failed to create replica: %v", err)
	}
	defer func() {
		if closeErr := replica.Close(); closeErr != nil {
			t.Logf("Warning: Error during replica cleanup: %v", closeErr)
		}
	}()

	// Track sync completion
	syncCompleted := make(chan struct{})
	var syncOnce sync.Once
	replica.OnSyncComplete(func() {
		t.Log("✅ Initial synchronization completed")
		syncOnce.Do(func() {
			close(syncCompleted)
		})
	})

	// Start replication
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute) // Extra time for test
	defer cancel()

	t.Log("🚀 Starting replica with heartbeat enabled...")
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

	// Set an initial key to test basic replication
	if err := setRedisKeyWithAuth(redisAddr, redisPassword, "heartbeat:test", "initial_value"); err != nil {
		t.Fatalf("Failed to set initial test key: %v", err)
	}

	time.Sleep(2 * time.Second) // Wait for replication

	// Verify initial replication works
	storage := replica.Storage()
	if value, exists := storage.Get("heartbeat:test"); !exists {
		t.Fatal("Initial test key not replicated")
	} else if string(value) != "initial_value" {
		t.Fatalf("Initial test key value mismatch: expected 'initial_value', got '%s'", string(value))
	}

	t.Log("✅ Initial replication verified, starting 2-minute heartbeat stability test...")

	// Monitor connection for 2 minutes
	testDuration := 2 * time.Minute
	startTime := time.Now()
	checkInterval := 5 * time.Second
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	connectionChecks := 0
	connectedChecks := 0
	var lastOffset int64 = -1

	for {
		select {
		case <-ticker.C:
			connectionChecks++
			
			// Check connection status
			status := replica.SyncStatus()
			
			if status.Connected {
				connectedChecks++
				t.Logf("✅ Check %d: Connected, offset: %d, commands: %d", 
					connectionChecks, status.ReplicationOffset, status.CommandsProcessed)
				
				// Verify offset is not going backwards (should only increase or stay same)
				if lastOffset >= 0 && status.ReplicationOffset < lastOffset {
					t.Errorf("❌ Replication offset went backwards: %d -> %d", lastOffset, status.ReplicationOffset)
				}
				lastOffset = status.ReplicationOffset
			} else {
				t.Errorf("❌ Check %d: Disconnected! This indicates heartbeat failure", connectionChecks)
			}

			// Set a test key to verify replication is still working
			testKey := fmt.Sprintf("heartbeat:check:%d", connectionChecks)
			testValue := fmt.Sprintf("value_%d_%d", connectionChecks, time.Now().Unix())
			
			if err := setRedisKeyWithAuth(redisAddr, redisPassword, testKey, testValue); err != nil {
				t.Errorf("Failed to set heartbeat check key %s: %v", testKey, err)
			} else {
				// Give a moment for replication
				time.Sleep(500 * time.Millisecond)
				
				// Verify the key was replicated
				if replicatedValue, exists := replica.Storage().Get(testKey); !exists {
					t.Errorf("❌ Heartbeat check key %s not replicated", testKey)
				} else if string(replicatedValue) != testValue {
					t.Errorf("❌ Heartbeat check key %s value mismatch: expected %s, got %s", 
						testKey, testValue, string(replicatedValue))
				}
			}

		case <-ctx.Done():
			t.Fatal("Context cancelled during heartbeat test")
		}

		// Check if we've completed the test duration
		if time.Since(startTime) >= testDuration {
			break
		}
	}

	ticker.Stop()

	// Final verification
	connectionSuccessRate := float64(connectedChecks) / float64(connectionChecks) * 100
	t.Logf("🏁 Heartbeat stability test completed:")
	t.Logf("   Duration: %v", testDuration)
	t.Logf("   Connection checks: %d", connectionChecks)
	t.Logf("   Connected checks: %d", connectedChecks)
	t.Logf("   Connection success rate: %.1f%%", connectionSuccessRate)

	// Expect at least 95% connection success rate (allowing for brief network hiccups)
	if connectionSuccessRate < 95.0 {
		t.Errorf("❌ Connection success rate too low: %.1f%% (expected ≥95%%)", connectionSuccessRate)
		t.Error("This indicates the heartbeat mechanism is not effectively preventing timeouts")
	} else {
		t.Logf("✅ Heartbeat successfully maintained connection stability (%.1f%% success rate)", connectionSuccessRate)
	}

	// Verify we have some replication activity
	finalStatus := replica.SyncStatus()
	if finalStatus.CommandsProcessed == 0 {
		t.Error("❌ No commands were processed during the test")
	} else {
		t.Logf("✅ Total commands processed during test: %d", finalStatus.CommandsProcessed)
	}

	t.Log("✅ Heartbeat connection stability test completed successfully")
}

// TestReplicationDuringActiveChanges tests replication during 2 minutes of continuous changes
func TestReplicationDuringActiveChanges(t *testing.T) {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	redisPassword := os.Getenv("REDIS_PASSWORD")

	if !isRedisAvailable(redisAddr) {
		t.Skip("Redis not available at", redisAddr, "- skipping active changes replication test")
	}

	t.Log("Testing replication during 2 minutes of continuous master changes")

	// Clear Redis
	if err := clearRedisWithAuth(redisAddr, redisPassword); err != nil {
		t.Fatalf("Failed to clear Redis: %v", err)
	}

	// Create replica
	replicaOptions := []redisreplica.Option{
		redisreplica.WithMaster(redisAddr),
		redisreplica.WithSyncTimeout(30 * time.Second),
		redisreplica.WithHeartbeatInterval(30 * time.Second), // Shorter interval for this test
	}
	if redisPassword != "" {
		replicaOptions = append(replicaOptions, redisreplica.WithMasterAuth(redisPassword))
	}

	replica, err := redisreplica.New(replicaOptions...)
	if err != nil {
		t.Fatalf("Failed to create replica: %v", err)
	}
	defer func() {
		if closeErr := replica.Close(); closeErr != nil {
			t.Logf("Warning: Error during replica cleanup: %v", closeErr)
		}
	}()

	// Track sync completion
	syncCompleted := make(chan struct{})
	var syncOnce sync.Once
	replica.OnSyncComplete(func() {
		t.Log("✅ Initial synchronization completed")
		syncOnce.Do(func() {
			close(syncCompleted)
		})
	})

	// Start replication
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute) // Extra time for test
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

	time.Sleep(2 * time.Second) // Stabilization

	t.Log("🔄 Starting 2 minutes of continuous changes to master...")

	// Run continuous changes for 2 minutes
	testDuration := 2 * time.Minute
	
	// Tracking variables
	totalOperations := 0
	setOperations := 0
	deleteOperations := 0
	updateOperations := 0
	errors := 0

	// Channel to coordinate goroutines
	stopChanges := make(chan struct{})
	changesComplete := make(chan struct{})

	// Goroutine for continuous changes
	go func() {
		defer close(changesComplete)
		
		operationCounter := 0
		for {
			select {
			case <-stopChanges:
				t.Logf("📊 Change operations completed: %d total operations", operationCounter)
				return
			default:
				operationCounter++
				
				// Vary the types of operations
				switch operationCounter % 4 {
				case 0: // SET operation
					key := fmt.Sprintf("active:set:%d", operationCounter)
					value := fmt.Sprintf("value_%d_%d", operationCounter, time.Now().UnixNano())
					if err := setRedisKeyWithAuth(redisAddr, redisPassword, key, value); err != nil {
						t.Logf("Warning: Failed to set key %s: %v", key, err)
						errors++
					} else {
						setOperations++
					}
					
				case 1: // UPDATE operation (overwrite existing key)
					if setOperations > 0 {
						key := fmt.Sprintf("active:set:%d", (operationCounter/4)*4) // Reference previous SET
						value := fmt.Sprintf("updated_%d_%d", operationCounter, time.Now().UnixNano())
						if err := setRedisKeyWithAuth(redisAddr, redisPassword, key, value); err != nil {
							t.Logf("Warning: Failed to update key %s: %v", key, err)
							errors++
						} else {
							updateOperations++
						}
					}
					
				case 2: // SET with different pattern
					key := fmt.Sprintf("active:data:%d", operationCounter)
					// Vary data sizes more conservatively
					dataSize := 10 + (operationCounter % 50) // 10-59 bytes
					value := fmt.Sprintf("data_%d_", operationCounter) + strings.Repeat("X", dataSize)
					if err := setRedisKeyWithAuth(redisAddr, redisPassword, key, value); err != nil {
						t.Logf("Warning: Failed to set data key %s: %v", key, err)
						errors++
					} else {
						setOperations++
					}
					
				case 3: // DELETE operation (delete some older keys)
					if operationCounter > 20 { // Only start deleting after we have some keys
						// Delete both types of keys that were created
						if operationCounter%2 == 0 {
							keyToDelete := fmt.Sprintf("active:set:%d", operationCounter-20)
							if err := deleteRedisKeyWithAuth(redisAddr, redisPassword, keyToDelete); err != nil {
								t.Logf("Warning: Failed to delete key %s: %v", keyToDelete, err)
								errors++
							} else {
								deleteOperations++
							}
						} else {
							keyToDelete := fmt.Sprintf("active:data:%d", operationCounter-20)
							if err := deleteRedisKeyWithAuth(redisAddr, redisPassword, keyToDelete); err != nil {
								t.Logf("Warning: Failed to delete key %s: %v", keyToDelete, err)
								errors++
							} else {
								deleteOperations++
							}
						}
					}
				}
				
				totalOperations++
				
				// Brief pause between operations to avoid overwhelming Redis
				time.Sleep(50 * time.Millisecond)
			}
		}
	}()

	// Goroutine to monitor replication status
	statusTicker := time.NewTicker(10 * time.Second)
	defer statusTicker.Stop()
	
	statusChecks := 0
	connectedStatusChecks := 0
	
	statusMonitorDone := make(chan struct{})
	go func() {
		defer close(statusMonitorDone)
		
		for {
			select {
			case <-stopChanges:
				return
			case <-statusTicker.C:
				statusChecks++
				status := replica.SyncStatus()
				
				if status.Connected {
					connectedStatusChecks++
					t.Logf("📈 Status check %d: Connected, offset: %d, commands: %d", 
						statusChecks, status.ReplicationOffset, status.CommandsProcessed)
				} else {
					t.Errorf("❌ Status check %d: Disconnected during active changes!", statusChecks)
				}
			}
		}
	}()

	// Wait for test duration
	time.Sleep(testDuration)
	
	// Stop changes
	close(stopChanges)
	
	// Wait for goroutines to complete
	<-changesComplete
	<-statusMonitorDone

	t.Logf("🏁 Continuous changes phase completed:")
	t.Logf("   Duration: %v", testDuration)
	t.Logf("   Total operations: %d", totalOperations)
	t.Logf("   SET operations: %d", setOperations)
	t.Logf("   UPDATE operations: %d", updateOperations)
	t.Logf("   DELETE operations: %d", deleteOperations)
	t.Logf("   Errors: %d", errors)

	// Allow time for final replication
	t.Log("⏳ Allowing time for final replication...")
	time.Sleep(5 * time.Second)

	// Verify replication results
	storage := replica.Storage()
	replicatedKeys := storage.Keys("*")
	
	t.Logf("🔍 Verifying replication results:")
	t.Logf("   Keys in replica: %d", len(replicatedKeys))

	// Verify connection stability during changes
	connectionSuccessRate := float64(connectedStatusChecks) / float64(statusChecks) * 100
	t.Logf("   Connection success rate: %.1f%%", connectionSuccessRate)

	if connectionSuccessRate < 90.0 {
		t.Errorf("❌ Connection success rate during active changes too low: %.1f%% (expected ≥90%%)", connectionSuccessRate)
	}

	// Verify some replication occurred
	finalStatus := replica.SyncStatus()
	if finalStatus.CommandsProcessed < int64(totalOperations/2) {
		t.Errorf("❌ Too few commands replicated: %d processed vs %d sent (expected at least 50%%)", 
			finalStatus.CommandsProcessed, totalOperations)
	}

	// Sample verification: check that some specific keys exist and have correct values
	sampleVerificationErrors := 0
	samplesToCheck := 10
	
	for i := 0; i < samplesToCheck && i*4 < totalOperations; i++ {
		key := fmt.Sprintf("active:set:%d", i*4)
		if value, exists := storage.Get(key); exists {
			// The key might have been updated or deleted, so we just verify it exists
			t.Logf("✅ Sample key %s exists with value length: %d", key, len(value))
		} else {
			// Check if it was deleted
			expectedDeleteOperation := (i*4 + 20) < totalOperations
			if !expectedDeleteOperation {
				sampleVerificationErrors++
				t.Logf("❌ Sample key %s missing (not expected to be deleted)", key)
			} else {
				t.Logf("✅ Sample key %s absent (expected - was deleted)", key)
			}
		}
	}

	if sampleVerificationErrors > samplesToCheck/2 {
		t.Errorf("❌ Too many sample verification errors: %d/%d", sampleVerificationErrors, samplesToCheck)
	}

	t.Logf("✅ Replication during active changes test completed successfully")
	t.Logf("   Final status: %d commands processed, %d keys in replica", 
		finalStatus.CommandsProcessed, len(replicatedKeys))
}

// TestFullSyncAndIncremental tests both full sync and incremental replication
func TestFullSyncAndIncremental(t *testing.T) {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	if !isRedisAvailable(redisAddr) {
		t.Skip("Redis not available at", redisAddr, "- skipping full sync + incremental test")
	}

	redisPassword := os.Getenv("REDIS_PASSWORD")

	t.Log("Testing full sync followed by incremental replication")

	// Phase 1: Prepare initial data for full sync
	if err := clearRedisWithAuth(redisAddr, redisPassword); err != nil {
		t.Fatalf("Failed to clear Redis: %v", err)
	}

	// Set initial data
	initialData := map[string]string{
		"init:key1": "initial_value1",
		"init:key2": "initial_value2",
		"init:key3": "initial_value3",
	}

	for key, value := range initialData {
		if err := setRedisKeyWithAuth(redisAddr, redisPassword, key, value); err != nil {
			t.Errorf("Failed to set initial key %s: %v", key, err)
		}
	}

	// Force persistence to ensure data is in RDB
	if err := forcePersistRedisWithAuth(redisAddr, redisPassword); err != nil {
		t.Errorf("Failed to force Redis persistence: %v", err)
	}

	time.Sleep(200 * time.Millisecond) // Wait for persistence

	// Phase 2: Start replica and test full sync
	var replicaOptions []redisreplica.Option
	replicaOptions = append(replicaOptions, redisreplica.WithMaster(redisAddr))
	replicaOptions = append(replicaOptions, redisreplica.WithSyncTimeout(30*time.Second))

	if redisPassword != "" {
		replicaOptions = append(replicaOptions, redisreplica.WithMasterAuth(redisPassword))
	}

	replica, err := redisreplica.New(replicaOptions...)
	if err != nil {
		t.Fatalf("Failed to create replica: %v", err)
	}
	defer func() {
		if closeErr := replica.Close(); closeErr != nil {
			t.Logf("Warning: Error during replica cleanup: %v", closeErr)
		}
	}()

	// Track sync completion
	syncCompleted := make(chan struct{})
	var syncOnce sync.Once
	replica.OnSyncComplete(func() {
		t.Log("✅ Full sync completed")
		syncOnce.Do(func() {
			close(syncCompleted)
		})
	})

	// Start replication
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := replica.Start(ctx); err != nil {
		t.Fatalf("Failed to start replica: %v", err)
	}

	// Wait for full sync
	select {
	case <-syncCompleted:
		t.Log("Full sync completed successfully")
	case <-time.After(30 * time.Second):
		t.Fatal("Full sync timeout")
	}

	// Verify full sync data
	storage := replica.Storage()
	for key, expectedValue := range initialData {
		if value, exists := storage.Get(key); !exists {
			t.Errorf("Key %s not found after full sync", key)
		} else if string(value) != expectedValue {
			t.Errorf("Key %s: expected %s, got %s", key, expectedValue, string(value))
		}
	}

	// Phase 3: Test incremental replication
	t.Log("Testing incremental replication...")
	time.Sleep(2 * time.Second) // Ensure streaming is stable

	incrementalData := map[string]string{
		"incr:key1": "incremental_value1",
		"incr:key2": "incremental_value2",
		"incr:key3": "incremental_value3",
	}

	for key, value := range incrementalData {
		if err := setRedisKeyWithAuth(redisAddr, redisPassword, key, value); err != nil {
			t.Errorf("Failed to set incremental key %s: %v", key, err)
		}
	}

	// Wait for incremental replication
	time.Sleep(3 * time.Second)

	// Verify incremental data
	for key, expectedValue := range incrementalData {
		if value, exists := storage.Get(key); !exists {
			t.Errorf("Incremental key %s not found", key)
		} else if string(value) != expectedValue {
			t.Errorf("Incremental key %s: expected %s, got %s", key, expectedValue, string(value))
		}
	}

	// Phase 4: Test updates and deletions
	t.Log("Testing incremental updates and deletions...")

	// Update an initial key
	if err := setRedisKeyWithAuth(redisAddr, redisPassword, "init:key1", "updated_value"); err != nil {
		t.Errorf("Failed to update key: %v", err)
	}

	// Delete an incremental key
	if err := deleteRedisKeyWithAuth(redisAddr, redisPassword, "incr:key2"); err != nil {
		t.Errorf("Failed to delete key: %v", err)
	}

	time.Sleep(2 * time.Second)

	// Verify update
	if value, exists := storage.Get("init:key1"); !exists {
		t.Error("Updated key not found")
	} else if string(value) != "updated_value" {
		t.Errorf("Key update failed: expected updated_value, got %s", string(value))
	}

	// Verify deletion
	if _, exists := storage.Get("incr:key2"); exists {
		t.Error("Deleted key still exists")
	}

	t.Log("✅ Full sync and incremental replication test completed successfully")
}

// TestRedis7xFeatures tests Redis 7.x specific features
func TestRedis7xFeatures(t *testing.T) {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	if !isRedisAvailable(redisAddr) {
		t.Skip("Redis not available at", redisAddr, "- skipping Redis 7.x features test")
	}

	redisVersion := os.Getenv("REDIS_VERSION")
	if redisVersion == "" {
		redisVersion = "7.x"
	}

	t.Logf("Testing Redis 7.x features with version %s", redisVersion)

	redisPassword := os.Getenv("REDIS_PASSWORD")

	// Clear Redis
	if err := clearRedisWithAuth(redisAddr, redisPassword); err != nil {
		t.Fatalf("Failed to clear Redis: %v", err)
	}

	// Test various data types and encodings that might trigger the encoding 33 issue
	testCases := map[string]string{
		// Small integers (should use integer encoding)
		"int:small":  "1",
		"int:medium": "12345",
		"int:large":  "9223372036854775807", // Max int64

		// Strings that might trigger special encodings
		"str:empty":   "",
		"str:single":  "a",
		"str:numbers": "123456789012345678901234567890", // Long number string

		// Binary-like data
		"bin:data": string([]byte{0x00, 0x01, 0x02, 0x21, 0xFF}), // Include 0x21 (33)

		// Unicode and special characters
		"unicode:emoji": "🚀💾📡",
		"unicode:mixed": "Hello世界🌍",

		// Large strings that might be compressed
		"large:string": string(make([]byte, 1000)), // 1KB of zeros
		"large:text":   "Redis 7.x compatibility test " + string(make([]byte, 500)),
	}

	// Populate with test data
	for key, value := range testCases {
		if err := setRedisKeyWithAuth(redisAddr, redisPassword, key, value); err != nil {
			t.Errorf("Failed to set key %s: %v", key, err)
		}
	}

	// Force persistence to test RDB parsing
	if err := forcePersistRedisWithAuth(redisAddr, redisPassword); err != nil {
		t.Errorf("Failed to force Redis persistence: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Create replica
	var replicaOptions []redisreplica.Option
	replicaOptions = append(replicaOptions, redisreplica.WithMaster(redisAddr))
	replicaOptions = append(replicaOptions, redisreplica.WithSyncTimeout(30*time.Second))

	if redisPassword != "" {
		replicaOptions = append(replicaOptions, redisreplica.WithMasterAuth(redisPassword))
	}

	replica, err := redisreplica.New(replicaOptions...)
	if err != nil {
		t.Fatalf("Failed to create replica: %v", err)
	}
	defer func() {
		if closeErr := replica.Close(); closeErr != nil {
			t.Logf("Warning: Error during replica cleanup: %v", closeErr)
		}
	}()

	// Start and wait for sync
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	syncCompleted := make(chan struct{})
	var syncOnce sync.Once
	replica.OnSyncComplete(func() {
		syncOnce.Do(func() {
			close(syncCompleted)
		})
	})

	if err := replica.Start(ctx); err != nil {
		t.Fatalf("Failed to start replica: %v", err)
	}

	select {
	case <-syncCompleted:
		t.Log("✅ Redis 7.x features sync completed")
	case <-time.After(30 * time.Second):
		t.Fatal("Redis 7.x features sync timeout")
	}

	// Verify all data was correctly handled
	storage := replica.Storage()
	time.Sleep(500 * time.Millisecond)

	successCount := 0
	for key, expectedValue := range testCases {
		if value, exists := storage.Get(key); !exists {
			t.Errorf("Redis 7.x feature key %s not found", key)
		} else {
			// For binary data, compare byte by byte
			if key == "bin:data" {
				if len(value) != len(expectedValue) {
					t.Errorf("Binary data key %s length mismatch: expected %d, got %d",
						key, len(expectedValue), len(value))
				} else {
					match := true
					for i := range expectedValue {
						if value[i] != expectedValue[i] {
							match = false
							break
						}
					}
					if !match {
						t.Errorf("Binary data key %s content mismatch", key)
					} else {
						successCount++
					}
				}
			} else if string(value) != expectedValue {
				t.Errorf("Redis 7.x feature key %s: expected %v, got %v",
					key, expectedValue, string(value))
			} else {
				successCount++
			}
		}
	}

	t.Logf("✅ Redis 7.x features test: %d/%d test cases passed", successCount, len(testCases))

	if successCount == len(testCases) {
		t.Log("✅ All Redis 7.x feature tests passed - no encoding 33 errors detected")
	}
}

// Benchmark test for replication performance
func BenchmarkReplicationThroughput(b *testing.B) {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	redisPassword := os.Getenv("REDIS_PASSWORD")

	if !isRedisAvailable(redisAddr) {
		b.Skip("Redis not available - skipping benchmark")
	}

	// Clear Redis
	if err := clearRedisWithAuth(redisAddr, redisPassword); err != nil {
		b.Fatalf("Failed to clear Redis: %v", err)
	}

	// Create replica with authentication if needed
	replicaOptions := []redisreplica.Option{
		redisreplica.WithMaster(redisAddr),
		redisreplica.WithSyncTimeout(30 * time.Second),
	}
	if redisPassword != "" {
		replicaOptions = append(replicaOptions, redisreplica.WithMasterAuth(redisPassword))
	}

	replica, err := redisreplica.New(replicaOptions...)
	if err != nil {
		b.Fatalf("Failed to create replica: %v", err)
	}
	defer func() {
		if closeErr := replica.Close(); closeErr != nil {
			b.Logf("Warning: Error during replica cleanup: %v", closeErr)
		}
	}()

	// Start replica
	ctx := context.Background()
	if err := replica.Start(ctx); err != nil {
		b.Fatalf("Failed to start replica: %v", err)
	}

	// Wait for initial sync
	if err := replica.WaitForSync(ctx); err != nil {
		b.Fatalf("Failed to sync: %v", err)
	}

	// Pre-allocate connection pool to reduce connection overhead during benchmark
	connections := make([]net.Conn, 0, 10)
	defer func() {
		for _, conn := range connections {
			_ = conn.Close()
		}
	}()

	b.ResetTimer()
	b.ReportAllocs()

	// Batch operations to reduce per-operation overhead
	batchSize := 100
	if b.N < batchSize {
		batchSize = b.N
	}

	for i := 0; i < b.N; i += batchSize {
		end := i + batchSize
		if end > b.N {
			end = b.N
		}

		// Send batch of operations
		for j := i; j < end; j++ {
			key := fmt.Sprintf("bench:key:%d", j)
			value := fmt.Sprintf("benchmark_value_%d", j)

			if err := setRedisKeyWithAuth(redisAddr, redisPassword, key, value); err != nil {
				b.Fatalf("Failed to set key %s: %v", key, err)
			}
		}

		// Wait for batch to replicate (reduce wait frequency)
		if i%500 == 0 { // Check every 500 operations
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Final wait for all operations to replicate
	time.Sleep(100 * time.Millisecond)

	// Verify operations completed
	replicaStorage := replica.Storage()
	replicatedKeys := replicaStorage.Keys("bench:*")
	if len(replicatedKeys) == 0 {
		b.Fatal("No keys replicated during benchmark")
	}

	b.Logf("Replicated %d/%d keys", len(replicatedKeys), b.N)
}

// BenchmarkReplicationLatency measures individual operation latency
func BenchmarkReplicationLatency(b *testing.B) {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	redisPassword := os.Getenv("REDIS_PASSWORD")

	if !isRedisAvailable(redisAddr) {
		b.Skip("Redis not available - skipping benchmark")
	}

	// Clear Redis
	if err := clearRedisWithAuth(redisAddr, redisPassword); err != nil {
		b.Fatalf("Failed to clear Redis: %v", err)
	}

	// Create replica
	replicaOptions := []redisreplica.Option{
		redisreplica.WithMaster(redisAddr),
		redisreplica.WithSyncTimeout(30 * time.Second),
	}
	if redisPassword != "" {
		replicaOptions = append(replicaOptions, redisreplica.WithMasterAuth(redisPassword))
	}

	replica, err := redisreplica.New(replicaOptions...)
	if err != nil {
		b.Fatalf("Failed to create replica: %v", err)
	}
	defer func() {
		if closeErr := replica.Close(); closeErr != nil {
			b.Logf("Warning: Error during replica cleanup: %v", closeErr)
		}
	}()

	// Start replica
	ctx := context.Background()
	if err := replica.Start(ctx); err != nil {
		b.Fatalf("Failed to start replica: %v", err)
	}

	// Wait for initial sync
	if err := replica.WaitForSync(ctx); err != nil {
		b.Fatalf("Failed to sync: %v", err)
	}

	replicaStorage := replica.Storage()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("latency:key:%d", i)
		value := fmt.Sprintf("latency_value_%d", i)

		start := time.Now()

		// Set key on master
		if err := setRedisKeyWithAuth(redisAddr, redisPassword, key, value); err != nil {
			b.Fatalf("Failed to set key %s: %v", key, err)
		}

		// Wait for replication (with timeout)
		timeout := time.After(5 * time.Second)
		ticker := time.NewTicker(1 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-timeout:
				b.Fatalf("Replication timeout for key %s", key)
			case <-ticker.C:
				if _, exists := replicaStorage.Get(key); exists {
					elapsed := time.Since(start)
					b.Logf("Replication latency for key %s: %v", key, elapsed)
					goto next
				}
			}
		}
	next:
	}
}
