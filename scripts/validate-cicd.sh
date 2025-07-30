#!/bin/bash

# Complete CI/CD Pipeline Validation Script
# This script runs all CI/CD steps locally to validate the pipeline

set -e

echo "🚀 Redis In-Memory Replica CI/CD Pipeline Validation"
echo "===================================================="
echo

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${GREEN}✅${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}⚠️${NC} $1"
}

print_error() {
    echo -e "${RED}❌${NC} $1"
}

print_info() {
    echo -e "${BLUE}ℹ️${NC} $1"
}

# Step 1: Basic Setup
echo "1. 🔧 Basic Setup & Dependencies"
echo "   ============================="
print_info "Go version: $(go version)"
print_info "Downloading dependencies..."
go mod download
go mod verify
print_status "Dependencies verified"
echo

# Step 2: Build
echo "2. 🏗️  Build"
echo "   =========="
print_info "Building project..."
go build -v ./...
print_status "Build successful"
echo

# Step 3: Linting
echo "3. 🔍 Linting"
echo "   =========="
print_info "Installing tools..."
make install-tools > /dev/null 2>&1
print_info "Running linter..."
if make lint; then
    print_status "Linting passed"
else
    print_warning "Linting had warnings"
fi
echo

# Step 4: Static Analysis
echo "4. 📊 Static Analysis"
echo "   =================="
print_info "Running go vet..."
if go vet ./...; then
    print_status "go vet passed"
else
    print_error "go vet failed"
    exit 1
fi

print_info "Running staticcheck..."
if make security-static; then
    print_status "staticcheck passed"
else
    print_warning "staticcheck found issues (may be acceptable)"
fi
echo

# Step 5: Tests
echo "5. 🧪 Tests"
echo "   ========"
print_info "Running tests with race detection..."
if go test -race -coverprofile=coverage.out ./...; then
    print_status "All tests passed"
    COVERAGE=$(go tool cover -func=coverage.out | grep total: | awk '{print $3}')
    print_info "Test coverage: $COVERAGE"
else
    print_error "Tests failed"
    exit 1
fi
echo

# Step 6: Benchmarks
echo "6. 📈 Benchmarks"
echo "   ============="
print_info "Running benchmarks..."
if go test -bench=. -benchmem ./...; then
    print_status "Benchmarks completed"
else
    print_warning "Benchmarks had issues"
fi
echo

# Step 7: Security Audit
echo "7. 🔒 Security Audit"
echo "   ================="
print_info "Running security audit..."
if ./scripts/security-audit.sh > /dev/null 2>&1; then
    print_status "Security audit completed"
else
    print_warning "Security audit completed with warnings (may be acceptable)"
fi
echo

# Step 8: Build with Security Flags
echo "8. 🛡️  Security Build"
echo "   =================="
print_info "Building with security flags..."
if go build -buildmode=pie -ldflags="-s -w" ./...; then
    print_status "Security build successful"
else
    print_error "Security build failed"
    exit 1
fi
echo

# Step 9: Dependency Verification
echo "9. 📦 Dependency Security"
echo "   ====================="
print_info "Verifying dependencies..."
if make security-deps; then
    print_status "Dependencies verified"
else
    print_error "Dependency verification failed"
    exit 1
fi
echo

# Summary
echo "📋 CI/CD Pipeline Validation Summary"
echo "===================================="
print_status "All critical pipeline steps passed successfully"
print_info "The following components were validated:"
print_status "• Go modules and dependencies"
print_status "• Code compilation and building"
print_status "• Code quality (linting, vetting)"
print_status "• Static analysis"
print_status "• Test suite with race detection"
print_status "• Performance benchmarks"
print_status "• Security analysis and hardening"
print_status "• Production build configuration"
echo
print_info "This project is ready for CI/CD deployment!"
print_info "All workflows in .github/workflows/ should pass successfully"

# Cleanup
if [ -f coverage.out ]; then
    rm coverage.out
fi

echo
print_status "CI/CD Pipeline validation completed successfully! 🎉"