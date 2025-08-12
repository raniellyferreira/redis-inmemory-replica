package redisreplica_test

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	redisreplica "github.com/raniellyferreira/redis-inmemory-replica"
	"github.com/raniellyferreira/redis-inmemory-replica/protocol"
)

// parseKeyspaceInfo extracts database numbers from INFO keyspace output
func parseKeyspaceInfo(infoOutput string) (map[int]map[string]int, []int) {
	dbInfo := make(map[int]map[string]int)
	var dbNums []int
	lines := strings.Split(infoOutput, "\r\n")
	
	inKeyspaceSection := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "# Keyspace" {
			inKeyspaceSection = true
			continue
		}
		if inKeyspaceSection && strings.HasPrefix(line, "#") {
			break // End of keyspace section
		}
		if inKeyspaceSection && strings.HasPrefix(line, "db") {
			var dbNum int
			var rest string
			if n, err := fmt.Sscanf(line, "db%d:%s", &dbNum, &rest); n == 2 && err == nil {
				dbNums = append(dbNums, dbNum)
				
				// Parse the stats (keys=X,expires=Y)
				stats := make(map[string]int)
				parts := strings.Split(rest, ",")
				for _, part := range parts {
					keyVal := strings.Split(part, "=")
					if len(keyVal) == 2 {
						if val, err := strconv.Atoi(keyVal[1]); err == nil {
							stats[keyVal[0]] = val
						}
					}
				}
				dbInfo[dbNum] = stats
			}
		}
	}
	
	sort.Ints(dbNums)
	return dbInfo, dbNums
}

// callInfoKeyspace makes a network call to INFO keyspace command
func callInfoKeyspace(addr string) (map[int]map[string]int, []int, string, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to connect: %v", err)
	}
	defer conn.Close()

	reader := protocol.NewReader(conn)
	writer := protocol.NewWriter(conn)

	// Send INFO keyspace command
	err = writer.WriteArray(createCommand("INFO", "keyspace"))
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to write command: %v", err)
	}
	err = writer.Flush()
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to flush: %v", err)
	}

	// Read response
	response, err := reader.ReadNext()
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to read response: %v", err)
	}

	infoOutput := string(response.Data)
	dbInfo, dbNums := parseKeyspaceInfo(infoOutput)
	
	return dbInfo, dbNums, infoOutput, nil
}

