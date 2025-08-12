package main

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	redisreplica "github.com/raniellyferreira/redis-inmemory-replica"
)

// DatabaseStats tracks statistics for database appearances in INFO output
type DatabaseStats struct {
	Appearances map[int]int // dbNum -> count of appearances
	TotalCalls  int
	mutex       sync.Mutex
}

func (ds *DatabaseStats) RecordAppearance(dbNums []int) {
	ds.mutex.Lock()
	defer ds.mutex.Unlock()
	
	ds.TotalCalls++
	for _, dbNum := range dbNums {
		ds.Appearances[dbNum]++
	}
}

func (ds *DatabaseStats) PrintStats() {
	ds.mutex.Lock()
	defer ds.mutex.Unlock()
	
	fmt.Printf("\n=== Database Appearance Statistics (out of %d calls) ===\n", ds.TotalCalls)
	
	// Sort by database number
	var dbNums []int
	for dbNum := range ds.Appearances {
		dbNums = append(dbNums, dbNum)
	}
	sort.Ints(dbNums)
	
	for _, dbNum := range dbNums {
		count := ds.Appearances[dbNum]
		percentage := float64(count) / float64(ds.TotalCalls) * 100
		fmt.Printf("db%d: %d/%d appearances (%.1f%%)\n", dbNum, count, ds.TotalCalls, percentage)
	}
	
	if len(dbNums) > 0 {
		minAppearances := ds.Appearances[dbNums[0]]
		maxAppearances := ds.Appearances[dbNums[0]]
		for _, count := range ds.Appearances {
			if count < minAppearances {
				minAppearances = count
			}
			if count > maxAppearances {
				maxAppearances = count
			}
		}
		
		variance := maxAppearances - minAppearances
		fmt.Printf("\nVariance: %d (min: %d, max: %d)\n", variance, minAppearances, maxAppearances)
		
		if variance > 0 {
			fmt.Printf("⚠️  INCONSISTENCY DETECTED: Database appearances vary by %d\n", variance)
		} else {
			fmt.Printf("✅ CONSISTENT: All databases appear equally\n")
		}
	}
}

// parseKeyspaceInfo extracts database numbers from INFO keyspace output
func parseKeyspaceInfo(infoOutput string) []int {
	var dbNums []int
	lines := strings.Split(infoOutput, "\n")
	
	inKeyspaceSection := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "# Keyspace" {
			inKeyspaceSection = true
			continue
		}
		if inKeyspaceSection && strings.HasPrefix(line, "#") {
			break // End of keyspace section
		}
		if inKeyspaceSection && strings.HasPrefix(line, "db") {
			var dbNum int
			if n, err := fmt.Sscanf(line, "db%d:", &dbNum); n == 1 && err == nil {
				dbNums = append(dbNums, dbNum)
			}
		}
	}
	
	sort.Ints(dbNums) // Ensure consistent ordering for comparison
	return dbNums
}

