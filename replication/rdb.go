package replication

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"strconv"
	"time"
)

// RDB format constants
const (
	// RDB version and opcodes
	RDBVersion9       = 9
	RDBVersion10      = 10
	RDBVersion11      = 11  
	RDBVersion12      = 12
	MaxSupportedRDBVersion = 12  // Support up to Redis 7.x RDB format
	RDBOpcodeEOF      = 0xFF
	RDBOpcodeDB       = 0xFE
	RDBOpcodeExpiry   = 0xFD
	RDBOpcodeExpiryMs = 0xFC
	RDBOpcodeResizeDB = 0xFB
	RDBOpcodeAux      = 0xFA

	// Type constants
	RDBTypeString          = 0
	RDBTypeList            = 1
	RDBTypeSet             = 2
	RDBTypeZSet            = 3
	RDBTypeHash            = 4
	RDBTypeZSet2           = 5
	RDBTypeModule          = 6
	RDBTypeModule2         = 7
	RDBTypeHashZipmap      = 9
	RDBTypeListZiplist     = 10
	RDBTypeSetIntset       = 11
	RDBTypeZSetZiplist     = 12
	RDBTypeHashZiplist     = 13
	RDBTypeListQuicklist   = 14
	RDBTypeStreamListpacks = 15
	
	// Extended types for newer Redis versions
	RDBTypeStreamListpacks2 = 19
	RDBTypeStreamListpacks3 = 20
)

// RDB version compatibility strategy
type VersionStrategy struct {
	version               int
	supportsBinaryAux     bool
	requiresStrictParsing bool
	maxSkippableErrors    int
}

var versionStrategies = map[int]VersionStrategy{
	9:  {version: 9, supportsBinaryAux: false, requiresStrictParsing: true, maxSkippableErrors: 0},
	10: {version: 10, supportsBinaryAux: true, requiresStrictParsing: true, maxSkippableErrors: 1},
	11: {version: 11, supportsBinaryAux: true, requiresStrictParsing: false, maxSkippableErrors: 2},
	12: {version: 12, supportsBinaryAux: true, requiresStrictParsing: false, maxSkippableErrors: 3},
}

// RDBHandler processes RDB entries during parsing
type RDBHandler interface {
	// OnDatabase is called when switching to a new database
	OnDatabase(index int) error

	// OnKey is called for each key-value pair
	OnKey(key []byte, value interface{}, expiry *time.Time) error

	// OnAux is called for auxiliary fields
	OnAux(key, value []byte) error

	// OnEnd is called when parsing is complete
	OnEnd() error
}

// RDBParser parses RDB files in streaming mode
type RDBParser struct {
	reader   io.Reader
	handler  RDBHandler
	br       *bufio.Reader
	strategy VersionStrategy
	errors   int
	logger   Logger
}

// NewRDBParser creates a new RDB parser
func NewRDBParser(r io.Reader, handler RDBHandler) *RDBParser {
	return &RDBParser{
		reader:  r,
		handler: handler,
		br:      bufio.NewReader(r),
		logger:  nil, // Will be set by client if needed
	}
}

// SetLogger sets the logger for the RDB parser
func (p *RDBParser) SetLogger(logger Logger) {
	p.logger = logger
}

// logDebug logs a debug message if logger is available
func (p *RDBParser) logDebug(msg string, args ...interface{}) {
	if p.logger != nil {
		p.logger.Debug(msg, args...)
	}
}

