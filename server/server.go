package server

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/raniellyferreira/redis-inmemory-replica/lua"
	"github.com/raniellyferreira/redis-inmemory-replica/protocol"
	"github.com/raniellyferreira/redis-inmemory-replica/replication"
	"github.com/raniellyferreira/redis-inmemory-replica/storage"
)

// Server provides Redis protocol server functionality
type Server struct {
	storage storage.Storage
	lua     *lua.Engine
	syncMgr *replication.SyncManager

	// Server configuration
	addr     string
	password string

	// Write redirection settings
	redirectWrites bool
	masterAddr     string
	masterPassword string

	// Connection management
	listener net.Listener
	clients  sync.Map // map[net.Conn]*Client

	// Control
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Metrics
	connCount    int64
	commandCount int64
	errorCount   int64
	mu           sync.RWMutex
}

// Client represents a connected Redis client
type Client struct {
	conn   net.Conn
	reader *protocol.Reader
	writer *protocol.Writer
	server *Server

	// Client state
	authenticated bool
	db            int
	lastCmd       time.Time

	// Control
	ctx    context.Context
	cancel context.CancelFunc
}

// NewServer creates a new Redis protocol server
func NewServer(addr string, storage storage.Storage) *Server {
	ctx, cancel := context.WithCancel(context.Background())

	return &Server{
		storage: storage,
		lua:     lua.NewEngine(storage),
		addr:    addr,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// SetPassword sets the authentication password for the server
func (s *Server) SetPassword(password string) {
	s.password = password
}

// SetSyncManager sets the sync manager for tracking sync status
func (s *Server) SetSyncManager(syncMgr *replication.SyncManager) {
	s.syncMgr = syncMgr
}

// SetWriteRedirection configures write command redirection to master
func (s *Server) SetWriteRedirection(enabled bool, masterAddr, masterPassword string) {
	s.redirectWrites = enabled
	s.masterAddr = masterAddr
	s.masterPassword = masterPassword
}

// Start starts the Redis server
func (s *Server) Start() error {
	var err error
	s.listener, err = net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.addr, err)
	}

	s.wg.Add(1)
	go s.acceptConnections()

	return nil
}

// Stop stops the Redis server
func (s *Server) Stop() error {
	s.cancel()

	if s.listener != nil {
		_ = s.listener.Close()
	}

	// Close all client connections
	s.clients.Range(func(key, value interface{}) bool {
		if client, ok := value.(*Client); ok {
			client.Close()
		}
		return true
	})

	s.wg.Wait()
	return nil
}

// Addr returns the server's listening address
func (s *Server) Addr() string {
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.addr
}

// Stats returns server statistics
func (s *Server) Stats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	clientCount := 0
	s.clients.Range(func(key, value interface{}) bool {
		clientCount++
		return true
	})

	return map[string]interface{}{
		"connected_clients": clientCount,
		"total_commands":    s.commandCount,
		"total_errors":      s.errorCount,
		"total_connections": s.connCount,
	}
}

// acceptConnections accepts new client connections
func (s *Server) acceptConnections() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		conn, err := s.listener.Accept()
		if err != nil {
			if s.ctx.Err() != nil {
				return // Server is shutting down
			}
			continue
		}

		s.handleNewClient(conn)
	}
}

// handleNewClient handles a new client connection
func (s *Server) handleNewClient(conn net.Conn) {
	s.mu.Lock()
	s.connCount++
	s.mu.Unlock()

	ctx, cancel := context.WithCancel(s.ctx)
	client := &Client{
		conn:          conn,
		reader:        protocol.NewReader(conn),
		writer:        protocol.NewWriter(conn),
		server:        s,
		authenticated: s.password == "", // Auto-authenticated if no password
		lastCmd:       time.Now(),
		ctx:           ctx,
		cancel:        cancel,
	}

	s.clients.Store(conn, client)

	s.wg.Add(1)
	go client.handle()
}

