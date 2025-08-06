package storage

import (
	"fmt"
	randv2 "math/rand/v2"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// atomicData represents the immutable data structure stored in atomic.Value
type atomicData struct {
	// databases contains all database data
	databases map[int]*atomicDatabase
	// currentDB is the currently selected database
	currentDB int
	// keyCount is the total number of keys in current database
	keyCount int64
	// memoryUsage is the estimated memory usage
	memoryUsage int64
	// generation is incremented on each update for ABA protection
	generation uint64
}

// atomicDatabase represents an immutable database
type atomicDatabase struct {
	data map[string]*Value
}

// AtomicMemoryStorage implements an atomic.Value-based storage engine
// This implementation uses copy-on-write semantics for thread-safe access
// without locks on read operations.
type AtomicMemoryStorage struct {
	// data holds the current atomicData snapshot
	data atomic.Value
	
	// writeMu protects write operations to prevent multiple concurrent writers
	// Only write operations need this mutex, reads are completely lock-free
	writeMu sync.Mutex
	
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
	
	// Performance counters
	writeCount atomic.Uint64
	readCount  atomic.Uint64
	copyCount  atomic.Uint64
}

// NewAtomicMemory creates a new atomic.Value-based storage instance
func NewAtomicMemory() *AtomicMemoryStorage {
	s := &AtomicMemoryStorage{
		cleanupStop:   make(chan struct{}),
		cleanupDone:   make(chan struct{}),
		cleanupConfig: CleanupConfigDefault,
		rng:          randv2.New(randv2.NewPCG(uint64(time.Now().UnixNano()), 0)),
	}
	
	// Initialize with empty data
	initialData := &atomicData{
		databases: map[int]*atomicDatabase{
			0: {data: make(map[string]*Value)},
		},
		currentDB:   0,
		keyCount:    0,
		memoryUsage: 0,
		generation:  1,
	}
	s.data.Store(initialData)
	
	// Start background cleanup goroutine
	go s.cleanupExpiredKeys()
	
	return s
}

// loadData is a helper to load current data with type assertion
func (s *AtomicMemoryStorage) loadData() *atomicData {
	return s.data.Load().(*atomicData)
}

// Get retrieves a value by key (lock-free)
func (s *AtomicMemoryStorage) Get(key string) ([]byte, bool) {
	s.readCount.Add(1)
	
	// Lock-free read operation
	data := s.loadData()
	db, exists := data.databases[data.currentDB]
	if !exists {
		return nil, false
	}
	
	value, exists := db.data[key]
	if !exists {
		return nil, false
	}
	
	// Check expiration
	if value.IsExpired() {
		// Trigger async cleanup for expired key
		go s.deleteExpiredKeyAsync(key)
		return nil, false
	}
	
	// Copy the data to avoid sharing mutable data
	var result []byte
	if value.Type == ValueTypeString {
		if stringVal, ok := value.Data.(*StringValue); ok {
			result = make([]byte, len(stringVal.Data))
			copy(result, stringVal.Data)
		}
	}
	
	// Notify observers (this is safe because data is immutable)
	for _, observer := range s.observers {
		observer.OnKeyAccessed(key)
	}
	
	if result != nil {
		return result, true
	}
	
	return nil, false
}

// Set stores a value with optional expiration (uses copy-on-write)
func (s *AtomicMemoryStorage) Set(key string, value []byte, expiry *time.Time) error {
	s.writeCount.Add(1)
	
	// Serialize write operations
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	
	for {
		oldData := s.loadData()
		newData := s.copyData(oldData)
		
		// Get current database
		db, exists := newData.databases[newData.currentDB]
		if !exists {
			db = &atomicDatabase{data: make(map[string]*Value)}
			newData.databases[newData.currentDB] = db
		}
		
		// Create new value
		newValue := &Value{
			Type:    ValueTypeString,
			Data:    &StringValue{Data: append([]byte(nil), value...)},
			Expiry:  expiry,
			Version: time.Now().UnixNano(),
		}
		
		// Check if key already exists
		_, exists = db.data[key]
		if !exists {
			newData.keyCount++
		}
		
		// Update the database data (creates new map)
		newDB := &atomicDatabase{
			data: make(map[string]*Value, len(db.data)+1),
		}
		for k, v := range db.data {
			newDB.data[k] = v
		}
		newDB.data[key] = newValue
		newData.databases[newData.currentDB] = newDB
		
		// Update memory usage
		newData.memoryUsage = s.calculateDataMemoryUsage(newData)
		newData.generation++
		
		// Atomic update
		if s.data.CompareAndSwap(oldData, newData) {
			s.copyCount.Add(1)
			break
		}
		// If CAS failed, retry with updated data
	}
	
	// Notify observers
	for _, observer := range s.observers {
		observer.OnKeySet(key, value)
	}
	
	// Check memory limit
	if s.memoryLimit > 0 {
		currentData := s.loadData()
		if currentData.memoryUsage > s.memoryLimit {
			s.evictLRU(1)
		}
	}
	
	return nil
}

// Del deletes one or more keys
func (s *AtomicMemoryStorage) Del(keys ...string) int64 {
	s.writeCount.Add(1)
	
	if len(keys) == 0 {
		return 0
	}
	
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	
	for {
		oldData := s.loadData()
		newData := s.copyData(oldData)
		
		db, exists := newData.databases[newData.currentDB]
		if !exists {
			return 0
		}
		
		// Create new database without deleted keys
		newDB := &atomicDatabase{
			data: make(map[string]*Value, len(db.data)),
		}
		deleted := int64(0)
		
		for k, v := range db.data {
			shouldDelete := false
			for _, delKey := range keys {
				if k == delKey {
					shouldDelete = true
					deleted++
					break
				}
			}
			if !shouldDelete {
				newDB.data[k] = v
			}
		}
		
		if deleted == 0 {
			return 0 // No keys were deleted
		}
		
		newData.databases[newData.currentDB] = newDB
		newData.keyCount -= deleted
		newData.memoryUsage = s.calculateDataMemoryUsage(newData)
		newData.generation++
		
		if s.data.CompareAndSwap(oldData, newData) {
			s.copyCount.Add(1)
			
			// Notify observers
			for _, delKey := range keys {
				for _, observer := range s.observers {
					observer.OnKeyDeleted(delKey)
				}
			}
			
			return deleted
		}
	}
}

// Exists checks if keys exist (lock-free)
func (s *AtomicMemoryStorage) Exists(keys ...string) int64 {
	s.readCount.Add(1)
	
	data := s.loadData()
	db, exists := data.databases[data.currentDB]
	if !exists {
		return 0
	}
	
	count := int64(0)
	for _, key := range keys {
		if value, exists := db.data[key]; exists && !value.IsExpired() {
			count++
		}
	}
	
	return count
}

// Expire sets expiration for a key
func (s *AtomicMemoryStorage) Expire(key string, expiry time.Time) bool {
	s.writeCount.Add(1)
	
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	
	for {
		oldData := s.loadData()
		db, exists := oldData.databases[oldData.currentDB]
		if !exists {
			return false
		}
		
		value, exists := db.data[key]
		if !exists || value.IsExpired() {
			return false
		}
		
		newData := s.copyData(oldData)
		
		// Create new database with updated expiry
		newDB := &atomicDatabase{
			data: make(map[string]*Value, len(db.data)),
		}
		for k, v := range db.data {
			if k == key {
				// Create new value with updated expiry
				newValue := *v // Copy the value
				newValue.Expiry = &expiry
				newDB.data[k] = &newValue
			} else {
				newDB.data[k] = v
			}
		}
		
		newData.databases[newData.currentDB] = newDB
		newData.generation++
		
		if s.data.CompareAndSwap(oldData, newData) {
			s.copyCount.Add(1)
			return true
		}
	}
}

// TTL returns the time to live for a key (lock-free)
func (s *AtomicMemoryStorage) TTL(key string) time.Duration {
	s.readCount.Add(1)
	
	data := s.loadData()
	db, exists := data.databases[data.currentDB]
	if !exists {
		return -2 * time.Second
	}
	
	value, exists := db.data[key]
	if !exists {
		return -2 * time.Second
	}
	
	if value.IsExpired() {
		return -2 * time.Second
	}
	
	if value.Expiry == nil {
		return -1 * time.Second
	}
	
	return time.Until(*value.Expiry)
}

// Keys returns all keys in the current database (lock-free)
func (s *AtomicMemoryStorage) Keys() []string {
	s.readCount.Add(1)
	
	data := s.loadData()
	db, exists := data.databases[data.currentDB]
	if !exists {
		return []string{}
	}
	
	keys := make([]string, 0, len(db.data))
	for key, value := range db.data {
		if !value.IsExpired() {
			keys = append(keys, key)
		}
	}
	
	return keys
}

// KeyCount returns the number of keys in the current database (lock-free)
func (s *AtomicMemoryStorage) KeyCount() int64 {
	s.readCount.Add(1)
	return s.loadData().keyCount
}

// FlushAll removes all keys from all databases
func (s *AtomicMemoryStorage) FlushAll() error {
	s.writeCount.Add(1)
	
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	
	newData := &atomicData{
		databases: map[int]*atomicDatabase{
			0: {data: make(map[string]*Value)},
		},
		currentDB:   0,
		keyCount:    0,
		memoryUsage: 0,
		generation:  s.loadData().generation + 1,
	}
	
	s.data.Store(newData)
	s.copyCount.Add(1)
	return nil
}

// Type returns the type of a key (lock-free)
func (s *AtomicMemoryStorage) Type(key string) ValueType {
	s.readCount.Add(1)
	
	data := s.loadData()
	db, exists := data.databases[data.currentDB]
	if !exists {
		return ValueTypeString
	}
	
	value, exists := db.data[key]
	if !exists || value.IsExpired() {
		return ValueTypeString
	}
	
	return value.Type
}

// MemoryUsage returns current memory usage in bytes (lock-free)
func (s *AtomicMemoryStorage) MemoryUsage() int64 {
	s.readCount.Add(1)
	return s.loadData().memoryUsage
}

// SelectDB selects a database
func (s *AtomicMemoryStorage) SelectDB(db int) error {
	s.writeCount.Add(1)
	
	if db < 0 || db > 15 {
		return fmt.Errorf("invalid database number: %d", db)
	}
	
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	
	for {
		oldData := s.loadData()
		newData := s.copyData(oldData)
		
		// Create database if it doesn't exist
		if _, exists := newData.databases[db]; !exists {
			newData.databases[db] = &atomicDatabase{
				data: make(map[string]*Value),
			}
		}
		
		newData.currentDB = db
		
		// Recalculate key count for current database
		if selectedDB, exists := newData.databases[db]; exists {
			newData.keyCount = int64(len(selectedDB.data))
		}
		
		newData.generation++
		
		if s.data.CompareAndSwap(oldData, newData) {
			s.copyCount.Add(1)
			return nil
		}
	}
}

// CurrentDB returns the current database number (lock-free)
func (s *AtomicMemoryStorage) CurrentDB() int {
	s.readCount.Add(1)
	return s.loadData().currentDB
}

// Scan provides cursor-based iteration over keys (lock-free)
func (s *AtomicMemoryStorage) Scan(cursor int64, match string, count int64) (int64, []string) {
	s.readCount.Add(1)
	
	data := s.loadData()
	db, exists := data.databases[data.currentDB]
	if !exists {
		return 0, []string{}
	}
	
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

// Info returns storage information (lock-free)
func (s *AtomicMemoryStorage) Info() map[string]interface{} {
	s.readCount.Add(1)
	
	data := s.loadData()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	
	return map[string]interface{}{
		"keys":         data.keyCount,
		"memory_usage": data.memoryUsage,
		"memory_limit": s.memoryLimit,
		"current_db":   data.currentDB,
		"databases":    len(data.databases),
		"go_memory":    m.Alloc,
		"generation":   data.generation,
		"read_count":   s.readCount.Load(),
		"write_count":  s.writeCount.Load(),
		"copy_count":   s.copyCount.Load(),
	}
}

// Close shuts down the storage
func (s *AtomicMemoryStorage) Close() error {
	close(s.cleanupStop)
	<-s.cleanupDone
	return nil
}

// SetMemoryLimit sets the memory limit
func (s *AtomicMemoryStorage) SetMemoryLimit(bytes int64) {
	atomic.StoreInt64(&s.memoryLimit, bytes)
}

// GetMemoryLimit returns the current memory limit
func (s *AtomicMemoryStorage) GetMemoryLimit() int64 {
	return atomic.LoadInt64(&s.memoryLimit)
}

// EvictLRU evicts the least recently used keys
func (s *AtomicMemoryStorage) EvictLRU(count int) int64 {
	s.writeCount.Add(1)
	
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.evictLRU(count)
}

// AddObserver adds a storage observer
func (s *AtomicMemoryStorage) AddObserver(observer StorageObserver) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.observers = append(s.observers, observer)
}

// SetCleanupConfig updates the cleanup configuration
func (s *AtomicMemoryStorage) SetCleanupConfig(config CleanupConfig) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.cleanupConfig = config
}

