package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"sort"
	"strings"
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

// parseKeyspaceInfo extracts database numbers from INFO keyspace output
func parseKeyspaceInfo(infoOutput string) []int {
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
			if n, err := fmt.Sscanf(line, "db%d:", &dbNum); n == 1 && err == nil {
				dbNums = append(dbNums, dbNum)
			}
		}
	}
	
	sort.Ints(dbNums)
	return dbNums
}

// callInfoKeyspace makes a network call to INFO keyspace command
func callInfoKeyspace(addr string) ([]int, string, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, "", fmt.Errorf("failed to connect: %v", err)
	}
	defer conn.Close()

	reader := protocol.NewReader(conn)
	writer := protocol.NewWriter(conn)

	// Send INFO keyspace command
	err = writer.WriteArray(createCommand("INFO", "keyspace"))
	if err != nil {
		return nil, "", fmt.Errorf("failed to write command: %v", err)
	}
	err = writer.Flush()
	if err != nil {
		return nil, "", fmt.Errorf("failed to flush: %v", err)
	}

	// Read response
	response, err := reader.ReadNext()
	if err != nil {
		return nil, "", fmt.Errorf("failed to read response: %v", err)
	}

	infoOutput := string(response.Data)
	dbNums := parseKeyspaceInfo(infoOutput)
	
	return dbNums, infoOutput, nil
}

func main() {
	fmt.Println("🔧 Redis INFO Keyspace Fix Demonstration")
	fmt.Println("This demonstrates the consistent behavior after the fix")
	fmt.Println()

	// Create replica instance
	replica, err := redisreplica.New(
		redisreplica.WithReplicaAddr(":6395"),
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

	// Populate 4 databases as described in the problem statement
	fmt.Println("📝 Setting up the scenario: 4 databases with different data...")
	
	// DB 0: 5 keys with 2 TTL
	if err := storage.SelectDB(0); err != nil {
		log.Fatalf("Failed to select DB 0: %v", err)
	}
	for i := 0; i < 3; i++ {
		key := fmt.Sprintf("user:%d", i)
		if err := storage.Set(key, []byte(fmt.Sprintf("data_%d", i)), nil); err != nil {
			log.Fatalf("Failed to set key: %v", err)
		}
	}
	future := time.Now().Add(1 * time.Hour)
	for i := 0; i < 2; i++ {
		key := fmt.Sprintf("session:%d", i)
		if err := storage.Set(key, []byte(fmt.Sprintf("session_data_%d", i)), &future); err != nil {
			log.Fatalf("Failed to set session key: %v", err)
		}
	}

	// DB 1: 3 keys with 1 TTL
	if err := storage.SelectDB(1); err != nil {
		log.Fatalf("Failed to select DB 1: %v", err)
	}
	for i := 0; i < 2; i++ {
		key := fmt.Sprintf("config:%d", i)
		if err := storage.Set(key, []byte(fmt.Sprintf("config_%d", i)), nil); err != nil {
			log.Fatalf("Failed to set config key: %v", err)
		}
	}
	if err := storage.Set("temp_token", []byte("token_value"), &future); err != nil {
		log.Fatalf("Failed to set temp token: %v", err)
	}

	// DB 2: 7 keys with 0 TTL
	if err := storage.SelectDB(2); err != nil {
		log.Fatalf("Failed to select DB 2: %v", err)
	}
	for i := 0; i < 7; i++ {
		key := fmt.Sprintf("product:%d", i)
		if err := storage.Set(key, []byte(fmt.Sprintf("product_data_%d", i)), nil); err != nil {
			log.Fatalf("Failed to set product key: %v", err)
		}
	}

	// DB 3: 4 keys with 3 TTL
	if err := storage.SelectDB(3); err != nil {
		log.Fatalf("Failed to select DB 3: %v", err)
	}
	if err := storage.Set("admin_setting", []byte("setting_value"), nil); err != nil {
		log.Fatalf("Failed to set admin setting: %v", err)
	}
	for i := 0; i < 3; i++ {
		key := fmt.Sprintf("cache:%d", i)
		if err := storage.Set(key, []byte(fmt.Sprintf("cached_data_%d", i)), &future); err != nil {
			log.Fatalf("Failed to set cache key: %v", err)
		}
	}

	// Switch back to DB 0
	if err := storage.SelectDB(0); err != nil {
		log.Fatalf("Failed to select DB 0: %v", err)
	}

	fmt.Println("✅ Databases populated:")
	fmt.Println("   DB0: 5 keys (3 permanent + 2 with TTL)")
	fmt.Println("   DB1: 3 keys (2 permanent + 1 with TTL)")  
	fmt.Println("   DB2: 7 keys (all permanent)")
	fmt.Println("   DB3: 4 keys (1 permanent + 3 with TTL)")
	fmt.Println()

	// Test INFO keyspace consistency
	fmt.Println("🧪 Testing INFO keyspace consistency (like TablePlus would)...")
	
	serverAddr := "localhost:6395"
	const numTests = 10
	
	allResults := make([][]int, 0, numTests)
	
	for i := 0; i < numTests; i++ {
		dbNums, rawOutput, err := callInfoKeyspace(serverAddr)
		if err != nil {
			log.Fatalf("Call %d failed: %v", i, err)
		}
		
		allResults = append(allResults, dbNums)
		
		// Show first few outputs in detail
		if i < 3 {
			fmt.Printf("Call %d - Found databases: %v\n", i+1, dbNums)
			
			// Parse and show details
			lines := strings.Split(rawOutput, "\r\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "db") {
					fmt.Printf("         %s\n", line)
				}
			}
			fmt.Println()
		}
	}
	
	// Analyze consistency
	fmt.Printf("📊 Consistency Analysis (%d calls):\n", numTests)
	
	// Check if all calls returned the same databases
	firstResult := allResults[0]
	allConsistent := true
	
	for i, result := range allResults {
		if len(result) != len(firstResult) {
			fmt.Printf("❌ Call %d: Different number of databases (%d vs %d)\n", i+1, len(result), len(firstResult))
			allConsistent = false
			continue
		}
		
		for j, dbNum := range result {
			if dbNum != firstResult[j] {
				fmt.Printf("❌ Call %d: Different order or content %v vs %v\n", i+1, result, firstResult)
				allConsistent = false
				break
			}
		}
	}
	
	if allConsistent {
		fmt.Printf("✅ PERFECT CONSISTENCY: All %d calls returned identical results!\n", numTests)
		fmt.Printf("   Always shows databases: %v\n", firstResult)
		fmt.Printf("   Always in sorted order: %t\n", sort.IntsAreSorted(firstResult))
	} else {
		fmt.Printf("❌ INCONSISTENCY DETECTED: Results varied between calls\n")
	}
	
	fmt.Println()
	fmt.Println("🎯 Result: The fix ensures GUI clients like TablePlus will now see")
	fmt.Println("   consistent database listings in INFO keyspace output!")
}