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
	PTTL(key string) time.Duration

	// Key operations
	Keys(pattern string) []string
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
	DatabaseInfo() map[int]map[string]interface{}

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

// Predefined CleanupConfig constants for different use cases

// CleanupConfigDefault provides balanced performance for most use cases
// Similar to Redis native behavior with good performance/resource balance
var CleanupConfigDefault = CleanupConfig{
	SampleSize:       20,   // Sample 20 keys per round
	MaxRounds:        4,    // Maximum 4 rounds per cleanup cycle
	BatchSize:        10,   // Delete up to 10 keys per batch
	ExpiredThreshold: 0.25, // Continue if >25% of sampled keys are expired
}

// CleanupConfigSmallDataset optimized for datasets with < 10,000 keys
// More conservative approach to minimize overhead
var CleanupConfigSmallDataset = CleanupConfig{
	SampleSize:       10,  // Smaller sample size for small datasets
	MaxRounds:        2,   // Fewer rounds to reduce overhead
	BatchSize:        5,   // Smaller batches
	ExpiredThreshold: 0.5, // Higher threshold to avoid unnecessary work
}

// CleanupConfigMediumDataset optimized for datasets with 10,000-100,000 keys
// Balanced approach with moderate resource usage
var CleanupConfigMediumDataset = CleanupConfig{
	SampleSize:       25,  // Moderate sample size
	MaxRounds:        5,   // Good balance of thoroughness and performance
	BatchSize:        12,  // Moderate batch size
	ExpiredThreshold: 0.3, // Moderate threshold
}

// CleanupConfigLargeDataset optimized for datasets with > 100,000 keys
// More aggressive cleanup for large-scale deployments
var CleanupConfigLargeDataset = CleanupConfig{
	SampleSize:       50,   // Larger sample for better coverage
	MaxRounds:        8,    // More rounds for thorough cleanup
	BatchSize:        25,   // Larger batches for efficiency
	ExpiredThreshold: 0.15, // Lower threshold for more aggressive cleanup
}

// CleanupConfigBestPerformance optimized for maximum cleanup throughput
// Uses larger batches and more aggressive settings for heavy workloads
var CleanupConfigBestPerformance = CleanupConfig{
	SampleSize:       100, // Large sample size for maximum coverage
	MaxRounds:        10,  // Maximum rounds for thorough cleanup
	BatchSize:        50,  // Large batches for maximum efficiency
	ExpiredThreshold: 0.1, // Very aggressive threshold
}

// CleanupConfigLowLatency optimized for latency-sensitive applications
// Minimizes cleanup impact on application response times
var CleanupConfigLowLatency = CleanupConfig{
	SampleSize:       15,  // Small sample to minimize lock time
	MaxRounds:        3,   // Few rounds to minimize impact
	BatchSize:        8,   // Small batches for minimal lock time
	ExpiredThreshold: 0.4, // Higher threshold to avoid excessive cleanup
}
