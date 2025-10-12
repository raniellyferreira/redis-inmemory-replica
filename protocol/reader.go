package protocol

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"
)

const (
	// CRLF is the Redis protocol line terminator
	CRLF = "\r\n"

	// maxBulkSize is the maximum size for bulk strings (1GB)
	maxBulkSize = 1024 * 1024 * 1024

	// maxArraySize is the maximum size for arrays
	maxArraySize = 1024 * 1024
)

var (
	crlfBytes = []byte(CRLF)
)

// Reader is a streaming RESP protocol reader that efficiently parses
// Redis protocol messages without unnecessary memory allocations
type Reader struct {
	br      *bufio.Reader
	scratch []byte // Reusable buffer for reading
}

// NewReader creates a new streaming RESP reader
func NewReader(r io.Reader) *Reader {
	return &Reader{
		br:      bufio.NewReader(r),
		scratch: make([]byte, 0, 512), // Initial capacity for scratch buffer
	}
}

// ReadNext reads the next RESP value from the stream
func (r *Reader) ReadNext() (Value, error) {
	typeByte, err := r.br.ReadByte()
	if err != nil {
		return Value{}, err
	}

	switch ValueType(typeByte) {
	case TypeSimpleString:
		return r.readSimpleString()
	case TypeError:
		return r.readError()
	case TypeInteger:
		return r.readInteger()
	case TypeBulkString:
		return r.readBulkString()
	case TypeArray:
		return r.readArray()
	default:
		if typeByte == 0 {
			return Value{}, fmt.Errorf("unknown RESP type: empty byte (connection may be closed)")
		}
		return Value{}, fmt.Errorf("unknown RESP type: %c (0x%02x)", typeByte, typeByte)
	}
}

// readSimpleString reads a simple string value
func (r *Reader) readSimpleString() (Value, error) {
	line, err := r.readLine()
	if err != nil {
		return Value{}, err
	}

	return Value{
		Type: TypeSimpleString,
		Data: line,
	}, nil
}

// readError reads an error value
func (r *Reader) readError() (Value, error) {
	line, err := r.readLine()
	if err != nil {
		return Value{}, err
	}

	return Value{
		Type: TypeError,
		Data: line,
	}, nil
}

// readInteger reads an integer value
func (r *Reader) readInteger() (Value, error) {
	line, err := r.readLine()
	if err != nil {
		return Value{}, err
	}

	integer, err := parseInt64(line)
	if err != nil {
		return Value{}, fmt.Errorf("invalid integer: %s", line)
	}

	return Value{
		Type:    TypeInteger,
		Integer: integer,
	}, nil
}

// parseInt64 parses an int64 from a byte slice without allocation
func parseInt64(b []byte) (int64, error) {
	if len(b) == 0 {
		return 0, strconv.ErrSyntax
	}

	var neg bool
	var i int

	switch b[0] {
	case '-':
		neg = true
		i = 1
	case '+':
		i = 1
	default:
		i = 0
	}

	if i >= len(b) {
		return 0, strconv.ErrSyntax
	}

	var n int64
	for ; i < len(b); i++ {
		if b[i] < '0' || b[i] > '9' {
			return 0, strconv.ErrSyntax
		}

		// Check for overflow
		if n > (1<<63-1)/10 {
			return 0, strconv.ErrRange
		}

		n = n*10 + int64(b[i]-'0')
	}

	if neg {
		return -n, nil
	}
	return n, nil
}

// readBulkString reads a bulk string value
func (r *Reader) readBulkString() (Value, error) {
	line, err := r.readLine()
	if err != nil {
		return Value{}, err
	}

	length, err := parseInt64(line)
	if err != nil {
		return Value{}, fmt.Errorf("invalid bulk string length: %s", line)
	}

	// Handle null bulk string
	if length == -1 {
		return Value{
			Type:   TypeBulkString,
			IsNull: true,
		}, nil
	}

	// Validate length
	if length < 0 || length > maxBulkSize {
		return Value{}, fmt.Errorf("invalid bulk string length: %d", length)
	}

	// Read the string data plus CRLF
	data := make([]byte, length)
	if _, err := io.ReadFull(r.br, data); err != nil {
		return Value{}, err
	}

	// Read and validate CRLF
	if err := r.expectCRLF(); err != nil {
		return Value{}, err
	}

	return Value{
		Type: TypeBulkString,
		Data: data,
	}, nil
}

// readArray reads an array value
func (r *Reader) readArray() (Value, error) {
	line, err := r.readLine()
	if err != nil {
		return Value{}, err
	}

	length, err := parseInt64(line)
	if err != nil {
		return Value{}, fmt.Errorf("invalid array length: %s", line)
	}

	// Handle null array
	if length == -1 {
		return Value{
			Type:   TypeArray,
			IsNull: true,
		}, nil
	}

	// Validate length
	if length < 0 || length > maxArraySize {
		return Value{}, fmt.Errorf("invalid array length: %d", length)
	}

	// Read array elements
	array := make([]Value, length)
	for i := int64(0); i < length; i++ {
		value, err := r.ReadNext()
		if err != nil {
			return Value{}, err
		}
		array[i] = value
	}

	return Value{
		Type:  TypeArray,
		Array: array,
	}, nil
}

