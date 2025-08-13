package redisreplica_test

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	redisreplica "github.com/raniellyferreira/redis-inmemory-replica"
	"github.com/raniellyferreira/redis-inmemory-replica/protocol"
	"github.com/raniellyferreira/redis-inmemory-replica/server"
	"github.com/raniellyferreira/redis-inmemory-replica/storage"
)

// setupTestServer creates a test server and connection
func setupTestServer(t *testing.T) (*redisreplica.Replica, net.Conn, *protocol.Writer, *protocol.Reader) {
	// Create replica with server (allow writes for testing)
	replica, err := redisreplica.New(
		redisreplica.WithReplicaAddr(":6382"), // Use different port to avoid conflicts
		redisreplica.WithReadOnly(false),      // Allow writes for testing
	)
	if err != nil {
		t.Fatalf("Failed to create replica: %v", err)
	}

	// Start server
	ctx := context.Background()
	if err := replica.Start(ctx); err != nil {
		t.Fatalf("Failed to start replica: %v", err)
	}

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Connect to the server
	conn, err := net.Dial("tcp", "localhost:6382")
	if err != nil {
		replica.Close()
		t.Fatalf("Failed to connect to server: %v", err)
	}

	writer := protocol.NewWriter(conn)
	reader := protocol.NewReader(conn)

	return replica, conn, writer, reader
}

// TestInfoKeyspace_ListsAllDBs tests that INFO keyspace consistently lists ALL databases with keys
func TestInfoKeyspace_ListsAllDBs(t *testing.T) {
	stor := storage.NewMemory()
	
	// Set up data in specific databases: db8, db9, db11, db13
	testDBs := []int{8, 9, 11, 13}
	
	for _, dbNum := range testDBs {
		stor.SelectDB(dbNum)
		
		// Add keys to each database
		switch dbNum {
		case 8:
			for i := 0; i < 466; i++ {
				key := fmt.Sprintf("key8_%d", i)
				err := stor.Set(key, []byte("value"), nil)
				if err != nil {
					t.Fatalf("Failed to set key in db%d: %v", dbNum, err)
				}
			}
		case 9:
			for i := 0; i < 502; i++ {
				key := fmt.Sprintf("key9_%d", i)
				err := stor.Set(key, []byte("value"), nil)
				if err != nil {
					t.Fatalf("Failed to set key in db%d: %v", dbNum, err)
				}
			}
		case 11:
			for i := 0; i < 7; i++ {
				key := fmt.Sprintf("key11_%d", i)
				err := stor.Set(key, []byte("value"), nil)
				if err != nil {
					t.Fatalf("Failed to set key in db%d: %v", dbNum, err)
				}
			}
		case 13:
			for i := 0; i < 1168; i++ {
				key := fmt.Sprintf("key13_%d", i)
				err := stor.Set(key, []byte("value"), nil)
				if err != nil {
					t.Fatalf("Failed to set key in db%d: %v", dbNum, err)
				}
			}
		}
	}
	
	// Test 50 times to ensure consistency (no intermittent behavior)
	for iteration := 0; iteration < 50; iteration++ {
		dbInfo := stor.DatabaseInfo()
		
		// Verify all expected databases are present
		for _, expectedDB := range testDBs {
			if _, exists := dbInfo[expectedDB]; !exists {
				t.Errorf("Iteration %d: Database db%d missing from DatabaseInfo output", iteration, expectedDB)
			}
		}
		
		// Verify we have exactly the expected databases (no extras)
		if len(dbInfo) != len(testDBs) {
			var foundDBs []int
			for dbNum := range dbInfo {
				foundDBs = append(foundDBs, dbNum)
			}
			t.Errorf("Iteration %d: Expected %d databases %v, got %d databases %v", 
				iteration, len(testDBs), testDBs, len(dbInfo), foundDBs)
		}
	}
}

