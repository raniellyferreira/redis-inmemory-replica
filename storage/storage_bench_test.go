package storage

import (
	"fmt"
	"testing"
	"time"
)

// BenchmarkStorageGet benchmarks Get operations with different scenarios
func BenchmarkStorageGet(b *testing.B) {
	scenarios := []struct {
		name     string
		setup    func(*MemoryStorage)
		hitRatio float64 // Percentage of cache hits (0-1)
	}{
		{
			name: "Hit_Small",
			setup: func(s *MemoryStorage) {
				_ = s.Set("key", []byte("small"), nil)
			},
			hitRatio: 1.0,
		},
		{
			name: "Hit_Medium",
			setup: func(s *MemoryStorage) {
				data := make([]byte, 1024) // 1KB
				_ = s.Set("key", data, nil)
			},
			hitRatio: 1.0,
		},
		{
			name: "Hit_Large",
			setup: func(s *MemoryStorage) {
				data := make([]byte, 1024*1024) // 1MB
				_ = s.Set("key", data, nil)
			},
			hitRatio: 1.0,
		},
		{
			name:     "Miss",
			setup:    func(s *MemoryStorage) {},
			hitRatio: 0.0,
		},
		{
			name: "Mixed_50pct",
			setup: func(s *MemoryStorage) {
				_ = s.Set("key1", []byte("value1"), nil)
				_ = s.Set("key2", []byte("value2"), nil)
			},
			hitRatio: 0.5,
		},
	}

	for _, sc := range scenarios {
		b.Run(sc.name, func(b *testing.B) {
			s := NewMemory()
			sc.setup(s)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				switch sc.hitRatio {
				case 1.0:
					s.Get("key")
				case 0.0:
					s.Get("nonexistent")
				default:
					// Mixed scenario
					if i%2 == 0 {
						s.Get("key1")
					} else {
						s.Get("nonexistent")
					}
				}
			}
		})
	}
}

// BenchmarkStorageSet benchmarks Set operations with different scenarios
func BenchmarkStorageSet(b *testing.B) {
	scenarios := []struct {
		name string
		size int
	}{
		{"Small_16B", 16},
		{"Medium_1KB", 1024},
		{"Large_64KB", 64 * 1024},
		{"VeryLarge_1MB", 1024 * 1024},
	}

	for _, sc := range scenarios {
		b.Run(sc.name, func(b *testing.B) {
			s := NewMemory()
			data := make([]byte, sc.size)

			b.ResetTimer()
			b.ReportAllocs()
			b.SetBytes(int64(sc.size))

			for i := 0; i < b.N; i++ {
				key := fmt.Sprintf("key_%d", i%1000) // Cycle through 1000 keys
				_ = s.Set(key, data, nil)
			}
		})
	}
}

// BenchmarkStorageSetWithTTL benchmarks Set operations with TTL
func BenchmarkStorageSetWithTTL(b *testing.B) {
	scenarios := []struct {
		name string
		ttl  time.Duration
		size int
	}{
		{"Small_1s", time.Second, 16},
		{"Small_1h", time.Hour, 16},
		{"Medium_1s", time.Second, 1024},
		{"Medium_1h", time.Hour, 1024},
	}

	for _, sc := range scenarios {
		b.Run(sc.name, func(b *testing.B) {
			s := NewMemory()
			data := make([]byte, sc.size)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				key := fmt.Sprintf("key_%d", i%1000)
				expiry := time.Now().Add(sc.ttl)
				_ = s.Set(key, data, &expiry)
			}
		})
	}
}

// BenchmarkStorageDelete benchmarks Delete operations
func BenchmarkStorageDelete(b *testing.B) {
	b.Run("Existing", func(b *testing.B) {
		s := NewMemory()
		// Pre-populate
		for i := 0; i < 10000; i++ {
			_ = s.Set(fmt.Sprintf("key_%d", i), []byte("value"), nil)
		}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			key := fmt.Sprintf("key_%d", i%10000)
			s.Del(key)
			// Re-add for next iteration
			if i%100 == 0 {
				_ = s.Set(key, []byte("value"), nil)
			}
		}
	})

	b.Run("NonExistent", func(b *testing.B) {
		s := NewMemory()

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			s.Del(fmt.Sprintf("nonexistent_%d", i))
		}
	})
}

// BenchmarkStorageKeys benchmarks Keys operation with different dataset sizes
func BenchmarkStorageKeys(b *testing.B) {
	sizes := []int{10, 100, 1000, 10000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Count_%d", size), func(b *testing.B) {
			s := NewMemory()
			// Pre-populate
			for i := 0; i < size; i++ {
				_ = s.Set(fmt.Sprintf("key_%d", i), []byte("value"), nil)
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				s.Keys("*")
			}
		})
	}
}

// BenchmarkStorageExpirationCheck benchmarks checking if keys are expired
func BenchmarkStorageExpirationCheck(b *testing.B) {
	scenarios := []struct {
		name           string
		expiredPercent float64
	}{
		{"NoExpired", 0.0},
		{"10PctExpired", 0.1},
		{"50PctExpired", 0.5},
		{"90PctExpired", 0.9},
	}

	for _, sc := range scenarios {
		b.Run(sc.name, func(b *testing.B) {
			s := NewMemory()
			totalKeys := 1000

			// Populate with mixed expired/non-expired keys
			expiredCount := int(float64(totalKeys) * sc.expiredPercent)
			for i := 0; i < totalKeys; i++ {
				key := fmt.Sprintf("key_%d", i)
				if i < expiredCount {
					// Already expired
					expiry := time.Now().Add(-time.Second)
					_ = s.Set(key, []byte("value"), &expiry)
				} else {
					// Not expired
					expiry := time.Now().Add(time.Hour)
					_ = s.Set(key, []byte("value"), &expiry)
				}
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				key := fmt.Sprintf("key_%d", i%totalKeys)
				s.Get(key) // This checks expiration
			}
		})
	}
}

// BenchmarkStorageConcurrentReads benchmarks concurrent read operations
func BenchmarkStorageConcurrentReads(b *testing.B) {
	s := NewMemory()
	// Pre-populate
	for i := 0; i < 1000; i++ {
		s.Set(fmt.Sprintf("key_%d", i), []byte("value"), nil)
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key_%d", i%1000)
			s.Get(key)
			i++
		}
	})
}

// BenchmarkStorageConcurrentWrites benchmarks concurrent write operations
func BenchmarkStorageConcurrentWrites(b *testing.B) {
	s := NewMemory()

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key_%d", i%1000)
			s.Set(key, []byte("value"), nil)
			i++
		}
	})
}

// BenchmarkStorageConcurrentMixed benchmarks mixed read/write operations
func BenchmarkStorageConcurrentMixed(b *testing.B) {
	s := NewMemory()
	// Pre-populate
	for i := 0; i < 1000; i++ {
		s.Set(fmt.Sprintf("key_%d", i), []byte("value"), nil)
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key_%d", i%1000)
			if i%3 == 0 {
				// 33% writes
				s.Set(key, []byte("value"), nil)
			} else {
				// 67% reads
				s.Get(key)
			}
			i++
		}
	})
}

// BenchmarkStorageSharding benchmarks the impact of different shard counts
func BenchmarkStorageSharding(b *testing.B) {
	shardCounts := []int{1, 16, 64, 256}

	for _, shards := range shardCounts {
		b.Run(fmt.Sprintf("Shards_%d", shards), func(b *testing.B) {
			s := NewMemory(WithShardCount(shards))

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				key := fmt.Sprintf("key_%d", i%10000)
				s.Set(key, []byte("value"), nil)
			}
		})
	}
}
