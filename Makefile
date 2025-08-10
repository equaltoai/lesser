.PHONY: build test clean deploy fmt lint install-tools build-win clean-win

# Variables
GOOS ?= linux
GOARCH ?= arm64
CGO_ENABLED ?= 0

# Detect Windows
ifeq ($(OS),Windows_NT)
    DETECTED_OS := Windows
else
    DETECTED_OS := $(shell uname -s)
endif

# List of Lambda functions to build
LAMBDAS := webfinger actor inbox outbox collections activity-processor graphql
# Add new lambdas here and create corresponding build targets below

# Build all Lambda functions
build:
	@echo "Building Lambda functions..."
	@for lambda in $(LAMBDAS); do \
		echo "Building cmd/$$lambda..."; \
		GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) \
			go build -ldflags="-s -w" -o bin/$$lambda ./cmd/$$lambda || exit 1; \
	done

# Build a specific Lambda function
build-%:
	@echo "Building cmd/$*..."
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) \
		go build -ldflags="-s -w" -o bin/$* ./cmd/$*

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Run tests with coverage and enforce minimum coverage
test-coverage-enforce:
	@echo "Running tests with coverage enforcement..."
	go test -v -coverprofile=coverage.out ./...
	@echo "Checking coverage requirements..."
	@go tool cover -func=coverage.out | tail -n 1 | awk '{print "Total coverage: " $$3}' 
	@go tool cover -func=coverage.out | tail -n 1 | awk '{gsub(/%/,"",$$3); if($$3 < 70) {print "Coverage " $$3 "% is below minimum 70%"; exit 1} else {print "Coverage " $$3 "% meets minimum requirements"}}'
	go tool cover -html=coverage.out -o coverage.html

# Run tests with detailed coverage by package
test-coverage-detail:
	@echo "Running detailed coverage analysis..."
	@for pkg in $$(go list ./pkg/...); do \
		echo "Testing $$pkg..."; \
		go test -v -coverprofile=coverage_tmp.out $$pkg; \
		if [ -f coverage_tmp.out ]; then \
			coverage=$$(go tool cover -func=coverage_tmp.out | tail -n 1 | awk '{gsub(/%/,"",$$3); print $$3}'); \
			echo "$$pkg: $${coverage}%"; \
		fi; \
		rm -f coverage_tmp.out; \
	done

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Run linter
lint:
	@echo "Running linter..."
	golangci-lint run --config .golangci.yml

# Run linter and automatically fix issues where possible
lint-fix:
	@echo "Running linter with auto-fix..."
	golangci-lint run --config .golangci.yml --fix

# Run linter on new code only (requires git)
lint-new:
	@echo "Running linter on new code..."
	golangci-lint run --config .golangci.yml --new-from-rev=main

# Run specific linter
lint-%:
	@echo "Running specific linter: $*..."
	golangci-lint run --config .golangci.yml --disable-all --enable=$*

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	rm -f coverage.out coverage.html

# Install development tools
install-tools:
	@echo "Installing development tools..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/aws/aws-lambda-go/cmd/build-lambda-zip@latest
	go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest
	go install github.com/99designs/gqlgen@latest
	@echo "Development tools installed successfully!"

# Create zip files for Lambda deployment
package: build
	@echo "Creating deployment packages..."
	@mkdir -p dist
	@for lambda in $(LAMBDAS); do \
		echo "Packaging $$lambda..."; \
		cd bin && zip ../dist/$$lambda.zip $$lambda && cd ..; \
	done

# Deploy infrastructure with Pulumi
deploy-infra:
	@echo "Deploying infrastructure with Pulumi..."
	cd infra && pulumi up

# Deploy Lambda functions (requires AWS CLI configured)
deploy-functions: package
	@echo "Deploying Lambda functions..."
	@for lambda in $(LAMBDAS); do \
		echo "Deploying $$lambda..."; \
		aws lambda update-function-code \
			--function-name lesser-$$lambda \
			--zip-file fileb://dist/$$lambda.zip \
			--no-cli-pager 2>/dev/null || \
		echo "Function lesser-$$lambda not found. Run 'make deploy-infra' first."; \
	done

