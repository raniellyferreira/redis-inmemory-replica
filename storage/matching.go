package storage

import (
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// MatchingStrategy defines the algorithm used for pattern matching
type MatchingStrategy int

const (
	// StrategySimple uses optimized algorithms for common patterns (single wildcard)
	StrategySimple MatchingStrategy = iota
	// StrategyRegex uses regular expressions for complex patterns
	StrategyRegex
	// StrategyAutomaton uses finite state machines for pattern matching
	StrategyAutomaton
	// StrategyGlob uses Go's standard library filepath.Match
	StrategyGlob
)

// MatchingConfig holds configuration for pattern matching
type MatchingConfig struct {
	mu       sync.RWMutex
	strategy MatchingStrategy
}

var (
	// Default global configuration
	globalMatchingConfig = &MatchingConfig{
		strategy: StrategyGlob, // Default to glob matching for full Redis pattern support
	}
)

// SetMatchingStrategy sets the global matching strategy
func SetMatchingStrategy(strategy MatchingStrategy) {
	globalMatchingConfig.mu.Lock()
	defer globalMatchingConfig.mu.Unlock()
	globalMatchingConfig.strategy = strategy
}

// GetMatchingStrategy returns the current global matching strategy
func GetMatchingStrategy() MatchingStrategy {
	globalMatchingConfig.mu.RLock()
	defer globalMatchingConfig.mu.RUnlock()
	return globalMatchingConfig.strategy
}

// MatchPatternWithStrategy matches a string against a pattern using the specified strategy
func MatchPatternWithStrategy(str, pattern string, strategy MatchingStrategy) bool {
	switch strategy {
	case StrategySimple:
		return matchPatternSimple(str, pattern)
	case StrategyRegex:
		return matchPatternRegex(str, pattern)
	case StrategyAutomaton:
		return matchPatternAutomaton(str, pattern)
	case StrategyGlob:
		return matchPatternGlob(str, pattern)
	default:
		// Fallback to simple strategy
		return matchPatternSimple(str, pattern)
	}
}

// matchPatternSimple implements the optimized algorithm for simple patterns
// This is based on the reference implementation provided in the problem statement
func matchPatternSimple(key, pattern string) bool {
	// If pattern is empty, only matches empty key
	if pattern == "" {
		return key == ""
	}

	// If pattern is "*", matches any non-empty key
	if pattern == "*" {
		return key != ""
	}

	// Exact match
	if key == pattern {
		return true
	}

	// If pattern ends with *, it's a prefix match
	if len(pattern) > 0 && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return strings.HasPrefix(key, prefix)
	}

	// If pattern starts with *, it's a suffix match
	if len(pattern) > 0 && pattern[0] == '*' {
		suffix := pattern[1:]
		return strings.HasSuffix(key, suffix)
	}

	// If pattern contains a single * in the middle, it's a substring match.
	if starIndex := strings.IndexByte(pattern, '*'); starIndex != -1 {
		// Ensure there's only one '*' to match the original function's behavior
		if strings.LastIndexByte(pattern, '*') != starIndex {
			return false // Multiple stars not supported in simple strategy
		}
		return strings.HasPrefix(key, pattern[:starIndex]) && strings.HasSuffix(key, pattern[starIndex+1:])
	}

	return false
}

// matchPatternRegex implements pattern matching using regular expressions
func matchPatternRegex(str, pattern string) bool {
	// Handle special case: single "*" should not match empty string
	if pattern == "*" && str == "" {
		return false
	}

	// Handle empty pattern
	if pattern == "" {
		return str == ""
	}

	// Convert glob pattern to regex pattern
	regexPattern := globToRegex(pattern)

	// Compile and match
	regex, err := regexp.Compile("^" + regexPattern + "$")
	if err != nil {
		// Fallback to simple strategy on regex compilation error
		return matchPatternSimple(str, pattern)
	}

	return regex.MatchString(str)
}

// globToRegex converts a glob pattern to a regular expression
func globToRegex(pattern string) string {
	var result strings.Builder
	for _, char := range pattern {
		switch char {
		case '*':
			result.WriteString(".*")
		case '?':
			result.WriteString(".")
		case '.', '^', '$', '+', '(', ')', '[', ']', '{', '}', '|', '\\':
			// Escape regex special characters
			result.WriteString("\\")
			result.WriteRune(char)
		default:
			result.WriteRune(char)
		}
	}
	return result.String()
}

// matchPatternAutomaton implements pattern matching using a finite state machine
func matchPatternAutomaton(str, pattern string) bool {
	// Handle special cases
	if pattern == "" {
		return str == ""
	}

	if pattern == "*" {
		return str != ""
	}

	// Use a simple recursive approach with memoization for automaton-like behavior
	return matchAutomaton(str, pattern, 0, 0, make(map[[2]int]bool))
}

// matchAutomaton implements recursive pattern matching with memoization
func matchAutomaton(str, pattern string, strIdx, patIdx int, memo map[[2]int]bool) bool {
	// Check memo
	key := [2]int{strIdx, patIdx}
	if result, exists := memo[key]; exists {
		return result
	}

	// Base cases
	if patIdx == len(pattern) {
		result := strIdx == len(str)
		memo[key] = result
		return result
	}

	if strIdx == len(str) {
		// Check if remaining pattern is all wildcards
		for i := patIdx; i < len(pattern); i++ {
			if pattern[i] != '*' {
				memo[key] = false
				return false
			}
		}
		memo[key] = true
		return true
	}

	// Current pattern character
	patChar := pattern[patIdx]
	strChar := str[strIdx]

	var result bool

	switch patChar {
	case '*':
		// Try matching zero characters (move pattern index)
		result = matchAutomaton(str, pattern, strIdx, patIdx+1, memo)

		// Try matching one or more characters (move string index)
		if !result {
			result = matchAutomaton(str, pattern, strIdx+1, patIdx, memo)
		}
	case '?':
		// Match single character
		result = matchAutomaton(str, pattern, strIdx+1, patIdx+1, memo)
	default:
		// Exact match required
		if patChar == strChar {
			result = matchAutomaton(str, pattern, strIdx+1, patIdx+1, memo)
		} else {
			// No match
			result = false
		}
	}

	memo[key] = result
	return result
}

// matchPatternGlob implements pattern matching using Go's standard library
func matchPatternGlob(str, pattern string) bool {
	// Handle special case: single "*" should not match empty string
	if pattern == "*" && str == "" {
		return false
	}

	// Handle empty pattern
	if pattern == "" {
		return str == ""
	}

	matched, err := filepath.Match(pattern, str)
	if err != nil {
		// Fallback to simple strategy on error
		return matchPatternSimple(str, pattern)
	}
	return matched
}
