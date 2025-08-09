package replication

import (
	"testing"
)

func TestLZFDecompression(t *testing.T) {
	tests := []struct {
		name            string
		compressed      []byte
		uncompressedLen int
		expected        []byte
		shouldError     bool
	}{
		{
			name:            "empty data",
			compressed:      []byte{},
			uncompressedLen: 0,
			expected:        []byte{},
			shouldError:     false,
		},
		{
			name:            "simple literal",
			compressed:      []byte{0x05, 'h', 'e', 'l', 'l', 'o', '!'},
			uncompressedLen: 6,
			expected:        []byte("hello!"),
			shouldError:     false,
		},
		{
			name:            "invalid length",
			compressed:      []byte{0x05, 'h', 'e', 'l'},
			uncompressedLen: 6,
			expected:        nil,
			shouldError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := lzfDecompress(tt.compressed, tt.uncompressedLen)

			if tt.shouldError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if len(result) != len(tt.expected) {
				t.Errorf("Length mismatch: expected %d, got %d", len(tt.expected), len(result))
				return
			}

			for i := range tt.expected {
				if result[i] != tt.expected[i] {
					t.Errorf("Content mismatch at index %d: expected %v, got %v", i, tt.expected[i], result[i])
					return
				}
			}
		})
	}
}