# Run local DynamoDB for development
local-dynamodb:
	@echo "Starting local DynamoDB..."
	docker run -p 8000:8000 amazon/dynamodb-local

# Initialize local development environment
dev-init:
	@echo "Initializing development environment..."
	@echo "Creating .env file..."
	@if [ ! -f .env ]; then \
		echo "DOMAIN=localhost" > .env; \
		echo "INSTANCE_NAME=Lesser Dev" >> .env; \
		echo "AWS_REGION=us-east-1" >> .env; \
		echo "DYNAMO_TABLE_NAME=lesser-dev" >> .env; \
		echo "S3_BUCKET_NAME=lesser-dev-media" >> .env; \
		echo "JWT_SECRET=$$(openssl rand -base64 32)" >> .env; \
		echo "Created .env file with default values"; \
	else \
		echo ".env file already exists"; \
	fi

# Run go mod tidy
tidy:
	@echo "Tidying Go modules..."
	go mod tidy

# Vendor dependencies
vendor:
	@echo "Vendoring dependencies..."
	go mod vendor

# Windows-specific build command
build-win:
	@echo "Building Lambda functions for Windows..."
	@if not exist bin mkdir bin
	@for %%l in (webfinger actor inbox outbox collections activity-processor graphql) do ( \
		echo Building cmd/%%l... && \
		go build -ldflags="-s -w" -o bin/%%l.exe ./cmd/%%l \
	)

# Windows-specific build-lambdas command
build-lambdas-win:
	@echo "Building all Lambda functions for Windows..."
	@if not exist bin mkdir bin
	@for %%l in (webfinger actor inbox outbox collections activity-processor graphql api objects auth auth-api search-indexer status-indexer push-delivery cost-aggregator ai-processor streaming stream-router note-processor moderation-processor report-trust-updater federation-tracker federation-delivery import-processor export-generator media-processor trend-aggregator) do ( \
		echo Building cmd/%%l... && \
		go build -ldflags="-s -w" -o bin/%%l.exe ./cmd/%%l \
	)

# Windows-specific clean command
clean-win:
	@echo "Cleaning build artifacts for Windows..."
	@if exist bin rmdir /s /q bin
	@if exist coverage.out del coverage.out
	@if exist coverage.html del coverage.html

# Run local development server
dev:
	@echo "Starting local development server..."
	@if [ ! -f .env ]; then \
		echo "No .env file found. Run 'make dev-init' first."; \
		exit 1; \
	fi
	@echo "Loading environment from .env..."
	@export $$(cat .env | grep -v '^#' | xargs) && \
		go run ./cmd/api

# Tail Lambda logs
logs:
	@echo "Tailing Lambda logs..."
	@if [ -z "$(FUNCTION)" ]; then \
		echo "Usage: make logs FUNCTION=function-name"; \
		echo "Available functions: api, graphql, inbox, outbox, etc."; \
		exit 1; \
	fi
	aws logs tail /aws/lambda/lesser-$(FUNCTION) --follow

# Deploy with Pulumi (alias for deploy-infra)
pulumi-up: deploy-infra

# Destroy infrastructure with Pulumi
pulumi-destroy:
	@echo "Destroying infrastructure with Pulumi..."
	@echo "WARNING: This will destroy all resources!"
	@read -p "Are you sure? (y/N): " confirm && [ "$$confirm" = "y" ] || exit 1
	cd infra && pulumi destroy --yes

