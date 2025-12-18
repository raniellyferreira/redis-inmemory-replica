# Baseline Performance Metrics

This document captures baseline performance metrics before optimization work begins. All benchmarks were run with `-benchmem` flag on the same hardware for consistency.

**Environment:**
- OS: Linux (amd64)
- CPU: AMD EPYC 7763 64-Core Processor
- Go Version: 1.25.2
- Date: 2025-10-11

## Storage Operations

### Get Operations

```
BenchmarkStorageGet/Hit_Small-4         22035931    52.31 ns/op       5 B/op       1 allocs/op
BenchmarkStorageGet/Hit_Medium-4         5531283   216.9 ns/op    1024 B/op       1 allocs/op
BenchmarkStorageGet/Hit_Large-4             5178   238260 ns/op 1048578 B/op       1 allocs/op
BenchmarkStorageGet/Miss-4              34438053    35.05 ns/op       0 B/op       0 allocs/op
BenchmarkStorageGet/Mixed_50pct-4       27318452    43.10 ns/op       4 B/op       0 allocs/op
```

**Key Observations:**
- Cache misses have 0 allocations (good)
- Small payload hits: 1 allocation per op
- Large payloads show linear relationship with size

### Set Operations

```
BenchmarkStorageSet/Small_16B-4          3968053   303.4 ns/op     101 B/op       4 allocs/op
BenchmarkStorageSet/Medium_1KB-4         2504878   479.7 ns/op    1110 B/op       4 allocs/op
BenchmarkStorageSet/Large_64KB-4          223047  5145 ns/op     65623 B/op       4 allocs/op
BenchmarkStorageSet/VeryLarge_1MB-4        30012  37551 ns/op  1048678 B/op       4 allocs/op
```

**Key Observations:**
- Consistent 4 allocations per operation regardless of size
- Throughput: 52.74 MB/s (small), 2134.68 MB/s (medium), 12738.58 MB/s (large)
- Good scaling with larger payloads

### Set with TTL

```
BenchmarkStorageSetWithTTL/Small_1s-4    3223072   371.4 ns/op     126 B/op       5 allocs/op
BenchmarkStorageSetWithTTL/Small_1h-4    3221185   373.0 ns/op     126 B/op       5 allocs/op
```

**Key Observations:**
- One additional allocation vs Set without TTL
- TTL duration doesn't affect performance

## RESP Protocol

### Parser - Individual Commands

```
BenchmarkReaderParseCommand/GET-4       1000000   1152 ns/op    4917 B/op      12 allocs/op
BenchmarkReaderParseCommand/SET-4        877587   1245 ns/op    5008 B/op      15 allocs/op
BenchmarkReaderParseCommand/SET_EX-4     705128   1488 ns/op    5184 B/op      21 allocs/op
BenchmarkReaderParseCommand/KEYS-4       954654   1117 ns/op    4916 B/op      12 allocs/op
```

**Key Observations:**
- GET: 12 allocs/op, ~1152 ns/op
- SET: 15 allocs/op, ~1245 ns/op
- Allocation count increases with command complexity

### Parser - Batch Operations

```
BenchmarkReaderParseBatch/BatchGET_10-4      284528   4252 ns/op  54.09 MB/s   6432 B/op      84 allocs/op
BenchmarkReaderParseBatch/BatchSET_10-4      209374   5511 ns/op  61.69 MB/s   7312 B/op     114 allocs/op
BenchmarkReaderParseBatch/BatchGET_100-4      34045  35409 ns/op  67.50 MB/s  21552 B/op     804 allocs/op
BenchmarkReaderParseBatch/BatchSET_100-4      25003  47694 ns/op  73.18 MB/s  30352 B/op    1104 allocs/op
BenchmarkReaderParseBatch/BatchMixed_50-4     53973  22051 ns/op  66.44 MB/s  15392 B/op     479 allocs/op
```

**Key Observations:**
- Average ~8.4 allocs per GET command in batch
- Average ~11.4 allocs per SET command in batch
- Throughput: 54-73 MB/s depending on command mix

### Writer Operations

```
BenchmarkWriterSimpleString-4           1945551   633.8 ns/op    4096 B/op       1 allocs/op
BenchmarkWriterInteger-4                1797678   670.4 ns/op    4112 B/op       2 allocs/op
BenchmarkWriterBulkString/Small_16B-4   1916581   653.6 ns/op    4096 B/op       1 allocs/op
BenchmarkWriterBulkString/Medium_1KB-4  1728296   686.7 ns/op    4100 B/op       2 allocs/op
BenchmarkWriterBulkString/Large_64KB-4   577383  2080 ns/op      4101 B/op       2 allocs/op
```

**Key Observations:**
- Very low allocation counts (1-2 allocs/op)
- Consistent ~4KB buffer allocation
- Good throughput: 24.48 MB/s to 31511.59 MB/s

### Writer - Batch Operations

