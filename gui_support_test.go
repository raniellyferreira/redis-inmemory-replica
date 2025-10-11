package redisreplica_test

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	redisreplica "github.com/raniellyferreira/redis-inmemory-replica"
	"github.com/raniellyferreira/redis-inmemory-replica/protocol"
)

// createCommand creates a protocol command array
func createCommand(cmd ...string) []protocol.Value {
	values := make([]protocol.Value, len(cmd))
	for i, arg := range cmd {
		values[i] = protocol.Value{
			Type: protocol.TypeBulkString,
			Data: []byte(arg),
		}
	}
	return values
}

// TestGUIClientSupport tests the functionality needed for GUI clients like TablePlus
func TestGUIClientSupport(t *testing.T) {
	// Create replica with server
	replica, err := redisreplica.New(
		redisreplica.WithReplicaAddr(":6381"), // Use different port to avoid conflicts
	)
	if err != nil {
		t.Fatalf("Failed to create replica: %v", err)
	}
	defer func() {
		if err := replica.Close(); err != nil {
			t.Logf("Failed to close replica: %v", err)
		}
	}()

	// Start server
	ctx := context.Background()
	if err := replica.Start(ctx); err != nil {
		t.Fatalf("Failed to start replica: %v", err)
	}

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Add test data to multiple databases
	storage := replica.Storage()
	
	// Add data to database 0
	if err := storage.Set("key1", []byte("value1"), nil); err != nil {
		t.Fatalf("Failed to set key1: %v", err)
	}
	if err := storage.Set("key2", []byte("value2"), nil); err != nil {
		t.Fatalf("Failed to set key2: %v", err)
	}
	
	// Switch to database 1 and add data
	if err := storage.SelectDB(1); err != nil {
		t.Fatalf("Failed to select database 1: %v", err)
	}
	if err := storage.Set("db1key1", []byte("db1value1"), nil); err != nil {
		t.Fatalf("Failed to set db1key1: %v", err)
	}
	
	// Switch back to database 0
	if err := storage.SelectDB(0); err != nil {
		t.Fatalf("Failed to select database 0: %v", err)
	}

	// Connect to the replica server
	conn, err := net.Dial("tcp", "localhost:6381")
	if err != nil {
		t.Fatalf("Failed to connect to replica: %v", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Logf("Failed to close connection: %v", err)
		}
	}()

	reader := protocol.NewReader(conn)
	writer := protocol.NewWriter(conn)

	t.Run("CONFIG_GET_databases", func(t *testing.T) {
		// Test CONFIG GET databases
		err = writer.WriteArray(createCommand("CONFIG", "GET", "databases"))
		if err != nil {
			t.Fatalf("Failed to write command: %v", err)
		}
		err = writer.Flush()
		if err != nil {
			t.Fatalf("Failed to flush: %v", err)
		}

		response, err := reader.ReadNext()
		if err != nil {
			t.Fatalf("Failed to read response: %v", err)
		}

		if response.Type != protocol.TypeArray || len(response.Array) != 2 {
			t.Fatalf("Expected array with 2 elements, got: %v", response)
		}

		if string(response.Array[0].Data) != "databases" {
			t.Errorf("Expected key 'databases', got: %s", string(response.Array[0].Data))
		}

		if string(response.Array[1].Data) != "16" {
			t.Errorf("Expected value '16', got: %s", string(response.Array[1].Data))
		}
	})

	t.Run("INFO_keyspace", func(t *testing.T) {
		// Test INFO keyspace
		err = writer.WriteArray(createCommand("INFO", "keyspace"))
		if err != nil {
			t.Fatalf("Failed to write command: %v", err)
		}
		err = writer.Flush()
		if err != nil {
			t.Fatalf("Failed to flush: %v", err)
		}

		response, err := reader.ReadNext()
		if err != nil {
			t.Fatalf("Failed to read response: %v", err)
		}

		infoOutput := string(response.Data)
		
		if !strings.Contains(infoOutput, "# Keyspace") {
			t.Error("Expected '# Keyspace' section in INFO output")
		}

		if !strings.Contains(infoOutput, "db0:keys=2") {
			t.Error("Expected 'db0:keys=2' in INFO keyspace output")
		}

		if !strings.Contains(infoOutput, "db1:keys=1") {
			t.Error("Expected 'db1:keys=1' in INFO keyspace output")
		}

		if !strings.Contains(infoOutput, "expires=0") {
			t.Error("Expected 'expires=0' in INFO keyspace output")
		}
	})

	t.Run("INFO_all_includes_keyspace", func(t *testing.T) {
		// Test INFO all (should include keyspace)
		err = writer.WriteArray(createCommand("INFO"))
		if err != nil {
			t.Fatalf("Failed to write command: %v", err)
		}
		err = writer.Flush()
		if err != nil {
			t.Fatalf("Failed to flush: %v", err)
		}

		response, err := reader.ReadNext()
		if err != nil {
			t.Fatalf("Failed to read response: %v", err)
		}

		infoOutput := string(response.Data)
		
		// Should include all standard sections plus keyspace
		if !strings.Contains(infoOutput, "# Server") {
			t.Error("Expected '# Server' section in INFO all output")
		}

		if !strings.Contains(infoOutput, "# Replication") {
			t.Error("Expected '# Replication' section in INFO all output")
		}

		if !strings.Contains(infoOutput, "# Memory") {
			t.Error("Expected '# Memory' section in INFO all output")
		}

		if !strings.Contains(infoOutput, "# Keyspace") {
			t.Error("Expected '# Keyspace' section in INFO all output")
		}

		if !strings.Contains(infoOutput, "db0:keys=2") {
			t.Error("Expected database information in INFO all output")
		}
	})

	t.Run("SELECT_and_DBSIZE", func(t *testing.T) {
		// Test SELECT command
		err = writer.WriteArray(createCommand("SELECT", "1"))
		if err != nil {
			t.Fatalf("Failed to write command: %v", err)
		}
		err = writer.Flush()
		if err != nil {
			t.Fatalf("Failed to flush: %v", err)
		}

		response, err := reader.ReadNext()
		if err != nil {
			t.Fatalf("Failed to read response: %v", err)
		}

		if response.Type != protocol.TypeSimpleString || response.String() != "OK" {
			t.Errorf("Expected 'OK' response to SELECT, got: %v", response)
		}

		// Test DBSIZE on selected database
		err = writer.WriteArray(createCommand("DBSIZE"))
		if err != nil {
			t.Fatalf("Failed to write command: %v", err)
		}
		err = writer.Flush()
		if err != nil {
			t.Fatalf("Failed to flush: %v", err)
		}

		response, err = reader.ReadNext()
		if err != nil {
			t.Fatalf("Failed to read response: %v", err)
		}

		if response.Type != protocol.TypeInteger || response.Integer != 1 {
			t.Errorf("Expected DBSIZE to return 1 for db1, got: %v", response)
		}
	})

	t.Run("CONFIG_GET_wildcard", func(t *testing.T) {
		// Test CONFIG GET * (wildcard)
		err = writer.WriteArray(createCommand("CONFIG", "GET", "*"))
		if err != nil {
			t.Fatalf("Failed to write command: %v", err)
		}
		err = writer.Flush()
		if err != nil {
			t.Fatalf("Failed to flush: %v", err)
		}

		response, err := reader.ReadNext()
		if err != nil {
			t.Fatalf("Failed to read response: %v", err)
		}

		if response.Type != protocol.TypeArray || len(response.Array) < 2 {
			t.Fatalf("Expected array with at least 2 elements for wildcard, got: %v", response)
		}

		// Should include databases parameter
		found := false
		for i := 0; i < len(response.Array); i += 2 {
			if i+1 < len(response.Array) && string(response.Array[i].Data) == "databases" {
				found = true
				break
			}
		}

		if !found {
			t.Error("Expected 'databases' parameter in CONFIG GET * response")
		}
	})

	t.Run("CONFIG_GET_unknown", func(t *testing.T) {
		// Test CONFIG GET with unknown parameter
		err = writer.WriteArray(createCommand("CONFIG", "GET", "unknown_parameter"))
		if err != nil {
			t.Fatalf("Failed to write command: %v", err)
		}
		err = writer.Flush()
		if err != nil {
			t.Fatalf("Failed to flush: %v", err)
		}

		response, err := reader.ReadNext()
		if err != nil {
			t.Fatalf("Failed to read response: %v", err)
		}

		if response.Type != protocol.TypeArray || len(response.Array) != 0 {
			t.Errorf("Expected empty array for unknown parameter, got: %v", response)
		}
	})
}

// TestReplicationStartStopIdempotent ensures that multiple close calls are safe
func TestReplicationStartStopIdempotent(t *testing.T) {
	// Create replica without auto-starting server
	replica, err := redisreplica.New()
	if err != nil {
		t.Fatalf("Failed to create replica: %v", err)
	}

	// Test multiple close calls (should be idempotent)
	for i := 0; i < 3; i++ {
		t.Logf("Close call %d", i+1)
		
		err := replica.Close()
		if err != nil {
			t.Fatalf("Close failed on call %d: %v", i+1, err)
		}
	}
	
	t.Log("Multiple close calls completed successfully")
	
	// Test that operations after close return appropriate errors
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	
	err = replica.Start(ctx)
	if err == nil {
		t.Error("Expected error when starting closed replica")
	}
	t.Logf("Start after close correctly returned error: %v", err)
}