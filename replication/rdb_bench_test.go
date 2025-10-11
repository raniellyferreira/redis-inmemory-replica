package replication

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"
)

// mockRDBHandler is a minimal RDB handler for benchmarking
type mockRDBHandler struct {
	keyCount int
}

func (h *mockRDBHandler) OnDatabase(index int) error {
	return nil
}

func (h *mockRDBHandler) OnKey(key []byte, value interface{}, expiry *time.Time) error {
	h.keyCount++
	return nil
}

func (h *mockRDBHandler) OnAux(key, value []byte) error {
	return nil
}

func (h *mockRDBHandler) OnEnd() error {
	return nil
}

// generateMockRDB creates a simple synthetic RDB for benchmarking
func generateMockRDB(keyCount int, valueSize int) []byte {
	var buf bytes.Buffer
	
	// RDB header
	buf.WriteString("REDIS0011")
	
	// AUX field: redis-ver
	buf.WriteByte(RDBOpcodeAux)
	writeString(&buf, []byte("redis-ver"))
	writeString(&buf, []byte("7.0.0"))
	
	// Select DB 0
	buf.WriteByte(RDBOpcodeDB)
	buf.WriteByte(0)
	
	// Resize DB hint
	buf.WriteByte(RDBOpcodeResizeDB)
	writeLength(&buf, keyCount)
	writeLength(&buf, 0)
	
	// Generate keys
	value := bytes.Repeat([]byte("x"), valueSize)
	for i := 0; i < keyCount; i++ {
		// String type
		buf.WriteByte(RDBTypeString)
		
		// Key
		key := []byte("key_" + string(rune('0'+i%10)))
		writeString(&buf, key)
		
		// Value
		writeString(&buf, value)
	}
	
	// EOF
	buf.WriteByte(RDBOpcodeEOF)
	
	// Checksum (8 bytes, all zeros for simplicity)
	buf.Write([]byte{0, 0, 0, 0, 0, 0, 0, 0})
	
	return buf.Bytes()
}

// writeString writes a string with length encoding
func writeString(buf *bytes.Buffer, data []byte) {
	writeLength(buf, len(data))
	buf.Write(data)
}

// writeLength writes an integer length
func writeLength(buf *bytes.Buffer, length int) {
	if length < 64 {
		// 6-bit encoding
		buf.WriteByte(byte(length))
	} else if length < 16384 {
		// 14-bit encoding
		buf.WriteByte(byte((length>>8)|0x40))
		buf.WriteByte(byte(length & 0xFF))
	} else {
		// 32-bit encoding
		buf.WriteByte(0x80)
		binary.Write(buf, binary.BigEndian, uint32(length))
	}
}

