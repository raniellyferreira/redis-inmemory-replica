// Package main demonstrates the timeout fix for Redis replication
package main

import (
	"fmt"
	"log"
	"time"

	"github.com/raniellyferreira/redis-inmemory-replica"
)

func main() {
	fmt.Println("🔧 Redis Replication Timeout Fix Demo")
	fmt.Println("=====================================")
	fmt.Println()

	fmt.Println("This demo explains the timeout issue and how it was fixed:")
	fmt.Println()

	fmt.Println("❌ BEFORE (Problem):")
	fmt.Println("   • Client sets 30-second read timeout on TCP connection")
	fmt.Println("   • Timeout applies to ALL read operations, including replication stream")
	fmt.Println("   • During quiet periods (no Redis writes), replica times out after 30s")
	fmt.Println("   • Results in: 'read tcp ...: i/o timeout' errors")
	fmt.Println("   • Causes unnecessary reconnections during normal operation")
	fmt.Println()

	fmt.Println("✅ AFTER (Solution):")
	fmt.Println("   • Keep read timeout for connection/auth/sync phases (30s)")
	fmt.Println("   • Remove read timeout during replication streaming phase")
	fmt.Println("   • Send periodic REPLCONF ACK commands (every 45s by default)")
	fmt.Println("   • Prevents master from timing out replica during idle periods")
	fmt.Println("   • Allows replica to wait indefinitely for next command")
	fmt.Println("   • Matches standard Redis replication behavior")
	fmt.Println("   • Eliminates false timeout errors during idle periods")
	fmt.Println()

	fmt.Println("🔄 Technical Details:")
	fmt.Println("   • conn.SetReadDeadline(time.Time{}) removes timeout")
	fmt.Println("   • Applied after entering streamCommands() phase")
	fmt.Println("   • Periodic REPLCONF ACK <offset> commands sent via heartbeat")
	fmt.Println("   • Heartbeat interval configurable (default 45s)")
	fmt.Println("   • Timeout still active for handshake operations")
	fmt.Println("   • Write timeout remains active for outgoing commands")
	fmt.Println()

	// Create a replica to demonstrate the configuration
	replica, err := redisreplica.New(
		redisreplica.WithMaster("localhost:6379"),
		redisreplica.WithSyncTimeout(30*time.Second),
		redisreplica.WithReadTimeout(30*time.Second),       // Used only for handshake
		redisreplica.WithWriteTimeout(10*time.Second),      // Always active
		redisreplica.WithHeartbeatInterval(45*time.Second), // Send REPLCONF ACK every 45s
	)
	if err != nil {
		log.Fatal("Failed to create replica:", err)
	}
	defer func() { _ = replica.Close() }()

	fmt.Println("📝 Example Configuration:")
	fmt.Printf("   • Master: localhost:6379\n")
	fmt.Printf("   • Sync Timeout: 30s (for connection/auth/sync)\n")
	fmt.Printf("   • Read Timeout: 30s (removed during streaming)\n")
	fmt.Printf("   • Write Timeout: 10s (always active)\n")
	fmt.Printf("   • Heartbeat Interval: 45s (REPLCONF ACK frequency)\n")
	fmt.Println()

	fmt.Println("💡 Impact:")
	fmt.Println("   • Eliminates 'i/o timeout' errors during normal operation")
	fmt.Println("   • Replica can handle long idle periods gracefully")
	fmt.Println("   • Reduces unnecessary reconnections and log noise")
	fmt.Println("   • Improves replication stability and performance")
	fmt.Println()

	fmt.Println("🧪 To test the fix:")
	fmt.Println("   1. Start a Redis master: redis-server --port 6379")
	fmt.Println("   2. Run: go run examples/basic/main.go")
	fmt.Println("   3. Wait more than 30 seconds without Redis activity")
	fmt.Println("   4. Observe: No timeout errors in replica logs")
	fmt.Println("   5. Add data to Redis: redis-cli SET test 'value'")
	fmt.Println("   6. Verify: Replica receives and processes the command")
	fmt.Println()

	// Note: We're not actually starting the replica here since this is just a demo
	// In a real scenario, you would call replica.Start(ctx) and replica.WaitForSync(ctx)

	fmt.Println("Demo completed! 🎉")
}