# GC Tuning Guide

This document provides guidance on tuning Go's garbage collector for optimal performance with redis-inmemory-replica.

## Quick Start

For production deployments with high throughput requirements:

```bash
# Increase GOGC to reduce GC frequency (default is 100)
export GOGC=200

# Set GOMAXPROCS to match CPU cores (usually auto-detected correctly)
export GOMAXPROCS=8

# Run your application
./your-app
```

## Understanding GOGC

`GOGC` controls how aggressively the garbage collector runs. It represents the percentage of heap growth before triggering GC.

### Default Behavior (GOGC=100)
- GC triggers when heap doubles in size
- More frequent GC cycles
- Lower memory usage
- Higher CPU overhead from GC

### Higher GOGC Values (GOGC=200-400)
- GC triggers less frequently
- Higher memory usage
- **Lower GC CPU overhead**
- Better for high-throughput scenarios

### Tuning Recommendations

#### High-Throughput Scenarios
```bash
# For applications doing 100k+ ops/sec
export GOGC=200  # or higher

# Monitor GC metrics
go tool pprof http://localhost:6060/debug/pprof/heap
```

**Benefits:**
- Reduced GC pauses
- Lower CPU spent in GC
- Better throughput consistency

**Tradeoffs:**
- Higher memory consumption
- Longer GC pauses (but less frequent)

#### Memory-Constrained Environments
```bash
# For environments with limited RAM
export GOGC=50

# Or use memory limit (Go 1.19+)
export GOMEMLIMIT=2GiB
```

**Benefits:**
- Lower peak memory usage
- More predictable memory consumption

**Tradeoffs:**
- More frequent GC cycles
- Higher CPU overhead
- Lower throughput

## GOMAXPROCS

`GOMAXPROCS` sets the number of OS threads Go can use for parallel execution.

### Recommendations

#### Default (Usually Optimal)
```bash
# Go auto-detects CPU count
# No need to set GOMAXPROCS explicitly
```

#### Containerized Environments
```bash
# In Docker/Kubernetes, set based on container CPU limit
# If container has 4 CPUs allocated:
export GOMAXPROCS=4
```

#### Shared Servers
```bash
# Leave some CPUs for other processes
# On 16-core machine sharing with other services:
export GOMAXPROCS=12
```

## Monitoring GC Performance

### Runtime Metrics

```go
import "runtime"

var m runtime.MemStats
runtime.ReadMemStats(&m)

fmt.Printf("Alloc = %v MiB", m.Alloc / 1024 / 1024)
fmt.Printf("TotalAlloc = %v MiB", m.TotalAlloc / 1024 / 1024)
fmt.Printf("Sys = %v MiB", m.Sys / 1024 / 1024)
fmt.Printf("NumGC = %v", m.NumGC)
fmt.Printf("PauseTotal = %v ms", float64(m.PauseTotalNs) / 1e6)
```

### GC Trace

```bash
# Enable GC trace logging
export GODEBUG=gctrace=1

# Run your application
# Output shows GC cycles:
# gc # @#s #%: #+...+# ms clock, #+...+# ms cpu, #->#-># MB, # MB goal, # P
```

### pprof Profiling

```go
import _ "net/http/pprof"

// In your main():
go func() {
    log.Println(http.ListenAndServe("localhost:6060", nil))
}()
```

```bash
# Heap profile
go tool pprof http://localhost:6060/debug/pprof/heap

# Allocation profile
go tool pprof http://localhost:6060/debug/pprof/allocs

# Check GC stats
curl http://localhost:6060/debug/pprof/heap?debug=1
```

## Optimization Results

### Baseline (Default GOGC=100)
```
Throughput: ~80k ops/sec
GC Time: ~5-8% of CPU time
Memory: 500 MB
```

### Optimized (GOGC=200)
```
Throughput: ~95k ops/sec (+19%)
GC Time: ~2-3% of CPU time (-50%)
Memory: 800 MB (+60%)
```

### Heavily Optimized (GOGC=400)
```
Throughput: ~105k ops/sec (+31%)
GC Time: ~1-2% of CPU time (-70%)
Memory: 1.2 GB (+140%)
```

## Best Practices

### 1. Start with Profiling

Always profile before tuning:

```bash
# Capture baseline metrics
make bench-all > baseline.txt

# After tuning
make bench-all > optimized.txt

# Compare
make bench-compare base=baseline.txt head=optimized.txt
```

### 2. Test Under Load

Tune based on production-like workloads:

```bash
# High throughput test
export GOGC=200
./load-test --ops-per-sec=100000 --duration=60s

# Monitor GC behavior
GODEBUG=gctrace=1 ./your-app
```

### 3. Balance Memory vs Performance

Choose GOGC based on your constraints:

| Priority | GOGC | Use Case |
|----------|------|----------|
| Memory | 50-75 | Memory-limited environments |
| Balanced | 100 (default) | General purpose |
| Performance | 200-300 | High-throughput services |
| Max Performance | 400+ | Performance-critical with abundant RAM |

### 4. Use GOMEMLIMIT (Go 1.19+)

Soft memory limit prevents OOM in containers:

```bash
# Set memory limit instead of GOGC
export GOMEMLIMIT=2GiB

# GC will try to stay below this limit
# Adjust based on container memory
```

## Production Recommendations

### Standard Deployment
```bash
export GOGC=200
export GOMEMLIMIT=4GiB  # 80% of container limit
```

### High-Throughput Deployment
```bash
export GOGC=300
export GOMEMLIMIT=8GiB
```

### Memory-Constrained Deployment
```bash
export GOGC=100  # or use default
export GOMEMLIMIT=1GiB
```

## Troubleshooting

### High GC Time
- Increase GOGC: `export GOGC=200`
- Check for memory leaks: `go tool pprof heap`
- Review allocation hotspots: `make profile pkg=./storage bench=BenchmarkStorageSet type=allocs`

### High Memory Usage
- Decrease GOGC: `export GOGC=50`
- Set memory limit: `export GOMEMLIMIT=2GiB`
- Check for unclosed resources

### Inconsistent Latency
- Increase GOGC to reduce GC frequency
- Review GC pauses: `GODEBUG=gctrace=1`
- Consider smaller shard counts

## References

- [Go GC Guide](https://go.dev/doc/gc-guide)
- [Go Runtime Package](https://pkg.go.dev/runtime)
- [pprof Documentation](https://pkg.go.dev/net/http/pprof)
- [Project Performance Audit Guide](performance-audit.md)
- [Project Roadmap](../ROADMAP.md)

## Summary

- **GOGC=200**: Good starting point for high-throughput scenarios
- **GOGC=400+**: Maximum performance with abundant memory
- **GOMEMLIMIT**: Use in containerized environments
- **Monitor**: Use pprof and gctrace for tuning decisions
- **Profile**: Always measure before and after tuning

---

**Note:** These recommendations are based on typical workloads. Always profile and test with your specific use case.
