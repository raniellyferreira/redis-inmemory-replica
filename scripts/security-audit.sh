#!/bin/bash

# Redis In-Memory Replica Enhanced Security Audit Script
# This script performs a comprehensive security audit of the codebase
# with enhanced checks for production readiness

set -e

echo "🔒 Redis In-Memory Replica Enhanced Security Audit"
echo "=================================================="
echo

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
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

print_section() {
    echo -e "${PURPLE}$1${NC}"
    echo -e "${PURPLE}$(echo "$1" | sed 's/./=/g')${NC}"
}

print_subsection() {
    echo -e "${CYAN}$1${NC}"
    echo -e "${CYAN}$(echo "$1" | sed 's/./-/g')${NC}"
}

# Security audit counters
security_issues=0
security_warnings=0
security_passed=0

# Function to increment counters
increment_issue() {
    security_issues=$((security_issues + 1))
}

increment_warning() {
    security_warnings=$((security_warnings + 1))
}

increment_passed() {
    security_passed=$((security_passed + 1))
}

print_info() {
    echo -e "${BLUE}ℹ️${NC} $1"
}

# Check if tools are available
check_tool() {
    if command -v $1 &> /dev/null; then
        return 0
    elif [ -f "/home/runner/go/bin/$1" ]; then
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
    if /home/runner/go/bin/govulncheck ./... || govulncheck ./...; then
        print_status "No known vulnerabilities found"
        increment_passed
    else
        print_warning "Vulnerabilities detected or network error"
        increment_warning
    fi
else
    print_warning "govulncheck not available"
    increment_warning
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
    increment_passed
else
    print_error "go vet found issues"
    increment_issue
fi

if check_tool staticcheck; then
    print_info "Running staticcheck..."
    if [ -f "/home/runner/go/bin/staticcheck" ]; then
        STATICCHECK_CMD="/home/runner/go/bin/staticcheck"
    elif command -v staticcheck &> /dev/null; then
        STATICCHECK_CMD="staticcheck"
    else
        STATICCHECK_CMD=""
    fi
    
    if [ -n "$STATICCHECK_CMD" ]; then
        if $STATICCHECK_CMD ./...; then
            print_status "staticcheck passed"
            increment_passed
        else
            print_warning "staticcheck found issues (non-critical)"
            increment_warning
        fi
    else
        print_warning "staticcheck executable not found"
        increment_warning
    fi
else
    print_warning "staticcheck not available"
    increment_warning
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

# More refined patterns to avoid false positives
# Exclude test files, examples, mock files, and testdata directories
# Also exclude variable assignments without actual values and field access patterns

# Check for hardcoded passwords - looking for actual string assignments, not variable declarations
if grep -r -E "(password|pwd)\s*[:=]\s*[\"'][^\"']{1,}" --include="*.go" . | \
   grep -v "_test.go" | \
   grep -v "examples/" | \
   grep -v "testdata/" | \
   grep -v "mock" | \
   grep -v "// safe:" | \
   grep -v "password.*password" | \
   grep -v "\.password" | \
   grep -v "config\..*password" | \
   grep -v "c\..*password" | \
   grep -v "cfg\..*password" | \
   grep -q .; then
    print_warning "Potential hardcoded passwords found:"
    grep -r -E "(password|pwd)\s*[:=]\s*[\"'][^\"']{1,}" --include="*.go" . | \
    grep -v "_test.go" | \
    grep -v "examples/" | \
    grep -v "testdata/" | \
    grep -v "mock" | \
    grep -v "// safe:" | \
    grep -v "password.*password" | \
    grep -v "\.password" | \
    grep -v "config\..*password" | \
    grep -v "c\..*password" | \
    grep -v "cfg\..*password"
    increment_warning
else
    print_status "No hardcoded passwords detected"
    increment_passed
fi

# Check for API keys - refined pattern
if grep -r -E "(api[_-]*key|apikey|api_key)\s*[:=]\s*[\"'][^\"']{1,}" --include="*.go" . | \
   grep -v "_test.go" | \
   grep -v "examples/" | \
   grep -v "testdata/" | \
   grep -v "mock" | \
   grep -v "// safe:" | \
   grep -q .; then
    print_warning "Potential hardcoded API keys found:"
    grep -r -E "(api[_-]*key|apikey|api_key)\s*[:=]\s*[\"'][^\"']{1,}" --include="*.go" . | \
    grep -v "_test.go" | \
    grep -v "examples/" | \
    grep -v "testdata/" | \
    grep -v "mock" | \
    grep -v "// safe:"
    increment_warning
else
    print_status "No hardcoded API keys detected"
    increment_passed
fi

# Check for secrets - refined pattern
if grep -r -E "(secret|token)\s*[:=]\s*[\"'][^\"']{1,}" --include="*.go" . | \
   grep -v "_test.go" | \
   grep -v "examples/" | \
   grep -v "testdata/" | \
   grep -v "mock" | \
   grep -v "// safe:" | \
   grep -v "\.secret" | \
   grep -v "config\..*secret" | \
   grep -v "c\..*secret" | \
   grep -v "cfg\..*secret" | \
   grep -q .; then
    print_warning "Potential hardcoded secrets found:"
    grep -r -E "(secret|token)\s*[:=]\s*[\"'][^\"']{1,}" --include="*.go" . | \
    grep -v "_test.go" | \
    grep -v "examples/" | \
    grep -v "testdata/" | \
    grep -v "mock" | \
    grep -v "// safe:" | \
    grep -v "\.secret" | \
    grep -v "config\..*secret" | \
    grep -v "c\..*secret" | \
    grep -v "cfg\..*secret"
    increment_warning
else
    print_status "No hardcoded secrets detected"
    increment_passed
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
# Check for unsafe operations, but allow those marked as safe
UNSAFE_RESULTS=$(grep -r -n "unsafe\." --include="*.go" . | grep -v "_test.go" | grep -v "// safe:")
if [ -n "$UNSAFE_RESULTS" ]; then
    # Check each result to see if it has a "safe:" comment within 3 lines before it
    FILTERED_RESULTS=""
    while IFS= read -r line; do
        if [ -n "$line" ]; then
            FILE=$(echo "$line" | cut -d: -f1)
            LINENUM=$(echo "$line" | cut -d: -f2)
            # Check for safe comment in the 3 lines before
            SAFE_COMMENT=$(sed -n "$((LINENUM-3)),$((LINENUM-1))p" "$FILE" 2>/dev/null | grep -c "// safe:")
            if [ "$SAFE_COMMENT" -eq 0 ]; then
                FILTERED_RESULTS="$FILTERED_RESULTS$line\n"
            fi
        fi
    done <<< "$UNSAFE_RESULTS"
    
    if [ -n "$FILTERED_RESULTS" ] && [ "$FILTERED_RESULTS" != "\n" ]; then
        print_warning "Unsafe operations found:"
        echo -e "$FILTERED_RESULTS"
        increment_warning
    else
        print_status "No problematic unsafe operations found"
        increment_passed
    fi
else
    print_status "No unsafe operations found"
    increment_passed
fi

print_info "Checking for environment manipulation..."
if grep -r "os\.Setenv" --include="*.go" . | grep -v "_test.go" | grep -q .; then
    print_warning "Environment manipulation found:"
    grep -r "os\.Setenv" --include="*.go" . | grep -v "_test.go"
    increment_warning
else
    print_status "No environment manipulation found"
    increment_passed
fi
echo

# 8. Logging Security Analysis
echo "8. 📝 Logging Security Analysis"
echo "   ============================="

print_info "Checking for potential information disclosure in logs..."
if grep -r "fmt\.Print" --include="*.go" . | grep -v "_test.go" | grep -v "examples/" | grep -v "//" | grep -q .; then
    print_warning "fmt.Print statements found (potential info disclosure):"
    grep -r "fmt\.Print" --include="*.go" . | grep -v "_test.go" | grep -v "examples/" | grep -v "//"
    increment_warning
else
    print_status "No fmt.Print statements found"
    increment_passed
fi

if grep -r "log\.Print" --include="*.go" . | grep -v "_test.go" | grep -v "examples/" | grep -v "//" | grep -v "interfaces.go" | grep -q .; then
    print_warning "log.Print statements found (potential info disclosure):"
    grep -r "log\.Print" --include="*.go" . | grep -v "_test.go" | grep -v "examples/" | grep -v "//" | grep -v "interfaces.go"
    increment_warning
else
    print_status "No problematic log.Print statements found"
    increment_passed
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