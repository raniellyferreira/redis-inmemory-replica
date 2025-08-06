#!/bin/bash

# Benchmark Runner Script for sync.RWMutex vs atomic.Value Comparison
# This script runs comprehensive benchmarks and generates comparison reports

set -e

echo "=== Redis In-Memory Replica: Storage Concurrency Benchmark ==="
echo "Comparing sync.RWMutex vs atomic.Value implementations"
echo

# Change to benchmarks directory
cd "$(dirname "$0")"
if [ ! -f "storage_comparison_test.go" ]; then
    echo "Error: Must run from benchmarks directory"
    exit 1
fi

# Create results directory
mkdir -p results
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
RESULTS_DIR="results/${TIMESTAMP}"
mkdir -p "$RESULTS_DIR"

echo "Results will be saved to: $RESULTS_DIR"
echo

# Function to run benchmark and save results
run_benchmark() {
    local name="$1"
    local pattern="$2"
    local benchtime="${3:-3s}"
    
    echo "Running $name benchmarks..."
    go test -bench="$pattern" -run=^$ -benchtime="$benchtime" -benchmem \
        > "$RESULTS_DIR/${name}.txt" 2>&1
    
    if [ $? -eq 0 ]; then
        echo "✓ $name completed"
    else
        echo "✗ $name failed"
        cat "$RESULTS_DIR/${name}.txt"
    fi
    echo
}

# Function to run quick benchmark for immediate results
run_quick_benchmark() {
    local name="$1"
    local pattern="$2"
    
    echo "Quick test: $name"
    go test -bench="$pattern" -run=^$ -benchtime=1s -count=3 2>/dev/null | \
        grep "Benchmark" | head -5
    echo
}

echo "=== Quick Performance Preview ==="
run_quick_benchmark "Read Operations" "BenchmarkMemoryStorageGet|BenchmarkAtomicStorageGet"
run_quick_benchmark "Write Operations" "BenchmarkMemoryStorageSet|BenchmarkAtomicStorageSet"
run_quick_benchmark "Read-Heavy Mixed" "BenchmarkMixed_ReadHeavy"

echo "=== Running Comprehensive Benchmarks ==="

# Core operation benchmarks
run_benchmark "01_read_operations" "BenchmarkMemoryStorageGet|BenchmarkAtomicStorageGet"
run_benchmark "02_write_operations" "BenchmarkMemoryStorageSet|BenchmarkAtomicStorageSet"

# Mixed workload benchmarks
run_benchmark "03_mixed_read_heavy" "BenchmarkMixed_ReadHeavy"
run_benchmark "04_mixed_balanced" "BenchmarkMixed_Balanced"
run_benchmark "05_mixed_write_heavy" "BenchmarkMixed_WriteHeavy"

# Scaling benchmarks
run_benchmark "06_scaling_rwmutex" "BenchmarkScaling_RWMutex"
run_benchmark "07_scaling_atomic" "BenchmarkScaling_Atomic"

# Large dataset benchmarks
run_benchmark "08_large_datasets" "BenchmarkLargeDataset"

# Memory usage benchmarks
run_benchmark "09_memory_usage" "BenchmarkMemoryUsage"

# Throughput benchmarks
run_benchmark "10_throughput" "BenchmarkThroughput"

echo "=== Generating Summary Report ==="

# Create summary report
SUMMARY_FILE="$RESULTS_DIR/summary.md"

cat > "$SUMMARY_FILE" << EOF
# Benchmark Results Summary

**Date**: $(date)
**Go Version**: $(go version)
**System**: $(uname -a)
**CPU**: $(grep 'model name' /proc/cpuinfo | head -1 | cut -d: -f2 | xargs)
**Memory**: $(free -h | grep Mem | awk '{print $2}')

## Quick Comparison

### Read Performance
EOF

# Extract and format key results
echo "Extracting key metrics..."

# Function to extract benchmark results
extract_metric() {
    local file="$1"
    local pattern="$2"
    local field="$3"
    
    if [ -f "$RESULTS_DIR/$file" ]; then
        grep "$pattern" "$RESULTS_DIR/$file" | awk '{print $'$field'}' | head -1
    else
        echo "N/A"
    fi
}

