package redisreplica

import (
	"fmt"
	"testing"
	"time"

	"github.com/raniellyferreira/redis-inmemory-replica/storage"
)

// BenchmarkDatabaseInfoExpiredKeysCounting compares different approaches
// for counting expired keys in DatabaseInfo method
func BenchmarkDatabaseInfoExpiredKeysCounting(b *testing.B) {
	stor := storage.NewMemory()

	// Setup test data with mix of expired and non-expired keys
	now := time.Now()
	expiredTime := now.Add(-1 * time.Hour) // 1 hour ago
	futureTime := now.Add(1 * time.Hour)   // 1 hour from now

	// Add test keys: 1000 total, 300 expired, 700 valid
	for i := 0; i < 700; i++ {
		key := "key" + string(rune(i))
		if err := stor.Set(key, []byte("value"), &futureTime); err != nil {
			b.Fatalf("Failed to set key %s: %v", key, err)
		}
	}
	for i := 0; i < 300; i++ {
		key := "expired" + string(rune(i))
		if err := stor.Set(key, []byte("value"), &expiredTime); err != nil {
			b.Fatalf("Failed to set expired key %s: %v", key, err)
		}
	}

	b.ResetTimer()

	b.Run("OptimizedImplementation", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = stor.DatabaseInfo()
		}
	})
}

// BenchmarkDatabaseInfoScaling tests performance with different data sizes
func BenchmarkDatabaseInfoScaling(b *testing.B) {
	sizes := []int{100, 1000, 10000, 50000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("keys_%d", size), func(b *testing.B) {
			stor := storage.NewMemory()

			// Setup test data
			now := time.Now()
			expiredTime := now.Add(-1 * time.Hour)
			futureTime := now.Add(1 * time.Hour)

			// 70% valid keys, 30% expired keys
			validCount := int(float64(size) * 0.7)
			expiredCount := size - validCount

			for i := 0; i < validCount; i++ {
				key := fmt.Sprintf("key%d", i)
				if err := stor.Set(key, []byte("value"), &futureTime); err != nil {
					b.Fatalf("Failed to set key %s: %v", key, err)
				}
			}
			for i := 0; i < expiredCount; i++ {
				key := fmt.Sprintf("expired%d", i)
				if err := stor.Set(key, []byte("value"), &expiredTime); err != nil {
					b.Fatalf("Failed to set expired key %s: %v", key, err)
				}
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = stor.DatabaseInfo()
			}
		})
	}
}

// BenchmarkDatabaseInfoMultiDB tests performance with multiple databases
func BenchmarkDatabaseInfoMultiDB(b *testing.B) {
	stor := storage.NewMemory()

	// Setup data across multiple databases
	now := time.Now()
	futureTime := now.Add(1 * time.Hour)

	for db := 0; db < 16; db++ {
		if err := stor.SelectDB(db); err != nil {
			b.Fatalf("Failed to select database %d: %v", db, err)
		}
		for i := 0; i < 100; i++ {
			key := fmt.Sprintf("key%d", i)
			if err := stor.Set(key, []byte("value"), &futureTime); err != nil {
				b.Fatalf("Failed to set key %s in db %d: %v", key, db, err)
			}
		}
	}

	// Go back to database 0
	if err := stor.SelectDB(0); err != nil {
		b.Fatalf("Failed to select database 0: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = stor.DatabaseInfo()
	}
}
