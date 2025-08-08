// Package redisreplica provides an in-memory Redis replica implementation
// with real-time synchronization capabilities.
//
// The replica connects to a Redis master server and maintains a synchronized
// copy of the data in memory. It provides a Redis-compatible interface for
// read operations.
//
// Basic usage:
//
//	replica, err := redisreplica.New(
//		redisreplica.WithMaster("localhost:6379"),
//		redisreplica.WithReplicaAddr(":6380"),
//	)
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer replica.Close()
//
//	// Wait for initial sync
//	if err := replica.Start(context.Background()); err != nil {
//		log.Fatal(err)
//	}
//
//	// Check sync status
//	status := replica.SyncStatus()
//	fmt.Printf("Sync completed: %v\n", status.InitialSyncCompleted)
//
// The library supports:
//
//   - Real-time replication using Redis RESP protocol
//   - Streaming RDB parsing for large datasets
//   - Redis-compatible server for client connections
//   - Comprehensive monitoring and observability
//   - Graceful shutdown and error recovery
//   - Memory-efficient streaming parsers
//
// For more examples and advanced usage, see the examples/ directory.
package redisreplica
