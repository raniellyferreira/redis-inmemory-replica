// Package main demonstrates database filtering functionality
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/raniellyferreira/redis-inmemory-replica"
)

func main() {
	fmt.Println("🔍 Redis In-Memory Replica - Database Filtering Example")
	fmt.Println("========================================================")

	// Get configuration from environment variables with secure defaults
	masterAddr := getEnvOrDefault("REDIS_MASTER_ADDR", "localhost:6379")
	masterAuth := os.Getenv("REDIS_MASTER_AUTH") // Empty if not set
	replicaAddr := getEnvOrDefault("REDIS_REPLICA_ADDR", ":6380")
	replicaAuth := os.Getenv("REDIS_REPLICA_AUTH") // Empty if not set - use env var for security

	// Create a replica that only replicates specific databases
	replica, err := redisreplica.New(
		redisreplica.WithMaster(masterAddr),
		redisreplica.WithMasterAuth(masterAuth), // Uses env var or empty string
		redisreplica.WithReplicaAddr(replicaAddr),
		redisreplica.WithReplicaAuth(replicaAuth), // Uses env var or empty string  
		redisreplica.WithDatabases([]int{0, 1, 2}), // Only replicate databases 0, 1, and 2
		redisreplica.WithSyncTimeout(30*time.Second),
	)
	if err != nil {
		log.Fatal("Failed to create replica:", err)
	}
	defer replica.Close()

	fmt.Println("📊 Configuration:")
	fmt.Printf("  Master: %s\n", masterAddr)
	fmt.Printf("  Replica: %s\n", replicaAddr)
	fmt.Println("  Databases: 0, 1, 2 (filtering enabled)")
	if replicaAuth != "" {
		fmt.Println("  Replica Auth: enabled")
	} else {
		fmt.Println("  Replica Auth: disabled")
	}
	fmt.Println()

	// Register callback for sync completion
	replica.OnSyncComplete(func() {
		fmt.Println("✅ Initial synchronization completed!")
		fmt.Println("   Only data from databases 0, 1, and 2 have been replicated")
		fmt.Println("   Data from other databases (3-15) will be ignored")
		
		// Show replica statistics
		storage := replica.Storage()
		fmt.Printf("   📈 Keys replicated: %d\n", storage.KeyCount())
		fmt.Printf("   💾 Memory usage: %d bytes\n", storage.MemoryUsage())
	})

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	fmt.Println("🚀 Starting replication...")
	if err := replica.Start(ctx); err != nil {
		log.Fatal("Failed to start replica:", err)
	}

	fmt.Println("⏳ Waiting for initial synchronization...")
	if err := replica.WaitForSync(ctx); err != nil {
		log.Println("⚠️  Sync did not complete (this is expected if Redis is not running)")
		log.Println("   To test this example:")
		log.Println("   1. Start Redis server: redis-server")
		log.Println("   2. Set environment variables (optional):")
		log.Println("      export REDIS_MASTER_ADDR=localhost:6379")
		log.Println("      export REDIS_MASTER_AUTH=your_master_password")
		log.Println("      export REDIS_REPLICA_ADDR=:6380") 
		log.Println("      export REDIS_REPLICA_AUTH=your_replica_password")
		log.Println("   3. Add data to different databases:")
		log.Println("      redis-cli SELECT 0 && redis-cli SET key1 value1")
		log.Println("      redis-cli SELECT 1 && redis-cli SET key2 value2") 
		log.Println("      redis-cli SELECT 3 && redis-cli SET key3 value3  # This won't be replicated")
		log.Println("   4. Run this example again")
		return
	}

	// Demonstrate reading from replica
	storage := replica.Storage()
	
	fmt.Println("\n📖 Reading from replica:")
	
	// Try to read from different databases
	for db := 0; db < 4; db++ {
		storage.SelectDB(db)
		keys := storage.Keys()
		
		if len(keys) > 0 {
			fmt.Printf("   Database %d: %d keys found\n", db, len(keys))
			for _, key := range keys[:min(len(keys), 3)] { // Show first 3 keys
				if value, exists := storage.Get(key); exists {
					fmt.Printf("     %s = %s\n", key, string(value))
				}
			}
		} else {
			status := ""
			if db < 3 {
				status = " (replicated but empty)"
			} else {
				status = " (not replicated)"
			}
			fmt.Printf("   Database %d: No keys%s\n", db, status)
		}
	}

	fmt.Println("\n🌟 Database filtering features:")
	fmt.Println("   ✅ Reduced memory usage (only selected databases)")
	fmt.Println("   ✅ Faster sync times (less data to transfer)")
	fmt.Println("   ✅ Selective data isolation")
	fmt.Println("   ✅ Authentication support for replica access")

	// Keep the example running for a while to demonstrate real-time sync
	fmt.Println("\n⏱️  Keeping replica running for 30 seconds...")
	fmt.Println("   Try adding data to Redis and watch it replicate!")
	fmt.Println("   Only changes to databases 0, 1, 2 will appear in the replica")
	
	time.Sleep(30 * time.Second)
	
	fmt.Println("\n👋 Shutting down...")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// getEnvOrDefault returns the environment variable value or a default value if not set
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}