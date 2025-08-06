package benchmarks

import (
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/raniellyferreira/redis-inmemory-replica/storage"
)

// BenchmarkStorage provides comprehensive benchmarks comparing
// traditional sync.RWMutex approach vs atomic.Value approach

// Benchmark scenarios:
// 1. Read-only workloads (100% reads)
// 2. Read-heavy workloads (95% reads, 5% writes)
// 3. Balanced workloads (80% reads, 20% writes)
// 4. Write-heavy workloads (50% reads, 50% writes)
// 5. Concurrent scaling tests (1, 2, 4, 8, 16, 32 goroutines)

// BenchmarkMemoryStorageGet_RWMutex benchmarks read operations with RWMutex
func BenchmarkMemoryStorageGet_RWMutex(b *testing.B) {
	storage := storage.NewMemory()
	
	// Pre-populate with test data
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key%d", i)
		value := fmt.Sprintf("value%d", i)
		storage.Set(key, []byte(value), nil)
	}
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key%d", i%1000)
			storage.Get(key)
			i++
		}
	})
}

// BenchmarkAtomicStorageGet benchmarks read operations with atomic.Value
func BenchmarkAtomicStorageGet(b *testing.B) {
	storage := storage.NewAtomicMemory()
	
	// Pre-populate with test data
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key%d", i)
		value := fmt.Sprintf("value%d", i)
		storage.Set(key, []byte(value), nil)
	}
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key%d", i%1000)
			storage.Get(key)
			i++
		}
	})
}

// BenchmarkMemoryStorageSet_RWMutex benchmarks write operations with RWMutex
func BenchmarkMemoryStorageSet_RWMutex(b *testing.B) {
	storage := storage.NewMemory()
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key%d", i)
			value := fmt.Sprintf("value%d", i)
			storage.Set(key, []byte(value), nil)
			i++
		}
	})
}

// BenchmarkAtomicStorageSet benchmarks write operations with atomic.Value
func BenchmarkAtomicStorageSet(b *testing.B) {
	storage := storage.NewAtomicMemory()
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key%d", i)
			value := fmt.Sprintf("value%d", i)
			storage.Set(key, []byte(value), nil)
			i++
		}
	})
}

// BenchmarkMixed_ReadHeavy_RWMutex benchmarks read-heavy workload (95% reads, 5% writes)
func BenchmarkMixed_ReadHeavy_RWMutex(b *testing.B) {
	storage := storage.NewMemory()
	
	// Pre-populate
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key%d", i)
		value := fmt.Sprintf("value%d", i)
		storage.Set(key, []byte(value), nil)
	}
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%20 == 0 { // 5% writes
				key := fmt.Sprintf("newkey%d", i)
				value := fmt.Sprintf("newvalue%d", i)
				storage.Set(key, []byte(value), nil)
			} else { // 95% reads
				key := fmt.Sprintf("key%d", i%1000)
				storage.Get(key)
			}
			i++
		}
	})
}

// BenchmarkMixed_ReadHeavy_Atomic benchmarks read-heavy workload with atomic.Value
func BenchmarkMixed_ReadHeavy_Atomic(b *testing.B) {
	storage := storage.NewAtomicMemory()
	
	// Pre-populate
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key%d", i)
		value := fmt.Sprintf("value%d", i)
		storage.Set(key, []byte(value), nil)
	}
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%20 == 0 { // 5% writes
				key := fmt.Sprintf("newkey%d", i)
				value := fmt.Sprintf("newvalue%d", i)
				storage.Set(key, []byte(value), nil)
			} else { // 95% reads
				key := fmt.Sprintf("key%d", i%1000)
				storage.Get(key)
			}
			i++
		}
	})
}