// GetCleanupConfig returns the current cleanup configuration
func (s *AtomicMemoryStorage) GetCleanupConfig() CleanupConfig {
	return s.cleanupConfig
}

// copyData creates a deep copy of atomicData for copy-on-write operations
func (s *AtomicMemoryStorage) copyData(data *atomicData) *atomicData {
	newData := &atomicData{
		databases:   make(map[int]*atomicDatabase, len(data.databases)),
		currentDB:   data.currentDB,
		keyCount:    data.keyCount,
		memoryUsage: data.memoryUsage,
		generation:  data.generation,
	}
	
	// Copy database references (databases themselves are immutable)
	for dbNum, db := range data.databases {
		newData.databases[dbNum] = db
	}
	
	return newData
}

// calculateDataMemoryUsage estimates memory usage for the data structure
func (s *AtomicMemoryStorage) calculateDataMemoryUsage(data *atomicData) int64 {
	usage := int64(0)
	
	for _, db := range data.databases {
		for key, value := range db.data {
			usage += int64(len(key))
			usage += s.calculateValueSize(value)
		}
	}
	
	return usage
}

// calculateValueSize estimates the size of a value in bytes
func (s *AtomicMemoryStorage) calculateValueSize(value *Value) int64 {
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
	}
	
	return size
}

