# Redis In-Memory Replica

[![Go Version](https://img.shields.io/badge/go-%3E%3D1.24.5-blue.svg)](https://golang.org/)
[![Go Reference](https://pkg.go.dev/badge/github.com/raniellyferreira/redis-inmemory-replica.svg)](https://pkg.go.dev/github.com/raniellyferreira/redis-inmemory-replica)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A production-ready Go library that implements an in-memory Redis replica with real-time synchronization capabilities. The library follows Go best practices for public packages and supports Redis clients like `github.com/redis/go-redis`.

## Features

- **Real-time Replication**: Connects to Redis master and maintains synchronized copy in memory
- **Streaming Parsers**: Memory-efficient RESP and RDB parsing for high-throughput applications
- **Redis Compatibility**: Works with popular Redis clients for read operations
- **Production Ready**: Comprehensive error handling, logging, metrics, and graceful shutdown
- **High Performance**: Optimized for >100k ops/sec with minimal memory overhead
- **Flexible Configuration**: Extensive configuration options with functional options pattern
- **Monitoring**: Built-in observability with metrics collection and status reporting

## Quick Start

### Installation

```bash
go get github.com/raniellyferreira/redis-inmemory-replica
```

### Basic Usage

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/raniellyferreira/redis-inmemory-replica"
)

func main() {
    // Create replica
    replica, err := redisreplica.New(
        redisreplica.WithMaster("localhost:6379"),
        redisreplica.WithReplicaAddr(":6380"),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer replica.Close()

    // Start replication
    ctx := context.Background()
    if err := replica.Start(ctx); err != nil {
        log.Fatal(err)
    }

    // Wait for initial sync
    if err := replica.WaitForSync(ctx); err != nil {
        log.Fatal(err)
    }

    // Check sync status
    status := replica.SyncStatus()
    log.Printf("Sync completed: %v", status.InitialSyncCompleted)

    // Access data
    storage := replica.Storage()
    if value, exists := storage.Get("mykey"); exists {
        log.Printf("Value: %s", value)
    }
}
```

## Configuration Options

The library supports extensive configuration through functional options:

```go
replica, err := redisreplica.New(
    // Master connection
    redisreplica.WithMaster("redis.example.com:6379"),
    redisreplica.WithMasterAuth("password"),
    redisreplica.WithTLS(&tls.Config{...}),
    
    // Local server
    redisreplica.WithReplicaAddr(":6380"),
    
    // Timeouts and limits
    redisreplica.WithSyncTimeout(30*time.Second),
    redisreplica.WithMaxMemory(1024*1024*1024), // 1GB
    
    // Observability
    redisreplica.WithLogger(customLogger),
    redisreplica.WithMetrics(metricsCollector),
    
    // Behavioral options
    redisreplica.WithCommandFilters([]string{"SET", "DEL"}),
    redisreplica.WithServerEnabled(true),
)
```

## Examples

### Basic Example

See [`examples/basic/main.go`](examples/basic/main.go) for a complete basic usage example.

```bash
# Run basic example (requires Redis on localhost:6379)
make examples
```

### Monitoring Example

See [`examples/monitoring/main.go`](examples/monitoring/main.go) for monitoring and observability features.

### Cluster Example

See [`examples/cluster/main.go`](examples/cluster/main.go) for managing multiple replicas.

## API Reference

### Core Types

#### Replica

The main replica instance:

```go
type Replica struct {
    Stats ReplicationStats // Exported for monitoring
}

// Create new replica
func New(opts ...Option) (*Replica, error)

// Lifecycle management
func (r *Replica) Start(ctx context.Context) error
func (r *Replica) Close() error

// Synchronization
func (r *Replica) WaitForSync(ctx context.Context) error
func (r *Replica) SyncStatus() SyncStatus
func (r *Replica) OnSyncComplete(fn func())

// Data access
func (r *Replica) Storage() storage.Storage
func (r *Replica) IsConnected() bool
func (r *Replica) GetInfo() map[string]interface{}
```

#### SyncStatus

Provides detailed synchronization status:

```go
type SyncStatus struct {
    InitialSyncCompleted bool
    InitialSyncProgress  float64 // 0.0 to 1.0
    Connected           bool
    MasterHost          string
    ReplicationOffset   int64
    LastSyncTime        time.Time
    BytesReceived       int64
    CommandsProcessed   int64
}
```

### Storage Interface

Direct access to the underlying storage:

```go
type Storage interface {
    // String operations
    Get(key string) ([]byte, bool)
    Set(key string, value []byte, expiry *time.Time) error
    Del(keys ...string) int64
    Exists(keys ...string) int64
    
    // Key operations
    Keys() []string
    KeyCount() int64
    Scan(cursor int64, match string, count int64) (int64, []string)
    
    // Database operations
    SelectDB(db int) error
    FlushAll() error
    
    // Metadata
    Info() map[string]interface{}
    MemoryUsage() int64
    Close() error
}
```

## Performance

The library is optimized for high performance:

- **Throughput**: >100k operations/second
- **Memory Overhead**: <20% compared to data size  
- **Sync Speed**: 1GB RDB sync in <30 seconds
- **GC Pressure**: Minimal through object pooling

### Benchmarks

```bash
# Run benchmarks
make benchmark
```

Example results:
```
BenchmarkRESPParser-8    	 1000000	      1200 ns/op	     256 B/op	       4 allocs/op
BenchmarkStorageGet-8    	 5000000	       300 ns/op	       0 B/op	       0 allocs/op
BenchmarkStorageSet-8    	 2000000	       800 ns/op	     128 B/op	       2 allocs/op
```

## Architecture

### Components

- **Protocol Package**: Streaming RESP parser and writer
- **Storage Package**: In-memory storage with Redis data types
- **Replication Package**: Redis replication protocol implementation
- **Main Package**: High-level API and configuration

### Data Flow

```
Redis Master → RESP Protocol → RDB Parser → Storage Layer → Application
                              ↓
                         Command Stream → Command Processor → Storage Updates
```

## Compatibility

- **Redis Versions**: 5.0+ replication protocol
- **Go Versions**: 1.24.5+
- **Redis Clients**: Compatible with `github.com/redis/go-redis` and others
- **Platforms**: Linux, macOS, Windows

## Development

### Setup

```bash
# Clone repository
git clone https://github.com/raniellyferreira/redis-inmemory-replica.git
cd redis-inmemory-replica

# Set up development environment
make dev-setup

# Run tests
make test

# Run linter
make lint
```

### Project Structure

```
redis-inmemory-replica/
├── doc.go              # Package documentation
├── replica.go          # Main API
├── options.go          # Configuration options
├── errors.go           # Custom errors
├── version.go          # Version information
├── protocol/           # RESP protocol implementation
├── storage/            # Storage interfaces and implementation
├── replication/        # Replication client and sync logic
├── examples/           # Usage examples
├── Makefile           # Build automation
└── README.md          # This file
```

### Testing

```bash
# Run all tests
make test

# Run tests with coverage
make coverage

# Run benchmarks
make benchmark

# Run linter
make lint
```

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

### Code Style

- Follow standard Go conventions
- Add tests for new functionality
- Update documentation as needed
- Run `make check` before submitting

## Monitoring and Observability

### Custom Logger

Implement the `Logger` interface for custom logging:

```go
type CustomLogger struct{}

func (l *CustomLogger) Debug(msg string, fields ...Field) {
    // Your debug logging
}

func (l *CustomLogger) Info(msg string, fields ...Field) {
    // Your info logging  
}

func (l *CustomLogger) Error(msg string, fields ...Field) {
    // Your error logging
}
```

### Metrics Collection

Implement the `MetricsCollector` interface:

```go
type CustomMetrics struct{}

func (m *CustomMetrics) RecordSyncDuration(duration time.Duration) {
    // Record sync duration
}

func (m *CustomMetrics) RecordCommandProcessed(cmd string, duration time.Duration) {
    // Record command processing metrics
}

// ... other methods
```

## Troubleshooting

### Common Issues

1. **Connection Refused**
   - Ensure Redis master is running and accessible
   - Check firewall settings and network connectivity

2. **Authentication Failed**
   - Verify Redis AUTH password
   - Check Redis configuration for authentication requirements

3. **Sync Timeout**
   - Increase sync timeout for large datasets
   - Check network bandwidth and Redis performance

4. **Memory Issues**
   - Set appropriate memory limits with `WithMaxMemory`
   - Monitor memory usage with `GetInfo()`

### Debug Logging

Enable debug logging for troubleshooting:

```go
replica, err := redisreplica.New(
    redisreplica.WithMaster("localhost:6379"),
    redisreplica.WithLogger(debugLogger),  // Custom logger with debug enabled
)
```

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- Redis team for the excellent protocol documentation
- Go community for best practices and patterns
- Contributors and users of this library

---

For more information, visit the [documentation](https://pkg.go.dev/github.com/raniellyferreira/redis-inmemory-replica) or check out the [examples](examples/).