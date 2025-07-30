package lua

import (
	"crypto/sha1"
	"fmt"
	"sync"

	lua "github.com/yuin/gopher-lua"
	"github.com/raniellyferreira/redis-inmemory-replica/storage"
)

// Engine provides Redis-compatible Lua script execution
type Engine struct {
	storage storage.Storage
	scripts sync.Map // map[string]string - SHA1 -> script content
}

// NewEngine creates a new Lua execution engine
func NewEngine(storage storage.Storage) *Engine {
	return &Engine{
		storage: storage,
	}
}

// Eval executes a Lua script with the given keys and arguments
func (e *Engine) Eval(script string, keys []string, args []string) (interface{}, error) {
	L := lua.NewState()
	defer L.Close()

	// Set up the Redis-compatible environment
	if err := e.setupRedisAPI(L, keys, args); err != nil {
		return nil, err
	}

	// Execute the script
	if err := L.DoString(script); err != nil {
		return nil, fmt.Errorf("script execution error: %w", err)
	}

	// Get the return value from the stack
	return e.convertLuaValue(L.Get(-1)), nil
}

// EvalSHA executes a previously loaded script by its SHA1 hash
func (e *Engine) EvalSHA(sha1 string, keys []string, args []string) (interface{}, error) {
	script, exists := e.scripts.Load(sha1)
	if !exists {
		return nil, fmt.Errorf("NOSCRIPT No matching script. Please use EVAL")
	}

	return e.Eval(script.(string), keys, args)
}

// LoadScript loads a script and returns its SHA1 hash
func (e *Engine) LoadScript(script string) string {
	hash := fmt.Sprintf("%x", sha1.Sum([]byte(script)))
	e.scripts.Store(hash, script)
	return hash
}

// ScriptExists checks if scripts with given SHA1 hashes exist
func (e *Engine) ScriptExists(hashes []string) []bool {
	results := make([]bool, len(hashes))
	for i, hash := range hashes {
		_, exists := e.scripts.Load(hash)
		results[i] = exists
	}
	return results
}

// ScriptFlush removes all cached scripts
func (e *Engine) ScriptFlush() {
	e.scripts.Range(func(key, value interface{}) bool {
		e.scripts.Delete(key)
		return true
	})
}

// setupRedisAPI configures the Lua state with Redis-compatible functions
func (e *Engine) setupRedisAPI(L *lua.LState, keys []string, args []string) error {
	// Create KEYS table
	keysTable := L.NewTable()
	for i, key := range keys {
		keysTable.RawSetInt(i+1, lua.LString(key)) // Lua arrays are 1-indexed
	}
	L.SetGlobal("KEYS", keysTable)

	// Create ARGV table
	argvTable := L.NewTable()
	for i, arg := range args {
		argvTable.RawSetInt(i+1, lua.LString(arg)) // Lua arrays are 1-indexed
	}
	L.SetGlobal("ARGV", argvTable)

	// Create redis table with call and pcall functions
	redisTable := L.NewTable()
	L.SetFuncs(redisTable, map[string]lua.LGFunction{
		"call":  e.redisCall,
		"pcall": e.redisPCall,
	})
	L.SetGlobal("redis", redisTable)

	return nil
}

// redisCall implements redis.call() function
func (e *Engine) redisCall(L *lua.LState) int {
	result, err := e.executeRedisCommand(L)
	if err != nil {
		L.Error(lua.LString(err.Error()), 1)
		return 0
	}
	L.Push(e.convertToLuaValue(L, result))
	return 1
}

// redisPCall implements redis.pcall() function (protected call)
func (e *Engine) redisPCall(L *lua.LState) int {
	result, err := e.executeRedisCommand(L)
	if err != nil {
		// Return error as a table with 'err' field
		errTable := L.NewTable()
		errTable.RawSetString("err", lua.LString(err.Error()))
		L.Push(errTable)
		return 1
	}
	L.Push(e.convertToLuaValue(L, result))
	return 1
}

