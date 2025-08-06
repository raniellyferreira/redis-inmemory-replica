package storage

import (
	"testing"
	"time"
)

// TestAtomicMemoryStorage_BasicOperations tests basic operations of atomic storage
func TestAtomicMemoryStorage_BasicOperations(t *testing.T) {
	storage := NewAtomicMemory()
	defer storage.Close()
	
	// Test Set and Get
	key := "test_key"
	value := []byte("test_value")
	
	err := storage.Set(key, value, nil)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	
	retrieved, exists := storage.Get(key)
	if !exists {
		t.Fatal("Key should exist after Set")
	}
	
	if string(retrieved) != string(value) {
		t.Fatalf("Expected %s, got %s", string(value), string(retrieved))
	}
	
	// Test KeyCount
	if storage.KeyCount() != 1 {
		t.Fatalf("Expected 1 key, got %d", storage.KeyCount())
	}
	
	// Test Del
	deleted := storage.Del(key)
	if deleted != 1 {
		t.Fatalf("Expected 1 deleted key, got %d", deleted)
	}
	
	_, exists = storage.Get(key)
	if exists {
		t.Fatal("Key should not exist after Del")
	}
	
	if storage.KeyCount() != 0 {
		t.Fatalf("Expected 0 keys, got %d", storage.KeyCount())
	}
}

// TestAtomicMemoryStorage_Expiration tests expiration functionality
func TestAtomicMemoryStorage_Expiration(t *testing.T) {
	storage := NewAtomicMemory()
	defer storage.Close()
	
	key := "expiring_key"
	value := []byte("expiring_value")
	expiry := time.Now().Add(100 * time.Millisecond)
	
	err := storage.Set(key, value, &expiry)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	
	// Should exist initially
	_, exists := storage.Get(key)
	if !exists {
		t.Fatal("Key should exist before expiration")
	}
	
	// Wait for expiration
	time.Sleep(150 * time.Millisecond)
	
	// Should not exist after expiration
	_, exists = storage.Get(key)
	if exists {
		t.Fatal("Key should not exist after expiration")
	}
}

// TestAtomicMemoryStorage_ConcurrentAccess tests concurrent access patterns
func TestAtomicMemoryStorage_ConcurrentAccess(t *testing.T) {
	storage := NewAtomicMemory()
	defer storage.Close()
	
	// Pre-populate with some data
	for i := 0; i < 100; i++ {
		key := string(rune('a' + i%26)) + string(rune('0' + i/26))
		value := []byte("value" + string(rune('0' + i%10)))
		storage.Set(key, value, nil)
	}
	
	// Test concurrent reads and writes
	done := make(chan bool, 10)
	
	// Start multiple readers
	for i := 0; i < 5; i++ {
		go func(readerID int) {
			for j := 0; j < 100; j++ {
				key := string(rune('a' + j%26)) + string(rune('0' + j/26))
				storage.Get(key)
			}
			done <- true
		}(i)
	}
	
	// Start multiple writers
	for i := 0; i < 5; i++ {
		go func(writerID int) {
			for j := 0; j < 50; j++ {
				key := "writer" + string(rune('0' + writerID)) + "_" + string(rune('0' + j%10))
				value := []byte("value_from_writer_" + string(rune('0' + writerID)))
				storage.Set(key, value, nil)
			}
			done <- true
		}(i)
	}
	
	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
	
	// Verify data integrity
	if storage.KeyCount() < 100 {
		t.Fatalf("Expected at least 100 keys, got %d", storage.KeyCount())
	}
}

// TestAtomicMemoryStorage_DatabaseSelection tests database selection
func TestAtomicMemoryStorage_DatabaseSelection(t *testing.T) {
	storage := NewAtomicMemory()
	defer storage.Close()
	
	// Set data in database 0
	storage.Set("key0", []byte("value0"), nil)
	
	// Switch to database 1
	err := storage.SelectDB(1)
	if err != nil {
		t.Fatalf("SelectDB failed: %v", err)
	}
	
	if storage.CurrentDB() != 1 {
		t.Fatalf("Expected current DB to be 1, got %d", storage.CurrentDB())
	}
	
	// Key should not exist in database 1
	_, exists := storage.Get("key0")
	if exists {
		t.Fatal("Key should not exist in database 1")
	}
	
	// Set data in database 1
	storage.Set("key1", []byte("value1"), nil)
	
	// Switch back to database 0
	storage.SelectDB(0)
	
	// Verify key0 still exists in database 0
	value, exists := storage.Get("key0")
	if !exists || string(value) != "value0" {
		t.Fatal("Key0 should exist in database 0")
	}
	
	// Verify key1 doesn't exist in database 0
	_, exists = storage.Get("key1")
	if exists {
		t.Fatal("Key1 should not exist in database 0")
	}
}

// TestAtomicMemoryStorage_Operations tests various operations
func TestAtomicMemoryStorage_Operations(t *testing.T) {
	storage := NewAtomicMemory()
	defer storage.Close()
	
	// Test Exists
	storage.Set("existing", []byte("value"), nil)
	
	count := storage.Exists("existing", "nonexistent")
	if count != 1 {
		t.Fatalf("Expected 1 existing key, got %d", count)
	}
	
	// Test Keys
	storage.Set("key1", []byte("value1"), nil)
	storage.Set("key2", []byte("value2"), nil)
	
	keys := storage.Keys()
	if len(keys) < 3 { // existing, key1, key2
		t.Fatalf("Expected at least 3 keys, got %d", len(keys))
	}
	
	// Test Type
	keyType := storage.Type("key1")
	if keyType != ValueTypeString {
		t.Fatalf("Expected ValueTypeString, got %v", keyType)
	}
	
	// Test MemoryUsage
	usage := storage.MemoryUsage()
	if usage <= 0 {
		t.Fatalf("Expected positive memory usage, got %d", usage)
	}
}

// TestAtomicMemoryStorage_Info tests info functionality
func TestAtomicMemoryStorage_Info(t *testing.T) {
	storage := NewAtomicMemory()
	defer storage.Close()
	
	// Add some data
	for i := 0; i < 10; i++ {
		key := "info_key_" + string(rune('0' + i))
		value := []byte("info_value_" + string(rune('0' + i)))
		storage.Set(key, value, nil)
	}
	
	info := storage.Info()
	
	// Check required fields
	if keys, ok := info["keys"].(int64); !ok || keys != 10 {
		t.Fatalf("Expected 10 keys in info, got %v", info["keys"])
	}
	
	if currentDB, ok := info["current_db"].(int); !ok || currentDB != 0 {
		t.Fatalf("Expected current_db to be 0, got %v", info["current_db"])
	}
	
	if generation, ok := info["generation"].(uint64); !ok || generation == 0 {
		t.Fatalf("Expected non-zero generation, got %v", info["generation"])
	}
	
	// Check performance counters
	if readCount, ok := info["read_count"].(uint64); !ok {
		t.Fatalf("Expected read_count in info, got %v", info["read_count"])
	} else if readCount == 0 {
		t.Fatal("Expected non-zero read count")
	}
	
	if writeCount, ok := info["write_count"].(uint64); !ok {
		t.Fatalf("Expected write_count in info, got %v", info["write_count"])
	} else if writeCount == 0 {
		t.Fatal("Expected non-zero write count")
	}
}