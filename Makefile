.PHONY: help build clean test deploy status destroy

# =============================================================================
# CONFIGURATION
# =============================================================================

# Build configuration
GOOS ?= linux
GOARCH ?= arm64
CGO_ENABLED ?= 0

# Detect OS for Windows compatibility
ifeq ($(OS),Windows_NT)
    DETECTED_OS := Windows
else
    DETECTED_OS := $(shell uname -s)
endif

# All Lambda functions to build
LAMBDAS := \
	activity-processor \
	actor \
	ai-processor \
	api \
	collections \
	cost-aggregator \
	dlq-processor \
	enhanced-federation-processor \
	export-generator \
	federation-aggregator \
	federation-delivery \
	federation-timeseries \
	federation-tracker \
	graphql \
	import-processor \
	inbox \
	media-processor \
	metrics-aggregator \
	metrics-processor \
	moderation-processor \
	note-processor \
	notification-processor \
	objects \
	outbox \
	push-delivery \
	report-trust-updater \
	search-indexer \
	status-indexer \
	stream-router \
	streaming \
	trend-aggregator \
	webfinger \
	websocket-cost-aggregator

# Environment configurations
ENV ?= dev
ENV_MAP_dev = development
ENV_MAP_test = staging
ENV_MAP_staging = staging
ENV_MAP_live = production
ENV_MAP_production = production
CDK_ENV = $(ENV_MAP_$(ENV))

# =============================================================================
# BUILD TARGETS
# =============================================================================

## Build all Lambda functions for deployment (incremental - only if missing)
build-lambdas:
	@mkdir -p bin
	@BUILT=0; \
	SKIPPED=0; \
	for lambda in $(LAMBDAS); do \
		if [ ! -f "bin/$$lambda.zip" ]; then \
			echo "Building $$lambda..."; \
			GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) \
				go build -ldflags="-s -w" -o bin/bootstrap ./cmd/$$lambda && \
			cd bin && zip -q $$lambda.zip bootstrap && rm bootstrap && cd .. || exit 1; \
			BUILT=$$((BUILT + 1)); \
		else \
			SKIPPED=$$((SKIPPED + 1)); \
		fi; \
	done; \
	if [ $$BUILT -gt 0 ]; then \
		echo "✓ Built $$BUILT Lambda function(s), skipped $$SKIPPED (already exist)"; \
	else \
		echo "✓ All $(words $(LAMBDAS)) Lambda functions already built (use 'make rebuild-lambdas' to force rebuild)"; \
	fi