// Close closes the client connection
func (c *Client) Close() {
	c.cancel()
	_ = c.conn.Close()
	c.server.clients.Delete(c.conn)
}

// handle handles client requests
func (c *Client) handle() {
	defer c.server.wg.Done()
	defer c.Close()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		// Set read deadline
		_ = c.conn.SetReadDeadline(time.Now().Add(30 * time.Second))

		value, err := c.reader.ReadNext()
		if err != nil {
			if err == io.EOF {
				return // Client disconnected
			}
			if c.ctx.Err() != nil {
				return // Server shutting down
			}
			c.writeError(fmt.Sprintf("Protocol error: %v", err))
			return
		}

		cmd, err := protocol.ParseCommand(value)
		if err != nil {
			c.writeError(fmt.Sprintf("Protocol error: %v", err))
			continue
		}

		c.lastCmd = time.Now()
		c.executeCommand(cmd)
	}
}

// executeCommand executes a Redis command
func (c *Client) executeCommand(cmd *protocol.Command) {
	c.server.mu.Lock()
	c.server.commandCount++
	c.server.mu.Unlock()

	// Check authentication first
	if !c.authenticated && strings.ToUpper(cmd.Name) != "AUTH" {
		c.writeError("NOAUTH Authentication required")
		return
	}

	switch strings.ToUpper(cmd.Name) {
	case "AUTH":
		c.handleAuth(cmd)
	case "PING":
		c.handlePing(cmd)
	case "SELECT":
		c.handleSelect(cmd)
	case "GET":
		c.handleGet(cmd)
	case "MGET":
		c.handleMGet(cmd)
	case "SET":
		c.handleSet(cmd)
	case "DEL":
		c.handleDel(cmd)
	case "EXISTS":
		c.handleExists(cmd)
	case "TTL":
		c.handleTTL(cmd)
	case "PTTL":
		c.handlePTTL(cmd)
	case "TYPE":
		c.handleType(cmd)
	case "KEYS":
		c.handleKeys(cmd)
	case "SCAN":
		c.handleScan(cmd)
	case "DBSIZE":
		c.handleDBSize(cmd)
	case "INFO":
		c.handleInfo(cmd)
	case "ROLE":
		c.handleRole(cmd)
	case "COMMAND":
		c.handleCommand(cmd)
	case "CONFIG":
		c.handleConfig(cmd)
	case "READONLY":
		c.handleReadOnly(cmd)
	case "EVAL":
		c.handleEval(cmd)
	case "EVALSHA":
		c.handleEvalSHA(cmd)
	case "SCRIPT":
		c.handleScript(cmd)
	case "QUIT":
		c.writeString("OK")
		c.Close()
	default:
		c.writeError(fmt.Sprintf("ERR unknown command '%s'", cmd.Name))
	}
}

// Command handlers

func (c *Client) handleAuth(cmd *protocol.Command) {
	if len(cmd.Args) != 1 {
		c.writeError("ERR wrong number of arguments for 'auth' command")
		return
	}

	if c.server.password == "" {
		c.writeError("ERR Client sent AUTH, but no password is set")
		return
	}

	if string(cmd.Args[0]) == c.server.password {
		c.authenticated = true
		c.writeString("OK")
	} else {
		c.writeError("ERR invalid password")
	}
}

func (c *Client) handlePing(cmd *protocol.Command) {
	if len(cmd.Args) == 0 {
		c.writeString("PONG")
	} else if len(cmd.Args) == 1 {
		c.writeBulkString(cmd.Args[0])
	} else {
		c.writeError("ERR wrong number of arguments for 'ping' command")
	}
}

func (c *Client) handleSelect(cmd *protocol.Command) {
	if len(cmd.Args) != 1 {
		c.writeError("ERR wrong number of arguments for 'select' command")
		return
	}

	db, err := strconv.Atoi(string(cmd.Args[0]))
	if err != nil {
		c.writeError("ERR invalid DB index")
		return
	}

	if err := c.server.storage.SelectDB(db); err != nil {
		c.writeError(fmt.Sprintf("ERR %v", err))
		return
	}

	c.db = db
	c.writeString("OK")
}

