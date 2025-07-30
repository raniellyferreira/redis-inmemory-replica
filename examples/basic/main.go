// Package main demonstrates basic usage of the redis-inmemory-replica library
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/raniellyferreira/redis-inmemory-replica"
)

func main() {
	// Create a new replica
	replica, err := redisreplica.New(
		redisreplica.WithMaster("localhost:6379"),
		redisreplica.WithReplicaAddr(":6380"),
		redisreplica.WithSyncTimeout(30*time.Second),
	)
	if err != nil {
		log.Fatal("Failed to create replica:", err)
	}
	defer replica.Close()

	// Register callback for sync completion
	replica.OnSyncComplete(func() {
		log.Println("✅ Initial synchronization completed!")
		log.Println("Replica is ready to serve read requests")
	})

	// Start replication
	log.Println("🚀 Starting replica...")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := replica.Start(ctx); err != nil {
		log.Fatal("Failed to start replica:", err)
	}

	log.Println("📡 Replica started, waiting for initial sync...")
	
	// Wait for initial synchronization
	if err := replica.WaitForSync(ctx); err != nil {
		log.Fatal("Failed to sync:", err)
	}

	// Print status information
	printStatus(replica)

	// Demonstrate direct storage access
	demonstrateStorageAccess(replica)

	// Keep running to demonstrate real-time sync
	log.Println("🔄 Replica is running. Make changes in Redis master to see real-time sync...")
	log.Println("Press Ctrl+C to stop")
	
	// Monitor status periodically
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			printStatus(replica)
		case <-ctx.Done():
			log.Println("Context cancelled, shutting down...")
			return
		}
	}
}

func printStatus(replica *redisreplica.Replica) {
	status := replica.SyncStatus()
	
	fmt.Printf("\n📊 Replica Status:\n")
	fmt.Printf("   Connected: %v\n", status.Connected)
	fmt.Printf("   Master: %s\n", status.MasterHost)
	fmt.Printf("   Initial Sync: %v (%.1f%%)\n", 
		status.InitialSyncCompleted, 
		status.InitialSyncProgress*100)
	fmt.Printf("   Replication Offset: %d\n", status.ReplicationOffset)
	fmt.Printf("   Commands Processed: %d\n", status.CommandsProcessed)
	fmt.Printf("   Last Sync: %v\n", status.LastSyncTime.Format(time.RFC3339))

	// Print storage info
	info := replica.GetInfo()
	if storageInfo, ok := info["keys"]; ok {
		fmt.Printf("   Keys in Storage: %v\n", storageInfo)
	}
	if memUsage, ok := info["memory_usage"]; ok {
		fmt.Printf("   Memory Usage: %v bytes\n", memUsage)
	}
}

func demonstrateStorageAccess(replica *redisreplica.Replica) {
	storage := replica.Storage()
	
	fmt.Printf("\n🔍 Demonstrating storage access:\n")
	
	// Try to get some common keys
	testKeys := []string{"test:key", "counter", "user:1", "config:app"}
	
	for _, key := range testKeys {
		if value, exists := storage.Get(key); exists {
			fmt.Printf("   %s = %s\n", key, string(value))
		}
	}
	
	// Show all keys
	allKeys := storage.Keys()
	if len(allKeys) > 0 {
		fmt.Printf("   All keys (%d): %v\n", len(allKeys), allKeys)
	} else {
		fmt.Printf("   No keys found in replica\n")
		fmt.Printf("   💡 Try setting some keys in your Redis master:\n")
		fmt.Printf("       redis-cli SET test:key \"Hello from replica!\"\n")
		fmt.Printf("       redis-cli SET counter 42\n")
	}
}