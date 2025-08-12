package redisreplica

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/raniellyferreira/redis-inmemory-replica/replication"
	"github.com/raniellyferreira/redis-inmemory-replica/server"
	"github.com/raniellyferreira/redis-inmemory-replica/storage"
)

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

// Replica represents an in-memory Redis replica
type Replica struct {
	// Configuration
	config *config

	// Components
	storage storage.Storage
	syncMgr *replication.SyncManager
	server  *server.Server

	// State
	mu      sync.RWMutex
	started bool
	closed  bool

	// Statistics (exported for monitoring)
	Stats ReplicationStats

	// Callbacks
	syncCallbacks []func()
}

// New creates a new Replica with the given options
//
// The replica is created but not started. Use Start() to begin replication.
//
// Example:
//
//	replica, err := redisreplica.New(
//		redisreplica.WithMaster("localhost:6379"),
//		redisreplica.WithReplicaAddr(":6380"),
//	)
//	if err != nil {
//		log.Fatal(err)
//	}
//
// Since: v1.0.0
func New(opts ...Option) (*Replica, error) {
	cfg := defaultConfig()

	// Apply options
	for _, opt := range opts {
		if err := opt(cfg); err != nil {
			return nil, err
		}
	}

	// Create storage
	stor := storage.NewMemory()
	if cfg.maxMemory > 0 {
		stor.SetMemoryLimit(cfg.maxMemory)
	}
	
	// Set the default database
	if cfg.defaultDatabase != 0 {
		if err := stor.SelectDB(cfg.defaultDatabase); err != nil {
			return nil, fmt.Errorf("failed to select default database %d: %w", cfg.defaultDatabase, err)
		}
	}

	// Create sync manager
	syncMgr := replication.NewSyncManager(cfg.masterAddr, stor)

	// Configure sync manager
	if cfg.masterPassword != "" {
		syncMgr.SetAuth(cfg.masterPassword)
	}

	if cfg.masterTLS != nil {
		syncMgr.SetTLS(cfg.masterTLS)
	}

	syncMgr.SetLogger(&replicationLogger{logger: cfg.logger})
	if cfg.metrics != nil {
		syncMgr.SetMetrics(&metricsAdapter{metrics: cfg.metrics})
	}

	syncMgr.SetSyncTimeout(cfg.syncTimeout)
	syncMgr.SetConnectTimeout(cfg.connectTimeout)
	syncMgr.SetReadTimeout(cfg.readTimeout)
	syncMgr.SetWriteTimeout(cfg.writeTimeout)
	syncMgr.SetHeartbeatInterval(cfg.heartbeatInterval)
	syncMgr.SetCommandFilters(cfg.commandFilters)
	syncMgr.SetDatabases(cfg.databases)

	replica := &Replica{
		config:  cfg,
		storage: stor,
		syncMgr: syncMgr,
		Stats: ReplicationStats{
			CommandsProcessed: make(map[string]int64),
		},
	}

	// Create server if replica address is provided
	if cfg.replicaAddr != "" {
		replica.server = server.NewServer(cfg.replicaAddr, stor)
		if cfg.replicaPassword != "" {
			replica.server.SetPassword(cfg.replicaPassword)
		}
		// Set server to track sync status for LOADING state
		replica.server.SetSyncManager(syncMgr)
		// Configure write redirection if enabled
		if cfg.redirectWrites {
			replica.server.SetWriteRedirection(true, cfg.masterAddr, cfg.masterPassword)
		}
	}

	// Register sync completion callback
	syncMgr.OnSyncComplete(func() {
		replica.mu.RLock()
		callbacks := make([]func(), len(replica.syncCallbacks))
		copy(callbacks, replica.syncCallbacks)
		replica.mu.RUnlock()

		for _, callback := range callbacks {
			callback()
		}
	})

	return replica, nil
}

// Start begins replication and starts the local server
//
// This method blocks until the initial connection is established or fails.
// Use WaitForSync() to wait for initial synchronization to complete.
//
// Example:
//
//	if err := replica.Start(context.Background()); err != nil {
//		log.Fatal(err)
//	}
//
// Since: v1.0.0
func (r *Replica) Start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return ErrClosed
	}

	if r.started {
		return nil // Already started
	}

	// Start replication in background (don't block server startup)
	go func() {
		if err := r.syncMgr.Start(context.Background()); err != nil {
			r.config.logger.Error("Failed to start replication", Field{Key: "error", Value: err})
		}
	}()

	r.started = true

	// Start server if replica address is configured
	if r.server != nil {
		if err := r.server.Start(); err != nil {
			r.config.logger.Error("Failed to start server", Field{Key: "error", Value: err}, Field{Key: "addr", Value: r.config.replicaAddr})
			return err
		}
		r.config.logger.Info("Replica server listening", Field{Key: "addr", Value: r.server.Addr()})
	}

	return nil
}

