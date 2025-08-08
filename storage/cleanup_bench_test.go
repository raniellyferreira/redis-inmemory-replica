package storage_test

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/raniellyferreira/redis-inmemory-replica/storage"
)

// BenchmarkCleanupLegacy simulates the old cleanup behavior for comparison
func BenchmarkCleanupLegacy(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping long benchmark in short mode")
	}

	scenarios := []struct {
		name        string
		totalKeys   int
		expiredKeys int
	}{
		{"Small_100", 100, 20},
		{"Medium_500", 500, 100}, // Reduced from 1000
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			b.StopTimer()

			s := storage.NewMemory()
			defer func() { _ = s.Close() }()

			// Disable the background cleanup for this test
			now := time.Now()
			pastTime := now.Add(-1 * time.Hour)
			futureTime := now.Add(1 * time.Hour)

			// Add expired keys
			for i := 0; i < scenario.expiredKeys; i++ {
				key := fmt.Sprintf("expired_%d", i)
				_ = s.Set(key, []byte("expired_value"), &pastTime)
			}

			// Add valid keys
			for i := 0; i < scenario.totalKeys-scenario.expiredKeys; i++ {
				key := fmt.Sprintf("valid_%d", i)
				_ = s.Set(key, []byte("valid_value"), &futureTime)
			}

			b.StartTimer()

			// Benchmark the cleanup simulation
			for i := 0; i < b.N; i++ {
				// This simulates the old approach - check every key
				s.Keys() // This will internally check all keys for expiration
			}
		})
	}
}

// BenchmarkCleanupIncremental benchmarks the new incremental cleanup
func BenchmarkCleanupIncremental(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping long benchmark in short mode")
	}

	scenarios := []struct {
		name        string
		totalKeys   int
		expiredKeys int
		config      storage.CleanupConfig
	}{
		{
			"Small_100",
			100, 20,
			storage.CleanupConfig{SampleSize: 10, MaxRounds: 2, BatchSize: 5, ExpiredThreshold: 0.25},
		},
		{
			"Medium_500", // Reduced from 1000
			500, 100,
			storage.CleanupConfig{SampleSize: 20, MaxRounds: 4, BatchSize: 10, ExpiredThreshold: 0.25},
		},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			b.StopTimer()

			s := storage.NewMemory()
			defer func() { _ = s.Close() }()

			s.SetCleanupConfig(scenario.config)

			now := time.Now()
			pastTime := now.Add(-1 * time.Hour)
			futureTime := now.Add(1 * time.Hour)

			// Add expired keys
			for i := 0; i < scenario.expiredKeys; i++ {
				key := fmt.Sprintf("expired_%d", i)
				_ = s.Set(key, []byte("expired_value"), &pastTime)
			}

			// Add valid keys
			for i := 0; i < scenario.totalKeys-scenario.expiredKeys; i++ {
				key := fmt.Sprintf("valid_%d", i)
				_ = s.Set(key, []byte("valid_value"), &futureTime)
			}

			b.StartTimer()

			// The actual incremental cleanup is harder to benchmark directly
			// since it runs in background. Instead, we benchmark scanning
			// which uses the improved approach
			for i := 0; i < b.N; i++ {
				_, keys := s.Scan(0, "*", int64(scenario.config.SampleSize))
				_ = keys // Use the result
			}
		})
	}
}

// BenchmarkCleanupConcurrentAccess benchmarks cleanup under concurrent load
func BenchmarkCleanupConcurrentAccess(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping concurrent benchmark in short mode")
	}

	s := storage.NewMemory()
	defer func() { _ = s.Close() }()

	// Configure for frequent cleanup
	s.SetCleanupConfig(storage.CleanupConfig{
		SampleSize:       20,
		MaxRounds:        4,
		BatchSize:        10,
		ExpiredThreshold: 0.2,
	})

	// Pre-populate with data
	now := time.Now()
	pastTime := now.Add(-1 * time.Hour)
	futureTime := now.Add(1 * time.Hour)

	for i := 0; i < 1000; i++ {
		// Mix of expired and valid keys
		if i%3 == 0 {
			_ = s.Set(fmt.Sprintf("expired_%d", i), []byte("value"), &pastTime)
		} else {
			_ = s.Set(fmt.Sprintf("valid_%d", i), []byte("value"), &futureTime)
		}
	}

	b.ResetTimer()

	// Run concurrent operations
	b.RunParallel(func(pb *testing.PB) {
		counter := 0
		for pb.Next() {
			key := fmt.Sprintf("bench_key_%d", counter%100)

			// Mix of operations
			switch counter % 4 {
			case 0:
				_ = s.Set(key, []byte("value"), nil)
			case 1:
				s.Get(key)
			case 2:
				s.Exists(key)
			case 3:
				s.TTL(key)
			}

			counter++
		}
	})
}

