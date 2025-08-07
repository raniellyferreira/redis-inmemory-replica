package main

import (
	"fmt"
	"log"

	"github.com/raniellyferreira/redis-inmemory-replica/lua"
	"github.com/raniellyferreira/redis-inmemory-replica/storage"
)

func main() {
	fmt.Println("Redis Replica with Lua Script Support Demo")
	fmt.Println("==========================================")

	// Create storage and Lua engine (in real use, this would come from the replica)
	stor := storage.NewMemory()
	engine := lua.NewEngine(stor)

	fmt.Println("\n📊 Demonstrating Lua script execution on replica storage:")

	// Simulate some data in storage
	stor.Set("user:1:name", []byte("Alice"), nil)
	stor.Set("user:1:email", []byte("alice@example.com"), nil)
	stor.Set("user:2:name", []byte("Bob"), nil)
	stor.Set("user:2:email", []byte("bob@example.com"), nil)

	// Example: Lua script to get user info
	script := `
		local user_id = ARGV[1]
		local name = redis.call('GET', 'user:' .. user_id .. ':name')
		local email = redis.call('GET', 'user:' .. user_id .. ':email')
		
		if not name then
			return nil
		end
		
		return {name, email}
	`

	fmt.Println("\n🔧 Script: Get user information")
	fmt.Println("   Keys: [] (using ARGV for user ID)")
	fmt.Println("   Args: [1]")

	result, err := engine.Eval(script, []string{}, []string{"1"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("   Result: %v\n", result)

	// Example: Atomic counter with Lua
	counterScript := `
		local key = KEYS[1]
		local increment = tonumber(ARGV[1]) or 1
		
		local current = redis.call('GET', key)
		if not current then
			current = 0
		else
			current = tonumber(current)
		end
		
		local new_value = current + increment
		redis.call('SET', key, tostring(new_value))
		return new_value
	`

	fmt.Println("\n🔢 Script: Atomic counter")
	fmt.Println("   Keys: [counter]")
	fmt.Println("   Args: [5]")

	result, err = engine.Eval(counterScript, []string{"counter"}, []string{"5"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("   Result: %v\n", result)

	// Run it again to show increment
	result, err = engine.Eval(counterScript, []string{"counter"}, []string{"3"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("   Result after second run: %v\n", result)

	// Example: Bulk operations
	bulkScript := `
		local prefix = ARGV[1]
		local count = tonumber(ARGV[2])
		local results = {}
		
		for i = 1, count do
			local key = prefix .. ':' .. i
			local value = 'item_' .. i
			redis.call('SET', key, value)
			results[i] = key .. '=' .. value
		end
		
		return results
	`

	fmt.Println("\n📦 Script: Bulk operations")
	fmt.Println("   Keys: []")
	fmt.Println("   Args: [items, 3]")

	result, err = engine.Eval(bulkScript, []string{}, []string{"items", "3"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("   Result: %v\n", result)

	fmt.Println("\n✅ Lua script execution on replica storage successful!")
	fmt.Println("\n💡 Integration points:")
	fmt.Println("   - ✅ Lua engine with Redis-compatible API")
	fmt.Println("   - ✅ EVAL, EVALSHA, SCRIPT commands implemented")
	fmt.Println("   - ✅ Full test suite and benchmarks")
	fmt.Println("   - ✅ Ready for Redis client integration")
	fmt.Println("\n🚀 Usage with github.com/redis/go-redis:")
	fmt.Println("   client := redis.NewClient(&redis.Options{Addr: \"replica:6380\"})")
	fmt.Println("   result := client.Eval(ctx, script, keys, args)")
}
