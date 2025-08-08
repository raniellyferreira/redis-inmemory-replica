# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.1.0] - 2025-07-31

### Added
- Configurable pattern matching strategies for optimized key lookups.
- Optimized incremental cleanup with sampling for better performance.
- Performance optimizations for high-throughput scenarios.

## [1.0.0] - 2024-01-30

### Added
- Initial release of Redis In-Memory Replica library
- Real-time Redis replication with master synchronization
- Streaming RESP protocol parser and writer
- In-memory storage engine with Redis data type support
- RDB file parsing for initial synchronization
- Comprehensive configuration options using functional options pattern
- Built-in observability with logging and metrics interfaces
- Multiple database support (0-15)
- Key expiration and TTL management
- Memory usage tracking and limits
- Command filtering capabilities
- Graceful shutdown and error recovery
- Complete test suite with >80% coverage
- Comprehensive documentation and examples
- Cross-platform support (Linux, macOS, Windows)
- GitHub Actions CI/CD pipeline

### Features
- **High Performance**: >100k ops/sec throughput with minimal memory overhead
- **Memory Efficient**: Streaming parsers prevent excessive memory usage
- **Production Ready**: Comprehensive error handling and monitoring
- **Redis Compatible**: Works with popular Redis clients like go-redis
- **Flexible Configuration**: Extensive options for timeouts, limits, and behavior
- **Observability**: Built-in metrics collection and structured logging
- **Reliability**: Automatic reconnection and partial sync support

### API
- `New()` - Create new replica with options
- `Start()` - Begin replication and start local server
- `WaitForSync()` - Wait for initial synchronization completion
- `SyncStatus()` - Get current synchronization status
- `Close()` - Graceful shutdown
- `Storage()` - Direct access to underlying storage
- `OnSyncComplete()` - Register sync completion callbacks

### Examples
- Basic usage example
- Monitoring and observability example  
- Multi-replica cluster example
- Database filtering example
- Lua scripting demonstration
- Pattern matching example

### Supported Redis Features
- String operations (GET, SET, DEL, EXISTS, TYPE)
- Key operations (SELECT, multiple databases 0-15)
- Key expiration and TTL (storage level)
- RESP protocol versions 2 and 3
- RDB file format parsing
- Command replication
- Lua script execution (EVAL, EVALSHA, SCRIPT LOAD/EXISTS/FLUSH)
- SSL/TLS connections to Redis masters

### Dependencies
- Go 1.23 or higher
- github.com/yuin/gopher-lua v1.1.1 (for Lua scripting support)

## [Unreleased]

### Planned Features
- Redis Cluster support
- Additional Redis data types (Lists, Sets, Hashes, Sorted Sets) - command handlers
- Persistence layer for restart recovery
- HTTP health check endpoints
- Prometheus metrics export
- Redis Streams support
- Advanced command filtering by command type
- Replica chaining support
