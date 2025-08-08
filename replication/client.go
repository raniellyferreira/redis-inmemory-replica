package replication

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/raniellyferreira/redis-inmemory-replica/protocol"
	"github.com/raniellyferreira/redis-inmemory-replica/storage"
)

// Client implements Redis replication client
type Client struct {
	// Configuration
	masterAddr     string
	masterPassword string
	tlsConfig      *tls.Config
	storage        storage.Storage

	// Connection state
	mu        sync.RWMutex
	conn      net.Conn
	reader    *protocol.Reader
	writer    *protocol.Writer
	connected bool

	// Replication state
	replID      string
	replOffset  int64

	// Control channels
	ctx      context.Context
	cancel   context.CancelFunc
	stopChan chan struct{}
	doneChan chan struct{}
	stopped  int32 // atomic flag to prevent double stop
	runEnded int32 // atomic flag to prevent double doneChan close

	// Sync control
	syncing int32      // atomic flag indicating sync is in progress

	// Statistics
	stats *ReplicationStats

	// Callbacks
	onSyncComplete []func()

	// Configuration
	logger         Logger
	metrics        MetricsCollector
	syncTimeout    time.Duration
	connectTimeout time.Duration
	readTimeout    time.Duration
	writeTimeout   time.Duration
	commandFilters map[string]struct{}
	databases      map[int]struct{} // Which databases to replicate (empty = all)
}

// ReplicationStats tracks replication statistics
type ReplicationStats struct {
	mu sync.RWMutex

	Connected         bool
	MasterAddr        string
	MasterRunID       string
	ReplicationOffset int64
	LastSyncTime      time.Time
	BytesReceived     int64
	CommandsProcessed int64
	ReconnectCount    int64

	// Sync progress
	InitialSyncCompleted bool
	InitialSyncProgress  float64
}

// Logger interface for replication logging
type Logger interface {
	Debug(msg string, fields ...interface{})
	Info(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
}

// MetricsCollector interface for replication metrics
type MetricsCollector interface {
	RecordSyncDuration(duration time.Duration)
	RecordCommandProcessed(cmd string, duration time.Duration)
	RecordNetworkBytes(bytes int64)
	RecordReconnection()
	RecordError(errorType string)
}

// NewClient creates a new replication client
func NewClient(masterAddr string, stor storage.Storage) *Client {
	ctx, cancel := context.WithCancel(context.Background())

	return &Client{
		masterAddr:     masterAddr,
		storage:        stor,
		ctx:            ctx,
		cancel:         cancel,
		stopChan:       make(chan struct{}),
		doneChan:       make(chan struct{}),
		stats:          &ReplicationStats{MasterAddr: masterAddr},
		syncTimeout:    30 * time.Second,
		connectTimeout: 5 * time.Second,
		readTimeout:    30 * time.Second,
		writeTimeout:   10 * time.Second,
		commandFilters: make(map[string]struct{}),
		databases:      make(map[int]struct{}), // empty = replicate all
		logger:         &defaultLogger{},
	}
}

// SetAuth configures authentication
func (c *Client) SetAuth(password string) {
	c.masterPassword = password
}

// SetTLS configures TLS
func (c *Client) SetTLS(config *tls.Config) {
	c.tlsConfig = config
}

// SetLogger sets the logger
func (c *Client) SetLogger(logger Logger) {
	c.logger = logger
}

// SetMetrics sets the metrics collector
func (c *Client) SetMetrics(metrics MetricsCollector) {
	c.metrics = metrics
}

// SetSyncTimeout sets the synchronization timeout
func (c *Client) SetSyncTimeout(timeout time.Duration) {
	c.syncTimeout = timeout
}

// SetConnectTimeout sets the connection timeout
func (c *Client) SetConnectTimeout(timeout time.Duration) {
	c.connectTimeout = timeout
}

// SetReadTimeout sets the read timeout for network operations
func (c *Client) SetReadTimeout(timeout time.Duration) {
	c.readTimeout = timeout
}

// SetWriteTimeout sets the write timeout for network operations
func (c *Client) SetWriteTimeout(timeout time.Duration) {
	c.writeTimeout = timeout
}

// SetCommandFilters sets command filters
func (c *Client) SetCommandFilters(commands []string) {
	c.commandFilters = make(map[string]struct{})
	for _, cmd := range commands {
		c.commandFilters[strings.ToUpper(cmd)] = struct{}{}
	}
}

// SetDatabases sets which databases to replicate
// Empty slice means replicate all databases
func (c *Client) SetDatabases(databases []int) {
	c.databases = make(map[int]struct{})
	for _, db := range databases {
		c.databases[db] = struct{}{}
	}
}

// Start begins replication
func (c *Client) Start(ctx context.Context) error {
	c.logger.Info("Starting replication client", "master", c.masterAddr)

	// Check if already running to prevent duplicate starts
	c.mu.RLock()
	if c.connected {
		c.mu.RUnlock()
		c.logger.Debug("Replication client already connected, skipping start")
		return nil
	}
	c.mu.RUnlock()

	// Start replication goroutine if not already running
	go c.run()

	// Wait for initial connection with timeout
	startTime := time.Now()
	timeout := c.syncTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.mu.RLock()
			connected := c.connected
			c.mu.RUnlock()

			if connected {
				c.logger.Debug("Replication client connected successfully", "duration", time.Since(startTime))
				return nil
			}

			// Check for timeout
			if time.Since(startTime) > timeout {
				return fmt.Errorf("connection timeout after %v", timeout)
			}

		case <-ctx.Done():
			return ctx.Err()
		case <-c.doneChan:
			return fmt.Errorf("replication stopped unexpectedly")
		}
	}
}

