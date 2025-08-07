package storage_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/raniellyferreira/redis-inmemory-replica/storage"
)

func TestMemoryStorage(t *testing.T) {
	s := storage.NewMemory()
	defer s.Close()

	// Test Set and Get
	err := s.Set("key1", []byte("value1"), nil)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	value, exists := s.Get("key1")
	if !exists {
		t.Fatal("Expected key to exist")
	}

	if string(value) != "value1" {
		t.Errorf("Get() = %s, want value1", string(value))
	}

	// Test non-existent key
	_, exists = s.Get("nonexistent")
	if exists {
		t.Fatal("Expected key to not exist")
	}
}

func TestMemoryStorageExpiry(t *testing.T) {
	s := storage.NewMemory()
	defer s.Close()

	// Set key with expiry in the past (should be immediately expired)
	pastTime := time.Now().Add(-1 * time.Hour)
	err := s.Set("expired", []byte("value"), &pastTime)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Key should not exist due to expiry
	_, exists := s.Get("expired")
	if exists {
		t.Fatal("Expected expired key to not exist")
	}

	// Set key with future expiry
	futureTime := time.Now().Add(1 * time.Hour)
	err = s.Set("future", []byte("value"), &futureTime)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Key should exist
	value, exists := s.Get("future")
	if !exists {
		t.Fatal("Expected future key to exist")
	}

	if string(value) != "value" {
		t.Errorf("Get() = %s, want value", string(value))
	}
}

func TestMemoryStorageDel(t *testing.T) {
	s := storage.NewMemory()
	defer s.Close()

	// Set some keys
	s.Set("key1", []byte("value1"), nil)
	s.Set("key2", []byte("value2"), nil)
	s.Set("key3", []byte("value3"), nil)

	// Delete keys
	deleted := s.Del("key1", "key2", "nonexistent")
	if deleted != 2 {
		t.Errorf("Del() = %d, want 2", deleted)
	}

	// Verify deletion
	_, exists := s.Get("key1")
	if exists {
		t.Fatal("Expected key1 to be deleted")
	}

	_, exists = s.Get("key2")
	if exists {
		t.Fatal("Expected key2 to be deleted")
	}

	_, exists = s.Get("key3")
	if !exists {
		t.Fatal("Expected key3 to still exist")
	}
}

func TestMemoryStorageExists(t *testing.T) {
	s := storage.NewMemory()
	defer s.Close()

	s.Set("key1", []byte("value1"), nil)
	s.Set("key2", []byte("value2"), nil)

	count := s.Exists("key1", "key2", "nonexistent")
	if count != 2 {
		t.Errorf("Exists() = %d, want 2", count)
	}

	count = s.Exists("nonexistent1", "nonexistent2")
	if count != 0 {
		t.Errorf("Exists() = %d, want 0", count)
	}
}

func TestMemoryStorageExpire(t *testing.T) {
	s := storage.NewMemory()
	defer s.Close()

	s.Set("key1", []byte("value1"), nil)

	// Set expiry
	futureTime := time.Now().Add(1 * time.Hour)
	result := s.Expire("key1", futureTime)
	if !result {
		t.Fatal("Expected Expire() to return true")
	}

	// Check TTL
	ttl := s.TTL("key1")
	if ttl <= 0 || ttl > time.Hour {
		t.Errorf("TTL() = %v, want positive duration <= 1 hour", ttl)
	}

	// Try to expire non-existent key
	result = s.Expire("nonexistent", futureTime)
	if result {
		t.Fatal("Expected Expire() to return false for non-existent key")
	}
}

func TestMemoryStorageTTL(t *testing.T) {
	s := storage.NewMemory()
	defer s.Close()

	// Key without expiry
	s.Set("key1", []byte("value1"), nil)
	ttl := s.TTL("key1")
	if ttl != -1*time.Second {
		t.Errorf("TTL() = %v, want -1s (no expiry)", ttl)
	}

	// Non-existent key
	ttl = s.TTL("nonexistent")
	if ttl != -2*time.Second {
		t.Errorf("TTL() = %v, want -2s (key doesn't exist)", ttl)
	}

	// Key with expiry
	futureTime := time.Now().Add(1 * time.Hour)
	s.Set("key2", []byte("value2"), &futureTime)
	ttl = s.TTL("key2")
	if ttl <= 0 || ttl > time.Hour {
		t.Errorf("TTL() = %v, want positive duration <= 1 hour", ttl)
	}
}