// TestInfoKeyspaceListsAllPopulatedDBs ensures INFO keyspace consistently lists all populated databases
func TestInfoKeyspaceListsAllPopulatedDBs(t *testing.T) {
	// Create replica with different port to avoid conflicts
	replica, err := redisreplica.New(
		redisreplica.WithReplicaAddr(":6391"),
	)
	if err != nil {
		t.Fatalf("Failed to create replica: %v", err)
	}
	defer replica.Close()

	// Start server
	ctx := context.Background()
	if err := replica.Start(ctx); err != nil {
		t.Fatalf("Failed to start replica: %v", err)
	}

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	storage := replica.Storage()

	// Populate 4 databases with different key counts
	for db := 0; db < 4; db++ {
		if err := storage.SelectDB(db); err != nil {
			t.Fatalf("Failed to select DB %d: %v", db, err)
		}
		
		// Add different number of keys per database
		keyCount := 3 + db*2  // DB0: 3 keys, DB1: 5 keys, DB2: 7 keys, DB3: 9 keys
		for i := 0; i < keyCount; i++ {
			key := fmt.Sprintf("db%d_key_%d", db, i)
			if err := storage.Set(key, []byte(fmt.Sprintf("value_%d", i)), nil); err != nil {
				t.Fatalf("Failed to set %s: %v", key, err)
			}
		}
		
		// Add some keys with TTL
		if db%2 == 0 {
			future := time.Now().Add(1 * time.Hour)
			for i := 0; i < 2; i++ {
				key := fmt.Sprintf("db%d_ttl_key_%d", db, i)
				if err := storage.Set(key, []byte(fmt.Sprintf("ttl_value_%d", i)), &future); err != nil {
					t.Fatalf("Failed to set %s with TTL: %v", key, err)
				}
			}
		}
	}

	// Switch back to DB 0
	if err := storage.SelectDB(0); err != nil {
		t.Fatalf("Failed to select DB 0: %v", err)
	}

	// Test INFO keyspace multiple times to ensure consistency
	const numCalls = 100
	serverAddr := "localhost:6391"
	
	expectedDBs := []int{0, 1, 2, 3}
	expectedCounts := map[int]int{
		0: 5, // 3 regular + 2 TTL
		1: 5, // 5 regular + 0 TTL
		2: 9, // 7 regular + 2 TTL
		3: 9, // 9 regular + 0 TTL
	}
	
	for i := 0; i < numCalls; i++ {
		dbInfo, dbNums, rawOutput, err := callInfoKeyspace(serverAddr)
		if err != nil {
			t.Fatalf("Call %d: INFO keyspace failed: %v", i, err)
		}
		
		// Check that all expected databases are present
		if len(dbNums) != len(expectedDBs) {
			t.Errorf("Call %d: Expected %d databases, got %d. DBs: %v\nRaw output: %s", 
				i, len(expectedDBs), len(dbNums), dbNums, rawOutput)
		}
		
		// Check each expected database is present
		for _, expectedDB := range expectedDBs {
			found := false
			for _, actualDB := range dbNums {
				if actualDB == expectedDB {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Call %d: Database db%d missing from output. Found DBs: %v\nRaw output: %s", 
					i, expectedDB, dbNums, rawOutput)
			}
		}
		
		// Check ordering is consistent (sorted)
		for i := 1; i < len(dbNums); i++ {
			if dbNums[i] <= dbNums[i-1] {
				t.Errorf("Call %d: Database ordering not sorted: %v\nRaw output: %s", 
					i, dbNums, rawOutput)
			}
		}
		
		// Check key counts are correct
		for dbNum, stats := range dbInfo {
			if expectedCount, exists := expectedCounts[dbNum]; exists {
				if actualCount := stats["keys"]; actualCount != expectedCount {
					t.Errorf("Call %d: DB%d expected %d keys, got %d\nRaw output: %s", 
						i, dbNum, expectedCount, actualCount, rawOutput)
				}
			}
		}
	}
	
	t.Logf("✅ Successfully tested %d INFO keyspace calls - all consistent", numCalls)
}

// TestInfoKeyspaceStableOrdering verifies databases are always listed in ascending order
func TestInfoKeyspaceStableOrdering(t *testing.T) {
	// Create replica with different port
	replica, err := redisreplica.New(
		redisreplica.WithReplicaAddr(":6392"),
	)
	if err != nil {
		t.Fatalf("Failed to create replica: %v", err)
	}
	defer replica.Close()

	// Start server
	ctx := context.Background()
	if err := replica.Start(ctx); err != nil {
		t.Fatalf("Failed to start replica: %v", err)
	}

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	storage := replica.Storage()

	// Populate databases in non-sequential order to test sorting
	dbsToPopulate := []int{15, 3, 7, 1, 12, 0, 5}
	
	for _, db := range dbsToPopulate {
		if err := storage.SelectDB(db); err != nil {
			t.Fatalf("Failed to select DB %d: %v", db, err)
		}
		
		key := fmt.Sprintf("test_key_db%d", db)
		if err := storage.Set(key, []byte("test_value"), nil); err != nil {
			t.Fatalf("Failed to set key in DB %d: %v", db, err)
		}
	}

	// Test ordering consistency
	const numCalls = 50
	serverAddr := "localhost:6392"
	
	var previousOrder []int
	
	for i := 0; i < numCalls; i++ {
		_, dbNums, rawOutput, err := callInfoKeyspace(serverAddr)
		if err != nil {
			t.Fatalf("Call %d: INFO keyspace failed: %v", i, err)
		}
		
		// Check that order is ascending
		for j := 1; j < len(dbNums); j++ {
			if dbNums[j] <= dbNums[j-1] {
				t.Errorf("Call %d: Databases not in ascending order: %v\nRaw output: %s", 
					i, dbNums, rawOutput)
			}
		}
		
		// Check consistency with previous calls
		if i > 0 {
			if len(dbNums) != len(previousOrder) {
				t.Errorf("Call %d: Inconsistent database count. Previous: %v, Current: %v", 
					i, previousOrder, dbNums)
			}
			
			for j, dbNum := range dbNums {
				if j < len(previousOrder) && dbNum != previousOrder[j] {
					t.Errorf("Call %d: Order changed. Previous: %v, Current: %v", 
						i, previousOrder, dbNums)
				}
			}
		}
		
		previousOrder = make([]int, len(dbNums))
		copy(previousOrder, dbNums)
	}
	
	// Verify final order matches expected sorted order
	sort.Ints(dbsToPopulate)
	if len(previousOrder) != len(dbsToPopulate) {
		t.Errorf("Expected %d databases, got %d", len(dbsToPopulate), len(previousOrder))
	}
	
	for i, expected := range dbsToPopulate {
		if i < len(previousOrder) && previousOrder[i] != expected {
			t.Errorf("Expected database order %v, got %v", dbsToPopulate, previousOrder)
			break
		}
	}
	
	t.Logf("✅ Database ordering is stable and sorted: %v", previousOrder)
}

