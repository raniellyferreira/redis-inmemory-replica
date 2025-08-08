package protocol_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/raniellyferreira/redis-inmemory-replica/protocol"
)

func TestRESPReader(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected protocol.Value
	}{
		{
			name:  "simple string",
			input: "+OK\r\n",
			expected: protocol.Value{
				Type: protocol.TypeSimpleString,
				Data: []byte("OK"),
			},
		},
		{
			name:  "error",
			input: "-ERR unknown command\r\n",
			expected: protocol.Value{
				Type: protocol.TypeError,
				Data: []byte("ERR unknown command"),
			},
		},
		{
			name:  "integer",
			input: ":42\r\n",
			expected: protocol.Value{
				Type:    protocol.TypeInteger,
				Integer: 42,
			},
		},
		{
			name:  "bulk string",
			input: "$5\r\nhello\r\n",
			expected: protocol.Value{
				Type: protocol.TypeBulkString,
				Data: []byte("hello"),
			},
		},
		{
			name:  "null bulk string",
			input: "$-1\r\n",
			expected: protocol.Value{
				Type:   protocol.TypeBulkString,
				IsNull: true,
			},
		},
		{
			name:  "empty bulk string",
			input: "$0\r\n\r\n",
			expected: protocol.Value{
				Type: protocol.TypeBulkString,
				Data: []byte(""),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := protocol.NewReader(strings.NewReader(tt.input))
			value, err := reader.ReadNext()
			if err != nil {
				t.Fatalf("ReadNext() error = %v", err)
			}

			if value.Type != tt.expected.Type {
				t.Errorf("Type = %v, want %v", value.Type, tt.expected.Type)
			}

			if !bytes.Equal(value.Data, tt.expected.Data) {
				t.Errorf("Data = %v, want %v", value.Data, tt.expected.Data)
			}

			if value.Integer != tt.expected.Integer {
				t.Errorf("Integer = %v, want %v", value.Integer, tt.expected.Integer)
			}

			if value.IsNull != tt.expected.IsNull {
				t.Errorf("IsNull = %v, want %v", value.IsNull, tt.expected.IsNull)
			}
		})
	}
}

func TestRESPArray(t *testing.T) {
	// Test array: ["SET", "key", "value"]
	input := "*3\r\n$3\r\nSET\r\n$3\r\nkey\r\n$5\r\nvalue\r\n"

	reader := protocol.NewReader(strings.NewReader(input))
	value, err := reader.ReadNext()
	if err != nil {
		t.Fatalf("ReadNext() error = %v", err)
	}

	if value.Type != protocol.TypeArray {
		t.Errorf("Type = %v, want %v", value.Type, protocol.TypeArray)
	}

	if len(value.Array) != 3 {
		t.Errorf("Array length = %d, want 3", len(value.Array))
	}

	expectedElements := []string{"SET", "key", "value"}
	for i, expected := range expectedElements {
		if string(value.Array[i].Data) != expected {
			t.Errorf("Array[%d] = %s, want %s", i, string(value.Array[i].Data), expected)
		}
	}
}

func TestRESPWriter(t *testing.T) {
	var buf bytes.Buffer
	writer := protocol.NewWriter(&buf)

	// Test simple string
	err := writer.WriteSimpleString("OK")
	if err != nil {
		t.Fatalf("WriteSimpleString() error = %v", err)
	}
	writer.Flush()

	expected := "+OK\r\n"
	if buf.String() != expected {
		t.Errorf("WriteSimpleString() = %q, want %q", buf.String(), expected)
	}

	// Test bulk string
	buf.Reset()
	err = writer.WriteBulkString([]byte("hello"))
	if err != nil {
		t.Fatalf("WriteBulkString() error = %v", err)
	}
	writer.Flush()

	expected = "$5\r\nhello\r\n"
	if buf.String() != expected {
		t.Errorf("WriteBulkString() = %q, want %q", buf.String(), expected)
	}

	// Test integer
	buf.Reset()
	err = writer.WriteInteger(42)
	if err != nil {
		t.Fatalf("WriteInteger() error = %v", err)
	}
	writer.Flush()

	expected = ":42\r\n"
	if buf.String() != expected {
		t.Errorf("WriteInteger() = %q, want %q", buf.String(), expected)
	}

	// Test command
	buf.Reset()
	err = writer.WriteCommand("SET", "key", "value")
	if err != nil {
		t.Fatalf("WriteCommand() error = %v", err)
	}
	writer.Flush()

	expected = "*3\r\n$3\r\nSET\r\n$3\r\nkey\r\n$5\r\nvalue\r\n"
	if buf.String() != expected {
		t.Errorf("WriteCommand() = %q, want %q", buf.String(), expected)
	}
}

