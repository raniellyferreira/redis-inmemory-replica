package storage

import (
	"strings"
	"testing"
)

// Test cases for pattern matching
var matchTestCases = []struct {
	name     string
	str      string
	pattern  string
	expected bool
}{
	// Empty patterns
	{"empty pattern, empty string", "", "", true},
	{"empty pattern, non-empty string", "test", "", false},
	{"non-empty pattern, empty string", "", "test", false},

	// Exact matches
	{"exact match", "hello", "hello", true},
	{"exact match case sensitive", "Hello", "hello", false},
	{"exact match different", "hello", "world", false},

	// Single wildcard pattern "*"
	{"single wildcard, non-empty string", "test", "*", true},
	{"single wildcard, empty string", "", "*", false},

	// Prefix patterns (ending with *)
	{"prefix match", "hello world", "hello*", true},
	{"prefix no match", "hi world", "hello*", false},
	{"prefix empty", "", "hello*", false},
	{"prefix exact", "hello", "hello*", true},

	// Suffix patterns (starting with *)
	{"suffix match", "hello world", "*world", true},
	{"suffix no match", "hello universe", "*world", false},
	{"suffix empty", "", "*world", false},
	{"suffix exact", "world", "*world", true},

	// Middle wildcard patterns
	{"middle wildcard match", "hello world", "hello*world", true},
	{"middle wildcard no match", "hello universe", "hello*world", false},
	{"middle wildcard empty middle", "helloworld", "hello*world", true},

	// Single character wildcard (?)
	{"single char wildcard", "hello", "hell?", true},
	{"single char wildcard no match", "hello", "hell??", false},
	{"single char wildcard multiple", "hello", "h?ll?", true},

	// Complex patterns (for regex/automaton strategies)
	{"multiple wildcards", "hello world test", "hello*world*", true},
	{"multiple wildcards no match", "hello universe test", "hello*world*", false},
	{"mixed wildcards", "hello world", "h?llo*", true},
	{"mixed wildcards no match", "hi world", "h?llo*", false},

	// Edge cases
	{"only stars", "anything", "***", true},
	{"only question marks", "abc", "???", true},
	{"only question marks no match", "abcd", "???", false},
	{"pattern longer than string", "hi", "hello", false},
	{"string longer than pattern", "hello", "hi", false},

	// Real-world Redis key patterns
	{"redis key prefix", "user:123:profile", "user:*", true},
	{"redis key suffix", "user:123:profile", "*:profile", true},
	{"redis key middle", "user:123:profile", "user:*:profile", true},
	{"redis key complex", "cache:user:123:data", "cache:*:*:data", true},
}

func TestMatchPatternSimple(t *testing.T) {
	for _, tc := range matchTestCases {
		// Skip complex patterns not supported by simple strategy
		if strings.Count(tc.pattern, "*") > 1 || strings.Contains(tc.pattern, "?") {
			continue
		}

		t.Run("simple_"+tc.name, func(t *testing.T) {
			result := matchPatternSimple(tc.str, tc.pattern)
			if result != tc.expected {
				t.Errorf("matchPatternSimple(%q, %q) = %v, expected %v",
					tc.str, tc.pattern, result, tc.expected)
			}
		})
	}
}

func TestMatchPatternRegex(t *testing.T) {
	for _, tc := range matchTestCases {
		t.Run("regex_"+tc.name, func(t *testing.T) {
			result := matchPatternRegex(tc.str, tc.pattern)
			if result != tc.expected {
				t.Errorf("matchPatternRegex(%q, %q) = %v, expected %v",
					tc.str, tc.pattern, result, tc.expected)
			}
		})
	}
}

func TestMatchPatternAutomaton(t *testing.T) {
	for _, tc := range matchTestCases {
		t.Run("automaton_"+tc.name, func(t *testing.T) {
			result := matchPatternAutomaton(tc.str, tc.pattern)
			if result != tc.expected {
				t.Errorf("matchPatternAutomaton(%q, %q) = %v, expected %v",
					tc.str, tc.pattern, result, tc.expected)
			}
		})
	}
}

func TestMatchPatternGlob(t *testing.T) {
	for _, tc := range matchTestCases {
		t.Run("glob_"+tc.name, func(t *testing.T) {
			result := matchPatternGlob(tc.str, tc.pattern)
			if result != tc.expected {
				t.Errorf("matchPatternGlob(%q, %q) = %v, expected %v",
					tc.str, tc.pattern, result, tc.expected)
			}
		})
	}
}

func TestMatchPatternWithStrategy(t *testing.T) {
	strategies := []struct {
		name     string
		strategy MatchingStrategy
	}{
		{"simple", StrategySimple},
		{"regex", StrategyRegex},
		{"automaton", StrategyAutomaton},
		{"glob", StrategyGlob},
	}

	for _, strategy := range strategies {
		t.Run(strategy.name, func(t *testing.T) {
			for _, tc := range matchTestCases {
				// Skip complex patterns not supported by simple strategy
				if strategy.strategy == StrategySimple &&
					(strings.Count(tc.pattern, "*") > 1 || strings.Contains(tc.pattern, "?")) {
					continue
				}

				result := MatchPatternWithStrategy(tc.str, tc.pattern, strategy.strategy)
				if result != tc.expected {
					t.Errorf("MatchPatternWithStrategy(%q, %q, %s) = %v, expected %v",
						tc.str, tc.pattern, strategy.name, result, tc.expected)
				}
			}
		})
	}
}

func TestGlobalConfiguration(t *testing.T) {
	// Save original strategy
	originalStrategy := GetMatchingStrategy()
	defer SetMatchingStrategy(originalStrategy)

	// Test setting and getting strategy
	testStrategies := []MatchingStrategy{
		StrategySimple,
		StrategyRegex,
		StrategyAutomaton,
		StrategyGlob,
	}

	for _, strategy := range testStrategies {
		SetMatchingStrategy(strategy)
		result := GetMatchingStrategy()
		if result != strategy {
			t.Errorf("Expected strategy %v, got %v", strategy, result)
		}
	}
}

func TestGlobToRegex(t *testing.T) {
	testCases := []struct {
		glob  string
		regex string
	}{
		{"hello", "hello"},
		{"*", ".*"},
		{"?", "."},
		{"hello*", "hello.*"},
		{"*world", ".*world"},
		{"hello*world", "hello.*world"},
		{"h?llo", "h.llo"},
		{"test.", "test\\."},
		{"test[abc]", "test\\[abc\\]"},
		{"test{a,b}", "test\\{a,b\\}"},
	}

	for _, tc := range testCases {
		result := globToRegex(tc.glob)
		if result != tc.regex {
			t.Errorf("globToRegex(%q) = %q, expected %q", tc.glob, result, tc.regex)
		}
	}
}

func TestMatchingErrorHandling(t *testing.T) {
	// Test with invalid regex patterns (should fallback to simple)
	result := matchPatternRegex("test", "[invalid")
	// Should not panic and return some result
	_ = result

	// Test automaton with empty pattern
	result = matchPatternAutomaton("test", "")
	if result {
		t.Error("Empty pattern should not match non-empty string")
	}

	// Test glob with invalid pattern (should fallback to simple)
	result = matchPatternGlob("test", "[")
	// Should not panic and return some result
	_ = result
}
