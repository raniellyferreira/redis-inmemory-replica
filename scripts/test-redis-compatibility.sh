#!/bin/bash
# Script to test Redis 7.x compatibility locally
# Usage: ./test-redis-compatibility.sh [version]

set -e

REDIS_VERSION=${1:-"7.2.4"}
CONTAINER_NAME="redis-test-${REDIS_VERSION//\./-}"

echo "🚀 Testing Redis ${REDIS_VERSION} compatibility..."

# Cleanup function
cleanup() {
    echo "🧹 Cleaning up..."
    docker stop "${CONTAINER_NAME}" 2>/dev/null || true
    docker rm "${CONTAINER_NAME}" 2>/dev/null || true
}

# Set up trap to cleanup on exit
trap cleanup EXIT

# Start Redis with specified version
echo "📦 Starting Redis ${REDIS_VERSION}..."
docker run -d \
    --name "${CONTAINER_NAME}" \
    -p 6379:6379 \
    "redis:${REDIS_VERSION}-alpine" \
    redis-server --save 60 1 --appendonly yes

# Wait for Redis to be ready
echo "⏳ Waiting for Redis to be ready..."
for i in {1..30}; do
    if docker exec "${CONTAINER_NAME}" redis-cli ping 2>/dev/null | grep -q PONG; then
        echo "✅ Redis is ready!"
        break
    fi
    echo "   Attempt $i/30: waiting for Redis..."
    sleep 1
done

# Verify Redis version
ACTUAL_VERSION=$(docker exec "${CONTAINER_NAME}" redis-cli INFO SERVER | grep redis_version | cut -d: -f2 | tr -d '\r')
echo "🔍 Redis version: ${ACTUAL_VERSION}"

if [[ "${ACTUAL_VERSION}" != "${REDIS_VERSION}"* ]]; then
    echo "❌ ERROR: Expected Redis ${REDIS_VERSION}, but got ${ACTUAL_VERSION}"
    exit 1
fi

# Run specific tests for the version
echo "🧪 Running Redis ${REDIS_VERSION} compatibility tests..."

export REDIS_ADDR=localhost:6379
export REDIS_VERSION="${REDIS_VERSION}"

# Test 1: Basic E2E
echo "   Test 1: Basic End-to-End..."
if go test -v -run TestEndToEndWithRealRedis -timeout 60s; then
    echo "   ✅ Basic E2E test passed"
else
    echo "   ❌ Basic E2E test failed"
    exit 1
fi

# Test 2: RDB Parsing
echo "   Test 2: RDB Parsing..."
if go test -v -run TestRDBParsingRobustness -timeout 60s; then
    echo "   ✅ RDB parsing test passed"
else
    echo "   ❌ RDB parsing test failed"
    exit 1
fi

# Test 3: Full Sync + Incremental
echo "   Test 3: Full Sync + Incremental..."
if go test -v -run TestFullSyncAndIncremental -timeout 60s; then
    echo "   ✅ Full sync + incremental test passed"
else
    echo "   ❌ Full sync + incremental test failed"
    exit 1
fi

# Test 4: Redis 7.x Features
echo "   Test 4: Redis 7.x Features..."
if go test -v -run TestRedis7xFeatures -timeout 60s; then
    echo "   ✅ Redis 7.x features test passed"
else
    echo "   ❌ Redis 7.x features test failed"
    exit 1
fi

echo ""
echo "🎉 All Redis ${REDIS_VERSION} compatibility tests passed!"
echo "✅ No 'invalid special string encoding: 33' errors detected"
echo "✅ RDB parsing robust for Redis 7.x"
echo "✅ Full sync and incremental replication working"

# Show Redis info for reference
echo ""
echo "📊 Redis ${REDIS_VERSION} Information:"
docker exec "${CONTAINER_NAME}" redis-cli INFO SERVER | grep -E "(redis_version|arch_bits|multiplexing_api|gcc_version)"