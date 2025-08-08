package replication

import (
	"fmt"
)

// LZF decompression implementation for Redis RDB parsing
// Based on the LZF compression format used by Redis

// lzfDecompress decompresses LZF compressed data
func lzfDecompress(compressed []byte, uncompressedLen int) ([]byte, error) {
	if len(compressed) == 0 {
		return []byte{}, nil
	}

	result := make([]byte, uncompressedLen)
	resultPos := 0
	compressedPos := 0

	for compressedPos < len(compressed) && resultPos < uncompressedLen {
		ctrl := compressed[compressedPos]
		compressedPos++

		if ctrl < 32 { // Literal run
			// Copy literal bytes
			literalLen := int(ctrl) + 1
			if compressedPos+literalLen > len(compressed) {
				return nil, fmt.Errorf("LZF decompression error: not enough literal data")
			}
			if resultPos+literalLen > uncompressedLen {
				return nil, fmt.Errorf("LZF decompression error: output buffer overflow")
			}

			copy(result[resultPos:], compressed[compressedPos:compressedPos+literalLen])
			resultPos += literalLen
			compressedPos += literalLen

		} else { // Back reference
			length := int(ctrl >> 5)
			if length == 7 {
				// Extended length
				if compressedPos >= len(compressed) {
					return nil, fmt.Errorf("LZF decompression error: missing extended length")
				}
				length += int(compressed[compressedPos])
				compressedPos++
			}
			length += 2 // Minimum match length is 2

			if compressedPos >= len(compressed) {
				return nil, fmt.Errorf("LZF decompression error: missing offset")
			}

			// Read offset
			offset := int(ctrl&31)<<8 + int(compressed[compressedPos])
			compressedPos++
			offset++ // Offset is 1-based

			if offset > resultPos {
				return nil, fmt.Errorf("LZF decompression error: invalid offset")
			}

			if resultPos+length > uncompressedLen {
				return nil, fmt.Errorf("LZF decompression error: output buffer overflow")
			}

			// Copy from back reference
			backRefPos := resultPos - offset
			for i := 0; i < length; i++ {
				result[resultPos] = result[backRefPos]
				resultPos++
				backRefPos++
			}
		}
	}

	if resultPos != uncompressedLen {
		return nil, fmt.Errorf("LZF decompression error: decompressed size mismatch: expected %d, got %d", uncompressedLen, resultPos)
	}

	return result, nil
}