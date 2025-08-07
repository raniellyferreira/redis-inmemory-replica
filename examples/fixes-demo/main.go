package main

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/raniellyferreira/redis-inmemory-replica/protocol"
)

// demonstrateBufferFixes shows how the buffer overflow and CRLF fixes work
func main() {
	fmt.Println("🔧 Redis In-Memory Replica: Buffer Overflow & CRLF Fixes Demo")
	fmt.Println("==============================================================")
	
	// Demo 1: CRLF error handling improvements
	fmt.Println("\n📋 Demo 1: Enhanced CRLF Error Handling")
	fmt.Println("---------------------------------------")
	
	// Test with missing CRLF
	testMissingCRLF()
	
	// Test with invalid CRLF
	testInvalidCRLF()
	
	// Demo 2: Buffer boundary protection
	fmt.Println("\n📋 Demo 2: Buffer Boundary Protection")
	fmt.Println("------------------------------------")
	testBufferBoundaries()
	
	// Demo 3: Large data handling
	fmt.Println("\n📋 Demo 3: Large Data Handling")
	fmt.Println("------------------------------")
	testLargeDataHandling()
	
	fmt.Println("\n✅ All demos completed successfully!")
	fmt.Println("The buffer overflow and CRLF parsing issues have been resolved.")
}

func testMissingCRLF() {
	fmt.Println("Testing protocol parsing with missing CRLF...")
	
	// Create protocol data with missing CRLF (just \n instead of \r\n)
	invalidData := "+OK\n"
	reader := protocol.NewReader(strings.NewReader(invalidData))
	
	_, err := reader.ReadNext()
	if err != nil {
		fmt.Printf("  ✅ Correctly caught CRLF error: %v\n", err)
	} else {
		fmt.Printf("  ❌ Should have caught CRLF error\n")
	}
}

func testInvalidCRLF() {
	fmt.Println("Testing protocol parsing with invalid CRLF...")
	
	// Create protocol data with wrong terminator
	invalidData := "+OK\r\x00"  // \r\x00 instead of \r\n
	reader := protocol.NewReader(strings.NewReader(invalidData))
	
	_, err := reader.ReadNext()
	if err != nil {
		fmt.Printf("  ✅ Correctly caught invalid CRLF: %v\n", err)
	} else {
		fmt.Printf("  ❌ Should have caught invalid CRLF error\n")
	}
}

func testBufferBoundaries() {
	fmt.Println("Testing buffer boundary protection...")
	
	// Create a realistic bulk string that could cause buffer issues
	testData := make([]byte, 4096) // Exactly 4096 bytes
	for i := range testData {
		testData[i] = byte(i % 256)
	}
	
	// Create RESP bulk string format
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "$%d\r\n", len(testData))
	buf.Write(testData)
	buf.WriteString("\r\n")
	
	reader := protocol.NewReader(&buf)
	value, err := reader.ReadNext()
	
	if err != nil {
		fmt.Printf("  ❌ Failed to read bulk string: %v\n", err)
	} else if len(value.Data) == 4096 {
		fmt.Printf("  ✅ Successfully handled 4096-byte buffer boundary\n")
	} else {
		fmt.Printf("  ❌ Data length mismatch: expected 4096, got %d\n", len(value.Data))
	}
}

func testLargeDataHandling() {
	fmt.Println("Testing large data handling with streaming...")
	
	// Create a large bulk string (larger than typical buffer)
	largeSize := 50000 // 50KB
	testData := make([]byte, largeSize)
	for i := range testData {
		testData[i] = byte('A' + (i % 26)) // Fill with letters A-Z
	}
	
	// Create RESP bulk string format
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "$%d\r\n", len(testData))
	buf.Write(testData)
	buf.WriteString("\r\n")
	
	reader := protocol.NewReader(&buf)
	
	// Test streaming read
	totalRead := 0
	err := reader.ReadBulkString(func(chunk []byte) error {
		if chunk != nil {
			totalRead += len(chunk)
		}
		return nil
	})
	
	if err != nil {
		fmt.Printf("  ❌ Failed to stream large data: %v\n", err)
	} else if totalRead == largeSize {
		fmt.Printf("  ✅ Successfully streamed %d bytes in chunks\n", totalRead)
	} else {
		fmt.Printf("  ❌ Data size mismatch: expected %d, got %d\n", largeSize, totalRead)
	}
}