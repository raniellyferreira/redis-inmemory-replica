package storage

import (
	"fmt"
	randv2 "math/rand/v2"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"github.com/cespare/xxhash/v2"
)

// shard represents a single shard of data with its own lock
type shard struct {
	mu   sync.RWMutex
	data map[string]*Value
}

// MemoryStorage implements an in-memory storage engine
type MemoryStorage struct {
	// Global lock for metadata operations
	mu        sync.RWMutex
	databases map[int]*shardedDatabase
	currentDB int

	// Sharding configuration
	shards    int
	shardMask uint64

	// Memory management
	memoryLimit int64
	observers   []StorageObserver

	// Background cleanup
	cleanupStop chan struct{}
	cleanupDone chan struct{}

	// Cleanup configuration
	cleanupConfig CleanupConfig

	// Random number generator for sampling
	rng *randv2.Rand
}

// shardedDatabase represents a Redis database with sharded data
type shardedDatabase struct {
	shards []shard
}

// MemoryOption is a function that configures a MemoryStorage instance
type MemoryOption func(*MemoryStorage)

// WithShardCount sets the number of shards for the storage
// The number is automatically rounded up to the next power of 2 for optimal performance
func WithShardCount(count int) MemoryOption {
	return func(s *MemoryStorage) {
		if count > 0 {
			s.shards = nextPowerOf2(count)
			s.shardMask = uint64(s.shards - 1)
		}
	}
}

// NewMemory creates a new in-memory storage instance with default number of shards (64)
func NewMemory(opts ...MemoryOption) *MemoryStorage {
	s := &MemoryStorage{
		databases:     make(map[int]*shardedDatabase),
		currentDB:     0,
		shards:        64, // Default to 64 shards
		shardMask:     63, // 64 - 1
		cleanupStop:   make(chan struct{}),
		cleanupDone:   make(chan struct{}),
		cleanupConfig: CleanupConfigDefault,
		rng:           randv2.New(randv2.NewPCG(uint64(time.Now().UnixNano()), 0)),
	}

	// Apply options
	for _, opt := range opts {
		opt(s)
	}

	// Initialize default database with shards
	s.databases[0] = s.newShardedDatabase()

	// Start background cleanup goroutine
	go s.cleanupExpiredKeys()

	return s
}

// NewMemoryWithShards creates a new in-memory storage instance with specified number of shards
// Deprecated: Use NewMemory(WithShardCount(n)) instead
// The number of shards is automatically rounded up to the next power of 2 for optimal performance
func NewMemoryWithShards(numShards int) *MemoryStorage {
	return NewMemory(WithShardCount(numShards))
}

// newShardedDatabase creates a new sharded database
func (s *MemoryStorage) newShardedDatabase() *shardedDatabase {
	db := &shardedDatabase{
		shards: make([]shard, s.shards),
	}
	for i := 0; i < s.shards; i++ {
		db.shards[i].data = make(map[string]*Value)
	}
	return db
}

// nextPowerOf2 returns the next power of 2 >= n
func nextPowerOf2(n int) int {
	if n <= 1 {
		return 1
	}
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	n |= n >> 32
	return n + 1
}

// keyHash computes the hash for a key and returns the shard index
func (s *MemoryStorage) keyHash(key string) uint64 {
	return xxhash.Sum64String(key) & s.shardMask
}

// Get retrieves a value by key
func (s *MemoryStorage) Get(key string) ([]byte, bool) {
	// Get database
	s.mu.RLock()
	db := s.databases[s.currentDB]
	s.mu.RUnlock()

	// Get shard for key
	shardIdx := s.keyHash(key)
	sh := &db.shards[shardIdx]

	sh.mu.RLock()
	value, exists := sh.data[key]
	if !exists {
		sh.mu.RUnlock()
		return nil, false
	}

	// Check expiration without releasing lock
	if value.IsExpired() {
		sh.mu.RUnlock()
		// Use a separate method to delete expired key safely
		s.deleteExpiredKey(key)
		return nil, false
	}

	// Copy the data while holding the read lock
	var result []byte
	if value.Type == ValueTypeString {
		if stringVal, ok := value.Data.(*StringValue); ok {
			result = make([]byte, len(stringVal.Data))
			copy(result, stringVal.Data)
		}
	}

	// Notify observers while holding shard lock to prevent race conditions
	s.mu.RLock()
	observers := s.observers
	s.mu.RUnlock()

	for _, observer := range observers {
		observer.OnKeyAccessed(key)
	}

	sh.mu.RUnlock()

	if result != nil {
		return result, true
	}

	return nil, false
}