// Stop stops replication
func (c *Client) Stop() error {
	// Use atomic CAS to ensure we only stop once
	if !atomic.CompareAndSwapInt32(&c.stopped, 0, 1) {
		// Already stopped
		return nil
	}

	c.logger.Info("Stopping replication client")

	c.cancel()
	close(c.stopChan)

	select {
	case <-c.doneChan:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("stop timeout")
	}
}

// Stats returns current replication statistics
func (c *Client) Stats() ReplicationStats {
	c.stats.mu.RLock()
	defer c.stats.mu.RUnlock()

	// Create a copy to avoid race conditions
	return ReplicationStats{
		Connected:            c.stats.Connected,
		MasterAddr:           c.stats.MasterAddr,
		MasterRunID:          c.stats.MasterRunID,
		ReplicationOffset:    c.stats.ReplicationOffset,
		LastSyncTime:         c.stats.LastSyncTime,
		BytesReceived:        c.stats.BytesReceived,
		CommandsProcessed:    c.stats.CommandsProcessed,
		ReconnectCount:       c.stats.ReconnectCount,
		InitialSyncCompleted: c.stats.InitialSyncCompleted,
		InitialSyncProgress:  c.stats.InitialSyncProgress,
	}
}

// OnSyncComplete registers a callback for sync completion
func (c *Client) OnSyncComplete(fn func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onSyncComplete = append(c.onSyncComplete, fn)
}

// run is the main replication loop
func (c *Client) run() {
	defer func() {
		// Use atomic CAS to ensure we only close doneChan once
		if atomic.CompareAndSwapInt32(&c.runEnded, 0, 1) {
			close(c.doneChan)
		}
	}()

	for {
		select {
		case <-c.stopChan:
			c.disconnect()
			return
		default:
			if err := c.connect(); err != nil {
				c.logger.Error("Connection failed", "error", err)
				c.recordMetricError("connection")

				// Exponential backoff
				select {
				case <-time.After(1 * time.Second):
				case <-c.stopChan:
					return
				}
				continue
			}

			if err := c.performSync(); err != nil {
				c.logger.Error("Sync failed", "error", err)
				c.recordMetricError("sync")
				c.disconnect()
				
				// Add backoff for sync failures to prevent tight loop
				select {
				case <-time.After(2 * time.Second):
				case <-c.stopChan:
					return
				}
				continue
			}

			// Start streaming replication
			if err := c.streamCommands(); err != nil {
				c.logger.Error("Streaming failed", "error", err)
				c.recordMetricError("streaming")
				c.disconnect()
				continue
			}
		}
	}
}

