package replication

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/raniellyferreira/redis-inmemory-replica/storage"
)

// TestSyncManagerConcurrentWaitForSync tests that multiple goroutines can safely wait for sync completion
// This test verifies the fix for the "close of closed channel" panic
func TestSyncManagerConcurrentWaitForSync(t *testing.T) {
	// Use an in-memory storage for testing
	stor := storage.NewMemory()

	// Create sync manager (note: we won't actually start it to avoid network dependencies)
	sm := NewSyncManager("localhost:6379", stor)

	// Set a mock logger that doesn't output
	sm.SetLogger(&testLogger{t: t})

	// Test concurrent WaitForSync calls
	const numWaiters = 10
	var wg sync.WaitGroup
	errors := make(chan error, numWaiters)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Start multiple goroutines waiting for sync
	for i := 0; i < numWaiters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// This should not panic even with concurrent access
			err := sm.WaitForSync(ctx)
			if err != nil && err != context.DeadlineExceeded {
				errors <- err
			}
		}(i)
	}

	// Give a small delay to ensure all waiters are set up
	time.Sleep(50 * time.Millisecond)

	// Simulate sync completion by calling the sync completion callback
	sm.mu.Lock()
	sm.initialSyncDone = true
	callbacks := make([]func(), len(sm.syncCallbacks))
	copy(callbacks, sm.syncCallbacks)
	sm.mu.Unlock()

	// Execute all callbacks - this should not panic
	for _, callback := range callbacks {
		callback()
	}

	// Wait for all waiters to complete
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success - all waiters completed without panic
	case err := <-errors:
		t.Fatalf("WaitForSync failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("Test timed out - possible deadlock")
	}

	// Check if there were any errors
	select {
	case err := <-errors:
		t.Errorf("Unexpected error: %v", err)
	default:
		// No errors, which is what we want
	}
}

// TestSyncManagerWaitForSyncAfterCompletion tests that WaitForSync returns immediately
// when sync is already complete
func TestSyncManagerWaitForSyncAfterCompletion(t *testing.T) {
	stor := storage.NewMemory()
	sm := NewSyncManager("localhost:6379", stor)
	sm.SetLogger(&testLogger{t: t})

	// Mark sync as already completed
	sm.mu.Lock()
	sm.initialSyncDone = true
	sm.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// This should return immediately without waiting
	start := time.Now()
	err := sm.WaitForSync(ctx)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("WaitForSync failed: %v", err)
	}

	// Should return almost immediately
	if duration > 100*time.Millisecond {
		t.Errorf("WaitForSync took too long: %v", duration)
	}
}

// TestSyncManagerCallbackSafety tests that sync callbacks are executed safely
func TestSyncManagerCallbackSafety(t *testing.T) {
	stor := storage.NewMemory()
	sm := NewSyncManager("localhost:6379", stor)
	sm.SetLogger(&testLogger{t: t})

	// Add multiple callbacks
	callbackCount := 0
	var mu sync.Mutex

	for i := 0; i < 5; i++ {
		sm.OnSyncComplete(func() {
			mu.Lock()
			callbackCount++
			mu.Unlock()
		})
	}

	// Simulate sync completion
	sm.mu.Lock()
	sm.initialSyncDone = true
	callbacks := make([]func(), len(sm.syncCallbacks))
	copy(callbacks, sm.syncCallbacks)
	sm.mu.Unlock()

	// Execute callbacks concurrently
	var wg sync.WaitGroup
	for _, callback := range callbacks {
		wg.Add(1)
		go func(cb func()) {
			defer wg.Done()
			cb()
		}(callback)
	}

	wg.Wait()

	mu.Lock()
	finalCount := callbackCount
	mu.Unlock()

	if finalCount != 5 {
		t.Errorf("Expected 5 callback executions, got %d", finalCount)
	}
}

// testLogger is a minimal logger for testing
type testLogger struct {
	t *testing.T
}

func (l *testLogger) Debug(msg string, fields ...interface{}) {
	// Silent for tests unless needed
}

func (l *testLogger) Info(msg string, fields ...interface{}) {
	l.t.Logf("INFO: %s %v", msg, fields)
}

func (l *testLogger) Error(msg string, fields ...interface{}) {
	l.t.Logf("ERROR: %s %v", msg, fields)
}
