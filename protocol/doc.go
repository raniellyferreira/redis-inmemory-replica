// Package protocol implements the Redis Serialization Protocol (RESP)
// for parsing and writing Redis protocol messages.
//
// This package provides streaming parsers that are memory-efficient
// and suitable for high-throughput applications. The parsers handle
// RESP protocol versions 2 and 3.
//
// Basic usage:
//
//	reader := protocol.NewReader(conn)
//	for {
//		value, err := reader.ReadNext()
//		if err != nil {
//			break
//		}
//		// Process value
//	}
//
// The package supports all RESP data types:
//   - Simple Strings
//   - Errors
//   - Integers
//   - Bulk Strings
//   - Arrays
//   - Null values
package protocol