// Set stores a value with optional expiration
func (s *MemoryStorage) Set(key string, value []byte, expiry *time.Time) error {
	// Get database
	s.mu.RLock()
	db := s.databases[s.currentDB]
	memLimit := s.memoryLimit
	s.mu.RUnlock()

	// Get shard for key
	shardIdx := s.keyHash(key)
	sh := &db.shards[shardIdx]

	// Create new value
	newValue := &Value{
		Type:    ValueTypeString,
		Data:    &StringValue{Data: append([]byte(nil), value...)},
		Expiry:  expiry,
		Version: time.Now().UnixNano(),
	}

	sh.mu.Lock()
	sh.data[key] = newValue
	sh.mu.Unlock()

	// Notify observers
	s.mu.RLock()
	for _, observer := range s.observers {
		observer.OnKeySet(key, value)
	}
	s.mu.RUnlock()

	// Check memory limit if configured
	if memLimit > 0 {
		memUsage := s.MemoryUsage()
		if memUsage > memLimit {
			s.mu.Lock()
			s.evictLRU(1)
			s.mu.Unlock()
		}
	}

	return nil
}

// Del deletes one or more keys
func (s *MemoryStorage) Del(keys ...string) int64 {
	// Get database
	s.mu.RLock()
	db := s.databases[s.currentDB]
	s.mu.RUnlock()

	deleted := int64(0)

	// Group keys by shard to minimize lock contention
	keysByShard := make(map[uint64][]string)
	for _, key := range keys {
		shardIdx := s.keyHash(key)
		keysByShard[shardIdx] = append(keysByShard[shardIdx], key)
	}

	// Delete keys shard by shard
	for shardIdx, shardKeys := range keysByShard {
		sh := &db.shards[shardIdx]
		sh.mu.Lock()
		for _, key := range shardKeys {
			if _, exists := sh.data[key]; exists {
				delete(sh.data, key)
				deleted++

				// Notify observers (need to get lock)
				s.mu.RLock()
				for _, observer := range s.observers {
					observer.OnKeyDeleted(key)
				}
				s.mu.RUnlock()
			}
		}
		sh.mu.Unlock()
	}

	return deleted
}

// Exists checks if keys exist
func (s *MemoryStorage) Exists(keys ...string) int64 {
	// Get database
	s.mu.RLock()
	db := s.databases[s.currentDB]
	s.mu.RUnlock()

	count := int64(0)

	// Group keys by shard
	keysByShard := make(map[uint64][]string)
	for _, key := range keys {
		shardIdx := s.keyHash(key)
		keysByShard[shardIdx] = append(keysByShard[shardIdx], key)
	}

	// Check existence shard by shard
	for shardIdx, shardKeys := range keysByShard {
		sh := &db.shards[shardIdx]
		sh.mu.RLock()
		for _, key := range shardKeys {
			if value, exists := sh.data[key]; exists && !value.IsExpired() {
				count++
			}
		}
		sh.mu.RUnlock()
	}

	return count
}

// Expire sets expiration for a key
func (s *MemoryStorage) Expire(key string, expiry time.Time) bool {
	// Get database
	s.mu.RLock()
	db := s.databases[s.currentDB]
	s.mu.RUnlock()

	// Get shard for key
	shardIdx := s.keyHash(key)
	sh := &db.shards[shardIdx]

	sh.mu.Lock()
	defer sh.mu.Unlock()

	value, exists := sh.data[key]
	if !exists || value.IsExpired() {
		return false
	}

	value.Expiry = &expiry
	return true
}

// TTL returns the time to live for a key
func (s *MemoryStorage) TTL(key string) time.Duration {
	// Get database
	s.mu.RLock()
	db := s.databases[s.currentDB]
	s.mu.RUnlock()

	// Get shard for key
	shardIdx := s.keyHash(key)
	sh := &db.shards[shardIdx]

	sh.mu.RLock()
	defer sh.mu.RUnlock()

	value, exists := sh.data[key]
	if !exists {
		return -2 * time.Second // Key doesn't exist
	}

	if value.IsExpired() {
		return -2 * time.Second // Key expired
	}

	if value.Expiry == nil {
		return -1 * time.Second // No expiration
	}

	return time.Until(*value.Expiry)
}