// BenchmarkCleanupConfigOptimization tests different cleanup configurations
func BenchmarkCleanupConfigOptimization(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping config optimization benchmark in short mode")
	}

	configs := []struct {
		name   string
		config storage.CleanupConfig
	}{
		{
			"Conservative",
			storage.CleanupConfig{SampleSize: 10, MaxRounds: 2, BatchSize: 5, ExpiredThreshold: 0.5},
		},
		{
			"Balanced",
			storage.CleanupConfig{SampleSize: 20, MaxRounds: 4, BatchSize: 10, ExpiredThreshold: 0.25},
		},
		{
			"Aggressive",
			storage.CleanupConfig{SampleSize: 50, MaxRounds: 8, BatchSize: 25, ExpiredThreshold: 0.1},
		},
	}

	for _, cfg := range configs {
		b.Run(cfg.name, func(b *testing.B) {
			s := storage.NewMemory()
			defer func() { _ = s.Close() }()

			s.SetCleanupConfig(cfg.config)

			// Setup data with 30% expired keys
			now := time.Now()
			pastTime := now.Add(-1 * time.Hour)
			futureTime := now.Add(1 * time.Hour)

			const totalKeys = 1000
			const expiredRatio = 0.3
			expiredCount := int(totalKeys * expiredRatio)

			for i := 0; i < expiredCount; i++ {
				_ = s.Set(fmt.Sprintf("expired_%d", i), []byte("value"), &pastTime)
			}
			for i := expiredCount; i < totalKeys; i++ {
				_ = s.Set(fmt.Sprintf("valid_%d", i), []byte("value"), &futureTime)
			}

			b.ResetTimer()

			// Simulate workload
			for i := 0; i < b.N; i++ {
				// Simulate mixed read/write workload
				key := fmt.Sprintf("workload_%d", i%100)
				_ = s.Set(key, []byte("value"), nil)
				s.Get(key)

				// Occasionally trigger operations that might interact with cleanup
				if i%10 == 0 {
					s.KeyCount()
				}
			}
		})
	}
}

// BenchmarkMemoryUsageCleanup measures memory efficiency
func BenchmarkMemoryUsageCleanup(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping memory usage benchmark in short mode")
	}

	s := storage.NewMemory()
	defer func() { _ = s.Close() }()

	// Configure for efficient cleanup
	s.SetCleanupConfig(storage.CleanupConfig{
		SampleSize:       30,
		MaxRounds:        6,
		BatchSize:        15,
		ExpiredThreshold: 0.2,
	})

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Create keys with very short expiry
		shortExpiry := time.Now().Add(1 * time.Millisecond)
		key := fmt.Sprintf("temp_key_%d", i)
		_ = s.Set(key, []byte("temporary_value"), &shortExpiry)

		// Allow some time for cleanup
		if i%100 == 0 {
			runtime.Gosched()
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Check memory usage efficiency
	memUsage := s.MemoryUsage()
	keyCount := s.KeyCount()

	b.ReportMetric(float64(memUsage), "bytes")
	b.ReportMetric(float64(keyCount), "keys")
}

// BenchmarkCleanupLatency measures latency impact of cleanup
func BenchmarkCleanupLatency(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping latency benchmark in short mode")
	}

	s := storage.NewMemory()
	defer func() { _ = s.Close() }()

	// Setup with many keys
	now := time.Now()
	pastTime := now.Add(-1 * time.Hour)
	futureTime := now.Add(1 * time.Hour)

	// 70% expired keys to trigger cleanup
	for i := 0; i < 7000; i++ {
		_ = s.Set(fmt.Sprintf("expired_%d", i), []byte("value"), &pastTime)
	}
	for i := 0; i < 3000; i++ {
		_ = s.Set(fmt.Sprintf("valid_%d", i), []byte("value"), &futureTime)
	}

	b.ResetTimer()

	// Measure operation latency during cleanup
	for i := 0; i < b.N; i++ {
		start := time.Now()

		// Perform a typical operation
		key := fmt.Sprintf("latency_test_%d", i%1000)
		_ = s.Set(key, []byte("test_value"), nil)
		value, _ := s.Get(key)
		_ = value

		latency := time.Since(start)
		if i%1000 == 0 {
			b.ReportMetric(float64(latency.Nanoseconds()), "ns/op")
		}
	}
}
