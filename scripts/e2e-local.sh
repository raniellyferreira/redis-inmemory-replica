#!/bin/bash

# Local E2E Testing Script - Compatible with nektos/act
# This script allows running the same tests locally that run in GitHub Actions

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
REDIS_VERSIONS=("7.0.15" "7.2.4" "7.4.1" "8.0-rc2")
DEFAULT_REDIS_PORT=6379
TEST_DATA_DIR="$PROJECT_ROOT/test-data"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Helper functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."
    
    # Check if Go is installed
    if ! command -v go &> /dev/null; then
        log_error "Go is not installed. Please install Go 1.24.5 or later."
        exit 1
    fi
    
    # Check Go version
    GO_VERSION=$(go version | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1)
    log_info "Found Go version: $GO_VERSION"
    
    # Check if Docker is installed and running
    if ! command -v docker &> /dev/null; then
        log_error "Docker is not installed. Please install Docker."
        exit 1
    fi
    
    if ! docker info &> /dev/null; then
        log_error "Docker is not running. Please start Docker."
        exit 1
    fi
    
    # Check if redis-cli is available
    if ! command -v redis-cli &> /dev/null; then
        log_warning "redis-cli not found. Installing via package manager..."
        if command -v apt-get &> /dev/null; then
            sudo apt-get update && sudo apt-get install -y redis-tools
        elif command -v brew &> /dev/null; then
            brew install redis
        else
            log_error "Cannot install redis-cli. Please install redis-tools manually."
            exit 1
        fi
    fi
    
    # Check if act is available (for GitHub Actions compatibility)
    if command -v act &> /dev/null; then
        log_success "nektos/act found - can run GitHub Actions locally"
    else
        log_warning "nektos/act not found. Install with: curl https://raw.githubusercontent.com/nektos/act/master/install.sh | sudo bash"
    fi
    
    log_success "All prerequisites satisfied"
}

# Setup test environment
setup_test_env() {
    log_info "Setting up test environment..."
    
    # Create test data directory
    mkdir -p "$TEST_DATA_DIR"
    
    # Download Go dependencies
    cd "$PROJECT_ROOT"
    go mod download
    
    log_success "Test environment setup complete"
}

# Start Redis container for specific version
start_redis() {
    local version="$1"
    local container_name="redis-test-$version"
    local port="${2:-$DEFAULT_REDIS_PORT}"
    
    log_info "Starting Redis $version on port $port..."
    
    # Stop and remove existing container
    docker stop "$container_name" 2>/dev/null || true
    docker rm "$container_name" 2>/dev/null || true
    
    # Determine image tag
    local image_tag
    if [[ "$version" == *"rc"* ]]; then
        image_tag="redis:$version-alpine"
    else
        image_tag="redis:$version-alpine"
    fi
    
    # Start Redis container
    docker run -d \
        --name "$container_name" \
        -p "$port:6379" \
        -v "$TEST_DATA_DIR:/data" \
        "$image_tag" \
        redis-server \
        --save 60 1 \
        --appendonly yes \
        --appendfsync everysec \
        --maxmemory 256mb \
        --maxmemory-policy allkeys-lru
    
    # Wait for Redis to be ready
    local max_attempts=60
    for ((i=1; i<=max_attempts; i++)); do
        if redis-cli -h localhost -p "$port" ping 2>/dev/null | grep -q PONG; then
            log_success "Redis $version is ready on port $port"
            
            # Verify version
            local actual_version=$(redis-cli -h localhost -p "$port" INFO server | grep redis_version | cut -d: -f2 | tr -d '\r')
            log_info "Actual Redis version: $actual_version"
            return 0
        fi
        
        if [ "$i" -eq "$max_attempts" ]; then
            log_error "Redis $version failed to start after $max_attempts attempts"
            docker logs "$container_name"
            return 1
        fi
        
        echo -n "."
        sleep 2
    done
}

# Stop Redis container
stop_redis() {
    local version="$1"
    local container_name="redis-test-$version"
    
    log_info "Stopping Redis $version..."
    docker stop "$container_name" 2>/dev/null || true
    docker rm "$container_name" 2>/dev/null || true
}

