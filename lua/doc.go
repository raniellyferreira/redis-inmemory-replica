// Package lua provides Redis-compatible Lua script execution functionality.
//
// This package implements the Redis scripting API that allows executing Lua scripts
// with access to Redis data and operations. It supports the EVAL and EVALSHA commands
// with Redis-compatible behavior.
//
// The Lua execution environment includes:
//   - redis.call() and redis.pcall() functions for executing Redis commands
//   - Access to KEYS and ARGV arrays passed from the client
//   - Proper data type conversion between Lua and Redis values
//
// Scripts are executed in a sandboxed environment with limited Lua standard library
// access for security purposes.
package lua