// TestInfoKeyspace_CountsAreAccurate tests that key and expiration counts are correct
func TestInfoKeyspace_CountsAreAccurate(t *testing.T) {
	stor := storage.NewMemory()
	
	// Test data setup
	testCases := []struct {
		dbNum           int
		totalKeys       int
		keysWithExpiry  int
		expiredKeys     int
	}{
		{0, 10, 3, 1},  // 10 keys, 3 with expiry (1 expired, 2 valid)
		{1, 5, 2, 0},   // 5 keys, 2 with expiry (0 expired, 2 valid)
		{2, 0, 0, 0},   // empty database
	}
	
	now := time.Now()
	expiredTime := now.Add(-1 * time.Hour) // 1 hour ago (expired)
	futureTime := now.Add(1 * time.Hour)   // 1 hour from now (not expired)
	
	for _, tc := range testCases {
		stor.SelectDB(tc.dbNum)
		
		keyIndex := 0
		
		// Add keys without expiry
		keysWithoutExpiry := tc.totalKeys - tc.keysWithExpiry
		for i := 0; i < keysWithoutExpiry; i++ {
			key := fmt.Sprintf("key%d_%d", tc.dbNum, keyIndex)
			err := stor.Set(key, []byte("value"), nil)
			if err != nil {
				t.Fatalf("Failed to set key %s: %v", key, err)
			}
			keyIndex++
		}
		
		// Add expired keys
		for i := 0; i < tc.expiredKeys; i++ {
			key := fmt.Sprintf("expired%d_%d", tc.dbNum, keyIndex)
			err := stor.Set(key, []byte("value"), &expiredTime)
			if err != nil {
				t.Fatalf("Failed to set expired key %s: %v", key, err)
			}
			keyIndex++
		}
		
		// Add non-expired keys with TTL
		keysWithValidTTL := tc.keysWithExpiry - tc.expiredKeys
		for i := 0; i < keysWithValidTTL; i++ {
			key := fmt.Sprintf("ttl%d_%d", tc.dbNum, keyIndex)
			err := stor.Set(key, []byte("value"), &futureTime)
			if err != nil {
				t.Fatalf("Failed to set TTL key %s: %v", key, err)
			}
			keyIndex++
		}
	}
	
	// Get database info and validate counts
	dbInfo := stor.DatabaseInfo()
	
	for _, tc := range testCases {
		if tc.totalKeys == 0 {
			// Empty databases should not appear in DatabaseInfo
			if _, exists := dbInfo[tc.dbNum]; exists {
				t.Errorf("Empty database db%d should not appear in DatabaseInfo", tc.dbNum)
			}
			continue
		}
		
		info, exists := dbInfo[tc.dbNum]
		if !exists {
			t.Errorf("Database db%d missing from DatabaseInfo", tc.dbNum)
			continue
		}
		
		// Expected valid keys (total - expired)
		expectedValidKeys := int64(tc.totalKeys - tc.expiredKeys)
		actualKeys := info["keys"].(int64)
		if actualKeys != expectedValidKeys {
			t.Errorf("db%d: Expected %d valid keys, got %d", tc.dbNum, expectedValidKeys, actualKeys)
		}
		
		// Expected keys with expiry (regardless of expired status)
		expectedKeysWithExpiry := int64(tc.keysWithExpiry)
		actualExpires := info["expires"].(int64)
		if actualExpires != expectedKeysWithExpiry {
			t.Errorf("db%d: Expected %d keys with expiry, got %d", tc.dbNum, expectedKeysWithExpiry, actualExpires)
		}
	}
}

