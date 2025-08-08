# End-to-End Testing Guide

## Overview

This guide explains how to run comprehensive end-to-end tests for the redis-inmemory-replica library across multiple Redis versions (7.0, 7.2, 7.4).

## Quick Start

```bash
# Clone the repository
git clone https://github.com/raniellyferreira/redis-inmemory-replica.git
cd redis-inmemory-replica

# Run all e2e tests locally
./scripts/e2e-local.sh

# Run tests for specific Redis version
./scripts/e2e-local.sh --version 7.2.4

# Include authentication tests
./scripts/e2e-local.sh --auth

# Run using GitHub Actions locally (requires nektos/act)
./scripts/e2e-local.sh --act
```

## Prerequisites

### Required Software

1. **Go 1.23+**
   ```bash
   # Check Go version
   go version
   
   # Install if needed (example for Linux)
   wget https://go.dev/dl/go1.23.linux-amd64.tar.gz
   sudo tar -C /usr/local -xzf go1.23.linux-amd64.tar.gz
   export PATH=$PATH:/usr/local/go/bin
   ```

2. **Docker**
   ```bash
   # Check Docker
   docker --version
   docker info
   
   # Install Docker (Ubuntu/Debian)
   curl -fsSL https://get.docker.com -o get-docker.sh
   sudo sh get-docker.sh
   sudo usermod -aG docker $USER
   ```

3. **Redis CLI Tools**
   ```bash
   # Ubuntu/Debian
   sudo apt-get update && sudo apt-get install -y redis-tools
   
   # macOS
   brew install redis
   
   # CentOS/RHEL
   sudo yum install redis
   ```

4. **nektos/act (Optional - for local GitHub Actions)**
   ```bash
   # Install act
   curl https://raw.githubusercontent.com/nektos/act/master/install.sh | sudo bash
   
   # Or with Homebrew
   brew install act
   ```

## Test Scenarios

### 1. Multi-Version Compatibility

Tests the library against different Redis versions to ensure compatibility:

| Redis Version | RDB Version | Key Features Tested |
|---------------|-------------|-------------------|
| 7.0.15        | 9, 10       | Basic compatibility, standard encodings |
| 7.2.4         | 10, 11      | Extended encodings, functions support |
| 7.4.1         | 11, 12      | Stream improvements, list optimizations |

### 2. RDB Format Testing

Specific test cases for RDB format variations:

```bash
# Test RDB v9/v10 (Redis 7.0)
./scripts/e2e-local.sh --version 7.0.15

# Test RDB v11 with functions (Redis 7.2)  
./scripts/e2e-local.sh --version 7.2.4

# Test RDB v12 with streams (Redis 7.4)
./scripts/e2e-local.sh --version 7.4.1

# Test latest stable Redis 7.4
./scripts/e2e-local.sh --version 7.4.1

# Note: Redis 8.0+ testing requires manual setup
# RDB parsing supports encoding 33 and future Redis versions
```

### 3. Authentication Testing

Tests both password and ACL-based authentication:

```bash
# Run authentication tests
./scripts/e2e-local.sh --auth

# Manual authentication test
docker run -d --name redis-auth -p 6379:6379 redis:7.2-alpine redis-server --requirepass mypassword
export REDIS_PASSWORD=mypassword
go test -v -run TestEndToEndWithRealRedis
```

## Running Tests

### Local Execution

#### Run All Tests
```bash
# Full test suite across all Redis versions
./scripts/e2e-local.sh

# Expected output:
# [INFO] Checking prerequisites...
# [SUCCESS] All prerequisites satisfied
# [INFO] Testing Redis version: 7.0.15
# [SUCCESS] Redis 7.0.15 is ready on port 6379
# [SUCCESS] All tests passed for Redis 7.0.15
# ...
# [SUCCESS] All tests passed!
```

#### Run Specific Version
```bash
# Test only Redis 7.2.4
./scripts/e2e-local.sh --version 7.2.4

# Test with authentication
./scripts/e2e-local.sh --version 7.2.4 --auth
```

### GitHub Actions Integration

#### Using nektos/act (Local)

```bash
# Install act
curl https://raw.githubusercontent.com/nektos/act/master/install.sh | sudo bash

# Run GitHub Actions workflow locally
./scripts/e2e-local.sh --act

# Or run specific workflow
act -W .github/workflows/e2e-multi-version.yml
```

#### GitHub Actions (CI/CD)

The workflows run automatically on:
- Push to `main` or `develop` branches
- Pull requests
- Manual workflow dispatch

```yaml
# Trigger specific Redis version test
workflow_dispatch:
  inputs:
    redis_version:
      description: 'Redis version to test'
      required: false
      type: string
```

### Manual Testing

#### Step-by-step Manual Testing

1. **Start Redis**
   ```bash
   docker run -d --name redis-test -p 6379:6379 redis:7.2.4-alpine
   ```

2. **Prepare Test Data**
   ```bash
   redis-cli SET "test:key1" "value1"
   redis-cli SET "test:large_int" "9223372036854775807"
   redis-cli LPUSH "test:list" "item1" "item2"
   redis-cli BGSAVE
   ```

3. **Run Library Tests**
   ```bash
   export REDIS_ADDR=localhost:6379
   go test -v -run TestEndToEndWithRealRedis
   go test -v -run TestRDBParsingRobustness
   ```

