.PHONY: build test clean deploy fmt lint install-tools

# Variables
GOOS ?= linux
GOARCH ?= arm64
CGO_ENABLED ?= 0

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

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Run linter
lint:
	@echo "Running linter..."
	golangci-lint run

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

# Help
help:
	@echo "Available targets:"
	@echo "  build           - Build all Lambda functions"
	@echo "  build-<name>    - Build specific Lambda function"
	@echo "  test            - Run tests"
	@echo "  test-coverage   - Run tests with coverage"
	@echo "  fmt             - Format code"
	@echo "  lint            - Run linter"
	@echo "  clean           - Clean build artifacts"
	@echo "  install-tools   - Install development tools"
	@echo "  package         - Create Lambda deployment packages"
	@echo "  deploy-infra    - Deploy infrastructure with Pulumi"
	@echo "  deploy-functions - Deploy Lambda functions"
	@echo "  local-dynamodb  - Start local DynamoDB"
	@echo "  dev-init        - Initialize development environment"
	@echo "  tidy            - Run go mod tidy"
	@echo "  vendor          - Vendor dependencies"
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