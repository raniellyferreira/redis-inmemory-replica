package storage

import (
	"fmt"
	randv2 "math/rand/v2"
	"runtime"
	"sync"
	"time"
	"unsafe"
)

// MemoryStorage implements an in-memory storage engine
type MemoryStorage struct {
	mu        sync.RWMutex
	data      map[string]*Value
	databases map[int]*database
	currentDB int

	// Memory management
	memoryLimit int64
	observers   []StorageObserver

	// Statistics
	keyCount    int64
	memoryUsage int64

	// Background cleanup
	cleanupStop chan struct{}
	cleanupDone chan struct{}

	// Cleanup configuration
	cleanupConfig CleanupConfig

	// Random number generator for sampling
	rng *randv2.Rand
}

// database represents a Redis database
type database struct {
	data map[string]*Value
}

// NewMemory creates a new in-memory storage instance
func NewMemory() *MemoryStorage {
	s := &MemoryStorage{
		data:          make(map[string]*Value),
		databases:     make(map[int]*database),
		currentDB:     0,
		cleanupStop:   make(chan struct{}),
		cleanupDone:   make(chan struct{}),
		cleanupConfig: CleanupConfigDefault,
		rng:           randv2.New(randv2.NewPCG(uint64(time.Now().UnixNano()), 0)),
	}

	// Initialize default database
	s.databases[0] = &database{
		data: make(map[string]*Value),
	}

	// Start background cleanup goroutine
	go s.cleanupExpiredKeys()

	return s
}

// Get retrieves a value by key
func (s *MemoryStorage) Get(key string) ([]byte, bool) {
	s.mu.RLock()

	db := s.databases[s.currentDB]
	value, exists := db.data[key]
	if !exists {
		s.mu.RUnlock()
		return nil, false
	}

	// Check expiration without releasing lock
	if value.IsExpired() {
		s.mu.RUnlock()
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

	// Notify observers (hold read lock to prevent value from changing)
	for _, observer := range s.observers {
		observer.OnKeyAccessed(key)
	}

	s.mu.RUnlock()

	if result != nil {
		return result, true
	}

	return nil, false
}

// Set stores a value with optional expiration
func (s *MemoryStorage) Set(key string, value []byte, expiry *time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	db := s.databases[s.currentDB]

	// Create new value
	newValue := &Value{
		Type:    ValueTypeString,
		Data:    &StringValue{Data: append([]byte(nil), value...)},
		Expiry:  expiry,
		Version: time.Now().UnixNano(),
	}

	// Check if key already exists
	_, exists := db.data[key]
	if !exists {
		s.keyCount++
	}

	db.data[key] = newValue
	s.updateMemoryUsage()

	// Notify observers
	for _, observer := range s.observers {
		observer.OnKeySet(key, value)
	}

	// Check memory limit
	if s.memoryLimit > 0 && s.memoryUsage > s.memoryLimit {
		s.evictLRU(1)
	}

	return nil
}

// Del deletes one or more keys
func (s *MemoryStorage) Del(keys ...string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	db := s.databases[s.currentDB]
	deleted := int64(0)

	for _, key := range keys {
		if _, exists := db.data[key]; exists {
			delete(db.data, key)
			deleted++
			s.keyCount--

			// Notify observers
			for _, observer := range s.observers {
				observer.OnKeyDeleted(key)
			}
		}
	}

	if deleted > 0 {
		s.updateMemoryUsage()
	}

	return deleted
}

// Exists checks if keys exist
func (s *MemoryStorage) Exists(keys ...string) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	db := s.databases[s.currentDB]
	count := int64(0)

	for _, key := range keys {
		if value, exists := db.data[key]; exists && !value.IsExpired() {
			count++
		}
	}

	return count
}

// Expire sets expiration for a key
func (s *MemoryStorage) Expire(key string, expiry time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	db := s.databases[s.currentDB]
	value, exists := db.data[key]
	if !exists || value.IsExpired() {
		return false
	}

	value.Expiry = &expiry
	return true
}

