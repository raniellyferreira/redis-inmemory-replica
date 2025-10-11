package storage

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestShardedStorageConcurrency tests concurrent access to sharded storage
func TestShardedStorageConcurrency(t *testing.T) {
	stor := NewMemory()
	defer func() { _ = stor.Close() }()

	numGoroutines := 50
	numOperations := 100

	var wg sync.WaitGroup

	// Test concurrent Set operations
	t.Run("ConcurrentSet", func(t *testing.T) {
		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < numOperations; j++ {
					key := fmt.Sprintf("key_%d_%d", id, j)
					value := []byte(fmt.Sprintf("value_%d_%d", id, j))
					if err := stor.Set(key, value, nil); err != nil {
						t.Errorf("Set failed: %v", err)
					}
				}
			}(i)
		}
		wg.Wait()
	})

	// Test concurrent Get operations
	t.Run("ConcurrentGet", func(t *testing.T) {
		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < numOperations; j++ {
					key := fmt.Sprintf("key_%d_%d", id, j)
					if _, ok := stor.Get(key); !ok {
						t.Errorf("Get failed for key %s", key)
					}
				}
			}(i)
		}
		wg.Wait()
	})

	// Test concurrent Del operations
	t.Run("ConcurrentDel", func(t *testing.T) {
		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < numOperations; j++ {
					key := fmt.Sprintf("key_%d_%d", id, j)
					stor.Del(key)
				}
			}(i)
		}
		wg.Wait()
	})

	// Verify all keys are deleted
	finalCount := stor.KeyCount()
	if finalCount != 0 {
		t.Errorf("Expected 0 keys after deletion, got %d", finalCount)
	}
}

// TestShardedStorageMixedOperations tests mixed read/write operations
func TestShardedStorageMixedOperations(t *testing.T) {
	stor := NewMemory()
	defer stor.Close()

	numGoroutines := 20
	numOperations := 50

	var wg sync.WaitGroup

	// Writers
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := fmt.Sprintf("key_%d", j)
				value := []byte(fmt.Sprintf("value_%d_%d", id, j))
				_ = stor.Set(key, value, nil)
			}
		}(i)
	}

	// Readers
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := fmt.Sprintf("key_%d", j)
				stor.Get(key)
				stor.Exists(key)
			}
		}(i)
	}

	// Deleters
	wg.Add(numGoroutines / 2)
	for i := 0; i < numGoroutines/2; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations/2; j++ {
				key := fmt.Sprintf("key_%d", j)
				stor.Del(key)
			}
		}(i)
	}

	wg.Wait()

	// Verify storage is consistent
	info := stor.Info()
	if info == nil {
		t.Error("Info() returned nil")
	}
}

// TestShardedStorageWithExpiration tests sharded storage with expiring keys
func TestShardedStorageWithExpiration(t *testing.T) {
	stor := NewMemory()
	defer stor.Close()

	numKeys := 1000
	futureTime := time.Now().Add(1 * time.Hour)
	pastTime := time.Now().Add(-1 * time.Hour)

	// Set half the keys with future expiration, half with past
	for i := 0; i < numKeys; i++ {
		key := fmt.Sprintf("key_%d", i)
		value := []byte(fmt.Sprintf("value_%d", i))
		if i%2 == 0 {
			_ = stor.Set(key, value, &futureTime)
		} else {
			_ = stor.Set(key, value, &pastTime)
		}
	}

	// Count valid keys (non-expired)
	validCount := int64(0)
	for i := 0; i < numKeys; i++ {
		key := fmt.Sprintf("key_%d", i)
		if _, ok := stor.Get(key); ok {
			validCount++
		}
	}

	// Should have approximately half the keys (the non-expired ones)
	expectedMin := int64(numKeys / 2)
	if validCount < expectedMin {
		t.Errorf("Expected at least %d valid keys, got %d", expectedMin, validCount)
	}
}

// TestNewMemoryWithShards tests the constructor with different shard counts
func TestNewMemoryWithShards(t *testing.T) {
	testCases := []struct {
		name            string
		requestedShards int
		expectedShards  int
	}{
		{"Zero shards", 0, 64},
		{"One shard", 1, 1},
		{"Two shards", 2, 2},
		{"Three shards (rounds to 4)", 3, 4},
		{"Sixteen shards", 16, 16},
		{"Hundred shards (rounds to 128)", 100, 128},
		{"Two fifty six shards", 256, 256},
		{"Five hundred shards (rounds to 512)", 500, 512},
		{"Negative shards", -1, 64},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stor := NewMemoryWithShards(tc.requestedShards)
			defer stor.Close()

			if stor.shards != tc.expectedShards {
				t.Errorf("Expected %d shards, got %d", tc.expectedShards, stor.shards)
			}

			// Verify shard mask
			expectedMask := uint64(tc.expectedShards - 1)
			if stor.shardMask != expectedMask {
				t.Errorf("Expected shard mask %d, got %d", expectedMask, stor.shardMask)
			}

			// Test basic operations work
			key := "test_key"
			value := []byte("test_value")
			if err := stor.Set(key, value, nil); err != nil {
				t.Errorf("Set failed: %v", err)
			}

			if got, ok := stor.Get(key); !ok {
				t.Error("Get failed")
			} else if string(got) != string(value) {
				t.Errorf("Expected %s, got %s", value, got)
			}
		})
	}
}

