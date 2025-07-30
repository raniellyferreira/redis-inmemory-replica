// Package server provides Redis protocol server functionality for the in-memory replica.
//
// This package extends the replica to act as a Redis server that can accept client
// connections and execute Redis commands, including Lua script execution via EVAL
// and EVALSHA commands.
//
// The server is compatible with Redis clients like github.com/redis/go-redis and
// supports:
//   - Basic Redis commands (GET, SET, DEL, etc.)
//   - Lua script execution (EVAL, EVALSHA, SCRIPT LOAD, SCRIPT EXISTS, SCRIPT FLUSH)
//   - RESP protocol for communication
//   - Concurrent client handling
package server