4. **Cleanup**
   ```bash
   docker stop redis-test
   docker rm redis-test
   ```

## Test Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `REDIS_ADDR` | Redis server address | `localhost:6379` |
| `REDIS_PASSWORD` | Redis password (if auth enabled) | - |
| `REDIS_USERNAME` | Redis username (for ACL) | - |
| `REDIS_VERSION` | Redis version being tested | - |
| `TEST_FOCUS` | Specific test focus area | - |

### Test Focus Areas

| Focus | Description | Redis Versions |
|-------|-------------|----------------|
| `rdb_v9_v10` | Basic RDB compatibility | 7.0.x |
| `rdb_v11` | Extended data types | 7.2.x |
| `rdb_v12` | Stream optimizations | 7.4.x |

## Troubleshooting

### Common Issues

#### 1. Docker Permission Denied
```bash
# Add user to docker group
sudo usermod -aG docker $USER
# Logout and login again, or:
newgrp docker
```

#### 2. Redis Connection Refused
```bash
# Check if Redis container is running
docker ps | grep redis

# Check Redis logs
docker logs redis-test

# Verify port is not in use
netstat -tulpn | grep 6379
```

#### 3. Go Module Issues
```bash
# Clean module cache
go clean -modcache

# Re-download dependencies
go mod download

# Verify module integrity
go mod verify
```

#### 4. Test Timeouts
```bash
# Increase test timeout
go test -timeout 600s -v -run TestEndToEndWithRealRedis

# Check system resources
docker stats
free -h
```

### Debug Mode

Enable verbose logging for debugging:

```bash
# Run with debug output
go test -v -run TestEndToEndWithRealRedis 2>&1 | tee test-debug.log

# Enable Go race detection
go test -race -v -run TestEndToEndWithRealRedis

# Enable Redis debug logging (in Redis container)
docker exec redis-test redis-cli CONFIG SET loglevel debug
docker logs -f redis-test
```

### Test Data Analysis

After running tests, check generated data:

```bash
# View test results
cat test-data/test-results.json

# View compatibility report
cat test-data/redis-compatibility-report.md

# Check Redis data
redis-cli -h localhost -p 6379 KEYS "*"
redis-cli -h localhost -p 6379 INFO persistence
```

## Performance Benchmarking

### Running Benchmarks

```bash
# Standard benchmark
go test -bench BenchmarkReplicationThroughput -benchtime=10s

# Memory profile
go test -bench BenchmarkReplicationThroughput -memprofile=mem.prof

# CPU profile  
go test -bench BenchmarkReplicationThroughput -cpuprofile=cpu.prof

# Analyze profiles
go tool pprof mem.prof
go tool pprof cpu.prof
```

### Expected Performance

| Metric | Redis 7.0 | Redis 7.2 | Redis 7.4 |
|--------|-----------|-----------|-----------|
| Sync Speed (1GB RDB) | ~30s | ~25s | ~20s |
| Command Throughput | >50k/s | >60k/s | >70k/s |
| Memory Overhead | <20% | <18% | <15% |

## Continuous Integration

### GitHub Actions Setup

The repository includes comprehensive GitHub Actions workflows:

1. **e2e-multi-version.yml** - Multi-version testing
2. **e2e.yml** - Standard e2e tests  
3. **test.yml** - Unit tests
4. **security-audit.yml** - Security scanning

### Local CI Simulation

```bash
# Simulate full CI pipeline
./scripts/e2e-local.sh --act

# Run individual workflow jobs
act -j e2e-matrix
act -j e2e-auth
act -j compatibility-report
```

## Contributing

### Adding New Test Cases

1. **Add Redis Version**
   ```bash
   # Update REDIS_VERSIONS in scripts/e2e-local.sh when new stable versions become available
   REDIS_VERSIONS=("7.0.15" "7.2.4" "7.4.1")
   ```

2. **Create Version-Specific Tests**
   ```go
   // Add to e2e_test.go for new Redis features
   func TestRedisNewFeatures(t *testing.T) {
       // Test new Redis-specific features when available
   }
   ```

3. **Update Workflows**
   ```yaml
   # Add to .github/workflows/e2e-multi-version.yml when Docker images are available
   redis_version:
     - "7.x.x"  # Add new stable versions
   ```

### Test Guidelines

- Keep tests deterministic and repeatable
- Use realistic data sizes and patterns
- Test both success and failure scenarios
- Include performance regression tests
- Maintain backward compatibility

## Reporting Issues

When reporting test failures, include:

1. **Environment Information**
   ```bash
   # Generate environment report
   ./scripts/e2e-local.sh --version 7.2.4 2>&1 | tee issue-report.log
   
   # Include system info
   uname -a
   go version
   docker --version
   ```

2. **Test Logs**
   - Complete test output
   - Redis container logs
   - Performance metrics (if relevant)

3. **Reproduction Steps**
   - Exact commands used
   - Redis configuration
   - Expected vs actual behavior

## Conclusion

The e2e testing framework provides comprehensive coverage across Redis versions and deployment scenarios. Regular testing ensures compatibility and catches regressions early in development.

For questions or issues, please open a GitHub issue with the `testing` label.