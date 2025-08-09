# Local Testing with act

This guide explains how to run the Redis 7.x compatibility tests locally using [act](https://github.com/nektos/act).

## Prerequisites

1. **Install act**: 
   ```bash
   # macOS
   brew install act
   
   # Linux
   curl https://raw.githubusercontent.com/nektos/act/master/install.sh | sudo bash
   
   # Windows (using Chocolatey)
   choco install act-cli
   ```

2. **Install Docker**: act requires Docker to run GitHub Actions locally.

3. **Configure act**: The repository includes a `.actrc` configuration file that sets up proper defaults.

## Running Tests

### Option 1: Run All Redis Compatibility Tests

```bash
# Run the full Redis 7.x compatibility workflow
act -W .github/workflows/redis-compatibility.yml

# Run specific job
act -W .github/workflows/redis-compatibility.yml -j redis-multi-version
```

### Option 2: Run Tests for Specific Redis Version

```bash
# Test Redis 7.0.15
act -W .github/workflows/redis-compatibility.yml -j redis-multi-version --matrix redis-version:7.0.15

# Test Redis 7.2.4  
act -W .github/workflows/redis-compatibility.yml -j redis-multi-version --matrix redis-version:7.2.4

# Test Redis 7.4.0
act -W .github/workflows/redis-compatibility.yml -j redis-multi-version --matrix redis-version:7.4.0
```

### Option 3: Run Authentication Tests

```bash
# Test with authentication
act -W .github/workflows/redis-compatibility.yml -j redis-multi-version-auth
```

### Option 4: Manual Testing Script

```bash
# Test specific Redis version locally
./scripts/test-redis-compatibility.sh 7.2.4

# Test all versions
for version in 7.0.15 7.2.4 7.4.0; do
    echo "Testing Redis $version..."
    ./scripts/test-redis-compatibility.sh "$version"
done
```

## Configuration

The `.actrc` file configures act with these defaults:

```
# Use Ubuntu image compatible with GitHub Actions
-P ubuntu-latest=ghcr.io/catthehacker/ubuntu:act-latest

# Network settings for Redis connectivity
--network host

# Resource limits
--memory=4g
--cpus=2

# Default environment
--env REDIS_ADDR=localhost:6379
```

## Troubleshooting

### Common Issues

#### 1. Docker Network Issues

If tests fail to connect to Redis:

```bash
# Run with custom network
act --network host -W .github/workflows/redis-compatibility.yml
```

#### 2. Memory/Resource Limits

If containers fail to start:

```bash
# Increase resources
act --memory=8g --cpus=4 -W .github/workflows/redis-compatibility.yml
```

#### 3. Platform Issues

If you encounter platform-specific issues:

```bash
# Use specific platform
act -P ubuntu-latest=ubuntu:20.04 -W .github/workflows/redis-compatibility.yml
```

### Debug Mode

Run with verbose output for debugging:

```bash
act --verbose -W .github/workflows/redis-compatibility.yml
```

### View Container Logs

```bash
# List running containers
docker ps

# View logs from Redis container
docker logs redis-test

# View logs from act container
docker logs <act-container-id>
```

## Manual Testing

For manual testing without act:

```bash
# 1. Start Redis
docker run -d --name redis-test -p 6379:6379 redis:7.2.4-alpine

# 2. Run tests
export REDIS_ADDR=localhost:6379
go test -v -run TestRedis7xFeatures

# 3. Cleanup
docker stop redis-test && docker rm redis-test
```

## Expected Results

When tests pass, you should see:

- ✅ All Redis 7.x compatibility tests passed
- ✅ No 'invalid special string encoding: 33' errors detected  
- ✅ RDB parsing robust for Redis 7.x
- ✅ Full sync and incremental replication working

## Performance Notes

- Local testing typically takes 5-10 minutes per Redis version
- Memory usage peaks around 2-4GB during tests
- Network traffic is minimal (localhost only)

## CI vs Local Differences

| Aspect | GitHub Actions | Local (act) |
|--------|---------------|-------------|
| Performance | Optimized runners | Depends on local machine |
| Networking | Isolated containers | Host networking |
| Storage | Ephemeral | Local Docker volumes |
| Artifacts | GitHub storage | Local filesystem |

## Contributing

When adding new tests:

1. **Test locally first**: Always test with act before pushing
2. **Update matrix**: Add new Redis versions to the matrix in workflows
3. **Document changes**: Update this guide for new requirements
4. **Verify cleanup**: Ensure containers are properly cleaned up

## Limitations

Current limitations of local testing:

- Cannot test GitHub-specific features (artifacts, secrets)
- Networking behavior may differ slightly
- Performance characteristics may vary
- Some GitHub Actions features are not fully supported

For complete validation, always run tests in both local (act) and CI environments.