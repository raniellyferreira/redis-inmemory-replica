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