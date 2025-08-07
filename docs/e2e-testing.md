# End-to-End Testing with Real Redis

This document describes how to run end-to-end tests that connect to a real Redis instance to validate the redis-inmemory-replica functionality.

## Prerequisites

1. **Redis Server**: You need a running Redis instance for testing
2. **redis-cli**: Command line interface for Redis (usually comes with Redis)
3. **Go**: Go 1.24.5 or later for running the tests

## Setting up Redis for Testing

### Option 1: Using Docker (Recommended)

```bash
# Start Redis in a Docker container
docker run --name redis-test -p 6379:6379 -d redis:latest

# Verify Redis is running
docker exec -it redis-test redis-cli ping
```

### Option 2: Local Redis Installation

```bash
# On Ubuntu/Debian
sudo apt install redis-server

# On macOS
brew install redis

# Start Redis
redis-server

# In another terminal, verify it's running
redis-cli ping
```

### Option 3: Using Redis in CI/CD

For GitHub Actions or other CI environments:

```yaml
services:
  redis:
    image: redis:latest
    ports:
      - 6379:6379
    options: >-
      --health-cmd "redis-cli ping"
      --health-interval 10s
      --health-timeout 5s
      --health-retries 5
```

## Running the Tests

### Basic E2E Test

The main end-to-end test validates:
- Initial synchronization (RDB sync)
- Real-time command replication
- Large value handling
- Rapid update scenarios
- Error recovery

```bash
# Run with default Redis address (localhost:6379)
go test -v -run TestEndToEndWithRealRedis

# Run with custom Redis address
REDIS_ADDR="redis.example.com:6379" go test -v -run TestEndToEndWithRealRedis

# Run with Redis requiring authentication
REDIS_ADDR="localhost:6379" REDIS_PASSWORD="mypassword" go test -v -run TestEndToEndWithRealRedis
```

### RDB Parsing Test

Tests the robustness of RDB file parsing with various data types:

```bash
go test -v -run TestRDBParsingRobustness
```

### Performance Benchmark

Measures replication throughput:

```bash
go test -v -bench BenchmarkReplicationThroughput
```

## Test Configuration

### Environment Variables

- `REDIS_ADDR`: Redis server address (default: `localhost:6379`)
- `REDIS_PASSWORD`: Redis authentication password (if required)
- `REDIS_TLS`: Set to `"true"` to use TLS connection

### Example Configurations

#### Local Development
```bash
export REDIS_ADDR="localhost:6379"
go test -v -run TestEndToEndWithRealRedis
```

#### Remote Redis with Authentication
```bash
export REDIS_ADDR="redis.example.com:6379"
export REDIS_PASSWORD="your-redis-password"
go test -v -run TestEndToEndWithRealRedis
```

#### Redis with TLS
```bash
export REDIS_ADDR="secure-redis.example.com:6380"
export REDIS_TLS="true"
go test -v -run TestEndToEndWithRealRedis
```

## What the Tests Validate

### 1. Connection and Authentication
- Establishes connection to Redis master
- Handles authentication if configured
- Validates connection timeouts and error handling

### 2. Initial Synchronization (RDB)
- Receives and parses RDB data from master
- Handles various data types (strings, numbers, special characters)
- Validates memory management during RDB parsing
- Tests buffer overflow protections

### 3. Real-time Replication
- SET commands with various value sizes
- DEL commands for key deletion
- Rapid updates to test buffer handling
- Large value replication (>10KB)

### 4. Error Recovery
- Network disconnection handling
- Malformed protocol data handling
- Buffer boundary validation
- CRLF terminator validation

### 5. Data Integrity
- Verifies all replicated data matches master
- Checks data types are preserved correctly
- Validates special characters and binary data

## Test Output

Successful test output will look like:

