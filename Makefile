# Blockchain Gateway Makefile

# Variables
BINARY_NAME=blockchain-gateway
BUILD_DIR=./build
MAIN_PATH=./cmd/server
GO_FILES=$(shell find . -name "*.go" -type f)
AIR_CONFIG=./.air.toml

# Go related variables
GOBASE=$(shell pwd)
GOBIN=$(GOBASE)/bin

# Commands
.PHONY: all build clean run test dev install-air

all: clean build

# Build the application
build:
	@echo "Building..."
	@go build -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)

# Clean build files
clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -rf tmp
	@go clean

# Run application
run: build
	@echo "Running..."
	@$(BUILD_DIR)/$(BINARY_NAME)

# Run tests
test:
	@echo "Testing..."
	@go test ./... -v

# Run with Air for live reloading
dev:
	@if command -v air > /dev/null; then \
		echo "Starting development server with Air..."; \
		air -c $(AIR_CONFIG); \
	else \
		echo "Air is not installed. Please run 'make install-air' first."; \
		exit 1; \
	fi

# Install Air for development
install-air:
	@echo "Installing Air..."
	@go install github.com/air-verse/air@latest

# Install all development tools
install-tools: install-air
	@echo "Installing development tools..."

# Generate Go module files
mod:
	@echo "Updating Go modules..."
	@go mod tidy

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...

# Lint code
lint:
	@if command -v golangci-lint > /dev/null; then \
		echo "Linting code..."; \
		golangci-lint run; \
	else \
		echo "golangci-lint is not installed. Please install it."; \
		exit 1; \
	fi

# Show help
help:
	@echo "Blockchain Gateway Make Commands:"
	@echo "make build      - Build the application"
	@echo "make clean      - Remove build artifacts"
	@echo "make run        - Build and run the application"
	@echo "make test       - Run tests"
	@echo "make dev        - Run with Air (live-reload)"
	@echo "make install-air - Install Air for development"
	@echo "make mod        - Update Go modules"
	@echo "make fmt        - Format code"
	@echo "make lint       - Lint code"
