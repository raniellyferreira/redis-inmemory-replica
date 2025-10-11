# Performance Audit Guide

This guide provides a comprehensive methodology for measuring, analyzing, and optimizing performance in the Redis In-Memory Replica project.

## Table of Contents

1. [Overview](#overview)
2. [Quick Start](#quick-start)
3. [Running Benchmarks](#running-benchmarks)
4. [Profiling](#profiling)
5. [Comparing Results](#comparing-results)
6. [Environment Best Practices](#environment-best-practices)
7. [Subsystem Analysis Checklist](#subsystem-analysis-checklist)
8. [Interpreting Results](#interpreting-results)
9. [Common Optimization Patterns](#common-optimization-patterns)

## Overview

This project focuses on high performance, and this guide provides the infrastructure to:
- Measure throughput and latency consistently
- Track performance regressions over time
- Quantify memory allocations and usage per critical path
- Guide optimization efforts for hot paths

### Critical Performance Paths

- **RESP Parser/Writer**: Protocol parsing and serialization
- **RDB Ingest**: Loading RDB snapshots
- **Storage Operations**: Get/Set with/without TTL
- **Pattern Matching**: Key pattern matching strategies
- **Expiration Cleanup**: Expired key removal
- **Lua Engine**: Script loading and execution

## Quick Start

```bash
# Run all benchmarks with standard configuration
make bench-all

# Profile a specific benchmark
make profile pkg=./storage bench='BenchmarkStorageSet$'

# Compare two benchmark runs
make bench-compare base=baseline.txt head=current.txt
```

## Running Benchmarks

### Basic Usage

Run benchmarks for a specific package:

```bash
go test -bench=. -run=^$ -benchmem ./storage/
```

Run a specific benchmark:

```bash
go test -bench=BenchmarkStorageGet -run=^$ -benchmem ./storage/
```

### Standard Flags

The project uses these standard flags for consistency:

- `-bench=.` - Run all benchmarks (or specify pattern)
- `-run=^$` - Don't run tests, only benchmarks
- `-benchmem` - Report memory allocations
- `-count=5` - Run each benchmark 5 times for stability
- `-benchtime=3s` - Run each benchmark for at least 3 seconds

Example:

```bash
go test -bench=. -run=^$ -benchmem -count=5 -benchtime=3s ./storage/
```

### Using the bench.sh Script

The `scripts/perf/bench.sh` script automates running benchmarks across all packages:

```bash
# Run with default settings (count=5)
./scripts/perf/bench.sh

# Run with custom count
./scripts/perf/bench.sh 10

# Results are saved to artifacts/bench/<package>_bench.txt
```

### Saving Results

Always save benchmark results for comparison:

```bash
go test -bench=. -run=^$ -benchmem -count=5 ./storage/ > artifacts/bench/baseline.txt
```

## Profiling

### CPU Profiling

Generate CPU profile for a specific benchmark:

```bash
go test -bench=BenchmarkStorageSet -run=^$ -cpuprofile=cpu.prof ./storage/
go tool pprof -http=:8080 cpu.prof  # Opens web UI
```

Or use the profile.sh script:

```bash
./scripts/perf/profile.sh ./storage BenchmarkStorageSet cpu
```

### Memory Profiling

Generate heap profile:

```bash
go test -bench=BenchmarkStorageSet -run=^$ -memprofile=mem.prof ./storage/
go tool pprof -http=:8080 mem.prof
```

### Allocation Profiling

Focus on allocation counts rather than memory used:

```bash
go test -bench=BenchmarkStorageSet -run=^$ -memprofile=allocs.prof -memprofilerate=1 ./storage/
go tool pprof -alloc_space -http=:8080 allocs.prof
```

### Using the profile.sh Script

```bash
# CPU profile
./scripts/perf/profile.sh ./storage BenchmarkStorageSet cpu

# Memory profile
./scripts/perf/profile.sh ./storage BenchmarkStorageSet mem

# Allocation profile
./scripts/perf/profile.sh ./storage BenchmarkStorageSet allocs

# Profiles are saved to artifacts/profiles/
```

### Analyzing Profiles with pprof

Common pprof commands:

```bash
# Top 20 functions by CPU time
go tool pprof -text -nodecount=20 cpu.prof

# Top 20 functions by memory allocation
go tool pprof -text -nodecount=20 -alloc_space mem.prof

# Top 20 functions by allocation count
go tool pprof -text -nodecount=20 -alloc_objects mem.prof

# Generate SVG graph
go tool pprof -svg cpu.prof > cpu_graph.svg

# Interactive mode
go tool pprof cpu.prof
# Then use commands: top, list <function>, web
```

## Comparing Results

### Using benchstat

Install benchstat:

```bash
go install golang.org/x/perf/cmd/benchstat@latest
```

Compare two benchmark runs:

```bash
benchstat baseline.txt current.txt
```

The output shows:

- Performance changes (faster/slower)
- Statistical significance (p-value)
- Memory allocation changes

Example output:

```
name                    old time/op    new time/op    delta
StorageSet-4              250ns ± 2%     200ns ± 3%  -20.00%  (p=0.000 n=10+10)

name                    old alloc/op   new alloc/op   delta
StorageSet-4               128B ± 0%       64B ± 0%  -50.00%  (p=0.000 n=10+10)

name                    old allocs/op  new allocs/op  delta
StorageSet-4               4.00 ± 0%      2.00 ± 0%  -50.00%  (p=0.000 n=10+10)
```

### Using the compare.sh Script

```bash
./scripts/perf/compare.sh artifacts/bench/baseline.txt artifacts/bench/current.txt
# Results saved to artifacts/bench/benchstat.diff
```

### Interpreting p-values

- p < 0.05: Change is statistically significant
- p >= 0.05: Change might be noise
- n=10+10: Number of samples from each run

## Environment Best Practices

### Reduce Variability

1. **CPU Frequency Scaling** (Linux):
   ```bash
   # Disable CPU frequency scaling
   sudo cpupower frequency-set --governor performance
   
   # Check current governor
   cat /sys/devices/system/cpu/cpu*/cpufreq/scaling_governor
   ```

2. **Isolate CPU Cores** (Advanced):
   ```bash
   # Pin process to specific CPU
   taskset -c 0 go test -bench=.
   ```

3. **Close Other Applications**:
   - Close browsers, IDEs, and background processes
   - Disable system updates during benchmarking

4. **Use Consistent Environment Variables**:
   ```bash
   GOMAXPROCS=1 go test -bench=.  # Single-threaded
   GOGC=100 go test -bench=.      # Default GC behavior
   ```

### Benchmark Configuration

1. **Run Multiple Iterations**:
   ```bash
   go test -bench=. -count=10  # More reliable results
   ```

2. **Sufficient Benchmark Time**:
   ```bash
   go test -bench=. -benchtime=10s  # Longer runs for small benchmarks
   ```

3. **Warm Up**:
   - First iteration may be slower due to cold caches
   - Use `-count=5` or higher to account for warm-up

### CI/CD Considerations

When running benchmarks in CI:

- Use dedicated runners when possible
- Fix GOMAXPROCS to reduce variability
- Don't fail builds on performance changes (use as informational)
- Store results as artifacts for historical analysis
- Run on schedule (e.g., weekly) rather than on every commit

## Subsystem Analysis Checklist

### RESP Parser/Writer

**Benchmarks to Run**:
- `BenchmarkRESPParser` - Parsing different RESP types
- `BenchmarkRESPWriter` - Writing responses

**What to Measure**:
- ns/op for parsing bulk strings, arrays, integers
- Allocations per operation
- Impact of buffer sizes

**Common Optimizations**:
- Pre-allocate buffers
- Reduce string conversions
- Use buffer pooling

### RDB Ingest

**Benchmarks to Run**:
- `BenchmarkRDBIngest` - Loading synthetic RDB data

**What to Measure**:
- Throughput (entries/second)
- Memory allocations during load
- Peak memory usage

**Common Optimizations**:
- Batch operations
- Pre-allocate maps with known sizes
- Optimize string/byte slice conversions

### Storage Operations

**Benchmarks to Run**:
- `BenchmarkStorageGet` - Get with hit/miss scenarios
- `BenchmarkStorageSet` - Set with/without TTL
- `BenchmarkStorageSetWithVaryingSize` - Different payload sizes
- `BenchmarkExpirationCleanup` - Expired key removal

**What to Measure**:
- Operation latency (ns/op)
- Allocations per operation
- Lock contention (if concurrent)

**Common Optimizations**:
- Reduce lock granularity
- Use sync.Pool for temporary objects
- Optimize TTL data structures

### Pattern Matching

**Benchmarks to Run**:
- `BenchmarkMatchPatternSimple` - Simple patterns (exact match, prefix)
- `BenchmarkMatchPatternGlob` - Glob patterns
- `BenchmarkMatchPatternRegex` - Complex regex patterns
- `BenchmarkMatchPatternAutomaton` - Automaton strategy

**What to Measure**:
- Match time for different pattern types
- Memory allocations
- Strategy selection overhead

**Common Optimizations**:
- Cache compiled patterns
- Use simpler strategies when possible
- Optimize glob matching

### Expiration Cleanup

**Benchmarks to Run**:
- `BenchmarkCleanupIncremental` - Incremental cleanup
- `BenchmarkCleanupConcurrentAccess` - Cleanup under load

**What to Measure**:
- Cleanup throughput (keys/second)
- Latency impact on operations
- Memory reclamation effectiveness

**Common Optimizations**:
- Tune cleanup batch sizes
- Adjust cleanup frequency
- Use time-based budgets

### Lua Engine

**Benchmarks to Run**:
- `BenchmarkLuaEngine_SimpleScript` - Basic scripts
- `BenchmarkLuaEngine_LoadScript` - Script compilation
- `BenchmarkLuaEngine_EvalSHA` - Cached script execution
- `BenchmarkLuaEngine_MemoryUsage` - Many script scenarios

**What to Measure**:
- Script execution time
- Compilation overhead
- Cache effectiveness
- Memory per script

**Common Optimizations**:
- Cache compiled scripts by SHA
- Limit script cache size
- Optimize Lua-Go data conversions

## Interpreting Results

### Understanding ns/op

- Lower is better
- Context matters: 100ns for a cache operation is different than 100ns for a network operation
- Compare against baseline, not absolute values

### Understanding Allocations

**alloc/op** - Bytes allocated per operation:
- Target: 0 for hot paths
- Each allocation has overhead (GC pressure)

**allocs/op** - Number of allocations per operation:
- Target: 0-2 for most operations
- More important than total bytes for GC pressure

### Red Flags

- **High allocations in hot paths**: Every allocation adds GC pressure
- **String conversions**: `string(bytes)` creates a copy
- **Interface conversions**: Can cause allocations
- **Small allocations**: Many small allocations worse than few large ones
- **Growing slices**: Pre-allocate when size is known

## Common Optimization Patterns

### 1. Pre-allocate Slices

**Before**:
```go
var results []string
for _, item := range items {
    results = append(results, process(item))
}
```

**After**:
```go
results := make([]string, 0, len(items))
for _, item := range items {
    results = append(results, process(item))
}
```

### 2. Use sync.Pool for Temporary Objects

**Before**:
```go
func process() {
    buf := make([]byte, 1024)
    // use buf
}
```

**After**:
```go
var bufPool = sync.Pool{
    New: func() interface{} {
        return make([]byte, 1024)
    },
}

func process() {
    buf := bufPool.Get().([]byte)
    defer bufPool.Put(buf)
    // use buf
}
```

### 3. Avoid String Conversions in Hot Paths

**Before**:
```go
func match(key []byte, pattern string) bool {
    return strings.HasPrefix(string(key), pattern)
}
```

**After**:
```go
func match(key []byte, pattern string) bool {
    return bytes.HasPrefix(key, []byte(pattern)) // Better: cache []byte(pattern)
}
```

### 4. Use String Builder for Concatenation

**Before**:
```go
var result string
for _, s := range items {
    result += s + ":"
}
```

**After**:
```go
var builder strings.Builder
for _, s := range items {
    builder.WriteString(s)
    builder.WriteByte(':')
}
result := builder.String()
```

### 5. Batch Operations

**Before**:
```go
for _, key := range keys {
    store.Delete(key)
}
```

**After**:
```go
store.DeleteBatch(keys)  // Single lock acquisition
```

## Continuous Monitoring

### Establishing Baselines

1. Run benchmarks on main branch:
   ```bash
   git checkout main
   ./scripts/perf/bench.sh
   cp -r artifacts/bench artifacts/bench-baseline
   ```

2. Run benchmarks on feature branch:
   ```bash
   git checkout feature-branch
   ./scripts/perf/bench.sh
   ```

3. Compare:
   ```bash
   ./scripts/perf/compare.sh artifacts/bench-baseline/storage_bench.txt artifacts/bench/storage_bench.txt
   ```

### Setting Performance Goals

Example goals:
- Storage Get: < 100ns/op, 0 allocs/op
- Storage Set: < 300ns/op, < 2 allocs/op
- Pattern Match (simple): < 50ns/op, 0 allocs/op
- Lua Script Exec: < 10µs/op

### Regression Detection

Monitor these metrics:
- **> 20% slowdown**: Investigate immediately
- **> 50% increase in allocations**: Check for new allocations in hot paths
- **New allocations in zero-allocation paths**: Regression

## Additional Resources

- [Go Blog: Profiling Go Programs](https://go.dev/blog/pprof)
- [Effective Go: Benchmarks](https://go.dev/doc/effective_go#benchmark)
- [benchstat documentation](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat)
- [Go Performance Wiki](https://github.com/golang/go/wiki/Performance)

## Summary

This performance audit infrastructure provides:
- ✅ Repeatable benchmarking methodology
- ✅ Profiling tools for CPU, memory, and allocations
- ✅ Statistical comparison with benchstat
- ✅ Best practices for reliable measurements
- ✅ Subsystem-specific analysis guidance
- ✅ Common optimization patterns

Use this guide to measure first, then optimize based on data.