// executeRedisCommand executes a Redis command from Lua
func (e *Engine) executeRedisCommand(L *lua.LState) (interface{}, error) {
	argc := L.GetTop()
	if argc == 0 {
		return nil, fmt.Errorf("wrong number of arguments for redis command")
	}

	// Get command name
	cmdName := L.ToString(1)
	if cmdName == "" {
		return nil, fmt.Errorf("command name must be a string")
	}

	// Get arguments
	args := make([]string, argc-1)
	for i := 2; i <= argc; i++ {
		args[i-2] = L.ToString(i)
	}

	// Execute the Redis command
	return e.executeCommand(cmdName, args)
}

// executeCommand executes a Redis command against the storage
func (e *Engine) executeCommand(cmd string, args []string) (interface{}, error) {
	switch cmd {
	case "GET":
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'get' command")
		}
		value, exists := e.storage.Get(args[0])
		if !exists {
			return nil, nil // Redis returns nil for non-existent keys
		}
		return string(value), nil

	case "SET":
		if len(args) < 2 {
			return nil, fmt.Errorf("wrong number of arguments for 'set' command")
		}
		err := e.storage.Set(args[0], []byte(args[1]), nil)
		if err != nil {
			return nil, err
		}
		return "OK", nil

	case "DEL":
		if len(args) == 0 {
			return nil, fmt.Errorf("wrong number of arguments for 'del' command")
		}
		deleted := e.storage.Del(args...)
		return deleted, nil

	case "EXISTS":
		if len(args) == 0 {
			return nil, fmt.Errorf("wrong number of arguments for 'exists' command")
		}
		count := e.storage.Exists(args...)
		return count, nil

	case "TYPE":
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'type' command")
		}
		valueType := e.storage.Type(args[0])
		return valueType.String(), nil

	default:
		return nil, fmt.Errorf("unknown or unsupported command: %s", cmd)
	}
}

// convertToLuaValue converts a Go value to a Lua value
func (e *Engine) convertToLuaValue(L *lua.LState, value interface{}) lua.LValue {
	if value == nil {
		return lua.LFalse // Redis nil becomes false in Lua
	}

	switch v := value.(type) {
	case string:
		return lua.LString(v)
	case int64:
		return lua.LNumber(float64(v))
	case int:
		return lua.LNumber(float64(v))
	case float64:
		return lua.LNumber(v)
	case bool:
		return lua.LBool(v)
	case []interface{}:
		table := L.NewTable()
		for i, item := range v {
			table.RawSetInt(i+1, e.convertToLuaValue(L, item))
		}
		return table
	default:
		return lua.LString(fmt.Sprintf("%v", v))
	}
}

// convertLuaValue converts a Lua value to a Go value
func (e *Engine) convertLuaValue(lv lua.LValue) interface{} {
	switch v := lv.(type) {
	case lua.LBool:
		return bool(v)
	case lua.LString:
		return string(v)
	case lua.LNumber:
		// Check if it's an integer
		f := float64(v)
		if f == float64(int64(f)) {
			return int64(f)
		}
		return f
	case *lua.LNilType:
		return nil
	case *lua.LTable:
		// Convert table to slice if it's array-like
		if e.isArrayLikeTable(v) {
			var result []interface{}
			v.ForEach(func(k, val lua.LValue) {
				result = append(result, e.convertLuaValue(val))
			})
			return result
		}
		// Convert to map for object-like tables
		result := make(map[string]interface{})
		v.ForEach(func(k, val lua.LValue) {
			result[k.String()] = e.convertLuaValue(val)
		})
		return result
	default:
		return lv.String()
	}
}

// isArrayLikeTable checks if a Lua table is array-like (consecutive integer keys starting from 1)
func (e *Engine) isArrayLikeTable(table *lua.LTable) bool {
	length := table.Len()
	if length == 0 {
		return true
	}

	for i := 1; i <= length; i++ {
		if table.RawGetInt(i) == lua.LNil {
			return false
		}
	}

	// Check if there are any non-integer keys
	hasNonIntKeys := false
	table.ForEach(func(k, v lua.LValue) {
		if _, ok := k.(lua.LNumber); !ok {
			hasNonIntKeys = true
		} else {
			// Check if it's a valid array index
			if num, ok := k.(lua.LNumber); ok {
				idx := int(num)
				if float64(idx) != float64(num) || idx < 1 || idx > length {
					hasNonIntKeys = true
				}
			}
		}
	})

	return !hasNonIntKeys
}