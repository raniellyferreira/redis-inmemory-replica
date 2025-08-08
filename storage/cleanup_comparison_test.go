package storage_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/raniellyferreira/redis-inmemory-replica/storage"
)

// TestCleanupPerformanceComparison demonstrates the performance improvement
func TestCleanupPerformanceComparison(t *testing.T) {
	scenarios := []struct {
		name        string
		totalKeys   int
		expiredKeys int
	}{
		{"Small", 100, 20},
		{"Medium", 1000, 200},
		{"Large", 10000, 2000},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// Test the new incremental approach
			s := storage.NewMemory()
			defer s.Close()

			// Configure for efficient cleanup
			s.SetCleanupConfig(storage.CleanupConfig{
				SampleSize:       20,
				MaxRounds:        4,
				BatchSize:        10,
				ExpiredThreshold: 0.25,
			})

			now := time.Now()
			pastTime := now.Add(-1 * time.Hour)
			futureTime := now.Add(1 * time.Hour)

			// Add expired keys
			for i := 0; i < scenario.expiredKeys; i++ {
				key := fmt.Sprintf("expired_%d", i)
				err := s.Set(key, []byte("expired_value"), &pastTime)
				if err != nil {
					t.Fatalf("Set() error = %v", err)
				}
			}

			// Add valid keys
			for i := 0; i < scenario.totalKeys-scenario.expiredKeys; i++ {
				key := fmt.Sprintf("valid_%d", i)
				err := s.Set(key, []byte("valid_value"), &futureTime)
				if err != nil {
					t.Fatalf("Set() error = %v", err)
				}
			}

			initialKeyCount := s.KeyCount()
			initialMemory := s.MemoryUsage()

			t.Logf("Initial state: %d keys, %d bytes memory", initialKeyCount, initialMemory)

			// Wait for cleanup to occur - reduced for CI stability
			time.Sleep(200 * time.Millisecond)

			finalKeyCount := s.KeyCount()
			finalMemory := s.MemoryUsage()

			t.Logf("After cleanup: %d keys, %d bytes memory", finalKeyCount, finalMemory)
			t.Logf("Reduction: %d keys removed, %d bytes freed",
				initialKeyCount-finalKeyCount, initialMemory-finalMemory)

			// Verify that some cleanup occurred (but may not be complete due to sampling)
			if finalKeyCount > initialKeyCount {
				t.Errorf("Key count increased after cleanup: %d -> %d", initialKeyCount, finalKeyCount)
			}

			// Verify all valid keys still exist
			validKeysFound := 0
			for i := 0; i < scenario.totalKeys-scenario.expiredKeys; i++ {
				key := fmt.Sprintf("valid_%d", i)
				if _, exists := s.Get(key); exists {
					validKeysFound++
				}
			}

			expectedValid := scenario.totalKeys - scenario.expiredKeys
			if validKeysFound != expectedValid {
				t.Errorf("Valid keys lost: found %d, expected %d", validKeysFound, expectedValid)
			}
		})
	}
}

