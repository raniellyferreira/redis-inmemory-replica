// Package main demonstrates running multiple replicas for different Redis masters
package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/raniellyferreira/redis-inmemory-replica"
)

// ReplicaManager manages multiple replicas
type ReplicaManager struct {
	replicas map[string]*redisreplica.Replica
	mu       sync.RWMutex
}

func NewReplicaManager() *ReplicaManager {
	return &ReplicaManager{
		replicas: make(map[string]*redisreplica.Replica),
	}
}

func (rm *ReplicaManager) AddReplica(name, masterAddr, replicaAddr string) error {
	replica, err := redisreplica.New(
		redisreplica.WithMaster(masterAddr),
		redisreplica.WithReplicaAddr(replicaAddr),
		redisreplica.WithSyncTimeout(30*time.Second),
	)
	if err != nil {
		return fmt.Errorf("failed to create replica %s: %w", name, err)
	}

	replica.OnSyncComplete(func() {
		log.Printf("✅ Replica %s synced with %s", name, masterAddr)
	})

	rm.mu.Lock()
	rm.replicas[name] = replica
	rm.mu.Unlock()

	return nil
}

func (rm *ReplicaManager) StartAll(ctx context.Context) error {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	var wg sync.WaitGroup
	errChan := make(chan error, len(rm.replicas))

	for name, replica := range rm.replicas {
		wg.Add(1)
		go func(name string, replica *redisreplica.Replica) {
			defer wg.Done()

			log.Printf("🚀 Starting replica %s...", name)
			if err := replica.Start(ctx); err != nil {
				errChan <- fmt.Errorf("failed to start replica %s: %w", name, err)
				return
			}

			log.Printf("⏳ Waiting for replica %s to sync...", name)
			if err := replica.WaitForSync(ctx); err != nil {
				errChan <- fmt.Errorf("failed to sync replica %s: %w", name, err)
				return
			}

			log.Printf("✅ Replica %s is ready", name)
		}(name, replica)
	}

	// Wait for all replicas to start
	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		return err
	}

	return nil
}

func (rm *ReplicaManager) StopAll() error {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	for name, replica := range rm.replicas {
		log.Printf("🛑 Stopping replica %s...", name)
		if err := replica.Close(); err != nil {
			log.Printf("❌ Error stopping replica %s: %v", name, err)
		}
	}

	return nil
}

func (rm *ReplicaManager) PrintStatusAll() {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	fmt.Printf("\n📊 Cluster Status Report:\n")
	fmt.Printf("═══════════════════════════════════════\n")

	for name, replica := range rm.replicas {
		status := replica.SyncStatus()
		info := replica.GetInfo()

		fmt.Printf("\n🔹 Replica: %s\n", name)
		fmt.Printf("   Master: %s\n", status.MasterHost)
		fmt.Printf("   Connected: %v\n", status.Connected)
		fmt.Printf("   Synced: %v\n", status.InitialSyncCompleted)
		fmt.Printf("   Offset: %d\n", status.ReplicationOffset)
		fmt.Printf("   Commands: %d\n", status.CommandsProcessed)

		if keys, ok := info["keys"]; ok {
			fmt.Printf("   Keys: %v\n", keys)
		}
		if mem, ok := info["memory_usage"]; ok {
			fmt.Printf("   Memory: %v bytes\n", mem)
		}
	}
}

func (rm *ReplicaManager) GetReplica(name string) *redisreplica.Replica {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.replicas[name]
}

func main() {
	log.Println("🔧 Setting up Redis Replica Cluster")

	manager := NewReplicaManager()

	// Configure multiple replicas for different masters
	// Note: Adjust these addresses to match your Redis setup
	replicaConfigs := []struct {
		name        string
		masterAddr  string
		replicaAddr string
	}{
		{"primary", "localhost:6379", ":6380"},
		{"cache", "localhost:6381", ":6382"},   // If you have a second Redis instance
		{"session", "localhost:6383", ":6384"}, // If you have a third Redis instance
	}

	// Add replicas
	for _, config := range replicaConfigs {
		if err := manager.AddReplica(config.name, config.masterAddr, config.replicaAddr); err != nil {
			log.Printf("⚠️  Failed to add replica %s: %v", config.name, err)
			continue
		}
		log.Printf("✅ Added replica %s for master %s", config.name, config.masterAddr)
	}

	// Start all replicas
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if err := manager.StartAll(ctx); err != nil {
		log.Fatal("Failed to start replicas:", err)
	}

	log.Println("🎉 All replicas started successfully!")

	// Set up graceful shutdown
	defer func() {
		log.Println("🛑 Shutting down cluster...")
		if err := manager.StopAll(); err != nil {
			log.Printf("❌ Error during shutdown: %v", err)
		}
	}()

	// Demonstrate cross-replica operations
	demonstrateCrossReplicaOperations(manager)

	// Monitor all replicas
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	log.Println("🔄 Starting cluster monitoring...")
	log.Println("💡 Try setting different keys in different Redis masters:")
	log.Println("   redis-cli -p 6379 SET primary:key 'Hello from primary'")
	log.Println("   redis-cli -p 6381 SET cache:key 'Hello from cache'")
	log.Println("   redis-cli -p 6383 SET session:key 'Hello from session'")

	for {
		select {
		case <-ticker.C:
			manager.PrintStatusAll()

		case <-ctx.Done():
			log.Println("Context cancelled, shutting down cluster...")
			return
		}
	}
}

func demonstrateCrossReplicaOperations(manager *ReplicaManager) {
	fmt.Printf("\n🔍 Cross-Replica Data Access:\n")
	fmt.Printf("═══════════════════════════════════════\n")

	replicas := []string{"primary", "cache", "session"}
	testKeys := []string{"user:1", "config:app", "counter"}

	for _, replicaName := range replicas {
		replica := manager.GetReplica(replicaName)
		if replica == nil {
			continue
		}

		storage := replica.Storage()
		fmt.Printf("\n🔹 Checking %s replica:\n", replicaName)

		for _, key := range testKeys {
			if value, exists := storage.Get(key); exists {
				fmt.Printf("   %s = %s\n", key, string(value))
			}
		}

		// Show all keys in this replica
		allKeys := storage.Keys()
		if len(allKeys) > 0 {
			fmt.Printf("   All keys: %v\n", allKeys)
		} else {
			fmt.Printf("   No keys found\n")
		}
	}
}