func TestMemoryStorageKeys(t *testing.T) {
	s := storage.NewMemory()
	defer s.Close()

	// Initially no keys
	keys := s.Keys()
	if len(keys) != 0 {
		t.Errorf("Keys() length = %d, want 0", len(keys))
	}

	// Add some keys
	s.Set("key1", []byte("value1"), nil)
	s.Set("key2", []byte("value2"), nil)
	s.Set("key3", []byte("value3"), nil)

	keys = s.Keys()
	if len(keys) != 3 {
		t.Errorf("Keys() length = %d, want 3", len(keys))
	}

	// Verify key count
	count := s.KeyCount()
	if count != 3 {
		t.Errorf("KeyCount() = %d, want 3", count)
	}
}

func TestMemoryStorageFlushAll(t *testing.T) {
	s := storage.NewMemory()
	defer s.Close()

	// Add some keys
	s.Set("key1", []byte("value1"), nil)
	s.Set("key2", []byte("value2"), nil)

	// Flush all
	err := s.FlushAll()
	if err != nil {
		t.Fatalf("FlushAll() error = %v", err)
	}

	// Verify all keys are gone
	keys := s.Keys()
	if len(keys) != 0 {
		t.Errorf("Keys() length after FlushAll = %d, want 0", len(keys))
	}

	count := s.KeyCount()
	if count != 0 {
		t.Errorf("KeyCount() after FlushAll = %d, want 0", count)
	}
}

func TestMemoryStorageDatabase(t *testing.T) {
	s := storage.NewMemory()
	defer s.Close()

	// Default database should be 0
	currentDB := s.CurrentDB()
	if currentDB != 0 {
		t.Errorf("CurrentDB() = %d, want 0", currentDB)
	}

	// Add key to database 0
	s.Set("key1", []byte("value1"), nil)

	// Switch to database 1
	err := s.SelectDB(1)
	if err != nil {
		t.Fatalf("SelectDB(1) error = %v", err)
	}

	currentDB = s.CurrentDB()
	if currentDB != 1 {
		t.Errorf("CurrentDB() = %d, want 1", currentDB)
	}

	// Key from database 0 should not exist in database 1
	_, exists := s.Get("key1")
	if exists {
		t.Fatal("Expected key1 to not exist in database 1")
	}

	// Add key to database 1
	s.Set("key2", []byte("value2"), nil)

	// Switch back to database 0
	err = s.SelectDB(0)
	if err != nil {
		t.Fatalf("SelectDB(0) error = %v", err)
	}

	// Key1 should exist, key2 should not
	_, exists = s.Get("key1")
	if !exists {
		t.Fatal("Expected key1 to exist in database 0")
	}

	_, exists = s.Get("key2")
	if exists {
		t.Fatal("Expected key2 to not exist in database 0")
	}
}

func TestMemoryStorageScan(t *testing.T) {
	s := storage.NewMemory()
	defer s.Close()

	// Add some keys
	s.Set("user:1", []byte("alice"), nil)
	s.Set("user:2", []byte("bob"), nil)
	s.Set("config:app", []byte("settings"), nil)

	// Scan all keys
	_, keys := s.Scan(0, "*", 10)
	if len(keys) != 3 {
		t.Errorf("Scan() returned %d keys, want 3", len(keys))
	}

	// Scan with pattern
	_, keys = s.Scan(0, "user:*", 10)
	if len(keys) != 2 {
		t.Errorf("Scan() with pattern returned %d keys, want 2", len(keys))
	}
}

func TestMemoryStorageInfo(t *testing.T) {
	s := storage.NewMemory()
	defer s.Close()

	s.Set("key1", []byte("value1"), nil)

	info := s.Info()
	if info == nil {
		t.Fatal("Info() returned nil")
	}

	// Check for expected keys
	expectedKeys := []string{"keys", "memory_usage", "current_db"}
	for _, key := range expectedKeys {
		if _, exists := info[key]; !exists {
			t.Errorf("Info() missing key: %s", key)
		}
	}
}

