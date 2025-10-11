package storage

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// BenchmarkConcurrentGet benchmarks concurrent Get operations
func BenchmarkConcurrentGet(b *testing.B) {
	benchmarkCases := []struct {
		name       string
		numShards  int
		goroutines int
	}{
		{"1Shard-1Goroutine", 1, 1},
		{"1Shard-10Goroutines", 1, 10},
		{"16Shards-10Goroutines", 16, 10},
		{"256Shards-10Goroutines", 256, 10},
		{"256Shards-50Goroutines", 256, 50},
	}

	for _, bc := range benchmarkCases {
		b.Run(bc.name, func(b *testing.B) {
			stor := NewMemoryWithShards(bc.numShards)
			defer func() { _ = stor.Close() }()

			// Populate with data
			numKeys := 1000
			for i := 0; i < numKeys; i++ {
				key := fmt.Sprintf("key_%d", i)
				value := []byte(fmt.Sprintf("value_%d", i))
				if err := stor.Set(key, value, nil); err != nil {
					b.Fatalf("Set failed: %v", err)
				}
			}

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				i := 0
				for pb.Next() {
					key := fmt.Sprintf("key_%d", i%numKeys)
					stor.Get(key)
					i++
				}
			})
		})
	}
}

// BenchmarkConcurrentSet benchmarks concurrent Set operations
func BenchmarkConcurrentSet(b *testing.B) {
	benchmarkCases := []struct {
		name       string
		numShards  int
		goroutines int
	}{
		{"1Shard-1Goroutine", 1, 1},
		{"1Shard-10Goroutines", 1, 10},
		{"16Shards-10Goroutines", 16, 10},
		{"256Shards-10Goroutines", 256, 10},
		{"256Shards-50Goroutines", 256, 50},
	}

	for _, bc := range benchmarkCases {
		b.Run(bc.name, func(b *testing.B) {
			stor := NewMemoryWithShards(bc.numShards)
			defer func() { _ = stor.Close() }()

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				i := 0
				for pb.Next() {
					key := fmt.Sprintf("key_%d", i)
					value := []byte(fmt.Sprintf("value_%d", i))
					_ = stor.Set(key, value, nil)
					i++
				}
			})
		})
	}
}

// BenchmarkConcurrentMixed benchmarks mixed read/write operations
func BenchmarkConcurrentMixed(b *testing.B) {
	benchmarkCases := []struct {
		name      string
		numShards int
	}{
		{"1Shard", 1},
		{"16Shards", 16},
		{"256Shards", 256},
	}

	for _, bc := range benchmarkCases {
		b.Run(bc.name, func(b *testing.B) {
			stor := NewMemoryWithShards(bc.numShards)
			defer func() { _ = stor.Close() }()

			// Populate with initial data
			numKeys := 100
			for i := 0; i < numKeys; i++ {
				key := fmt.Sprintf("key_%d", i)
				value := []byte(fmt.Sprintf("value_%d", i))
				if err := stor.Set(key, value, nil); err != nil {
					b.Fatalf("Set failed: %v", err)
				}
			}

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				i := 0
				for pb.Next() {
					key := fmt.Sprintf("key_%d", i%numKeys)
					// Mix of operations: 70% reads, 20% writes, 10% deletes
					op := i % 10
					if op < 7 {
						stor.Get(key)
					} else if op < 9 {
						value := []byte(fmt.Sprintf("value_%d", i))
						_ = stor.Set(key, value, nil)
					} else {
						stor.Del(key)
					}
					i++
				}
			})
		})
	}
}

// BenchmarkKeysAggregation benchmarks aggregation operations across shards
func BenchmarkKeysAggregation(b *testing.B) {
	benchmarkCases := []struct {
		name      string
		numShards int
		numKeys   int
	}{
		{"1Shard-100Keys", 1, 100},
		{"16Shards-100Keys", 16, 100},
		{"256Shards-100Keys", 256, 100},
		{"256Shards-1000Keys", 256, 1000},
	}

	for _, bc := range benchmarkCases {
		b.Run(bc.name, func(b *testing.B) {
			stor := NewMemoryWithShards(bc.numShards)
			defer func() { _ = stor.Close() }()

			// Populate with data
			for i := 0; i < bc.numKeys; i++ {
				key := fmt.Sprintf("key_%d", i)
				value := []byte(fmt.Sprintf("value_%d", i))
				if err := stor.Set(key, value, nil); err != nil {
					b.Fatalf("Set failed: %v", err)
				}
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				stor.Keys("*")
			}
		})
	}
}