// PTTL returns the time to live for a key in milliseconds
func (s *MemoryStorage) PTTL(key string) time.Duration {
	// Get database
	s.mu.RLock()
	db := s.databases[s.currentDB]
	s.mu.RUnlock()

	// Get shard for key
	shardIdx := s.keyHash(key)
	sh := &db.shards[shardIdx]

	sh.mu.RLock()
	defer sh.mu.RUnlock()

	value, exists := sh.data[key]
	if !exists {
		return -2 * time.Millisecond // Key doesn't exist
	}

	if value.IsExpired() {
		return -2 * time.Millisecond // Key expired
	}

	if value.Expiry == nil {
		return -1 * time.Millisecond // No expiration
	}

	return time.Until(*value.Expiry)
}

// Keys returns all keys matching the pattern in the current database
// Pattern supports glob-style patterns:
// * matches any number of characters (including zero)
// ? matches a single character
// [abc] matches any character in the brackets
// [a-z] matches any character in the range
func (s *MemoryStorage) Keys(pattern string) []string {
	// Get database
	s.mu.RLock()
	db := s.databases[s.currentDB]
	s.mu.RUnlock()

	keys := make([]string, 0)

	// Iterate over all shards and collect matching keys
	for i := 0; i < s.shards; i++ {
		sh := &db.shards[i]
		sh.mu.RLock()

		// Handle empty or "*" pattern - return all keys
		if pattern == "" || pattern == "*" {
			for key, value := range sh.data {
				if !value.IsExpired() {
					keys = append(keys, key)
				}
			}
		} else {
			// Use pattern matching system for consistent pattern support
			for key, value := range sh.data {
				if !value.IsExpired() {
					if matchPattern(key, pattern) {
						keys = append(keys, key)
					}
				}
			}
		}

		sh.mu.RUnlock()
	}

	return keys
}

// KeyCount returns the number of keys in the current database
func (s *MemoryStorage) KeyCount() int64 {
	// Get database
	s.mu.RLock()
	db := s.databases[s.currentDB]
	s.mu.RUnlock()

	count := int64(0)

	// Count keys across all shards
	for i := 0; i < s.shards; i++ {
		sh := &db.shards[i]
		sh.mu.RLock()
		count += int64(len(sh.data))
		sh.mu.RUnlock()
	}

	return count
}

// FlushAll removes all keys from all databases
func (s *MemoryStorage) FlushAll() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for db := range s.databases {
		s.databases[db] = s.newShardedDatabase()
	}

	return nil
}

// Type returns the type of a key
func (s *MemoryStorage) Type(key string) ValueType {
	// Get database
	s.mu.RLock()
	db := s.databases[s.currentDB]
	s.mu.RUnlock()

	// Get shard for key
	shardIdx := s.keyHash(key)
	sh := &db.shards[shardIdx]

	sh.mu.RLock()
	defer sh.mu.RUnlock()

	value, exists := sh.data[key]
	if !exists || value.IsExpired() {
		return ValueTypeString // Default for non-existent keys
	}

	return value.Type
}

// MemoryUsage returns current memory usage in bytes
func (s *MemoryStorage) MemoryUsage() int64 {
	s.mu.RLock()
	databases := make([]*shardedDatabase, 0, len(s.databases))
	for _, db := range s.databases {
		databases = append(databases, db)
	}
	s.mu.RUnlock()

	usage := int64(0)

	// Calculate memory usage across all databases and shards
	for _, db := range databases {
		for i := 0; i < s.shards; i++ {
			sh := &db.shards[i]
			sh.mu.RLock()
			for key, value := range sh.data {
				usage += int64(len(key))
				usage += s.calculateValueSize(value)
			}
			sh.mu.RUnlock()
		}
	}

	return usage
}

// SelectDB selects a database
func (s *MemoryStorage) SelectDB(db int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if db < 0 || db > 15 { // Redis supports 0-15 databases by default
		return fmt.Errorf("invalid database number: %d", db)
	}

	// Create database if it doesn't exist
	if _, exists := s.databases[db]; !exists {
		s.databases[db] = s.newShardedDatabase()
	}

	s.currentDB = db

	return nil
}