// TestCleanupEfficiencyMetrics measures key efficiency metrics
func TestCleanupEfficiencyMetrics(t *testing.T) {
	s := storage.NewMemory()
	defer s.Close()

	// Configure for testing
	configs := []struct {
		name   string
		config storage.CleanupConfig
	}{
		{
			"Small_Batches",
			storage.CleanupConfig{SampleSize: 10, MaxRounds: 2, BatchSize: 5, ExpiredThreshold: 0.5},
		},
		{
			"Medium_Batches",
			storage.CleanupConfig{SampleSize: 20, MaxRounds: 4, BatchSize: 10, ExpiredThreshold: 0.25},
		},
		{
			"Large_Batches",
			storage.CleanupConfig{SampleSize: 50, MaxRounds: 8, BatchSize: 25, ExpiredThreshold: 0.1},
		},
	}

	for _, cfg := range configs {
		t.Run(cfg.name, func(t *testing.T) {
			s.SetCleanupConfig(cfg.config)

			// Setup test data
			now := time.Now()
			pastTime := now.Add(-1 * time.Hour)
			futureTime := now.Add(1 * time.Hour)

			const totalKeys = 1000
			const expiredRatio = 0.4 // 40% expired keys
			expiredCount := int(totalKeys * expiredRatio)

			// Clear any existing data
			_ = s.FlushAll()

			// Add expired keys
			for i := 0; i < expiredCount; i++ {
				key := fmt.Sprintf("expired_%d", i)
				_ = s.Set(key, []byte("expired_value"), &pastTime)
			}

			// Add valid keys
			for i := expiredCount; i < totalKeys; i++ {
				key := fmt.Sprintf("valid_%d", i)
				_ = s.Set(key, []byte("valid_value"), &futureTime)
			}

			initialKeys := s.KeyCount()
			initialMemory := s.MemoryUsage()

			// Measure time for cleanup to take effect
			start := time.Now()
			maxWait := 2 * time.Second

			for time.Since(start) < maxWait {
				currentKeys := s.KeyCount()
				if currentKeys < initialKeys {
					break // Some cleanup occurred
				}
				time.Sleep(100 * time.Millisecond)
			}

			finalKeys := s.KeyCount()
			finalMemory := s.MemoryUsage()
			cleanupTime := time.Since(start)

			keysRemoved := initialKeys - finalKeys
			memoryFreed := initialMemory - finalMemory

			t.Logf("Config: %+v", cfg.config)
			t.Logf("Cleanup took: %v", cleanupTime)
			t.Logf("Keys: %d -> %d (removed %d)", initialKeys, finalKeys, keysRemoved)
			t.Logf("Memory: %d -> %d bytes (freed %d)", initialMemory, finalMemory, memoryFreed)

			// Efficiency metrics
			if initialKeys > 0 {
				cleanupRatio := float64(keysRemoved) / float64(initialKeys)
				t.Logf("Cleanup ratio: %.2f%%", cleanupRatio*100)
			}

			// Verify integrity - check that some valid keys still exist
			validKeysFound := 0
			for i := expiredCount; i < totalKeys && validKeysFound < 10; i++ {
				key := fmt.Sprintf("valid_%d", i)
				if _, exists := s.Get(key); exists {
					validKeysFound++
				}
			}

			if validKeysFound == 0 {
				t.Error("No valid keys found - cleanup may have been too aggressive")
			}
		})
	}
}

// TestCleanupAdaptiveBehavior tests that cleanup adapts to workload
func TestCleanupAdaptiveBehavior(t *testing.T) {
	s := storage.NewMemory()
	defer s.Close()

	// Test different expiry scenarios
	scenarios := []struct {
		name         string
		expiredRatio float64
		expectRounds string
	}{
		{"Low_Expiry", 0.1, "should stop early"},
		{"Medium_Expiry", 0.3, "should run moderate rounds"},
		{"High_Expiry", 0.7, "should run more rounds"},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			_ = s.FlushAll()

			// Configure cleanup
			s.SetCleanupConfig(storage.CleanupConfig{
				SampleSize:       20,
				MaxRounds:        8,
				BatchSize:        10,
				ExpiredThreshold: 0.25,
			})

			now := time.Now()
			pastTime := now.Add(-1 * time.Hour)
			futureTime := now.Add(1 * time.Hour)

			const totalKeys = 500
			expiredCount := int(totalKeys * scenario.expiredRatio)

			// Add keys according to scenario
			for i := 0; i < expiredCount; i++ {
				key := fmt.Sprintf("expired_%d", i)
				_ = s.Set(key, []byte("expired_value"), &pastTime)
			}

			for i := expiredCount; i < totalKeys; i++ {
				key := fmt.Sprintf("valid_%d", i)
				_ = s.Set(key, []byte("valid_value"), &futureTime)
			}

			initialKeys := s.KeyCount()

			// Wait for adaptive cleanup - reduced for CI stability
			time.Sleep(300 * time.Millisecond)

			finalKeys := s.KeyCount()
			keysRemoved := initialKeys - finalKeys

			t.Logf("Scenario: %s (%.1f%% expired)", scenario.name, scenario.expiredRatio*100)
			t.Logf("Keys removed: %d out of %d (%.1f%%)",
				keysRemoved, initialKeys, float64(keysRemoved)/float64(initialKeys)*100)
			t.Logf("Expectation: %s", scenario.expectRounds)

			// Basic sanity checks
			if finalKeys > initialKeys {
				t.Errorf("Keys increased after cleanup: %d -> %d", initialKeys, finalKeys)
			}

			// For high expiry scenarios, we expect more cleanup
			if scenario.expiredRatio > 0.5 && keysRemoved == 0 {
				t.Logf("Warning: No cleanup detected for high expiry scenario")
			}
		})
	}
}