// evictLRU evicts least recently used keys (internal, must hold writeMu)
func (s *AtomicMemoryStorage) evictLRU(count int) int64 {
	// Simplified LRU implementation for demonstration
	for {
		oldData := s.loadData()
		db, exists := oldData.databases[oldData.currentDB]
		if !exists || len(db.data) == 0 {
			return 0
		}
		
		// Create new database without some keys
		newDB := &atomicDatabase{
			data: make(map[string]*Value, len(db.data)),
		}
		
		evicted := int64(0)
		for key, value := range db.data {
			if evicted >= int64(count) {
				newDB.data[key] = value
			} else {
				evicted++
				// Notify observers
				for _, observer := range s.observers {
					observer.OnKeyDeleted(key)
				}
			}
		}
		
		if evicted == 0 {
			return 0
		}
		
		newData := s.copyData(oldData)
		newData.databases[newData.currentDB] = newDB
		newData.keyCount -= evicted
		newData.memoryUsage = s.calculateDataMemoryUsage(newData)
		newData.generation++
		
		if s.data.CompareAndSwap(oldData, newData) {
			s.copyCount.Add(1)
			return evicted
		}
	}
}

// deleteExpiredKeyAsync asynchronously deletes an expired key
func (s *AtomicMemoryStorage) deleteExpiredKeyAsync(key string) {
	s.Del(key)
}