# Test harness and integration tests
test-integration:
	@echo "Running integration tests..."
	@TEST_ENV=integration go test -tags=integration -v -timeout=30m ./pkg/testing/harness/...
	@TEST_ENV=integration go test -tags=integration -v -timeout=30m ./cmd/*/...

# Unit tests only
test-unit:
	@echo "Running unit tests only..."
	go test -short -v ./...

# Benchmark tests
test-benchmark:
	@echo "Running benchmark tests..."
	go test -bench=. -benchmem -v ./pkg/testing/benchmarks/...

# Test with race detection
test-race:
	@echo "Running tests with race detection..."
	go test -race -v ./...

# Test specific package
test-package:
	@echo "Testing specific package: $(PKG)"
	@if [ -z "$(PKG)" ]; then \
		echo "Usage: make test-package PKG=path/to/package"; \
		exit 1; \
	fi
	go test -v ./$(PKG)

# Run tests in watch mode (requires entr)
test-watch:
	@echo "Running tests in watch mode (requires 'entr' command)..."
	@if ! command -v entr >/dev/null 2>&1; then \
		echo "Error: 'entr' command not found. Install with: brew install entr"; \
		exit 1; \
	fi
	@find . -name "*.go" | entr -c make test

# Additional test targets
test-api:
	@echo "Running API tests..."
	cd tests && python -m pytest api/ -v

test-federation:
	@echo "Running federation tests..."
	cd tests && python -m pytest federation/ -v

test-search:
	@echo "Running search tests..."
	cd tests && python -m pytest search/ -v

test-ai:
	@echo "Running AI integration tests..."
	cd tests && python -m pytest ai/ -v

test-auth:
	@echo "Running authentication tests..."
	cd tests && python -m pytest auth/ -v

test-load:
	@echo "Running k6 load tests..."
	@$(MAKE) k6-local

# Load testing targets
k6-auth:
	@echo "Testing auth endpoints..."
	k6 run tests/load/auth_test.js

k6-timeline:
	@echo "Testing timeline performance..."
	k6 run tests/load/timeline_test.js

k6-posting:
	@echo "Testing post creation..."
	k6 run tests/load/posting_test.js

k6-federation:
	@echo "Testing federation..."
	k6 run tests/load/federation_test.js

# Generate code targets
generate:
	@echo "Generating code..."
	@$(MAKE) gqlgen

# Security and code quality
sec-scan:
	@echo "Running security scan..."
	gosec ./...

vuln-check:
	@echo "Checking for vulnerabilities..."
	govulncheck ./...

# Documentation targets
docs:
	@echo "Building documentation..."
	@echo "Documentation is in markdown format in the docs/ directory"
	@echo "See docs/DOCUMENTATION_INDEX.md for a complete guide"

docs-serve:
	@echo "Serving documentation locally..."
	@command -v mdbook >/dev/null 2>&1 || { echo "mdbook not installed. Run 'cargo install mdbook' first."; exit 1; }
	mdbook serve docs/

# Database operations
db-migrate:
	@echo "Running database migrations..."
	go run ./cmd/migrate

db-seed:
	@echo "Seeding database with test data..."
	go run ./cmd/seed

db-reset:
	@echo "Resetting database..."
	@echo "WARNING: This will delete all data!"
	@read -p "Are you sure? (y/N): " confirm && [ "$$confirm" = "y" ] || exit 1
	go run ./cmd/reset-db

# Monitoring and debugging
metrics:
	@echo "Showing CloudWatch metrics..."
	@if [ -z "$(METRIC)" ]; then \
		echo "Usage: make metrics METRIC=metric-name"; \
		echo "Examples: make metrics METRIC=Invocations"; \
		exit 1; \
	fi
	aws cloudwatch get-metric-statistics \
		--namespace AWS/Lambda \
		--metric-name $(METRIC) \
		--start-time $$(date -u -d '1 hour ago' +%Y-%m-%dT%H:%M:%S) \
		--end-time $$(date -u +%Y-%m-%dT%H:%M:%S) \
		--period 300 \
		--statistics Sum,Average

status:
	@echo "Checking system status..."
	@echo "Checking Lambda functions..."
	@aws lambda list-functions --query 'Functions[?starts_with(FunctionName, `lesser-`)].{Name:FunctionName,Runtime:Runtime,LastModified:LastModified}' --output table
	@echo ""
	@echo "Checking DynamoDB tables..."
	@aws dynamodb list-tables --query 'TableNames[?contains(@, `lesser`)]' --output table

# Performance profiling
profile-cpu:
	@echo "Running CPU profile..."
	go test -cpuprofile cpu.prof -bench . ./...
	go tool pprof cpu.prof

profile-mem:
	@echo "Running memory profile..."
	go test -memprofile mem.prof -bench . ./...
	go tool pprof mem.prof

# Help
help:
	@echo "Available targets:"
	@echo ""
	@echo "Building:"
	@echo "  build           - Build all Lambda functions"
	@echo "  build-<name>    - Build specific Lambda function"
	@echo "  build-lambdas   - Build all Lambda deployment packages"
	@echo "  package         - Create Lambda deployment packages"
	@echo "  generate        - Generate GraphQL code"
	@echo ""
	@echo "Testing:"
	@echo "  test                  - Run all Go unit tests"
	@echo "  test-unit             - Run unit tests only (short)"
	@echo "  test-integration      - Run integration tests"
	@echo "  test-benchmark        - Run benchmark tests"
	@echo "  test-coverage         - Run tests with coverage"
	@echo "  test-coverage-enforce - Run tests with minimum 70% coverage requirement"
	@echo "  test-coverage-detail  - Run detailed coverage by package"
	@echo "  test-race             - Run tests with race detection"
	@echo "  test-package PKG=path - Test specific package"
	@echo "  test-watch            - Run tests in watch mode (requires entr)"
	@echo "  test-api              - Run Python API tests"
	@echo "  test-federation       - Run federation tests"
	@echo "  test-search           - Run search tests"
	@echo "  test-ai               - Run AI integration tests"
	@echo "  test-auth             - Run authentication tests"
	@echo "  test-load             - Run k6 load tests"
	@echo ""
	@echo "Load Testing:"
	@echo "  k6-auth         - Test auth endpoints"
	@echo "  k6-timeline     - Test timeline performance"
	@echo "  k6-posting      - Test post creation"
	@echo "  k6-federation   - Test federation"
	@echo ""
	@echo "Development:"
	@echo "  dev             - Run local development server"
	@echo "  dev-init        - Initialize development environment"
	@echo "  fmt             - Format Go code"
	@echo "  lint            - Run linters"
	@echo "  clean           - Clean build artifacts"
	@echo ""
	@echo "Deployment:"
	@echo "  deploy          - Deploy with Pulumi"
	@echo "  deploy-preview  - Preview deployment"
	@echo "  pulumi-up       - Deploy infrastructure"
	@echo "  pulumi-destroy  - Destroy infrastructure"
	@echo "  init-deploy     - Initialize deployment"
	@echo ""
	@echo "Monitoring:"
	@echo "  logs            - Tail Lambda logs (FUNCTION=name)"
	@echo "  metrics         - Show CloudWatch metrics (METRIC=name)"
	@echo "  status          - Check system status"
	@echo ""
	@echo "Database:"
	@echo "  db-migrate      - Run database migrations"
	@echo "  db-seed         - Seed database with test data"
	@echo "  db-reset        - Reset database (destructive)"
	@echo "  local-dynamodb  - Start local DynamoDB"
	@echo ""
	@echo "Security:"
	@echo "  sec-scan        - Run security scan"
	@echo "  vuln-check      - Check for vulnerabilities"
	@echo ""
	@echo "Utilities:"
	@echo "  tidy            - Run go mod tidy"
	@echo "  vendor          - Vendor dependencies"
	@echo "  install-tools   - Install development tools"
	@echo "  docs            - Show documentation info"
	@echo "  help            - Show this help"

.PHONY: integration-test
integration-test:
	@echo "Running integration tests..."
	@$(TEST_ENV) go test -tags=integration -v -timeout=30s ./...

.PHONY: build-lambdas
build-lambdas:
	@echo "Building Lambda functions..."
	@mkdir -p bin
	@echo "Building api..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/bootstrap ./cmd/api
	@cd bin && zip -q api.zip bootstrap && rm bootstrap
	@echo "Building actor..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/bootstrap ./cmd/actor
	@cd bin && zip -q actor.zip bootstrap && rm bootstrap
	@echo "Building inbox..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/bootstrap ./cmd/inbox
	@cd bin && zip -q inbox.zip bootstrap && rm bootstrap
	@echo "Building outbox..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/bootstrap ./cmd/outbox
	@cd bin && zip -q outbox.zip bootstrap && rm bootstrap
	@echo "Building collections..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/bootstrap ./cmd/collections
	@cd bin && zip -q collections.zip bootstrap && rm bootstrap
	@echo "Building objects..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/bootstrap ./cmd/objects
	@cd bin && zip -q objects.zip bootstrap && rm bootstrap
	@echo "Building webfinger..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/bootstrap ./cmd/webfinger
	@cd bin && zip -q webfinger.zip bootstrap && rm bootstrap
	@echo "Building auth..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/bootstrap ./cmd/auth
	@cd bin && zip -q auth.zip bootstrap && rm bootstrap
	@echo "Building auth-api..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/bootstrap ./cmd/auth-api
	@cd bin && zip -q auth-api.zip bootstrap && rm bootstrap
	@echo "Building activity-processor..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/bootstrap ./cmd/activity-processor
	@cd bin && zip -q activity-processor.zip bootstrap && rm bootstrap
	@echo "Building search-indexer..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/bootstrap ./cmd/search-indexer
	@cd bin && zip -q search-indexer.zip bootstrap && rm bootstrap
	@echo "Building status-indexer..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/bootstrap ./cmd/status-indexer
	@cd bin && zip -q status-indexer.zip bootstrap && rm bootstrap
	@echo "Building push-delivery..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/bootstrap ./cmd/push-delivery
	@cd bin && zip -q push-delivery.zip bootstrap && rm bootstrap
	@echo "Building cost-aggregator..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/bootstrap ./cmd/cost-aggregator
	@cd bin && zip -q cost-aggregator.zip bootstrap && rm bootstrap
	@echo "Building graphql..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/bootstrap ./cmd/graphql
	@cd bin && zip -q graphql.zip bootstrap && rm bootstrap
	@echo "Building ai-processor..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/bootstrap ./cmd/ai-processor
	@cd bin && zip -q ai-processor.zip bootstrap && rm bootstrap
	@echo "Building streaming..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/bootstrap ./cmd/streaming
	@cd bin && zip -q streaming.zip bootstrap && rm bootstrap
	@echo "Building stream-router..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/bootstrap ./cmd/stream-router
	@cd bin && zip -q stream-router.zip bootstrap && rm bootstrap
	@echo "Building note-processor..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/bootstrap ./cmd/note-processor
	@cd bin && zip -q note-processor.zip bootstrap && rm bootstrap
	@echo "Building moderation-processor..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/bootstrap ./cmd/moderation-processor
	@cd bin && zip -q moderation-processor.zip bootstrap && rm bootstrap
	@echo "Building report-trust-updater..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/bootstrap ./cmd/report-trust-updater
	@cd bin && zip -q report-trust-updater.zip bootstrap && rm bootstrap
	@echo "Building federation-tracker..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/bootstrap ./cmd/federation-tracker
	@cd bin && zip -q federation-tracker.zip bootstrap && rm bootstrap
	@echo "Building federation-delivery..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/bootstrap ./cmd/federation-delivery
	@cd bin && zip -q federation-delivery.zip bootstrap && rm bootstrap
	@echo "Building import-processor..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/bootstrap ./cmd/import-processor
	@cd bin && zip -q import-processor.zip bootstrap && rm bootstrap
	@echo "Building export-generator..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/bootstrap ./cmd/export-generator
	@cd bin && zip -q export-generator.zip bootstrap && rm bootstrap
	@echo "Building media-processor..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/bootstrap ./cmd/media-processor
	@cd bin && zip -q media-processor.zip bootstrap && rm bootstrap
	@echo "Building trend-aggregator..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/bootstrap ./cmd/trend-aggregator
	@cd bin && zip -q trend-aggregator.zip bootstrap && rm bootstrap
	@echo "Lambda functions built successfully!"

.PHONY: deploy
deploy: build-lambdas
	@echo "Deploying with Pulumi..."
	@cd infra && pulumi up --yes

.PHONY: deploy-preview
deploy-preview: build-lambdas
	@echo "Previewing deployment..."
	@cd infra && pulumi preview

build-activity-processor:
	@echo "Building activity-processor..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/bootstrap ./cmd/activity-processor
	@cd bin && zip -q activity-processor.zip bootstrap && rm bootstrap

build-configure-instance:
	@echo "Building cmd/configure-instance..."
	@GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 \
		go build -ldflags="-s -w" -o bin/configure-instance ./cmd/configure-instance

# Build auth Lambda
build-auth:
	@echo "Building auth..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/bootstrap ./cmd/auth
	@cd bin && zip -q auth.zip bootstrap && rm bootstrap

# Build auth-api Lambda
build-auth-api:
	@echo "Building auth-api..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/bootstrap ./cmd/auth-api
	@cd bin && zip -q auth-api.zip bootstrap && rm bootstrap

# Build search-indexer Lambda
build-search-indexer:
	@echo "Building search-indexer..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/bootstrap ./cmd/search-indexer
	@cd bin && zip -q search-indexer.zip bootstrap && rm bootstrap

# Build status-indexer Lambda
build-status-indexer:
	@echo "Building status-indexer..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/bootstrap ./cmd/status-indexer
	@cd bin && zip -q status-indexer.zip bootstrap && rm bootstrap

# Generate GraphQL code
gqlgen:
	@echo "Generating GraphQL code..."
	@go run github.com/99designs/gqlgen generate

# Build GraphQL Lambda
build-graphql:
	@echo "Building graphql Lambda..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/bootstrap ./cmd/graphql
	@cd bin && zip -q graphql.zip bootstrap && rm bootstrap

# k6 Load Testing Commands
.PHONY: k6-local k6-cloud k6-stream k6-setup

# Run k6 test locally
k6-local:
	@echo "Running k6 load test locally..."
	@if [ -z "$(LESSER_URL)" ]; then \
		echo "Error: LESSER_URL environment variable is not set"; \
		echo "Please set it: export LESSER_URL=https://your-instance.com"; \
		exit 1; \
	fi
	@if [ -z "$(LESSER_TOKEN)" ]; then \
		echo "Warning: LESSER_TOKEN not set, running without authentication"; \
	fi
	k6 run tests/load/lesser_load_test.js

# Run k6 test on Grafana Cloud
k6-cloud:
	@echo "Running k6 load test on Grafana Cloud..."
	@if [ -z "$(LESSER_URL)" ]; then \
		echo "Error: LESSER_URL environment variable is not set"; \
		echo "Please set it: export LESSER_URL=https://your-instance.com"; \
		exit 1; \
	fi
	k6 cloud run tests/load/lesser_load_test.js

# Run locally but stream results to Grafana Cloud
k6-stream:
	@echo "Running k6 locally with cloud streaming..."
	@if [ -z "$(LESSER_URL)" ]; then \
		echo "Error: LESSER_URL environment variable is not set"; \
		echo "Please set it: export LESSER_URL=https://your-instance.com"; \
		exit 1; \
	fi
	k6 run -o cloud tests/load/lesser_load_test.js

# Setup k6 environment file template
k6-setup:
	@echo "Creating k6 environment template..."
	@if [ ! -f .env.k6 ]; then \
		echo "# Lesser instance configuration" > .env.k6; \
		echo "export LESSER_URL=\"https://your-lesser-instance.com\"" >> .env.k6; \
		echo "export LESSER_TOKEN=\"your-access-token\"" >> .env.k6; \
		echo "" >> .env.k6; \
		echo "# Grafana Cloud k6 configuration (optional)" >> .env.k6; \
		echo "export K6_PROJECT_ID=\"your-project-id\"" >> .env.k6; \
		echo "" >> .env.k6; \
		echo "Created .env.k6 - Please update with your actual values"; \
		echo "Then run: source .env.k6"; \
	else \
		echo ".env.k6 already exists"; \
	fi 

# Initial deployment setup - generates VAPID keys and admin account
.PHONY: init-deploy
init-deploy:
	@echo "Building init-deploy tool..."
	@GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) \
		go build -ldflags="-s -w" -o bin/init-deploy ./cmd/init-deploy
	@echo "Running initial deployment setup..."
	@if [ -z "$(DOMAIN)" ]; then \
		echo "Error: DOMAIN environment variable is required"; \
		echo "Usage: make init-deploy DOMAIN=your-domain.com"; \
		exit 1; \
	fi
	@bin/init-deploy $(DOMAIN)

# Build init-deploy tool only
.PHONY: build-init-deploy
build-init-deploy:
	@echo "Building init-deploy tool..."
	@GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) \
		go build -ldflags="-s -w" -o bin/init-deploy ./cmd/init-deploy