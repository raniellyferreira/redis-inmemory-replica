package storage

import "time"

// ValueType represents the Redis data type
type ValueType int

const (
	ValueTypeString ValueType = iota
	ValueTypeList
	ValueTypeSet
	ValueTypeZSet
	ValueTypeHash
	ValueTypeStream
)

// String returns the Redis-compatible type name
func (vt ValueType) String() string {
	switch vt {
	case ValueTypeString:
		return "string"
	case ValueTypeList:
		return "list"
	case ValueTypeSet:
		return "set"
	case ValueTypeZSet:
		return "zset"
	case ValueTypeHash:
		return "hash"
	case ValueTypeStream:
		return "stream"
	default:
		return "none"
	}
}

// Value represents a stored value with metadata
type Value struct {
	Type     ValueType
	Data     interface{}
	Expiry   *time.Time
	Version  int64 // For optimistic locking
}

// IsExpired returns true if the value has expired
func (v *Value) IsExpired() bool {
	return v.Expiry != nil && time.Now().After(*v.Expiry)
}

// StringValue represents a string value
type StringValue struct {
	Data []byte
}

// ListValue represents a list value
type ListValue struct {
	Elements [][]byte
}

// SetValue represents a set value
type SetValue struct {
	Members map[string]struct{}
}

// ZSetValue represents a sorted set value
type ZSetValue struct {
	Members map[string]float64
	Scores  []ZSetMember // Sorted by score
}

// ZSetMember represents a sorted set member with score
type ZSetMember struct {
	Member string
	Score  float64
}

// HashValue represents a hash value
type HashValue struct {
	Fields map[string][]byte
}

// StreamValue represents a stream value
type StreamValue struct {
	Entries []StreamEntry
	LastID  string
}

// StreamEntry represents a stream entry
type StreamEntry struct {
	ID     string
	Fields map[string][]byte
}

// KeyInfo provides metadata about a key
type KeyInfo struct {
	Key       string
	Type      ValueType
	TTL       time.Duration // -1 for no expiry, -2 for expired/not found
	Size      int64         // Size in bytes
	LastAccess time.Time
}