```
=== RUN   TestEndToEndWithRealRedis
    e2e_test.go:25: Running end-to-end test with Redis at localhost:6379
    e2e_test.go:51: 🚀 Starting replica...
    e2e_test.go:59: 📡 Waiting for initial synchronization...
    e2e_test.go:26: ✅ Initial synchronization completed
    e2e_test.go:65: Initial sync completed successfully
    e2e_test.go:68: Test 1: Setting keys in Redis master
    e2e_test.go:84: ✅ Key test:key1 correctly replicated: value1
    e2e_test.go:84: ✅ Key test:key2 correctly replicated: value2
    e2e_test.go:84: ✅ Key counter correctly replicated: 42
    e2e_test.go:84: ✅ Key user:123 correctly replicated: john_doe
    e2e_test.go:96: ✅ Key deletion correctly replicated
    e2e_test.go:147: ✅ Large value correctly replicated
    e2e_test.go:172: ✅ Rapid updates correctly replicated
    e2e_test.go:177: Final replica status:
    e2e_test.go:178:   Connected: true
    e2e_test.go:179:   Commands processed: 15
    e2e_test.go:180:   Replication offset: 15
--- PASS: TestEndToEndWithRealRedis (8.45s)
```

## Troubleshooting

### Redis Connection Issues

```
Error: Redis not available at localhost:6379 - skipping e2e test
```

**Solutions:**
1. Ensure Redis is running: `redis-cli ping`
2. Check Redis is listening on correct port: `netstat -tlnp | grep 6379`
3. Verify firewall settings allow connection
4. Use correct REDIS_ADDR environment variable

### Authentication Errors

```
Error: AUTH failed: ERR invalid password
```

**Solutions:**
1. Check Redis password configuration: `grep requirepass /etc/redis/redis.conf`
2. Set REDIS_PASSWORD environment variable
3. Test authentication manually: `redis-cli -a yourpassword ping`

### Memory Issues

```
Error: runtime error: index out of range
```

This indicates the buffer overflow fixes are working correctly. If this happens during normal operation, it means:
1. The RDB data from Redis is larger than expected
2. Network fragmentation is occurring
3. The fixes successfully prevented a crash

### Performance Issues

If tests are slow:
1. Ensure Redis is running locally or in fast network
2. Check Redis configuration for slow queries: `redis-cli slowlog get`
3. Monitor network latency: `ping redis-host`

## Integration with CI/CD

### GitHub Actions Example

```yaml
name: E2E Tests
on: [push, pull_request]

jobs:
  e2e:
    runs-on: ubuntu-latest
    services:
      redis:
        image: redis:latest
        ports:
          - 6379:6379
        options: >-
          --health-cmd "redis-cli ping"
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5

    steps:
    - uses: actions/checkout@v3
    - uses: actions/setup-go@v4
      with:
        go-version: '1.24.5'
    
    - name: Wait for Redis
      run: |
        until redis-cli ping; do
          echo "Waiting for Redis..."
          sleep 1
        done
    
    - name: Run E2E Tests
      run: go test -v -run TestEndToEndWithRealRedis
      env:
        REDIS_ADDR: localhost:6379
```

### Docker Compose for Testing

```yaml
version: '3.8'
services:
  redis:
    image: redis:latest
    ports:
      - "6379:6379"
    command: redis-server --appendonly yes
    
  test-runner:
    build: .
    depends_on:
      - redis
    environment:
      - REDIS_ADDR=redis:6379
    command: go test -v -run TestEndToEndWithRealRedis
```

## Security Considerations

When running tests against production-like Redis instances:

1. **Use dedicated test databases**: `SELECT 15` to use database 15
2. **Clear test data**: Tests automatically run `FLUSHALL` 
3. **Use authentication**: Set strong passwords for Redis
4. **Network isolation**: Use private networks for test Redis instances
5. **Monitor resources**: Ensure tests don't impact production

## Contributing

When adding new E2E tests:

1. Follow the existing test pattern
2. Use descriptive test names
3. Clean up test data
4. Add appropriate timeouts
5. Include helpful debug logging
6. Test both success and failure scenarios

## Debugging

To enable detailed debug logging during tests:

```go
// In your test, add this logger
replica.SetLogger(&debugLogger{t: t})

type debugLogger struct {
    t *testing.T
}

func (l *debugLogger) Debug(msg string, fields ...interface{}) {
    l.t.Logf("DEBUG: %s %v", msg, fields)
}
// ... implement other methods
```

This will show detailed replication protocol interactions and help diagnose issues.