// CurrentDB returns the current database number
func (s *MemoryStorage) CurrentDB() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentDB
}

// Scan provides cursor-based iteration over keys
func (s *MemoryStorage) Scan(cursor int64, match string, count int64) (int64, []string) {
	// Get database
	s.mu.RLock()
	db := s.databases[s.currentDB]
	s.mu.RUnlock()

	keys := make([]string, 0, count)
	i := int64(0)
	nextCursor := int64(0)

	// Iterate through shards
	for shardIdx := 0; shardIdx < s.shards; shardIdx++ {
		sh := &db.shards[shardIdx]
		sh.mu.RLock()

		for key, value := range sh.data {
			if i < cursor {
				i++
				continue
			}

			if value.IsExpired() {
				continue
			}

			// Apply pattern matching if specified
			if match != "" && !matchPattern(key, match) {
				continue
			}

			keys = append(keys, key)

			if int64(len(keys)) >= count {
				nextCursor = i + 1
				sh.mu.RUnlock()
				return nextCursor, keys
			}

			i++
		}

		sh.mu.RUnlock()
	}

	return nextCursor, keys
}

// Info returns storage information
func (s *MemoryStorage) Info() map[string]interface{} {
	s.mu.RLock()
	currentDB := s.currentDB
	numDatabases := len(s.databases)
	memLimit := s.memoryLimit
	s.mu.RUnlock()

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return map[string]interface{}{
		"keys":         s.KeyCount(),
		"memory_usage": s.MemoryUsage(),
		"memory_limit": memLimit,
		"current_db":   currentDB,
		"databases":    numDatabases,
		"go_memory":    m.Alloc,
		"shards":       s.shards,
	}
}

// DatabaseInfo returns information about all databases with keys
// For databases with many keys, it uses sampling to estimate expired count for performance
func (s *MemoryStorage) DatabaseInfo() map[int]map[string]interface{} {
	s.mu.RLock()
	databases := make(map[int]*shardedDatabase)
	for dbNum, db := range s.databases {
		databases[dbNum] = db
	}
	s.mu.RUnlock()

	dbInfo := make(map[int]map[string]interface{})

	for dbNum, db := range databases {
		keyCount := int64(0)
		expiredCount := int64(0)

		// Count keys across all shards
		for i := 0; i < s.shards; i++ {
			sh := &db.shards[i]
			sh.mu.RLock()
			shardKeyCount := int64(len(sh.data))
			keyCount += shardKeyCount

			// For large databases, use sampling to estimate expired count for better performance
			if shardKeyCount > 100 {
				// Sample up to 50 keys per shard to estimate expired ratio
				sampleSize := 50
				if int(shardKeyCount) < sampleSize {
					sampleSize = int(shardKeyCount)
				}

				expiredSample := 0
				sampled := 0

				// Create a stable iteration order by copying keys first
				keys := make([]string, 0, sampleSize)
				for key := range sh.data {
					keys = append(keys, key)
					if len(keys) >= sampleSize {
						break
					}
				}

				// Now sample from the stable key list
				for _, key := range keys {
					if value, exists := sh.data[key]; exists && value.IsExpired() {
						expiredSample++
					}
					sampled++
				}

				// Estimate expired count based on sample ratio
				if sampled > 0 {
					expiredRatio := float64(expiredSample) / float64(sampled)
					expiredCount += int64(expiredRatio * float64(shardKeyCount))
				}
			} else {
				// For smaller shard datasets, count all expired keys precisely
				for _, value := range sh.data {
					if value.IsExpired() {
						expiredCount++
					}
				}
			}

			sh.mu.RUnlock()
		}

		if keyCount == 0 {
			continue // Skip empty databases for keyspace info
		}

		dbInfo[dbNum] = map[string]interface{}{
			"keys":    keyCount,
			"expires": expiredCount,
		}
	}

	return dbInfo
}

// Close shuts down the storage
func (s *MemoryStorage) Close() error {
	close(s.cleanupStop)
	<-s.cleanupDone
	return nil
}

// SetMemoryLimit sets the memory limit
func (s *MemoryStorage) SetMemoryLimit(bytes int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.memoryLimit = bytes
}