func (c *Client) handleGet(cmd *protocol.Command) {
	if len(cmd.Args) != 1 {
		c.writeError("ERR wrong number of arguments for 'get' command")
		return
	}

	// Check if initial sync is completed
	if c.server.syncMgr != nil && !c.server.syncMgr.IsInitialSyncCompleted() {
		c.writeError("LOADING Redis is loading the dataset in memory")
		return
	}

	value, exists := c.server.storage.Get(string(cmd.Args[0]))
	if !exists {
		c.writeNull()
	} else {
		c.writeBulkString(value)
	}
}

func (c *Client) handleSet(cmd *protocol.Command) {
	if c.server.redirectWrites {
		c.redirectToMaster(cmd)
	} else {
		c.writeError("READONLY You can't write against a read only replica")
	}
}

func (c *Client) handleDel(cmd *protocol.Command) {
	if c.server.redirectWrites {
		c.redirectToMaster(cmd)
	} else {
		c.writeError("READONLY You can't write against a read only replica")
	}
}

func (c *Client) handleExists(cmd *protocol.Command) {
	if len(cmd.Args) == 0 {
		c.writeError("ERR wrong number of arguments for 'exists' command")
		return
	}

	keys := make([]string, len(cmd.Args))
	for i, arg := range cmd.Args {
		keys[i] = string(arg)
	}

	count := c.server.storage.Exists(keys...)
	c.writeInteger(count)
}

func (c *Client) handleType(cmd *protocol.Command) {
	if len(cmd.Args) != 1 {
		c.writeError("ERR wrong number of arguments for 'type' command")
		return
	}

	valueType := c.server.storage.Type(string(cmd.Args[0]))
	c.writeString(valueType.String())
}

func (c *Client) handleEval(cmd *protocol.Command) {
	if len(cmd.Args) < 2 {
		c.writeError("ERR wrong number of arguments for 'eval' command")
		return
	}

	script := string(cmd.Args[0])
	numKeys, err := strconv.Atoi(string(cmd.Args[1]))
	if err != nil {
		c.writeError("ERR value is not an integer or out of range")
		return
	}

	if numKeys < 0 || len(cmd.Args) < 2+numKeys {
		c.writeError("ERR Number of keys can't be negative or greater than args")
		return
	}

	keys := make([]string, numKeys)
	for i := 0; i < numKeys; i++ {
		keys[i] = string(cmd.Args[2+i])
	}

	args := make([]string, len(cmd.Args)-2-numKeys)
	for i := 0; i < len(args); i++ {
		args[i] = string(cmd.Args[2+numKeys+i])
	}

	result, err := c.server.lua.Eval(script, keys, args)
	if err != nil {
		c.writeError(fmt.Sprintf("ERR %v", err))
		return
	}

	c.writeResult(result)
}

func (c *Client) handleEvalSHA(cmd *protocol.Command) {
	if len(cmd.Args) < 2 {
		c.writeError("ERR wrong number of arguments for 'evalsha' command")
		return
	}

	sha := string(cmd.Args[0])
	numKeys, err := strconv.Atoi(string(cmd.Args[1]))
	if err != nil {
		c.writeError("ERR value is not an integer or out of range")
		return
	}

	if numKeys < 0 || len(cmd.Args) < 2+numKeys {
		c.writeError("ERR Number of keys can't be negative or greater than args")
		return
	}

	keys := make([]string, numKeys)
	for i := 0; i < numKeys; i++ {
		keys[i] = string(cmd.Args[2+i])
	}

	args := make([]string, len(cmd.Args)-2-numKeys)
	for i := 0; i < len(args); i++ {
		args[i] = string(cmd.Args[2+numKeys+i])
	}

	result, err := c.server.lua.EvalSHA(sha, keys, args)
	if err != nil {
		c.writeError(fmt.Sprintf("ERR %v", err))
		return
	}

	c.writeResult(result)
}