// TestInfoKeyspaceAccurateCounts validates that key counts and expiration counts are accurate
func TestInfoKeyspaceAccurateCounts(t *testing.T) {
	// Create replica with different port
	replica, err := redisreplica.New(
		redisreplica.WithReplicaAddr(":6393"),
	)
	if err != nil {
		t.Fatalf("Failed to create replica: %v", err)
	}
	defer replica.Close()

	// Start server
	ctx := context.Background()
	if err := replica.Start(ctx); err != nil {
		t.Fatalf("Failed to start replica: %v", err)
	}

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	storage := replica.Storage()
	serverAddr := "localhost:6393"

	// Test case 1: Database with only non-expiring keys
	if err := storage.SelectDB(0); err != nil {
		t.Fatalf("Failed to select DB 0: %v", err)
	}
	
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("permanent_key_%d", i)
		if err := storage.Set(key, []byte(fmt.Sprintf("value_%d", i)), nil); err != nil {
			t.Fatalf("Failed to set permanent key: %v", err)
		}
	}

	// Test case 2: Database with mixed keys (permanent + expiring)
	if err := storage.SelectDB(1); err != nil {
		t.Fatalf("Failed to select DB 1: %v", err)
	}
	
	// Add permanent keys
	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("permanent_key_%d", i)
		if err := storage.Set(key, []byte(fmt.Sprintf("value_%d", i)), nil); err != nil {
			t.Fatalf("Failed to set permanent key: %v", err)
		}
	}
	
	// Add expiring keys
	future := time.Now().Add(1 * time.Hour)
	for i := 0; i < 3; i++ {
		key := fmt.Sprintf("expiring_key_%d", i)
		if err := storage.Set(key, []byte(fmt.Sprintf("exp_value_%d", i)), &future); err != nil {
			t.Fatalf("Failed to set expiring key: %v", err)
		}
	}

	// Test case 3: Database with only expiring keys
	if err := storage.SelectDB(2); err != nil {
		t.Fatalf("Failed to select DB 2: %v", err)
	}
	
	for i := 0; i < 7; i++ {
		key := fmt.Sprintf("expiring_only_key_%d", i)
		if err := storage.Set(key, []byte(fmt.Sprintf("exp_value_%d", i)), &future); err != nil {
			t.Fatalf("Failed to set expiring key: %v", err)
		}
	}

	// Verify counts
	dbInfo, dbNums, rawOutput, err := callInfoKeyspace(serverAddr)
	if err != nil {
		t.Fatalf("INFO keyspace failed: %v", err)
	}

	expectedResults := map[int]map[string]int{
		0: {"keys": 10, "expires": 0},
		1: {"keys": 8, "expires": 3},
		2: {"keys": 7, "expires": 7},
	}

	for dbNum, expected := range expectedResults {
		if actual, exists := dbInfo[dbNum]; exists {
			for metric, expectedVal := range expected {
				if actualVal, ok := actual[metric]; ok {
					if actualVal != expectedVal {
						t.Errorf("DB%d %s: expected %d, got %d\nRaw output: %s", 
							dbNum, metric, expectedVal, actualVal, rawOutput)
					}
				} else {
					t.Errorf("DB%d missing %s metric\nRaw output: %s", 
						dbNum, metric, rawOutput)
				}
			}
		} else {
			t.Errorf("DB%d missing from INFO output\nFound DBs: %v\nRaw output: %s", 
				dbNum, dbNums, rawOutput)
		}
	}
	
	t.Logf("✅ Key counts and expiration counts are accurate")
}

