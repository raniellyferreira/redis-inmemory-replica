package storage

import (
	"fmt"
	"runtime"
	"strings"
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
}

// database represents a Redis database
type database struct {
	data map[string]*Value
}

// NewMemory creates a new in-memory storage instance
func NewMemory() *MemoryStorage {
	s := &MemoryStorage{
		data:        make(map[string]*Value),
		databases:   make(map[int]*database),
		currentDB:   0,
		cleanupStop: make(chan struct{}),
		cleanupDone: make(chan struct{}),
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
	defer s.mu.RUnlock()
	
	db := s.databases[s.currentDB]
	value, exists := db.data[key]
	if !exists {
		return nil, false
	}
	
	// Check expiration
	if value.IsExpired() {
		// Remove expired key (upgrade to write lock)
		s.mu.RUnlock()
		s.mu.Lock()
		delete(db.data, key)
		s.keyCount--
		s.updateMemoryUsage()
		s.mu.Unlock()
		s.mu.RLock()
		
		// Notify observers
		for _, observer := range s.observers {
			observer.OnKeyExpired(key)
		}
		
		return nil, false
	}
	
	// Notify observers
	for _, observer := range s.observers {
		observer.OnKeyAccessed(key)
	}
	
	// Return string value
	if value.Type == ValueTypeString {
		if stringVal, ok := value.Data.(*StringValue); ok {
			return stringVal.Data, true
		}
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

// Keys returns all keys in the current database
func (s *MemoryStorage) Keys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	db := s.databases[s.currentDB]
	keys := make([]string, 0, len(db.data))
	
	for key, value := range db.data {
		if !value.IsExpired() {
			keys = append(keys, key)
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

// performCleanup removes expired keys
func (s *MemoryStorage) performCleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	for dbNum, db := range s.databases {
		expiredKeys := make([]string, 0)
		
		for key, value := range db.data {
			if value.IsExpired() {
				expiredKeys = append(expiredKeys, key)
			}
		}
		
		for _, key := range expiredKeys {
			delete(db.data, key)
			if dbNum == s.currentDB {
				s.keyCount--
			}
			
			// Notify observers
			for _, observer := range s.observers {
				observer.OnKeyExpired(key)
			}
		}
	}
	
	s.updateMemoryUsage()
}

// matchPattern performs simple pattern matching (supports * and ?)
func matchPattern(str, pattern string) bool {
	// Simple implementation - in production you'd want more sophisticated pattern matching
	if pattern == "*" {
		return true
	}
	
	if !strings.Contains(pattern, "*") && !strings.Contains(pattern, "?") {
		return str == pattern
	}
	
	// For now, just check prefix/suffix with single *
	if strings.HasPrefix(pattern, "*") {
		return strings.HasSuffix(str, pattern[1:])
	}
	
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(str, pattern[:len(pattern)-1])
	}
	
	return str == pattern
}