func (c *Client) handleScript(cmd *protocol.Command) {
	if len(cmd.Args) == 0 {
		c.writeError("ERR wrong number of arguments for 'script' command")
		return
	}

	subCmd := strings.ToUpper(string(cmd.Args[0]))

	switch subCmd {
	case "LOAD":
		if len(cmd.Args) != 2 {
			c.writeError("ERR wrong number of arguments for 'script load' command")
			return
		}
		sha := c.server.lua.LoadScript(string(cmd.Args[1]))
		c.writeString(sha)

	case "EXISTS":
		if len(cmd.Args) < 2 {
			c.writeError("ERR wrong number of arguments for 'script exists' command")
			return
		}
		hashes := make([]string, len(cmd.Args)-1)
		for i := 1; i < len(cmd.Args); i++ {
			hashes[i-1] = string(cmd.Args[i])
		}
		results := c.server.lua.ScriptExists(hashes)

		array := make([]interface{}, len(results))
		for i, exists := range results {
			if exists {
				array[i] = int64(1)
			} else {
				array[i] = int64(0)
			}
		}
		c.writeArray(array)

	case "FLUSH":
		if len(cmd.Args) != 1 {
			c.writeError("ERR wrong number of arguments for 'script flush' command")
			return
		}
		c.server.lua.ScriptFlush()
		c.writeString("OK")

	default:
		c.writeError(fmt.Sprintf("ERR unknown SCRIPT subcommand '%s'", subCmd))
	}
}

func (c *Client) handleMGet(cmd *protocol.Command) {
	if len(cmd.Args) == 0 {
		c.writeError("ERR wrong number of arguments for 'mget' command")
		return
	}

	// Check if initial sync is completed
	if c.server.syncMgr != nil && !c.server.syncMgr.IsInitialSyncCompleted() {
		c.writeError("LOADING Redis is loading the dataset in memory")
		return
	}

	values := make([]interface{}, len(cmd.Args))
	for i, arg := range cmd.Args {
		value, exists := c.server.storage.Get(string(arg))
		if exists {
			values[i] = string(value)
		} else {
			values[i] = nil
		}
	}
	c.writeArray(values)
}

func (c *Client) handleTTL(cmd *protocol.Command) {
	if len(cmd.Args) != 1 {
		c.writeError("ERR wrong number of arguments for 'ttl' command")
		return
	}

	ttl := c.server.storage.TTL(string(cmd.Args[0]))
	// Convert to seconds as Redis expects
	seconds := int64(ttl.Seconds())
	c.writeInteger(seconds)
}

func (c *Client) handlePTTL(cmd *protocol.Command) {
	if len(cmd.Args) != 1 {
		c.writeError("ERR wrong number of arguments for 'pttl' command")
		return
	}

	ttl := c.server.storage.PTTL(string(cmd.Args[0]))
	// Convert to milliseconds as Redis expects
	milliseconds := int64(ttl / time.Millisecond)
	c.writeInteger(milliseconds)
}

func (c *Client) handleKeys(cmd *protocol.Command) {
	if len(cmd.Args) != 1 {
		c.writeError("ERR wrong number of arguments for 'keys' command")
		return
	}

	pattern := string(cmd.Args[0])
	matchingKeys := c.server.storage.Keys(pattern)

	// Convert to interface{} slice for writeArray
	result := make([]interface{}, len(matchingKeys))
	for i, key := range matchingKeys {
		result[i] = key
	}

	c.writeArray(result)
}