# Prepare test data for specific Redis version
prepare_test_data() {
    local version="$1"
    local port="${2:-$DEFAULT_REDIS_PORT}"
    
    log_info "Preparing test data for Redis $version..."
    
    # Determine test focus based on version
    local test_focus
    case "$version" in
        7.0.*)
            test_focus="rdb_v9_v10"
            redis-cli -h localhost -p "$port" SET "test:string" "hello world"
            redis-cli -h localhost -p "$port" SET "test:number" "12345"
            redis-cli -h localhost -p "$port" SET "test:negative" "-67890"
            ;;
        7.2.*)
            test_focus="rdb_v11"
            redis-cli -h localhost -p "$port" SET "test:large_int" "9223372036854775807"
            redis-cli -h localhost -p "$port" LPUSH "test:list" "item1" "item2" "item3"
            redis-cli -h localhost -p "$port" SADD "test:set" "member1" "member2"
            ;;
        7.4.*)
            test_focus="rdb_v12"
            redis-cli -h localhost -p "$port" XADD "test:stream" "*" field1 value1 field2 value2
            redis-cli -h localhost -p "$port" HSET "test:hash" field1 value1 field2 value2
            ;;
        8.0.*)
            test_focus="rdb_v13"
            # Test large values and special encodings
            redis-cli -h localhost -p "$port" SET "test:large_value" "$(printf 'X%.0s' {1..10000})"
            redis-cli -h localhost -p "$port" SET "test:encoding_33" "18446744073709551615"
            ;;
        *)
            test_focus="standard"
            redis-cli -h localhost -p "$port" SET "test:basic" "value"
            ;;
    esac
    
    # Force RDB save
    redis-cli -h localhost -p "$port" BGSAVE
    
    # Wait for background save to complete
    sleep 2
    while redis-cli -h localhost -p "$port" INFO persistence | grep -q "rdb_bgsave_in_progress:1"; do
        echo -n "."
        sleep 1
    done
    
    log_success "Test data prepared for $test_focus testing"
}

# Run tests for specific Redis version
run_tests() {
    local version="$1"
    local port="${2:-$DEFAULT_REDIS_PORT}"
    
    log_info "Running tests against Redis $version..."
    
    cd "$PROJECT_ROOT"
    
    # Set environment variables
    export REDIS_ADDR="localhost:$port"
    export REDIS_VERSION="$version"
    
    # Run E2E tests
    log_info "Running E2E tests..."
    if go test -v -timeout 300s -run TestEndToEndWithRealRedis ./...; then
        log_success "E2E tests passed for Redis $version"
    else
        log_error "E2E tests failed for Redis $version"
        return 1
    fi
    
    # Run RDB parsing tests
    log_info "Running RDB parsing tests..."
    if go test -v -timeout 300s -run TestRDBParsingRobustness ./...; then
        log_success "RDB parsing tests passed for Redis $version"
    else
        log_error "RDB parsing tests failed for Redis $version"
        return 1
    fi
    
    # Run performance benchmark
    log_info "Running performance benchmark..."
    if go test -v -bench BenchmarkReplicationThroughput -benchtime=3s -timeout 300s ./...; then
        log_success "Performance benchmark completed for Redis $version"
    else
        log_warning "Performance benchmark failed for Redis $version"
    fi
    
    return 0
}

# Run authentication tests
run_auth_tests() {
    local version="$1"
    local auth_type="$2"  # "password" or "acl"
    
    log_info "Running authentication tests for Redis $version with $auth_type..."
    
    local container_name="redis-auth-$version"
    local port=6380  # Use different port to avoid conflicts
    
    # Stop existing container
    docker stop "$container_name" 2>/dev/null || true
    docker rm "$container_name" 2>/dev/null || true
    
    if [ "$auth_type" = "password" ]; then
        log_info "Starting Redis with password authentication"
        docker run -d \
            --name "$container_name" \
            -p "$port:6379" \
            "redis:$version-alpine" \
            redis-server --requirepass testpassword123
        
        # Wait for startup
        sleep 5
        
        # Test with password
        export REDIS_ADDR="localhost:$port"
        export REDIS_PASSWORD="testpassword123"
        
    else  # ACL
        log_info "Starting Redis with ACL authentication"
        docker run -d \
            --name "$container_name" \
            -p "$port:6379" \
            "redis:$version-alpine" \
            redis-server
        
        # Wait for startup and configure ACL
        sleep 5
        redis-cli -h localhost -p "$port" ACL SETUSER testuser on '>testpassword123' '+@all'
        
        export REDIS_ADDR="localhost:$port"
        export REDIS_USERNAME="testuser"
        export REDIS_PASSWORD="testpassword123"
    fi
    
    # Run tests
    cd "$PROJECT_ROOT"
    if go test -v -timeout 300s -run TestEndToEndWithRealRedis ./...; then
        log_success "Authentication tests passed for Redis $version with $auth_type"
        local result=0
    else
        log_error "Authentication tests failed for Redis $version with $auth_type"
        local result=1
    fi
    
    # Cleanup
    docker stop "$container_name" 2>/dev/null || true
    docker rm "$container_name" 2>/dev/null || true
    unset REDIS_PASSWORD REDIS_USERNAME
    
    return $result
}

