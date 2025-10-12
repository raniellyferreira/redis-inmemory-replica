package protocol

import (
	"bytes"
	"testing"
)

// BenchmarkReaderParseSimpleString benchmarks parsing simple strings
func BenchmarkReaderParseSimpleString(b *testing.B) {
	input := []byte("+OK\r\n")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		r := NewReader(bytes.NewReader(input))
		_, err := r.ReadNext()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkReaderParseError benchmarks parsing error messages
func BenchmarkReaderParseError(b *testing.B) {
	input := []byte("-ERR unknown command\r\n")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		r := NewReader(bytes.NewReader(input))
		_, err := r.ReadNext()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkReaderParseInteger benchmarks parsing integers
func BenchmarkReaderParseInteger(b *testing.B) {
	input := []byte(":1234567890\r\n")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		r := NewReader(bytes.NewReader(input))
		_, err := r.ReadNext()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkReaderParseBulkString benchmarks parsing bulk strings
func BenchmarkReaderParseBulkString(b *testing.B) {
	sizes := []struct {
		name string
		data []byte
	}{
		{"Small_16B", bytes.Repeat([]byte("x"), 16)},
		{"Medium_1KB", bytes.Repeat([]byte("x"), 1024)},
		{"Large_64KB", bytes.Repeat([]byte("x"), 64*1024)},
	}

	for _, size := range sizes {
		b.Run(size.name, func(b *testing.B) {
			var buf bytes.Buffer
			buf.WriteString("$")
			buf.WriteString(string(itoa(len(size.data))))
			buf.WriteString("\r\n")
			buf.Write(size.data)
			buf.WriteString("\r\n")
			input := buf.Bytes()

			b.ResetTimer()
			b.ReportAllocs()
			b.SetBytes(int64(len(size.data)))

			for i := 0; i < b.N; i++ {
				r := NewReader(bytes.NewReader(input))
				_, err := r.ReadNext()
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkReaderParseArray benchmarks parsing arrays
func BenchmarkReaderParseArray(b *testing.B) {
	scenarios := []struct {
		name  string
		input []byte
	}{
		{
			name:  "Empty",
			input: []byte("*0\r\n"),
		},
		{
			name:  "SmallArray_3",
			input: []byte("*3\r\n$3\r\nSET\r\n$3\r\nkey\r\n$5\r\nvalue\r\n"),
		},
		{
			name:  "MediumArray_10",
			input: []byte("*10\r\n$1\r\n1\r\n$1\r\n2\r\n$1\r\n3\r\n$1\r\n4\r\n$1\r\n5\r\n$1\r\n6\r\n$1\r\n7\r\n$1\r\n8\r\n$1\r\n9\r\n$2\r\n10\r\n"),
		},
	}

	for _, sc := range scenarios {
		b.Run(sc.name, func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				r := NewReader(bytes.NewReader(sc.input))
				_, err := r.ReadNext()
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkReaderParseCommand benchmarks parsing common Redis commands
func BenchmarkReaderParseCommand(b *testing.B) {
	commands := []struct {
		name  string
		input []byte
	}{
		{
			name:  "GET",
			input: []byte("*2\r\n$3\r\nGET\r\n$3\r\nkey\r\n"),
		},
		{
			name:  "SET",
			input: []byte("*3\r\n$3\r\nSET\r\n$3\r\nkey\r\n$5\r\nvalue\r\n"),
		},
		{
			name:  "SET_EX",
			input: []byte("*5\r\n$3\r\nSET\r\n$3\r\nkey\r\n$5\r\nvalue\r\n$2\r\nEX\r\n$2\r\n60\r\n"),
		},
		{
			name:  "KEYS",
			input: []byte("*2\r\n$4\r\nKEYS\r\n$1\r\n*\r\n"),
		},
	}

	for _, cmd := range commands {
		b.Run(cmd.name, func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				r := NewReader(bytes.NewReader(cmd.input))
				_, err := r.ReadNext()
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkWriterSimpleString benchmarks writing simple strings
func BenchmarkWriterSimpleString(b *testing.B) {
	var buf bytes.Buffer

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		buf.Reset()
		w := NewWriter(&buf)
		if err := w.WriteSimpleString("OK"); err != nil {
			b.Fatal(err)
		}
		if err := w.Flush(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkWriterInteger benchmarks writing integers
func BenchmarkWriterInteger(b *testing.B) {
	var buf bytes.Buffer

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		buf.Reset()
		w := NewWriter(&buf)
		if err := w.WriteInteger(1234567890); err != nil {
			b.Fatal(err)
		}
		if err := w.Flush(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkWriterBulkString benchmarks writing bulk strings
func BenchmarkWriterBulkString(b *testing.B) {
	sizes := []struct {
		name string
		data []byte
	}{
		{"Small_16B", bytes.Repeat([]byte("x"), 16)},
		{"Medium_1KB", bytes.Repeat([]byte("x"), 1024)},
		{"Large_64KB", bytes.Repeat([]byte("x"), 64*1024)},
	}

	for _, size := range sizes {
		b.Run(size.name, func(b *testing.B) {
			var buf bytes.Buffer

			b.ResetTimer()
			b.ReportAllocs()
			b.SetBytes(int64(len(size.data)))

			for i := 0; i < b.N; i++ {
				buf.Reset()
				w := NewWriter(&buf)
				if err := w.WriteBulkString(size.data); err != nil {
					b.Fatal(err)
				}
				if err := w.Flush(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkWriterArray benchmarks writing arrays
func BenchmarkWriterArray(b *testing.B) {
	scenarios := []struct {
		name   string
		values []Value
	}{
		{
			name:   "Empty",
			values: []Value{},
		},
		{
			name: "SmallArray_3",
			values: []Value{
				{Type: TypeBulkString, Data: []byte("SET")},
				{Type: TypeBulkString, Data: []byte("key")},
				{Type: TypeBulkString, Data: []byte("value")},
			},
		},
		{
			name: "MediumArray_10",
			values: func() []Value {
				vals := make([]Value, 10)
				for i := 0; i < 10; i++ {
					vals[i] = Value{Type: TypeInteger, Integer: int64(i)}
				}
				return vals
			}(),
		},
	}

	for _, sc := range scenarios {
		b.Run(sc.name, func(b *testing.B) {
			var buf bytes.Buffer

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				buf.Reset()
				w := NewWriter(&buf)
				if err := w.WriteArray(sc.values); err != nil {
					b.Fatal(err)
				}
				if err := w.Flush(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkWriterCommand benchmarks writing Redis commands
func BenchmarkWriterCommand(b *testing.B) {
	commands := []struct {
		name string
		cmd  string
		args []string
	}{
		{
			name: "GET",
			cmd:  "GET",
			args: []string{"key"},
		},
		{
			name: "SET",
			cmd:  "SET",
			args: []string{"key", "value"},
		},
		{
			name: "SET_EX",
			cmd:  "SET",
			args: []string{"key", "value", "EX", "60"},
		},
	}

	for _, cmd := range commands {
		b.Run(cmd.name, func(b *testing.B) {
			var buf bytes.Buffer

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				buf.Reset()
				w := NewWriter(&buf)
				if err := w.WriteCommand(cmd.cmd, cmd.args...); err != nil {
					b.Fatal(err)
				}
				if err := w.Flush(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// Helper function to convert int to string (avoiding strconv import in bench)
func itoa(n int) string {
	if n == 0 {
		return "0"
	}

	var buf [20]byte
	i := len(buf) - 1
	neg := n < 0
	if neg {
		n = -n
	}

	for n > 0 {
		buf[i] = byte('0' + n%10)
		n /= 10
		i--
	}

	if neg {
		buf[i] = '-'
		i--
	}

	return string(buf[i+1:])
}

// BenchmarkReaderParseBatch benchmarks parsing multiple commands in sequence (batch operations)
func BenchmarkReaderParseBatch(b *testing.B) {
	scenarios := []struct {
		name       string
		batchSize  int
		cmdBuilder func(int) []byte
	}{
		{
			name:      "BatchGET_10",
			batchSize: 10,
			cmdBuilder: func(i int) []byte {
				key := "key" + itoa(i%10)
				var buf bytes.Buffer
				buf.WriteString("*2\r\n$3\r\nGET\r\n$")
				buf.WriteString(itoa(len(key)))
				buf.WriteString("\r\n")
				buf.WriteString(key)
				buf.WriteString("\r\n")
				return buf.Bytes()
			},
		},
		{
			name:      "BatchSET_10",
			batchSize: 10,
			cmdBuilder: func(i int) []byte {
				key := "key" + itoa(i%10)
				var buf bytes.Buffer
				buf.WriteString("*3\r\n$3\r\nSET\r\n$")
				buf.WriteString(itoa(len(key)))
				buf.WriteString("\r\n")
				buf.WriteString(key)
				buf.WriteString("\r\n$5\r\nvalue\r\n")
				return buf.Bytes()
			},
		},
		{
			name:      "BatchGET_100",
			batchSize: 100,
			cmdBuilder: func(i int) []byte {
				key := "key" + itoa(i%100)
				var buf bytes.Buffer
				buf.WriteString("*2\r\n$3\r\nGET\r\n$")
				buf.WriteString(itoa(len(key)))
				buf.WriteString("\r\n")
				buf.WriteString(key)
				buf.WriteString("\r\n")
				return buf.Bytes()
			},
		},
		{
			name:      "BatchSET_100",
			batchSize: 100,
			cmdBuilder: func(i int) []byte {
				key := "key" + itoa(i%100)
				var buf bytes.Buffer
				buf.WriteString("*3\r\n$3\r\nSET\r\n$")
				buf.WriteString(itoa(len(key)))
				buf.WriteString("\r\n")
				buf.WriteString(key)
				buf.WriteString("\r\n$5\r\nvalue\r\n")
				return buf.Bytes()
			},
		},
		{
			name:      "BatchMixed_50",
			batchSize: 50,
			cmdBuilder: func(i int) []byte {
				key := "key" + itoa(i%50)
				var buf bytes.Buffer
				if i%2 == 0 {
					buf.WriteString("*2\r\n$3\r\nGET\r\n$")
					buf.WriteString(itoa(len(key)))
					buf.WriteString("\r\n")
					buf.WriteString(key)
					buf.WriteString("\r\n")
				} else {
					buf.WriteString("*3\r\n$3\r\nSET\r\n$")
					buf.WriteString(itoa(len(key)))
					buf.WriteString("\r\n")
					buf.WriteString(key)
					buf.WriteString("\r\n$5\r\nvalue\r\n")
				}
				return buf.Bytes()
			},
		},
	}

	for _, sc := range scenarios {
		b.Run(sc.name, func(b *testing.B) {
			// Pre-build the batch
			var buf bytes.Buffer
			for i := 0; i < sc.batchSize; i++ {
				buf.Write(sc.cmdBuilder(i))
			}
			batchData := buf.Bytes()

			b.ResetTimer()
			b.ReportAllocs()
			b.SetBytes(int64(len(batchData)))

			for i := 0; i < b.N; i++ {
				r := NewReader(bytes.NewReader(batchData))
				for j := 0; j < sc.batchSize; j++ {
					_, err := r.ReadNext()
					if err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}

// BenchmarkWriterBatch benchmarks writing multiple commands in sequence (batch operations)
func BenchmarkWriterBatch(b *testing.B) {
	scenarios := []struct {
		name      string
		batchSize int
		writeFunc func(*Writer, int) error
	}{
		{
			name:      "BatchSimpleString_10",
			batchSize: 10,
			writeFunc: func(w *Writer, i int) error {
				return w.WriteSimpleString("OK")
			},
		},
		{
			name:      "BatchInteger_10",
			batchSize: 10,
			writeFunc: func(w *Writer, i int) error {
				return w.WriteInteger(int64(i))
			},
		},
		{
			name:      "BatchBulkString_10",
			batchSize: 10,
			writeFunc: func(w *Writer, i int) error {
				return w.WriteBulkString([]byte("value"))
			},
		},
		{
			name:      "BatchCommand_50",
			batchSize: 50,
			writeFunc: func(w *Writer, i int) error {
				if i%2 == 0 {
					return w.WriteCommand("GET", "key")
				}
				return w.WriteCommand("SET", "key", "value")
			},
		},
		{
			name:      "BatchCommand_100",
			batchSize: 100,
			writeFunc: func(w *Writer, i int) error {
				return w.WriteCommand("SET", "key", "value")
			},
		},
	}

	for _, sc := range scenarios {
		b.Run(sc.name, func(b *testing.B) {
			var buf bytes.Buffer

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				buf.Reset()
				w := NewWriter(&buf)

				for j := 0; j < sc.batchSize; j++ {
					if err := sc.writeFunc(w, j); err != nil {
						b.Fatal(err)
					}
				}

				if err := w.Flush(); err != nil {
					b.Fatal(err)
				}
			}

			b.SetBytes(int64(buf.Len()))
		})
	}
}
