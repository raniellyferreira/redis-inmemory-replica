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
)

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
	reader  io.Reader
	handler RDBHandler
	br      *bufio.Reader
}

// NewRDBParser creates a new RDB parser
func NewRDBParser(r io.Reader, handler RDBHandler) *RDBParser {
	return &RDBParser{
		reader:  r,
		handler: handler,
		br:      bufio.NewReader(r),
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

	if version > RDBVersion9 {
		return fmt.Errorf("unsupported RDB version: %d", version)
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
				return fmt.Errorf("failed to read expiry timestamp: %w", err)
			}
			t := time.Unix(int64(timestamp), 0)
			expiry = &t

		case RDBOpcodeExpiryMs:
			// Expiry in milliseconds
			var timestamp uint64
			if err := binary.Read(p.br, binary.LittleEndian, &timestamp); err != nil {
				return fmt.Errorf("failed to read expiry timestamp: %w", err)
			}
			t := time.Unix(int64(timestamp/1000), int64((timestamp%1000)*1000000))
			expiry = &t

		case RDBOpcodeResizeDB:
			// Database resize hint - skip for now
			if _, err := p.readLength(); err != nil {
				return err
			}
			if _, err := p.readLength(); err != nil {
				return err
			}

		case RDBOpcodeAux:
			// Auxiliary field
			key, err := p.readString()
			if err != nil {
				return fmt.Errorf("failed to read aux key: %w", err)
			}
			value, err := p.readString()
			if err != nil {
				return fmt.Errorf("failed to read aux value: %w", err)
			}
			if err := p.handler.OnAux(key, value); err != nil {
				return err
			}

		default:
			// Key-value pair
			key, err := p.readString()
			if err != nil {
				return fmt.Errorf("failed to read key: %w", err)
			}

			value, err := p.readValue(opcode)
			if err != nil {
				return fmt.Errorf("failed to read value for key %s: %w", key, err)
			}

			if err := p.handler.OnKey(key, value, expiry); err != nil {
				return err
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

// readString reads a length-prefixed string
func (p *RDBParser) readString() ([]byte, error) {
	length, err := p.readLength()
	if err != nil {
		return nil, err
	}

	// Allocate exact buffer size needed for the string data
	// This allocation is necessary as we need to read exactly 'length' bytes
	// from the RDB stream. Buffer pooling could be added here for optimization
	// if needed, but it would complicate the code for minimal gains.
	data := make([]byte, length)
	if _, err := io.ReadFull(p.br, data); err != nil {
		return nil, err
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

	default:
		// For unsupported types, skip the value
		return nil, fmt.Errorf("unsupported RDB type: %d", valueType)
	}
}

// ParseRDB is a convenience function to parse an RDB stream
func ParseRDB(r io.Reader, handler RDBHandler) error {
	parser := NewRDBParser(r, handler)
	return parser.Parse()
}
