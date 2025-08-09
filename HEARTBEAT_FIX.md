# Redis Replication Heartbeat Timeout Fix

## Problem Description

Redis replica connections were experiencing frequent timeouts with errors like:
```
Heartbeat ACK failed error=failed to flush REPLCONF ACK: write ... i/o timeout
```

This was causing connection drops and repeated partial resync cycles.

## Root Cause

The issue was in the connection timeout management:

1. **Write deadline set once**: During connection establishment, a write deadline was set to `now + writeTimeout` (10s default)
2. **Write deadline never refreshed**: During streaming phase, read deadline was removed but write deadline remained expired
3. **Heartbeat failure**: When heartbeat sent REPLCONF ACK every 30s, the write deadline had expired 20s ago

## Solution Summary

### 1. Write Deadline Renewal
**Before:**
```go
// Set once during connection, never refreshed
conn.SetWriteDeadline(now.Add(c.writeTimeout))

// Later in heartbeat (after deadline expired):
writer.WriteCommand("REPLCONF", "ACK", offset) // TIMEOUT!
```

**After:**
```go
// Refresh deadline before each REPLCONF ACK
if writeTimeout > 0 {
    deadline := time.Now().Add(writeTimeout)
    conn.SetWriteDeadline(deadline)
}
writer.WriteCommand("REPLCONF", "ACK", offset) // SUCCESS!
```

### 2. Write Serialization
**Before:**
```go
// No synchronization - potential race conditions
func (c *Client) sendReplconfACK() {
    writer.WriteCommand("REPLCONF", "ACK", offset)
}
func (c *Client) authenticate() {
    writer.WriteCommand("AUTH", password)
}
```

**After:**
```go
// Serialized writes with mutex
func (c *Client) sendReplconfACK() {
    c.writeMu.Lock()
    defer c.writeMu.Unlock()
    // ... set deadline and write
}
```

### 3. Improved Heartbeat Interval
**Before:**
```go
heartbeatInterval: 30 * time.Second, // Too conservative
```

**After:**
```go
heartbeatInterval: 10 * time.Second, // Aligned with Redis default
```

### 4. TCP Keepalive
**Before:**
```go
// No TCP keepalive configuration
conn, err := dialer.Dial("tcp", masterAddr)
```

**After:**
```go
conn, err := dialer.Dial("tcp", masterAddr)
if tcpConn, ok := conn.(*net.TCPConn); ok {
    tcpConn.SetKeepAlive(true)
    tcpConn.SetKeepAlivePeriod(15 * time.Second)
}
```

### 5. Enhanced Error Handling
**Before:**
```go
if err := c.sendReplconfACK(); err != nil {
    c.logger.Debug("Heartbeat ACK failed", "error", err)
    // No retry logic or failure tracking
}
```

**After:**
```go
if err := c.sendReplconfACK(); err != nil {
    c.heartbeatFailures++
    if strings.Contains(err.Error(), "i/o timeout") {
        c.logger.Debug("Heartbeat ACK timeout", "failures", c.heartbeatFailures)
        if c.heartbeatFailures >= c.maxHeartbeatFailures {
            c.logger.Error("Too many consecutive heartbeat failures")
        }
    }
} else {
    if c.heartbeatFailures > 0 {
        c.logger.Debug("Heartbeat recovered", "previous_failures", c.heartbeatFailures)
        c.heartbeatFailures = 0
    }
}
```

## Results

These changes should eliminate the "i/o timeout" errors in REPLCONF ACK operations and significantly reduce connection drops that lead to partial resync cycles.

## Testing

Added comprehensive tests covering:
- Write deadline renewal scenarios  
- Heartbeat interval vs write timeout combinations
- Retry logic and error handling
- TCP keepalive configuration
- Write serialization protection

All tests pass with race detection enabled.