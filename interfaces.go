package redisreplica

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// Field represents a structured log field
type Field struct {
	Key   string
	Value interface{}
}

// Logger interface for custom logging implementations
type Logger interface {
	// Debug logs a debug message with optional fields
	Debug(msg string, fields ...Field)

	// Info logs an info message with optional fields
	Info(msg string, fields ...Field)

	// Error logs an error message with optional fields
	Error(msg string, fields ...Field)
}

// MetricsCollector interface for metrics collection
type MetricsCollector interface {
	// RecordSyncDuration records the time taken for synchronization
	RecordSyncDuration(duration time.Duration)

	// RecordCommandProcessed records a processed command with its duration
	RecordCommandProcessed(cmd string, duration time.Duration)

	// RecordNetworkBytes records network bytes transferred
	RecordNetworkBytes(bytes int64)

	// RecordKeyCount records the current number of keys
	RecordKeyCount(count int64)

	// RecordMemoryUsage records current memory usage
	RecordMemoryUsage(bytes int64)

	// RecordReconnection records a reconnection event
	RecordReconnection()

	// RecordError records an error event
	RecordError(errorType string)
}

// ReplicationStats provides detailed replication statistics
type ReplicationStats struct {
	mu sync.RWMutex

	// Connection stats
	ConnectedAt    time.Time
	DisconnectedAt time.Time
	ReconnectCount int64

	// Sync stats
	InitialSyncStart time.Time
	InitialSyncEnd   time.Time
	FullSyncCount    int64
	PartialSyncCount int64

	// Data stats
	KeyCount         int64
	MemoryUsage      int64
	NetworkBytesRecv int64

	// Command stats
	CommandsProcessed map[string]int64
}

// GetConnectedAt returns when the replica connected (thread-safe)
func (s *ReplicationStats) GetConnectedAt() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ConnectedAt
}

// GetKeyCount returns the current key count (thread-safe)
func (s *ReplicationStats) GetKeyCount() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.KeyCount
}

// GetMemoryUsage returns the current memory usage (thread-safe)
func (s *ReplicationStats) GetMemoryUsage() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.MemoryUsage
}

// GetCommandCount returns the count for a specific command (thread-safe)
func (s *ReplicationStats) GetCommandCount(cmd string) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.CommandsProcessed[cmd]
}

// defaultLogger is a simple logger implementation using the standard log package
type defaultLogger struct{}

func (l *defaultLogger) Debug(msg string, fields ...Field) {
	l.logWithFields("DEBUG", msg, fields...)
}

func (l *defaultLogger) Info(msg string, fields ...Field) {
	l.logWithFields("INFO", msg, fields...)
}

func (l *defaultLogger) Error(msg string, fields ...Field) {
	l.logWithFields("ERROR", msg, fields...)
}

func (l *defaultLogger) logWithFields(level, msg string, fields ...Field) {
	logMsg := level + ": " + msg
	for _, field := range fields {
		logMsg += " " + field.Key + "=" + formatValue(field.Value)
	}
	log.Println(logMsg)
}

func formatValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case error:
		return val.Error()
	default:
		return fmt.Sprintf("%v", val)
	}
}