// BenchmarkMixed_Balanced_RWMutex benchmarks balanced workload (80% reads, 20% writes)
func BenchmarkMixed_Balanced_RWMutex(b *testing.B) {
	storage := storage.NewMemory()
	
	// Pre-populate
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key%d", i)
		value := fmt.Sprintf("value%d", i)
		storage.Set(key, []byte(value), nil)
	}
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%5 == 0 { // 20% writes
				key := fmt.Sprintf("newkey%d", i)
				value := fmt.Sprintf("newvalue%d", i)
				storage.Set(key, []byte(value), nil)
			} else { // 80% reads
				key := fmt.Sprintf("key%d", i%1000)
				storage.Get(key)
			}
			i++
		}
	})
}

// BenchmarkMixed_Balanced_Atomic benchmarks balanced workload with atomic.Value
func BenchmarkMixed_Balanced_Atomic(b *testing.B) {
	storage := storage.NewAtomicMemory()
	
	// Pre-populate
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key%d", i)
		value := fmt.Sprintf("value%d", i)
		storage.Set(key, []byte(value), nil)
	}
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%5 == 0 { // 20% writes
				key := fmt.Sprintf("newkey%d", i)
				value := fmt.Sprintf("newvalue%d", i)
				storage.Set(key, []byte(value), nil)
			} else { // 80% reads
				key := fmt.Sprintf("key%d", i%1000)
				storage.Get(key)
			}
			i++
		}
	})
}

// BenchmarkMixed_WriteHeavy_RWMutex benchmarks write-heavy workload (50% reads, 50% writes)
func BenchmarkMixed_WriteHeavy_RWMutex(b *testing.B) {
	storage := storage.NewMemory()
	
	// Pre-populate
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key%d", i)
		value := fmt.Sprintf("value%d", i)
		storage.Set(key, []byte(value), nil)
	}
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%2 == 0 { // 50% writes
				key := fmt.Sprintf("newkey%d", i)
				value := fmt.Sprintf("newvalue%d", i)
				storage.Set(key, []byte(value), nil)
			} else { // 50% reads
				key := fmt.Sprintf("key%d", i%1000)
				storage.Get(key)
			}
			i++
		}
	})
}

// BenchmarkMixed_WriteHeavy_Atomic benchmarks write-heavy workload with atomic.Value
func BenchmarkMixed_WriteHeavy_Atomic(b *testing.B) {
	storage := storage.NewAtomicMemory()
	
	// Pre-populate
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key%d", i)
		value := fmt.Sprintf("value%d", i)
		storage.Set(key, []byte(value), nil)
	}
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%2 == 0 { // 50% writes
				key := fmt.Sprintf("newkey%d", i)
				value := fmt.Sprintf("newvalue%d", i)
				storage.Set(key, []byte(value), nil)
			} else { // 50% reads
				key := fmt.Sprintf("key%d", i%1000)
				storage.Get(key)
			}
			i++
		}
	})
}

// BenchmarkScaling tests how each approach scales with different numbers of goroutines

// BenchmarkScaling_RWMutex_1 tests RWMutex with 1 goroutine
func BenchmarkScaling_RWMutex_1(b *testing.B) {
	benchmarkScaling(b, storage.NewMemory(), 1)
}

// BenchmarkScaling_RWMutex_2 tests RWMutex with 2 goroutines
func BenchmarkScaling_RWMutex_2(b *testing.B) {
	benchmarkScaling(b, storage.NewMemory(), 2)
}

// BenchmarkScaling_RWMutex_4 tests RWMutex with 4 goroutines
func BenchmarkScaling_RWMutex_4(b *testing.B) {
	benchmarkScaling(b, storage.NewMemory(), 4)
}

// BenchmarkScaling_RWMutex_8 tests RWMutex with 8 goroutines
func BenchmarkScaling_RWMutex_8(b *testing.B) {
	benchmarkScaling(b, storage.NewMemory(), 8)
}

// BenchmarkScaling_RWMutex_16 tests RWMutex with 16 goroutines
func BenchmarkScaling_RWMutex_16(b *testing.B) {
	benchmarkScaling(b, storage.NewMemory(), 16)
}

