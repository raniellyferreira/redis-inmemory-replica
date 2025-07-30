package main

import (
	"fmt"
	"log"

	"github.com/raniellyferreira/redis-inmemory-replica/lua"
	"github.com/raniellyferreira/redis-inmemory-replica/storage"
)

func main() {
	// Create storage and Lua engine
	stor := storage.NewMemory()
	engine := lua.NewEngine(stor)

	fmt.Println("Redis-compatible Lua Script Execution Demo")
	fmt.Println("==========================================")

	// Example 1: Simple script
	fmt.Println("\n1. Simple script execution:")
	result, err := engine.Eval("return 'Hello from Lua!'", []string{}, []string{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("   Result: %v\n", result)

	// Example 2: Using KEYS and ARGV
	fmt.Println("\n2. Script with KEYS and ARGV:")
	script := "return KEYS[1] .. ':' .. ARGV[1]"
	result, err = engine.Eval(script, []string{"user"}, []string{"123"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("   Script: %s\n", script)
	fmt.Printf("   Result: %v\n", result)

	// Example 3: Redis commands in Lua
	fmt.Println("\n3. Redis commands in Lua:")
	script = `
		redis.call('SET', KEYS[1], ARGV[1])
		local value = redis.call('GET', KEYS[1])
		return 'Stored and retrieved: ' .. value
	`
	result, err = engine.Eval(script, []string{"mykey"}, []string{"myvalue"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("   Result: %v\n", result)

	// Example 4: Script caching with EVALSHA
	fmt.Println("\n4. Script caching (EVALSHA):")
	script = "return 'This is a cached script with arg: ' .. (ARGV[1] or 'none')"
	sha := engine.LoadScript(script)
	fmt.Printf("   Script SHA1: %s\n", sha)

	result, err = engine.EvalSHA(sha, []string{}, []string{"hello"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("   Result: %v\n", result)

	// Example 5: Complex script with loops and Redis operations
	fmt.Println("\n5. Complex script example:")
	script = `
		local results = {}
		for i = 1, #KEYS do
			local key = KEYS[i]
			local val = ARGV[i] or 'default'
			redis.call('SET', key, val)
			results[i] = key .. '=' .. redis.call('GET', key)
		end
		return results
	`
	keys := []string{"key1", "key2", "key3"}
	args := []string{"val1", "val2", "val3"}
	result, err = engine.Eval(script, keys, args)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("   Keys: %v\n", keys)
	fmt.Printf("   Args: %v\n", args)
	fmt.Printf("   Result: %v\n", result)

	// Example 6: Error handling with redis.pcall
	fmt.Println("\n6. Error handling with redis.pcall:")
	script = `
		local result = redis.pcall('GET', 'nonexistent')
		if type(result) == 'table' and result.err then
			return 'Error: ' .. result.err
		else
			return 'Success: ' .. tostring(result)
		end
	`
	result, err = engine.Eval(script, []string{}, []string{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("   Result: %v\n", result)

	fmt.Println("\n✅ All examples completed successfully!")
	fmt.Println("\nThe Lua engine is now ready for integration with Redis clients like github.com/redis/go-redis")
}