// TestInfoKeyspaceConcurrent tests INFO keyspace under concurrent load
func TestInfoKeyspaceConcurrent(t *testing.T) {
	// Create replica with different port
	replica, err := redisreplica.New(
		redisreplica.WithReplicaAddr(":6394"),
	)
	if err != nil {
		t.Fatalf("Failed to create replica: %v", err)
	}
	defer replica.Close()

	// Start server
	ctx := context.Background()
	if err := replica.Start(ctx); err != nil {
		t.Fatalf("Failed to start replica: %v", err)
	}

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	storage := replica.Storage()
	serverAddr := "localhost:6394"

	// Populate initial databases
	for db := 0; db < 4; db++ {
		if err := storage.SelectDB(db); err != nil {
			t.Fatalf("Failed to select DB %d: %v", db, err)
		}
		
		for i := 0; i < 5; i++ {
			key := fmt.Sprintf("db%d_initial_key_%d", db, i)
			if err := storage.Set(key, []byte(fmt.Sprintf("value_%d", i)), nil); err != nil {
				t.Fatalf("Failed to set initial key: %v", err)
			}
		}
	}

	var wg sync.WaitGroup
	var mutex sync.Mutex
	infoResults := make([][]int, 0, 100)
	inconsistencies := make([]string, 0)

	// Start INFO readers
	for w := 0; w < 3; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			for i := 0; i < 50; i++ {
				_, dbNums, _, err := callInfoKeyspace(serverAddr)
				if err != nil {
					t.Errorf("Worker %d: INFO keyspace failed: %v", workerID, err)
					return
				}
				
				mutex.Lock()
				infoResults = append(infoResults, dbNums)
				
				// Check for missing core databases (0-3)
				coreDBs := []int{0, 1, 2, 3}
				for _, coreDB := range coreDBs {
					found := false
					for _, actualDB := range dbNums {
						if actualDB == coreDB {
							found = true
							break
						}
					}
					if !found {
						inconsistencies = append(inconsistencies, 
							fmt.Sprintf("Worker %d missing core DB%d. Found: %v", workerID, coreDB, dbNums))
					}
				}
				mutex.Unlock()
				
				// Small delay
				time.Sleep(time.Millisecond * 5)
			}
		}(w)
	}

	// Start concurrent writers
	for w := 0; w < 2; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			for i := 0; i < 30; i++ {
				dbNum := i % 4
				if err := storage.SelectDB(dbNum); err != nil {
					t.Errorf("Writer %d: Failed to select DB %d: %v", workerID, dbNum, err)
					return
				}
				
				// Add and remove keys
				key := fmt.Sprintf("concurrent_key_w%d_i%d", workerID, i)
				if err := storage.Set(key, []byte("concurrent_value"), nil); err != nil {
					t.Errorf("Writer %d: Failed to set key: %v", workerID, err)
				}
				
				// Occasionally delete keys
				if i%5 == 0 {
					deleteKey := fmt.Sprintf("concurrent_key_w%d_i%d", workerID, i-5)
					storage.Del(deleteKey)
				}
				
				time.Sleep(time.Millisecond * 3)
			}
		}(w)
	}

	wg.Wait()

	// Analyze results
	mutex.Lock()
	defer mutex.Unlock()

	if len(inconsistencies) > 0 {
		t.Errorf("Found %d inconsistencies during concurrent test:", len(inconsistencies))
		for i, inconsistency := range inconsistencies {
			if i < 5 { // Show first 5 inconsistencies
				t.Errorf("  %s", inconsistency)
			}
		}
		if len(inconsistencies) > 5 {
			t.Errorf("  ... and %d more", len(inconsistencies)-5)
		}
	}

	t.Logf("✅ Completed %d concurrent INFO calls with %d inconsistencies", 
		len(infoResults), len(inconsistencies))
}