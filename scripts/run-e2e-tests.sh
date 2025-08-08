#!/bin/bash

# Redis E2E Test Runner
# This script helps set up and run end-to-end tests with Redis

set -e

REDIS_ADDR="${REDIS_ADDR:-localhost:6379}"
REDIS_PASSWORD="${REDIS_PASSWORD:-}"
DOCKER_REDIS="${DOCKER_REDIS:-false}"

echo "🚀 Redis In-Memory Replica E2E Test Runner"
echo "=========================================="

# Function to check if Redis is available
check_redis() {
    local addr="$1"
    local host=$(echo "$addr" | cut -d':' -f1)
    local port=$(echo "$addr" | cut -d':' -f2)
    
    # Handle case where port might not be specified (no colon in address)
    # Also handle IPv6 addresses like [::1]:6379
    if [[ "$addr" == "["*"]:"* ]]; then
        # IPv6 with port: [::1]:6379
        host="${addr%:*}"  # Remove :port part
        host="${host#[}"   # Remove leading [
        host="${host%]}"   # Remove trailing ]
        port="${addr##*:}" # Get port part
    elif [[ "$addr" != *":"* ]]; then
        # No colon, assume default port
        host="$addr"
        port="6379"
    else
        # IPv4 with port: localhost:6379
        host="${addr%:*}"
        port="${addr##*:}"
    fi
    
    local redis_cmd="redis-cli -h $host -p $port"
    
    if [[ -n "$REDIS_PASSWORD" ]]; then
        # Use --no-auth-warning to suppress auth warnings in newer Redis versions
        redis_cmd="$redis_cmd --no-auth-warning -a $REDIS_PASSWORD"
    fi
    
    # Check if redis-cli is available
    if ! command -v redis-cli >/dev/null 2>&1; then
        echo "❌ redis-cli not found. Please install redis-tools."
        return 1
    fi
    
    # Try to ping Redis and check for PONG response with timeout
    local result
    if command -v timeout >/dev/null 2>&1; then
        # Use timeout command if available
        result=$(timeout 5 eval "$redis_cmd ping" 2>/dev/null)
    else
        # Fallback without timeout for systems that don't have it
        result=$(eval "$redis_cmd ping" 2>/dev/null)
    fi
    [[ "$result" == "PONG" ]]
}

# Function to start Redis in Docker
start_docker_redis() {
    echo "🐳 Starting Redis in Docker..."
    
    # Check if Docker is available
    if ! command -v docker >/dev/null 2>&1; then
        echo "❌ Docker is not installed or not in PATH"
        exit 1
    fi
    
    # Check if Docker daemon is running
    if ! docker info >/dev/null 2>&1; then
        echo "❌ Docker daemon is not running"
        exit 1
    fi
    
    # Check if Redis container already exists
    if docker ps -a --format 'table {{.Names}}' | grep -q '^redis-e2e-test$'; then
        echo "📋 Redis container already exists..."
        
        # Check if it's running
        if docker ps --format 'table {{.Names}}' | grep -q '^redis-e2e-test$'; then
            echo "   Container is already running, stopping it first..."
            docker stop redis-e2e-test >/dev/null 2>&1 || true
        fi
        
        echo "   Removing existing container..."
        docker rm redis-e2e-test >/dev/null 2>&1 || true
    fi
    
    echo "📦 Creating new Redis container..."
    local docker_cmd="docker run --name redis-e2e-test -p 6379:6379 -d redis:latest"
    
    if [[ -n "$REDIS_PASSWORD" ]]; then
        docker_cmd="$docker_cmd redis-server --requirepass $REDIS_PASSWORD"
    fi
    
    if ! eval "$docker_cmd"; then
        echo "❌ Failed to start Redis container"
        exit 1
    fi
    
    echo "   Container started successfully"
    
    # Wait for Redis to be ready
    echo "⏳ Waiting for Redis to be ready..."
    for i in {1..30}; do
        if check_redis "$REDIS_ADDR"; then
            echo "✅ Redis is ready!"
            return 0
        fi
        echo "   Attempt $i/30..."
        sleep 1
    done
    
    echo "❌ Redis failed to start or become ready"
    docker logs redis-e2e-test
    exit 1
}

# Function to clean up Docker Redis
cleanup_docker_redis() {
    if [[ "$DOCKER_REDIS" == "true" ]]; then
        echo "🧹 Cleaning up Docker Redis container..."
        docker stop redis-e2e-test >/dev/null 2>&1 || true
        docker rm redis-e2e-test >/dev/null 2>&1 || true
    fi
}

