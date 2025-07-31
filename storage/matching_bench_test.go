package storage

import (
	"fmt"
	"strings"
	"testing"
)

// Benchmark data sets
var (
	// Simple patterns (single wildcard)
	simplePatterns = []string{
		"user:*",
		"*:profile",
		"cache:*:data",
		"session:user:*",
		"*",
		"exact_match",
	}

	// Complex patterns (multiple wildcards, question marks)
	complexPatterns = []string{
		"user:*:profile:*",
		"cache:*:*:data",
		"session:*:user:*:*",
		"h?llo*",
		"*test*pattern*",
		"???:*:???",
	}

	// Test strings of various lengths
	testStrings = []string{
		"user:123",
		"user:123:profile",
		"cache:session:123:data",
		"session:user:456:profile:active",
		"hello_world_test_pattern",
		"a",
		"very_long_string_that_should_test_performance_with_longer_inputs_and_complex_patterns",
		"abc:def:ghi",
		"short",
		"medium_length_string",
	}
)

// BenchmarkMatchPatternSimple benchmarks the simple strategy
func BenchmarkMatchPatternSimple(b *testing.B) {
	// Reduce the number of test combinations to prevent timeouts
	patterns := []string{"user:*", "*:profile", "exact_match"}
	strings := []string{"user:123", "user:123:profile", "cache:session:data"}
	
	for _, pattern := range patterns {
		for _, str := range strings {
			b.Run(fmt.Sprintf("%s_vs_%s", sanitizeName(pattern), sanitizeName(str)), func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					matchPatternSimple(str, pattern)
				}
			})
		}
	}
}

// BenchmarkMatchPatternRegex benchmarks the regex strategy
func BenchmarkMatchPatternRegex(b *testing.B) {
	// Reduce combinations to prevent timeouts
	patterns := []string{"user:*", "*:profile", "user:*:profile:*"}
	strings := []string{"user:123", "user:123:profile", "cache:session:data"}
	
	for _, pattern := range patterns {
		for _, str := range strings {
			b.Run(fmt.Sprintf("%s_vs_%s", sanitizeName(pattern), sanitizeName(str)), func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					matchPatternRegex(str, pattern)
				}
			})
		}
	}
}

// BenchmarkMatchPatternAutomaton benchmarks the automaton strategy
func BenchmarkMatchPatternAutomaton(b *testing.B) {
	// Reduce combinations to prevent timeouts
	patterns := []string{"user:*", "*:profile", "user:*:profile:*"}
	strings := []string{"user:123", "user:123:profile", "cache:session:data"}
	
	for _, pattern := range patterns {
		for _, str := range strings {
			b.Run(fmt.Sprintf("%s_vs_%s", sanitizeName(pattern), sanitizeName(str)), func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					matchPatternAutomaton(str, pattern)
				}
			})
		}
	}
}

// BenchmarkMatchPatternGlob benchmarks the glob strategy
func BenchmarkMatchPatternGlob(b *testing.B) {
	// Reduce combinations to prevent timeouts
	patterns := []string{"user:*", "*:profile", "user:*:profile:*"}
	strings := []string{"user:123", "user:123:profile", "cache:session:data"}
	
	for _, pattern := range patterns {
		for _, str := range strings {
			b.Run(fmt.Sprintf("%s_vs_%s", sanitizeName(pattern), sanitizeName(str)), func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					matchPatternGlob(str, pattern)
				}
			})
		}
	}
}

// BenchmarkMatchPatternWithStrategy benchmarks the strategy dispatcher
func BenchmarkMatchPatternWithStrategy(b *testing.B) {
	// Simplified version to prevent timeouts
	testCases := []struct {
		strategy MatchingStrategy
		pattern  string
		str      string
	}{
		{StrategySimple, "user:*", "user:123"},
		{StrategyRegex, "user:*:profile:*", "user:123:profile:settings"},
		{StrategyAutomaton, "*:profile", "user:profile"},
		{StrategyGlob, "cache:*:data", "cache:session:data"},
	}

	for _, tc := range testCases {
		strategyName := map[MatchingStrategy]string{
			StrategySimple:    "simple",
			StrategyRegex:     "regex", 
			StrategyAutomaton: "automaton",
			StrategyGlob:      "glob",
		}[tc.strategy]
		
		b.Run(fmt.Sprintf("%s_%s_%s", strategyName, sanitizeName(tc.pattern), sanitizeName(tc.str)), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				MatchPatternWithStrategy(tc.str, tc.pattern, tc.strategy)
			}
		})
	}
}