// TTL returns the time to live for a key
func (s *MemoryStorage) TTL(key string) time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()

	db := s.databases[s.currentDB]
	value, exists := db.data[key]
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
	s.mu.RLock()
	defer s.mu.RUnlock()

	db := s.databases[s.currentDB]
	value, exists := db.data[key]
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
	s.mu.RLock()
	defer s.mu.RUnlock()

	db := s.databases[s.currentDB]
	keys := make([]string, 0)

	// Handle empty or "*" pattern - return all keys
	if pattern == "" || pattern == "*" {
		for key, value := range db.data {
			if !value.IsExpired() {
				keys = append(keys, key)
			}
		}
		return keys
	}

	// Use pattern matching system for consistent pattern support
	for key, value := range db.data {
		if !value.IsExpired() {
			if matchPattern(key, pattern) {
				keys = append(keys, key)
			}
		}
	}

	return keys
}

// KeyCount returns the number of keys in the current database
func (s *MemoryStorage) KeyCount() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.keyCount
}

// FlushAll removes all keys from all databases
func (s *MemoryStorage) FlushAll() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for db := range s.databases {
		s.databases[db] = &database{
			data: make(map[string]*Value),
		}
	}

	s.keyCount = 0
	s.memoryUsage = 0

	return nil
}

// Type returns the type of a key
func (s *MemoryStorage) Type(key string) ValueType {
	s.mu.RLock()
	defer s.mu.RUnlock()

	db := s.databases[s.currentDB]
	value, exists := db.data[key]
	if !exists || value.IsExpired() {
		return ValueTypeString // Default for non-existent keys
	}

	return value.Type
}

// MemoryUsage returns current memory usage in bytes
func (s *MemoryStorage) MemoryUsage() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.memoryUsage
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
		s.databases[db] = &database{
			data: make(map[string]*Value),
		}
	}

	s.currentDB = db

	// Recalculate key count for current database
	s.keyCount = int64(len(s.databases[db].data))

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
	s.mu.RLock()
	defer s.mu.RUnlock()

	db := s.databases[s.currentDB]
	keys := make([]string, 0, count)

	i := int64(0)
	nextCursor := int64(0)

	for key, value := range db.data {
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
			break
		}

		i++
	}

	return nextCursor, keys
}

// Info returns storage information
func (s *MemoryStorage) Info() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return map[string]interface{}{
		"keys":         s.keyCount,
		"memory_usage": s.memoryUsage,
		"memory_limit": s.memoryLimit,
		"current_db":   s.currentDB,
		"databases":    len(s.databases),
		"go_memory":    m.Alloc,
	}
}

// DatabaseInfo returns information about all databases with keys
// For databases with many keys, it uses sampling to estimate expired count for performance
func (s *MemoryStorage) DatabaseInfo() map[int]map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dbInfo := make(map[int]map[string]interface{})
	
	for dbNum, db := range s.databases {
		if len(db.data) == 0 {
			continue // Skip empty databases for keyspace info
		}
		
		keyCount := int64(len(db.data))
		expiredCount := int64(0)
		
		// For large databases, use sampling to estimate expired count for better performance
		if keyCount > 1000 {
			// Sample up to 100 keys to estimate expired ratio
			sampleSize := 100
			if int(keyCount) < sampleSize {
				sampleSize = int(keyCount)
			}
			
			expiredSample := 0
			sampled := 0
			
			for _, value := range db.data {
				if value.IsExpired() {
					expiredSample++
				}
				sampled++
				if sampled >= sampleSize {
					break
				}
			}
			
			// Estimate expired count based on sample ratio
			if sampled > 0 {
				expiredRatio := float64(expiredSample) / float64(sampled)
				expiredCount = int64(expiredRatio * float64(keyCount))
			}
		} else {
			// For smaller databases, count all expired keys precisely
			for _, value := range db.data {
				if value.IsExpired() {
					expiredCount++
				}
			}
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

	for key := range db.data {
		if evicted >= int64(count) {
			break
		}

		delete(db.data, key)
		evicted++
		s.keyCount--

		// Notify observers
		for _, observer := range s.observers {
			observer.OnKeyDeleted(key)
		}
	}

	if evicted > 0 {
		s.updateMemoryUsage()
	}

	return evicted
}

// updateMemoryUsage calculates current memory usage (internal, must hold lock)
func (s *MemoryStorage) updateMemoryUsage() {
	usage := int64(0)

	for _, db := range s.databases {
		for key, value := range db.data {
			usage += int64(len(key))
			usage += s.calculateValueSize(value)
		}
	}

	s.memoryUsage = usage
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

	for dbNum, db := range s.getActiveDatabases() {
		s.cleanupDatabase(dbNum, db, config)
	}
}

// getActiveDatabases returns a snapshot of active databases
func (s *MemoryStorage) getActiveDatabases() map[int]*database {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[int]*database, len(s.databases))
	for dbNum, db := range s.databases {
		result[dbNum] = db
	}
	return result
}