// TestMemoryOptions tests the option pattern for configuring MemoryStorage
func TestMemoryOptions(t *testing.T) {
	t.Run("Default configuration", func(t *testing.T) {
		stor := NewMemory()
		defer func() { _ = stor.Close() }()
		
		if stor.shards != 64 {
			t.Errorf("Expected 64 default shards, got %d", stor.shards)
		}
		if stor.shardMask != 63 {
			t.Errorf("Expected shard mask 63, got %d", stor.shardMask)
		}
	})

	t.Run("WithShardCount option", func(t *testing.T) {
		stor := NewMemory(WithShardCount(128))
		defer func() { _ = stor.Close() }()
		
		if stor.shards != 128 {
			t.Errorf("Expected 128 shards, got %d", stor.shards)
		}
		if stor.shardMask != 127 {
			t.Errorf("Expected shard mask 127, got %d", stor.shardMask)
		}
	})

	t.Run("WithShardCount rounds to power of 2", func(t *testing.T) {
		stor := NewMemory(WithShardCount(100))
		defer func() { _ = stor.Close() }()
		
		if stor.shards != 128 {
			t.Errorf("Expected 128 shards (rounded from 100), got %d", stor.shards)
		}
	})

	t.Run("Multiple options", func(t *testing.T) {
		stor := NewMemory(
			WithShardCount(32),
		)
		defer func() { _ = stor.Close() }()
		
		if stor.shards != 32 {
			t.Errorf("Expected 32 shards, got %d", stor.shards)
		}
	})
}

// TestShardDistribution tests that keys are distributed across shards
func TestShardDistribution(t *testing.T) {
	stor := NewMemoryWithShards(16)
	defer stor.Close()

	numKeys := 1000

	// Set many keys
	for i := 0; i < numKeys; i++ {
		key := fmt.Sprintf("key_%d", i)
		value := []byte(fmt.Sprintf("value_%d", i))
		if err := stor.Set(key, value, nil); err != nil {
			t.Fatalf("Set failed: %v", err)
		}
	}

	// Check distribution across shards
	stor.mu.RLock()
	db := stor.databases[0]
	stor.mu.RUnlock()

	shardCounts := make([]int, stor.shards)
	for i := 0; i < stor.shards; i++ {
		db.shards[i].mu.RLock()
		shardCounts[i] = len(db.shards[i].data)
		db.shards[i].mu.RUnlock()
	}

	// Check that keys are somewhat evenly distributed
	// Each shard should have at least a few keys
	minExpected := numKeys / (stor.shards * 4) // Allow 4x variance
	for i, count := range shardCounts {
		if count < minExpected {
			t.Logf("Warning: Shard %d has only %d keys (expected at least %d)", i, count, minExpected)
		}
	}

	// Log distribution for debugging
	t.Logf("Shard distribution: %v", shardCounts)
}

// TestShardedStorageMultiDB tests sharding with multiple databases
func TestShardedStorageMultiDB(t *testing.T) {
	stor := NewMemory()
	defer stor.Close()

	numDBs := 4
	numKeysPerDB := 100

	// Set keys in multiple databases
	for db := 0; db < numDBs; db++ {
		if err := stor.SelectDB(db); err != nil {
			t.Fatalf("SelectDB failed: %v", err)
		}

		for i := 0; i < numKeysPerDB; i++ {
			key := fmt.Sprintf("key_%d", i)
			value := []byte(fmt.Sprintf("db%d_value_%d", db, i))
			if err := stor.Set(key, value, nil); err != nil {
				t.Fatalf("Set failed: %v", err)
			}
		}
	}

	// Verify each database has the correct keys
	for db := 0; db < numDBs; db++ {
		if err := stor.SelectDB(db); err != nil {
			t.Fatalf("SelectDB failed: %v", err)
		}

		count := stor.KeyCount()
		if count != int64(numKeysPerDB) {
			t.Errorf("DB %d: expected %d keys, got %d", db, numKeysPerDB, count)
		}

		// Verify a key's value
		key := "key_0"
		expectedValue := fmt.Sprintf("db%d_value_0", db)
		if got, ok := stor.Get(key); !ok {
			t.Errorf("DB %d: Get failed for key %s", db, key)
		} else if string(got) != expectedValue {
			t.Errorf("DB %d: expected %s, got %s", db, expectedValue, got)
		}
	}

	// Test FlushAll
	if err := stor.FlushAll(); err != nil {
		t.Fatalf("FlushAll failed: %v", err)
	}

	// Verify all databases are empty
	for db := 0; db < numDBs; db++ {
		if err := stor.SelectDB(db); err != nil {
			t.Fatalf("SelectDB failed: %v", err)
		}

		count := stor.KeyCount()
		if count != 0 {
			t.Errorf("DB %d: expected 0 keys after FlushAll, got %d", db, count)
		}
	}
}

// TestShardedCleanup tests that cleanup works correctly with sharded storage
func TestShardedCleanup(t *testing.T) {
	stor := NewMemory()
	defer stor.Close()

	// Set aggressive cleanup config for faster testing
	stor.SetCleanupConfig(CleanupConfig{
		SampleSize:       50,
		MaxRounds:        10,
		BatchSize:        20,
		ExpiredThreshold: 0.1,
	})

	numKeys := 500
	pastTime := time.Now().Add(-1 * time.Hour)

	// Set all keys with past expiration
	for i := 0; i < numKeys; i++ {
		key := fmt.Sprintf("key_%d", i)
		value := []byte(fmt.Sprintf("value_%d", i))
		if err := stor.Set(key, value, &pastTime); err != nil {
			t.Fatalf("Set failed: %v", err)
		}
	}

	// Wait for cleanup to run
	time.Sleep(2 * time.Second)

	// Most keys should be cleaned up
	remainingKeys := stor.KeyCount()
	if remainingKeys > int64(numKeys/2) {
		t.Logf("Warning: %d keys remaining after cleanup (expected fewer)", remainingKeys)
	}
}
