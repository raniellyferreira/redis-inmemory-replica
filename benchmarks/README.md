# Storage Concurrency Benchmarks

This directory contains comprehensive benchmarks comparing `sync.RWMutex` and `atomic.Value` approaches for concurrent data access in the Redis in-memory replica storage.

## Overview

The benchmarks evaluate two different concurrency strategies:

1. **sync.RWMutex** (current implementation): Traditional reader-writer locks
2. **atomic.Value** (experimental): Copy-on-write with lock-free reads

## Quick Start

### Run All Benchmarks
```bash
./run_benchmarks.sh
```

### Run Specific Benchmark Categories
```bash
# Quick comparison of read operations
go test -bench="BenchmarkMemoryStorageGet|BenchmarkAtomicStorageGet" -benchtime=3s

# Write operations comparison
go test -bench="BenchmarkMemoryStorageSet|BenchmarkAtomicStorageSet" -benchtime=3s

# Mixed workload (95% reads, 5% writes)
go test -bench="BenchmarkMixed_ReadHeavy" -benchtime=3s

# Scaling analysis
go test -bench="BenchmarkScaling" -benchtime=2s
```

### Memory Usage Analysis
```bash
go test -bench="BenchmarkMemoryUsage" -benchmem
```

## Benchmark Categories

### 1. Core Operations
- **Read Operations**: `Get()` performance comparison
- **Write Operations**: `Set()` performance comparison  
- **Memory Usage**: Memory efficiency and allocation patterns

### 2. Mixed Workloads
- **Read-Heavy**: 95% reads, 5% writes (typical cache scenario)
- **Balanced**: 80% reads, 20% writes (general purpose)
- **Write-Heavy**: 50% reads, 50% writes (high-update scenario)

### 3. Scalability Analysis
- **Concurrency Scaling**: Performance with 1, 2, 4, 8, 16 goroutines
- **Dataset Size Impact**: Performance with 1K, 10K, 100K keys
- **Throughput Measurement**: Maximum operations per second

### 4. Latency Analysis
- **Percentile Latencies**: P50, P95, P99 response times
- **Contention Impact**: Performance under lock contention
- **Memory Pressure**: Performance with GC pressure

## Benchmark Results Interpretation

### Key Metrics

- **ns/op**: Nanoseconds per operation (lower is better)
- **ops/sec**: Operations per second (higher is better)
- **allocs/op**: Memory allocations per operation (lower is better)
- **B/op**: Bytes allocated per operation (lower is better)

### Expected Performance Characteristics

#### sync.RWMutex
- **Read latency**: 50-100ns under low contention
- **Write latency**: 200-500ns
- **Memory overhead**: Minimal (~100 bytes)
- **Scalability**: Good up to 8-16 cores for reads

#### atomic.Value  
- **Read latency**: 20-40ns (lock-free)
- **Write latency**: 10μs-1ms (depends on dataset size)
- **Memory overhead**: 2x during updates
- **Scalability**: Excellent for reads, poor for writes

## Real-World Usage Scenarios

### Scenario 1: Configuration Cache
**Characteristics**: 99% reads, 1% writes, small dataset
**Recommendation**: atomic.Value
**Expected improvement**: 2-3x faster reads

### Scenario 2: Session Storage  
**Characteristics**: 90% reads, 10% writes, medium dataset
**Recommendation**: sync.RWMutex or hybrid approach
**Reason**: Balanced performance, write efficiency

### Scenario 3: High-Frequency Analytics
**Characteristics**: 70% reads, 30% writes, large dataset
**Recommendation**: sync.RWMutex
**Reason**: Write performance critical, memory efficiency

### Scenario 4: Redis Replica Storage
**Characteristics**: 80% reads, 20% writes, variable dataset
**Recommendation**: sync.RWMutex (current implementation)
**Reason**: Balanced workload, operational simplicity

## Benchmark Environment

### System Requirements
- Go 1.24.5+
- Linux/macOS/Windows
- Minimum 4 CPU cores for meaningful concurrency testing
- 8GB+ RAM for large dataset tests

### Benchmark Configuration
- **Runtime**: 3 seconds per benchmark (configurable)
- **Warmup**: Automatic in go test framework
- **Iterations**: Automatic based on runtime stability
- **CPU cores**: Uses all available cores (`runtime.GOMAXPROCS(0)`)

## Understanding the Results

### When atomic.Value Wins
- **Pure read workloads**: 2-4x faster
- **Read-heavy mixed workloads**: 1.5-2x faster 
- **High concurrency reads**: Better scaling

### When sync.RWMutex Wins
- **Write operations**: 10-1000x faster
- **Balanced workloads**: More consistent performance
- **Large datasets**: Better memory efficiency
- **Complex operations**: Easier to implement correctly

### Memory Usage Patterns
- **sync.RWMutex**: Constant memory usage
- **atomic.Value**: Peaks at 2x during writes, then returns to normal

## Advanced Benchmarking

### Custom Benchmark Parameters
```bash
# Adjust benchmark time
go test -bench=. -benchtime=10s

# Increase memory allocations tracking
go test -bench=. -benchmem -memprofile=mem.prof

# CPU profiling
go test -bench=. -cpuprofile=cpu.prof

# Block profiling (lock contention)
go test -bench=. -blockprofile=block.prof
```

### Analyzing Profiles
```bash
# View CPU profile
go tool pprof cpu.prof

# View memory profile  
go tool pprof mem.prof

# View blocking profile
go tool pprof block.prof
```

## Contributing

### Adding New Benchmarks

1. Follow the naming convention: `Benchmark[Operation]_[Implementation]`
2. Include both memory and CPU measurements
3. Test with multiple dataset sizes
4. Document expected behavior

### Example Benchmark
```go
func BenchmarkNewOperation_RWMutex(b *testing.B) {
    storage := storage.NewMemory()
    
    // Setup phase
    // ... populate test data
    
    b.ResetTimer() // Start measuring from here
    
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            // Operation to benchmark
        }
    })
}
```

## Troubleshooting

### Common Issues

1. **Inconsistent Results**: Ensure system is not under load during benchmarking
2. **Memory Allocation Errors**: Increase available memory or reduce dataset size
3. **Compilation Errors**: Check Go version and dependencies

### Performance Tips

1. **Disable CPU frequency scaling** for consistent results:
   ```bash
   echo performance | sudo tee /sys/devices/system/cpu/cpu*/cpufreq/scaling_governor
   ```

2. **Run on dedicated hardware** for production benchmarks

3. **Multiple runs** for statistical significance:
   ```bash
   go test -bench=. -count=10 > results.txt
   benchstat results.txt
   ```

## References

- [Go Benchmarking Guide](https://golang.org/pkg/testing/#hdr-Benchmarks)
- [sync.RWMutex Documentation](https://golang.org/pkg/sync/#RWMutex)
- [sync/atomic Documentation](https://golang.org/pkg/sync/atomic/)
- [Effective Go - Concurrency](https://golang.org/doc/effective_go.html#concurrency)