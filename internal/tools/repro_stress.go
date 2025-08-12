package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	redisreplica "github.com/raniellyferreira/redis-inmemory-replica"
	"github.com/raniellyferreira/redis-inmemory-replica/protocol"
)

// createCommand creates a protocol command array
func createCommand(cmd ...string) []protocol.Value {
	values := make([]protocol.Value, len(cmd))
	for i, arg := range cmd {
		values[i] = protocol.Value{
			Type: protocol.TypeBulkString,
			Data: []byte(arg),
		}
	}
	return values
}

// parseKeyspaceInfo extracts detailed keyspace info from INFO output
func parseKeyspaceInfo(infoOutput string) (map[int]map[string]int, []int) {
	dbInfo := make(map[int]map[string]int)
	var dbNums []int
	lines := strings.Split(infoOutput, "\r\n")
	
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
			var rest string
			if n, err := fmt.Sscanf(line, "db%d:%s", &dbNum, &rest); n == 2 && err == nil {
				dbNums = append(dbNums, dbNum)
				
				// Parse the stats (keys=X,expires=Y)
				stats := make(map[string]int)
				parts := strings.Split(rest, ",")
				for _, part := range parts {
					keyVal := strings.Split(part, "=")
					if len(keyVal) == 2 {
						if val, err := strconv.Atoi(keyVal[1]); err == nil {
							stats[keyVal[0]] = val
						}
					}
				}
				dbInfo[dbNum] = stats
			}
		}
	}
	
	sort.Ints(dbNums)
	return dbInfo, dbNums
}

// callInfoKeyspace makes a network call to INFO keyspace command
func callInfoKeyspace(addr string) (map[int]map[string]int, []int, string, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to connect: %v", err)
	}
	defer conn.Close()

	reader := protocol.NewReader(conn)
	writer := protocol.NewWriter(conn)

	// Send INFO keyspace command
	err = writer.WriteArray(createCommand("INFO", "keyspace"))
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to write command: %v", err)
	}
	err = writer.Flush()
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to flush: %v", err)
	}

	// Read response
	response, err := reader.ReadNext()
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to read response: %v", err)
	}

	infoOutput := string(response.Data)
	dbInfo, dbNums := parseKeyspaceInfo(infoOutput)
	
	return dbInfo, dbNums, infoOutput, nil
}