func (c *Client) handleScan(cmd *protocol.Command) {
	if len(cmd.Args) < 1 {
		c.writeError("ERR wrong number of arguments for 'scan' command")
		return
	}

	cursor, err := strconv.ParseInt(string(cmd.Args[0]), 10, 64)
	if err != nil {
		c.writeError("ERR invalid cursor")
		return
	}

	var pattern string
	var count int64 = 10 // default count

	// Parse optional arguments
	for i := 1; i < len(cmd.Args); i++ {
		arg := strings.ToUpper(string(cmd.Args[i]))
		switch arg {
		case "MATCH":
			if i+1 >= len(cmd.Args) {
				c.writeError("ERR syntax error")
				return
			}
			pattern = string(cmd.Args[i+1])
			i++
		case "COUNT":
			if i+1 >= len(cmd.Args) {
				c.writeError("ERR syntax error")
				return
			}
			count, err = strconv.ParseInt(string(cmd.Args[i+1]), 10, 64)
			if err != nil || count <= 0 {
				c.writeError("ERR invalid count")
				return
			}
			i++
		}
	}

	newCursor, keys := c.server.storage.Scan(cursor, pattern, count)

	// Convert keys to interface slice
	keyInterfaces := make([]interface{}, len(keys))
	for i, key := range keys {
		keyInterfaces[i] = key
	}

	result := []interface{}{
		strconv.FormatInt(newCursor, 10),
		keyInterfaces,
	}
	c.writeArray(result)
}

func (c *Client) handleDBSize(cmd *protocol.Command) {
	if len(cmd.Args) != 0 {
		c.writeError("ERR wrong number of arguments for 'dbsize' command")
		return
	}

	size := c.server.storage.KeyCount()
	c.writeInteger(size)
}

func (c *Client) handleInfo(cmd *protocol.Command) {
	sections := []string{"all"}
	if len(cmd.Args) > 0 {
		sections = make([]string, len(cmd.Args))
		for i, arg := range cmd.Args {
			sections[i] = strings.ToLower(string(arg))
		}
	}

	var info strings.Builder

	for _, section := range sections {
		switch section {
		case "all", "server":
			info.WriteString("# Server\r\n")
			info.WriteString("redis_version:7.0.0\r\n")
			info.WriteString("redis_mode:replica\r\n")
			info.WriteString("arch_bits:64\r\n")
			info.WriteString("os:Linux\r\n")
			info.WriteString("\r\n")
		}

		if section == "all" || section == "replication" {
			info.WriteString("# Replication\r\n")
			info.WriteString("role:slave\r\n")

			if c.server.syncMgr != nil {
				status := c.server.syncMgr.SyncStatus()
				info.WriteString(fmt.Sprintf("master_host:%s\r\n", status.MasterHost))

				if status.Connected {
					info.WriteString("master_link_status:up\r\n")
				} else {
					info.WriteString("master_link_status:down\r\n")
				}

				// Calculate seconds since last sync
				lastIO := int64(time.Since(status.LastSyncTime).Seconds())
				info.WriteString(fmt.Sprintf("master_last_io_seconds_ago:%d\r\n", lastIO))

				if status.InitialSyncCompleted {
					info.WriteString("master_sync_in_progress:0\r\n")
				} else {
					info.WriteString("master_sync_in_progress:1\r\n")
				}

				info.WriteString(fmt.Sprintf("master_repl_offset:%d\r\n", status.ReplicationOffset))
			} else {
				info.WriteString("master_host:unknown\r\n")
				info.WriteString("master_link_status:down\r\n")
				info.WriteString("master_last_io_seconds_ago:-1\r\n")
				info.WriteString("master_sync_in_progress:0\r\n")
				info.WriteString("master_repl_offset:0\r\n")
			}
			info.WriteString("\r\n")
		}

		if section == "all" || section == "memory" {
			info.WriteString("# Memory\r\n")
			memoryUsage := c.server.storage.MemoryUsage()
			info.WriteString(fmt.Sprintf("used_memory:%d\r\n", memoryUsage))
			info.WriteString(fmt.Sprintf("used_memory_human:%s\r\n", formatBytes(memoryUsage)))
			info.WriteString("\r\n")
		}

		if section == "all" || section == "keyspace" {
			// Get database information for keyspace section
			dbInfo := c.server.storage.DatabaseInfo()
			if len(dbInfo) > 0 {
				info.WriteString("# Keyspace\r\n")
				for dbNum, dbStats := range dbInfo {
					// Safe type assertions with fallback values
					keys, ok := dbStats["keys"].(int64)
					if !ok {
						keys = 0
					}
					expires, ok := dbStats["expires"].(int64)
					if !ok {
						expires = 0
					}
					info.WriteString(fmt.Sprintf("db%d:keys=%d,expires=%d\r\n", dbNum, keys, expires))
				}
				info.WriteString("\r\n")
			}
		}
	}

	c.writeBulkString([]byte(strings.TrimSpace(info.String())))
}

