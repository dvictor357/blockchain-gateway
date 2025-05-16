#!/bin/bash

# Blockchain Gateway Development Helper Script
#
# This script provides useful commands for developers working on the blockchain-gateway project.
# It helps with various development tasks like running the server, tests, linting, etc.

set -e

# Colors for terminal output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Project directories
PROJECT_ROOT=$(pwd)
LOG_DIR="$PROJECT_ROOT/logs"
mkdir -p "$LOG_DIR"

# Ensure we're in the right directory
if [[ ! -f "go.mod" ]]; then
    echo -e "${RED}Error: This script must be run from the project root (where go.mod is located)${NC}"
    exit 1
fi

# Check for required dependencies
check_dependencies() {
    echo -e "${BLUE}Checking dependencies...${NC}"

    # Check for Go
    if ! command -v go &> /dev/null; then
        echo -e "${RED}Error: Go is not installed${NC}"
        exit 1
    fi

    # Check for Air (for hot reloading)
    if ! command -v air &> /dev/null; then
        echo -e "${YELLOW}Warning: Air is not installed. Live-reload will not be available.${NC}"
        echo -e "${YELLOW}Run 'go install github.com/air-verse/air@latest' to install it.${NC}"
    fi

    echo -e "${GREEN}All required dependencies are installed!${NC}"
}

# Start development server with Air (hot reloading)
start_dev() {
    echo -e "${BLUE}Starting development server with Air...${NC}"

    # Load development environment variables
    if [[ -f .env.development ]]; then
        export $(grep -v '^#' .env.development | xargs)
    fi

    # Check if Air is installed
    if command -v air &> /dev/null; then
        # Start with Air
        air -c .air.toml
    else
        # Fallback to regular go run
        echo -e "${YELLOW}Air not found, falling back to 'go run'${NC}"
        go run cmd/server/main.go
    fi
}

# Start Docker development environment
start_docker_dev() {
    echo -e "${BLUE}Starting Docker development environment...${NC}"
    docker-compose up blockchain-gateway-dev
}

# Run tests
run_tests() {
    echo -e "${BLUE}Running tests...${NC}"
    go test ./... -v
}

# Run linting
run_lint() {
    echo -e "${BLUE}Linting code...${NC}"
    if command -v golangci-lint &> /dev/null; then
        golangci-lint run
    else
        echo -e "${YELLOW}golangci-lint is not installed. Using basic go formatting instead.${NC}"
        go fmt ./...
    fi
}

# Build the application
build_app() {
    echo -e "${BLUE}Building application...${NC}"
    go build -o build/blockchain-gateway ./cmd/server
    echo -e "${GREEN}Build complete: ./build/blockchain-gateway${NC}"
}

# Clean build artifacts
clean() {
    echo -e "${BLUE}Cleaning build artifacts...${NC}"
    rm -rf build tmp
    go clean
    echo -e "${GREEN}Cleanup complete${NC}"
}

# Update dependencies
update_deps() {
    echo -e "${BLUE}Updating dependencies...${NC}"
    go mod tidy
    go mod verify
    echo -e "${GREEN}Dependencies updated${NC}"
}

# Generate documentation
generate_docs() {
    echo -e "${BLUE}Generating documentation...${NC}"
    if command -v godoc &> /dev/null; then
        echo -e "${GREEN}Starting godoc server at http://localhost:6060${NC}"
        echo -e "${YELLOW}Press Ctrl+C to stop${NC}"
        godoc -http=:6060
    else
        echo -e "${YELLOW}godoc is not installed. Run 'go install golang.org/x/tools/cmd/godoc@latest' to install it.${NC}"
    fi
}

# Show help
show_help() {
    echo -e "${BLUE}Blockchain Gateway Development Helper${NC}"
    echo ""
    echo -e "Usage: ${YELLOW}./dev.sh <command>${NC}"
    echo ""
    echo "Commands:"
    echo -e "  ${GREEN}start${NC}         Start development server with hot-reloading (Air)"
    echo -e "  ${GREEN}docker${NC}        Start Docker development environment"
    echo -e "  ${GREEN}build${NC}         Build the application"
    echo -e "  ${GREEN}test${NC}          Run tests"
    echo -e "  ${GREEN}lint${NC}          Run linter"
    echo -e "  ${GREEN}clean${NC}         Clean build artifacts"
    echo -e "  ${GREEN}deps${NC}          Update dependencies"
    echo -e "  ${GREEN}docs${NC}          Generate and serve documentation"
    echo -e "  ${GREEN}check${NC}         Check dependencies"
    echo -e "  ${GREEN}help${NC}          Show this help message"
    echo ""
}

# Process command line argument
case "$1" in
    start)
        check_dependencies
        start_dev
        ;;
    docker)
        check_dependencies
        start_docker_dev
        ;;
    test)
        run_tests
        ;;
    lint)
        run_lint
        ;;
    build)
        build_app
        ;;
    clean)
        clean
        ;;
    deps)
        update_deps
        ;;
    docs)
        generate_docs
        ;;
    check)
        check_dependencies
        ;;
    help|*)
        show_help
        ;;
esac
