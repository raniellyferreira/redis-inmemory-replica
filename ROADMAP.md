# Performance Optimization Roadmap

## Context

This roadmap outlines a structured approach to bring the performance of this library to a level comparable to (or exceeding) Redis/rueidis (reference: https://github.com/redis/rueidis), with a focus on reducing allocations, optimizing hot paths, and reducing GC pressure.

All work is organized in phases with defined metrics and reproducible benchmarking infrastructure.

## Objectives

### 1. Reduce General Allocations (allocs/op and B/op)

**Target Areas:**
- Map hotspots using pprof (-alloc_space, -alloc_objects) and -benchmem
- Eliminate unnecessary []byte ↔ string conversions in internal APIs
- Reuse buffers/slices with sync.Pool where demonstrably beneficial
- Avoid heap escapes in interfaces/closures on hot paths

**Methodology:**
- Profile with `make profile pkg=./storage bench=BenchmarkStorageSet type=allocs`
- Measure before/after with benchstat
- Target: Zero allocations on cache hit paths

### 2. Optimize RESP Parser (Hot Path)

**Target Areas:**
- Avoid bytes.Split/bytes.Buffer where they generate copies; prefer manual parsing on a single buffer
- Pre-allocate argument slices (capacity based on command patterns) and reuse via pool
- Minimize number conversions (e.g., fast parsing for small integers)
- Keep []byte when possible instead of building intermediate strings

**Current Metrics (Baseline):**
```
BenchmarkReaderParseCommand/GET-4     1000000    1152 ns/op    4917 B/op    12 allocs/op
BenchmarkReaderParseCommand/SET-4      877587    1245 ns/op    5008 B/op    15 allocs/op
```

**Target Improvements:**
- 30-50% reduction in allocs/op for common commands
- 10-25% reduction in ns/op

### 3. Optimize Storage Shard Selection Algorithm

**Current Implementation:**
- Already using power-of-2 shard count with bitmask (implemented)
- Using hash/maphash for distribution

**Potential Improvements:**
- Consider xxhash for faster hash computation
- Avoid recomputing hash/index when key flows through pipeline (cache inline when safe)
- Benchmark current vs xxhash implementation

**Current Metrics:**
```
BenchmarkConcurrentGet/256Shards-50Goroutines-4    11416401    99.04 ns/op    27 B/op    2 allocs/op
BenchmarkStorageGet/Hit_Small-4                    22359874    53.07 ns/op     5 B/op    1 allocs/op
```

### 4. Improve RDB Parsing Throughput

**Target Areas:**
- Reuse decoding buffers and reduce copies during LZF decompression when applicable
- Apply batching per shard to reduce lock contention when inserting many keys
- Pre-size structures (maps/slices) when size is predictable

**Current Metrics:**
```
BenchmarkRDBIngest/Small_10keys_16B-4          Baseline TBD
BenchmarkRDBIngest/Medium_100keys_1KB-4        Baseline TBD
BenchmarkRDBIngest/Large_1000keys_1KB-4        Baseline TBD
```

**Target Improvements:**
- 20-40% reduction in allocations during parsing
- Batch inserts to reduce shard lock overhead

### 5. Fine-tune Lua Script Cache

**Current Status:**
- Basic SHA-based cache implemented
- No size limit or eviction policy

**Planned Improvements:**
- Implement cache size limit with LRU/ARC eviction policy
- Add hit-rate metrics
- Reuse Lua context/stack when possible to reduce transient allocations in Eval/EvalSHA
- Benchmark cache hit vs miss performance

**Current Metrics:**
```
BenchmarkLuaEngine_EvalSHA-4              13088    92695 ns/op    219217 B/op    868 allocs/op
BenchmarkLuaEngine_SimpleScript-4         14030    86945 ns/op    219216 B/op    868 allocs/op
```

**Target:**
- Reduce allocations by 20-30% on cached script execution
- Implement bounded cache with metrics

### 6. Reduce GC Pressure in High-Throughput Scenarios

**Strategies:**
- Reduce churn of short-lived objects with targeted pooling
- Avoid allocations in logs on hot paths (use lazy logging/guard clauses)
- Document GOGC and GOMAXPROCS tuning (as configuration guidance, not runtime fixation)

## Metrics and Initial Targets

### Baseline Metrics (Current)

For detailed baseline metrics across all subsystems, see [docs/baseline-metrics.md](docs/baseline-metrics.md).

**Storage Operations:**
```
BenchmarkStorageGet/Hit_Small-4       22359874    53.07 ns/op     5 B/op    1 allocs/op
BenchmarkStorageSet/Small_16B-4        3968053   303.4 ns/op   101 B/op    4 allocs/op
```

**RESP Parsing:**
```
BenchmarkReaderParseCommand/SET-4      877587    1245 ns/op    5008 B/op   15 allocs/op
BenchmarkReaderParseBatch/BatchSET_10-4 209374   5511 ns/op    7312 B/op  114 allocs/op
```

**RDB Ingest:**
```
BenchmarkRDBIngest/Medium_100keys_1KB-4   Baseline documented (currently implemented)
```

**Lua Execution:**
```
BenchmarkLuaEngine_SimpleScript-4      14030     86945 ns/op   219216 B/op  868 allocs/op
BenchmarkLuaEngine_EvalSHA-4           13088     92695 ns/op   219217 B/op  868 allocs/op
```

### Target Improvements

- **30-50% reduction** in allocs/op in main benchmarks (storage Get/Set, RESP parse, pattern matching)
- **10-25% improvement** in ns/op in the same scenarios after eliminating copies and tuning pools
- **Approach rueidis numbers** in RESP parsing/encoding for equivalent microbenchmarks (as reference, not 1:1 functional commitment)

## Execution Plan by Phases

### Phase A — Diagnostics and Guardrails

**Status: ✅ COMPLETE**

- ✅ Add/update microbenchmarks with -benchmem covering:
  - ✅ Storage: Get/Set with/without TTL and small/medium/large payloads; hits/misses
  - ✅ RESP parser/writer: Common commands in batch; arrays and bulk strings
  - ✅ RDB ingest synthetic (non-I/O), focused on parsing and insertion cost
  - ✅ Lua: Load and Eval/EvalSHA with different cardinalities and KEYS/ARGV
- ✅ Performance audit infrastructure in place:
  - ✅ `scripts/perf/bench.sh` - Comprehensive benchmark runner
  - ✅ `scripts/perf/profile.sh` - CPU/memory/allocation profiling
  - ✅ `scripts/perf/compare.sh` - Statistical comparison with benchstat
  - ✅ Documentation in `docs/performance-audit.md`

### Phase B — Tactical Interventions (Incremental and Testable)

**Status: 🔄 PLANNED**

Each item will be implemented in focused PRs/commits with before/after measurements:

#### B1. RESP Parser Optimization
- [ ] Remove avoidable allocations in readLine/readArray
- [ ] Implement argument buffer pools
- [ ] Manual parsing instead of bytes.Split where beneficial
- [ ] Fast path for small integers
- **Target:** 30-50% reduction in allocs/op for common commands

#### B2. Storage Shard Optimization
- [ ] Benchmark current hash/maphash vs xxhash
- [ ] Implement hash caching where safe in pipeline
- [ ] Evaluate if further optimizations are needed (already using bitmask)
- **Target:** Maintain or improve current performance

#### B3. RDB Parser Optimization
- [ ] Implement batching per shard
- [ ] Add reusable buffer pools for decoding
- [ ] Pre-size maps when count is known from RDB metadata
- **Target:** 20-40% reduction in allocations during bulk import

#### B4. Lua Cache Enhancement
- [ ] Implement bounded cache with size limit
- [ ] Add LRU eviction policy
- [ ] Add hit-rate metrics
- [ ] Explore context/stack reuse
- **Target:** 20-30% reduction in allocations on cached execution

#### B5. GC Pressure Reduction
- [ ] Audit and add pooling for frequently allocated objects
- [ ] Add lazy logging guards on hot paths
- [ ] Document GOGC tuning recommendations
- **Target:** Reduce overall GC time by 15-25%

### Phase C — Validation and Regression Testing

**Status: 🔄 ONGOING**

For each optimization:
- [ ] Compare baseline vs head with benchstat
- [ ] Document gains/losses per benchmark
- [ ] Run tests with -race
- [ ] Ensure no functional regressions
- [ ] Update benchmarks and documentation

## Deliverables

### This PR

- ✅ Detailed roadmap (this document)
- ✅ Comprehensive benchmark coverage
- ✅ Performance audit infrastructure and scripts
- ✅ Baseline metrics documented
- ⚠️ No behavioral changes in this phase

### Future PRs

Each optimization area will have dedicated PRs with:
- Before/after benchmark results
- Profiling data showing improvements
- Test coverage for new optimizations
- Documentation updates

## Benchmark Coverage Checklist

### Storage Operations
- ✅ Get with hits (small/medium/large payloads)
- ✅ Get with misses
- ✅ Set with different payload sizes
- ✅ Set with TTL
- ✅ Delete operations
- ✅ Concurrent read/write scenarios
- ✅ Expiration checks

### RESP Parser/Writer
- ✅ Parse simple strings, errors, integers
- ✅ Parse bulk strings (various sizes)
- ✅ Parse arrays (empty, small, medium)
- ✅ Parse common commands (GET, SET, etc.)
- ✅ Write operations for all types
- ✅ Command encoding

### RDB Ingest
- ✅ Synthetic RDB parsing (various key counts)
- ✅ Different value sizes
- ✅ String types
- ✅ Lists parsing
- ✅ Entries with expiry
- ✅ Auxiliary fields

### Lua Engine
- ✅ Simple script execution
- ✅ Scripts with KEYS/ARGV
- ✅ Redis command calls from Lua
- ✅ EvalSHA (cached scripts)
- ✅ Complex scripts with loops
- ✅ Array operations
- ✅ String manipulation
- ✅ LoadScript/ScriptExists/ScriptFlush
- ✅ Parallel execution
- ✅ Memory usage patterns

### Pattern Matching
- ✅ Simple patterns
- ✅ Regex patterns
- ✅ Glob patterns
- ✅ Automaton patterns

### Expiration Cleanup
- ✅ Active expiration benchmarks
- ✅ Concurrent expiration scenarios

### Sharding
- ✅ Different shard counts (1, 16, 256)
- ✅ Concurrent access patterns
- ✅ Key aggregation
- ✅ Shard contention

## Risks and Considerations

### Potential Issues

1. **Pool Contention**
   - Pools can degrade performance if poorly sized
   - Risk of false sharing in concurrent scenarios
   - **Mitigation:** Profile before and after, use benchstat for statistical significance

2. **Hash/Sharding Changes**
   - Changes to hashing/sharding may break internal compatibility
   - **Mitigation:** Evaluate migration strategy, maintain backward compatibility where possible

3. **Premature Optimization**
   - Risk of micro-optimizations without measured benefit
   - **Mitigation:** All optimizations must be backed by profiling data and benchmark improvements

4. **Maintenance Burden**
   - Complex optimizations may reduce code clarity
   - **Mitigation:** Document optimizations, add comments, maintain test coverage

### Performance Tradeoffs

- **Memory vs Speed:** Some optimizations may increase memory usage (pools, caches)
- **Code Complexity:** Optimized code may be less readable
- **Maintenance:** Need to balance performance with maintainability

## Reference Implementations

- **rueidis:** https://github.com/redis/rueidis (RESP performance reference)
- **pprof:** Standard Go profiling tool
- **benchstat:** Statistical benchmark comparison tool

## Acceptance Criteria

### For This Roadmap PR
- ✅ Benchmarks and scripts are reproducible and documented
- ✅ Baseline metrics are captured for all critical paths
- ✅ Infrastructure for profiling and comparison is in place
- ✅ Documentation clearly describes the optimization plan

### For Future Optimization PRs
- Measurable reductions in allocs/op on hot paths
- Benchstat comparison showing statistical significance
- No regressions in functional tests (with -race)
- Public API behavior remains stable
- Documentation updated with performance characteristics

## Quick Start

### Running Benchmarks

```bash
# Run all benchmarks
make bench-all

# Profile a specific area
make profile pkg=./storage bench=BenchmarkStorageGet type=allocs

# Compare two runs
make bench-compare base=baseline.txt head=current.txt
```

### Analyzing Results

```bash
# View comprehensive results
cat artifacts/bench/latest.txt

# Interactive profiling
go tool pprof artifacts/profiles/<profile>.prof

# Web-based profiling
go tool pprof -http=:8080 artifacts/profiles/<profile>.prof
```

## Notes

- Items marked as **[Planned]** are proposals that must be validated by experimentation and profiling before implementation
- All metrics should be measured on the same hardware for consistency
- CI/CD integration will track performance regressions over time
- This is a living document that will be updated as work progresses

## Timeline

- **Phase A:** Complete (infrastructure and baseline)
- **Phase B:** To be scheduled in focused PRs (Q1-Q2)
- **Phase C:** Ongoing with each optimization

---

**Last Updated:** 2025-10-11  
**Status:** Phase A Complete, Phase B Planned
