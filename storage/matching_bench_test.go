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
	patterns := simplePatterns // Simple strategy only supports simple patterns
	
	for _, pattern := range patterns {
		b.Run(fmt.Sprintf("pattern_%s", sanitizeName(pattern)), func(b *testing.B) {
			for _, str := range testStrings {
				b.Run(fmt.Sprintf("str_%s", sanitizeName(str)), func(b *testing.B) {
					for i := 0; i < b.N; i++ {
						matchPatternSimple(str, pattern)
					}
				})
			}
		})
	}
}

// BenchmarkMatchPatternRegex benchmarks the regex strategy
func BenchmarkMatchPatternRegex(b *testing.B) {
	allPatterns := append(simplePatterns, complexPatterns...)
	
	for _, pattern := range allPatterns {
		b.Run(fmt.Sprintf("pattern_%s", sanitizeName(pattern)), func(b *testing.B) {
			for _, str := range testStrings {
				b.Run(fmt.Sprintf("str_%s", sanitizeName(str)), func(b *testing.B) {
					for i := 0; i < b.N; i++ {
						matchPatternRegex(str, pattern)
					}
				})
			}
		})
	}
}

// BenchmarkMatchPatternAutomaton benchmarks the automaton strategy
func BenchmarkMatchPatternAutomaton(b *testing.B) {
	allPatterns := append(simplePatterns, complexPatterns...)
	
	for _, pattern := range allPatterns {
		b.Run(fmt.Sprintf("pattern_%s", sanitizeName(pattern)), func(b *testing.B) {
			for _, str := range testStrings {
				b.Run(fmt.Sprintf("str_%s", sanitizeName(str)), func(b *testing.B) {
					for i := 0; i < b.N; i++ {
						matchPatternAutomaton(str, pattern)
					}
				})
			}
		})
	}
}

// BenchmarkMatchPatternGlob benchmarks the glob strategy
func BenchmarkMatchPatternGlob(b *testing.B) {
	allPatterns := append(simplePatterns, complexPatterns...)
	
	for _, pattern := range allPatterns {
		b.Run(fmt.Sprintf("pattern_%s", sanitizeName(pattern)), func(b *testing.B) {
			for _, str := range testStrings {
				b.Run(fmt.Sprintf("str_%s", sanitizeName(str)), func(b *testing.B) {
					for i := 0; i < b.N; i++ {
						matchPatternGlob(str, pattern)
					}
				})
			}
		})
	}
}

// BenchmarkMatchPatternWithStrategy benchmarks the strategy dispatcher
func BenchmarkMatchPatternWithStrategy(b *testing.B) {
	strategies := []struct {
		name     string
		strategy MatchingStrategy
		patterns []string
	}{
		{"simple", StrategySimple, simplePatterns},
		{"regex", StrategyRegex, append(simplePatterns, complexPatterns...)},
		{"automaton", StrategyAutomaton, append(simplePatterns, complexPatterns...)},
		{"glob", StrategyGlob, append(simplePatterns, complexPatterns...)},
	}

	for _, strategy := range strategies {
		b.Run(strategy.name, func(b *testing.B) {
			for _, pattern := range strategy.patterns {
				b.Run(fmt.Sprintf("pattern_%s", sanitizeName(pattern)), func(b *testing.B) {
					for _, str := range testStrings {
						b.Run(fmt.Sprintf("str_%s", sanitizeName(str)), func(b *testing.B) {
							for i := 0; i < b.N; i++ {
								MatchPatternWithStrategy(str, pattern, strategy.strategy)
							}
						})
					}
				})
			}
		})
	}
}

// BenchmarkCompareStrategies compares all strategies with the same inputs
func BenchmarkCompareStrategies(b *testing.B) {
	// Use simple patterns for fair comparison
	patterns := simplePatterns
	
	for _, pattern := range patterns {
		for _, str := range testStrings {
			testName := fmt.Sprintf("%s_vs_%s", sanitizeName(pattern), sanitizeName(str))
			
			b.Run(fmt.Sprintf("simple_%s", testName), func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					matchPatternSimple(str, pattern)
				}
			})
			
			b.Run(fmt.Sprintf("regex_%s", testName), func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					matchPatternRegex(str, pattern)
				}
			})
			
			b.Run(fmt.Sprintf("automaton_%s", testName), func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					matchPatternAutomaton(str, pattern)
				}
			})
			
			b.Run(fmt.Sprintf("glob_%s", testName), func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					matchPatternGlob(str, pattern)
				}
			})
		}
	}
}

// BenchmarkComplexPatterns specifically tests performance with complex patterns
func BenchmarkComplexPatterns(b *testing.B) {
	complexStrs := []string{
		"user:12345:profile:settings:privacy",
		"cache:session:user:98765:data:temp:active",
		"hello_world_pattern_matching_test",
		"very_long_key_name_with_multiple_segments_and_complex_structure",
	}
	
	strategies := []struct {
		name     string
		strategy MatchingStrategy
	}{
		{"regex", StrategyRegex},
		{"automaton", StrategyAutomaton},
		{"glob", StrategyGlob},
	}

	for _, strategy := range strategies {
		b.Run(strategy.name, func(b *testing.B) {
			for _, pattern := range complexPatterns {
				b.Run(fmt.Sprintf("pattern_%s", sanitizeName(pattern)), func(b *testing.B) {
					for _, str := range complexStrs {
						b.Run(fmt.Sprintf("str_%s", sanitizeName(str)), func(b *testing.B) {
							for i := 0; i < b.N; i++ {
								MatchPatternWithStrategy(str, pattern, strategy.strategy)
							}
						})
					}
				})
			}
		})
	}
}

// BenchmarkOriginalVsOptimized compares the original implementation with optimized ones
func BenchmarkOriginalVsOptimized(b *testing.B) {
	patterns := []string{"user:*", "*:profile", "cache:*:data"}
	strings := []string{"user:123", "user:123:profile", "cache:session:data"}

	for _, pattern := range patterns {
		for _, str := range strings {
			testName := fmt.Sprintf("%s_vs_%s", sanitizeName(pattern), sanitizeName(str))
			
			b.Run(fmt.Sprintf("original_%s", testName), func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					// This will call the original implementation when we update the main function
					matchPatternOriginal(str, pattern)
				}
			})
			
			b.Run(fmt.Sprintf("optimized_%s", testName), func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					matchPatternSimple(str, pattern)
				}
			})
		}
	}
}

// BenchmarkGlobToRegex benchmarks the glob to regex conversion
func BenchmarkGlobToRegex(b *testing.B) {
	patterns := append(simplePatterns, complexPatterns...)
	
	for _, pattern := range patterns {
		b.Run(fmt.Sprintf("pattern_%s", sanitizeName(pattern)), func(b *testing.B) {
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