// GetMemoryLimit returns the current memory limit
func (s *MemoryStorage) GetMemoryLimit() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.memoryLimit
}

// EvictLRU evicts the least recently used keys
func (s *MemoryStorage) EvictLRU(count int) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.evictLRU(count)
}

// AddObserver adds a storage observer
func (s *MemoryStorage) AddObserver(observer StorageObserver) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.observers = append(s.observers, observer)
}

// SetCleanupConfig updates the cleanup configuration
func (s *MemoryStorage) SetCleanupConfig(config CleanupConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupConfig = config
}

// GetCleanupConfig returns the current cleanup configuration
func (s *MemoryStorage) GetCleanupConfig() CleanupConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cleanupConfig
}

// evictLRU evicts least recently used keys (internal, must hold lock)
func (s *MemoryStorage) evictLRU(count int) int64 {
	// This is a simplified LRU implementation
	// In production, you'd want a more sophisticated approach
	db := s.databases[s.currentDB]
	evicted := int64(0)

	// Evict from shards in round-robin fashion
	for evicted < int64(count) {
		evictedThisRound := int64(0)
		for i := 0; i < s.shards && evicted < int64(count); i++ {
			sh := &db.shards[i]
			sh.mu.Lock()
			for key := range sh.data {
				delete(sh.data, key)
				evicted++
				evictedThisRound++

				// Notify observers
				for _, observer := range s.observers {
					observer.OnKeyDeleted(key)
				}

				if evicted >= int64(count) {
					sh.mu.Unlock()
					return evicted
				}
				break // Only evict one key per shard per round
			}
			sh.mu.Unlock()
		}
		// If we didn't evict anything this round, we're out of keys
		if evictedThisRound == 0 {
			break
		}
	}

	return evicted
}

// calculateValueSize estimates the size of a value in bytes
func (s *MemoryStorage) calculateValueSize(value *Value) int64 {
	if value == nil {
		return 0
	}

	// safe: intentional use of unsafe.Sizeof for memory accounting
	size := int64(unsafe.Sizeof(*value))

	switch value.Type {
	case ValueTypeString:
		if stringVal, ok := value.Data.(*StringValue); ok {
			size += int64(len(stringVal.Data))
		}
		// Add other types as needed
	}

	return size
}

// cleanupExpiredKeys runs in background to clean up expired keys
func (s *MemoryStorage) cleanupExpiredKeys() {
	defer close(s.cleanupDone)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.cleanupStop:
			return
		case <-ticker.C:
			s.performCleanup()
		}
	}
}

// performCleanup removes expired keys using incremental sampling approach
func (s *MemoryStorage) performCleanup() {
	config := s.GetCleanupConfig()

	s.mu.RLock()
	databases := make(map[int]*shardedDatabase)
	for dbNum, db := range s.databases {
		databases[dbNum] = db
	}
	s.mu.RUnlock()

	for dbNum, db := range databases {
		s.cleanupDatabase(dbNum, db, config)
	}
}

// cleanupDatabase performs incremental cleanup on a single database
func (s *MemoryStorage) cleanupDatabase(dbNum int, db *shardedDatabase, config CleanupConfig) {
	// Clean up each shard
	for shardIdx := 0; shardIdx < s.shards; shardIdx++ {
		s.cleanupShard(shardIdx, db, config)
	}
}

// cleanupShard performs incremental cleanup on a single shard
func (s *MemoryStorage) cleanupShard(shardIdx int, db *shardedDatabase, config CleanupConfig) {
	sh := &db.shards[shardIdx]

	for round := 0; round < config.MaxRounds; round++ {
		expiredKeys := s.sampleAndFindExpiredInShard(sh, config.SampleSize)

		if len(expiredKeys) == 0 {
			break // No expired keys found, stop cleaning this shard
		}

		// Delete expired keys in batches to minimize lock time
		s.deleteExpiredKeysInShardBatched(sh, expiredKeys, config.BatchSize)

		// Check if we should continue with another round
		expiredRatio := float64(len(expiredKeys)) / float64(config.SampleSize)
		if expiredRatio < config.ExpiredThreshold {
			break // Not many expired keys found, stop cleaning
		}

		// Yield CPU briefly between rounds to allow other operations
		runtime.Gosched()
	}
}