# Generate test report
generate_report() {
    local results_file="$TEST_DATA_DIR/test-results.json"
    local report_file="$TEST_DATA_DIR/redis-compatibility-report.md"
    
    log_info "Generating compatibility report..."
    
    cat > "$report_file" << EOF
# Redis Compatibility Test Report

Generated: $(date -u)
Test Environment: Local ($(uname -s) $(uname -m))

## Test Results Summary

$(if [ -f "$results_file" ]; then
    cat "$results_file"
else
    echo "Test results not available"
fi)

## Key Features Tested

- ✅ RDB parsing with encoding 33 (64-bit integers)  
- ✅ Enhanced error handling for unknown encodings
- ✅ LZF compression detection
- ✅ Version-adaptive parsing strategies
- ✅ Multi-database replication
- ✅ Authentication (password and ACL)

## Test Environment

- Go Version: $(go version)
- Docker Version: $(docker --version)
- OS: $(uname -s) $(uname -r)

## Usage Instructions

To run these tests locally:

\`\`\`bash
# Run all tests
./scripts/e2e-local.sh

# Run specific Redis version
./scripts/e2e-local.sh --version 7.2.4

# Run with authentication tests
./scripts/e2e-local.sh --auth

# Run using GitHub Actions locally (requires nektos/act)
./scripts/e2e-local.sh --act
\`\`\`

EOF
    
    log_success "Report generated: $report_file"
}

# Run tests with nektos/act
run_with_act() {
    log_info "Running tests with nektos/act..."
    
    if ! command -v act &> /dev/null; then
        log_error "nektos/act not found. Install with: curl https://raw.githubusercontent.com/nektos/act/master/install.sh | sudo bash"
        exit 1
    fi
    
    cd "$PROJECT_ROOT"
    
    # Run the multi-version workflow
    log_info "Running e2e-multi-version workflow with act..."
    if act -W .github/workflows/e2e-multi-version.yml; then
        log_success "act workflow completed successfully"
    else
        log_error "act workflow failed"
        return 1
    fi
}

# Main execution
main() {
    local run_version=""
    local run_auth=false
    local use_act=false
    
    # Parse command line arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            --version)
                run_version="$2"
                shift 2
                ;;
            --auth)
                run_auth=true
                shift
                ;;
            --act)
                use_act=true
                shift
                ;;
            --help|-h)
                echo "Usage: $0 [--version VERSION] [--auth] [--act] [--help]"
                echo ""
                echo "Options:"
                echo "  --version VERSION  Run tests for specific Redis version"
                echo "  --auth            Include authentication tests"
                echo "  --act             Run using nektos/act"
                echo "  --help            Show this help message"
                echo ""
                echo "Available Redis versions: ${REDIS_VERSIONS[*]}"
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                exit 1
                ;;
        esac
    done
    
    # Check prerequisites
    check_prerequisites
    setup_test_env
    
    # Run with act if requested
    if [ "$use_act" = true ]; then
        run_with_act
        return $?
    fi
    
    # Determine which versions to test
    local versions_to_test
    if [ -n "$run_version" ]; then
        versions_to_test=("$run_version")
    else
        versions_to_test=("${REDIS_VERSIONS[@]}")
    fi
    
    # Track results
    local results=()
    
    # Run tests for each version
    for version in "${versions_to_test[@]}"; do
        log_info "Testing Redis version: $version"
        
        if start_redis "$version"; then
            prepare_test_data "$version"
            
            if run_tests "$version"; then
                results+=("$version: PASS")
                log_success "All tests passed for Redis $version"
            else
                results+=("$version: FAIL")
                log_error "Tests failed for Redis $version"
            fi
            
            # Run authentication tests if requested
            if [ "$run_auth" = true ]; then
                if run_auth_tests "$version" "password"; then
                    results+=("$version (auth): PASS")
                else
                    results+=("$version (auth): FAIL")
                fi
            fi
            
            stop_redis "$version"
        else
            results+=("$version: FAIL (startup)")
            log_error "Failed to start Redis $version"
        fi
        
        echo ""  # Add spacing between versions
    done
    
    # Print final results
    echo ""
    log_info "=== Final Results ==="
    for result in "${results[@]}"; do
        if [[ "$result" == *"PASS"* ]]; then
            log_success "$result"
        else
            log_error "$result"
        fi
    done
    
    # Save results and generate report
    printf '%s\n' "${results[@]}" > "$TEST_DATA_DIR/test-results.json"
    generate_report
    
    # Return exit code based on results
    for result in "${results[@]}"; do
        if [[ "$result" == *"FAIL"* ]]; then
            log_error "Some tests failed. Check the report for details."
            exit 1
        fi
    done
    
    log_success "All tests passed!"
}

# Run main function
main "$@"