func (c *Client) handleRole(cmd *protocol.Command) {
	if len(cmd.Args) != 0 {
		c.writeError("ERR wrong number of arguments for 'role' command")
		return
	}

	role := []interface{}{"slave"}

	if c.server.syncMgr != nil {
		status := c.server.syncMgr.SyncStatus()
		// Parse master host and port
		parts := strings.Split(status.MasterHost, ":")
		if len(parts) == 2 {
			role = append(role, parts[0]) // host
			if port, err := strconv.Atoi(parts[1]); err == nil {
				role = append(role, int64(port)) // port
			} else {
				role = append(role, int64(6379)) // default port
			}
		} else {
			role = append(role, status.MasterHost, int64(6379))
		}
		role = append(role, status.ReplicationOffset)
	} else {
		role = append(role, "unknown", int64(6379), int64(0))
	}

	c.writeArray(role)
}

func (c *Client) handleCommand(cmd *protocol.Command) {
	if len(cmd.Args) == 0 {
		// Return number of commands supported
		c.writeInteger(20) // Approximate count of supported commands
		return
	}

	subCmd := strings.ToUpper(string(cmd.Args[0]))
	switch subCmd {
	case "DOCS":
		// Simple stub - return empty array
		c.writeArray([]interface{}{})
	default:
		c.writeError("ERR unknown COMMAND subcommand")
	}
}

func (c *Client) handleReadOnly(cmd *protocol.Command) {
	if len(cmd.Args) != 0 {
		c.writeError("ERR wrong number of arguments for 'readonly' command")
		return
	}
	c.writeString("OK")
}

func (c *Client) handleConfig(cmd *protocol.Command) {
	if len(cmd.Args) < 1 {
		c.writeError("ERR wrong number of arguments for 'config' command")
		return
	}

	subCmd := strings.ToUpper(string(cmd.Args[0]))
	switch subCmd {
	case "GET":
		c.handleConfigGet(cmd)
	default:
		c.writeError("ERR Unknown CONFIG subcommand or wrong number of arguments")
	}
}

func (c *Client) handleConfigGet(cmd *protocol.Command) {
	if len(cmd.Args) != 2 {
		c.writeError("ERR wrong number of arguments for 'config get' command")
		return
	}

	parameter := strings.ToLower(string(cmd.Args[1]))
	switch parameter {
	case "databases":
		// Redis typically supports 16 databases (0-15)
		result := []interface{}{
			"databases",
			"16",
		}
		c.writeArray(result)
	case "*":
		// Return minimal config for wildcard
		result := []interface{}{
			"databases", "16",
		}
		c.writeArray(result)
	default:
		// Return empty array for unknown parameters (Redis behavior)
		c.writeArray([]interface{}{})
	}
}

// Helper function to format bytes in human readable format
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%dB", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// Response writers

func (c *Client) writeString(s string) {
	_ = c.writer.WriteSimpleString(s)
	_ = c.writer.Flush()
}

