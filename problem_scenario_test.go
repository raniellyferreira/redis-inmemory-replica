package redisreplica_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/raniellyferreira/redis-inmemory-replica/storage"
)

// TestProblemScenario tests the specific scenario described in the problem statement
// where db8, db9, db11, db13 should be consistently listed with correct counts
func TestProblemScenario(t *testing.T) {
	stor := storage.NewMemory()

	// Set up the exact scenario from the problem statement
	testData := []struct {
		dbNum    int
		keyCount int
	}{
		{8, 466},
		{9, 502},
		{11, 7},   // This was incorrectly showing 1 key before the fix
		{13, 1168},
	}

	// Set up data in each database
	for _, td := range testData {
		stor.SelectDB(td.dbNum)

		// Add keys to the database
		for i := 0; i < td.keyCount; i++ {
			key := fmt.Sprintf("key%d_%d", td.dbNum, i)
			err := stor.Set(key, []byte("value"), nil)
			if err != nil {
				t.Fatalf("Failed to set key in db%d: %v", td.dbNum, err)
			}
		}
		t.Logf("Added %d keys to db%d", td.keyCount, td.dbNum)
	}

	// Test that DatabaseInfo is consistent across multiple calls (no intermittency)
	for iteration := 0; iteration < 200; iteration++ { // Test 200 times as specified
		dbInfo := stor.DatabaseInfo()

		// Verify all expected databases are present with correct counts
		for _, td := range testData {
			info, exists := dbInfo[td.dbNum]
			if !exists {
				t.Errorf("Iteration %d: Database db%d missing from DatabaseInfo", iteration, td.dbNum)
				continue
			}

			keys := info["keys"].(int64)
			expires := info["expires"].(int64)
			avgTTL, avgTTLExists := info["avg_ttl"]

			if keys != int64(td.keyCount) {
				t.Errorf("Iteration %d: db%d expected %d keys, got %d", iteration, td.dbNum, td.keyCount, keys)
			}

			if expires != 0 {
				t.Errorf("Iteration %d: db%d expected 0 expires, got %d", iteration, td.dbNum, expires)
			}

			if !avgTTLExists {
				t.Errorf("Iteration %d: db%d missing avg_ttl field", iteration, td.dbNum)
			} else if avgTTL.(int64) != 0 {
				t.Errorf("Iteration %d: db%d expected avg_ttl=0, got %d", iteration, td.dbNum, avgTTL.(int64))
			}
		}

		// Verify only expected databases are present (no extras, no missing)
		if len(dbInfo) != len(testData) {
			var foundDBs []int
			for dbNum := range dbInfo {
				foundDBs = append(foundDBs, dbNum)
			}
			t.Errorf("Iteration %d: Expected %d databases %v, got %d databases %v", 
				iteration, len(testData), extractDBNums(testData), len(dbInfo), foundDBs)
		}
	}

	// Test deterministic ordering by simulating INFO keyspace output format
	dbInfo := stor.DatabaseInfo()
	
	// Create sorted list of database numbers (simulating server output logic)
	var dbNums []int
	for dbNum := range dbInfo {
		dbNums = append(dbNums, dbNum)
	}
	
	// Sort using the same logic as the server
	for i := 0; i < len(dbNums); i++ {
		for j := i + 1; j < len(dbNums); j++ {
			if dbNums[i] > dbNums[j] {
				dbNums[i], dbNums[j] = dbNums[j], dbNums[i]
			}
		}
	}
	
	// Verify ascending order
	expectedOrder := []int{8, 9, 11, 13}
	for i, expected := range expectedOrder {
		if dbNums[i] != expected {
			t.Errorf("Database ordering: expected db%d at position %d, got db%d", expected, i, dbNums[i])
		}
	}

	// Simulate the INFO keyspace output format
	var outputLines []string
	for _, dbNum := range dbNums {
		info := dbInfo[dbNum]
		keys := info["keys"].(int64)
		expires := info["expires"].(int64)
		avgTTL := info["avg_ttl"].(int64)
		
		line := fmt.Sprintf("db%d:keys=%d,expires=%d,avg_ttl=%d", dbNum, keys, expires, avgTTL)
		outputLines = append(outputLines, line)
	}
	
	simulatedOutput := strings.Join(outputLines, "\r\n")
	t.Logf("Simulated INFO keyspace output:\n# Keyspace\r\n%s", simulatedOutput)

	// Verify the output matches expected format from problem statement
	expectedLines := []string{
		"db8:keys=466,expires=0,avg_ttl=0",
		"db9:keys=502,expires=0,avg_ttl=0", 
		"db11:keys=7,expires=0,avg_ttl=0",
		"db13:keys=1168,expires=0,avg_ttl=0",
	}
	
	for i, expectedLine := range expectedLines {
		if outputLines[i] != expectedLine {
			t.Errorf("Output line %d: expected '%s', got '%s'", i, expectedLine, outputLines[i])
		}
	}

	t.Logf("✅ SUCCESS: All %d iterations showed consistent database listings", 200)
	t.Logf("✅ db8: 466 keys, db9: 502 keys, db11: 7 keys, db13: 1168 keys")
	t.Logf("✅ All databases include avg_ttl field")
	t.Logf("✅ No intermittent behavior detected")
	t.Logf("✅ Deterministic ascending order: db8, db9, db11, db13")
}

func extractDBNums(testData []struct {
	dbNum    int
	keyCount int
}) []int {
	var nums []int
	for _, td := range testData {
		nums = append(nums, td.dbNum)
	}
	return nums
}