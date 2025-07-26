#!/bin/bash

# Test setup script for Lesser project

set -e

echo "Setting up test environment..."

# Load test environment variables
if [ -f .env.test ]; then
    export $(cat .env.test | grep -v '^#' | xargs)
    echo "✓ Loaded test environment variables"
else
    echo "✗ .env.test file not found!"
    exit 1
fi

# Check if running in CI
if [ -n "$CI" ]; then
    echo "Running in CI environment"
else
    echo "Running in local environment"
fi

# Create test directories
mkdir -p tmp/test-data
mkdir -p tmp/test-logs

# Ensure Go modules are downloaded
echo "Downloading Go modules..."
go mod download

# Build test binaries
echo "Building test binaries..."
go build -o /dev/null ./...

# Run tests based on arguments
if [ "$1" == "unit" ]; then
    echo "Running unit tests..."
    go test -v -race -cover ./pkg/...
elif [ "$1" == "integration" ]; then
    echo "Running integration tests..."
    go test -v -tags=integration ./tests/integration/...
elif [ "$1" == "all" ]; then
    echo "Running all tests..."
    go test -v -race -cover ./...
elif [ "$1" == "benchmark" ]; then
    echo "Running benchmarks..."
    go test -bench=. -benchmem ./pkg/...
else
    echo "Usage: ./scripts/test-setup.sh [unit|integration|all|benchmark]"
    echo "  unit        - Run unit tests only"
    echo "  integration - Run integration tests only"
    echo "  all         - Run all tests"
    echo "  benchmark   - Run performance benchmarks"
    exit 1
fi

echo "Test run complete!"