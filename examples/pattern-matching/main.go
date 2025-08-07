package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/raniellyferreira/redis-inmemory-replica/storage"
)

func main() {
	fmt.Println("Redis In-Memory Replica - Pattern Matching Strategies Demo")
	fmt.Println("========================================================")

	// Test patterns and strings
	patterns := []string{"user:*", "*:profile", "cache:*:data", "user:*:profile:*", "h?llo"}
	testStrings := []string{
		"user:123",
		"user:123:profile",
		"cache:session:data",
		"user:123:profile:settings",
		"hello",
		"nomatch",
	}

	strategies := []struct {
		name     string
		strategy storage.MatchingStrategy
	}{
		{"Simple", storage.StrategySimple},
		{"Regex", storage.StrategyRegex},
		{"Automaton", storage.StrategyAutomaton},
		{"Glob", storage.StrategyGlob},
	}

	fmt.Println("\n1. Comparing Different Strategies")
	fmt.Println(strings.Repeat("-", 50))

	for _, pattern := range patterns {
		fmt.Printf("\nPattern: %s\n", pattern)

		for _, str := range testStrings {
			fmt.Printf("  %-30s -> ", str)

			for _, strategy := range strategies {
				// Skip unsupported patterns for simple strategy
				if strategy.strategy == storage.StrategySimple &&
					(countChars(pattern, '*') > 1 || contains(pattern, '?')) {
					fmt.Printf("%-10s ", "N/A")
					continue
				}

				match := storage.MatchPatternWithStrategy(str, pattern, strategy.strategy)
				if match {
					fmt.Printf("%-10s ", "✓")
				} else {
					fmt.Printf("%-10s ", "✗")
				}
			}
			fmt.Println()
		}
	}

	fmt.Println("\n2. Performance Demonstration")
	fmt.Println(strings.Repeat("-", 30))

	// Simple performance test
	pattern := "user:*"
	testString := "user:123"
	iterations := 1000000

	for _, strategy := range strategies {
		if strategy.strategy == storage.StrategySimple ||
			strategy.strategy == storage.StrategyGlob {

			start := time.Now()
			for i := 0; i < iterations; i++ {
				storage.MatchPatternWithStrategy(testString, pattern, strategy.strategy)
			}
			duration := time.Since(start)

			fmt.Printf("%-10s: %v (%d iterations)\n",
				strategy.name, duration, iterations)
		}
	}

	fmt.Println("\n3. Global Configuration Demo")
	fmt.Println(strings.Repeat("-", 30))

	// Save original strategy
	originalStrategy := storage.GetMatchingStrategy()

	// Test different global configurations
	fmt.Printf("Original strategy: %v\n", originalStrategy)

	// Switch to regex for complex patterns
	storage.SetMatchingStrategy(storage.StrategyRegex)
	fmt.Printf("Changed to: %v\n", storage.GetMatchingStrategy())

	// Test with global configuration
	complexPattern := "user:*:profile:*"
	testStr := "user:123:profile:settings"

	// This will use the globally configured regex strategy
	match := matchPatternWrapper(testStr, complexPattern)
	fmt.Printf("Complex pattern match (%s vs %s): %v\n", testStr, complexPattern, match)

	// Restore original strategy
	storage.SetMatchingStrategy(originalStrategy)
	fmt.Printf("Restored to: %v\n", storage.GetMatchingStrategy())

	fmt.Println("\n4. Real-world Redis Use Cases")
	fmt.Println(strings.Repeat("-", 30))

	redisKeys := []string{
		"user:1001:session",
		"user:1002:profile",
		"cache:user:1001:data",
		"session:active:user:1001",
		"temp:cache:session:abc123",
	}

	redisPatterns := []string{
		"user:*",           // All user keys
		"*:profile",        // All profile keys
		"cache:*:data",     // All cache data
		"session:*:user:*", // All user sessions
	}

	fmt.Println("Redis key patterns:")
	for _, pattern := range redisPatterns {
		fmt.Printf("\nPattern: %s\n", pattern)
		for _, key := range redisKeys {
			// Use simple strategy for these common patterns
			match := storage.MatchPatternWithStrategy(key, pattern, storage.StrategySimple)
			status := "✗"
			if match {
				status = "✓"
			}
			fmt.Printf("  %s %s\n", status, key)
		}
	}

	fmt.Println("\nDemo completed!")
}

// Helper function that would use the global configuration
func matchPatternWrapper(str, pattern string) bool {
	// This simulates how the actual matchPattern function works
	return storage.MatchPatternWithStrategy(str, pattern, storage.GetMatchingStrategy())
}

// Helper functions
func countChars(s string, char rune) int {
	count := 0
	for _, c := range s {
		if c == char {
			count++
		}
	}
	return count
}

func contains(s string, char rune) bool {
	for _, c := range s {
		if c == char {
			return true
		}
	}
	return false
}
