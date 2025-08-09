package server

import (
	"bufio"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/raniellyferreira/redis-inmemory-replica/storage"
)

// Simple Redis client for testing
type testClient struct {
	conn   net.Conn
	reader *bufio.Reader
}

func newTestClient(addr string) (*testClient, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	return &testClient{
		conn:   conn,
		reader: bufio.NewReader(conn),
	}, nil
}

func (c *testClient) Close() error {
	return c.conn.Close()
}

func (c *testClient) sendCommand(cmd string, args ...string) (string, error) {
	// Build RESP command
	parts := append([]string{cmd}, args...)
	resp := "*" + strconv.Itoa(len(parts)) + "\r\n"
	for _, part := range parts {
		resp += "$" + strconv.Itoa(len(part)) + "\r\n" + part + "\r\n"
	}

	// Send command
	_, err := c.conn.Write([]byte(resp))
	if err != nil {
		return "", err
	}

	// Read response
	return c.readResponse()
}

func (c *testClient) readResponse() (string, error) {
	line, err := c.reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	line = strings.TrimSpace(line)
	if len(line) == 0 {
		return "", nil
	}

	switch line[0] {
	case '+': // Simple string
		return line[1:], nil
	case '-': // Error
		return line, nil
	case ':': // Integer
		return line[1:], nil
	case '$': // Bulk string
		size, err := strconv.Atoi(line[1:])
		if err != nil {
			return "", err
		}
		if size == -1 {
			return "(nil)", nil
		}
		data := make([]byte, size+2) // +2 for CRLF
		_, err = c.reader.Read(data)
		if err != nil {
			return "", err
		}
		return string(data[:size]), nil
	case '*': // Array
		size, err := strconv.Atoi(line[1:])
		if err != nil {
			return "", err
		}
		if size == -1 {
			return "(nil)", nil
		}

		result := "["
		for i := 0; i < size; i++ {
			if i > 0 {
				result += ", "
			}
			item, err := c.readResponse()
			if err != nil {
				return "", err
			}
			result += item
		}
		result += "]"
		return result, nil
	default:
		return line, nil
	}
}

func TestServer_BasicCommands(t *testing.T) {
	// Create server
	stor := storage.NewMemory()
	server := NewServer(":0", stor) // Use random port

	err := server.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = server.Stop() }()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Connect client
	client, err := newTestClient(server.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	// Test PING
	resp, err := client.sendCommand("PING")
	if err != nil {
		t.Fatal(err)
	}
	if resp != "PONG" {
		t.Errorf("expected PONG, got %s", resp)
	}

	// Test SET/GET
	resp, err = client.sendCommand("SET", "testkey", "testvalue")
	if err != nil {
		t.Fatal(err)
	}
	if resp != "OK" {
		t.Errorf("expected OK, got %s", resp)
	}

	resp, err = client.sendCommand("GET", "testkey")
	if err != nil {
		t.Fatal(err)
	}
	if resp != "testvalue" {
		t.Errorf("expected testvalue, got %s", resp)
	}
}

func TestServer_LuaScripts(t *testing.T) {
	// Create server
	stor := storage.NewMemory()
	server := NewServer(":0", stor) // Use random port

	err := server.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = server.Stop() }()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Connect client
	client, err := newTestClient(server.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	// Test simple EVAL
	resp, err := client.sendCommand("EVAL", "return 'hello world'", "0")
	if err != nil {
		t.Fatal(err)
	}
	if resp != "hello world" {
		t.Errorf("expected 'hello world', got %s", resp)
	}

	// Test EVAL with KEYS and ARGV
	resp, err = client.sendCommand("EVAL", "return KEYS[1] .. ':' .. ARGV[1]", "1", "user", "123")
	if err != nil {
		t.Fatal(err)
	}
	if resp != "user:123" {
		t.Errorf("expected 'user:123', got %s", resp)
	}

	// Test EVAL with Redis commands
	resp, err = client.sendCommand("EVAL", "redis.call('SET', KEYS[1], ARGV[1]); return redis.call('GET', KEYS[1])", "1", "luakey", "luavalue")
	if err != nil {
		t.Fatal(err)
	}
	if resp != "luavalue" {
		t.Errorf("expected 'luavalue', got %s", resp)
	}

	// Test SCRIPT LOAD and EVALSHA
	resp, err = client.sendCommand("SCRIPT", "LOAD", "return 'cached script'")
	if err != nil {
		t.Fatal(err)
	}
	sha := resp

	resp, err = client.sendCommand("EVALSHA", sha, "0")
	if err != nil {
		t.Fatal(err)
	}
	if resp != "cached script" {
		t.Errorf("expected 'cached script', got %s", resp)
	}

	// Test SCRIPT EXISTS
	resp, err = client.sendCommand("SCRIPT", "EXISTS", sha, "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if resp != "[1, 0]" {
		t.Errorf("expected '[1, 0]', got %s", resp)
	}

	// Test SCRIPT FLUSH
	resp, err = client.sendCommand("SCRIPT", "FLUSH")
	if err != nil {
		t.Fatal(err)
	}
	if resp != "OK" {
		t.Errorf("expected 'OK', got %s", resp)
	}

	// Verify script was flushed
	resp, err = client.sendCommand("SCRIPT", "EXISTS", sha)
	if err != nil {
		t.Fatal(err)
	}
	if resp != "[0]" {
		t.Errorf("expected '[0]', got %s", resp)
	}
}

func TestServer_ErrorHandling(t *testing.T) {
	// Create server
	stor := storage.NewMemory()
	server := NewServer(":0", stor)

	err := server.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = server.Stop() }()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Connect client
	client, err := newTestClient(server.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	// Test unknown command
	resp, err := client.sendCommand("UNKNOWNCMD")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(resp, "-ERR unknown command") {
		t.Errorf("expected error for unknown command, got %s", resp)
	}

	// Test Lua syntax error
	resp, err = client.sendCommand("EVAL", "invalid lua syntax !!!", "0")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(resp, "-ERR") {
		t.Errorf("expected error for invalid Lua syntax, got %s", resp)
	}

	// Test EVALSHA with non-existent script
	resp, err = client.sendCommand("EVALSHA", "nonexistent", "0")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(resp, "-ERR") {
		t.Errorf("expected error for non-existent script, got %s", resp)
	}
}

func TestServer_Stats(t *testing.T) {
	// Create server
	stor := storage.NewMemory()
	server := NewServer(":0", stor)

	err := server.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = server.Stop() }()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Connect client
	client, err := newTestClient(server.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	// Send some commands
	_, _ = client.sendCommand("PING")
	_, _ = client.sendCommand("SET", "key", "value")
	_, _ = client.sendCommand("GET", "key")

	// Check stats
	stats := server.Stats()

	if stats["connected_clients"].(int) != 1 {
		t.Errorf("expected 1 connected client, got %v", stats["connected_clients"])
	}

	if stats["total_commands"].(int64) < 3 {
		t.Errorf("expected at least 3 commands, got %v", stats["total_commands"])
	}

	if stats["total_connections"].(int64) < 1 {
		t.Errorf("expected at least 1 connection, got %v", stats["total_connections"])
	}
}
