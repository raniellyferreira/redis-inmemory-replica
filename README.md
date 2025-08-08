# Redis In-Memory Replica

[![Go Version](https://img.shields.io/badge/go-%3E%3D1.24.5-blue.svg)](https://golang.org/)
[![Go Reference](https://pkg.go.dev/badge/github.com/raniellyferreira/redis-inmemory-replica.svg)](https://pkg.go.dev/github.com/raniellyferreira/redis-inmemory-replica)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A production-ready Go library that implements an in-memory Redis replica with real-time synchronization capabilities. The library follows Go best practices for public packages and supports Redis clients like `github.com/redis/go-redis`.

## Features

- **Real-time Replication**: Connects to Redis master and maintains synchronized copy in memory
- **Streaming Parsers**: Memory-efficient RESP and RDB parsing for high-throughput applications
- **Redis Compatibility**: Works with popular Redis clients for read operations
- **Lua Script Execution**: Full Redis-compatible Lua scripting with EVAL/EVALSHA support
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

## Lua Script Execution

The library provides comprehensive Redis-compatible Lua script execution, enabling atomic operations and complex data processing.

### Supported Commands

- `EVAL script numkeys key1 ... arg1 ...` - Execute Lua script
- `EVALSHA sha1 numkeys key1 ... arg1 ...` - Execute cached script
- `SCRIPT LOAD script` - Cache script and return SHA1
- `SCRIPT EXISTS sha1 [sha1 ...]` - Check if scripts exist
- `SCRIPT FLUSH` - Remove all cached scripts

### Basic Lua Scripting

```go
package main

import (
    "fmt"
    "log"
    
    "github.com/raniellyferreira/redis-inmemory-replica/lua"
    "github.com/raniellyferreira/redis-inmemory-replica/storage"
)

func main() {
    // Create storage and Lua engine
    stor := storage.NewMemory()
    engine := lua.NewEngine(stor)

    // Simple script execution
    result, err := engine.Eval("return 'Hello from Lua!'", []string{}, []string{})
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Result: %v\n", result)

    // Using KEYS and ARGV
    script := "return KEYS[1] .. ':' .. ARGV[1]"
    result, err = engine.Eval(script, []string{"user"}, []string{"123"})
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Result: %v\n", result) // Output: "user:123"
}
```

### Redis Commands in Lua

Scripts can execute Redis commands using `redis.call()` and `redis.pcall()`:

```go
// Redis commands in Lua scripts
script := `
    redis.call('SET', KEYS[1], ARGV[1])
    local value = redis.call('GET', KEYS[1])
    return 'Stored and retrieved: ' .. value
`
result, err := engine.Eval(script, []string{"mykey"}, []string{"myvalue"})
// Result: "Stored and retrieved: myvalue"

// Error handling with redis.pcall()
script = `
    local result = redis.pcall('GET', 'nonexistent')
    if type(result) == 'table' and result.err then
        return 'Error: ' .. result.err
    else
        return 'Success: ' .. tostring(result)
    end
`
result, err = engine.Eval(script, []string{}, []string{})
```

### Script Caching

Use script caching for better performance with frequently executed scripts:

```go
// Load and cache a script
script := "return 'This is a cached script with arg: ' .. (ARGV[1] or 'none')"
sha := engine.LoadScript(script)
fmt.Printf("Script SHA1: %s\n", sha)

// Execute cached script by SHA1
result, err := engine.EvalSHA(sha, []string{}, []string{"hello"})
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Result: %v\n", result)

// Check if scripts exist
exists := engine.ScriptExists([]string{sha})
fmt.Printf("Script exists: %v\n", exists[0])

// Flush all cached scripts
engine.FlushScripts()
```

### Complex Script Example

```go
// Complex script with loops and multiple Redis operations
script := `
    local results = {}
    for i = 1, #KEYS do
        local key = KEYS[i]
        local val = ARGV[i] or 'default'
        redis.call('SET', key, val)
        results[i] = key .. '=' .. redis.call('GET', key)
    end
    return results
`

keys := []string{"key1", "key2", "key3"}
args := []string{"val1", "val2", "val3"}
result, err := engine.Eval(script, keys, args)
// Result: ["key1=val1", "key2=val2", "key3=val3"]
```

### Integration with Redis Clients

The Lua execution engine integrates seamlessly with Redis clients:

```go
// Using with github.com/redis/go-redis
client := redis.NewClient(&redis.Options{
    Addr: "replica:6380", // Your replica server address
})

script := "return redis.call('SET', KEYS[1], ARGV[1])"
result := client.Eval(ctx, script, []string{"mykey"}, "myvalue")

// Using EVALSHA for cached scripts
sha := client.ScriptLoad(ctx, script).Val()
result = client.EvalSHA(ctx, sha, []string{"mykey"}, "myvalue")
```

### Performance

The Lua execution engine is optimized for high performance:

- **Simple scripts**: ~85μs per execution
- **Redis commands**: ~97μs per execution  
- **Cached scripts (EVALSHA)**: ~85μs per execution
- **Memory efficient**: ~220KB per execution with minimal allocations

### Data Type Conversion

The engine provides seamless conversion between Lua and Redis data types:

| Lua Type | Redis Type | Notes |
|----------|------------|-------|
| `nil` | Null bulk string | Represents Redis NULL |
| `false` | Null bulk string | Redis treats false as NULL |
| `string` | Bulk string | Direct conversion |
| `number` | Integer/Bulk string | Integers vs floats handled appropriately |
| `table` (array) | Array | Lua arrays become Redis arrays |
| `table` (hash) | Array of key-value pairs | Lua hashes flattened to arrays |

## Configuration Options

The library supports extensive configuration through functional options:

```go
replica, err := redisreplica.New(
    // Master connection
    redisreplica.WithMaster("redis.example.com:6379"),
    redisreplica.WithMasterAuth(os.Getenv("REDIS_PASSWORD")), // Use environment variable
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

## Storage Cleanup Configurations

The library includes an optimized incremental cleanup system that mirrors Redis native behavior. Six predefined cleanup configurations are available for different use cases:

### Predefined Configurations

```go
import "github.com/raniellyferreira/redis-inmemory-replica/storage"

// For most use cases (default)
replica.Storage().SetCleanupConfig(storage.CleanupConfigDefault)

// Optimized for specific dataset sizes
replica.Storage().SetCleanupConfig(storage.CleanupConfigSmallDataset)    // < 10,000 keys
replica.Storage().SetCleanupConfig(storage.CleanupConfigMediumDataset)   // 10,000-100,000 keys  
replica.Storage().SetCleanupConfig(storage.CleanupConfigLargeDataset)    // > 100,000 keys

// Performance-focused configurations
replica.Storage().SetCleanupConfig(storage.CleanupConfigBestPerformance) // Maximum cleanup throughput
replica.Storage().SetCleanupConfig(storage.CleanupConfigLowLatency)      // Minimal impact on response times
```

### Configuration Details

| Configuration | Sample Size | Max Rounds | Batch Size | Expired Threshold | Use Case |
|---------------|-------------|------------|------------|-------------------|----------|
| `CleanupConfigDefault` | 20 | 4 | 10 | 25% | Balanced performance for most applications |
| `CleanupConfigSmallDataset` | 10 | 2 | 5 | 50% | Datasets with < 10,000 keys |
| `CleanupConfigMediumDataset` | 25 | 5 | 12 | 30% | Datasets with 10,000-100,000 keys |
| `CleanupConfigLargeDataset` | 50 | 8 | 25 | 15% | Datasets with > 100,000 keys |
| `CleanupConfigBestPerformance` | 100 | 10 | 50 | 10% | Maximum cleanup throughput |
| `CleanupConfigLowLatency` | 15 | 3 | 8 | 40% | Latency-sensitive applications |

### Custom Configuration

For specific requirements, you can create custom cleanup configurations:

```go
import "github.com/raniellyferreira/redis-inmemory-replica/storage"

customConfig := storage.CleanupConfig{
    SampleSize:       30,    // Keys sampled per round
    MaxRounds:        6,     // Max cleanup rounds per cycle  
    BatchSize:        15,    // Keys deleted per batch
    ExpiredThreshold: 0.2,   // Continue if ≥20% sampled keys expired
}

replica.Storage().SetCleanupConfig(customConfig)
```

## Security

This library includes comprehensive security features for production deployments:

### TLS Configuration

Use secure TLS with proper certificate validation:

```go
// Option 1: Custom TLS configuration
tlsConfig := &tls.Config{
    ServerName:         "redis.example.com",
    InsecureSkipVerify: false, // Always verify certificates in production
    MinVersion:         tls.VersionTLS12,
}

replica, err := redisreplica.New(
    redisreplica.WithMaster("redis.example.com:6380"),
    redisreplica.WithTLS(tlsConfig),
)

// Option 2: Secure TLS with defaults (recommended)
replica, err := redisreplica.New(
    redisreplica.WithMaster("redis.example.com:6380"),
    redisreplica.WithSecureTLS("redis.example.com"), // Secure defaults
)
```

### Authentication & Authorization

Configure strong authentication using environment variables:

```go
replica, err := redisreplica.New(
    redisreplica.WithMaster("redis.example.com:6379"),
    redisreplica.WithMasterAuth(os.Getenv("REDIS_MASTER_PASSWORD")), // Never hardcode
    redisreplica.WithReplicaAuth(os.Getenv("REDIS_REPLICA_PASSWORD")), // For replica server
    redisreplica.WithReadOnly(true), // Enforce read-only mode
)
```

**Security Note**: Always use environment variables or secure configuration management for credentials. Never hardcode passwords in source code.

### Network Security

Configure proper timeouts and limits:

```go
replica, err := redisreplica.New(
    redisreplica.WithConnectTimeout(10*time.Second),
    redisreplica.WithReadTimeout(30*time.Second),
    redisreplica.WithWriteTimeout(10*time.Second),
    redisreplica.WithMaxMemory(100*1024*1024), // 100MB limit
)
```

### Database Filtering

Limit replication to specific databases:

```go
replica, err := redisreplica.New(
    redisreplica.WithDatabases([]int{0, 1}), // Only replicate databases 0 and 1
)
```

### Security Auditing

The library includes comprehensive security scanning that filters out false positives:

```bash
# Run comprehensive security audit
make security-audit

# Install security tools
make security-install

# Run vulnerability scan
make security-scan

# Run static security analysis
make security-static
```

**Enhanced Security Features:**
- **Smart Secret Detection**: Distinguishes between actual hardcoded secrets and legitimate variable names
- **Test File Exclusion**: Security scans automatically ignore test files and examples
- **Safe Operation Marking**: Use `// safe: reason` comments for intentional unsafe operations
- **CI/CD Integration**: Automated security checks in GitHub Actions prevent security issues

### Security Best Practices

1. **Always use TLS** in production environments
2. **Configure strong authentication** for both master and replica
3. **Set appropriate timeouts** to prevent hanging connections
4. **Use memory limits** to prevent DoS attacks
5. **Filter databases** to limit exposure
6. **Monitor and log** security events
7. **Keep dependencies updated** and scan for vulnerabilities

For detailed security guidelines, see [SECURITY.md](SECURITY.md).

## Examples

### Basic Example

See [`examples/basic/main.go`](examples/basic/main.go) for a complete basic usage example.

```bash
# Run basic example (requires Redis on localhost:6379)
make examples
```

### Lua Scripting Examples

See [`examples/lua-demo/main.go`](examples/lua-demo/main.go) for standalone Lua scripting examples.

See [`examples/replica-lua-demo/main.go`](examples/replica-lua-demo/main.go) for Lua scripting integrated with replication.

```bash
# Run Lua demo examples
cd examples/lua-demo && go run main.go
cd examples/replica-lua-demo && go run main.go
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
- **Lua Package**: Redis-compatible Lua script execution engine
- **Server Package**: Redis protocol server with command processing
- **Main Package**: High-level API and configuration

### Data Flow

```
Redis Master → RESP Protocol → RDB Parser → Storage Layer → Application
                              ↓
                         Command Stream → Command Processor → Storage Updates
                              ↓
                         Lua Engine → Script Execution → Redis Commands
```

## Compatibility

- **Redis Versions**: 5.0+ replication protocol, **Enhanced Redis 7.x support**
- **Go Versions**: 1.24.5+
- **Redis Clients**: Compatible with `github.com/redis/go-redis` and others
- **Platforms**: Linux, macOS, Windows

### Redis 7.x Compatibility

The library has been enhanced with comprehensive Redis 7.x support:

- ✅ **Full RDB parsing compatibility** for Redis 7.0, 7.2, and 7.4
- ✅ **Enhanced special string encoding support** (fixes encoding 33 errors)
- ✅ **Multi-version testing** with automated CI workflows
- ✅ **Graceful handling** of LZF compression and future encodings
- ✅ **Local testing support** with [act](https://github.com/nektos/act)

For detailed Redis 7.x compatibility information, see [docs/redis-7x-compatibility.md](docs/redis-7x-compatibility.md).

### Local Testing

Test Redis 7.x compatibility locally:

```bash
# Install act (GitHub Actions runner)
brew install act  # macOS
# or follow: https://github.com/nektos/act

# Run Redis 7.x compatibility tests
act -W .github/workflows/redis-compatibility.yml

# Test specific Redis version
./scripts/test-redis-compatibility.sh 7.2.4
```

See [docs/local-testing-with-act.md](docs/local-testing-with-act.md) for complete testing guide.

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
├── lua/                # Lua script execution engine
├── server/             # Redis protocol server
├── examples/           # Usage examples
│   ├── basic/          # Basic replication example
│   ├── lua-demo/       # Lua scripting examples
│   ├── replica-lua-demo/ # Integrated Lua + replication
│   ├── monitoring/     # Monitoring and metrics
│   └── cluster/        # Multiple replica management
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

5. **RDB Parsing Errors (Redis 7.x)**
   - **Fixed**: "invalid special string encoding: 33" errors
   - The library now gracefully handles Redis 7.x RDB formats
   - Use Redis 7.x compatible versions for best results

### Redis 7.x Specific Issues

#### "unsupported special string encoding" errors

**Status**: ✅ **FIXED** in this version

The library now supports:
- All standard Redis 7.x string encodings (0-3)
- LZF compression detection and handling
- Graceful fallback for future encodings (4-63)

#### Large RDB files in Redis 7.x

**Solution**: Increase timeouts for larger databases:

```go
replica, err := redisreplica.New(
    redisreplica.WithMaster("redis7.example.com:6379"),
    redisreplica.WithSyncTimeout(60*time.Second), // Increased for Redis 7.x
)
```

### Debug Logging

Enable debug logging for troubleshooting:

```go
replica, err := redisreplica.New(
    redisreplica.WithMaster("localhost:6379"),
    redisreplica.WithLogger(debugLogger),  // Custom logger with debug enabled
)
```

For comprehensive troubleshooting, see [docs/redis-7x-compatibility.md#troubleshooting](docs/redis-7x-compatibility.md#troubleshooting).

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- Redis team for the excellent protocol documentation
- Go community for best practices and patterns
- Contributors and users of this library

---

For more information, visit the [documentation](https://pkg.go.dev/github.com/raniellyferreira/redis-inmemory-replica) or check out the [examples](examples/).