// BenchmarkCompareStrategies compares all strategies with the same inputs
func BenchmarkCompareStrategies(b *testing.B) {
	// Simplified comparison to prevent timeouts
	pattern := "user:*"
	str := "user:123"
	
	b.Run("simple", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			matchPatternSimple(str, pattern)
		}
	})
	
	b.Run("regex", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			matchPatternRegex(str, pattern)
		}
	})
	
	b.Run("automaton", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			matchPatternAutomaton(str, pattern)
		}
	})
	
	b.Run("glob", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			matchPatternGlob(str, pattern)
		}
	})
}

// BenchmarkComplexPatterns specifically tests performance with complex patterns
func BenchmarkComplexPatterns(b *testing.B) {
	// Simplified to prevent timeouts
	testCases := []struct {
		strategy MatchingStrategy
		pattern  string
		str      string
	}{
		{StrategyRegex, "user:*:profile:*", "user:12345:profile:settings:privacy"},
		{StrategyAutomaton, "cache:*:*:data", "cache:session:user:98765:data"},
		{StrategyGlob, "h?llo*", "hello_world_pattern_matching_test"},
	}

	for _, tc := range testCases {
		strategyName := map[MatchingStrategy]string{
			StrategySimple:    "simple",
			StrategyRegex:     "regex", 
			StrategyAutomaton: "automaton",
			StrategyGlob:      "glob",
		}[tc.strategy]
		
		b.Run(fmt.Sprintf("%s_%s", strategyName, sanitizeName(tc.pattern)), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				MatchPatternWithStrategy(tc.str, tc.pattern, tc.strategy)
			}
		})
	}
}

// BenchmarkOriginalVsOptimized compares the original implementation with optimized ones
func BenchmarkOriginalVsOptimized(b *testing.B) {
	// Simplified to prevent timeouts
	pattern := "user:*"
	str := "user:123"
	
	b.Run("original", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			matchPatternOriginal(str, pattern)
		}
	})
	
	b.Run("optimized", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			matchPatternSimple(str, pattern)
		}
	})
}

// BenchmarkGlobToRegex benchmarks the glob to regex conversion
func BenchmarkGlobToRegex(b *testing.B) {
	// Simplified to prevent timeouts
	patterns := []string{"user:*", "*:profile", "user:*:profile:*"}
	
	for _, pattern := range patterns {
		b.Run(sanitizeName(pattern), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				globToRegex(pattern)
			}
		})
	}
}

// BenchmarkMemoryUsage benchmarks memory allocations
func BenchmarkMemoryUsage(b *testing.B) {
	pattern := "user:*:profile:*"
	str := "user:123:profile:settings"
	
	strategies := []struct {
		name string
		fn   func(string, string) bool
	}{
		{"simple", matchPatternSimple},
		{"regex", matchPatternRegex},
		{"automaton", matchPatternAutomaton},
		{"glob", matchPatternGlob},
	}

	for _, strategy := range strategies {
		b.Run(strategy.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				strategy.fn(str, pattern)
			}
		})
	}
}

// sanitizeName removes special characters from strings to make them valid benchmark names
func sanitizeName(s string) string {
	result := ""
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			result += string(r)
		} else {
			result += "_"
		}
	}
	return result
}

// matchPatternOriginal implements the original algorithm for comparison
func matchPatternOriginal(str, pattern string) bool {
	// Simple implementation - in production you'd want more sophisticated pattern matching
	if pattern == "*" {
		return true
	}
	
	if !strings.Contains(pattern, "*") && !strings.Contains(pattern, "?") {
		return str == pattern
	}
	
	// For now, just check prefix/suffix with single *
	if strings.HasPrefix(pattern, "*") {
		return strings.HasSuffix(str, pattern[1:])
	}
	
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(str, pattern[:len(pattern)-1])
	}
	
	return str == pattern
}