// sampleAndFindExpiredInShard samples keys and finds expired ones in a specific shard
func (s *MemoryStorage) sampleAndFindExpiredInShard(sh *shard, sampleSize int) []string {
	// Take a read lock to get a sample of keys
	sh.mu.RLock()
	defer sh.mu.RUnlock()

	if len(sh.data) == 0 {
		return nil
	}

	// Adjust sample size if needed
	actualSampleSize := sampleSize
	if len(sh.data) < sampleSize {
		actualSampleSize = len(sh.data)
	}

	// Sample keys randomly using reservoir sampling for better performance
	sampledKeys := make([]string, 0, actualSampleSize)

	if len(sh.data) <= actualSampleSize {
		// If we need all keys or close to it, just collect them
		for key := range sh.data {
			sampledKeys = append(sampledKeys, key)
		}
	} else {
		// Use reservoir sampling algorithm - more efficient for large datasets
		i := 0
		for key := range sh.data {
			if i < actualSampleSize {
				// Fill reservoir
				sampledKeys = append(sampledKeys, key)
			} else {
				// Replace random element with probability sampleSize/i
				j := s.rng.IntN(i + 1)
				if j < actualSampleSize {
					sampledKeys[j] = key
				}
			}
			i++
		}
	}

	// Check which of the sampled keys are expired
	expiredKeys := make([]string, 0, len(sampledKeys))
	for _, key := range sampledKeys {
		if value, exists := sh.data[key]; exists && value.IsExpired() {
			expiredKeys = append(expiredKeys, key)
		}
	}

	return expiredKeys
}

// deleteExpiredKeysInShardBatched deletes expired keys in batches to minimize lock time
func (s *MemoryStorage) deleteExpiredKeysInShardBatched(sh *shard, expiredKeys []string, batchSize int) {
	for i := 0; i < len(expiredKeys); i += batchSize {
		end := i + batchSize
		if end > len(expiredKeys) {
			end = len(expiredKeys)
		}

		batch := expiredKeys[i:end]
		s.deleteKeyBatchInShard(sh, batch)

		// Yield between batches for better concurrency
		if end < len(expiredKeys) {
			runtime.Gosched()
		}
	}
}

// deleteKeyBatchInShard deletes a batch of keys in a shard with minimal lock time
func (s *MemoryStorage) deleteKeyBatchInShard(sh *shard, keys []string) {
	sh.mu.Lock()
	defer sh.mu.Unlock()

	for _, key := range keys {
		if value, exists := sh.data[key]; exists {
			// Double-check expiration under write lock
			if value.IsExpired() {
				delete(sh.data, key)

				// Notify observers
				s.mu.RLock()
				for _, observer := range s.observers {
					observer.OnKeyExpired(key)
				}
				s.mu.RUnlock()
			}
		}
	}
}

// deleteExpiredKey safely deletes an expired key without race conditions
func (s *MemoryStorage) deleteExpiredKey(key string) {
	// Get database
	s.mu.RLock()
	db := s.databases[s.currentDB]
	s.mu.RUnlock()

	// Get shard for key
	shardIdx := s.keyHash(key)
	sh := &db.shards[shardIdx]

	sh.mu.Lock()
	value, exists := sh.data[key]
	if !exists {
		sh.mu.Unlock()
		return
	}

	// Double-check expiration under write lock
	if value.IsExpired() {
		delete(sh.data, key)
		sh.mu.Unlock()

		// Notify observers
		s.mu.RLock()
		for _, observer := range s.observers {
			observer.OnKeyExpired(key)
		}
		s.mu.RUnlock()
	} else {
		sh.mu.Unlock()
	}
}

// matchPattern performs pattern matching using the configured strategy.
// This function provides backward compatibility while allowing for different
// matching strategies to be used based on global configuration.
//
// Supported strategies:
// - StrategySimple: Optimized for single wildcard patterns (default)
// - StrategyRegex: Uses regular expressions for complex patterns
// - StrategyAutomaton: Uses finite state machines for pattern matching
// - StrategyGlob: Uses Go's standard library filepath.Match
func matchPattern(str, pattern string) bool {
	return MatchPatternWithStrategy(str, pattern, GetMatchingStrategy())
}
