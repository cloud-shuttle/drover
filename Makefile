# Drover Makefile
# Simple build and install targets

.PHONY: all build install test clean deps help

# Variables
BINARY_NAME=drover
BUILD_DIR=./build
GO?=go
GOFLAGS?=
INSTALL_DIR?=$(HOME)/bin

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
	@echo "  build       - Build the drover binary"
	@echo "  install     - Build and install to ~/bin"
	@echo "  install-system - Build and install to /usr/local/bin"
	@echo "  deps        - Install dependencies"
	@echo "  test        - Run tests"
	@echo "  clean       - Remove build artifacts"
	@echo "  help        - Show this help"
