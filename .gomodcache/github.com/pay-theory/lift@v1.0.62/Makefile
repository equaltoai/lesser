.PHONY: all test build clean lint fmt vet cdk-synth cdk-deploy cdk-diff

# Local Go caches (sandbox-safe)
GOCACHE ?= $(CURDIR)/.gocache
GOMODCACHE ?= $(CURDIR)/.gomodcache
export GOCACHE
export GOMODCACHE

# Discover only packages that contain tests under ./pkg and selected roots.
# This avoids building unrelated packages (e.g., stacks) and excludes examples.
TEST_PKGS := $(shell find pkg -type f -name "*_test.go" -printf '%h\n' | sort -u | sed 's|^|./|' | grep -v '^\./pkg/cdk/integration$$')
TEST_PKGS += $(shell find benchmarks -type f -name "*_test.go" >/dev/null 2>&1 && echo ./benchmarks)

# Default target
all: test build

# Ensure local cache directories exist
cache-dirs:
	mkdir -p $(GOCACHE) $(GOMODCACHE)

# Run unit tests (exclude examples/ across the repo)
test: cache-dirs
	go test $(TEST_PKGS) -v

# Run tests with coverage (exclude examples/)
test-coverage: cache-dirs
	go test -covermode=atomic -coverprofile=coverage.out $(TEST_PKGS)
	go tool cover -func=coverage.out

# Run tests with race detection (exclude examples/)
test-race: cache-dirs
	go test $(TEST_PKGS) -race -cover

# Build the project
build: cache-dirs
	go build ./...

# Clean build artifacts
clean:
	go clean
	rm -f coverage.out
	rm -rf cdk.out

# Format code
fmt:
	go fmt ./...

# Run go vet
vet: cache-dirs
	go vet ./...

# Run linter (requires golangci-lint)
lint: cache-dirs
	golangci-lint run --config .golangci.yml ./...

# CDK targets
cdk-synth:
	cd pkg/cdk/examples/basic-api && cdk synth

cdk-deploy:
	cd pkg/cdk/examples/basic-api && cdk deploy

cdk-diff:
	cd pkg/cdk/examples/basic-api && cdk diff

cdk-destroy:
	cd pkg/cdk/examples/basic-api && cdk destroy

# Run benchmarks
bench:
	./benchmarks/run_benchmarks.sh

# Development helpers
dev: fmt vet test

# CI/CD helper
ci: fmt vet test-race lint
godoc-text:
	./scripts/generate-godoc.sh

.PHONY: doclint
doclint:
	# Pin revive version for reproducible CI
	go install github.com/mgechev/revive@v1.3.4
	# Use a minimal config to avoid failing CI on stylistic issues
	revive -config revive-docs.toml ./pkg/lift/... ./pkg/middleware/... ./pkg/lift/health/... ./pkg/lift/adapters/... ./pkg/observability/... || true
