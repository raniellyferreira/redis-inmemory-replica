#!/bin/bash

# Redis In-Memory Replica Security Audit Script
# This script performs a comprehensive security audit of the codebase

set -e

echo "🔒 Redis In-Memory Replica Security Audit"
echo "=========================================="
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

# Check if tools are available
check_tool() {
    if command -v $1 &> /dev/null; then
        return 0
    else
        return 1
    fi
}

# Install security tools if needed
install_tools() {
    echo "Installing security audit tools..."
    
    if ! check_tool govulncheck; then
        print_info "Installing govulncheck..."
        go install golang.org/x/vuln/cmd/govulncheck@latest
    fi
    
    if ! check_tool staticcheck; then
        print_info "Installing staticcheck..."
        go install honnef.co/go/tools/cmd/staticcheck@latest
    fi
}

# Check for Go installation
if ! check_tool go; then
    print_error "Go is not installed. Please install Go to run security audits."
    exit 1
fi

print_info "Go version: $(go version)"
echo

# Install tools
install_tools
echo

# 1. Vulnerability Scanning
echo "1. 🛡️  Vulnerability Scanning"
echo "   =========================="

if check_tool govulncheck; then
    print_info "Running govulncheck..."
    if govulncheck ./...; then
        print_status "No known vulnerabilities found"
    else
        print_warning "Vulnerabilities detected or network error"
    fi
else
    print_warning "govulncheck not available"
fi
echo

# 2. Static Security Analysis
echo "2. 🔍 Static Security Analysis"
echo "   ==========================="

print_info "Skipping gosec due to Go version compatibility issues"
print_info "Using staticcheck for static analysis instead"
echo

# 3. Code Quality Analysis
echo "3. 📊 Code Quality Analysis"
echo "   ========================"

print_info "Running go vet..."
if go vet ./...; then
    print_status "go vet passed"
else
    print_error "go vet found issues"
fi

if check_tool staticcheck; then
    print_info "Running staticcheck..."
    if staticcheck ./...; then
        print_status "staticcheck passed"
    else
        print_warning "staticcheck found issues"
    fi
else
    print_warning "staticcheck not available"
fi
echo

# 4. Dependency Analysis
echo "4. 📦 Dependency Analysis"
echo "   ======================"

print_info "Checking Go modules..."
go mod verify
go mod tidy

# Check for dependencies
DEP_COUNT=$(go list -m all | wc -l)
if [ $DEP_COUNT -eq 1 ]; then
    print_status "No external dependencies - excellent security posture"
else
    print_info "Dependencies found: $((DEP_COUNT - 1))"
    go list -m all | tail -n +2
fi
echo

# 5. Secret Detection
echo "5. 🔐 Secret Detection"
echo "   =================="

print_info "Scanning for potential secrets..."

# Check for hardcoded passwords (excluding test files and examples)
if grep -r -i "password.*=" --include="*.go" . | grep -v "_test.go" | grep -v "examples/" | grep -v "// " | grep -q .; then
    print_warning "Potential hardcoded passwords found:"
    grep -r -i "password.*=" --include="*.go" . | grep -v "_test.go" | grep -v "examples/" | grep -v "// "
else
    print_status "No hardcoded passwords detected"
fi

# Check for API keys
if grep -r "api[_-]*key.*=" --include="*.go" . | grep -v "_test.go" | grep -v "examples/" | grep -v "// " | grep -q .; then
    print_warning "Potential hardcoded API keys found:"
    grep -r "api[_-]*key.*=" --include="*.go" . | grep -v "_test.go" | grep -v "examples/" | grep -v "// "
else
    print_status "No hardcoded API keys detected"
fi

# Check for secrets
if grep -r "secret.*=" --include="*.go" . | grep -v "_test.go" | grep -v "examples/" | grep -v "// " | grep -q .; then
    print_warning "Potential hardcoded secrets found:"
    grep -r "secret.*=" --include="*.go" . | grep -v "_test.go" | grep -v "examples/" | grep -v "// "