## Force rebuild all Lambda functions (ignores existing binaries)
rebuild-lambdas:
	@echo "Force rebuilding all Lambda functions..."
	@rm -f bin/*.zip
	@$(MAKE) build-lambdas

## Build a specific Lambda function
build-%:
	@echo "Building $*..."
	@mkdir -p bin
	@GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) \
		go build -ldflags="-s -w" -o bin/bootstrap ./cmd/$*
	@cd bin && zip -q $*.zip bootstrap && rm bootstrap
	@echo "✓ Built $*.zip"

## Build all Lambda functions (legacy/local)
build:
	@echo "Building Lambda functions for local use..."
	@mkdir -p bin
	@for lambda in $(LAMBDAS); do \
		echo "Building cmd/$$lambda..."; \
		GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) \
			go build -ldflags="-s -w" -o bin/$$lambda ./cmd/$$lambda || exit 1; \
	done
	@echo "✓ Build complete"

## Build CloudFront key generation Lambda (for CDK custom resource)
build-cloudfront-keygen:
	@echo "Building CloudFront key generation Lambda..."
	@mkdir -p bin
	@GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) \
		go build -tags lambda.norpc -ldflags="-s -w" -o bin/cloudfront-keygen ./cmd/cloudfront-keygen
	@echo "✓ Built bin/cloudfront-keygen ($(shell ls -lh bin/cloudfront-keygen 2>/dev/null | awk '{print $$5}'))"

## Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin/
	@rm -f coverage.out coverage.html
	@echo "✓ Clean complete"

# =============================================================================
# CDK DEPLOYMENT TARGETS
# =============================================================================

## Bootstrap CDK in your AWS account (required once per account/region)
cdk-bootstrap:
	@echo "Bootstrapping CDK..."
	@if [ -n "$(AWS_PROFILE)" ]; then \
		echo "Using AWS profile: $(AWS_PROFILE)"; \
		ACCOUNT=$$(AWS_PROFILE=$(AWS_PROFILE) aws sts get-caller-identity --query Account --output text 2>/dev/null); \
		if [ -z "$$ACCOUNT" ]; then \
			echo "Error: Failed to get account ID from AWS profile"; \
			echo "Make sure you're logged in: aws sso login --profile $(AWS_PROFILE)"; \
			exit 1; \
		fi; \
		AWS_REGION=$${AWS_REGION:-us-east-1}; \
		echo "Bootstrapping CDK in account $$ACCOUNT region $$AWS_REGION..."; \
		AWS_PROFILE=$(AWS_PROFILE) cdk bootstrap aws://$$ACCOUNT/$$AWS_REGION; \
	elif [ -n "$(AWS_ACCOUNT)" ]; then \
		AWS_REGION=$${AWS_REGION:-us-east-1}; \
		echo "Bootstrapping CDK in account $(AWS_ACCOUNT) region $$AWS_REGION..."; \
		cdk bootstrap aws://$(AWS_ACCOUNT)/$$AWS_REGION; \
	else \
		echo "Error: Either AWS_PROFILE or AWS_ACCOUNT is required"; \
		echo ""; \
		echo "For AWS SSO users:"; \
		echo "  make cdk-bootstrap AWS_PROFILE=your-profile-name"; \
		echo ""; \
		echo "For standard credentials:"; \
		echo "  make cdk-bootstrap AWS_ACCOUNT=123456789012"; \
		exit 1; \
	fi

## Show what will be deployed without actually deploying
cdk-diff:
	@echo "Showing deployment diff for $(CDK_ENV) environment..."
	@cd infra/cdk && cdk diff --all --context environment=$(CDK_ENV)

## Generate CloudFormation template
cdk-synth:
	@echo "Synthesizing CloudFormation template for $(CDK_ENV)..."
	@cd infra/cdk && cdk synth --context environment=$(CDK_ENV)

## List all CDK stacks
cdk-list:
	@echo "Listing all CDK stacks..."
	@cd infra/cdk && cdk list

# =============================================================================
# SHARED RESOURCES DEPLOYMENT
# =============================================================================

## Deploy shared resources (KMS, Secrets) - ONCE for all environments
deploy-shared:
	@echo "Deploying shared resources (used by ALL environments)..."
	@cd infra/cdk && cdk deploy LesserSharedStack \
		--context environment=development \
		--require-approval never
	@echo "✓ Shared resources deployed"
	@echo ""
	@echo "These resources are now available to all environments:"
	@echo "  - KMS Key: alias/lesser-encryption"
	@echo "  - Actor Private Key: lesser/actor-private-key"
	@echo "  - JWT Secret: lesser/jwt-secret (auto-generated)"

## Check if shared resources exist
check-shared:
	@echo "Checking if shared resources are deployed..."
	@if aws cloudformation describe-stacks --stack-name LesserSharedStack --profile $(AWS_PROFILE) >/dev/null 2>&1; then \
		echo "✓ LesserSharedStack exists"; \
	else \
		echo "✗ LesserSharedStack does not exist"; \
		echo ""; \
		echo "Run 'make deploy-shared' first to create shared resources"; \
		exit 1; \
	fi

# =============================================================================
# ENVIRONMENT-SPECIFIC DEPLOYMENT
# =============================================================================

## Deploy to development environment
deploy-dev: ENV=dev
deploy-dev: build-lambdas build-cloudfront-keygen check-shared
	@echo "Deploying to DEVELOPMENT environment..."
	@echo "Step 1/2: Deploying monitoring stack..."
	@cd infra/cdk && cdk deploy LesserMonitoringStack-development \
		--context environment=development \
		--require-approval never
	@echo "Step 2/2: Deploying application stack..."
	@cd infra/cdk && cdk deploy LesserStack-development \
		--context environment=development \
		--require-approval never
	@echo "✓ Development deployment complete"

## Deploy to test/staging environment
deploy-test: ENV=test
deploy-test: build-lambdas build-cloudfront-keygen check-shared
	@echo "Deploying to TEST/STAGING environment..."
	@if [ -z "$(DOMAIN)" ]; then \
		echo "Error: DOMAIN is required for staging"; \
		echo "Usage: make deploy-test DOMAIN=staging.yourdomain.com"; \
		exit 1; \
	fi
	@echo "Step 1/2: Deploying monitoring stack..."
	@cd infra/cdk && cdk deploy LesserMonitoringStack-staging \
		--context environment=staging \
		--context domain=$(DOMAIN) \
		--require-approval broadening
	@echo "Step 2/2: Deploying application stack..."
	@cd infra/cdk && cdk deploy LesserStack-staging \
		--context environment=staging \
		--context domain=$(DOMAIN) \
		--require-approval broadening
	@echo "✓ Staging deployment complete"

## Deploy to live/production environment
deploy-live: ENV=live
deploy-live: build-lambdas build-cloudfront-keygen check-shared
	@echo "Deploying to LIVE/PRODUCTION environment..."
	@if [ -z "$(DOMAIN)" ]; then \
		echo "Error: DOMAIN is required for production"; \
		echo "Usage: make deploy-live DOMAIN=yourdomain.com"; \
		exit 1; \
	fi
	@echo "Step 1/2: Deploying monitoring stack..."
	@cd infra/cdk && cdk deploy LesserMonitoringStack-production \
		--context environment=production \
		--context domain=$(DOMAIN) \
		--require-approval broadening
	@echo "Step 2/2: Deploying application stack..."
	@cd infra/cdk && cdk deploy LesserStack-production \
		--context environment=production \
		--context domain=$(DOMAIN) \
		--require-approval broadening
	@echo "✓ Production deployment complete"

## Generic deploy command (use ENV variable to specify environment)
deploy: build-lambdas build-cloudfront-keygen
	@if [ "$(ENV)" = "live" ] || [ "$(ENV)" = "production" ]; then \
		$(MAKE) deploy-live DOMAIN=$(DOMAIN); \
	elif [ "$(ENV)" = "test" ] || [ "$(ENV)" = "staging" ]; then \
		$(MAKE) deploy-test DOMAIN=$(DOMAIN); \
	else \
		$(MAKE) deploy-dev; \
	fi

# =============================================================================
# DEPLOYMENT STATUS & MANAGEMENT
# =============================================================================

## Show deployment status for all environments
status:
	@echo "=== Lesser Deployment Status ==="
	@echo ""
	@echo "Shared Resources:"
	@if aws cloudformation describe-stacks --stack-name LesserSharedStack >/dev/null 2>&1; then \
		aws cloudformation describe-stacks --stack-name LesserSharedStack \
			--query 'Stacks[0].{Name:StackName,Status:StackStatus,Updated:LastUpdatedTime}' \
			--output table; \
	else \
		echo "✗ LesserSharedStack not deployed"; \
	fi
	@echo ""
	@echo "Lambda Functions:"
	@aws lambda list-functions \
		--query 'Functions[?starts_with(FunctionName, `lesser-`)].{Name:FunctionName,Runtime:Runtime,Memory:MemorySize,Timeout:Timeout,LastModified:LastModified}' \
		--output table 2>/dev/null || echo "No Lambda functions found or AWS credentials not configured"
	@echo ""
	@echo "DynamoDB Tables:"
	@aws dynamodb list-tables \
		--query 'TableNames[?contains(@, `lesser`)]' \
		--output table 2>/dev/null || echo "No DynamoDB tables found"
	@echo ""
	@echo "CloudFormation Stacks:"
	@aws cloudformation list-stacks \
		--stack-status-filter CREATE_COMPLETE UPDATE_COMPLETE \
		--query 'StackSummaries[?starts_with(StackName, `Lesser`)].{Stack:StackName,Status:StackStatus,Updated:LastUpdatedTime}' \
		--output table 2>/dev/null || echo "No CloudFormation stacks found"

## Show status for specific environment
status-dev: ENV=dev
status-dev:
	@echo "=== Development Environment Status ==="
	@$(MAKE) _status-env CDK_ENV=development

status-test: ENV=test
status-test:
	@echo "=== Test/Staging Environment Status ==="
	@$(MAKE) _status-env CDK_ENV=staging

status-live: ENV=live
status-live:
	@echo "=== Live/Production Environment Status ==="
	@$(MAKE) _status-env CDK_ENV=production

_status-env:
	@echo ""
	@echo "CloudFormation Stacks:"
	@aws cloudformation describe-stacks \
		--query 'Stacks[?contains(StackName, `$(CDK_ENV)`)].{Name:StackName,Status:StackStatus,Updated:LastUpdatedTime}' \
		--output table 2>/dev/null || echo "No stacks found for $(CDK_ENV)"
	@echo ""
	@echo "Lambda Functions:"
	@aws lambda list-functions \
		--query 'Functions[?contains(FunctionName, `$(CDK_ENV)`)].{Name:FunctionName,Memory:MemorySize,Timeout:Timeout,LastModified:LastModified}' \
		--output table 2>/dev/null || echo "No functions found"

## Show CloudWatch dashboard URL for environment
dashboard:
	@if [ "$(ENV)" = "live" ] || [ "$(ENV)" = "production" ]; then \
		ENV_NAME="production"; \
	elif [ "$(ENV)" = "test" ] || [ "$(ENV)" = "staging" ]; then \
		ENV_NAME="staging"; \
	else \
		ENV_NAME="development"; \
	fi; \
	REGION=$${AWS_REGION:-us-east-1}; \
	echo "CloudWatch Dashboard URL:"; \
	echo "https://console.aws.amazon.com/cloudwatch/home?region=$$REGION#dashboards:name=lesser-$$ENV_NAME"

# =============================================================================
# DESTROY/TEARDOWN TARGETS
# =============================================================================

## Destroy shared resources (WARNING: affects ALL environments!)
destroy-shared:
	@echo "⚠️  DANGER: This will destroy shared resources used by ALL environments!"
	@echo "This will affect: development, staging, AND production!"
	@read -p "Type 'DELETE SHARED' to confirm: " confirm && [ "$$confirm" = "DELETE SHARED" ] || exit 1
	@echo "Destroying shared resources..."
	@cd infra/cdk && cdk destroy LesserSharedStack \
		--context environment=development \
		--force

## Destroy development environment
destroy-dev:
	@echo "WARNING: This will destroy the DEVELOPMENT environment!"
	@echo "Note: Shared resources (KMS, Secrets) will NOT be destroyed"
	@read -p "Are you sure? Type 'yes' to confirm: " confirm && [ "$$confirm" = "yes" ] || exit 1
	@echo "Destroying development environment..."
	@cd infra/cdk && cdk destroy LesserStack-development LesserMonitoringStack-development \
		--context environment=development \
		--force

## Destroy test/staging environment
destroy-test:
	@echo "WARNING: This will destroy the TEST/STAGING environment!"
	@echo "Note: Shared resources (KMS, Secrets) will NOT be destroyed"
	@read -p "Are you sure? Type 'yes' to confirm: " confirm && [ "$$confirm" = "yes" ] || exit 1
	@echo "Destroying staging environment..."
	@cd infra/cdk && cdk destroy LesserStack-staging LesserMonitoringStack-staging \
		--context environment=staging \
		--force

## Destroy live/production environment
destroy-live:
	@echo "⚠️  DANGER: This will destroy the LIVE/PRODUCTION environment!"
	@echo "This action is IRREVERSIBLE and will delete all production data!"
	@echo "Note: Shared resources (KMS, Secrets) will NOT be destroyed"
	@read -p "Type 'DELETE PRODUCTION' to confirm: " confirm && [ "$$confirm" = "DELETE PRODUCTION" ] || exit 1
	@echo "Destroying production environment..."
	@cd infra/cdk && cdk destroy LesserStack-production LesserMonitoringStack-production \
		--context environment=production \
		--force

## Generic destroy command (use ENV variable)
destroy:
	@if [ "$(ENV)" = "live" ] || [ "$(ENV)" = "production" ]; then \
		$(MAKE) destroy-live; \
	elif [ "$(ENV)" = "test" ] || [ "$(ENV)" = "staging" ]; then \
		$(MAKE) destroy-test; \
	else \
		$(MAKE) destroy-dev; \
	fi

# =============================================================================
# MONITORING & DEBUGGING
# =============================================================================

## Tail logs for a specific Lambda function
logs:
	@if [ -z "$(FUNCTION)" ]; then \
		echo "Error: FUNCTION is required"; \
		echo "Usage: make logs FUNCTION=api ENV=dev"; \
		echo ""; \
		echo "Available functions:"; \
		for lambda in $(LAMBDAS); do echo "  - $$lambda"; done; \
		exit 1; \
	fi
	@if [ "$(ENV)" = "live" ] || [ "$(ENV)" = "production" ]; then \
		ENV_NAME="production"; \
	elif [ "$(ENV)" = "test" ] || [ "$(ENV)" = "staging" ]; then \
		ENV_NAME="staging"; \
	else \
		ENV_NAME="development"; \
	fi; \
	echo "Tailing logs for lesser-$$ENV_NAME-$(FUNCTION)..."; \
	aws logs tail /aws/lambda/lesser-$$ENV_NAME-$(FUNCTION) --follow

## Show CloudWatch metrics for a Lambda function
metrics:
	@if [ -z "$(FUNCTION)" ]; then \
		echo "Error: FUNCTION is required"; \
		echo "Usage: make metrics FUNCTION=api ENV=dev"; \
		exit 1; \
	fi
	@if [ "$(ENV)" = "live" ] || [ "$(ENV)" = "production" ]; then \
		ENV_NAME="production"; \
	elif [ "$(ENV)" = "test" ] || [ "$(ENV)" = "staging" ]; then \
		ENV_NAME="staging"; \
	else \
		ENV_NAME="development"; \
	fi; \
	echo "Fetching metrics for lesser-$$ENV_NAME-$(FUNCTION)..."; \
	aws cloudwatch get-metric-statistics \
		--namespace AWS/Lambda \
		--metric-name Invocations \
		--dimensions Name=FunctionName,Value=lesser-$$ENV_NAME-$(FUNCTION) \
		--start-time $$(date -u -d '1 hour ago' +%Y-%m-%dT%H:%M:%S) \
		--end-time $$(date -u +%Y-%m-%dT%H:%M:%S) \
		--period 300 \
		--statistics Sum,Average

## Show recent errors for environment
errors:
	@if [ "$(ENV)" = "live" ] || [ "$(ENV)" = "production" ]; then \
		ENV_NAME="production"; \
	elif [ "$(ENV)" = "test" ] || [ "$(ENV)" = "staging" ]; then \
		ENV_NAME="staging"; \
	else \
		ENV_NAME="development"; \
	fi; \
	echo "Recent errors in $$ENV_NAME environment:"; \
	aws logs filter-log-events \
		--log-group-name /aws/lambda/lesser-$$ENV_NAME-api \
		--filter-pattern "ERROR" \
		--max-items 10 \
		--output text 2>/dev/null || echo "No errors found or log group doesn't exist"

# =============================================================================
# TESTING
# =============================================================================

## Run all tests
test:
	@echo "Running tests..."
	@JWT_SECRET=$${JWT_SECRET:-dummy_value} DYNAMODB_ENCRYPTION_KEY=$${DYNAMODB_ENCRYPTION_KEY:-0123456789abcdef0123456789abcdef} go test -v ./...

## Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	@JWT_SECRET=$${JWT_SECRET:-dummy_value} DYNAMODB_ENCRYPTION_KEY=$${DYNAMODB_ENCRYPTION_KEY:-0123456789abcdef0123456789abcdef} go test -v -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

## Run tests with race detection
test-race:
	@echo "Running tests with race detection..."
	@JWT_SECRET=$${JWT_SECRET:-dummy_value} go test -race -v ./...

## Run integration tests
test-integration:
	@echo "Running integration tests..."
	@JWT_SECRET=$${JWT_SECRET:-dummy_value} TEST_ENV=integration go test -tags=integration -v -timeout=30m ./pkg/testing/harness/...

## Run unit tests only
test-unit:
	@echo "Running unit tests only..."
	@JWT_SECRET=$${JWT_SECRET:-dummy_value} go test -short -v ./...

# =============================================================================
# CODE QUALITY
# =============================================================================

## Format Go code
fmt:
	@echo "Formatting code..."
	@go fmt ./...

## Run linter
lint:
	@echo "Running linter..."
	@golangci-lint run --config .golangci.yml

## Run linter with auto-fix
lint-fix:
	@echo "Running linter with auto-fix..."
	@golangci-lint run --config .golangci.yml --fix

## Run security scan
sec-scan:
	@echo "Running security scan..."
	@gosec ./...

## Check for vulnerabilities
vuln-check:
	@echo "Checking for vulnerabilities..."
	@govulncheck ./...

# =============================================================================
# DEVELOPMENT
# =============================================================================

## Initialize local development environment
dev-init:
	@echo "Initializing development environment..."
	@if [ ! -f .env ]; then \
		echo "DOMAIN=localhost" > .env; \
		echo "INSTANCE_NAME=Lesser Dev" >> .env; \
		echo "AWS_REGION=us-east-1" >> .env; \
		echo "DYNAMO_TABLE_NAME=lesser-dev" >> .env; \
		echo "S3_BUCKET_NAME=lesser-dev-media" >> .env; \
		echo "JWT_SECRET=$$(openssl rand -base64 32)" >> .env; \
		echo "✓ Created .env file with default values"; \
	else \
		echo ".env file already exists"; \
	fi

## Run local development server
dev:
	@echo "Starting local development server..."
	@if [ ! -f .env ]; then \
		echo "No .env file found. Run 'make dev-init' first."; \
		exit 1; \
	fi
	@export $$(cat .env | grep -v '^#' | xargs) && go run ./cmd/api

## Run local DynamoDB for development
local-dynamodb:
	@echo "Starting local DynamoDB..."
	@docker run -p 8000:8000 amazon/dynamodb-local

## Generate GraphQL code
gqlgen:
	@echo "Generating GraphQL code..."
	@go run github.com/99designs/gqlgen generate

## Tidy Go modules
tidy:
	@echo "Tidying Go modules..."
	@go mod tidy

## Install development tools
install-tools:
	@echo "Installing development tools..."
	@go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	@go install github.com/securego/gosec/v2/cmd/gosec@latest
	@go install golang.org/x/vuln/cmd/govulncheck@latest
	@go install github.com/99designs/gqlgen@latest
	@npm install -g aws-cdk@latest
	@echo "✓ Development tools installed successfully!"

# =============================================================================
# UTILITIES
# =============================================================================

## Initial deployment setup (generates VAPID keys and admin account)
init-deploy:
	@echo "Building init-deploy tool..."
	@GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) \
		go build -ldflags="-s -w" -o bin/init-deploy ./cmd/init-deploy
	@if [ -z "$(DOMAIN)" ]; then \
		echo "Error: DOMAIN is required"; \
		echo "Usage: make init-deploy DOMAIN=your-domain.com"; \
		exit 1; \
	fi
	@echo "Running initial deployment setup..."
	@bin/init-deploy $(DOMAIN)

## Show deployment outputs (API endpoints, etc.)
outputs:
	@if [ "$(ENV)" = "live" ] || [ "$(ENV)" = "production" ]; then \
		STACK_NAME="LesserStack-production"; \
	elif [ "$(ENV)" = "test" ] || [ "$(ENV)" = "staging" ]; then \
		STACK_NAME="LesserStack-staging"; \
	else \
		STACK_NAME="LesserStack-development"; \
	fi; \
	echo "Stack outputs for $$STACK_NAME:"; \
	aws cloudformation describe-stacks \
		--stack-name $$STACK_NAME \
		--query 'Stacks[0].Outputs' \
		--output table 2>/dev/null || echo "Stack not found"

## Validate all Lambda binaries exist
validate-build:
	@echo "Validating Lambda builds..."
	@MISSING=0; \
	for lambda in $(LAMBDAS); do \
		if [ ! -f "bin/$$lambda.zip" ]; then \
			echo "✗ Missing: bin/$$lambda.zip"; \
			MISSING=$$((MISSING + 1)); \
		fi; \
	done; \
	if [ $$MISSING -eq 0 ]; then \
		echo "✓ All $(words $(LAMBDAS)) Lambda functions are built"; \
	else \
		echo "✗ Missing $$MISSING Lambda function(s)"; \
		exit 1; \
	fi

## Show estimated AWS costs
cost-estimate:
	@echo "Note: Cost estimation requires AWS Cost Explorer API"
	@echo "Fetching cost data for Lesser resources..."
	@aws ce get-cost-and-usage \
		--time-period Start=$$(date -u -d '30 days ago' +%Y-%m-%d),End=$$(date -u +%Y-%m-%d) \
		--granularity MONTHLY \
		--metrics BlendedCost \
		--group-by Type=TAG,Key=Project \
		--filter file://<(echo '{"Tags":{"Key":"Project","Values":["lesser"]}}') 2>/dev/null || \
		echo "Cost data not available. Enable Cost Explorer in AWS Console."

# =============================================================================
# HELP
# =============================================================================

## Show this help message
help:
	@echo "Lesser - Serverless ActivityPub CDK Deployment"
	@echo ""
	@echo "Usage: make [target] [ENV=dev|test|live]"
	@echo ""
	@echo "BUILD TARGETS:"
	@echo "  build-lambdas       Build Lambda functions (incremental - skips existing)"
	@echo "  rebuild-lambdas     Force rebuild all Lambda functions"
	@echo "  build-<function>    Build a specific Lambda function"
	@echo "  build               Build all functions for local use"
	@echo "  clean               Clean build artifacts"
	@echo ""
	@echo "CDK COMMANDS:"
	@echo "  cdk-bootstrap       Bootstrap CDK (required once per account)"
	@echo "  cdk-diff            Show deployment changes without deploying"
	@echo "  cdk-synth           Generate CloudFormation template"
	@echo "  cdk-list            List all CDK stacks"
	@echo ""
	@echo "DEPLOYMENT:"
	@echo "  deploy-shared       Deploy shared resources (KMS, Secrets, JWT) - ONCE for all envs"
	@echo "  deploy-dev          Deploy to development environment"
	@echo "  deploy-test         Deploy to test/staging (requires DOMAIN=...)"
	@echo "  deploy-live         Deploy to production (requires DOMAIN=...)"
	@echo "  deploy              Deploy to environment specified by ENV variable"
	@echo "  check-shared        Check if shared resources are deployed"
	@echo ""
	@echo "STATUS & MONITORING:"
	@echo "  status              Show status of all deployments"
	@echo "  status-dev          Show development environment status"
	@echo "  status-test         Show test/staging environment status"
	@echo "  status-live         Show production environment status"
	@echo "  dashboard           Show CloudWatch dashboard URL"
	@echo "  logs                Tail Lambda logs (requires FUNCTION=... ENV=...)"
	@echo "  metrics             Show CloudWatch metrics (requires FUNCTION=... ENV=...)"
	@echo "  errors              Show recent errors for environment"
	@echo "  outputs             Show CloudFormation stack outputs"
	@echo ""
	@echo "TEARDOWN:"
	@echo "  destroy-shared      Destroy shared resources (affects ALL environments!)"
	@echo "  destroy-dev         Destroy development environment"
	@echo "  destroy-test        Destroy test/staging environment"
	@echo "  destroy-live        Destroy production environment (DANGEROUS!)"
	@echo "  destroy             Destroy environment specified by ENV variable"
	@echo ""
	@echo "TESTING:"
	@echo "  test                Run all tests"
	@echo "  test-coverage       Run tests with coverage report"
	@echo "  test-race           Run tests with race detection"
	@echo "  test-integration    Run integration tests"
	@echo "  test-unit           Run unit tests only"
	@echo ""
	@echo "CODE QUALITY:"
	@echo "  fmt                 Format Go code"
	@echo "  lint                Run linter"
	@echo "  lint-fix            Run linter with auto-fix"
	@echo "  sec-scan            Run security scan"
	@echo "  vuln-check          Check for vulnerabilities"
	@echo ""
	@echo "DEVELOPMENT:"
	@echo "  dev-init            Initialize local development environment"
	@echo "  dev                 Run local development server"
	@echo "  local-dynamodb      Start local DynamoDB container"
	@echo "  gqlgen              Generate GraphQL code"
	@echo "  tidy                Tidy Go modules"
	@echo "  install-tools       Install development tools"
	@echo ""
	@echo "UTILITIES:"
	@echo "  init-deploy         Initialize deployment (requires DOMAIN=...)"
	@echo "  validate-build      Verify all Lambda functions are built"
	@echo "  cost-estimate       Show estimated AWS costs"
	@echo "  help                Show this help message"
	@echo ""
	@echo "EXAMPLES:"
	@echo "  # AWS SSO Users (First Time Setup):"
	@echo "  aws sso login --profile my-profile"
	@echo "  make cdk-bootstrap AWS_PROFILE=my-profile"
	@echo "  AWS_PROFILE=my-profile make deploy-shared      # Deploy once for all envs"
	@echo "  AWS_PROFILE=my-profile make deploy-dev         # Deploy dev environment"
	@echo ""
	@echo "  # Standard AWS Credentials:"
	@echo "  make cdk-bootstrap AWS_ACCOUNT=123456789012"
	@echo "  make deploy-dev"
	@echo ""
	@echo "  # Other examples:"
	@echo "  make build-lambdas                    # Build functions (incremental)"
	@echo "  make rebuild-lambdas                  # Force rebuild all functions"
	@echo "  make deploy-test DOMAIN=test.app.com  # Deploy to staging"
	@echo "  make deploy-live DOMAIN=app.com       # Deploy to prod (JWT auto-generated)"
	@echo "  make status-live                      # Check production status"
	@echo "  make logs FUNCTION=api ENV=dev        # Tail API logs in dev"
	@echo "  make destroy-dev                      # Tear down dev environment"
	@echo ""
	@echo "Available Lambda Functions ($(words $(LAMBDAS)) total):"
	@for lambda in $(LAMBDAS); do echo "  - $$lambda"; done

# Default target
.DEFAULT_GOAL := help