func (c *Client) writeError(s string) {
	c.server.mu.Lock()
	c.server.errorCount++
	c.server.mu.Unlock()
	// Clean error message by removing internal newlines which can break RESP protocol
	cleanMsg := strings.ReplaceAll(s, "\n", " ")
	cleanMsg = strings.ReplaceAll(cleanMsg, "\r", " ")
	_ = c.writer.WriteError(cleanMsg)
	_ = c.writer.Flush()
}

func (c *Client) writeBulkString(data []byte) {
	_ = c.writer.WriteBulkString(data)
	_ = c.writer.Flush()
}

func (c *Client) writeNull() {
	_ = c.writer.WriteNullBulkString()
	_ = c.writer.Flush()
}

func (c *Client) writeInteger(i int64) {
	_ = c.writer.WriteInteger(i)
	_ = c.writer.Flush()
}

func (c *Client) writeArray(array []interface{}) {
	// Convert interface{} array to Value array
	values := make([]protocol.Value, len(array))
	for i, item := range array {
		values[i] = c.convertToValue(item)
	}
	_ = c.writer.WriteArray(values)
	_ = c.writer.Flush()
}

func (c *Client) convertToValue(item interface{}) protocol.Value {
	switch v := item.(type) {
	case nil:
		return protocol.Value{Type: protocol.TypeBulkString, IsNull: true}
	case string:
		return protocol.Value{Type: protocol.TypeBulkString, Data: []byte(v)}
	case int64:
		return protocol.Value{Type: protocol.TypeInteger, Integer: v}
	case int:
		return protocol.Value{Type: protocol.TypeInteger, Integer: int64(v)}
	case bool:
		if v {
			return protocol.Value{Type: protocol.TypeInteger, Integer: 1}
		}
		return protocol.Value{Type: protocol.TypeBulkString, IsNull: true}
	case []byte:
		return protocol.Value{Type: protocol.TypeBulkString, Data: v}
	case []interface{}:
		// Convert []interface{} to array protocol value
		array := make([]protocol.Value, len(v))
		for i, item := range v {
			array[i] = c.convertToValue(item)
		}
		return protocol.Value{Type: protocol.TypeArray, Array: array}
	case []string:
		// Convert []string to array protocol value
		array := make([]protocol.Value, len(v))
		for i, s := range v {
			array[i] = protocol.Value{Type: protocol.TypeBulkString, Data: []byte(s)}
		}
		return protocol.Value{Type: protocol.TypeArray, Array: array}
	default:
		return protocol.Value{Type: protocol.TypeBulkString, Data: []byte(fmt.Sprintf("%v", v))}
	}
}

func (c *Client) writeResult(result interface{}) {
	switch v := result.(type) {
	case nil:
		c.writeNull()
	case bool:
		if v {
			c.writeInteger(1)
		} else {
			c.writeNull()
		}
	case string:
		c.writeBulkString([]byte(v))
	case int64:
		c.writeInteger(v)
	case float64:
		// Convert float to string for Redis compatibility
		c.writeBulkString([]byte(fmt.Sprintf("%.17g", v)))
	case []interface{}:
		c.writeArray(v)
	case []string:
		// Convert []string to []interface{} properly
		array := make([]interface{}, len(v))
		for i, s := range v {
			array[i] = s
		}
		c.writeArray(array)
	case map[string]interface{}:
		// Convert map to array of key-value pairs
		array := make([]interface{}, 0, len(v)*2)
		for key, value := range v {
			array = append(array, key, value)
		}
		c.writeArray(array)
	default:
		c.writeBulkString([]byte(fmt.Sprintf("%v", v)))
	}
}

