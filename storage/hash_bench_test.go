package storage

import (
	"hash/maphash"
	"testing"
	
	"github.com/cespare/xxhash/v2"
)

// BenchmarkHashAlgorithms compares different hash algorithms for shard selection
func BenchmarkHashAlgorithms(b *testing.B) {
	keys := []string{
		"user:123",
		"session:abc123def456",
		"cache:product:999",
		"short",
		"averagesizedkeyname",
		"verylongkeynamethatmightbeusedinsomeapplications",
	}
	
	b.Run("maphash", func(b *testing.B) {
		var h maphash.Hash
		seed := maphash.MakeSeed()
		h.SetSeed(seed)
		shardMask := uint64(255) // 256 shards
		
		b.ResetTimer()
		b.ReportAllocs()
		
		for i := 0; i < b.N; i++ {
			key := keys[i%len(keys)]
			h.Reset()
			h.WriteString(key)
			_ = h.Sum64() & shardMask
		}
	})
	
	b.Run("xxhash", func(b *testing.B) {
		shardMask := uint64(255) // 256 shards
		
		b.ResetTimer()
		b.ReportAllocs()
		
		for i := 0; i < b.N; i++ {
			key := keys[i%len(keys)]
			_ = xxhash.Sum64String(key) & shardMask
		}
	})
}

// BenchmarkShardSelection compares shard selection performance
func BenchmarkShardSelection(b *testing.B) {
	s := NewMemory(WithShardCount(256))
	keys := make([]string, 1000)
	for i := 0; i < len(keys); i++ {
		keys[i] = "key:" + string(rune('a'+i%26)) + string(rune('0'+i%10))
	}
	
	b.Run("current_maphash", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		
		for i := 0; i < b.N; i++ {
			key := keys[i%len(keys)]
			_ = s.keyHash(key)
		}
	})
	
	b.Run("xxhash", func(b *testing.B) {
		shardMask := uint64(255)
		
		b.ResetTimer()
		b.ReportAllocs()
		
		for i := 0; i < b.N; i++ {
			key := keys[i%len(keys)]
			_ = xxhash.Sum64String(key) & shardMask
		}
	})
}