func main() {
	fmt.Println("🔍 Redis INFO Keyspace Reproduction Script")
	fmt.Println("This script reproduces the inconsistent keyspace database listing issue")
	fmt.Println()

	// Create replica instance
	replica, err := redisreplica.New(
		redisreplica.WithReplicaAddr(":6382"), // Use different port
	)
	if err != nil {
		log.Fatalf("Failed to create replica: %v", err)
	}
	defer replica.Close()

	// Start server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := replica.Start(ctx); err != nil {
		log.Fatalf("Failed to start replica: %v", err)
	}

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	storage := replica.Storage()

	// Populate databases 0-3 with different key counts
	fmt.Println("📝 Populating databases...")
	
	// DB 0: 5 keys (2 with TTL)
	if err := storage.SelectDB(0); err != nil {
		log.Fatalf("Failed to select DB 0: %v", err)
	}
	for i := 0; i < 3; i++ {
		key := fmt.Sprintf("db0_key_%d", i)
		if err := storage.Set(key, []byte(fmt.Sprintf("value_%d", i)), nil); err != nil {
			log.Fatalf("Failed to set %s: %v", key, err)
		}
	}
	// Add keys with TTL
	future := time.Now().Add(1 * time.Hour)
	for i := 3; i < 5; i++ {
		key := fmt.Sprintf("db0_ttl_key_%d", i)
		if err := storage.Set(key, []byte(fmt.Sprintf("ttl_value_%d", i)), &future); err != nil {
			log.Fatalf("Failed to set %s with TTL: %v", key, err)
		}
	}

	// DB 1: 3 keys (1 with TTL)
	if err := storage.SelectDB(1); err != nil {
		log.Fatalf("Failed to select DB 1: %v", err)
	}
	for i := 0; i < 2; i++ {
		key := fmt.Sprintf("db1_key_%d", i)
		if err := storage.Set(key, []byte(fmt.Sprintf("value_%d", i)), nil); err != nil {
			log.Fatalf("Failed to set %s: %v", key, err)
		}
	}
	// Add key with TTL
	key := "db1_ttl_key"
	if err := storage.Set(key, []byte("ttl_value"), &future); err != nil {
		log.Fatalf("Failed to set %s with TTL: %v", key, err)
	}

	// DB 2: 7 keys (no TTL)
	if err := storage.SelectDB(2); err != nil {
		log.Fatalf("Failed to select DB 2: %v", err)
	}
	for i := 0; i < 7; i++ {
		key := fmt.Sprintf("db2_key_%d", i)
		if err := storage.Set(key, []byte(fmt.Sprintf("value_%d", i)), nil); err != nil {
			log.Fatalf("Failed to set %s: %v", key, err)
		}
	}

	// DB 3: 4 keys (3 with TTL)
	if err := storage.SelectDB(3); err != nil {
		log.Fatalf("Failed to select DB 3: %v", err)
	}
	for i := 0; i < 1; i++ {
		key := fmt.Sprintf("db3_key_%d", i)
		if err := storage.Set(key, []byte(fmt.Sprintf("value_%d", i)), nil); err != nil {
			log.Fatalf("Failed to set %s: %v", key, err)
		}
	}
	// Add keys with TTL
	for i := 1; i < 4; i++ {
		key := fmt.Sprintf("db3_ttl_key_%d", i)
		if err := storage.Set(key, []byte(fmt.Sprintf("ttl_value_%d", i)), &future); err != nil {
			log.Fatalf("Failed to set %s with TTL: %v", key, err)
		}
	}

	// Switch back to DB 0
	if err := storage.SelectDB(0); err != nil {
		log.Fatalf("Failed to select DB 0: %v", err)
	}

	fmt.Println("Expected databases in INFO keyspace:")
	fmt.Println("  db0: 5 keys, 2 expires")
	fmt.Println("  db1: 3 keys, 1 expires")
	fmt.Println("  db2: 7 keys, 0 expires")
	fmt.Println("  db3: 4 keys, 3 expires")
	fmt.Println()

	// Test concurrent INFO calls to reproduce the issue
	fmt.Println("🚀 Running concurrent INFO keyspace calls...")
	
	stats := &DatabaseStats{
		Appearances: make(map[int]int),
	}
	
	const numCalls = 200
	const numWorkers = 10
	
	var wg sync.WaitGroup
	callsChan := make(chan int, numCalls)
	
	// Queue up calls
	for i := 0; i < numCalls; i++ {
		callsChan <- i
	}
	close(callsChan)
	
	// Start workers
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			for callNum := range callsChan {
				// Get database info via the same interface used by INFO command
				dbInfo := storage.DatabaseInfo()
				
				// Extract database numbers from the result
				var dbNums []int
				for dbNum := range dbInfo {
					dbNums = append(dbNums, dbNum)
				}
				
				stats.RecordAppearance(dbNums)
				
				// Occasional logging to show progress
				if callNum%50 == 0 {
					fmt.Printf("Worker %d: Call %d - Found DBs: %v\n", workerID, callNum, dbNums)
				}
				
				// Small delay to increase chance of race conditions
				time.Sleep(time.Microsecond * 10)
			}
		}(w)
	}
	
	wg.Wait()
	stats.PrintStats()
	
	fmt.Println("\n🔚 Reproduction script completed")
	fmt.Println("If you see variance in database appearances, the bug is reproduced!")
}