func main() {
	fmt.Println("🔥 Redis INFO Keyspace Stress Test")
	fmt.Println("This script stress tests with concurrent writes while reading INFO")
	fmt.Println()

	// Create replica instance
	replica, err := redisreplica.New(
		redisreplica.WithReplicaAddr(":6384"), // Use different port
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
	time.Sleep(200 * time.Millisecond)

	storage := replica.Storage()
	const serverAddr = "localhost:6384"

	fmt.Println("📝 Initial database population...")
	
	// Populate 4 databases with some initial data
	for db := 0; db < 4; db++ {
		if err := storage.SelectDB(db); err != nil {
			log.Fatalf("Failed to select DB %d: %v", db, err)
		}
		
		// Add some initial keys
		for i := 0; i < 5; i++ {
			key := fmt.Sprintf("db%d_initial_key_%d", db, i)
			if err := storage.Set(key, []byte(fmt.Sprintf("value_%d", i)), nil); err != nil {
				log.Fatalf("Failed to set %s: %v", key, err)
			}
		}
	}

	// Switch back to DB 0
	if err := storage.SelectDB(0); err != nil {
		log.Fatalf("Failed to select DB 0: %v", err)
	}

	fmt.Println("🚀 Starting concurrent stress test...")
	fmt.Println("This will run concurrent INFO calls while continuously modifying databases")
	
	var wg sync.WaitGroup
	
	// Statistics collection
	var mutex sync.Mutex
	infoResults := make([][]int, 0, 1000)
	unexpectedResults := make([]string, 0)
	
	// Start INFO reader workers
	for w := 0; w < 5; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			for i := 0; i < 200; i++ {
				dbInfo, dbNums, rawOutput, err := callInfoKeyspace(serverAddr)
				if err != nil {
					log.Printf("Worker %d: INFO call failed: %v", workerID, err)
					continue
				}
				
				mutex.Lock()
				infoResults = append(infoResults, dbNums)
				
				// Check for unexpected results
				if len(dbNums) == 0 {
					unexpectedResults = append(unexpectedResults, fmt.Sprintf("Empty result: %s", rawOutput))
				} else if len(dbNums) < 4 {
					unexpectedResults = append(unexpectedResults, fmt.Sprintf("Missing DBs - found %v, output: %s", dbNums, rawOutput))
				}
				
				// Check individual database stats
				for dbNum, stats := range dbInfo {
					if keys, exists := stats["keys"]; exists && keys == 0 {
						unexpectedResults = append(unexpectedResults, fmt.Sprintf("DB%d reported 0 keys: %s", dbNum, rawOutput))
					}
				}
				mutex.Unlock()
				
				// Short sleep between calls
				time.Sleep(time.Millisecond * 2)
			}
		}(w)
	}
	
	// Start database modification workers
	for w := 0; w < 3; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			
			for i := 0; i < 100; i++ {
				// Randomly modify databases
				dbNum := i % 4
				if err := storage.SelectDB(dbNum); err != nil {
					log.Printf("Writer worker %d: Failed to select DB %d: %v", workerID, dbNum, err)
					continue
				}
				
				// Add keys
				for j := 0; j < 3; j++ {
					key := fmt.Sprintf("db%d_dynamic_worker%d_batch%d_key%d", dbNum, workerID, i, j)
					if err := storage.Set(key, []byte(fmt.Sprintf("value_%d_%d", i, j)), nil); err != nil {
						log.Printf("Writer worker %d: Failed to set %s: %v", workerID, key, err)
					}
				}
				
				// Delete some keys occasionally
				if i%10 == 0 {
					key := fmt.Sprintf("db%d_initial_key_0", dbNum)
					storage.Del(key)
				}
				
				// Switch between databases rapidly
				time.Sleep(time.Millisecond * 5)
			}
		}(w)
	}
	
	// Start database switching worker
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			dbNum := i % 16 // Switch between all possible DBs
			storage.SelectDB(dbNum)
			time.Sleep(time.Millisecond * 1)
		}
	}()
	
	// Wait for all workers to complete
	wg.Wait()
	
	// Analyze results
	fmt.Println("\n📊 Analysis of Results:")
	
	mutex.Lock()
	defer mutex.Unlock()
	
	dbCounts := make(map[int]int)
	totalCalls := len(infoResults)
	
	for _, dbNums := range infoResults {
		for _, dbNum := range dbNums {
			dbCounts[dbNum]++
		}
	}
	
	fmt.Printf("Total INFO calls: %d\n", totalCalls)
	
	// Check for any databases
	var allDBs []int
	for dbNum := range dbCounts {
		allDBs = append(allDBs, dbNum)
	}
	sort.Ints(allDBs)
	
	fmt.Printf("Databases seen: %v\n", allDBs)
	
	for _, dbNum := range allDBs {
		count := dbCounts[dbNum]
		percentage := float64(count) / float64(totalCalls) * 100
		fmt.Printf("  db%d: %d/%d appearances (%.1f%%)\n", dbNum, count, totalCalls, percentage)
	}
	
	// Report unexpected results
	fmt.Printf("\nUnexpected results found: %d\n", len(unexpectedResults))
	if len(unexpectedResults) > 0 {
		fmt.Println("First few unexpected results:")
		for i, result := range unexpectedResults {
			if i >= 5 {
				break
			}
			fmt.Printf("  %d: %s\n", i+1, result)
		}
		
		if len(unexpectedResults) > 5 {
			fmt.Printf("  ... and %d more\n", len(unexpectedResults)-5)
		}
	}
	
	// Check variance
	if len(allDBs) > 1 {
		minCount := dbCounts[allDBs[0]]
		maxCount := dbCounts[allDBs[0]]
		for _, count := range dbCounts {
			if count < minCount {
				minCount = count
			}
			if count > maxCount {
				maxCount = count
			}
		}
		
		variance := maxCount - minCount
		if variance > 0 {
			fmt.Printf("\n⚠️  INCONSISTENCY DETECTED: Database appearances vary by %d (min: %d, max: %d)\n", variance, minCount, maxCount)
		} else {
			fmt.Printf("\n✅ CONSISTENT: All databases appear equally\n")
		}
	}
	
	fmt.Println("\n🔚 Stress test completed")
}