// cleanupDatabase performs incremental cleanup on a single database
func (s *MemoryStorage) cleanupDatabase(dbNum int, db *database, config CleanupConfig) {
	for round := 0; round < config.MaxRounds; round++ {
		expiredKeys := s.sampleAndFindExpired(db, config.SampleSize)

		if len(expiredKeys) == 0 {
			break // No expired keys found, stop cleaning this database
		}

		// Delete expired keys in batches to minimize lock time
		s.deleteExpiredKeysBatched(dbNum, db, expiredKeys, config.BatchSize)

		// Check if we should continue with another round
		expiredRatio := float64(len(expiredKeys)) / float64(config.SampleSize)
		if expiredRatio < config.ExpiredThreshold {
			break // Not many expired keys found, stop cleaning
		}

		// Yield CPU briefly between rounds to allow other operations
		runtime.Gosched()
	}
}

// sampleAndFindExpired samples keys and finds expired ones
func (s *MemoryStorage) sampleAndFindExpired(db *database, sampleSize int) []string {
	// Take a read lock to get a sample of keys
	s.mu.RLock()

	if len(db.data) == 0 {
		s.mu.RUnlock()
		return nil
	}

	// Adjust sample size if needed
	actualSampleSize := sampleSize
	if len(db.data) < sampleSize {
		actualSampleSize = len(db.data)
	}

	// Sample keys randomly using reservoir sampling for better performance
	sampledKeys := make([]string, 0, actualSampleSize)

	if len(db.data) <= actualSampleSize {
		// If we need all keys or close to it, just collect them
		for key := range db.data {
			sampledKeys = append(sampledKeys, key)
		}
	} else {
		// Use reservoir sampling algorithm - more efficient for large datasets
		i := 0
		for key := range db.data {
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
		if value, exists := db.data[key]; exists && value.IsExpired() {
			expiredKeys = append(expiredKeys, key)
		}
	}

	s.mu.RUnlock()
	return expiredKeys
}

// deleteExpiredKeysBatched deletes expired keys in batches to minimize lock time
func (s *MemoryStorage) deleteExpiredKeysBatched(dbNum int, db *database, expiredKeys []string, batchSize int) {
	currentDB := s.getCurrentDB()

	for i := 0; i < len(expiredKeys); i += batchSize {
		end := i + batchSize
		if end > len(expiredKeys) {
			end = len(expiredKeys)
		}

		batch := expiredKeys[i:end]
		s.deleteKeyBatch(dbNum, db, batch, dbNum == currentDB)

		// Yield between batches for better concurrency
		if end < len(expiredKeys) {
			runtime.Gosched()
		}
	}
}

// getCurrentDB returns the current database number
func (s *MemoryStorage) getCurrentDB() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentDB
}

// deleteKeyBatch deletes a batch of keys with minimal lock time
func (s *MemoryStorage) deleteKeyBatch(dbNum int, db *database, keys []string, updateKeyCount bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	deletedCount := 0

	for _, key := range keys {
		if value, exists := db.data[key]; exists {
			// Double-check expiration under write lock
			if value.IsExpired() {
				delete(db.data, key)
				deletedCount++

				// Notify observers
				for _, observer := range s.observers {
					observer.OnKeyExpired(key)
				}
			}
		}
	}

	// Update key count only for current database
	if updateKeyCount && deletedCount > 0 {
		s.keyCount -= int64(deletedCount)
	}

	// Update memory usage only if keys were actually deleted
	if deletedCount > 0 {
		s.updateMemoryUsage()
	}
}

// deleteExpiredKey safely deletes an expired key without race conditions
func (s *MemoryStorage) deleteExpiredKey(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	db := s.databases[s.currentDB]
	value, exists := db.data[key]
	if !exists {
		return
	}

	// Double-check expiration under write lock
	if value.IsExpired() {
		delete(db.data, key)
		s.keyCount--
		s.updateMemoryUsage()

		// Notify observers
		for _, observer := range s.observers {
			observer.OnKeyExpired(key)
		}
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