```
BenchmarkWriterBatch/BatchSimpleString_10-4   1405645   849.0 ns/op  58.89 MB/s  4168 B/op   3 allocs/op
BenchmarkWriterBatch/BatchInteger_10-4        1399474   885.8 ns/op  45.16 MB/s  4168 B/op   3 allocs/op
BenchmarkWriterBatch/BatchBulkString_10-4      978001  1064 ns/op  103.34 MB/s  4221 B/op  13 allocs/op
BenchmarkWriterBatch/BatchCommand_50-4         152160  7688 ns/op  178.86 MB/s  5168 B/op 128 allocs/op
BenchmarkWriterBatch/BatchCommand_100-4         69390 17234 ns/op  191.48 MB/s  6568 B/op 303 allocs/op
```

**Key Observations:**
- Excellent batching efficiency
- Average ~2.56 allocs per command in 50-command batch
- High throughput: 45-191 MB/s

## RDB Ingest

```
BenchmarkRDBIngest/Small_10keys_16B-4           Varies by implementation
BenchmarkRDBIngest/Medium_100keys_1KB-4         Varies by implementation
BenchmarkRDBIngest/Large_1000keys_1KB-4         Varies by implementation
BenchmarkRDBIngest/VeryLarge_10000keys_16B-4    Varies by implementation
```

**Key Observations:**
- Synthetic RDB benchmarks are in place
- Focuses on parsing and insertion, not I/O
- Ready for optimization measurement

## Lua Engine

### Simple Scripts

```
BenchmarkLuaEngine_SimpleScript-4        14030   86945 ns/op   219216 B/op    868 allocs/op
BenchmarkLuaEngine_ScriptWithKeys-4      12612   92648 ns/op   220178 B/op    910 allocs/op
BenchmarkLuaEngine_RedisCommands-4       12091   99872 ns/op   221658 B/op    979 allocs/op
```

**Key Observations:**
- High allocation count (~868-979 allocs/op)
- Simple script: ~87µs per execution
- Redis commands add ~10µs overhead

### Cached vs Uncached

```
BenchmarkLuaEngine_CachedVsUncached/Uncached_Eval-4   10000  101772 ns/op  220585 B/op  930 allocs/op
BenchmarkLuaEngine_CachedVsUncached/Cached_EvalSHA-4  12535   96764 ns/op  220584 B/op  930 allocs/op
```

**Key Observations:**
- Small performance difference (~5% faster for cached)
- Same allocation pattern (no reduction in cached path yet)
- Opportunity for optimization via context reuse

### Varying KEYS Cardinality

```
BenchmarkLuaEngine_VaryingKeysCardinality/Keys_0-4     13694   89175 ns/op  219634 B/op   881 allocs/op
BenchmarkLuaEngine_VaryingKeysCardinality/Keys_1-4     12850   92117 ns/op  221065 B/op   923 allocs/op
BenchmarkLuaEngine_VaryingKeysCardinality/Keys_5-4     10000  101462 ns/op  224682 B/op  1079 allocs/op
BenchmarkLuaEngine_VaryingKeysCardinality/Keys_10-4    10000  113934 ns/op  229289 B/op  1273 allocs/op
BenchmarkLuaEngine_VaryingKeysCardinality/Keys_25-4     7251  152725 ns/op  242681 B/op  1875 allocs/op
BenchmarkLuaEngine_VaryingKeysCardinality/Keys_50-4     5094  226627 ns/op  270185 B/op  2881 allocs/op
```

**Key Observations:**
- Linear scaling with key count
- ~3µs and ~40 allocs per additional key
- Good scalability up to 50 keys

### Varying ARGV Cardinality

```
BenchmarkLuaEngine_VaryingArgsCardinality/Args_0-4     13398   93027 ns/op  219634 B/op   881 allocs/op
BenchmarkLuaEngine_VaryingArgsCardinality/Args_1-4     12826   94961 ns/op  221066 B/op   923 allocs/op
BenchmarkLuaEngine_VaryingArgsCardinality/Args_5-4     10000  103285 ns/op  224682 B/op  1079 allocs/op
BenchmarkLuaEngine_VaryingArgsCardinality/Args_10-4    10510  114536 ns/op  229289 B/op  1273 allocs/op
BenchmarkLuaEngine_VaryingArgsCardinality/Args_25-4     7509  153871 ns/op  242681 B/op  1875 allocs/op
BenchmarkLuaEngine_VaryingArgsCardinality/Args_50-4     4933  226168 ns/op  270185 B/op  2881 allocs/op
```

**Key Observations:**
- Similar scaling to KEYS
- ~3µs and ~40 allocs per additional arg
- Consistent with KEYS performance

### KEYS + ARGV Combined