// Parse parses the RDB stream
func (p *RDBParser) Parse() error {
	// Read and validate RDB header
	header := make([]byte, 9)
	if _, err := io.ReadFull(p.br, header); err != nil {
		return fmt.Errorf("failed to read RDB header: %w", err)
	}

	if string(header[:5]) != "REDIS" {
		return fmt.Errorf("invalid RDB magic: %s", header[:5])
	}

	version, err := strconv.Atoi(string(header[5:]))
	if err != nil {
		return fmt.Errorf("invalid RDB version: %s", header[5:])
	}

	if version > MaxSupportedRDBVersion {
		return fmt.Errorf("unsupported RDB version: %d (max supported: %d)", version, MaxSupportedRDBVersion)
	}

	// Set version-specific strategy
	if strategy, ok := versionStrategies[version]; ok {
		p.strategy = strategy
	} else {
		// Use most permissive strategy for unknown versions
		p.strategy = VersionStrategy{
			version:               version,
			supportsBinaryAux:     true,
			requiresStrictParsing: false,
			maxSkippableErrors:    5,
		}
	}

	// Parse RDB content
	currentDB := 0
	var expiry *time.Time

	for {
		opcode, err := p.br.ReadByte()
		if err == io.EOF {
			break
		}
		if err != nil {
			if p.canSkipError() {
				continue
			}
			return fmt.Errorf("failed to read opcode: %w", err)
		}

		switch opcode {
		case RDBOpcodeEOF:
			// End of RDB
			return p.handler.OnEnd()

		case RDBOpcodeDB:
			// Database selector
			db, err := p.readLength()
			if err != nil {
				if p.canSkipError() {
					continue
				}
				return fmt.Errorf("failed to read database number: %w", err)
			}
			currentDB = int(db)
			if err := p.handler.OnDatabase(currentDB); err != nil {
				return err
			}

		case RDBOpcodeExpiry:
			// Expiry in seconds
			var timestamp uint32
			if err := binary.Read(p.br, binary.LittleEndian, &timestamp); err != nil {
				if p.canSkipError() {
					expiry = nil
					continue
				}
				return fmt.Errorf("failed to read expiry timestamp: %w", err)
			}
			t := time.Unix(int64(timestamp), 0)
			expiry = &t

		case RDBOpcodeExpiryMs:
			// Expiry in milliseconds
			var timestamp uint64
			if err := binary.Read(p.br, binary.LittleEndian, &timestamp); err != nil {
				if p.canSkipError() {
					expiry = nil
					continue
				}
				return fmt.Errorf("failed to read expiry timestamp: %w", err)
			}
			t := time.Unix(int64(timestamp/1000), int64((timestamp%1000)*1000000))
			expiry = &t

		case RDBOpcodeResizeDB:
			// Database resize hint - skip for now
			if _, err := p.readLength(); err != nil {
				if !p.canSkipError() {
					return err
				}
			}
			if _, err := p.readLength(); err != nil {
				if !p.canSkipError() {
					return err
				}
			}

		case RDBOpcodeAux:
			// Auxiliary field
			if err := p.readAuxField(); err != nil {
				if !p.canSkipError() {
					return fmt.Errorf("failed to read aux field: %w", err)
				}
				// Continue parsing even if aux field fails
			}

		default:
			// Key-value pair
			if err := p.readKeyValue(opcode, expiry); err != nil {
				if !p.canSkipError() {
					return err
				}
				// Continue parsing even if key-value fails
			}

			// Reset expiry after use
			expiry = nil
		}
	}

	return p.handler.OnEnd()
}

// readLength reads a length-encoded integer
func (p *RDBParser) readLength() (uint64, error) {
	b, err := p.br.ReadByte()
	if err != nil {
		return 0, err
	}

	switch (b & 0xC0) >> 6 {
	case 0:
		// 6-bit length
		return uint64(b & 0x3F), nil

	case 1:
		// 14-bit length
		b2, err := p.br.ReadByte()
		if err != nil {
			return 0, err
		}
		return uint64(b&0x3F)<<8 | uint64(b2), nil

	case 2:
		// 32-bit length
		var length uint32
		if err := binary.Read(p.br, binary.BigEndian, &length); err != nil {
			return 0, err
		}
		return uint64(length), nil

	case 3:
		// Special format
		switch b & 0x3F {
		case 0:
			// 8-bit integer
			b, err := p.br.ReadByte()
			return uint64(b), err
		case 1:
			// 16-bit integer
			var val uint16
			err := binary.Read(p.br, binary.LittleEndian, &val)
			return uint64(val), err
		case 2:
			// 32-bit integer
			var val uint32
			err := binary.Read(p.br, binary.LittleEndian, &val)
			return uint64(val), err
		default:
			return 0, fmt.Errorf("invalid special length encoding: %d", b&0x3F)
		}
	}

	return 0, fmt.Errorf("invalid length encoding: %d", (b&0xC0)>>6)
}

// canSkipError determines if we can skip parsing errors based on version strategy
func (p *RDBParser) canSkipError() bool {
	p.errors++
	return p.errors <= p.strategy.maxSkippableErrors
}

// readAuxField reads auxiliary field with version-specific handling
func (p *RDBParser) readAuxField() error {
	key, err := p.readString()
	if err != nil {
		return fmt.Errorf("failed to read aux key: %w", err)
	}

	// For binary aux fields in newer versions, use safe reading
	var value []byte
	if p.strategy.supportsBinaryAux {
		value, err = p.readBinaryString()
	} else {
		value, err = p.readString()
	}
	
	if err != nil {
		return fmt.Errorf("failed to read aux value for key %s: %w", key, err)
	}

	// Only call handler if both key and value were successfully read
	if err := p.handler.OnAux(key, value); err != nil {
		return err
	}

	return nil
}

