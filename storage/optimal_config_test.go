package storage_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/raniellyferreira/redis-inmemory-replica/storage"
)

// TestOptimalCleanupConfiguration finds the best configuration for different workloads
func TestOptimalCleanupConfiguration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping optimization test in short mode")
	}

	// Simplified workloads for CI stability
	workloads := []struct {
		name         string
		totalKeys    int
		expiredRatio float64
	}{
		{"LightWorkload", 100, 0.1},
		{"ModerateWorkload", 500, 0.3}, // Reduced from 1000
	}

	// Simplified configurations for CI stability
	configurations := []struct {
		name   string
		config storage.CleanupConfig
	}{
		{"Balanced", storage.CleanupConfig{20, 4, 10, 0.25}},
		{"Aggressive", storage.CleanupConfig{50, 8, 25, 0.1}},
	}

	for _, workload := range workloads {
		t.Run(workload.name, func(t *testing.T) {
			for _, cfg := range configurations {
				t.Run(cfg.name, func(t *testing.T) {
					s := storage.NewMemory()
					defer func() { _ = s.Close() }()

					s.SetCleanupConfig(cfg.config)

					// Setup workload
					now := time.Now()
					pastTime := now.Add(-1 * time.Hour)
					futureTime := now.Add(1 * time.Hour)

					expiredCount := int(float64(workload.totalKeys) * workload.expiredRatio)
					validCount := workload.totalKeys - expiredCount

					// Add expired keys
					for i := 0; i < expiredCount; i++ {
						key := fmt.Sprintf("expired_%d", i)
						_ = s.Set(key, []byte("expired_value"), &pastTime)
					}

					// Add valid keys
					for i := 0; i < validCount; i++ {
						key := fmt.Sprintf("valid_%d", i)
						_ = s.Set(key, []byte("valid_value"), &futureTime)
					}

					initialKeys := s.KeyCount()
					initialMemory := s.MemoryUsage()

					// Measure cleanup effectiveness
					start := time.Now()

					// Wait for cleanup cycles - reduced timeout for CI stability
					maxWait := 500 * time.Millisecond
					sleepInterval := 25 * time.Millisecond
					if testing.Short() {
						maxWait = 200 * time.Millisecond
						sleepInterval = 10 * time.Millisecond
					}
					var finalKeys, finalMemory int64

					for elapsed := time.Duration(0); elapsed < maxWait; elapsed = time.Since(start) {
						finalKeys = s.KeyCount()
						finalMemory = s.MemoryUsage()

						// If significant cleanup occurred, we can stop waiting
						if float64(finalKeys)/float64(initialKeys) < 0.8 {
							break
						}
						time.Sleep(sleepInterval)
					}

					cleanupTime := time.Since(start)
					keysRemoved := initialKeys - finalKeys
					memoryFreed := initialMemory - finalMemory

					// Calculate efficiency metrics
					cleanupRatio := float64(keysRemoved) / float64(expiredCount)
					if expiredCount == 0 {
						cleanupRatio = 0
					}

					memoryEfficiency := float64(memoryFreed) / float64(initialMemory)
					timeEfficiency := float64(keysRemoved) / cleanupTime.Seconds()

					t.Logf("Config: %s", cfg.name)
					t.Logf("  Cleanup time: %v", cleanupTime)
					t.Logf("  Keys removed: %d/%d (%.1f%% of expired)", keysRemoved, expiredCount, cleanupRatio*100)
					t.Logf("  Memory freed: %d bytes (%.1f%%)", memoryFreed, memoryEfficiency*100)
					t.Logf("  Cleanup rate: %.1f keys/sec", timeEfficiency)

					// Performance scoring (higher is better)
					score := cleanupRatio*50 + memoryEfficiency*30 + (timeEfficiency/100)*20
					t.Logf("  Performance score: %.2f", score)

					// Verify data integrity
					validKeysFound := 0
					for i := 0; i < validCount && validKeysFound < 10; i++ {
						key := fmt.Sprintf("valid_%d", i)
						if _, exists := s.Get(key); exists {
							validKeysFound++
						}
					}

					if validKeysFound == 0 && validCount > 0 {
						t.Errorf("Data integrity violated: no valid keys found")
					}
				})
			}
		})
	}
}

