package storage

import (
	"sort"
	"testing"
	"time"
)

func TestMemoryStorageKeysPattern(t *testing.T) {
	s := NewMemory()
	defer func() { _ = s.Close() }()

	// Add test keys
	testKeys := []string{
		"user:1",
		"user:2",
		"user:10",
		"config:app",
		"config:db",
		"session:abc123",
		"session:def456",
		"cache:key1",
		"temp_file",
		"log_2024",
	}

	for _, key := range testKeys {
		err := s.Set(key, []byte("value"), nil)
		if err != nil {
			t.Fatalf("Failed to set key %s: %v", key, err)
		}
	}

	tests := []struct {
		name     string
		pattern  string
		expected []string
	}{
		{
			name:     "match all with asterisk",
			pattern:  "*",
			expected: testKeys,
		},
		{
			name:     "match all with empty pattern",
			pattern:  "",
			expected: testKeys,
		},
		{
			name:     "match user prefix",
			pattern:  "user:*",
			expected: []string{"user:1", "user:2", "user:10"},
		},
		{
			name:     "match config prefix",
			pattern:  "config:*",
			expected: []string{"config:app", "config:db"},
		},
		{
			name:     "match session prefix",
			pattern:  "session:*",
			expected: []string{"session:abc123", "session:def456"},
		},
		{
			name:     "match single character wildcard",
			pattern:  "user:?",
			expected: []string{"user:1", "user:2"},
		},
		{
			name:     "match character class",
			pattern:  "user:[12]",
			expected: []string{"user:1", "user:2"},
		},
		{
			name:     "match character range",
			pattern:  "config:[a-d]*",
			expected: []string{"config:app", "config:db"},
		},
		{
			name:     "match wildcard suffix",
			pattern:  "*_*",
			expected: []string{"temp_file", "log_2024"},
		},
		{
			name:     "match exact key",
			pattern:  "cache:key1",
			expected: []string{"cache:key1"},
		},
		{
			name:     "no matches",
			pattern:  "nonexistent:*",
			expected: []string{},
		},
		{
			name:     "match middle wildcard",
			pattern:  "user:1*",
			expected: []string{"user:1", "user:10"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.Keys(tt.pattern)

			// Sort both slices for comparison
			sort.Strings(result)
			sort.Strings(tt.expected)

			if !slicesEqual(result, tt.expected) {
				t.Errorf("Keys(%q) = %v, want %v", tt.pattern, result, tt.expected)
			}
		})
	}
}

func TestMemoryStorageKeysPatternWithExpiredKeys(t *testing.T) {
	s := NewMemory()
	defer func() { _ = s.Close() }()

	// Add keys, some with expiration
	err := s.Set("active:key1", []byte("value1"), nil)
	if err != nil {
		t.Fatalf("Failed to set active:key1: %v", err)
	}

	err = s.Set("active:key2", []byte("value2"), nil)
	if err != nil {
		t.Fatalf("Failed to set active:key2: %v", err)
	}

	// This key should be excluded from results as it's expired
	pastTime := time.Now().Add(-1 * time.Hour) // 1 hour in the past
	err = s.Set("expired:key1", []byte("expired"), &pastTime)
	if err != nil {
		t.Fatalf("Failed to set expired:key1: %v", err)
	}

	// Test that expired keys are not returned
	result := s.Keys("*")
	sort.Strings(result)

	expected := []string{"active:key1", "active:key2"}
	sort.Strings(expected)

	if !slicesEqual(result, expected) {
		t.Errorf("Keys(*) = %v, want %v (expired keys should be excluded)", result, expected)
	}

	// Test pattern matching excludes expired keys
	result = s.Keys("expired:*")
	if len(result) != 0 {
		t.Errorf("Keys(expired:*) = %v, want empty slice (expired keys should be excluded)", result)
	}
}

func TestMemoryStorageKeysPatternInvalidPattern(t *testing.T) {
	s := NewMemory()
	defer func() { _ = s.Close() }()

	// Add a test key
	err := s.Set("test:key", []byte("value"), nil)
	if err != nil {
		t.Fatalf("Failed to set test:key: %v", err)
	}

	// Test with malformed pattern (unclosed bracket)
	// This should not crash and should return empty results
	result := s.Keys("test:[key")

	// Invalid patterns should return empty results
	if len(result) != 0 {
		t.Errorf("Keys with invalid pattern should return empty results, got %v", result)
	}
}

// Helper function to compare string slices
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
