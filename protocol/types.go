package protocol

import (
	"fmt"
	"strconv"
	"strings"
)

// ValueType represents the type of a RESP value
type ValueType byte

const (
	// RESP value types
	TypeSimpleString ValueType = '+'
	TypeError        ValueType = '-'
	TypeInteger      ValueType = ':'
	TypeBulkString   ValueType = '$'
	TypeArray        ValueType = '*'
	TypeNull         ValueType = 'N' // RESP3 null
)

// Value represents a parsed RESP value
type Value struct {
	Type     ValueType
	Data     []byte
	Integer  int64
	Array    []Value
	IsNull   bool
}

// String returns a string representation of the value
func (v Value) String() string {
	switch v.Type {
	case TypeSimpleString:
		return string(v.Data)
	case TypeError:
		return string(v.Data)
	case TypeInteger:
		return strconv.FormatInt(v.Integer, 10)
	case TypeBulkString:
		if v.IsNull {
			return "(nil)"
		}
		return string(v.Data)
	case TypeArray:
		if v.IsNull {
			return "(nil)"
		}
		parts := make([]string, len(v.Array))
		for i, item := range v.Array {
			parts[i] = item.String()
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case TypeNull:
		return "(nil)"
	default:
		return fmt.Sprintf("unknown type %c", v.Type)
	}
}

// Bytes returns the byte representation of the value
func (v Value) Bytes() []byte {
	return v.Data
}

// Int returns the integer value, or 0 if not an integer
func (v Value) Int() int64 {
	return v.Integer
}

// IsError returns true if this is an error value
func (v Value) IsError() bool {
	return v.Type == TypeError
}

// Error returns the error message if this is an error value
func (v Value) Error() string {
	if v.Type == TypeError {
		return string(v.Data)
	}
	return ""
}

// Command represents a Redis command parsed from a RESP array
type Command struct {
	Name string
	Args [][]byte
}

// ParseCommand parses a RESP array value into a Command
func ParseCommand(v Value) (*Command, error) {
	if v.Type != TypeArray || len(v.Array) == 0 {
		return nil, fmt.Errorf("invalid command format")
	}
	
	cmd := &Command{
		Args: make([][]byte, len(v.Array)-1),
	}
	
	// First element is the command name
	if v.Array[0].Type != TypeBulkString {
		return nil, fmt.Errorf("command name must be bulk string")
	}
	cmd.Name = strings.ToUpper(string(v.Array[0].Data))
	
	// Remaining elements are arguments
	for i := 1; i < len(v.Array); i++ {
		if v.Array[i].Type != TypeBulkString {
			return nil, fmt.Errorf("command arguments must be bulk strings")
		}
		cmd.Args[i-1] = v.Array[i].Data
	}
	
	return cmd, nil
}

// String returns a string representation of the command
func (c *Command) String() string {
	args := make([]string, len(c.Args))
	for i, arg := range c.Args {
		args[i] = string(arg)
	}
	return c.Name + " " + strings.Join(args, " ")
}