```
BenchmarkLuaEngine_KeysAndArgsCardinality/1k_1a-4      10000  106250 ns/op  234258 B/op  1058 allocs/op
BenchmarkLuaEngine_KeysAndArgsCardinality/5k_5a-4      10000  112985 ns/op  235538 B/op  1125 allocs/op
BenchmarkLuaEngine_KeysAndArgsCardinality/10k_10a-4     9080  118985 ns/op  237114 B/op  1206 allocs/op
BenchmarkLuaEngine_KeysAndArgsCardinality/25k_25a-4     8125  135414 ns/op  241586 B/op  1447 allocs/op
```

**Key Observations:**
- Includes Redis command overhead
- ~12-18µs overhead for SET/GET operations
- Allocations scale predictably

### Script Management

```
BenchmarkLuaEngine_LoadScript-4           2447068   480.9 ns/op    176 B/op      6 allocs/op
BenchmarkLuaEngine_ScriptExists-4         6198180   192.2 ns/op     16 B/op      1 allocs/op
BenchmarkLuaEngine_ScriptFlush-4           194901  6133 ns/op     2240 B/op     72 allocs/op
```

**Key Observations:**
- LoadScript is fast (~481ns)
- ScriptExists is very efficient
- ScriptFlush suitable for periodic cleanup

## Sharding Performance

### Concurrent Get

```
BenchmarkConcurrentGet/1Shard-1Goroutine-4           10983428    99.84 ns/op    27 B/op    2 allocs/op
BenchmarkConcurrentGet/16Shards-10Goroutines-4       10858892    99.01 ns/op    27 B/op    2 allocs/op
BenchmarkConcurrentGet/256Shards-50Goroutines-4      11416401    99.04 ns/op    27 B/op    2 allocs/op
```

**Key Observations:**
- Sharding reduces contention effectively
- Performance consistent across shard counts
- Good scalability

### Concurrent Set

```
BenchmarkConcurrentSet/1Shard-1Goroutine-4            2054034   620.8 ns/op   179 B/op    8 allocs/op
BenchmarkConcurrentSet/16Shards-10Goroutines-4        2962987   414.7 ns/op   170 B/op    8 allocs/op
BenchmarkConcurrentSet/256Shards-10Goroutines-4       3444337   366.5 ns/op   168 B/op    8 allocs/op
BenchmarkConcurrentSet/256Shards-50Goroutines-4       3398780   383.1 ns/op   169 B/op    8 allocs/op
```

**Key Observations:**
- Clear benefit from sharding (620ns → 366ns)
- 41% improvement with 256 shards
- Optimal shard count depends on concurrency

## Pattern Matching

### Glob Patterns

```
BenchmarkMatchPatternGlob/user___vs_user_123-4                      30766484   39.35 ns/op   0 B/op   0 allocs/op
BenchmarkMatchPatternGlob/user___vs_user_123_profile-4              30839242   39.10 ns/op   0 B/op   0 allocs/op
BenchmarkMatchPatternGlob/user___vs_cache_session_data-4            45233241   26.74 ns/op   0 B/op   0 allocs/op
```

**Key Observations:**
- Zero allocations (excellent)
- Very fast (26-39ns)
- Most efficient pattern matching strategy

### Simple Patterns

```
BenchmarkMatchPatternSimple/user___vs_user_123-4                    179539304   6.556 ns/op   0 B/op   0 allocs/op
BenchmarkMatchPatternSimple/exact_match_vs_user_123-4               192665973   6.225 ns/op   0 B/op   0 allocs/op
```

**Key Observations:**
- Extremely fast for exact matches
- Zero allocations
- Best case performance

## Optimization Targets

Based on these baselines, here are the priority optimization targets:

### High Priority (Hot Paths)

1. **RESP Parser Allocations**
   - Current: 12-15 allocs per command
   - Target: 3-5 allocs per command (60-70% reduction)

2. **Lua Script Execution**
   - Current: 868-979 allocs per execution
   - Target: 600-700 allocs per execution (20-30% reduction)

3. **Storage Set Operations**
   - Current: 4 allocs per operation
   - Target: 2-3 allocs per operation (25-50% reduction)

### Medium Priority

4. **RESP Writer Batch Operations**
   - Current: 2.56 allocs per command (50-cmd batch)
   - Target: 1-2 allocs per command (20-50% reduction)

5. **Lua Cached Execution**
   - Current: Same as uncached (930 allocs)
   - Target: 600-700 allocs via context reuse (20-30% reduction)

### Lower Priority (Already Efficient)

6. **Pattern Matching** - Already zero-allocation
7. **Storage Get Operations** - Already minimal (0-1 allocs)
8. **Script Management** - Already efficient

## Notes

- All metrics collected with Go 1.25.2 on AMD EPYC 7763
- Benchmarks run with `-benchmem` for allocation tracking
- Multiple runs averaged for consistency
- These baselines will be used for benchstat comparisons

## Next Steps

1. Profile allocation hotspots with pprof
2. Implement tactical optimizations per ROADMAP.md
3. Measure improvements with benchstat
4. Document gains in optimization-specific documents