// BenchmarkScaling_Atomic_1 tests atomic.Value with 1 goroutine
func BenchmarkScaling_Atomic_1(b *testing.B) {
	benchmarkScaling(b, storage.NewAtomicMemory(), 1)
}

// BenchmarkScaling_Atomic_2 tests atomic.Value with 2 goroutines
func BenchmarkScaling_Atomic_2(b *testing.B) {
	benchmarkScaling(b, storage.NewAtomicMemory(), 2)
}

// BenchmarkScaling_Atomic_4 tests atomic.Value with 4 goroutines
func BenchmarkScaling_Atomic_4(b *testing.B) {
	benchmarkScaling(b, storage.NewAtomicMemory(), 4)
}

// BenchmarkScaling_Atomic_8 tests atomic.Value with 8 goroutines
func BenchmarkScaling_Atomic_8(b *testing.B) {
	benchmarkScaling(b, storage.NewAtomicMemory(), 8)
}

// BenchmarkScaling_Atomic_16 tests atomic.Value with 16 goroutines
func BenchmarkScaling_Atomic_16(b *testing.B) {
	benchmarkScaling(b, storage.NewAtomicMemory(), 16)
}

// benchmarkScaling runs a scaling benchmark with specified number of goroutines
func benchmarkScaling(b *testing.B, stor storage.Storage, numGoroutines int) {
	// Pre-populate
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key%d", i)
		value := fmt.Sprintf("value%d", i)
		stor.Set(key, []byte(value), nil)
	}
	
	b.ResetTimer()
	
	var wg sync.WaitGroup
	opsPerGoroutine := b.N / numGoroutines
	
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			
			for i := 0; i < opsPerGoroutine; i++ {
				if i%10 == 0 { // 10% writes
					key := fmt.Sprintf("newkey%d_%d", goroutineID, i)
					value := fmt.Sprintf("newvalue%d_%d", goroutineID, i)
					stor.Set(key, []byte(value), nil)
				} else { // 90% reads
					key := fmt.Sprintf("key%d", i%1000)
					stor.Get(key)
				}
			}
		}(g)
	}
	
	wg.Wait()
}

// BenchmarkMemoryUsage tests memory usage patterns

// BenchmarkMemoryUsage_RWMutex measures memory usage of RWMutex approach
func BenchmarkMemoryUsage_RWMutex(b *testing.B) {
	runtime.GC()
	var m1, m2 runtime.MemStats
	runtime.ReadMemStats(&m1)
	
	storage := storage.NewMemory()
	
	// Add data
	for i := 0; i < 10000; i++ {
		key := fmt.Sprintf("key%d", i)
		value := fmt.Sprintf("value%d_with_some_longer_content_to_measure_memory", i)
		storage.Set(key, []byte(value), nil)
	}
	
	runtime.GC()
	runtime.ReadMemStats(&m2)
	
	b.ReportMetric(float64(m2.Alloc-m1.Alloc)/10000, "bytes/key")
	b.ReportMetric(float64(storage.MemoryUsage()), "estimated_bytes")
}

// BenchmarkMemoryUsage_Atomic measures memory usage of atomic.Value approach
func BenchmarkMemoryUsage_Atomic(b *testing.B) {
	runtime.GC()
	var m1, m2 runtime.MemStats
	runtime.ReadMemStats(&m1)
	
	storage := storage.NewAtomicMemory()
	
	// Add data
	for i := 0; i < 10000; i++ {
		key := fmt.Sprintf("key%d", i)
		value := fmt.Sprintf("value%d_with_some_longer_content_to_measure_memory", i)
		storage.Set(key, []byte(value), nil)
	}
	
	runtime.GC()
	runtime.ReadMemStats(&m2)
	
	b.ReportMetric(float64(m2.Alloc-m1.Alloc)/10000, "bytes/key")
	b.ReportMetric(float64(storage.MemoryUsage()), "estimated_bytes")
}

// BenchmarkLargeDataset tests performance with larger datasets

