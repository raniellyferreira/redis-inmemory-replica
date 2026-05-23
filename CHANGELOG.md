# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.5.0] - 2026-05-23

This release consolidates nine months of work landed in `main` since `v1.4.0`
(PRs #25, #26, #28, #29, #30). Public API is unchanged from `v1.4.x` — no
breaking changes. Drop-in upgrade.

This is the **final feature release of the v1 line**. Active development now
moves to the v2 refactor (see ADR 0001 below), which will ship at module path
`github.com/raniellyferreira/redis-inmemory-replica/v2`; v1.5.x will receive
critical fixes only.

### Added
- **Performance audit infrastructure** with a comprehensive benchmark suite
  covering `storage`, `lua`, `protocol`, `replication`, and the root package,
  plus profiling scripts (`scripts/perf/bench.sh`, `compare.sh`, `profile.sh`)
  and a scheduled CI workflow (`.github/workflows/performance-audit.yml`)
  running weekly. (#25)
- **ADR 0001 — v2 total refactor (Accepted)** at `docs/adr/0001-v2-total-refactor.md`
  documenting the architectural roadmap for v2.0 (polymorphic `Value` model,
  streaming RDB ingest, modular replication client, command-applier registry,
  RDB type coverage for Redis 6.0–8.0, persistent partial-resync state,
  Go 1.26 baseline, per-phase performance gates, 700-line hard cap on
  non-test `.go` files). Status: **Accepted**. No functional impact on
  v1.5.x. (#29)
- **Project skills and agents under `.claude/`** (7 agents, 3 skills) so
  contributors using Claude Code automatically follow this project's
  engineering, code-review, and modern-Go standards. (#29)

### Performance
- **Lua script cache: ~86% latency reduction and ~91% allocation reduction**
  via cache key restructuring and pre-sized allocations on the hot path. (#26 — B4)
- **Storage key routing: ~25% throughput improvement** via `xxhash` for the
  sharded in-memory storage introduced in v1.4.0. (#26 — B2)
- **RESP parser: reduced allocations on the parse hot path.** (#26 — B1)
- **RDB parser: batching and pre-sizing** for faster initial-sync ingest. (#26 — B3)
- **GC tuning guide** added to the docs for high-throughput deployments. (#26 — B5)

### Fixed
- **Race condition in the Lua engine cache counters.** `cacheHits` and
  `cacheMisses` were being incremented from `EvalSHA` without holding any
  lock while `CacheStats()` read them under `RLock` — a real race detected
  by `go test -race`. Now uses `sync/atomic` (`AddUint64` / `LoadUint64` /
  `StoreUint64`), idiomatic and faster than re-locking. (#28)
- **Cleanup of `INFO` command builders** in `server/server.go`: replaced
  `WriteString(fmt.Sprintf(...))` with `fmt.Fprintf(...)` (staticcheck
  QF1012, semantically identical, avoids an intermediate string
  allocation). (#29)

### Security
- **`crypto/x509` vulnerabilities patched** by bumping the Go toolchain:
  - **GO-2025-4175** — improper application of excluded DNS name constraints
  - **GO-2025-4155** — excessive resource consumption when printing error strings
  - **GO-2025-4007** — quadratic complexity when checking name constraints
- **`govulncheck ./...` reports zero vulnerabilities** on this release.

### Changed
- **Go toolchain: 1.25.2 → 1.26.3** (latest stable as of 2026-05-23,
  verified via https://go.dev/dl/). Updated `go.mod` and all CI workflows
  (test, lint, e2e, redis-compatibility, security-audit, benchmarks,
  performance-audit, release). (#28, #30)
- **`github.com/cespare/xxhash/v2` promoted to direct dependency** —
  it is actively used by storage sharding since v1.4.0 and by the new
  RESP/storage optimizations in this release. (#28)

### Internal
- **`version.go` re-aligned with the published tag.** The `Version`
  constant was `"1.1.0"` since v1.1.0 and had drifted from the actual
  published tags (v1.2.0, v1.3.0, v1.4.0) — this release sets it to
  `"1.5.0"`.

### Compatibility notes
- No public-API breakage. `import "github.com/raniellyferreira/redis-inmemory-replica"`
  paths and all exported types, functions, and options are unchanged from
  v1.4.x.
- Minimum Go version (consumer-side) follows `go.mod`'s `go 1.26.3`
  directive. Consumers on Go ≥ 1.26 are unaffected; consumers on Go
  1.25.x must upgrade.
- The v2 refactor (ADR 0001) will be published at a **different module
  path** (`.../v2`), so v1.5.x and v2.x can coexist in a single program
  during migration.

## [1.4.0] - 2025-08-11

### Added
- **Complete GUI client support** for tools like TablePlus and Redis Desktop Manager
- **INFO keyspace** command section showing database-specific key counts and expiration statistics
- **CONFIG GET databases** command for client database discovery
- **Enhanced SCAN command** with proper Redis protocol compliance and array response formatting
- **WithDefaultDatabase(db int)** option to set the initial database connection (0-15)
- **Improved benchmark stability** with better error handling for replication failures

### Fixed
- **SCAN command bug** where results were incorrectly formatted as strings instead of proper Redis arrays
- **GUI client compatibility** issues that prevented database listings from displaying correctly
- **INFO keyspace listing inconsistency** where databases would randomly be missing from output due to concurrent access issues
- **Restored performance optimization** in DatabaseInfo() for large databases (>1000 keys) using sampling to estimate expired counts
- **INFO keyspace listing bug** where not all databases with data were consistently shown (reverted complex sampling optimization)
- **Benchmark failures** from "replication stopped unexpectedly" errors are now handled gracefully

### Changed
- Enhanced benchmark timeouts and error handling for more stable CI/CD execution
- Improved INFO command keyspace section to only show databases containing keys

## [1.3.0] - 2025-08-09

### Added
- **Production-ready Redis server** that automatically starts when `WithReplicaAddr()` is provided
- **Complete RESP2 protocol support** for read operations and auxiliary commands
- **Enhanced Redis command support**:
  - `MGET` - Multiple key retrieval
  - `TTL/PTTL` - Time to live in seconds/milliseconds
  - `INFO` - Server and replication information with sections (server, replication, memory)
  - `ROLE` - Replication role information (returns `["slave", master_host, master_port, offset]`)
  - `KEYS pattern` - Key pattern matching with glob-style patterns
  - `SCAN cursor [MATCH pattern] [COUNT n]` - Cursor-based key iteration
  - `DBSIZE` - Number of keys in current database
  - `COMMAND` - Command documentation stub
  - `READONLY` - Read-only mode acknowledgment
- **READONLY error responses** for write commands (`SET`, `DEL`, etc.) with message "READONLY You can't write against a read only replica"
- **LOADING error responses** for read commands before initial sync completion with message "LOADING Redis is loading the dataset in memory"
- **Graceful server shutdown** integrated with `Replica.Close()`
- **Concurrent client support** with thread-safe operations
- **Comprehensive integration tests** using `github.com/redis/go-redis/v9`
- **Enhanced storage interface** with `PTTL()` method for millisecond precision TTL
- **Enhanced sync manager** with `IsInitialSyncCompleted()` method
- **Write command redirection** with `WithWriteRedirection(enabled bool)` option to optionally redirect write commands (`SET`, `DEL`, etc.) to master instead of returning READONLY errors

### Changed
- **BREAKING**: Server now starts automatically when `WithReplicaAddr()` is provided - no separate configuration needed
- **BREAKING**: Default replica address is now empty (no default `:6380` address)
- **Server startup is non-blocking** - replica server starts even if master connection fails
- **Improved error handling** for protocol errors and command validation

### Removed
- **BREAKING**: `enableServer` configuration field and `WithServerEnabled()` option removed
- **BREAKING**: Manual server configuration no longer needed

### Fixed
- Server initialization race conditions
- Memory leaks in concurrent client handling
- Protocol error handling for malformed commands

## [1.2.0] - 2025-08-08

### Added
- Complete Redis 7.x compatibility with LZF decompression, enhanced RDB parsing, comprehensive code quality improvements, robust authentication handling, and reliable shutdown process
- Implement batched RDB logging, improve PSYNC reconnection logic, and resolve streaming timeout errors

## [1.1.0] - 2025-07-31

### Added
- Configurable pattern matching strategies for optimized key lookups.
- Optimized incremental cleanup with sampling for better performance.
- Performance optimizations for high-throughput scenarios.

## [1.0.0] - 2025-07-30

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
- Go 1.24.5 or higher
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