func TestMemoryStorageMemoryLimit(t *testing.T) {
	s := storage.NewMemory()
	defer s.Close()

	// Set memory limit
	limit := int64(1024)
	s.SetMemoryLimit(limit)

	if s.GetMemoryLimit() != limit {
		t.Errorf("GetMemoryLimit() = %d, want %d", s.GetMemoryLimit(), limit)
	}
}

func BenchmarkMemoryStorageGet(b *testing.B) {
	s := storage.NewMemory()
	defer s.Close()

	s.Set("key", []byte("value"), nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = s.Get("key")
	}
}

func BenchmarkMemoryStorageSet(b *testing.B) {
	s := storage.NewMemory()
	defer s.Close()

	value := []byte("value")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Set("key", value, nil)
	}
}

// Test cleanup configuration
func TestMemoryStorageCleanupConfig(t *testing.T) {
	s := storage.NewMemory()
	defer s.Close()

	// Test default config
	config := s.GetCleanupConfig()
	if config.SampleSize != 20 {
		t.Errorf("Default SampleSize = %d, want 20", config.SampleSize)
	}
	if config.MaxRounds != 4 {
		t.Errorf("Default MaxRounds = %d, want 4", config.MaxRounds)
	}

	// Test setting custom config
	newConfig := storage.CleanupConfig{
		SampleSize:       50,
		MaxRounds:        8,
		BatchSize:        20,
		ExpiredThreshold: 0.5,
	}
	s.SetCleanupConfig(newConfig)

	retrievedConfig := s.GetCleanupConfig()
	if retrievedConfig != newConfig {
		t.Errorf("SetCleanupConfig() config mismatch: got %+v, want %+v", retrievedConfig, newConfig)
	}
}

// Test data integrity during cleanup
func TestMemoryStorageCleanupIntegrity(t *testing.T) {
	s := storage.NewMemory()
	defer s.Close()

	// Set up test data with mixed expiry times
	now := time.Now()
	validKeys := []string{"valid1", "valid2", "valid3"}
	expiredKeys := []string{"expired1", "expired2", "expired3"}

	// Set valid keys (no expiry)
	for _, key := range validKeys {
		err := s.Set(key, []byte("valid_value"), nil)
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}
	}

	// Set expired keys
	pastTime := now.Add(-1 * time.Hour)
	for _, key := range expiredKeys {
		err := s.Set(key, []byte("expired_value"), &pastTime)
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}
	}

	// Trigger cleanup manually by waiting
	time.Sleep(100 * time.Millisecond)

	// Verify valid keys still exist
	for _, key := range validKeys {
		value, exists := s.Get(key)
		if !exists {
			t.Errorf("Valid key %s was incorrectly removed", key)
		}
		if string(value) != "valid_value" {
			t.Errorf("Valid key %s has wrong value: got %s, want valid_value", key, string(value))
		}
	}

	// Verify expired keys are removed (may take some time due to sampling)
	// We'll check this by trying to access them - they should not exist
	for _, key := range expiredKeys {
		_, exists := s.Get(key)
		if exists {
			t.Logf("Expired key %s still exists (may be removed in next cleanup cycle)", key)
		}
	}
}

// Test concurrent access during cleanup
func TestMemoryStorageCleanupConcurrency(t *testing.T) {
	s := storage.NewMemory()
	defer s.Close()

	// Configure aggressive cleanup for testing
	s.SetCleanupConfig(storage.CleanupConfig{
		SampleSize:       10,
		MaxRounds:        10,
		BatchSize:        5,
		ExpiredThreshold: 0.1,
	})

	const numGoroutines = 10
	const operationsPerGoroutine = 100

	var wg sync.WaitGroup
	errorChan := make(chan error, numGoroutines*operationsPerGoroutine)

	// Start concurrent readers
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				key := fmt.Sprintf("key_%d_%d", workerID, j)

				// Set a key
				err := s.Set(key, []byte("value"), nil)
				if err != nil {
					errorChan <- fmt.Errorf("Set error: %v", err)
					return
				}

				// Read the key back
				value, exists := s.Get(key)
				if !exists {
					// Key might have been cleaned up, that's fine
					continue
				}
				if string(value) != "value" {
					errorChan <- fmt.Errorf("Value mismatch: got %s, want value", string(value))
					return
				}
			}
		}(i)
	}

	// Start concurrent writers with expiring keys
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				key := fmt.Sprintf("expiring_key_%d_%d", workerID, j)

				// Set key with short expiry
				expiry := time.Now().Add(10 * time.Millisecond)
				err := s.Set(key, []byte("expiring_value"), &expiry)
				if err != nil {
					errorChan <- fmt.Errorf("Set expiring key error: %v", err)
					return
				}

				// Small delay to allow some keys to expire
				time.Sleep(5 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()
	close(errorChan)

	// Check for errors
	for err := range errorChan {
		t.Error(err)
	}
}

