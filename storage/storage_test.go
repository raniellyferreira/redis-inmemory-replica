package storage_test

import (
	"testing"
	"time"

	"github.com/raniellyferreira/redis-inmemory-replica/storage"
)

func TestMemoryStorage(t *testing.T) {
	s := storage.NewMemory()
	defer s.Close()

	// Test Set and Get
	err := s.Set("key1", []byte("value1"), nil)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	value, exists := s.Get("key1")
	if !exists {
		t.Fatal("Expected key to exist")
	}

	if string(value) != "value1" {
		t.Errorf("Get() = %s, want value1", string(value))
	}

	// Test non-existent key
	_, exists = s.Get("nonexistent")
	if exists {
		t.Fatal("Expected key to not exist")
	}
}

func TestMemoryStorageExpiry(t *testing.T) {
	s := storage.NewMemory()
	defer s.Close()

	// Set key with expiry in the past (should be immediately expired)
	pastTime := time.Now().Add(-1 * time.Hour)
	err := s.Set("expired", []byte("value"), &pastTime)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Key should not exist due to expiry
	_, exists := s.Get("expired")
	if exists {
		t.Fatal("Expected expired key to not exist")
	}

	// Set key with future expiry
	futureTime := time.Now().Add(1 * time.Hour)
	err = s.Set("future", []byte("value"), &futureTime)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Key should exist
	value, exists := s.Get("future")
	if !exists {
		t.Fatal("Expected future key to exist")
	}

	if string(value) != "value" {
		t.Errorf("Get() = %s, want value", string(value))
	}
}

func TestMemoryStorageDel(t *testing.T) {
	s := storage.NewMemory()
	defer s.Close()

	// Set some keys
	s.Set("key1", []byte("value1"), nil)
	s.Set("key2", []byte("value2"), nil)
	s.Set("key3", []byte("value3"), nil)

	// Delete keys
	deleted := s.Del("key1", "key2", "nonexistent")
	if deleted != 2 {
		t.Errorf("Del() = %d, want 2", deleted)
	}

	// Verify deletion
	_, exists := s.Get("key1")
	if exists {
		t.Fatal("Expected key1 to be deleted")
	}

	_, exists = s.Get("key2")
	if exists {
		t.Fatal("Expected key2 to be deleted")
	}

	_, exists = s.Get("key3")
	if !exists {
		t.Fatal("Expected key3 to still exist")
	}
}

func TestMemoryStorageExists(t *testing.T) {
	s := storage.NewMemory()
	defer s.Close()

	s.Set("key1", []byte("value1"), nil)
	s.Set("key2", []byte("value2"), nil)

	count := s.Exists("key1", "key2", "nonexistent")
	if count != 2 {
		t.Errorf("Exists() = %d, want 2", count)
	}

	count = s.Exists("nonexistent1", "nonexistent2")
	if count != 0 {
		t.Errorf("Exists() = %d, want 0", count)
	}
}

func TestMemoryStorageExpire(t *testing.T) {
	s := storage.NewMemory()
	defer s.Close()

	s.Set("key1", []byte("value1"), nil)

	// Set expiry
	futureTime := time.Now().Add(1 * time.Hour)
	result := s.Expire("key1", futureTime)
	if !result {
		t.Fatal("Expected Expire() to return true")
	}

	// Check TTL
	ttl := s.TTL("key1")
	if ttl <= 0 || ttl > time.Hour {
		t.Errorf("TTL() = %v, want positive duration <= 1 hour", ttl)
	}

	// Try to expire non-existent key
	result = s.Expire("nonexistent", futureTime)
	if result {
		t.Fatal("Expected Expire() to return false for non-existent key")
	}
}

func TestMemoryStorageTTL(t *testing.T) {
	s := storage.NewMemory()
	defer s.Close()

	// Key without expiry
	s.Set("key1", []byte("value1"), nil)
	ttl := s.TTL("key1")
	if ttl != -1*time.Second {
		t.Errorf("TTL() = %v, want -1s (no expiry)", ttl)
	}

	// Non-existent key
	ttl = s.TTL("nonexistent")
	if ttl != -2*time.Second {
		t.Errorf("TTL() = %v, want -2s (key doesn't exist)", ttl)
	}

	// Key with expiry
	futureTime := time.Now().Add(1 * time.Hour)
	s.Set("key2", []byte("value2"), &futureTime)
	ttl = s.TTL("key2")
	if ttl <= 0 || ttl > time.Hour {
		t.Errorf("TTL() = %v, want positive duration <= 1 hour", ttl)
	}
}