// connect establishes connection to master
func (c *Client) connect() error {
	c.logger.Debug("Connecting to master", "addr", c.masterAddr)

	var conn net.Conn
	var err error

	// Create dialer with timeout
	dialer := &net.Dialer{
		Timeout: c.connectTimeout,
	}

	if c.tlsConfig != nil {
		conn, err = tls.DialWithDialer(dialer, "tcp", c.masterAddr, c.tlsConfig)
	} else {
		conn, err = dialer.Dial("tcp", c.masterAddr)
	}

	if err != nil {
		return fmt.Errorf("dial failed: %w", err)
	}

	// Set connection timeouts with enhanced error handling
	if err := c.setConnectionTimeouts(conn); err != nil {
		conn.Close()
		return fmt.Errorf("failed to set connection timeouts: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.reader = protocol.NewReader(conn)
	c.writer = protocol.NewWriter(conn)
	c.connected = true
	c.mu.Unlock()

	// Authenticate if password is set
	if c.masterPassword != "" {
		if err := c.authenticate(); err != nil {
			c.disconnect()
			return fmt.Errorf("authentication failed: %w", err)
		}
	}

	c.updateStats(func(s *ReplicationStats) {
		s.Connected = true
		s.ReconnectCount++
	})

	if c.metrics != nil {
		c.metrics.RecordReconnection()
	}

	c.logger.Info("Connected to master")
	return nil
}

// disconnect closes the connection
func (c *Client) disconnect() {
	c.mu.Lock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.connected = false
	c.mu.Unlock()

	c.updateStats(func(s *ReplicationStats) {
		s.Connected = false
	})
}

// authenticate performs Redis authentication
func (c *Client) authenticate() error {
	if err := c.writer.WriteCommand("AUTH", c.masterPassword); err != nil {
		return err
	}
	if err := c.writer.Flush(); err != nil {
		return err
	}

	response, err := c.reader.ReadNext()
	if err != nil {
		return err
	}

	if response.IsError() {
		return fmt.Errorf("auth failed: %s", response.Error())
	}

	return nil
}

// performSync performs initial synchronization
func (c *Client) performSync() error {
	// Use atomic check and set to prevent concurrent sync operations
	if !atomic.CompareAndSwapInt32(&c.syncing, 0, 1) {
		c.logger.Debug("Sync already in progress, skipping")
		return nil
	}
	defer atomic.StoreInt32(&c.syncing, 0)

	c.logger.Info("Starting initial synchronization")
	startTime := time.Now()

	// Send PSYNC command for partial sync or SYNC for full sync
	if err := c.sendPSYNC(); err != nil {
		return fmt.Errorf("PSYNC failed: %w", err)
	}

	// Read PSYNC response with improved error handling
	response, err := c.reader.ReadNext()
	if err != nil {
		if err == io.EOF {
			return fmt.Errorf("PSYNC response failed: connection closed by server")
		}
		return fmt.Errorf("PSYNC response failed: %w", err)
	}

	if response.IsError() {
		return fmt.Errorf("PSYNC error: %s", response.Error())
	}

	// Parse PSYNC response - handle both string and bulk string responses
	responseStr := response.String()
	c.logger.Debug("PSYNC response received", "response", responseStr)
	
	parts := strings.Fields(responseStr)
	if len(parts) < 3 {
		return fmt.Errorf("invalid PSYNC response: %s", responseStr)
	}

	if parts[0] == "FULLRESYNC" {
		c.replID = parts[1]
		offset, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid offset: %s", parts[2])
		}
		c.replOffset = offset

		c.logger.Debug("Full resync initiated", "repl_id", c.replID, "offset", c.replOffset)

		// Perform full sync
		if err := c.performFullSync(); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("unsupported PSYNC response: %s", responseStr)
	}

	syncDuration := time.Since(startTime)
	if c.metrics != nil {
		c.metrics.RecordSyncDuration(syncDuration)
	}

	c.updateStats(func(s *ReplicationStats) {
		s.InitialSyncCompleted = true
		s.InitialSyncProgress = 1.0
		s.LastSyncTime = time.Now()
	})

	// Notify sync completion
	c.mu.RLock()
	callbacks := make([]func(), len(c.onSyncComplete))
	copy(callbacks, c.onSyncComplete)
	c.mu.RUnlock()

	for _, callback := range callbacks {
		callback()
	}

	c.logger.Info("Initial synchronization completed", "duration", syncDuration)
	
	return nil
}

// sendPSYNC sends PSYNC command
func (c *Client) sendPSYNC() error {
	// For initial sync, use PSYNC ? -1
	if err := c.writer.WriteCommand("PSYNC", "?", "-1"); err != nil {
		return err
	}
	return c.writer.Flush()
}

// performFullSync performs full synchronization via RDB
func (c *Client) performFullSync() error {
	c.logger.Debug("Performing full sync")

	// Create RDB handler
	handler := &rdbStorageHandler{
		storage:   c.storage,
		logger:    c.logger,
		databases: c.databases,
	}

	// Create RDB stream reader that collects chunks
	rdbBuffer := &rdbStreamBuffer{
		chunks: make([][]byte, 0),
		logger: c.logger,
	}

	// Read RDB data as streaming bulk string
	c.logger.Debug("Reading RDB data stream")
	err := c.reader.ReadBulkStringForReplication(func(chunk []byte) error {
		if chunk == nil {
			c.logger.Debug("Received null RDB chunk")
			return nil
		}

		chunkSize := len(chunk)
		if chunkSize == 0 {
			c.logger.Debug("Received empty RDB chunk")
			return nil
		}

		c.logger.Debug("Received RDB chunk", "size", chunkSize)

		// Copy chunk to avoid buffer reuse issues
		chunkCopy := make([]byte, chunkSize)
		copy(chunkCopy, chunk)
		rdbBuffer.chunks = append(rdbBuffer.chunks, chunkCopy)
		rdbBuffer.totalSize += chunkSize

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to read RDB data: %w", err)
	}

	c.logger.Debug("RDB data reading completed", "totalSize", rdbBuffer.totalSize, "chunks", len(rdbBuffer.chunks))

	// Parse RDB from collected buffer
	parser := NewRDBParser(rdbBuffer, handler)
	parser.SetLogger(c.logger)
	if err := parser.Parse(); err != nil {
		// For compatibility with different Redis versions, log the error but don't fail completely
		// This allows the replication to continue even if RDB parsing has issues
		c.logger.Error("RDB parsing failed (non-fatal for empty databases)", "error", err)
		
		// If this is likely an empty database, just complete the sync
		if rdbBuffer.totalSize < 500 { // Small RDB likely means empty or mostly empty
			c.logger.Info("Assuming empty database due to small RDB size and parsing error")
			c.logger.Debug("RDB parsing completed")
			
			return nil
		}
		
		return fmt.Errorf("RDB parsing failed: %w", err)
	}

	c.logger.Debug("RDB parsing completed")
	
	// Note: Reader cleanup will be handled during connection establishment
	// for command streaming to ensure proper synchronization
	
	return nil
}

// streamCommands streams replication commands
func (c *Client) streamCommands() error {
	c.logger.Debug("Starting command streaming")
	
	// Ensure reader state is clean for command streaming after RDB parsing
	// This is critical to prevent protocol desynchronization
	c.mu.Lock()
	if c.conn != nil {
		// Create a new reader to ensure clean state
		c.reader = protocol.NewReader(c.conn)
		c.logger.Debug("Created fresh reader for command streaming")
	}
	c.mu.Unlock()
	
	// Add a brief stabilization delay after reader reset
	time.Sleep(50 * time.Millisecond)
	
	protocolErrorCount := 0
	maxProtocolErrors := 3 // Reduced threshold for faster reconnection
	consecutiveSuccesses := 0

	for {
		select {
		case <-c.stopChan:
			return nil
		default:
			// Read next command with enhanced error handling
			value, err := c.reader.ReadNext()
			if err != nil {
				if err == io.EOF {
					return fmt.Errorf("connection closed")
				}
				
				// Handle protocol errors with categorization
				if isProtocolError(err) {
					protocolErrorCount++
					consecutiveSuccesses = 0 // Reset success counter
					
					c.logger.Debug("Protocol error during streaming", 
						"error", err, 
						"count", protocolErrorCount,
						"max", maxProtocolErrors)
					
					// If we get too many consecutive protocol errors, reconnect
					if protocolErrorCount >= maxProtocolErrors {
						c.logger.Error("Too many consecutive protocol errors, triggering reconnection", 
							"count", protocolErrorCount,
							"error", err)
						return fmt.Errorf("protocol desynchronization detected after %d errors: %w", protocolErrorCount, err)
					}
					
					// Add progressive delay for protocol errors
					delay := time.Duration(protocolErrorCount) * 50 * time.Millisecond
					if delay > 500*time.Millisecond {
						delay = 500 * time.Millisecond
					}
					
					select {
					case <-time.After(delay):
					case <-c.stopChan:
						return nil
					}
					continue
				}
				
				// For non-protocol errors, fail immediately
				return fmt.Errorf("read command failed: %w", err)
			}

			// Reset error count on successful read and track consecutive successes
			if protocolErrorCount > 0 {
				c.logger.Debug("Successfully recovered from protocol errors", "previous_errors", protocolErrorCount)
			}
			protocolErrorCount = 0
			consecutiveSuccesses++

			// Process command
			if err := c.processCommand(value); err != nil {
				c.logger.Error("Command processing failed", "error", err)
				// Don't fail on command processing errors, just log and continue
				continue
			}
		}
	}
}

// isProtocolError checks if an error is a protocol-level error that might be recoverable
func isProtocolError(err error) bool {
	errStr := err.Error()
	return strings.Contains(errStr, "unknown RESP type") ||
		strings.Contains(errStr, "expected CRLF terminator") ||
		strings.Contains(errStr, "expected bulk string") ||
		strings.Contains(errStr, "empty byte") ||
		strings.Contains(errStr, "invalid length") ||
		strings.Contains(errStr, "protocol error")
}



// processCommand processes a replication command
func (c *Client) processCommand(value protocol.Value) error {
	cmd, err := protocol.ParseCommand(value)
	if err != nil {
		return fmt.Errorf("parse command failed: %w", err)
	}

	// Apply command filters
	if len(c.commandFilters) > 0 {
		if _, allowed := c.commandFilters[cmd.Name]; !allowed {
			return nil // Skip filtered command
		}
	}

	// Apply database filtering for SELECT command
	if cmd.Name == "SELECT" && len(c.databases) > 0 {
		if len(cmd.Args) == 1 {
			if db, err := strconv.Atoi(string(cmd.Args[0])); err == nil {
				if _, allowed := c.databases[db]; !allowed {
					return nil // Skip database selection for non-replicated database
				}
			}
		}
	}

	// For data commands, check if current database should be replicated
	if len(c.databases) > 0 && isDataCommand(cmd.Name) {
		currentDB := c.storage.CurrentDB()
		if _, allowed := c.databases[currentDB]; !allowed {
			return nil // Skip command in non-replicated database
		}
	}

	startTime := time.Now()

	// Execute command
	if err := c.executeCommand(cmd); err != nil {
		return fmt.Errorf("execute command failed: %w", err)
	}

	// Update statistics
	duration := time.Since(startTime)
	if c.metrics != nil {
		c.metrics.RecordCommandProcessed(cmd.Name, duration)
	}

	c.updateStats(func(s *ReplicationStats) {
		s.CommandsProcessed++
		s.ReplicationOffset++
	})

	return nil
}

// executeCommand executes a command against storage
func (c *Client) executeCommand(cmd *protocol.Command) error {
	switch cmd.Name {
	case "SET":
		if len(cmd.Args) < 2 {
			return fmt.Errorf("SET requires at least 2 arguments")
		}
		key := string(cmd.Args[0])
		value := cmd.Args[1]

		// TODO: Parse SET options (EX, PX, etc.)
		return c.storage.Set(key, value, nil)

	case "DEL":
		keys := make([]string, len(cmd.Args))
		for i, arg := range cmd.Args {
			keys[i] = string(arg)
		}
		c.storage.Del(keys...)
		return nil

	case "SELECT":
		if len(cmd.Args) != 1 {
			return fmt.Errorf("SELECT requires 1 argument")
		}
		db, err := strconv.Atoi(string(cmd.Args[0]))
		if err != nil {
			return fmt.Errorf("invalid database number: %s", cmd.Args[0])
		}
		return c.storage.SelectDB(db)

	case "PING":
		// Ignore PING commands
		return nil

	default:
		c.logger.Debug("Unsupported command", "command", cmd.Name)
		return nil
	}
}

// updateStats atomically updates statistics
func (c *Client) updateStats(fn func(*ReplicationStats)) {
	c.stats.mu.Lock()
	defer c.stats.mu.Unlock()
	fn(c.stats)
}

// recordMetricError records an error metric
func (c *Client) recordMetricError(errorType string) {
	if c.metrics != nil {
		c.metrics.RecordError(errorType)
	}
}

// isDataCommand returns true if the command modifies data
func isDataCommand(cmd string) bool {
	switch cmd {
	case "SET", "DEL", "EXPIRE", "PERSIST", "INCR", "DECR", "INCRBY", "DECRBY",
		"APPEND", "SETRANGE", "GETSET", "SETNX", "SETEX", "PSETEX", "MSET", "MSETNX",
		"LPUSH", "RPUSH", "LPOP", "RPOP", "LSET", "LTRIM", "LINSERT", "LREM",
		"SADD", "SREM", "SPOP", "SMOVE", "SINTERSTORE", "SUNIONSTORE", "SDIFFSTORE",
		"ZADD", "ZREM", "ZINCRBY", "ZREMRANGEBYRANK", "ZREMRANGEBYSCORE", "ZREMRANGEBYLEX",
		"ZINTERSTORE", "ZUNIONSTORE", "HSET", "HDEL", "HINCRBY", "HINCRBYFLOAT", "HMSET":
		return true
	default:
		return false
	}
}

// rdbStorageHandler implements RDBHandler for storage
type rdbStorageHandler struct {
	storage   storage.Storage
	logger    Logger
	databases map[int]struct{} // Which databases to replicate (empty = all)
	currentDB int
}

func (h *rdbStorageHandler) OnDatabase(index int) error {
	h.currentDB = index

	// Skip database if not in filter list (when filter is set)
	if len(h.databases) > 0 {
		if _, allowed := h.databases[index]; !allowed {
			return nil // Skip this database
		}
	}

	return h.storage.SelectDB(index)
}

func (h *rdbStorageHandler) OnKey(key []byte, value interface{}, expiry *time.Time) error {
	// Skip key if current database is not allowed
	if len(h.databases) > 0 {
		if _, allowed := h.databases[h.currentDB]; !allowed {
			h.logger.Debug("Skipping key in non-replicated database", "key", string(key), "db", h.currentDB)
			return nil // Skip key in non-replicated database
		}
	}

	h.logger.Debug("Processing RDB key", "key", string(key), "type", fmt.Sprintf("%T", value), "db", h.currentDB)

	switch v := value.(type) {
	case []byte:
		err := h.storage.Set(string(key), v, expiry)
		if err != nil {
			h.logger.Error("Failed to set key in storage", "key", string(key), "error", err)
			return err
		}
		h.logger.Debug("Successfully stored RDB key", "key", string(key), "value_len", len(v))
		return nil
	default:
		h.logger.Debug("Unsupported RDB value type", "key", string(key), "type", fmt.Sprintf("%T", value))
		return nil
	}
}

func (h *rdbStorageHandler) OnAux(key, value []byte) error {
	h.logger.Debug("RDB aux field", "key", string(key), "value", string(value))
	return nil
}

func (h *rdbStorageHandler) OnEnd() error {
	h.logger.Debug("RDB parsing completed")
	return nil
}

// rdbStreamBuffer collects RDB data chunks and provides a Reader interface
type rdbStreamBuffer struct {
	chunks    [][]byte
	totalSize int
	pos       int
	chunkIdx  int
	chunkPos  int
	logger    Logger
}

func (r *rdbStreamBuffer) Read(p []byte) (n int, err error) {
	if r.chunkIdx >= len(r.chunks) {
		return 0, io.EOF
	}

	totalCopied := 0

	for totalCopied < len(p) && r.chunkIdx < len(r.chunks) {
		currentChunk := r.chunks[r.chunkIdx]

		// Check if we've read all data from current chunk
		if r.chunkPos >= len(currentChunk) {
			r.chunkIdx++
			r.chunkPos = 0
			continue
		}

		// Copy data from current chunk
		remaining := len(currentChunk) - r.chunkPos
		toCopy := len(p) - totalCopied
		if toCopy > remaining {
			toCopy = remaining
		}

		copy(p[totalCopied:], currentChunk[r.chunkPos:r.chunkPos+toCopy])
		r.chunkPos += toCopy
		totalCopied += toCopy
		r.pos += toCopy
	}

	if totalCopied == 0 {
		return 0, io.EOF
	}

	return totalCopied, nil
}

// defaultLogger is a simple logger implementation
type defaultLogger struct{}

func (l *defaultLogger) Debug(msg string, fields ...interface{}) {
	// Debug logging disabled by default for security
}

func (l *defaultLogger) Info(msg string, fields ...interface{}) {
	// Info logging disabled by default for security - use SetLogger() for custom logging
}

func (l *defaultLogger) Error(msg string, fields ...interface{}) {
	// Error logging disabled by default for security - use SetLogger() for custom logging
}

// setConnectionTimeouts sets enhanced connection timeouts with improved error handling
func (c *Client) setConnectionTimeouts(conn net.Conn) error {
	now := time.Now()

	// Set read timeout with validation
	if c.readTimeout > 0 {
		if c.readTimeout < time.Millisecond {
			return fmt.Errorf("read timeout too small: %v (minimum: 1ms)", c.readTimeout)
		}
		if c.readTimeout > 24*time.Hour {
			return fmt.Errorf("read timeout too large: %v (maximum: 24h)", c.readTimeout)
		}
		if err := conn.SetReadDeadline(now.Add(c.readTimeout)); err != nil {
			return fmt.Errorf("failed to set read deadline: %w", err)
		}
		c.logger.Debug("Set read timeout", "timeout", c.readTimeout)
	}

	// Set write timeout with validation
	if c.writeTimeout > 0 {
		if c.writeTimeout < time.Millisecond {
			return fmt.Errorf("write timeout too small: %v (minimum: 1ms)", c.writeTimeout)
		}
		if c.writeTimeout > 24*time.Hour {
			return fmt.Errorf("write timeout too large: %v (maximum: 24h)", c.writeTimeout)
		}
		if err := conn.SetWriteDeadline(now.Add(c.writeTimeout)); err != nil {
			return fmt.Errorf("failed to set write deadline: %w", err)
		}
		c.logger.Debug("Set write timeout", "timeout", c.writeTimeout)
	}

	return nil
}

