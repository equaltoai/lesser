.PHONY: build test clean deploy fmt lint install-tools

# Variables
GOOS ?= linux
GOARCH ?= amd64
CGO_ENABLED ?= 0

# Lambda function directories
LAMBDAS := webfinger actor inbox outbox collections activity-processor media

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
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/api ./cmd/api
	@cd bin && zip -q api.zip api && rm api
	@echo "Building actor..."
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/actor ./cmd/actor
	@cd bin && zip -q actor.zip actor && rm actor
	@echo "Building inbox..."
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/inbox ./cmd/inbox
	@cd bin && zip -q inbox.zip inbox && rm inbox
	@echo "Building outbox..."
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/outbox ./cmd/outbox
	@cd bin && zip -q outbox.zip outbox && rm outbox
	@echo "Building collections..."
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/collections ./cmd/collections
	@cd bin && zip -q collections.zip collections && rm collections
	@echo "Building objects..."
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/objects ./cmd/objects
	@cd bin && zip -q objects.zip objects && rm objects
	@echo "Building webfinger..."
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/webfinger ./cmd/webfinger
	@cd bin && zip -q webfinger.zip webfinger && rm webfinger
	@echo "Building auth..."
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/auth ./cmd/auth
	@cd bin && zip -q auth.zip auth && rm auth
	@echo "Building media..."
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/media ./cmd/media
	@cd bin && zip -q media.zip media && rm media
	@echo "Building activity-processor..."
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/activity-processor ./cmd/activity-processor
	@cd bin && zip -q activity-processor.zip activity-processor && rm activity-processor
	@echo "Lambda functions built successfully!"

.PHONY: deploy
deploy: build-lambdas
	@echo "Deploying with Pulumi..."
	@cd infra && pulumi up

.PHONY: deploy-preview
deploy-preview: build-lambdas
	@echo "Previewing deployment..."
	@cd infra && pulumi preview 