// BenchmarkRDBIngest benchmarks RDB parsing with synthetic data
func BenchmarkRDBIngest(b *testing.B) {
	scenarios := []struct {
		name      string
		keyCount  int
		valueSize int
	}{
		{"Small_10keys_16B", 10, 16},
		{"Medium_100keys_1KB", 100, 1024},
		{"Large_1000keys_1KB", 1000, 1024},
		{"VeryLarge_10000keys_16B", 10000, 16},
	}
	
	for _, sc := range scenarios {
		b.Run(sc.name, func(b *testing.B) {
			rdbData := generateMockRDB(sc.keyCount, sc.valueSize)
			
			b.ResetTimer()
			b.ReportAllocs()
			b.SetBytes(int64(len(rdbData)))
			
			for i := 0; i < b.N; i++ {
				handler := &mockRDBHandler{}
				parser := NewRDBParser(bytes.NewReader(rdbData), handler)
				if err := parser.Parse(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkRDBParseStrings benchmarks parsing string entries
func BenchmarkRDBParseStrings(b *testing.B) {
	valueSizes := []int{16, 256, 1024, 16384}
	
	for _, size := range valueSizes {
		b.Run(string(rune('0'+size/1000))+"KB", func(b *testing.B) {
			rdbData := generateMockRDB(100, size)
			
			b.ResetTimer()
			b.ReportAllocs()
			
			for i := 0; i < b.N; i++ {
				handler := &mockRDBHandler{}
				parser := NewRDBParser(bytes.NewReader(rdbData), handler)
				if err := parser.Parse(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkRDBParseLists benchmarks parsing list entries
func BenchmarkRDBParseLists(b *testing.B) {
	// Create RDB with list data
	var buf bytes.Buffer
	
	// RDB header
	buf.WriteString("REDIS0011")
	
	// Select DB 0
	buf.WriteByte(RDBOpcodeDB)
	buf.WriteByte(0)
	
	// List with 10 elements
	buf.WriteByte(RDBTypeList)
	writeString(&buf, []byte("mylist"))
	
	// List length
	writeLength(&buf, 10)
	
	// List elements
	for i := 0; i < 10; i++ {
		writeString(&buf, []byte("element"))
	}
	
	// EOF
	buf.WriteByte(RDBOpcodeEOF)
	buf.Write([]byte{0, 0, 0, 0, 0, 0, 0, 0})
	
	rdbData := buf.Bytes()
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		handler := &mockRDBHandler{}
		parser := NewRDBParser(bytes.NewReader(rdbData), handler)
		if err := parser.Parse(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRDBParseWithExpiry benchmarks parsing entries with expiration
func BenchmarkRDBParseWithExpiry(b *testing.B) {
	var buf bytes.Buffer
	
	// RDB header
	buf.WriteString("REDIS0011")
	
	// Select DB 0
	buf.WriteByte(RDBOpcodeDB)
	buf.WriteByte(0)
	
	// Entry with millisecond expiry
	expiry := time.Now().Add(time.Hour).UnixMilli()
	buf.WriteByte(RDBOpcodeExpiryMs)
	binary.Write(&buf, binary.LittleEndian, uint64(expiry))
	
	// String type
	buf.WriteByte(RDBTypeString)
	writeString(&buf, []byte("key"))
	writeString(&buf, []byte("value"))
	
	// EOF
	buf.WriteByte(RDBOpcodeEOF)
	buf.Write([]byte{0, 0, 0, 0, 0, 0, 0, 0})
	
	rdbData := buf.Bytes()
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		handler := &mockRDBHandler{}
		parser := NewRDBParser(bytes.NewReader(rdbData), handler)
		if err := parser.Parse(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRDBParseAuxFields benchmarks parsing auxiliary fields
func BenchmarkRDBParseAuxFields(b *testing.B) {
	var buf bytes.Buffer
	
	// RDB header
	buf.WriteString("REDIS0011")
	
	// Multiple AUX fields
	auxFields := []struct {
		key   string
		value string
	}{
		{"redis-ver", "7.0.0"},
		{"redis-bits", "64"},
		{"ctime", "1234567890"},
		{"used-mem", "1048576"},
	}
	
	for _, aux := range auxFields {
		buf.WriteByte(RDBOpcodeAux)
		writeString(&buf, []byte(aux.key))
		writeString(&buf, []byte(aux.value))
	}
	
	// Select DB 0
	buf.WriteByte(RDBOpcodeDB)
	buf.WriteByte(0)
	
	// EOF
	buf.WriteByte(RDBOpcodeEOF)
	buf.Write([]byte{0, 0, 0, 0, 0, 0, 0, 0})
	
	rdbData := buf.Bytes()
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		handler := &mockRDBHandler{}
		parser := NewRDBParser(bytes.NewReader(rdbData), handler)
		if err := parser.Parse(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkLZFDecompress benchmarks LZF decompression
func BenchmarkLZFDecompress(b *testing.B) {
	// Create compressible data (not used yet, needs actual LZF compression implementation)
	_ = bytes.Repeat([]byte("Hello World! This is a test string for compression. "), 100)
	
	// Note: This would require actual LZF compression
	// For now, we'll benchmark the decompression logic if available
	b.Skip("LZF compression benchmark requires actual compressed data")
}
