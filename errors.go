package redisreplica

import (
	"errors"
	"fmt"
)

// Error types for specific failure scenarios
var (
	// ErrNotConnected indicates the replica is not connected to the master
	ErrNotConnected = errors.New("not connected to master")

	// ErrSyncInProgress indicates synchronization is currently in progress
	ErrSyncInProgress = errors.New("synchronization in progress")

	// ErrReadOnly indicates an attempt to perform a write operation on a read-only replica
	ErrReadOnly = errors.New("replica is read-only")

	// ErrInvalidCommand indicates an unsupported or malformed command
	ErrInvalidCommand = errors.New("invalid command")

	// ErrInvalidConfig indicates invalid configuration options
	ErrInvalidConfig = errors.New("invalid configuration")

	// ErrTimeout indicates an operation timed out
	ErrTimeout = errors.New("operation timed out")

	// ErrClosed indicates the replica has been closed
	ErrClosed = errors.New("replica is closed")
)

// SyncError represents a synchronization error with additional context
type SyncError struct {
	Phase   string // "handshake", "rdb", "streaming"
	Err     error
	Retries int
}

// Error implements the error interface
func (e *SyncError) Error() string {
	return fmt.Sprintf("sync error in phase %s after %d retries: %v", e.Phase, e.Retries, e.Err)
}

// Unwrap returns the wrapped error
func (e *SyncError) Unwrap() error {
	return e.Err
}

// ConnectionError represents a connection-related error
type ConnectionError struct {
	Addr string
	Err  error
}

// Error implements the error interface
func (e *ConnectionError) Error() string {
	return fmt.Sprintf("connection error to %s: %v", e.Addr, e.Err)
}

// Unwrap returns the wrapped error
func (e *ConnectionError) Unwrap() error {
	return e.Err
}

// ProtocolError represents a Redis protocol parsing error
type ProtocolError struct {
	Message string
	Data    []byte
}

// Error implements the error interface
func (e *ProtocolError) Error() string {
	return fmt.Sprintf("protocol error: %s", e.Message)
}