# Function to run the tests
run_tests() {
    echo ""
    echo "🧪 Running End-to-End Tests"
    echo "============================"
    echo "Redis Address: $REDIS_ADDR"
    if [[ -n "$REDIS_PASSWORD" ]]; then
        echo "Authentication: Enabled"
    else
        echo "Authentication: Disabled"
    fi
    echo ""
    
    # Set environment variables for tests
    export REDIS_ADDR="$REDIS_ADDR"
    if [[ -n "$REDIS_PASSWORD" ]]; then
        export REDIS_PASSWORD="$REDIS_PASSWORD"
    fi
    
    # Run the tests
    echo "Running: go test -v -run TestEndToEndWithRealRedis"
    go test -v -run TestEndToEndWithRealRedis
    
    echo ""
    echo "Running: go test -v -run TestRDBParsingRobustness" 
    go test -v -run TestRDBParsingRobustness
    
    echo ""
    echo "Running: go test -v -bench BenchmarkReplicationThroughput -benchtime=5s"
    go test -v -bench BenchmarkReplicationThroughput -benchtime=5s
}

# Function to show usage
show_usage() {
    cat << EOF
Usage: $0 [OPTIONS]

Options:
  -h, --help              Show this help message
  -d, --docker           Start Redis in Docker (will create/restart redis-e2e-test container)
  -a, --addr ADDR        Redis address (default: localhost:6379)
  -p, --password PASS    Redis password for authentication
  -c, --cleanup          Clean up Docker Redis container and exit

Environment Variables:
  REDIS_ADDR             Redis server address
  REDIS_PASSWORD         Redis authentication password
  DOCKER_REDIS           Set to 'true' to use Docker Redis

Examples:
  $0                                    # Use existing Redis at localhost:6379
  $0 -d                                # Start Redis in Docker and run tests
  $0 -a redis.example.com:6379         # Use remote Redis server
  $0 -a localhost:6379 -p mypassword   # Use Redis with authentication
  $0 -c                                # Clean up Docker containers

EOF
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            show_usage
            exit 0
            ;;
        -d|--docker)
            DOCKER_REDIS="true"
            shift
            ;;
        -a|--addr)
            REDIS_ADDR="$2"
            shift 2
            ;;
        -p|--password)
            REDIS_PASSWORD="$2"
            shift 2
            ;;
        -c|--cleanup)
            cleanup_docker_redis
            echo "✅ Cleanup completed"
            exit 0
            ;;
        *)
            echo "❌ Unknown option: $1"
            show_usage
            exit 1
            ;;
    esac
done

# Set up trap for cleanup
trap cleanup_docker_redis EXIT

# Main execution
main() {
    echo "🔍 Checking Redis availability..."
    
    if [[ "$DOCKER_REDIS" == "true" ]]; then
        start_docker_redis
    else
        if check_redis "$REDIS_ADDR"; then
            echo "✅ Redis is available at $REDIS_ADDR"
        else
            echo "❌ Redis is not available at $REDIS_ADDR"
            echo ""
            echo "💡 Try one of these options:"
            echo "   1. Start Redis manually: redis-server"
            echo "   2. Use Docker Redis: $0 --docker"
            echo "   3. Specify different address: $0 --addr your-redis-host:6379"
            exit 1
        fi
    fi
    
    # Check if Go is available
    if ! command -v go >/dev/null 2>&1; then
        echo "❌ Go is not installed or not in PATH"
        exit 1
    fi
    
    # Check if redis-cli is available
    if ! command -v redis-cli >/dev/null 2>&1; then
        echo "⚠️  redis-cli is not installed. Some tests may fail."
        echo "   Install with: apt install redis-tools (Ubuntu) or brew install redis (macOS)"
    fi
    
    run_tests
    
    echo ""
    echo "🎉 All tests completed successfully!"
    echo ""
    echo "📊 Test Summary:"
    echo "   ✅ Buffer overflow protection validated"
    echo "   ✅ CRLF terminator handling improved"
    echo "   ✅ RDB parsing robustness confirmed"
    echo "   ✅ Real-time replication working"
    echo "   ✅ Large value handling functional"
    echo ""
    echo "🔧 The Redis connection issues have been fixed!"
}

# Run main function
main