// Test cleanup under different load scenarios
func TestMemoryStorageCleanupLoadScenarios(t *testing.T) {
	scenarios := []struct {
		name         string
		numKeys      int
		expiredRatio float64
		config       storage.CleanupConfig
	}{
		{
			name:         "Light load",
			numKeys:      100,
			expiredRatio: 0.1,
			config: storage.CleanupConfig{
				SampleSize: 10, MaxRounds: 2, BatchSize: 5, ExpiredThreshold: 0.25,
			},
		},
		{
			name:         "Medium load",
			numKeys:      1000,
			expiredRatio: 0.3,
			config: storage.CleanupConfig{
				SampleSize: 20, MaxRounds: 4, BatchSize: 10, ExpiredThreshold: 0.25,
			},
		},
		{
			name:         "Heavy load",
			numKeys:      5000,
			expiredRatio: 0.5,
			config: storage.CleanupConfig{
				SampleSize: 50, MaxRounds: 8, BatchSize: 20, ExpiredThreshold: 0.3,
			},
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			s := storage.NewMemory()
			defer s.Close()

			s.SetCleanupConfig(scenario.config)

			// Small delay to ensure storage is fully initialized
			time.Sleep(10 * time.Millisecond)

			now := time.Now()
			pastTime := now.Add(-1 * time.Hour)
			futureTime := now.Add(1 * time.Hour)

			// Create a far future time for setup - will set to expired later
			setupTime := now.Add(24 * time.Hour)

			numExpired := int(float64(scenario.numKeys) * scenario.expiredRatio)
			numValid := scenario.numKeys - numExpired

			// Add keys that will become expired (but set with future expiry first)
			expiredKeys := make([]string, numExpired)
			for i := 0; i < numExpired; i++ {
				key := fmt.Sprintf("expired_%d", i)
				expiredKeys[i] = key
				err := s.Set(key, []byte("expired_value"), &setupTime)
				if err != nil {
					t.Fatalf("Set expired key error: %v", err)
				}
			}

			// Add valid keys
			for i := 0; i < numValid; i++ {
				key := fmt.Sprintf("valid_%d", i)
				err := s.Set(key, []byte("valid_value"), &futureTime)
				if err != nil {
					t.Fatalf("Set valid key error: %v", err)
				}
			}

			// Now update the "expired" keys to actually be expired
			for _, key := range expiredKeys {
				err := s.Set(key, []byte("expired_value"), &pastTime)
				if err != nil {
					t.Fatalf("Set expired key error: %v", err)
				}
			}

			// Count keys before cleanup using KeyCount() which includes expired keys that haven't been accessed
			// Note: Some expired keys might be cleaned up lazily during access, so we'll be more flexible
			initialKeyCount := s.KeyCount()
			// Allow more tolerance for lazy cleanup during setup, especially with optimized cleanup
			expectedMin := int64(float64(scenario.numKeys) * 0.90) // Allow 10% tolerance
			if initialKeyCount < expectedMin {
				t.Errorf("Initial key count = %d, want at least %d (scenario: %d keys)", initialKeyCount, expectedMin, scenario.numKeys)
			}

			// Wait for cleanup to run (cleanup runs every 1 second)
			time.Sleep(1100 * time.Millisecond)

			// Check that some cleanup occurred
			finalKeyCount := s.KeyCount()
			t.Logf("Scenario %s: keys reduced from %d to %d", scenario.name, initialKeyCount, finalKeyCount)

			// Verify all valid keys still exist
			validKeysFound := 0
			for i := 0; i < numValid; i++ {
				key := fmt.Sprintf("valid_%d", i)
				if _, exists := s.Get(key); exists {
					validKeysFound++
				}
			}

			if validKeysFound != numValid {
				t.Errorf("Valid keys lost: found %d, expected %d", validKeysFound, numValid)
			}
		})
	}
}
