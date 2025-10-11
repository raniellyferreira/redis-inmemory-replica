package lua

import (
	"testing"

	"github.com/raniellyferreira/redis-inmemory-replica/storage"
)

func BenchmarkLuaEngine_SimpleScript(b *testing.B) {
	stor := storage.NewMemory()
	engine := NewEngine(stor)
	script := "return 'hello world'"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := engine.Eval(script, []string{}, []string{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLuaEngine_ScriptWithKeys(b *testing.B) {
	stor := storage.NewMemory()
	engine := NewEngine(stor)
	script := "return KEYS[1] .. ':' .. ARGV[1]"
	keys := []string{"user"}
	args := []string{"123"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := engine.Eval(script, keys, args)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLuaEngine_RedisCommands(b *testing.B) {
	stor := storage.NewMemory()
	engine := NewEngine(stor)
	script := "redis.call('SET', KEYS[1], ARGV[1]); return redis.call('GET', KEYS[1])"
	keys := []string{"benchkey"}
	args := []string{"benchvalue"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := engine.Eval(script, keys, args)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLuaEngine_EvalSHA(b *testing.B) {
	stor := storage.NewMemory()
	engine := NewEngine(stor)
	script := "return 'cached script'"
	sha := engine.LoadScript(script)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := engine.EvalSHA(sha, []string{}, []string{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLuaEngine_ComplexScript(b *testing.B) {
	stor := storage.NewMemory()
	engine := NewEngine(stor)
	script := `
		local result = {}
		for i = 1, #KEYS do
			redis.call('SET', KEYS[i], ARGV[i])
			result[i] = redis.call('GET', KEYS[i])
		end
		return result
	`
	keys := []string{"key1", "key2", "key3", "key4", "key5"}
	args := []string{"val1", "val2", "val3", "val4", "val5"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := engine.Eval(script, keys, args)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLuaEngine_LuaOperations(b *testing.B) {
	stor := storage.NewMemory()
	engine := NewEngine(stor)
	script := `
		local sum = 0
		for i = 1, 100 do
			sum = sum + i
		end
		return sum
	`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := engine.Eval(script, []string{}, []string{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLuaEngine_ArrayOperations(b *testing.B) {
	stor := storage.NewMemory()
	engine := NewEngine(stor)
	script := `
		local arr = {}
		for i = 1, 10 do
			arr[i] = "item" .. i
		end
		return arr
	`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := engine.Eval(script, []string{}, []string{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLuaEngine_StringManipulation(b *testing.B) {
	stor := storage.NewMemory()
	engine := NewEngine(stor)
	script := `
		local result = ""
		for i = 1, #ARGV do
			result = result .. ARGV[i] .. ":"
		end
		return string.sub(result, 1, -2)  -- remove trailing colon
	`
	args := []string{"hello", "world", "from", "lua", "script"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := engine.Eval(script, []string{}, args)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLuaEngine_LoadScript(b *testing.B) {
	stor := storage.NewMemory()
	engine := NewEngine(stor)
	scripts := []string{
		"return 'script1'",
		"return 'script2'",
		"return 'script3'",
		"return 'script4'",
		"return 'script5'",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		script := scripts[i%len(scripts)]
		engine.LoadScript(script)
	}
}

func BenchmarkLuaEngine_ScriptExists(b *testing.B) {
	stor := storage.NewMemory()
	engine := NewEngine(stor)

	// Pre-load some scripts
	hashes := make([]string, 10)
	for i := 0; i < 10; i++ {
		hashes[i] = engine.LoadScript("return '" + string(rune('a'+i)) + "'")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.ScriptExists(hashes)
	}
}

func BenchmarkLuaEngine_ScriptFlush(b *testing.B) {
	stor := storage.NewMemory()
	engine := NewEngine(stor)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Load some scripts
		for j := 0; j < 10; j++ {
			engine.LoadScript("return '" + string(rune('a'+j)) + "'")
		}
		// Flush them
		engine.ScriptFlush()
	}
}

// Benchmark parallel execution
func BenchmarkLuaEngine_ParallelExecution(b *testing.B) {
	stor := storage.NewMemory()
	engine := NewEngine(stor)
	script := "return redis.call('SET', KEYS[1], ARGV[1])"

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := []string{"parallel_key_" + string(rune('a'+i%26))}
			arg := []string{"value_" + string(rune('a'+i%26))}
			_, err := engine.Eval(script, key, arg)
			if err != nil {
				b.Fatal(err)
			}
			i++
		}
	})
}

// Benchmark memory usage with many scripts
func BenchmarkLuaEngine_MemoryUsage(b *testing.B) {
	stor := storage.NewMemory()
	engine := NewEngine(stor)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Create unique script to avoid SHA collision
		script := "return '" + string(rune('a'+i%26)) + string(rune('a'+(i/26)%26)) + "'"
		sha := engine.LoadScript(script)

		// Execute the script occasionally
		if i%100 == 0 {
			_, err := engine.EvalSHA(sha, []string{}, []string{})
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}

// BenchmarkLuaEngine_VaryingKeysCardinality benchmarks with different numbers of KEYS
func BenchmarkLuaEngine_VaryingKeysCardinality(b *testing.B) {
	stor := storage.NewMemory()
	engine := NewEngine(stor)
	
	keyCounts := []int{0, 1, 5, 10, 25, 50}
	
	for _, count := range keyCounts {
		name := "Keys_" + itoa(count)
		b.Run(name, func(b *testing.B) {
			// Build script that accesses all KEYS
			script := "local result = {} "
			for i := 1; i <= count; i++ {
				script += "result[" + itoa(i) + "] = KEYS[" + itoa(i) + "] "
			}
			script += "return result"
			
			// Prepare keys array
			keys := make([]string, count)
			for i := 0; i < count; i++ {
				keys[i] = "key_" + itoa(i)
			}
			
			b.ResetTimer()
			b.ReportAllocs()
			
			for i := 0; i < b.N; i++ {
				_, err := engine.Eval(script, keys, []string{})
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkLuaEngine_VaryingArgsCardinality benchmarks with different numbers of ARGV
func BenchmarkLuaEngine_VaryingArgsCardinality(b *testing.B) {
	stor := storage.NewMemory()
	engine := NewEngine(stor)
	
	argCounts := []int{0, 1, 5, 10, 25, 50}
	
	for _, count := range argCounts {
		name := "Args_" + itoa(count)
		b.Run(name, func(b *testing.B) {
			// Build script that accesses all ARGV
			script := "local result = {} "
			for i := 1; i <= count; i++ {
				script += "result[" + itoa(i) + "] = ARGV[" + itoa(i) + "] "
			}
			script += "return result"
			
			// Prepare args array
			args := make([]string, count)
			for i := 0; i < count; i++ {
				args[i] = "value_" + itoa(i)
			}
			
			b.ResetTimer()
			b.ReportAllocs()
			
			for i := 0; i < b.N; i++ {
				_, err := engine.Eval(script, []string{}, args)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// Helper function to convert int to string
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	
	var buf [20]byte
	i := len(buf) - 1
	neg := n < 0
	if neg {
		n = -n
	}
	
	for n > 0 {
		buf[i] = byte('0' + n%10)
		n /= 10
		i--
	}
	
	if neg {
		buf[i] = '-'
		i--
	}
	
	return string(buf[i+1:])
}

// BenchmarkLuaEngine_KeysAndArgsCardinality benchmarks with both KEYS and ARGV
func BenchmarkLuaEngine_KeysAndArgsCardinality(b *testing.B) {
	stor := storage.NewMemory()
	engine := NewEngine(stor)
	
	scenarios := []struct {
		name     string
		keyCount int
		argCount int
	}{
		{"1k_1a", 1, 1},
		{"5k_5a", 5, 5},
		{"10k_10a", 10, 10},
		{"25k_25a", 25, 25},
	}
	
	for _, sc := range scenarios {
		b.Run(sc.name, func(b *testing.B) {
			// Script that processes both KEYS and ARGV
			script := `
				local result = {}
				for i = 1, #KEYS do
					redis.call('SET', KEYS[i], ARGV[i])
					result[i] = redis.call('GET', KEYS[i])
				end
				return result
			`
			
			// Prepare keys and args
			keys := make([]string, sc.keyCount)
			args := make([]string, sc.argCount)
			for i := 0; i < sc.keyCount; i++ {
				keys[i] = "key_" + string(rune('a'+i%26))
			}
			for i := 0; i < sc.argCount; i++ {
				args[i] = "value_" + string(rune('a'+i%26))
			}
			
			b.ResetTimer()
			b.ReportAllocs()
			
			for i := 0; i < b.N; i++ {
				_, err := engine.Eval(script, keys, args)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkLuaEngine_CachedVsUncached benchmarks cached (EvalSHA) vs uncached (Eval) execution
func BenchmarkLuaEngine_CachedVsUncached(b *testing.B) {
	stor := storage.NewMemory()
	engine := NewEngine(stor)
	
	script := "return redis.call('SET', KEYS[1], ARGV[1])"
	sha := engine.LoadScript(script)
	keys := []string{"benchmark_key"}
	args := []string{"benchmark_value"}
	
	b.Run("Uncached_Eval", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := engine.Eval(script, keys, args)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
	
	b.Run("Cached_EvalSHA", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := engine.EvalSHA(sha, keys, args)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