// readKeyValue reads a key-value pair with error recovery
func (p *RDBParser) readKeyValue(valueType byte, expiry *time.Time) error {
	key, err := p.readString()
	if err != nil {
		return fmt.Errorf("failed to read key: %w", err)
	}

	value, err := p.readValue(valueType)
	if err != nil {
		return fmt.Errorf("failed to read value for key %s: %w", key, err)
	}

	// Only call OnKey if value was successfully parsed (not skipped)
	if value != nil {
		if err := p.handler.OnKey(key, value, expiry); err != nil {
			return err
		}
	}

	return nil
}

// readBinaryString reads a string that may contain binary data
func (p *RDBParser) readBinaryString() ([]byte, error) {
	// Use the same string reading logic but handle binary data
	return p.readString()
}

// readCompressedString reads a compressed string (LZF format)
func (p *RDBParser) readCompressedString() ([]byte, error) {
	// Read compressed length
	compressedLen, err := p.readLength()
	if err != nil {
		return nil, fmt.Errorf("failed to read compressed length: %w", err)
	}
	
	// Read uncompressed length  
	uncompressedLen, err := p.readLength()
	if err != nil {
		return nil, fmt.Errorf("failed to read uncompressed length: %w", err)
	}
	
	// Read compressed data
	compressedData := make([]byte, compressedLen)
	if _, err := io.ReadFull(p.br, compressedData); err != nil {
		return nil, fmt.Errorf("failed to read compressed data: %w", err)
	}
	
	// For now, return the compressed data as-is since LZF decompression
	// would require external library. In practice, many Redis instances
	// don't use compression for small strings.
	p.logDebug("LZF compressed string detected", 
		"compressed_len", compressedLen, 
		"uncompressed_len", uncompressedLen)
	
	// TODO: Implement LZF decompression or return compressed data
	// For compatibility, return as binary data
	return compressedData, nil
}
func (p *RDBParser) readString() ([]byte, error) {
	// First byte determines the encoding
	b, err := p.br.ReadByte()
	if err != nil {
		return nil, err
	}

	switch (b & 0xC0) >> 6 {
	case 0:
		// 6-bit length
		length := uint64(b & 0x3F)
		return p.readStringData(length)

	case 1:
		// 14-bit length
		b2, err := p.br.ReadByte()
		if err != nil {
			return nil, err
		}
		length := uint64(b&0x3F)<<8 | uint64(b2)
		return p.readStringData(length)

	case 2:
		// 32-bit length
		var length uint32
		if err := binary.Read(p.br, binary.BigEndian, &length); err != nil {
			return nil, err
		}
		return p.readStringData(uint64(length))

	case 3:
		// Special encoding - string content is encoded as integer or special format
		encoding := b & 0x3F
		switch encoding {
		case 0:
			// 8-bit integer
			val, err := p.br.ReadByte()
			if err != nil {
				return nil, err
			}
			return []byte(fmt.Sprintf("%d", val)), nil
		case 1:
			// 16-bit integer
			var val uint16
			if err := binary.Read(p.br, binary.LittleEndian, &val); err != nil {
				return nil, err
			}
			return []byte(fmt.Sprintf("%d", val)), nil
		case 2:
			// 32-bit integer  
			var val int32  // Use signed integer
			if err := binary.Read(p.br, binary.LittleEndian, &val); err != nil {
				return nil, err
			}
			return []byte(fmt.Sprintf("%d", val)), nil
		case 3:
			// Compressed string (LZF) - Redis 7+ feature
			return p.readCompressedString()
		case 33: // 0x21
			// 64-bit integer (Redis 7+ extended encoding)
			var val int64
			if err := binary.Read(p.br, binary.LittleEndian, &val); err != nil {
				return nil, err
			}
			return []byte(fmt.Sprintf("%d", val)), nil
		default:
			// For unknown encodings in newer Redis versions, attempt graceful handling
			if p.canSkipError() {
				p.logDebug("Unknown special string encoding, attempting to skip", "encoding", encoding)
				// Try to read as a regular string with small length assumption
				if encoding >= 32 && encoding <= 63 {
					// These might be Redis 8+ encodings, try to read 8 bytes and treat as number
					data := make([]byte, 8)
					if _, err := p.br.Read(data); err == nil {
						return data, nil
					}
				}
				return []byte{}, nil // Return empty string as fallback
			}
			return nil, fmt.Errorf("invalid special string encoding: %d", encoding)
		}
	}

	return nil, fmt.Errorf("invalid string encoding: %d", (b&0xC0)>>6)
}

func (p *RDBParser) readStringData(length uint64) ([]byte, error) {
	// Handle empty string
	if length == 0 {
		return []byte{}, nil
	}

	// Guard against extremely large strings that might indicate parser errors
	if length > 100000 { // 100KB is reasonable max for RDB strings
		return nil, fmt.Errorf("string length too large: %d", length)
	}

	// Allocate exact buffer size needed for the string data
	data := make([]byte, length)
	if _, err := io.ReadFull(p.br, data); err != nil {
		return nil, fmt.Errorf("failed to read string data: %w", err)
	}

	return data, nil
}

