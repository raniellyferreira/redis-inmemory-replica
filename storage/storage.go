package storage

import (
	"context"
	"time"
)

// Storage defines the interface for data storage operations
type Storage interface {
	// String operations
	Get(key string) ([]byte, bool)
	Set(key string, value []byte, expiry *time.Time) error
	Del(keys ...string) int64
	Exists(keys ...string) int64
	
	// Expiration operations
	Expire(key string, expiry time.Time) bool
	TTL(key string) time.Duration
	
	// Key operations
	Keys() []string
	KeyCount() int64
	FlushAll() error
	
	// Type operations
	Type(key string) ValueType
	
	// Memory operations
	MemoryUsage() int64
	
	// Database operations
	SelectDB(db int) error
	CurrentDB() int
	
	// Iteration
	Scan(cursor int64, match string, count int64) (int64, []string)
	
	// Info and stats
	Info() map[string]interface{}
	
	// Shutdown
	Close() error
}

// TransactionalStorage extends Storage with transaction support
type TransactionalStorage interface {
	Storage
	
	// Transaction operations
	Multi() Transaction
	Watch(keys ...string) error
	Unwatch() error
}

// Transaction represents a Redis transaction
type Transaction interface {
	// Queue commands
	Set(key string, value []byte, expiry *time.Time) Transaction
	Get(key string) Transaction
	Del(keys ...string) Transaction
	
	// Execute transaction
	Exec() ([]interface{}, error)
	Discard() error
}

// StorageObserver provides hooks for storage events
type StorageObserver interface {
	OnKeySet(key string, value []byte)
	OnKeyDeleted(key string)
	OnKeyExpired(key string)
	OnKeyAccessed(key string)
}

// SnapshotStorage extends Storage with snapshot capabilities
type SnapshotStorage interface {
	Storage
	
	// Create a point-in-time snapshot
	Snapshot() (Snapshot, error)
	
	// Restore from snapshot
	Restore(snapshot Snapshot) error
}

// Snapshot represents a point-in-time storage snapshot
type Snapshot interface {
	// Iterate over all key-value pairs
	ForEach(ctx context.Context, fn func(key string, value *Value) error) error
	
	// Get snapshot metadata
	Timestamp() time.Time
	KeyCount() int64
	
	// Cleanup
	Close() error
}

// MemoryLimitedStorage extends Storage with memory management
type MemoryLimitedStorage interface {
	Storage
	
	// Memory management
	SetMemoryLimit(bytes int64)
	GetMemoryLimit() int64
	EvictLRU(count int) int64
}

// CleanupConfigurableStorage extends Storage with cleanup configuration
type CleanupConfigurableStorage interface {
	Storage
	
	// Cleanup configuration
	SetCleanupConfig(config CleanupConfig)
	GetCleanupConfig() CleanupConfig
}

// CleanupConfig holds configuration for incremental cleanup
type CleanupConfig struct {
	// SampleSize is the number of keys to sample per round
	SampleSize int
	// MaxRounds is the maximum number of rounds per cleanup cycle
	MaxRounds int
	// BatchSize is the number of keys to delete in each batch
	BatchSize int
	// ExpiredThreshold continues cleanup if this percentage of sampled keys are expired
	ExpiredThreshold float64
}