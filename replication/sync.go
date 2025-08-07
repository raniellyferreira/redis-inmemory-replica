package replication

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/raniellyferreira/redis-inmemory-replica/storage"
)

// SyncManager manages synchronization between master and replica
type SyncManager struct {
	client  *Client
	storage storage.Storage

	// Sync state
	mu              sync.RWMutex
	initialSyncDone bool
	syncCallbacks   []func()
	starting        bool // flag to prevent concurrent Start calls

	// Configuration
	maxRetries int
	retryDelay time.Duration
}

// SyncStatus represents the current synchronization status
type SyncStatus struct {
	InitialSyncCompleted bool
	InitialSyncProgress  float64 // 0.0 to 1.0
	Connected            bool
	MasterHost           string
	ReplicationOffset    int64
	LastSyncTime         time.Time
	BytesReceived        int64
	CommandsProcessed    int64
}

// NewSyncManager creates a new synchronization manager
func NewSyncManager(masterAddr string, stor storage.Storage) *SyncManager {
	client := NewClient(masterAddr, stor)

	return &SyncManager{
		client:     client,
		storage:    stor,
		maxRetries: 5,
		retryDelay: time.Second,
	}
}

// SetAuth configures authentication
func (sm *SyncManager) SetAuth(password string) {
	sm.client.SetAuth(password)
}

// SetTLS configures TLS
func (sm *SyncManager) SetTLS(config interface{}) {
	// Note: type assertion would be needed for *tls.Config
	// sm.client.SetTLS(config.(*tls.Config))
}

// SetLogger sets the logger
func (sm *SyncManager) SetLogger(logger Logger) {
	sm.client.SetLogger(logger)
}

// SetMetrics sets the metrics collector
func (sm *SyncManager) SetMetrics(metrics MetricsCollector) {
	sm.client.SetMetrics(metrics)
}

// SetSyncTimeout sets the synchronization timeout
func (sm *SyncManager) SetSyncTimeout(timeout time.Duration) {
	sm.client.SetSyncTimeout(timeout)
}

// SetConnectTimeout sets the connection timeout
func (sm *SyncManager) SetConnectTimeout(timeout time.Duration) {
	sm.client.SetConnectTimeout(timeout)
}

// SetReadTimeout sets the read timeout
func (sm *SyncManager) SetReadTimeout(timeout time.Duration) {
	sm.client.SetReadTimeout(timeout)
}

// SetWriteTimeout sets the write timeout
func (sm *SyncManager) SetWriteTimeout(timeout time.Duration) {
	sm.client.SetWriteTimeout(timeout)
}

// SetCommandFilters sets command filters
func (sm *SyncManager) SetCommandFilters(commands []string) {
	sm.client.SetCommandFilters(commands)
}

// SetDatabases sets which databases to replicate
func (sm *SyncManager) SetDatabases(databases []int) {
	sm.client.SetDatabases(databases)
}

// Start begins synchronization
func (sm *SyncManager) Start(ctx context.Context) error {
	sm.mu.Lock()
	if sm.starting {
		sm.mu.Unlock()
		sm.client.logger.Debug("Sync already starting, skipping duplicate Start call")
		return nil
	}
	sm.starting = true
	sm.mu.Unlock()

	defer func() {
		sm.mu.Lock()
		sm.starting = false
		sm.mu.Unlock()
	}()

	// Register sync completion callback
	sm.client.OnSyncComplete(func() {
		sm.mu.Lock()
		sm.initialSyncDone = true
		callbacks := make([]func(), len(sm.syncCallbacks))
		copy(callbacks, sm.syncCallbacks)
		sm.mu.Unlock()

		// Notify callbacks
		for _, callback := range callbacks {
			callback()
		}
	})

	// Start replication with retries
	var lastErr error
	for i := 0; i < sm.maxRetries; i++ {
		if err := sm.client.Start(ctx); err != nil {
			lastErr = err
			if i < sm.maxRetries-1 {
				select {
				case <-time.After(sm.retryDelay):
					continue
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		} else {
			return nil
		}
	}

	return fmt.Errorf("failed to start sync after %d retries: %w", sm.maxRetries, lastErr)
}

// Stop stops synchronization
func (sm *SyncManager) Stop() error {
	return sm.client.Stop()
}

// WaitForSync blocks until initial synchronization is complete
func (sm *SyncManager) WaitForSync(ctx context.Context) error {
	// Check if already synced
	sm.mu.RLock()
	if sm.initialSyncDone {
		sm.mu.RUnlock()
		return nil
	}
	sm.mu.RUnlock()

	// Wait for sync completion
	syncDone := make(chan struct{})

	sm.mu.Lock()
	sm.syncCallbacks = append(sm.syncCallbacks, func() {
		close(syncDone)
	})
	sm.mu.Unlock()

	select {
	case <-syncDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// SyncStatus returns the current synchronization status
func (sm *SyncManager) SyncStatus() SyncStatus {
	stats := sm.client.Stats()

	sm.mu.RLock()
	initialSyncDone := sm.initialSyncDone
	sm.mu.RUnlock()

	return SyncStatus{
		InitialSyncCompleted: initialSyncDone,
		InitialSyncProgress:  stats.InitialSyncProgress,
		Connected:            stats.Connected,
		MasterHost:           stats.MasterAddr,
		ReplicationOffset:    stats.ReplicationOffset,
		LastSyncTime:         stats.LastSyncTime,
		BytesReceived:        stats.BytesReceived,
		CommandsProcessed:    stats.CommandsProcessed,
	}
}

// OnSyncComplete registers a callback for when initial sync completes
func (sm *SyncManager) OnSyncComplete(fn func()) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.initialSyncDone {
		// Already synced, call immediately
		go fn()
		return
	}

	sm.syncCallbacks = append(sm.syncCallbacks, fn)
}

// IsConnected returns true if connected to master
func (sm *SyncManager) IsConnected() bool {
	stats := sm.client.Stats()
	return stats.Connected
}

// GetStats returns detailed replication statistics
func (sm *SyncManager) GetStats() ReplicationStats {
	return sm.client.Stats()
}

// Storage returns the underlying storage
func (sm *SyncManager) Storage() storage.Storage {
	return sm.storage
}