// BenchmarkConcurrentExpiration benchmarks expiration handling with sharding
func BenchmarkConcurrentExpiration(b *testing.B) {
	benchmarkCases := []struct {
		name      string
		numShards int
	}{
		{"1Shard", 1},
		{"16Shards", 16},
		{"256Shards", 256},
	}

	for _, bc := range benchmarkCases {
		b.Run(bc.name, func(b *testing.B) {
			stor := NewMemoryWithShards(bc.numShards)
			defer func() { _ = stor.Close() }()

			futureTime := time.Now().Add(1 * time.Hour)

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				i := 0
				for pb.Next() {
					key := fmt.Sprintf("key_%d", i)
					value := []byte(fmt.Sprintf("value_%d", i))
					_ = stor.Set(key, value, &futureTime)
					stor.TTL(key)
					i++
				}
			})
		})
	}
}

// BenchmarkShardedMemoryUsage benchmarks memory usage calculation across shards
func BenchmarkShardedMemoryUsage(b *testing.B) {
	benchmarkCases := []struct {
		name      string
		numShards int
		numKeys   int
	}{
		{"1Shard-100Keys", 1, 100},
		{"16Shards-100Keys", 16, 100},
		{"256Shards-100Keys", 256, 100},
		{"256Shards-1000Keys", 256, 1000},
	}

	for _, bc := range benchmarkCases {
		b.Run(bc.name, func(b *testing.B) {
			stor := NewMemoryWithShards(bc.numShards)
			defer func() { _ = stor.Close() }()

			// Populate with data
			for i := 0; i < bc.numKeys; i++ {
				key := fmt.Sprintf("key_%d", i)
				value := []byte(fmt.Sprintf("value_%d", i))
				if err := stor.Set(key, value, nil); err != nil {
					b.Fatalf("Set failed: %v", err)
				}
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				stor.MemoryUsage()
			}
		})
	}
}

// BenchmarkShardContention simulates high contention scenarios
func BenchmarkShardContention(b *testing.B) {
	benchmarkCases := []struct {
		name      string
		numShards int
		numKeys   int // Small number of keys creates more contention
	}{
		{"1Shard-10Keys", 1, 10},
		{"16Shards-10Keys", 16, 10},
		{"256Shards-10Keys", 256, 10},
		{"1Shard-100Keys", 1, 100},
		{"256Shards-100Keys", 256, 100},
	}

	for _, bc := range benchmarkCases {
		b.Run(bc.name, func(b *testing.B) {
			stor := NewMemoryWithShards(bc.numShards)
			defer func() { _ = stor.Close() }()

			// Pre-populate keys
			for i := 0; i < bc.numKeys; i++ {
				key := fmt.Sprintf("key_%d", i)
				value := []byte(fmt.Sprintf("value_%d", i))
				if err := stor.Set(key, value, nil); err != nil {
					b.Fatalf("Set failed: %v", err)
				}
			}

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				i := 0
				for pb.Next() {
					// All goroutines access same small set of keys
					key := fmt.Sprintf("key_%d", i%bc.numKeys)
					value := []byte(fmt.Sprintf("value_%d", i))

					// Mix of operations to simulate real workload
					if i%3 == 0 {
						_ = stor.Set(key, value, nil)
					} else {
						stor.Get(key)
					}
					i++
				}
			})
		})
	}
}

// TestConcurrentStress performs a stress test with many concurrent operations
func TestConcurrentStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	stor := NewMemory()
	defer func() { _ = stor.Close() }()

	numGoroutines := 100
	numOperations := 1000

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines)

	// Start multiple goroutines performing various operations
	wg.Add(numGoroutines)
	for g := 0; g < numGoroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < numOperations; i++ {
				key := fmt.Sprintf("key_%d_%d", id, i%100)
				value := []byte(fmt.Sprintf("value_%d_%d", id, i))

				// Perform various operations
				switch i % 7 {
				case 0, 1, 2: // 43% Sets
					if err := stor.Set(key, value, nil); err != nil {
						errors <- fmt.Errorf("Set failed: %w", err)
						return
					}
				case 3, 4: // 29% Gets
					stor.Get(key)
				case 5: // 14% Exists
					stor.Exists(key)
				case 6: // 14% Del
					stor.Del(key)
				}
			}
		}(g)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errors)

	// Check for any errors
	for err := range errors {
		t.Error(err)
	}

	// Verify storage is still functional
	if err := stor.Set("test", []byte("value"), nil); err != nil {
		t.Errorf("Storage broken after stress test: %v", err)
	}

	if _, ok := stor.Get("test"); !ok {
		t.Error("Cannot retrieve key after stress test")
	}
}
