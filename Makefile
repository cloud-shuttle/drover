.PHONY: help build build-all clean test install lint fmt check-fmt run

# Variables
BINARY_NAME=drover
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-s -w -X main.Version=$(VERSION)"
GO=go
GOFLAGS=$(LDFLAGS)

# Platform-specific binaries
BINARIES=$(BINARY_NAME)-linux-amd64 $(BINARY_NAME)-linux-arm64 $(BINARY_NAME)-darwin-amd64 $(BINARY_NAME)-darwin-arm64

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build for current architecture
	@echo "Building $(BINARY_NAME) for current platform..."
	$(GO) build $(GOFLAGS) -o $(BINARY_NAME) ./cmd/drover

build-all: ## Build for all platforms
	@echo "Building $(BINARY_NAME) for all platforms..."
	@$(MAKE) $(BINARY_NAME)-linux-amd64
	@$(MAKE) $(BINARY_NAME)-linux-arm64
	@$(MAKE) $(BINARY_NAME)-darwin-amd64
	@$(MAKE) $(BINARY_NAME)-darwin-arm64
	@echo "Built binaries:"
	@ls -lh $(BINARIES)

build-linux-amd64: ## Build for Linux AMD64
	@echo "Building for linux/amd64..."
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BINARY_NAME)-linux-amd64 ./cmd/drover

build-linux-arm64: ## Build for Linux ARM64
	@echo "Building for linux/arm64..."
	GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -o $(BINARY_NAME)-linux-arm64 ./cmd/drover

build-darwin-amd64: ## Build for macOS Intel
	@echo "Building for darwin/amd64..."
	GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BINARY_NAME)-darwin-amd64 ./cmd/drover

build-darwin-arm64: ## Build for macOS Apple Silicon
	@echo "Building for darwin/arm64..."
	GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) -o $(BINARY_NAME)-darwin-arm64 ./cmd/drover

clean: ## Remove build artifacts
	@echo "Cleaning..."
	rm -f $(BINARY_NAME) $(BINARY_NAME)-*
	rm -f coverage.out

test: ## Run tests
	@echo "Running tests..."
	$(GO) test -v -race -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

test-short: ## Run short tests
	@echo "Running short tests..."
	$(GO) test -short -v ./...

install: ## Install to $GOPATH/bin or ~/go/bin
	@echo "Installing $(BINARY_NAME)..."
	$(GO) install $(GOFLAGS) ./cmd/drover

lint: ## Run linter
	@echo "Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Install with: curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin latest"; \
	fi

fmt: ## Format code
	@echo "Formatting code..."
	$(GO) fmt ./...

check-fmt: ## Check if code is formatted
	@echo "Checking format..."
	@test -z "$$($(GO) fmt ./...)" || (echo "Code is not formatted. Run 'make fmt'" && exit 1)

vet: ## Run go vet
	@echo "Running go vet..."
	$(GO) vet ./...

deps: ## Download dependencies
	@echo "Downloading dependencies..."
	$(GO) mod download
	$(GO) mod tidy

run: ## Run the CLI (for testing)
	@echo "Running $(BINARY_NAME)..."
	$(GO) run ./cmd/drover

release: build-all ## Create release artifacts
	@echo "Creating release tarballs..."
	@for binary in $(BINARIES); do \
		tar -czf $$binary.tar.gz $$binary; \
	done
	@echo "Release artifacts:"
	@ls -lh $(BINARY_NAME)-*.tar.gz

.DEFAULT_GOAL := help