func TestMemoryStorageKeys(t *testing.T) {
	s := storage.NewMemory()
	defer s.Close()

	// Initially no keys
	keys := s.Keys()
	if len(keys) != 0 {
		t.Errorf("Keys() length = %d, want 0", len(keys))
	}

	// Add some keys
	s.Set("key1", []byte("value1"), nil)
	s.Set("key2", []byte("value2"), nil)
	s.Set("key3", []byte("value3"), nil)

	keys = s.Keys()
	if len(keys) != 3 {
		t.Errorf("Keys() length = %d, want 3", len(keys))
	}

	// Verify key count
	count := s.KeyCount()
	if count != 3 {
		t.Errorf("KeyCount() = %d, want 3", count)
	}
}

func TestMemoryStorageFlushAll(t *testing.T) {
	s := storage.NewMemory()
	defer s.Close()

	// Add some keys
	s.Set("key1", []byte("value1"), nil)
	s.Set("key2", []byte("value2"), nil)

	// Flush all
	err := s.FlushAll()
	if err != nil {
		t.Fatalf("FlushAll() error = %v", err)
	}

	// Verify all keys are gone
	keys := s.Keys()
	if len(keys) != 0 {
		t.Errorf("Keys() length after FlushAll = %d, want 0", len(keys))
	}

	count := s.KeyCount()
	if count != 0 {
		t.Errorf("KeyCount() after FlushAll = %d, want 0", count)
	}
}

func TestMemoryStorageDatabase(t *testing.T) {
	s := storage.NewMemory()
	defer s.Close()

	// Default database should be 0
	currentDB := s.CurrentDB()
	if currentDB != 0 {
		t.Errorf("CurrentDB() = %d, want 0", currentDB)
	}

	// Add key to database 0
	s.Set("key1", []byte("value1"), nil)

	// Switch to database 1
	err := s.SelectDB(1)
	if err != nil {
		t.Fatalf("SelectDB(1) error = %v", err)
	}

	currentDB = s.CurrentDB()
	if currentDB != 1 {
		t.Errorf("CurrentDB() = %d, want 1", currentDB)
	}

	// Key from database 0 should not exist in database 1
	_, exists := s.Get("key1")
	if exists {
		t.Fatal("Expected key1 to not exist in database 1")
	}

	// Add key to database 1
	s.Set("key2", []byte("value2"), nil)

	// Switch back to database 0
	err = s.SelectDB(0)
	if err != nil {
		t.Fatalf("SelectDB(0) error = %v", err)
	}

	// Key1 should exist, key2 should not
	_, exists = s.Get("key1")
	if !exists {
		t.Fatal("Expected key1 to exist in database 0")
	}

	_, exists = s.Get("key2")
	if exists {
		t.Fatal("Expected key2 to not exist in database 0")
	}
}

func TestMemoryStorageScan(t *testing.T) {
	s := storage.NewMemory()
	defer s.Close()

	// Add some keys
	s.Set("user:1", []byte("alice"), nil)
	s.Set("user:2", []byte("bob"), nil)
	s.Set("config:app", []byte("settings"), nil)

	// Scan all keys
	_, keys := s.Scan(0, "*", 10)
	if len(keys) != 3 {
		t.Errorf("Scan() returned %d keys, want 3", len(keys))
	}

	// Scan with pattern
	_, keys = s.Scan(0, "user:*", 10)
	if len(keys) != 2 {
		t.Errorf("Scan() with pattern returned %d keys, want 2", len(keys))
	}
}

func TestMemoryStorageInfo(t *testing.T) {
	s := storage.NewMemory()
	defer s.Close()

	s.Set("key1", []byte("value1"), nil)

	info := s.Info()
	if info == nil {
		t.Fatal("Info() returned nil")
	}

	// Check for expected keys
	expectedKeys := []string{"keys", "memory_usage", "current_db"}
	for _, key := range expectedKeys {
		if _, exists := info[key]; !exists {
			t.Errorf("Info() missing key: %s", key)
		}
	}
}

func TestMemoryStorageMemoryLimit(t *testing.T) {
	s := storage.NewMemory()
	defer s.Close()

	// Set memory limit
	limit := int64(1024)
	s.SetMemoryLimit(limit)

	if s.GetMemoryLimit() != limit {
		t.Errorf("GetMemoryLimit() = %d, want %d", s.GetMemoryLimit(), limit)
	}
}

func BenchmarkMemoryStorageGet(b *testing.B) {
	s := storage.NewMemory()
	defer s.Close()

	s.Set("key", []byte("value"), nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = s.Get("key")
	}
}

func BenchmarkMemoryStorageSet(b *testing.B) {
	s := storage.NewMemory()
	defer s.Close()

	value := []byte("value")
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Set("key", value, nil)
	}
}