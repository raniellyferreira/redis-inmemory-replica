# Redis In-Memory Replica Library Makefile

.DEFAULT_GOAL := help
.PHONY: help build test lint clean examples install-tools benchmark coverage docs security-audit security-install security-scan security-static security-deps

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
BINARY_NAME=redis-replica
BINARY_UNIX=$(BINARY_NAME)_unix

# Build flags
VERSION ?= $(shell git describe --tags --always --dirty)
COMMIT_HASH ?= $(shell git rev-parse HEAD)
BUILD_TIME ?= $(shell date -u +%Y%m%d.%H%M%S)
LDFLAGS = -ldflags "-X github.com/raniellyferreira/redis-inmemory-replica.GitCommit=$(COMMIT_HASH) \
                    -X github.com/raniellyferreira/redis-inmemory-replica.BuildTime=$(BUILD_TIME)"

help: ## Show this help message
	@echo 'Usage: make <target>'
	@echo ''
	@echo 'Targets:'
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ## Build the library and examples
	@echo "Building library..."
	$(GOBUILD) $(LDFLAGS) -v ./...
	@echo "Building examples..."
	$(GOBUILD) $(LDFLAGS) -o bin/basic-example ./examples/basic
	$(GOBUILD) $(LDFLAGS) -o bin/monitoring-example ./examples/monitoring
	$(GOBUILD) $(LDFLAGS) -o bin/cluster-example ./examples/cluster
	$(GOBUILD) $(LDFLAGS) -o bin/database-filtering-example ./examples/database-filtering
	$(GOBUILD) $(LDFLAGS) -o bin/rdb-logging-demo ./examples/rdb-logging-demo
	$(GOBUILD) $(LDFLAGS) -o bin/timeout-demo ./examples/timeout-demo

test: ## Run tests
	@echo "Running tests..."
	$(GOTEST) -v -race -coverprofile=coverage.out ./...

test-short: ## Run short tests only
	@echo "Running short tests..."
	$(GOTEST) -short -v ./...

benchmark: ## Run benchmarks
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem ./...

coverage: test ## Generate test coverage report
	@echo "Generating coverage report..."
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

lint: install-tools ## Run linter
	@echo "Running linter..."
	$(shell go env GOPATH)/bin/golangci-lint run

lint-fix: install-tools ## Run linter with auto-fix
	@echo "Running linter with auto-fix..."
	$(shell go env GOPATH)/bin/golangci-lint run --fix

clean: ## Clean build artifacts
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -rf bin/
	rm -f coverage.out coverage.html

deps: ## Download dependencies
	@echo "Downloading dependencies..."
	$(GOMOD) download

tidy: ## Tidy dependencies
	@echo "Tidying dependencies..."
	$(GOMOD) tidy

vendor: ## Vendor dependencies
	@echo "Vendoring dependencies..."
	$(GOMOD) vendor

install-tools: ## Install required tools
	@echo "Installing tools..."
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(shell go env GOPATH)/bin v1.54.2)

examples: build ## Run example programs
	@echo "Running basic example..."
	@echo "Note: Make sure Redis is running on localhost:6379"
	@timeout 10s ./bin/basic-example || echo "Example completed (timeout or error is expected for demo)"

docs: ## Generate documentation
	@echo "Generating documentation..."
	@echo "Visit: https://pkg.go.dev/github.com/raniellyferreira/redis-inmemory-replica"
	$(GOCMD) doc -all . > docs.txt
	@echo "Local docs saved to docs.txt"

format: ## Format code
	@echo "Formatting code..."
	$(GOCMD) fmt ./...

vet: ## Run go vet
	@echo "Running go vet..."
	$(GOCMD) vet ./...

check: lint vet test ## Run all checks (lint, vet, test)

release: check ## Prepare for release
	@echo "Preparing release..."
	@git tag -l | sort -V | tail -5
	@echo "Latest tags shown above. Use 'git tag vX.Y.Z' to create a new release tag."

docker-test: ## Run tests in Docker
	@echo "Running tests in Docker..."
	docker run --rm -v $(PWD):/app -w /app golang:1.21 make test

# Development helpers
dev-setup: deps install-tools ## Set up development environment
	@echo "Development environment ready!"

watch-test: ## Watch files and run tests automatically
	@echo "Watching for changes..."
	@which fswatch > /dev/null || (echo "Please install fswatch: brew install fswatch" && exit 1)
	fswatch -o . -e ".*" -i "\\.go$$" | xargs -n1 -I{} make test-short

# Build for different architectures
build-all: ## Build for all supported platforms
	@echo "Building for all platforms..."
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o bin/$(BINARY_UNIX) ./examples/basic
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o bin/$(BINARY_NAME).exe ./examples/basic
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o bin/$(BINARY_NAME)_darwin ./examples/basic
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o bin/$(BINARY_NAME)_darwin_arm64 ./examples/basic

# Info targets
version: ## Show version information
	@echo "Version: $(VERSION)"
	@echo "Commit: $(COMMIT_HASH)"
	@echo "Build time: $(BUILD_TIME)"

info: ## Show project information
	@echo "Redis In-Memory Replica Library"
	@echo "==============================="
	@echo "Go version: $(shell go version)"
	@echo "Git commit: $(COMMIT_HASH)"
	@echo "Build time: $(BUILD_TIME)"
	@echo ""
	@echo "Project structure:"
	@tree -I 'vendor|bin|.git' -L 2 || find . -type d -name vendor -prune -o -type d -name bin -prune -o -type d -name .git -prune -o -type d -print | head -20

# Security targets
security-audit: ## Run comprehensive security audit
	@echo "Running security audit..."
	@./scripts/security-audit.sh

security-install: ## Install security tools
	@echo "Installing security tools..."
	@go install golang.org/x/vuln/cmd/govulncheck@latest
	@go install honnef.co/go/tools/cmd/staticcheck@latest

security-scan: ## Run vulnerability scan  
	@echo "Running vulnerability scan..."
	@$(shell go env GOPATH)/bin/govulncheck ./... || echo "Vulnerability scan completed with warnings"

security-static: ## Run static security analysis
	@echo "Running static security analysis..."
	@$(shell go env GOPATH)/bin/staticcheck ./... || echo "Static analysis completed with warnings"

security-deps: ## Check dependencies for vulnerabilities
	@echo "Checking dependencies..."
	@go mod verify
	@go mod tidy
	@go list -m all