// cleanupExpiredKeys runs in background to clean up expired keys
func (s *AtomicMemoryStorage) cleanupExpiredKeys() {
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

// performCleanup removes expired keys using sampling approach
func (s *AtomicMemoryStorage) performCleanup() {
	config := s.GetCleanupConfig()
	data := s.loadData()
	
	for _, db := range data.databases {
		if len(db.data) == 0 {
			continue
		}
		
		// Sample keys to find expired ones
		expiredKeys := s.sampleAndFindExpiredAtomic(db, config.SampleSize)
		if len(expiredKeys) == 0 {
			continue
		}
		
		// Delete expired keys if enough were found
		expiredRatio := float64(len(expiredKeys)) / float64(config.SampleSize)
		if expiredRatio >= config.ExpiredThreshold {
			// Use background deletion to avoid blocking
			go func(keys []string) {
				s.Del(keys...)
			}(expiredKeys)
		}
	}
}

// sampleAndFindExpiredAtomic samples keys and finds expired ones (lock-free)
func (s *AtomicMemoryStorage) sampleAndFindExpiredAtomic(db *atomicDatabase, sampleSize int) []string {
	if len(db.data) == 0 {
		return nil
	}
	
	actualSampleSize := sampleSize
	if len(db.data) < sampleSize {
		actualSampleSize = len(db.data)
	}
	
	expiredKeys := make([]string, 0, actualSampleSize)
	sampled := 0
	
	for key, value := range db.data {
		if sampled >= actualSampleSize {
			break
		}
		
		if value.IsExpired() {
			expiredKeys = append(expiredKeys, key)
		}
		
		sampled++
	}
	
	return expiredKeys
}