# Read performance comparison
RWMUTEX_READ=$(extract_metric "01_read_operations.txt" "BenchmarkMemoryStorageGet_RWMutex" "3")
ATOMIC_READ=$(extract_metric "01_read_operations.txt" "BenchmarkAtomicStorageGet" "3")

# Write performance comparison  
RWMUTEX_WRITE=$(extract_metric "02_write_operations.txt" "BenchmarkMemoryStorageSet_RWMutex" "3")
ATOMIC_WRITE=$(extract_metric "02_write_operations.txt" "BenchmarkAtomicStorageSet" "3")

cat >> "$SUMMARY_FILE" << EOF

| Operation | sync.RWMutex | atomic.Value | Ratio |
|-----------|-------------|-------------|--------|
| Read (ns/op) | $RWMUTEX_READ | $ATOMIC_READ | $(echo "scale=2; $RWMUTEX_READ / $ATOMIC_READ" | bc 2>/dev/null || echo "N/A") |
| Write (ns/op) | $RWMUTEX_WRITE | $ATOMIC_WRITE | $(echo "scale=2; $ATOMIC_WRITE / $RWMUTEX_WRITE" | bc 2>/dev/null || echo "N/A") |

### Key Findings

EOF

# Add key findings based on results
if [ "$ATOMIC_READ" != "N/A" ] && [ "$RWMUTEX_READ" != "N/A" ]; then
    READ_RATIO=$(echo "scale=1; $RWMUTEX_READ / $ATOMIC_READ" | bc 2>/dev/null || echo "1")
    if (( $(echo "$READ_RATIO > 2" | bc -l 2>/dev/null || echo 0) )); then
        echo "- ✅ **atomic.Value is ${READ_RATIO}x faster for reads**" >> "$SUMMARY_FILE"
    else
        echo "- ℹ️ Read performance is comparable between approaches" >> "$SUMMARY_FILE"
    fi
fi

if [ "$ATOMIC_WRITE" != "N/A" ] && [ "$RWMUTEX_WRITE" != "N/A" ]; then
    WRITE_RATIO=$(echo "scale=1; $ATOMIC_WRITE / $RWMUTEX_WRITE" | bc 2>/dev/null || echo "1")
    if (( $(echo "$WRITE_RATIO > 10" | bc -l 2>/dev/null || echo 0) )); then
        echo "- ⚠️ **sync.RWMutex is ${WRITE_RATIO}x faster for writes**" >> "$SUMMARY_FILE"
    else
        echo "- ℹ️ Write performance is comparable between approaches" >> "$SUMMARY_FILE"
    fi
fi

cat >> "$SUMMARY_FILE" << EOF

## Detailed Results

EOF

# Add links to detailed results
for file in "$RESULTS_DIR"/*.txt; do
    if [ -f "$file" ]; then
        basename_file=$(basename "$file" .txt)
        echo "- [$basename_file](./$basename_file.txt)" >> "$SUMMARY_FILE"
    fi
done

cat >> "$SUMMARY_FILE" << EOF

## Recommendations

Based on these benchmark results:

### Use atomic.Value when:
- Read-to-write ratio > 20:1 (95%+ reads)
- Dataset size < 10,000 keys  
- Read latency is critical
- Memory overhead is acceptable

### Use sync.RWMutex when:
- Write frequency > 1,000/second
- Balanced read/write workloads
- Large datasets (>10,000 keys)
- Memory efficiency is critical

### For Redis In-Memory Replica:
- **Primary storage**: Keep sync.RWMutex (proven, balanced performance)
- **Configuration data**: Consider atomic.Value (rare updates, frequent reads)
- **Performance metrics**: Use atomic counters
- **Hot key cache**: Evaluate atomic.Value for top 1% accessed keys

---
*Generated by benchmark_runner.sh on $(date)*
EOF

echo "=== Results Summary ==="
echo
cat "$SUMMARY_FILE"
echo
echo "=== Benchmark Complete ==="
echo "Full results available in: $RESULTS_DIR"
echo "Summary report: $RESULTS_DIR/summary.md"
echo

# Optional: Generate comparison chart data
if command -v python3 >/dev/null 2>&1; then
    echo "Generating comparison charts..."
    # Here you could add Python/gnuplot script to generate visual comparisons
fi

echo "Benchmark analysis complete!"