else
    print_status "No hardcoded secrets detected"
fi
echo

# 6. Network Security Analysis
echo "6. 🌐 Network Security Analysis"
echo "   ============================="

print_info "Checking TLS usage..."
TLS_USAGE=$(grep -r "tls\." --include="*.go" . | wc -l)
if [ $TLS_USAGE -gt 0 ]; then
    print_status "TLS usage detected in $TLS_USAGE locations"
else
    print_warning "No TLS usage detected"
fi

print_info "Checking for insecure connections..."
if grep -r "InsecureSkipVerify.*true" --include="*.go" . | grep -v "_test.go" | grep -q .; then
    print_warning "Insecure TLS verification found:"
    grep -r "InsecureSkipVerify.*true" --include="*.go" . | grep -v "_test.go"
else
    print_status "No insecure TLS verification found"
fi
echo

# 7. Memory Safety Analysis
echo "7. 🧠 Memory Safety Analysis"
echo "   ========================="

print_info "Checking for unsafe operations..."
if grep -r "unsafe\." --include="*.go" . | grep -v "_test.go" | grep -q .; then
    print_warning "Unsafe operations found:"
    grep -r "unsafe\." --include="*.go" . | grep -v "_test.go"
else
    print_status "No unsafe operations found"
fi

print_info "Checking for environment manipulation..."
if grep -r "os\.Setenv" --include="*.go" . | grep -v "_test.go" | grep -q .; then
    print_warning "Environment manipulation found:"
    grep -r "os\.Setenv" --include="*.go" . | grep -v "_test.go"
else
    print_status "No environment manipulation found"
fi
echo

# 8. Logging Security Analysis
echo "8. 📝 Logging Security Analysis"
echo "   ============================="

print_info "Checking for potential information disclosure in logs..."
if grep -r "fmt\.Print" --include="*.go" . | grep -v "_test.go" | grep -v "examples/" | grep -v "//" | grep -q .; then
    print_warning "fmt.Print statements found (potential info disclosure):"
    grep -r "fmt\.Print" --include="*.go" . | grep -v "_test.go" | grep -v "examples/" | grep -v "//"
else
    print_status "No fmt.Print statements found"
fi

if grep -r "log\.Print" --include="*.go" . | grep -v "_test.go" | grep -v "examples/" | grep -v "//" | grep -q .; then
    print_warning "log.Print statements found (potential info disclosure):"
    grep -r "log\.Print" --include="*.go" . | grep -v "_test.go" | grep -v "examples/" | grep -v "//"
else
    print_status "No log.Print statements found in main code"
fi
echo

# 9. Test Coverage Analysis
echo "9. 🧪 Test Coverage Analysis"
echo "   =========================="

print_info "Running tests with race detection..."
if go test -race ./...; then
    print_status "All tests passed with race detection"
else
    print_error "Tests failed"
fi

print_info "Generating coverage report..."
go test -coverprofile=coverage.out ./...
COVERAGE=$(go tool cover -func=coverage.out | grep total: | awk '{print $3}')
print_info "Test coverage: $COVERAGE"

if [ -f coverage.out ]; then
    rm coverage.out
fi
echo

# 10. Build Security Analysis
echo "10. 🏗️  Build Security Analysis"
echo "    ============================"

print_info "Building with security flags..."
if go build -buildmode=pie -ldflags="-s -w" ./...; then
    print_status "Build successful with security flags"
else
    print_error "Build failed with security flags"
fi
echo

# Summary
echo "📋 Security Audit Summary"
echo "=========================="
print_info "Security audit completed. Review any warnings above."
print_info "For production deployments, ensure:"
print_status "• TLS is properly configured with certificate validation"
print_status "• Strong authentication credentials are used"
print_status "• Network access is properly restricted"
print_status "• Memory limits are configured"
print_status "• Regular security updates are applied"
echo

print_info "Security documentation is available in SECURITY.md"
print_info "Automated security checks run in CI/CD via .github/workflows/security-audit.yml"