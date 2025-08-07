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
	"github.com/raniellyferreira/redis-inmemory-replica/storage"
)

// Server provides Redis protocol server functionality
type Server struct {
	storage storage.Storage
	lua     *lua.Engine

	// Server configuration
	addr     string
	password string

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
		s.listener.Close()
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
	c.conn.Close()
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
		c.conn.SetReadDeadline(time.Now().Add(30 * time.Second))

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
	case "SET":
		c.handleSet(cmd)
	case "DEL":
		c.handleDel(cmd)
	case "EXISTS":
		c.handleExists(cmd)
	case "TYPE":
		c.handleType(cmd)
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

	value, exists := c.server.storage.Get(string(cmd.Args[0]))
	if !exists {
		c.writeNull()
	} else {
		c.writeBulkString(value)
	}
}

func (c *Client) handleSet(cmd *protocol.Command) {
	if len(cmd.Args) < 2 {
		c.writeError("ERR wrong number of arguments for 'set' command")
		return
	}

	key := string(cmd.Args[0])
	value := cmd.Args[1]

	// TODO: Handle SET options (EX, PX, NX, XX, etc.)

	err := c.server.storage.Set(key, value, nil)
	if err != nil {
		c.writeError(fmt.Sprintf("ERR %v", err))
		return
	}

	c.writeString("OK")
}

func (c *Client) handleDel(cmd *protocol.Command) {
	if len(cmd.Args) == 0 {
		c.writeError("ERR wrong number of arguments for 'del' command")
		return
	}

	keys := make([]string, len(cmd.Args))
	for i, arg := range cmd.Args {
		keys[i] = string(arg)
	}

	deleted := c.server.storage.Del(keys...)
	c.writeInteger(deleted)
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

// Response writers

func (c *Client) writeString(s string) {
	c.writer.WriteSimpleString(s)
	c.writer.Flush()
}

func (c *Client) writeError(s string) {
	c.server.mu.Lock()
	c.server.errorCount++
	c.server.mu.Unlock()
	// Clean error message by removing internal newlines which can break RESP protocol
	cleanMsg := strings.ReplaceAll(s, "\n", " ")
	cleanMsg = strings.ReplaceAll(cleanMsg, "\r", " ")
	c.writer.WriteError(cleanMsg)
	c.writer.Flush()
}

func (c *Client) writeBulkString(data []byte) {
	c.writer.WriteBulkString(data)
	c.writer.Flush()
}

func (c *Client) writeNull() {
	c.writer.WriteNullBulkString()
	c.writer.Flush()
}

func (c *Client) writeInteger(i int64) {
	c.writer.WriteInteger(i)
	c.writer.Flush()
}

func (c *Client) writeArray(array []interface{}) {
	// Convert interface{} array to Value array
	values := make([]protocol.Value, len(array))
	for i, item := range array {
		values[i] = c.convertToValue(item)
	}
	c.writer.WriteArray(values)
	c.writer.Flush()
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