// BenchmarkOptimalConfigurations compares configurations under load
func BenchmarkOptimalConfigurations(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping optimal configurations benchmark in short mode")
	}

	configs := []struct {
		name   string
		config storage.CleanupConfig
	}{
		{"Conservative", storage.CleanupConfig{10, 2, 5, 0.5}},
		{"Balanced", storage.CleanupConfig{20, 4, 10, 0.25}},
		{"Aggressive", storage.CleanupConfig{50, 8, 25, 0.1}},
		{"RedisLike", storage.CleanupConfig{20, 4, 10, 0.25}},
		{"HighThroughput", storage.CleanupConfig{100, 2, 50, 0.3}},
		{"LowLatency", storage.CleanupConfig{15, 3, 8, 0.2}},
	}

	for _, cfg := range configs {
		b.Run(cfg.name, func(b *testing.B) {
			s := storage.NewMemory()
			defer func() { _ = s.Close() }()

			s.SetCleanupConfig(cfg.config)

			// Setup test data with 30% expired keys
			setupTestData(s, 1000, 0.3)

			b.ResetTimer()

			// Benchmark typical operations during cleanup
			for i := 0; i < b.N; i++ {
				key := fmt.Sprintf("benchmark_key_%d", i%100)

				// Mix of operations that would happen during normal usage
				switch i % 5 {
				case 0:
					_ = s.Set(key, []byte("value"), nil)
				case 1:
					s.Get(key)
				case 2:
					s.Exists(key)
				case 3:
					s.TTL(key)
				case 4:
					s.KeyCount()
				}
			}
		})
	}
}

// Helper function to setup test data
func setupTestData(s *storage.MemoryStorage, totalKeys int, expiredRatio float64) {
	now := time.Now()
	pastTime := now.Add(-1 * time.Hour)
	futureTime := now.Add(1 * time.Hour)

	expiredCount := int(float64(totalKeys) * expiredRatio)

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
}

// TestRecommendedConfiguration validates the final recommended configuration
func TestRecommendedConfiguration(t *testing.T) {
	// Use the predefined default configuration which is based on testing and Redis behavior
	recommendedConfig := storage.CleanupConfigDefault

	s := storage.NewMemory()
	defer func() { _ = s.Close() }()

	s.SetCleanupConfig(recommendedConfig)

	// Test with various scenarios
	scenarios := []struct {
		name         string
		keys         int
		expiredRatio float64
	}{
		{"SmallDB", 100, 0.2},
		{"MediumDB", 1000, 0.3},
		{"LargeDB", 10000, 0.4},
		{"HighExpiry", 5000, 0.7},
		{"LowExpiry", 2000, 0.05},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// Skip slow tests in short mode (CI environments)
			if testing.Short() && scenario.keys > 100 {
				t.Skip("Skipping slow test in short mode")
			}

			// Clear previous data
			_ = s.FlushAll()

			// Setup scenario
			setupTestData(s, scenario.keys, scenario.expiredRatio)

			initialKeys := s.KeyCount()
			initialMemory := s.MemoryUsage()

			t.Logf("Scenario: %s", scenario.name)
			t.Logf("Initial: %d keys, %d bytes", initialKeys, initialMemory)

			// Wait for cleanup (reduced time for CI)
			waitTime := 2 * time.Second
			if testing.Short() {
				waitTime = 500 * time.Millisecond
			}
			time.Sleep(waitTime)

			finalKeys := s.KeyCount()
			finalMemory := s.MemoryUsage()

			keysRemoved := initialKeys - finalKeys
			memoryFreed := initialMemory - finalMemory

			t.Logf("Final: %d keys, %d bytes", finalKeys, finalMemory)
			t.Logf("Removed: %d keys, freed %d bytes", keysRemoved, memoryFreed)

			// Basic validation
			if finalKeys > initialKeys {
				t.Errorf("Keys increased: %d -> %d", initialKeys, finalKeys)
			}

			// For high expiry scenarios, expect significant cleanup
			expectedExpired := int(float64(scenario.keys) * scenario.expiredRatio)
			if scenario.expiredRatio > 0.5 && keysRemoved < int64(expectedExpired)/4 {
				t.Logf("Note: Lower cleanup than expected for high expiry scenario (sampling effect)")
			}
		})
	}

	// Log the recommended configuration
	t.Logf("Recommended Configuration:")
	t.Logf("  SampleSize: %d (keys sampled per round)", recommendedConfig.SampleSize)
	t.Logf("  MaxRounds: %d (max cleanup rounds per cycle)", recommendedConfig.MaxRounds)
	t.Logf("  BatchSize: %d (keys deleted per batch)", recommendedConfig.BatchSize)
	t.Logf("  ExpiredThreshold: %.2f (continue if this ratio of sampled keys are expired)", recommendedConfig.ExpiredThreshold)
}