// WaitForSync blocks until initial synchronization is complete or context is cancelled
//
// This method should be called after Start() to ensure the replica is fully
// synchronized before serving read requests.
//
// Example:
//
//	if err := replica.WaitForSync(ctx); err != nil {
//		log.Fatal(err)
//	}
//
// Since: v1.0.0
func (r *Replica) WaitForSync(ctx context.Context) error {
	if !r.isStarted() {
		return ErrNotConnected
	}

	return r.syncMgr.WaitForSync(ctx)
}

// SyncStatus returns the current synchronization status
//
// The returned status includes information about connection state,
// sync progress, and replication statistics.
//
// Example:
//
//	status := replica.SyncStatus()
//	fmt.Printf("Sync completed: %v\n", status.InitialSyncCompleted)
//	fmt.Printf("Replication offset: %d\n", status.ReplicationOffset)
//
// Since: v1.0.0
func (r *Replica) SyncStatus() SyncStatus {
	status := r.syncMgr.SyncStatus()

	return SyncStatus{
		InitialSyncCompleted: status.InitialSyncCompleted,
		InitialSyncProgress:  status.InitialSyncProgress,
		Connected:            status.Connected,
		MasterHost:           status.MasterHost,
		ReplicationOffset:    status.ReplicationOffset,
		LastSyncTime:         status.LastSyncTime,
		BytesReceived:        status.BytesReceived,
		CommandsProcessed:    status.CommandsProcessed,
	}
}

// Close gracefully shuts down the replica
//
// This method stops replication, closes connections, and cleans up resources.
// It should be called when the replica is no longer needed.
//
// Example:
//
//	defer replica.Close()
//
// Since: v1.0.0
func (r *Replica) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil
	}

	r.closed = true

	// Stop server first
	if r.server != nil {
		if err := r.server.Stop(); err != nil {
			r.config.logger.Error("Error stopping server", Field{Key: "error", Value: err})
		}
	}

	// Stop replication
	if r.started {
		if err := r.syncMgr.Stop(); err != nil {
			return err
		}
	}

	// Close storage
	if err := r.storage.Close(); err != nil {
		return err
	}

	return nil
}

// OnSyncComplete registers a callback for when initial sync completes
//
// The callback will be called once when the initial synchronization
// with the master is complete. If sync is already complete, the callback
// is called immediately.
//
// Example:
//
//	replica.OnSyncComplete(func() {
//		log.Println("Replica is ready to serve requests")
//	})
//
// Since: v1.0.0
func (r *Replica) OnSyncComplete(fn func()) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.syncCallbacks = append(r.syncCallbacks, fn)
}

// Storage returns the underlying storage for direct access
//
// This provides direct access to the storage layer for advanced use cases.
// Most users should use the Redis-compatible server interface instead.
//
// Example:
//
//	value, exists := replica.Storage().Get("mykey")
//	if exists {
//		fmt.Printf("Value: %s\n", value)
//	}
//
// Since: v1.0.0
func (r *Replica) Storage() storage.Storage {
	return r.storage
}

// IsConnected returns true if the replica is connected to the master
//
// This is a convenience method that checks the connection status
// without retrieving the full sync status.
//
// Example:
//
//	if replica.IsConnected() {
//		// Replica is connected to master
//	}
//
// Since: v1.0.0
func (r *Replica) IsConnected() bool {
	return r.syncMgr.IsConnected()
}

// GetInfo returns detailed information about the replica
//
// This includes storage statistics, replication status, and configuration.
//
// Example:
//
//	info := replica.GetInfo()
//	fmt.Printf("Key count: %v\n", info["keys"])
//	fmt.Printf("Memory usage: %v\n", info["memory_usage"])
//
// Since: v1.0.0
func (r *Replica) GetInfo() map[string]interface{} {
	info := r.storage.Info()

	// Add replication info
	status := r.SyncStatus()
	info["replication"] = map[string]interface{}{
		"connected":              status.Connected,
		"master_host":            status.MasterHost,
		"initial_sync_completed": status.InitialSyncCompleted,
		"replication_offset":     status.ReplicationOffset,
		"commands_processed":     status.CommandsProcessed,
	}

	// Add version info
	info["version"] = VersionInfo()

	return info
}

// isStarted returns true if the replica is started (thread-safe)
func (r *Replica) isStarted() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.started && !r.closed
}
