package main

import (
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/raniellyferreira/redis-inmemory-replica/storage"
)

func main() {
	fmt.Println("=== Redis In-Memory Replica: Concurrency Analysis Demo ===")
	fmt.Println()

	// Create both storage implementations
	rwMutexStorage := storage.NewMemory()
	atomicStorage := storage.NewAtomicMemory()

	defer rwMutexStorage.Close()
	defer atomicStorage.Close()

	// Demonstrate basic functionality
	fmt.Println("1. Basic Functionality Comparison")
	demonstrateBasicOps(rwMutexStorage, atomicStorage)
	fmt.Println()

	// Performance comparison
	fmt.Println("2. Performance Comparison")
	comparePerformance(rwMutexStorage, atomicStorage)
	fmt.Println()

	// Concurrency demonstration
	fmt.Println("3. Concurrency Demonstration")
	demonstrateConcurrency(rwMutexStorage, atomicStorage)
	fmt.Println()

	// Memory usage comparison
	fmt.Println("4. Memory Usage Comparison")
	compareMemoryUsage(rwMutexStorage, atomicStorage)
	fmt.Println()

	fmt.Println("=== Demo Complete ===")
	fmt.Println("For comprehensive benchmarks, run: cd benchmarks && ./run_benchmarks.sh")
}

func demonstrateBasicOps(rwMutex, atomic storage.Storage) {
	fmt.Println("Testing basic operations...")
	
	// Test both implementations
	testStorage := func(name string, stor storage.Storage) {
		// Set a key
		stor.Set("demo:key", []byte("demo:value"), nil)
		
		// Get the key
		value, exists := stor.Get("demo:key")
		
		// Report results
		fmt.Printf("  %s: Set/Get test - exists: %v, value: %s\n", 
			name, exists, string(value))
		
		// Test key count
		fmt.Printf("  %s: Key count: %d\n", name, stor.KeyCount())
		
		// Clean up
		stor.Del("demo:key")
	}
	
	testStorage("sync.RWMutex", rwMutex)
	testStorage("atomic.Value", atomic)
}

func comparePerformance(rwMutex, atomic storage.Storage) {
	// Pre-populate both storages
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("perf:key:%d", i)
		value := fmt.Sprintf("value:%d", i)
		rwMutex.Set(key, []byte(value), nil)
		atomic.Set(key, []byte(value), nil)
	}
	
	// Test read performance
	fmt.Println("Read Performance (1000 operations):")
	
	readTest := func(name string, stor storage.Storage) {
		start := time.Now()
		for i := 0; i < 1000; i++ {
			key := fmt.Sprintf("perf:key:%d", i%1000)
			stor.Get(key)
		}
		duration := time.Since(start)
		opsPerSec := float64(1000) / duration.Seconds()
		fmt.Printf("  %s: %v (%d ns/op, %.0f ops/sec)\n", 
			name, duration, duration.Nanoseconds()/1000, opsPerSec)
	}
	
	readTest("sync.RWMutex", rwMutex)
	readTest("atomic.Value", atomic)
	
	// Test write performance
	fmt.Println("Write Performance (100 operations):")
	
	writeTest := func(name string, stor storage.Storage) {
		start := time.Now()
		for i := 0; i < 100; i++ {
			key := fmt.Sprintf("write:key:%d", i)
			value := fmt.Sprintf("write:value:%d", i)
			stor.Set(key, []byte(value), nil)
		}
		duration := time.Since(start)
		opsPerSec := float64(100) / duration.Seconds()
		fmt.Printf("  %s: %v (%d ns/op, %.0f ops/sec)\n", 
			name, duration, duration.Nanoseconds()/100, opsPerSec)
	}
	
	writeTest("sync.RWMutex", rwMutex)
	writeTest("atomic.Value", atomic)
}

func demonstrateConcurrency(rwMutex, atomic storage.Storage) {
	numGoroutines := runtime.NumCPU()
	numOpsPerGoroutine := 500
	
	fmt.Printf("Concurrent test with %d goroutines, %d ops each (read-heavy: 90%% reads, 10%% writes):\n", 
		numGoroutines, numOpsPerGoroutine)
	
	concurrencyTest := func(name string, stor storage.Storage) {
		start := time.Now()
		var wg sync.WaitGroup
		
		for g := 0; g < numGoroutines; g++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()
				
				for i := 0; i < numOpsPerGoroutine; i++ {
					if i%10 == 0 { // 10% writes
						key := fmt.Sprintf("conc:%d:%d", goroutineID, i)
						value := fmt.Sprintf("value:%d:%d", goroutineID, i)
						stor.Set(key, []byte(value), nil)
					} else { // 90% reads
						key := fmt.Sprintf("perf:key:%d", i%1000)
						stor.Get(key)
					}
				}
			}(g)
		}
		
		wg.Wait()
		duration := time.Since(start)
		totalOps := numGoroutines * numOpsPerGoroutine
		opsPerSec := float64(totalOps) / duration.Seconds()
		
		fmt.Printf("  %s: %v (%.0f ops/sec)\n", name, duration, opsPerSec)
	}
	
	concurrencyTest("sync.RWMutex", rwMutex)
	concurrencyTest("atomic.Value", atomic)
}

func compareMemoryUsage(rwMutex, atomic storage.Storage) {
	// Add some data to both storages
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("mem:key:%d", i)
		value := fmt.Sprintf("memory:test:value:with:some:length:%d", i)
		rwMutex.Set(key, []byte(value), nil)
		atomic.Set(key, []byte(value), nil)
	}
	
	// Get memory usage estimates
	rwMutexUsage := rwMutex.MemoryUsage()
	atomicUsage := atomic.MemoryUsage()
	
	fmt.Printf("Memory Usage Estimates (1000 keys):\n")
	fmt.Printf("  sync.RWMutex: %d bytes (%.2f KB)\n", rwMutexUsage, float64(rwMutexUsage)/1024)
	fmt.Printf("  atomic.Value: %d bytes (%.2f KB)\n", atomicUsage, float64(atomicUsage)/1024)
	
	if atomicUsage > rwMutexUsage {
		ratio := float64(atomicUsage) / float64(rwMutexUsage)
		fmt.Printf("  atomic.Value uses %.2fx more memory\n", ratio)
	} else {
		ratio := float64(rwMutexUsage) / float64(atomicUsage)
		fmt.Printf("  sync.RWMutex uses %.2fx more memory\n", ratio)
	}
	
	// Show additional info from atomic storage
	fmt.Printf("  atomic.Value implementation provides additional metrics\n")
}