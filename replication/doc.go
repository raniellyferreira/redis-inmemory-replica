// Package replication implements Redis replication client and synchronization logic.
//
// This package handles the complete Redis replication protocol, including:
//   - Initial handshake with master
//   - RDB file parsing for bulk synchronization
//   - Streaming command replication
//   - Reconnection and error recovery
//
// Basic usage:
//
//	client := replication.NewClient("localhost:6379", storage)
//	err := client.Start(context.Background())
//	if err != nil {
//		log.Fatal(err)
//	}
//
// The replication client supports:
//   - Authentication
//   - TLS connections
//   - Partial resynchronization
//   - Command filtering
//   - Connection pooling
package replication
