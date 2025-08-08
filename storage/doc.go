// Package storage provides storage interfaces and implementations
// for the Redis in-memory replica.
//
// The storage layer abstracts the underlying data storage mechanism,
// allowing for different implementations (in-memory, persistent, etc.).
//
// Basic usage:
//
//	storage := storage.NewMemory()
//	err := storage.Set("key", []byte("value"), nil)
//	value, exists := storage.Get("key")
//
// The package supports:
//   - Thread-safe operations
//   - Expiration handling
//   - Memory usage tracking
//   - Key enumeration
//   - Atomic operations
package storage