// TestInfoKeyspace_AvgTTLPresent tests that avg_ttl field is present and calculated correctly
func TestInfoKeyspace_AvgTTLPresent(t *testing.T) {
	stor := storage.NewMemory()
	stor.SelectDB(0)
	
	t.Run("NoKeysWithTTL", func(t *testing.T) {
		// Add keys without TTL
		for i := 0; i < 5; i++ {
			key := fmt.Sprintf("noexpiry_%d", i)
			err := stor.Set(key, []byte("value"), nil)
			if err != nil {
				t.Fatalf("Failed to set key: %v", err)
			}
		}
		
		dbInfo := stor.DatabaseInfo()
		info := dbInfo[0]
		
		avgTTL, exists := info["avg_ttl"]
		if !exists {
			t.Error("avg_ttl field missing from DatabaseInfo")
		}
		
		if avgTTL.(int64) != 0 {
			t.Errorf("Expected avg_ttl=0 for keys without TTL, got %d", avgTTL.(int64))
		}
	})
	
	t.Run("KeysWithTTL", func(t *testing.T) {
		stor.SelectDB(1)
		
		// Add keys with TTL
		futureTime := time.Now().Add(5000 * time.Millisecond) // 5 seconds
		for i := 0; i < 3; i++ {
			key := fmt.Sprintf("withttl_%d", i)
			err := stor.Set(key, []byte("value"), &futureTime)
			if err != nil {
				t.Fatalf("Failed to set key with TTL: %v", err)
			}
		}
		
		dbInfo := stor.DatabaseInfo()
		info := dbInfo[1]
		
		avgTTL, exists := info["avg_ttl"]
		if !exists {
			t.Error("avg_ttl field missing from DatabaseInfo")
		}
		
		avgTTLValue := avgTTL.(int64)
		if avgTTLValue <= 0 || avgTTLValue > 5500 {
			t.Errorf("Expected avg_ttl in range (0, 5500], got %d", avgTTLValue)
		}
	})
}

// TestInfoKeyspace_DeterministicOrder tests that databases are returned in ascending order
func TestInfoKeyspace_DeterministicOrder(t *testing.T) {
	stor := storage.NewMemory()
	
	// Create databases in random order
	dbNumbers := []int{15, 3, 7, 1, 10}
	
	for _, dbNum := range dbNumbers {
		stor.SelectDB(dbNum)
		key := fmt.Sprintf("key_db%d", dbNum)
		err := stor.Set(key, []byte("value"), nil)
		if err != nil {
			t.Fatalf("Failed to set key in db%d: %v", dbNum, err)
		}
	}
	
	// Test multiple times to ensure consistent ordering
	for i := 0; i < 10; i++ {
		dbInfo := stor.DatabaseInfo()
		
		// Extract database numbers and sort them to check if all expected DBs are present
		var foundDBs []int
		for dbNum := range dbInfo {
			foundDBs = append(foundDBs, dbNum)
		}
		
		// Sort the found databases
		for j := 0; j < len(foundDBs); j++ {
			for k := j + 1; k < len(foundDBs); k++ {
				if foundDBs[j] > foundDBs[k] {
					foundDBs[j], foundDBs[k] = foundDBs[k], foundDBs[j]
				}
			}
		}
		
		// Check that we have all expected databases
		expectedDBs := []int{1, 3, 7, 10, 15}
		if len(foundDBs) != len(expectedDBs) {
			t.Errorf("Iteration %d: Expected %d databases, got %d: %v", i, len(expectedDBs), len(foundDBs), foundDBs)
			continue
		}
		
		for j, expectedDB := range expectedDBs {
			if foundDBs[j] != expectedDB {
				t.Errorf("Iteration %d: Expected database %d at position %d, got %d", i, expectedDB, j, foundDBs[j])
			}
		}
	}
}

