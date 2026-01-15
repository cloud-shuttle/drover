# Drover Makefile
# Simple build and install targets

.PHONY: all build install test clean deps help build-all

# Variables
BINARY_NAME=drover
BUILD_DIR=./build
GO?=go
GOFLAGS?=
INSTALL_DIR?=$(HOME)/bin
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS?=-ldflags "-X main.version=$(VERSION)"

all: build

deps:
	$(GO) mod download
	$(GO) mod tidy

build:
	@echo "Building drover..."
	$(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/drover

install: build
	@echo "Installing drover to $(INSTALL_DIR)..."
	@mkdir -p $(INSTALL_DIR)
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_DIR)/
	@echo "✅ Installed to $(INSTALL_DIR)/drover"
	@if command -v drover >/dev/null 2>&1; then \
		echo "✅ Drover is ready to use! Try: drover --help"; \
	else \
		echo "⚠️  $(INSTALL_DIR) may not be in your PATH"; \
		echo ""; \
		echo "Add to your ~/.bashrc or ~/.zshrc:"; \
		echo "   export PATH=\"$$PATH:$(INSTALL_DIR)\""; \
	fi

install-system: build
	@echo "Installing drover to /usr/local/bin (requires sudo)..."
	@sudo cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/
	@echo "✅ Installed to /usr/local/bin/drover"
	@echo "✨ Ready to use from any directory!"

test:
	$(GO) test -v ./...

clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@echo "Clean!"

help:
	@echo "Drover - AI Workflow Orchestrator"
	@echo ""
	@echo "Usage:"
	@echo "  make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  build       - Build the drover binary for current platform"
	@echo "  build-all   - Build drover for all platforms (linux, darwin, windows)"
	@echo "  install     - Build and install to ~/bin"
	@echo "  install-system - Build and install to /usr/local/bin"
	@echo "  deps        - Install dependencies"
	@echo "  test        - Run tests"
	@echo "  clean       - Remove build artifacts"
	@echo "  help        - Show this help"

# Cross-platform build targets
build-all: build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64 build-windows-amd64
	@echo "✅ All builds complete in $(BUILD_DIR)/"

build-linux-amd64:
	@echo "Building drover for linux/amd64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/drover-linux-amd64 ./cmd/drover

build-linux-arm64:
	@echo "Building drover for linux/arm64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/drover-linux-arm64 ./cmd/drover

build-darwin-amd64:
	@echo "Building drover for darwin/amd64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/drover-darwin-amd64 ./cmd/drover

build-darwin-arm64:
	@echo "Building drover for darwin/arm64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/drover-darwin-arm64 ./cmd/drover

build-windows-amd64:
	@echo "Building drover for windows/amd64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/drover-windows-amd64.exe ./cmd/drover