// BenchmarkLargeDataset_RWMutex_10K tests RWMutex with 10K keys
func BenchmarkLargeDataset_RWMutex_10K(b *testing.B) {
	benchmarkLargeDataset(b, storage.NewMemory(), 10000)
}

// BenchmarkLargeDataset_RWMutex_100K tests RWMutex with 100K keys
func BenchmarkLargeDataset_RWMutex_100K(b *testing.B) {
	benchmarkLargeDataset(b, storage.NewMemory(), 100000)
}

// BenchmarkLargeDataset_Atomic_10K tests atomic.Value with 10K keys
func BenchmarkLargeDataset_Atomic_10K(b *testing.B) {
	benchmarkLargeDataset(b, storage.NewAtomicMemory(), 10000)
}

// BenchmarkLargeDataset_Atomic_100K tests atomic.Value with 100K keys
func BenchmarkLargeDataset_Atomic_100K(b *testing.B) {
	benchmarkLargeDataset(b, storage.NewAtomicMemory(), 100000)
}

// benchmarkLargeDataset runs benchmark with specified dataset size
func benchmarkLargeDataset(b *testing.B, stor storage.Storage, datasetSize int) {
	// Pre-populate with large dataset
	for i := 0; i < datasetSize; i++ {
		key := fmt.Sprintf("key%d", i)
		value := fmt.Sprintf("value%d", i)
		stor.Set(key, []byte(value), nil)
	}
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%20 == 0 { // 5% writes
				key := fmt.Sprintf("newkey%d", i)
				value := fmt.Sprintf("newvalue%d", i)
				stor.Set(key, []byte(value), nil)
			} else { // 95% reads
				key := fmt.Sprintf("key%d", i%datasetSize)
				stor.Get(key)
			}
			i++
		}
	})
}

// BenchmarkLatency measures operation latencies

// LatencyBenchmark provides latency measurement utilities
type LatencyBenchmark struct {
	latencies []time.Duration
	mutex     sync.Mutex
}

func (lb *LatencyBenchmark) Record(duration time.Duration) {
	lb.mutex.Lock()
	lb.latencies = append(lb.latencies, duration)
	lb.mutex.Unlock()
}

func (lb *LatencyBenchmark) Percentile(p float64) time.Duration {
	if len(lb.latencies) == 0 {
		return 0
	}
	
	// Simple percentile calculation (not fully accurate but sufficient for benchmarking)
	index := int(float64(len(lb.latencies)) * p / 100.0)
	if index >= len(lb.latencies) {
		index = len(lb.latencies) - 1
	}
	return lb.latencies[index]
}

// BenchmarkThroughput measures maximum throughput

// BenchmarkThroughput_RWMutex measures maximum throughput for RWMutex
func BenchmarkThroughput_RWMutex(b *testing.B) {
	storage := storage.NewMemory()
	
	// Pre-populate
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key%d", i)
		value := fmt.Sprintf("value%d", i)
		storage.Set(key, []byte(value), nil)
	}
	
	b.ResetTimer()
	
	// Measure operations per second
	start := time.Now()
	
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			key := fmt.Sprintf("key%d", rand.Intn(1000))
			storage.Get(key)
		}
	})
	
	duration := time.Since(start)
	opsPerSec := float64(b.N) / duration.Seconds()
	b.ReportMetric(opsPerSec, "ops/sec")
}

// BenchmarkThroughput_Atomic measures maximum throughput for atomic.Value
func BenchmarkThroughput_Atomic(b *testing.B) {
	storage := storage.NewAtomicMemory()
	
	// Pre-populate
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key%d", i)
		value := fmt.Sprintf("value%d", i)
		storage.Set(key, []byte(value), nil)
	}
	
	b.ResetTimer()
	
	// Measure operations per second
	start := time.Now()
	
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			key := fmt.Sprintf("key%d", rand.Intn(1000))
			storage.Get(key)
		}
	})
	
	duration := time.Since(start)
	opsPerSec := float64(b.N) / duration.Seconds()
	b.ReportMetric(opsPerSec, "ops/sec")
}