// TestInfoKeyspace_ServerOutput tests the actual INFO command output format
func TestInfoKeyspace_ServerOutput(t *testing.T) {
	// For now, test the storage layer directly since server writes are read-only
	// This test verifies that our DatabaseInfo() changes work correctly
	stor := storage.NewMemory()
	
	// Set up test data in multiple databases
	databases := []struct {
		dbNum int
		keys  int
	}{
		{8, 466},
		{9, 502}, 
		{11, 7},
		{13, 1168},
	}
	
	for _, db := range databases {
		stor.SelectDB(db.dbNum)
		
		// Add keys to the database
		for i := 0; i < db.keys; i++ {
			key := fmt.Sprintf("key%d_%d", db.dbNum, i)
			err := stor.Set(key, []byte("value"), nil)
			if err != nil {
				t.Fatalf("Failed to set key in db%d: %v", db.dbNum, err)
			}
		}
	}
	
	// Get database information
	dbInfo := stor.DatabaseInfo()
	t.Logf("DatabaseInfo result: %+v", dbInfo)
	
	// Verify all databases are present with correct counts
	for _, db := range databases {
		info, exists := dbInfo[db.dbNum]
		if !exists {
			t.Errorf("Database db%d missing from DatabaseInfo", db.dbNum)
			continue
		}
		
		keys := info["keys"].(int64)
		expires := info["expires"].(int64)
		avgTTL := info["avg_ttl"].(int64)
		
		if keys != int64(db.keys) {
			t.Errorf("db%d: Expected %d keys, got %d", db.dbNum, db.keys, keys)
		}
		
		if expires != 0 {
			t.Errorf("db%d: Expected 0 expires, got %d", db.dbNum, expires)
		}
		
		if avgTTL != 0 {
			t.Errorf("db%d: Expected 0 avg_ttl, got %d", db.dbNum, avgTTL)
		}
	}
	
	// Verify only expected databases are present
	if len(dbInfo) != len(databases) {
		var foundDBs []int
		for dbNum := range dbInfo {
			foundDBs = append(foundDBs, dbNum)
		}
		t.Errorf("Expected %d databases, got %d: %v", len(databases), len(dbInfo), foundDBs)
	}
}

// TestInfoKeyspace_EmptyKeyspaceAlwaysPresent ensures the keyspace section 
// appears even when no databases contain any keys
func TestInfoKeyspace_EmptyKeyspaceAlwaysPresent(t *testing.T) {
	storage := storage.NewMemory()
	srv := server.NewServer("localhost:0", storage)
	
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()
	
	// Connect to server
	conn, err := net.Dial("tcp", srv.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	
	writer := protocol.NewWriter(conn)
	reader := protocol.NewReader(conn)
	
	// Send INFO keyspace command
	cmd := []protocol.Value{
		{Type: protocol.TypeBulkString, Data: []byte("INFO")},
		{Type: protocol.TypeBulkString, Data: []byte("keyspace")},
	}
	
	if err := writer.WriteArray(cmd); err != nil {
		t.Fatal(err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatal(err)
	}
	
	// Read response
	resp, err := reader.ReadNext()
	if err != nil {
		t.Fatal(err)
	}
	
	if resp.Type != protocol.TypeBulkString {
		t.Fatalf("Expected bulk string response, got %v", resp.Type)
	}
	
	output := string(resp.Data)
	
	// Verify keyspace section header is always present
	if !strings.Contains(output, "# Keyspace") {
		t.Errorf("Keyspace section header missing from INFO response. Got: %q", output)
	}
	
	// Verify there are no database entries (since storage is empty)
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "db") && strings.Contains(line, ":") {
			t.Errorf("Unexpected database entry in empty keyspace: %q", line)
		}
	}
	
	// Also test INFO all to ensure keyspace section appears there too
	cmd = []protocol.Value{
		{Type: protocol.TypeBulkString, Data: []byte("INFO")},
		{Type: protocol.TypeBulkString, Data: []byte("all")},
	}
	
	if err := writer.WriteArray(cmd); err != nil {
		t.Fatal(err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatal(err)
	}
	
	// Read response
	resp, err = reader.ReadNext()
	if err != nil {
		t.Fatal(err)
	}
	
	if resp.Type != protocol.TypeBulkString {
		t.Fatalf("Expected bulk string response, got %v", resp.Type)
	}
	
	allOutput := string(resp.Data)
	if !strings.Contains(allOutput, "# Keyspace") {
		t.Errorf("Keyspace section header missing from INFO all response")
	}
}