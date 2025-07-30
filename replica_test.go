package redisreplica_test

import (
	"testing"
	"time"

	redisreplica "github.com/raniellyferreira/redis-inmemory-replica"
)

func TestNew(t *testing.T) {
	replica, err := redisreplica.New(
		redisreplica.WithMaster("localhost:6379"),
		redisreplica.WithReplicaAddr(":6380"),
	)
	if err != nil {
		t.Fatalf("Failed to create replica: %v", err)
	}
	defer replica.Close()

	if replica == nil {
		t.Fatal("Expected replica to be non-nil")
	}
}

func TestNewWithInvalidOptions(t *testing.T) {
	// Test with empty master address
	_, err := redisreplica.New(
		redisreplica.WithMaster(""),
	)
	if err == nil {
		t.Fatal("Expected error with empty master address")
	}

	// Test with invalid timeout
	_, err = redisreplica.New(
		redisreplica.WithMaster("localhost:6379"),
		redisreplica.WithSyncTimeout(-1*time.Second),
	)
	if err == nil {
		t.Fatal("Expected error with invalid timeout")
	}
}

func TestReplicaConfiguration(t *testing.T) {
	logger := &testLogger{}
	metrics := &testMetrics{}

	replica, err := redisreplica.New(
		redisreplica.WithMaster("localhost:6379"),
		redisreplica.WithReplicaAddr(":6380"),
		redisreplica.WithSyncTimeout(30*time.Second),
		redisreplica.WithMaxMemory(1024*1024),
		redisreplica.WithLogger(logger),
		redisreplica.WithMetrics(metrics),
		redisreplica.WithCommandFilters([]string{"SET", "GET"}),
	)
	if err != nil {
		t.Fatalf("Failed to create replica: %v", err)
	}
	defer replica.Close()

	// Verify storage is accessible
	storage := replica.Storage()
	if storage == nil {
		t.Fatal("Expected storage to be non-nil")
	}

	// Test storage operations
	err = storage.Set("test", []byte("value"), nil)
	if err != nil {
		t.Fatalf("Failed to set value: %v", err)
	}

	value, exists := storage.Get("test")
	if !exists {
		t.Fatal("Expected key to exist")
	}

	if string(value) != "value" {
		t.Fatalf("Expected 'value', got '%s'", string(value))
	}
}

func TestReplicaStatus(t *testing.T) {
	replica, err := redisreplica.New(
		redisreplica.WithMaster("localhost:6379"),
	)
	if err != nil {
		t.Fatalf("Failed to create replica: %v", err)
	}
	defer replica.Close()

	// Test initial status
	status := replica.SyncStatus()
	if status.InitialSyncCompleted {
		t.Fatal("Expected initial sync to not be completed")
	}

	if status.MasterHost != "localhost:6379" {
		t.Fatalf("Expected master host 'localhost:6379', got '%s'", status.MasterHost)
	}

	// Test connection status
	if replica.IsConnected() {
		t.Fatal("Expected replica to not be connected initially")
	}
}

func TestReplicaInfo(t *testing.T) {
	replica, err := redisreplica.New(
		redisreplica.WithMaster("localhost:6379"),
	)
	if err != nil {
		t.Fatalf("Failed to create replica: %v", err)
	}
	defer replica.Close()

	info := replica.GetInfo()
	if info == nil {
		t.Fatal("Expected info to be non-nil")
	}

	// Check for expected keys
	expectedKeys := []string{"keys", "memory_usage", "replication", "version"}
	for _, key := range expectedKeys {
		if _, exists := info[key]; !exists {
			t.Fatalf("Expected info key '%s' to exist", key)
		}
	}

	// Check version info
	if versionInfo, ok := info["version"].(map[string]string); ok {
		if version, exists := versionInfo["version"]; !exists || version == "" {
			t.Fatal("Expected version information")
		}
	}
}

func TestSyncCallback(t *testing.T) {
	replica, err := redisreplica.New(
		redisreplica.WithMaster("localhost:6379"),
	)
	if err != nil {
		t.Fatalf("Failed to create replica: %v", err)
	}
	defer replica.Close()

	callbackCalled := false
	replica.OnSyncComplete(func() {
		callbackCalled = true
	})

	// Since we can't actually connect to Redis in this test,
	// we just verify the callback was registered without error
	if callbackCalled {
		t.Fatal("Callback should not be called immediately")
	}
}

// Test helper types
type testLogger struct{}

func (l *testLogger) Debug(msg string, fields ...redisreplica.Field) {}
func (l *testLogger) Info(msg string, fields ...redisreplica.Field)  {}
func (l *testLogger) Error(msg string, fields ...redisreplica.Field) {}

type testMetrics struct{}

func (m *testMetrics) RecordSyncDuration(duration time.Duration)                  {}
func (m *testMetrics) RecordCommandProcessed(cmd string, duration time.Duration) {}
func (m *testMetrics) RecordNetworkBytes(bytes int64)                             {}
func (m *testMetrics) RecordKeyCount(count int64)                                 {}
func (m *testMetrics) RecordMemoryUsage(bytes int64)                              {}
func (m *testMetrics) RecordReconnection()                                        {}
func (m *testMetrics) RecordError(errorType string)                               {}