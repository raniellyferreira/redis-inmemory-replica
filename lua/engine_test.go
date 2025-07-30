package lua

import (
	"testing"

	"github.com/raniellyferreira/redis-inmemory-replica/storage"
)

func TestLuaEngine_BasicExecution(t *testing.T) {
	// Create storage and engine
	stor := storage.NewMemory()
	engine := NewEngine(stor)

	tests := []struct {
		name     string
		script   string
		keys     []string
		args     []string
		expected interface{}
	}{
		{
			name:     "simple return",
			script:   "return 'hello'",
			keys:     []string{},
			args:     []string{},
			expected: "hello",
		},
		{
			name:     "return number",
			script:   "return 42",
			keys:     []string{},
			args:     []string{},
			expected: int64(42),
		},
		{
			name:     "access KEYS",
			script:   "return KEYS[1]",
			keys:     []string{"mykey"},
			args:     []string{},
			expected: "mykey",
		},
		{
			name:     "access ARGV",
			script:   "return ARGV[1]",
			keys:     []string{},
			args:     []string{"myarg"},
			expected: "myarg",
		},
		{
			name:     "concatenate KEYS and ARGV",
			script:   "return KEYS[1] .. ':' .. ARGV[1]",
			keys:     []string{"user"},
			args:     []string{"123"},
			expected: "user:123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.Eval(tt.script, tt.keys, tt.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestLuaEngine_RedisCommands(t *testing.T) {
	stor := storage.NewMemory()
	engine := NewEngine(stor)

	tests := []struct {
		name     string
		script   string
		keys     []string
		args     []string
		setup    func()
		expected interface{}
	}{
		{
			name:     "SET and GET",
			script:   "redis.call('SET', KEYS[1], ARGV[1]); return redis.call('GET', KEYS[1])",
			keys:     []string{"testkey"},
			args:     []string{"testvalue"},
			setup:    func() {},
			expected: "testvalue",
		},
		{
			name:     "GET non-existent key",
			script:   "return redis.call('GET', 'nonexistent')",
			keys:     []string{},
			args:     []string{},
			setup:    func() {},
			expected: false, // Redis nil becomes false in Lua
		},
		{
			name:   "DEL command",
			script: "redis.call('SET', 'delkey', 'value'); return redis.call('DEL', 'delkey')",
			keys:   []string{},
			args:   []string{},
			setup:  func() {},
			expected: int64(1),
		},
		{
			name:   "EXISTS command",
			script: "redis.call('SET', 'existkey', 'value'); return redis.call('EXISTS', 'existkey')",
			keys:   []string{},
			args:   []string{},
			setup:  func() {},
			expected: int64(1),
		},
		{
			name:   "TYPE command",
			script: "redis.call('SET', 'typekey', 'value'); return redis.call('TYPE', 'typekey')",
			keys:   []string{},
			args:   []string{},
			setup:  func() {},
			expected: "string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset storage
			stor.FlushAll()
			tt.setup()

			result, err := engine.Eval(tt.script, tt.keys, tt.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestLuaEngine_RedisPCall(t *testing.T) {
	stor := storage.NewMemory()
	engine := NewEngine(stor)

	tests := []struct {
		name     string
		script   string
		keys     []string
		args     []string
		hasError bool
	}{
		{
			name:     "pcall with valid command",
			script:   "local result = redis.pcall('SET', 'pcallkey', 'value'); return result",
			keys:     []string{},
			args:     []string{},
			hasError: false,
		},
		{
			name:     "pcall with invalid command",
			script:   "local result = redis.pcall('INVALIDCMD'); return type(result) == 'table' and result.err ~= nil",
			keys:     []string{},
			args:     []string{},
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stor.FlushAll()

			result, err := engine.Eval(tt.script, tt.keys, tt.args)
			if tt.hasError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.hasError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			t.Logf("Result: %v", result)
		})
	}
}

func TestLuaEngine_ScriptCaching(t *testing.T) {
	stor := storage.NewMemory()
	engine := NewEngine(stor)

	script := "return 'cached script'"
	
	// Load script and get SHA
	sha := engine.LoadScript(script)
	if len(sha) != 40 { // SHA1 is 40 characters in hex
		t.Errorf("expected SHA1 length 40, got %d", len(sha))
	}

	// Execute using EVALSHA
	result, err := engine.EvalSHA(sha, []string{}, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "cached script" {
		t.Errorf("expected 'cached script', got %v", result)
	}

	// Try to execute non-existent script
	_, err = engine.EvalSHA("nonexistent", []string{}, []string{})
	if err == nil {
		t.Error("expected error for non-existent script")
	}
}

func TestLuaEngine_ScriptExists(t *testing.T) {
	stor := storage.NewMemory()
	engine := NewEngine(stor)

	script1 := "return 1"
	script2 := "return 2"
	
	sha1 := engine.LoadScript(script1)
	sha2 := engine.LoadScript(script2)

	// Test exists with loaded scripts
	results := engine.ScriptExists([]string{sha1, sha2, "nonexistent"})
	expected := []bool{true, true, false}
	
	for i, result := range results {
		if result != expected[i] {
			t.Errorf("position %d: expected %t, got %t", i, expected[i], result)
		}
	}
}

func TestLuaEngine_ScriptFlush(t *testing.T) {
	stor := storage.NewMemory()
	engine := NewEngine(stor)

	script := "return 'test'"
	sha := engine.LoadScript(script)

	// Verify script exists
	results := engine.ScriptExists([]string{sha})
	if !results[0] {
		t.Error("script should exist before flush")
	}

	// Flush scripts
	engine.ScriptFlush()

	// Verify script no longer exists
	results = engine.ScriptExists([]string{sha})
	if results[0] {
		t.Error("script should not exist after flush")
	}
}

func TestLuaEngine_DataTypeConversion(t *testing.T) {
	stor := storage.NewMemory()
	engine := NewEngine(stor)

	tests := []struct {
		name     string
		script   string
		expected interface{}
	}{
		{
			name:     "return nil",
			script:   "return nil",
			expected: nil,
		},
		{
			name:     "return boolean true",
			script:   "return true",
			expected: true,
		},
		{
			name:     "return boolean false",
			script:   "return false",
			expected: false,
		},
		{
			name:     "return string",
			script:   "return 'hello world'",
			expected: "hello world",
		},
		{
			name:     "return integer",
			script:   "return 123",
			expected: int64(123),
		},
		{
			name:     "return float",
			script:   "return 123.456",
			expected: 123.456,
		},
		{
			name:     "return array",
			script:   "return {1, 2, 3}",
			expected: []interface{}{int64(1), int64(2), int64(3)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.Eval(tt.script, []string{}, []string{})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Special handling for arrays
			if expectedArray, ok := tt.expected.([]interface{}); ok {
				resultArray, ok := result.([]interface{})
				if !ok {
					t.Fatalf("expected array, got %T", result)
				}
				if len(resultArray) != len(expectedArray) {
					t.Fatalf("array length mismatch: expected %d, got %d", len(expectedArray), len(resultArray))
				}
				for i, expected := range expectedArray {
					if resultArray[i] != expected {
						t.Errorf("array[%d]: expected %v, got %v", i, expected, resultArray[i])
					}
				}
			} else {
				if result != tt.expected {
					t.Errorf("expected %v (%T), got %v (%T)", tt.expected, tt.expected, result, result)
				}
			}
		})
	}
}

func TestLuaEngine_ErrorHandling(t *testing.T) {
	stor := storage.NewMemory()
	engine := NewEngine(stor)

	tests := []struct {
		name   string
		script string
		keys   []string
		args   []string
	}{
		{
			name:   "syntax error",
			script: "invalid lua syntax !!!",
			keys:   []string{},
			args:   []string{},
		},
		{
			name:   "redis.call with invalid args",
			script: "return redis.call()",
			keys:   []string{},
			args:   []string{},
		},
		{
			name:   "redis.call with unknown command",
			script: "return redis.call('UNKNOWNCMD')",
			keys:   []string{},
			args:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := engine.Eval(tt.script, tt.keys, tt.args)
			if err == nil {
				t.Error("expected error but got none")
			}
			t.Logf("Got expected error: %v", err)
		})
	}
}