// redirectToMaster forwards a command to the master Redis server and returns the response
func (c *Client) redirectToMaster(cmd *protocol.Command) {
	if c.server.masterAddr == "" {
		c.writeError("ERR master address not configured for write redirection")
		return
	}

	// Connect to master
	conn, err := net.DialTimeout("tcp", c.server.masterAddr, 5*time.Second)
	if err != nil {
		c.writeError(fmt.Sprintf("ERR failed to connect to master: %v", err))
		return
	}
	defer func() {
		_ = conn.Close() // Intentionally ignore error in defer
	}()

	// Set connection timeouts
	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		c.writeError(fmt.Sprintf("ERR failed to set connection deadline: %v", err))
		return
	}

	masterWriter := protocol.NewWriter(conn)
	masterReader := protocol.NewReader(conn)

	// Authenticate with master if password is set
	if c.server.masterPassword != "" {
		authCmd := &protocol.Command{
			Name: "AUTH",
			Args: [][]byte{[]byte(c.server.masterPassword)},
		}
		if err := c.writeCommandToMaster(masterWriter, authCmd); err != nil {
			c.writeError(fmt.Sprintf("ERR failed to send auth to master: %v", err))
			return
		}

		// Read auth response
		resp, err := masterReader.ReadNext()
		if err != nil {
			c.writeError(fmt.Sprintf("ERR failed to read auth response: %v", err))
			return
		}
		if resp.Type == protocol.TypeError {
			c.writeError(string(resp.Data))
			return
		}
	}

	// Send the original command to master
	if err := c.writeCommandToMaster(masterWriter, cmd); err != nil {
		c.writeError(fmt.Sprintf("ERR failed to send command to master: %v", err))
		return
	}

	// Read response from master and forward to client
	resp, err := masterReader.ReadNext()
	if err != nil {
		c.writeError(fmt.Sprintf("ERR failed to read response from master: %v", err))
		return
	}

	// Forward the response to the client
	c.forwardResponse(resp)
}

// writeCommandToMaster sends a command to the master server
func (c *Client) writeCommandToMaster(writer *protocol.Writer, cmd *protocol.Command) error {
	// Build command array
	cmdArray := make([]protocol.Value, 1+len(cmd.Args))
	cmdArray[0] = protocol.Value{Type: protocol.TypeBulkString, Data: []byte(cmd.Name)}
	for i, arg := range cmd.Args {
		cmdArray[i+1] = protocol.Value{Type: protocol.TypeBulkString, Data: arg}
	}

	if err := writer.WriteArray(cmdArray); err != nil {
		return err
	}
	return writer.Flush()
}

// forwardResponse forwards a response from master to the client
func (c *Client) forwardResponse(resp protocol.Value) {
	switch resp.Type {
	case protocol.TypeSimpleString:
		c.writeString(string(resp.Data))
	case protocol.TypeError:
		c.writeError(string(resp.Data))
	case protocol.TypeInteger:
		c.writeInteger(resp.Integer)
	case protocol.TypeBulkString:
		if resp.IsNull {
			c.writeNull()
		} else {
			c.writeBulkString(resp.Data)
		}
	case protocol.TypeArray:
		if resp.IsNull {
			c.writeNull()
		} else {
			// Convert protocol values to interface array for writeArray
			array := make([]interface{}, len(resp.Array))
			for i, val := range resp.Array {
				array[i] = c.protocolValueToInterface(val)
			}
			c.writeArray(array)
		}
	default:
		c.writeError("ERR unsupported response type from master")
	}
}

// protocolValueToInterface converts a protocol.Value to interface{} for writeArray
func (c *Client) protocolValueToInterface(val protocol.Value) interface{} {
	switch val.Type {
	case protocol.TypeSimpleString:
		return string(val.Data)
	case protocol.TypeError:
		return fmt.Sprintf("ERR %s", string(val.Data))
	case protocol.TypeInteger:
		return val.Integer
	case protocol.TypeBulkString:
		if val.IsNull {
			return nil
		}
		return val.Data
	case protocol.TypeArray:
		if val.IsNull {
			return nil
		}
		array := make([]interface{}, len(val.Array))
		for i, item := range val.Array {
			array[i] = c.protocolValueToInterface(item)
		}
		return array
	default:
		return string(val.Data)
	}
}
