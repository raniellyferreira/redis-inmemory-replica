# Pattern Matching Strategies

This document describes the different pattern matching strategies available in the Redis In-Memory Replica library.

## Overview

The `matchPattern` function has been enhanced to support multiple matching strategies, allowing users to choose between performance-optimized simple patterns and more complex pattern matching capabilities.

## Available Strategies

### 1. Simple Strategy (StrategySimple) - **Default**

**Best for**: Common Redis patterns with single wildcards
**Performance**: Fastest (2.6-17ns per operation, 0 allocations)
**Supported patterns**:
- Exact matches: `hello`
- Prefix matches: `user:*`
- Suffix matches: `*:profile`
- Single middle wildcard: `user:*:profile`

**Limitations**: Does not support multiple wildcards or `?` patterns

### 2. Regex Strategy (StrategyRegex)

**Best for**: Complex patterns with multiple wildcards
**Performance**: Slower (3000-5000ns per operation, 40-60 allocations)
**Supported patterns**:
- All simple patterns
- Multiple wildcards: `user:*:profile:*`
- Question mark wildcards: `h?llo`
- Mixed patterns: `user:*:data:?`

### 3. Automaton Strategy (StrategyAutomaton)

**Best for**: Complex patterns with better performance than regex
**Performance**: Medium (30-4000ns per operation, 0-13 allocations)
**Supported patterns**:
- All patterns supported by regex
- Uses recursive matching with memoization

### 4. Glob Strategy (StrategyGlob)

**Best for**: Standard filesystem-like patterns
**Performance**: Fast for simple patterns (30-700ns per operation, 0 allocations)
**Supported patterns**:
- Uses Go's `filepath.Match` function
- Standard glob patterns

## Configuration

### Global Configuration

```go
import "github.com/raniellyferreira/redis-inmemory-replica/storage"

// Set the global matching strategy
storage.SetMatchingStrategy(storage.StrategySimple) // Default
storage.SetMatchingStrategy(storage.StrategyRegex)  // For complex patterns
storage.SetMatchingStrategy(storage.StrategyAutomaton)
storage.SetMatchingStrategy(storage.StrategyGlob)

// Get the current strategy
currentStrategy := storage.GetMatchingStrategy()
```

### Direct Strategy Usage

```go
// Use a specific strategy for a single match
result := storage.MatchPatternWithStrategy("user:123", "user:*", storage.StrategySimple)
```

## Performance Comparison

Based on benchmarks with common Redis patterns:

| Strategy  | Performance (ns/op) | Allocations | Best Use Case |
|-----------|-------------------|-------------|---------------|
| Simple    | 2.6-17            | 0           | Single wildcard patterns |
| Glob      | 30-700            | 0           | Standard glob patterns |
| Automaton | 30-4000           | 0-13        | Complex patterns, medium performance |
| Regex     | 3000-5000         | 40-60       | Very complex patterns |

## Recommendations

1. **Use Simple Strategy (default)** for:
   - Redis SCAN operations with simple patterns
   - High-performance applications with basic pattern needs
   - Patterns like `user:*`, `*:profile`, `cache:*:data`

2. **Use Glob Strategy** for:
   - Filesystem-like patterns
   - When you need standard glob behavior
   - Good balance of features and performance

3. **Use Automaton Strategy** for:
   - Complex patterns that need better performance than regex
   - Patterns with multiple wildcards
   - When you need complex patterns but performance matters

4. **Use Regex Strategy** for:
   - Very complex patterns
   - When maximum pattern flexibility is needed
   - Low-frequency operations where performance is not critical

## Examples

```go
// Simple patterns (use default Simple strategy)
storage.SetMatchingStrategy(storage.StrategySimple)
match1 := matchPattern("user:123", "user:*")        // true
match2 := matchPattern("user:123:profile", "*:profile") // true

// Complex patterns (switch to Regex or Automaton)
storage.SetMatchingStrategy(storage.StrategyRegex)
match3 := matchPattern("user:123:profile:settings", "user:*:profile:*") // true
match4 := matchPattern("hello", "h?llo")                                // true

// Direct strategy usage
match5 := storage.MatchPatternWithStrategy("test", "t?st", storage.StrategyGlob) // true
```

## Backward Compatibility

The `matchPattern` function maintains complete backward compatibility. Existing code will continue to work without changes, using the Simple strategy by default for optimal performance.

## Migration Guide

To take advantage of the new strategies:

1. **No changes needed** for existing simple patterns - they will automatically use the optimized Simple strategy
2. **For complex patterns**: Change the global strategy or use `MatchPatternWithStrategy` directly
3. **For performance-critical applications**: Benchmark your specific patterns to choose the optimal strategy

## Thread Safety

All strategy configuration functions are thread-safe and can be called concurrently from multiple goroutines.