// ReadBulkString reads a bulk string in chunks, calling fn for each chunk
// This is useful for handling large bulk strings without loading them entirely into memory
func (r *Reader) ReadBulkString(fn func(chunk []byte) error) error {
	// Read the '$' type byte if not already read
	typeByte, err := r.br.ReadByte()
	if err != nil {
		return err
	}

	if ValueType(typeByte) != TypeBulkString {
		return fmt.Errorf("expected bulk string, got %c", typeByte)
	}

	// Read length
	line, err := r.readLine()
	if err != nil {
		return err
	}

	length, err := parseInt64(line)
	if err != nil {
		return fmt.Errorf("invalid bulk string length: %s", line)
	}

	// Handle null bulk string
	if length == -1 {
		return fn(nil)
	}

	// Validate length
	if length < 0 || length > maxBulkSize {
		return fmt.Errorf("invalid bulk string length: %d", length)
	}

	// Read data in chunks
	const chunkSize = 8192
	buffer := make([]byte, chunkSize)
	remaining := length

	for remaining > 0 {
		toRead := chunkSize
		if remaining < int64(chunkSize) {
			toRead = int(remaining)
		}

		n, err := io.ReadFull(r.br, buffer[:toRead])
		if err != nil {
			return err
		}

		if err := fn(buffer[:n]); err != nil {
			return err
		}

		remaining -= int64(n)
	}

	// Read and validate CRLF
	return r.expectCRLF()
}

// ReadBulkStringForReplication reads a bulk string for replication context.
// This is specifically for RDB data in replication where there's no CRLF after the data.
func (r *Reader) ReadBulkStringForReplication(fn func(chunk []byte) error) error {
	// Read the '$' type byte if not already read
	typeByte, err := r.br.ReadByte()
	if err != nil {
		return err
	}

	if ValueType(typeByte) != TypeBulkString {
		return fmt.Errorf("expected bulk string, got %c", typeByte)
	}

	// Read length
	line, err := r.readLine()
	if err != nil {
		return err
	}

	length, err := parseInt64(line)
	if err != nil {
		return fmt.Errorf("invalid bulk string length: %s", line)
	}

	// Handle null bulk string
	if length == -1 {
		return fn(nil)
	}

	// Validate length
	if length < 0 || length > maxBulkSize {
		return fmt.Errorf("invalid bulk string length: %d", length)
	}

	// Read data in chunks
	const chunkSize = 8192
	buffer := make([]byte, chunkSize)
	remaining := length

	for remaining > 0 {
		toRead := chunkSize
		if remaining < int64(chunkSize) {
			toRead = int(remaining)
		}

		n, err := io.ReadFull(r.br, buffer[:toRead])
		if err != nil {
			return err
		}

		if err := fn(buffer[:n]); err != nil {
			return err
		}

		remaining -= int64(n)
	}

	// No CRLF expected after RDB data in replication context
	return nil
}

// Skip skips the next RESP value without parsing it completely
func (r *Reader) Skip() error {
	typeByte, err := r.br.ReadByte()
	if err != nil {
		return err
	}

	switch ValueType(typeByte) {
	case TypeSimpleString, TypeError:
		_, err := r.readLine()
		return err

	case TypeInteger:
		_, err := r.readLine()
		return err

	case TypeBulkString:
		line, err := r.readLine()
		if err != nil {
			return err
		}

		length, err := parseInt64(line)
		if err != nil {
			return fmt.Errorf("invalid bulk string length: %s", line)
		}

		if length == -1 {
			return nil // null bulk string
		}

		if length < 0 || length > maxBulkSize {
			return fmt.Errorf("invalid bulk string length: %d", length)
		}

		// Skip the data and CRLF
		if _, err := r.br.Discard(int(length) + 2); err != nil {
			return err
		}
		return nil

	case TypeArray:
		line, err := r.readLine()
		if err != nil {
			return err
		}

		length, err := parseInt64(line)
		if err != nil {
			return fmt.Errorf("invalid array length: %s", line)
		}

		if length == -1 {
			return nil // null array
		}

		if length < 0 || length > maxArraySize {
			return fmt.Errorf("invalid array length: %d", length)
		}

		// Skip array elements recursively
		for i := int64(0); i < length; i++ {
			if err := r.Skip(); err != nil {
				return err
			}
		}
		return nil

	default:
		return fmt.Errorf("unknown RESP type: %c", typeByte)
	}
}

// readLine reads a line terminated by CRLF
func (r *Reader) readLine() ([]byte, error) {
	line, err := r.br.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read line: %w", err)
	}

	// Remove CRLF - must have at least \r\n
	if len(line) < 2 {
		return nil, fmt.Errorf("line too short (%d bytes), expected CRLF terminator", len(line))
	}

	if !bytes.HasSuffix(line, crlfBytes) {
		// Provide detailed error about what we actually received
		if len(line) >= 2 {
			lastTwo := line[len(line)-2:]
			return nil, fmt.Errorf("missing CRLF terminator, got [%d, %d] instead of [13, 10]", lastTwo[0], lastTwo[1])
		}
		return nil, fmt.Errorf("missing CRLF terminator, line ends with [%d]", line[len(line)-1])
	}

	return line[:len(line)-2], nil
}

// expectCRLF reads and validates CRLF terminator
func (r *Reader) expectCRLF() error {
	crlf := make([]byte, 2)
	n, err := io.ReadFull(r.br, crlf)
	if err != nil {
		return fmt.Errorf("failed to read CRLF terminator (read %d/2 bytes): %w", n, err)
	}

	if !bytes.Equal(crlf, crlfBytes) {
		return fmt.Errorf("expected CRLF terminator [13, 10], got [%d, %d]", crlf[0], crlf[1])
	}

	return nil
}