// readValue reads a value based on its type
func (p *RDBParser) readValue(valueType byte) (interface{}, error) {
	switch valueType {
	case RDBTypeString:
		return p.readString()

	case RDBTypeList:
		length, err := p.readLength()
		if err != nil {
			return nil, err
		}

		list := make([][]byte, length)
		for i := uint64(0); i < length; i++ {
			element, err := p.readString()
			if err != nil {
				return nil, err
			}
			list[i] = element
		}
		return list, nil

	case RDBTypeSet:
		length, err := p.readLength()
		if err != nil {
			return nil, err
		}

		set := make(map[string]struct{})
		for i := uint64(0); i < length; i++ {
			member, err := p.readString()
			if err != nil {
				return nil, err
			}
			set[string(member)] = struct{}{}
		}
		return set, nil

	case RDBTypeHash:
		length, err := p.readLength()
		if err != nil {
			return nil, err
		}

		hash := make(map[string][]byte)
		for i := uint64(0); i < length; i++ {
			field, err := p.readString()
			if err != nil {
				return nil, err
			}
			value, err := p.readString()
			if err != nil {
				return nil, err
			}
			hash[string(field)] = value
		}
		return hash, nil

	case RDBTypeListQuicklist:
		// Quick list format (Redis 3.2+)
		return p.readQuicklist()

	case RDBTypeStreamListpacks, RDBTypeStreamListpacks2, RDBTypeStreamListpacks3:
		// Stream formats - skip for now but don't fail
		return p.skipStreamData()

	default:
		// For unsupported types, use adaptive strategy
		return p.skipUnsupportedType(valueType)
	}
}

// readQuicklist reads a quicklist structure
func (p *RDBParser) readQuicklist() (interface{}, error) {
	length, err := p.readLength()
	if err != nil {
		return nil, err
	}

	var allElements [][]byte
	for i := uint64(0); i < length; i++ {
		// Each quicklist node is a ziplist
		ziplistData, err := p.readString()
		if err != nil {
			if p.canSkipError() {
				continue
			}
			return nil, err
		}
		
		// For simplicity, treat ziplist as a single element
		// A full implementation would decompress the ziplist
		allElements = append(allElements, ziplistData)
	}

	return allElements, nil
}

// skipStreamData skips stream data structures
func (p *RDBParser) skipStreamData() (interface{}, error) {
	// Read length and skip the data
	length, err := p.readLength()
	if err != nil {
		return nil, err
	}

	// Skip the stream data by reading and discarding
	for i := uint64(0); i < length; i++ {
		if _, err := p.readString(); err != nil {
			if p.canSkipError() {
				continue
			}
			return nil, err
		}
	}

	return nil, nil // Return nil to indicate skipped
}

// skipUnsupportedType tries to skip unsupported RDB types gracefully
func (p *RDBParser) skipUnsupportedType(valueType byte) (interface{}, error) {
	// Strategy depends on the type value
	if valueType < 16 {
		// Simple types - try to read as string and discard
		_, err := p.readString()
		if err != nil {
			if p.canSkipError() {
				return nil, nil
			}
			return nil, fmt.Errorf("failed to skip unsupported RDB type %d: %w", valueType, err)
		}
		return nil, nil // Return nil to indicate skipped value
	}
	
	if valueType < 32 {
		// Encoded types - try to read length then skip data
		length, err := p.readLength()
		if err != nil {
			if p.canSkipError() {
				return nil, nil
			}
			return nil, fmt.Errorf("failed to read length for unsupported type %d: %w", valueType, err)
		}
		
		// Skip the data
		skipData := make([]byte, length)
		if _, err := io.ReadFull(p.br, skipData); err != nil {
			if p.canSkipError() {
				return nil, nil
			}
			return nil, fmt.Errorf("failed to skip data for type %d: %w", valueType, err)
		}
		
		return nil, nil // Return nil to indicate skipped value
	}
	
	// For very unknown types, if we can skip errors, do so
	if p.canSkipError() {
		return nil, nil
	}
	
	// Only fail hard if we're in strict parsing mode
	if p.strategy.requiresStrictParsing {
		return nil, fmt.Errorf("unsupported RDB type: %d", valueType)
	}
	
	return nil, nil // Skip silently in permissive mode
}

// ParseRDB is a convenience function to parse an RDB stream
func ParseRDB(r io.Reader, handler RDBHandler) error {
	parser := NewRDBParser(r, handler)
	return parser.Parse()
}