func TestParseCommand(t *testing.T) {
	// Create array value representing ["SET", "key", "value"]
	value := protocol.Value{
		Type: protocol.TypeArray,
		Array: []protocol.Value{
			{Type: protocol.TypeBulkString, Data: []byte("SET")},
			{Type: protocol.TypeBulkString, Data: []byte("key")},
			{Type: protocol.TypeBulkString, Data: []byte("value")},
		},
	}

	cmd, err := protocol.ParseCommand(value)
	if err != nil {
		t.Fatalf("ParseCommand() error = %v", err)
	}

	if cmd.Name != "SET" {
		t.Errorf("Command name = %s, want SET", cmd.Name)
	}

	if len(cmd.Args) != 2 {
		t.Errorf("Args length = %d, want 2", len(cmd.Args))
	}

	if string(cmd.Args[0]) != "key" {
		t.Errorf("Args[0] = %s, want key", string(cmd.Args[0]))
	}

	if string(cmd.Args[1]) != "value" {
		t.Errorf("Args[1] = %s, want value", string(cmd.Args[1]))
	}
}

func TestValueString(t *testing.T) {
	tests := []struct {
		name     string
		value    protocol.Value
		expected string
	}{
		{
			name: "simple string",
			value: protocol.Value{
				Type: protocol.TypeSimpleString,
				Data: []byte("OK"),
			},
			expected: "OK",
		},
		{
			name: "integer",
			value: protocol.Value{
				Type:    protocol.TypeInteger,
				Integer: 42,
			},
			expected: "42",
		},
		{
			name: "null bulk string",
			value: protocol.Value{
				Type:   protocol.TypeBulkString,
				IsNull: true,
			},
			expected: "(nil)",
		},
		{
			name: "error",
			value: protocol.Value{
				Type: protocol.TypeError,
				Data: []byte("ERR unknown command"),
			},
			expected: "ERR unknown command",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.value.String()
			if result != tt.expected {
				t.Errorf("String() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestSkipBulkString(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		expectError      bool
		nextReadExpected string
	}{
		{
			name:             "skip regular bulk string",
			input:            "$5\r\nhello\r\n+OK\r\n",
			expectError:      false,
			nextReadExpected: "OK",
		},
		{
			name:             "skip empty bulk string",
			input:            "$0\r\n\r\n+OK\r\n",
			expectError:      false,
			nextReadExpected: "OK",
		},
		{
			name:             "skip null bulk string",
			input:            "$-1\r\n+OK\r\n",
			expectError:      false,
			nextReadExpected: "OK",
		},
		{
			name:             "skip large bulk string",
			input:            "$10\r\n0123456789\r\n+OK\r\n",
			expectError:      false,
			nextReadExpected: "OK",
		},
		{
			name:             "skip bulk string with binary data",
			input:            "$6\r\n\x00\x01\x02\xff\xfe\xfd\r\n+OK\r\n",
			expectError:      false,
			nextReadExpected: "OK",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := protocol.NewReader(strings.NewReader(tt.input))

			// Skip the first value (bulk string)
			err := reader.Skip()
			if tt.expectError && err == nil {
				t.Errorf("Skip() expected error but got none")
			} else if !tt.expectError && err != nil {
				t.Errorf("Skip() unexpected error = %v", err)
			}

			if !tt.expectError {
				// Read the next value to verify skip worked correctly
				value, err := reader.ReadNext()
				if err != nil {
					t.Fatalf("ReadNext() after Skip() error = %v", err)
				}

				if value.Type != protocol.TypeSimpleString {
					t.Errorf("Next value type = %v, want %v", value.Type, protocol.TypeSimpleString)
				}

				if string(value.Data) != tt.nextReadExpected {
					t.Errorf("Next value = %q, want %q", string(value.Data), tt.nextReadExpected)
				}
			}
		})
	}
}

func BenchmarkRESPReader(b *testing.B) {
	input := "+OK\r\n"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := protocol.NewReader(strings.NewReader(input))
		_, err := reader.ReadNext()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRESPWriter(b *testing.B) {
	var buf bytes.Buffer
	writer := protocol.NewWriter(&buf)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		writer.Reset(&buf)
		err := writer.WriteSimpleString("OK")
		if err != nil {
			b.Fatal(err)
		}
		writer.Flush()
	}
}
