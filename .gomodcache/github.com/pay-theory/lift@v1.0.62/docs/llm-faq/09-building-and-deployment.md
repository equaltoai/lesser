# Best Practices for Building and Deploying Lift Applications

## Overview

This comprehensive guide covers best practices for building, testing, and deploying Lift applications to AWS Lambda. It includes build optimization, CI/CD patterns, deployment strategies, and production readiness checklist.

## Building Lift Applications

### Build Process for Lambda

Lambda requires a specific build process for Go applications:

#### For ARM64 (Recommended - Better Price/Performance)

```bash
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build \
  -tags lambda.norpc \
  -ldflags="-s -w" \
  -o dist/bootstrap \
  cmd/main.go
```

#### For x86_64 (AMD64)

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build \
  -tags lambda.norpc \
  -ldflags="-s -w" \
  -o dist/bootstrap \
  cmd/main.go
```

### Build Flags Explained

- `GOOS=linux` - Target Linux operating system (Lambda runtime)
- `GOARCH=arm64` - Target ARM64 architecture (Graviton2)
- `CGO_ENABLED=0` - Disable CGO for static binary
- `-tags lambda.norpc` - Optimize for Lambda RPC
- `-ldflags="-s -w"` - Strip debug info, reduce binary size
- `-o dist/bootstrap` - Output filename must be "bootstrap"
- `cmd/main.go` - Entry point

### Build Automation with Makefile

Create a `Makefile` for consistent builds:

```makefile
.PHONY: build build-x86 test lint clean deploy package

# Default Go settings
GO := go
GOFLAGS := -v
LDFLAGS := -ldflags="-s -w"
BUILD_DIR := dist

# Build for ARM64 (recommended)
build:
	@echo "Building for Lambda ARM64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GO) build \
		$(GOFLAGS) \
		-tags lambda.norpc \
		$(LDFLAGS) \
		-o $(BUILD_DIR)/bootstrap \
		cmd/main.go
	@echo "Build complete: $(BUILD_DIR)/bootstrap"
	@echo "Binary size: $$(du -h $(BUILD_DIR)/bootstrap | cut -f1)"

# Build for x86_64
build-x86:
	@echo "Building for Lambda x86_64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build \
		$(GOFLAGS) \
		-tags lambda.norpc \
		$(LDFLAGS) \
		-o $(BUILD_DIR)/bootstrap \
		cmd/main.go
	@echo "Build complete: $(BUILD_DIR)/bootstrap"

# Run tests
test:
	@echo "Running tests..."
	$(GO) test -v -cover ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GO) test -v -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Run linter
lint:
	@echo "Running linter..."
	golangci-lint run ./...

# Format code
fmt:
	@echo "Formatting code..."
	$(GO) fmt ./...
	goimports -w .

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

# Package for deployment
package: build
	@echo "Packaging..."
	cd $(BUILD_DIR) && zip function.zip bootstrap
	@echo "Package created: $(BUILD_DIR)/function.zip"

# Deploy with CDK
deploy: build
	@echo "Deploying with CDK..."
	cd cdk && cdk deploy

# Install dependencies
deps:
	@echo "Installing dependencies..."
	$(GO) mod download
	$(GO) mod tidy

# Verify build
verify: build
	@echo "Verifying build..."
	@file $(BUILD_DIR)/bootstrap
	@echo "Binary size: $$(du -h $(BUILD_DIR)/bootstrap | cut -f1)"
```

Usage:

```bash
make build          # Build for ARM64
make test           # Run tests
make test-coverage  # Generate coverage report
make lint           # Run linter
make package        # Build and package
make deploy         # Deploy with CDK
```

### Build Optimization

#### 1. Reduce Binary Size

```makefile
# Basic optimization
LDFLAGS := -ldflags="-s -w"

# Advanced optimization (may affect debugging)
LDFLAGS := -ldflags="-s -w -X main.version=$(VERSION)"

# Ultra-compact build (experimental)
build-compact:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build \
		-tags lambda.norpc,netgo \
		-ldflags="-s -w -extldflags '-static'" \
		-trimpath \
		-o dist/bootstrap \
		cmd/main.go
	upx --best --lzma dist/bootstrap  # Requires UPX
```

#### 2. Dependency Management

```bash
# Remove unused dependencies
go mod tidy

# Vendor dependencies for reproducible builds
go mod vendor

# Verify dependencies
go mod verify
```

#### 3. Build Cache

```bash
# Use build cache for faster rebuilds
go build -i -o dist/bootstrap cmd/main.go

# Clean cache if needed
go clean -cache
```

## Testing Before Deployment

### Local Testing

#### CDK Local Testing

```bash
# Install dependencies
npm install -g aws-cdk

# Initialize CDK (if not already done)
cd cdk && cdk init

# Deploy infrastructure
make deploy
```

#### Create Test Events

```json
// tests/events/api-request.json
{
  "httpMethod": "POST",
  "path": "/api/v1/users",
  "headers": {
    "Content-Type": "application/json",
    "Authorization": "Bearer test-token"
  },
  "body": "{\"name\":\"John Doe\",\"email\":\"john@example.com\"}"
}
```

### Unit Testing

```go
// internal/handlers/users_test.go
package handlers_test

import (
    "testing"
    
    "github.com/pay-theory/lift/pkg/testing"
    "github.com/stretchr/testify/assert"
    
    "github.com/yourcompany/my-app/internal/handlers"
)

func TestGetUser(t *testing.T) {
    // Create test context
    ctx := testing.NewTestContext(
        testing.WithMethod("GET"),
        testing.WithPath("/users/123"),
        testing.WithParam("id", "123"),
    )
    
    // Execute handler
    err := handlers.GetUser(ctx)
    
    // Assertions
    assert.NoError(t, err)
    assert.Equal(t, 200, ctx.Response.StatusCode)
}
```

### Integration Testing

```go
// tests/integration/api_test.go
package integration_test

import (
    "testing"
    
    "github.com/pay-theory/lift/pkg/testing"
)

func TestUserAPI(t *testing.T) {
    // Create test app
    app := testing.NewTestApp()
    
    // Register routes
    registerRoutes(app.App())
    
    // Test create user
    resp := app.POST("/api/v1/users", map[string]string{
        "name":  "Test User",
        "email": "test@example.com",
    })
    
    resp.AssertStatus(201)
    resp.AssertJSONPath("$.name", "Test User")
    
    // Test get user
    userID := resp.JSONPath("$.id")
    
    getResp := app.GET("/api/v1/users/" + userID)
    getResp.AssertStatus(200)
    getResp.AssertJSONPath("$.id", userID)
}
```

## Deployment Strategies

### 1. AWS CDK Deployment

#### Create CDK Stack

```go
// cdk/main.go
package main

import (
    "github.com/aws/aws-cdk-go/awscdk/v2"
    "github.com/aws/jsii-runtime-go"
    "github.com/pay-theory/lift/pkg/cdk/patterns"
)

func main() {
    app := awscdk.NewApp(nil)
    
    // Environment-specific stacks
    environments := []string{"dev", "staging", "prod"}
    
    for _, env := range environments {
        stack := awscdk.NewStack(app, aws.String("MyLiftApp-"+env), 
            &awscdk.StackProps{
                Env: &awscdk.Environment{
                    Region: aws.String("us-east-1"),
                },
            })
        
        // Use Lift CDK patterns
        liftApp := patterns.NewLiftApp(stack, aws.String("my-app-"+env),
            &patterns.LiftAppProps{
                AppName:            aws.String("my-app"),
                CodeAssetPath:      aws.String("../dist"),
                EnableDatabase:     aws.Bool(true),
                EnableRateLimiting: aws.Bool(true),
            })
    }
    
    app.Synth(nil)
}
```

#### Deploy with CDK

```bash
# First deployment
cd cdk && cdk bootstrap

# Deploy all environments
cd cdk && cdk deploy --all

# Deploy specific environment
cd cdk && cdk deploy MyLiftApp-dev

# Deploy with specific configuration
cd cdk && cdk deploy MyLiftApp-prod --context environment=prod
```

#### Legacy Parameters Section (being removed):
  Environment:
    Type: String
    Default: dev
    AllowedValues:
      - dev
      - staging
      - prod
  
  LogLevel:
    Type: String
    Default: INFO
    AllowedValues:
      - DEBUG
      - INFO
      - WARN
      - ERROR

Resources:
  ApiFunction:
    Type: AWS::Serverless::Function
    Properties:
      CodeUri: dist/
      Handler: bootstrap
      Description: !Sub '${Environment} API Function'
      Events:
        HttpApiEvent:
          Type: HttpApi
          Properties:
            Path: /{proxy+}
            Method: ANY
      Policies:
        - DynamoDBCrudPolicy:
            TableName: !Ref DynamoTable
        - CloudWatchPutMetricPolicy: {}
  
  DynamoTable:
    Type: AWS::DynamoDB::Table
    Properties:
      BillingMode: PAY_PER_REQUEST
      AttributeDefinitions:
        - AttributeName: pk
          AttributeType: S
        - AttributeName: sk
          AttributeType: S
      KeySchema:
        - AttributeName: pk
          KeyType: HASH
        - AttributeName: sk
          KeyType: RANGE
      PointInTimeRecoverySpecification:
        PointInTimeRecoveryEnabled: true
      SSESpecification:
        SSEEnabled: true

Outputs:
  ApiUrl:
    Description: API Gateway endpoint URL
    Value: !Sub 'https://${ServerlessHttpApi}.execute-api.${AWS::Region}.amazonaws.com/'
  
  FunctionArn:
    Description: Lambda Function ARN
    Value: !GetAtt ApiFunction.Arn
  
  TableName:
    Description: DynamoDB Table Name
    Value: !Ref DynamoTable
```

#### Deploy with SAM

```bash
# First deployment (guided)
sam build
sam deploy --guided

# Subsequent deployments
sam build && sam deploy

# Deploy to specific environment
sam build && sam deploy --parameter-overrides Environment=prod LogLevel=INFO

# Deploy with custom stack name
sam deploy --stack-name my-app-prod --parameter-overrides Environment=prod
```

### 2. AWS CDK Deployment

#### Create CDK App

```go
// cdk/main.go
package main

import (
    "os"
    
    "github.com/aws/aws-cdk-go/awscdk/v2"
    "github.com/aws/jsii-runtime-go"
    "github.com/pay-theory/lift/pkg/cdk/patterns"
)

func main() {
    app := awscdk.NewApp(nil)
    
    env := &awscdk.Environment{
        Account: jsii.String(os.Getenv("CDK_DEFAULT_ACCOUNT")),
        Region:  jsii.String(os.Getenv("CDK_DEFAULT_REGION")),
    }
    
    environment := os.Getenv("ENVIRONMENT")
    if environment == "" {
        environment = "dev"
    }
    
    stack := awscdk.NewStack(app, jsii.String("MyApp-"+environment), &awscdk.StackProps{
        Env:         env,
        Description: jsii.String("My Lift Application - " + environment),
    })
    
    // Use Lift pattern
    patterns.NewLiftApp(stack, jsii.String("MyApp"), &patterns.LiftAppProps{
        AppName:            jsii.String("my-app"),
        CodeAssetPath:      jsii.String("../dist"),
        EnableDatabase:     jsii.Bool(true),
        EnableRateLimiting: jsii.Bool(environment == "prod"),
        EnableTracing:      jsii.Bool(environment != "dev"),
        MemorySize:         jsii.Number(getMemorySize(environment)),
        Environment: &map[string]*string{
            "LOG_LEVEL":   jsii.String(getLogLevel(environment)),
            "ENVIRONMENT": jsii.String(environment),
        },
    })
    
    app.Synth(nil)
}

func getMemorySize(env string) float64 {
    if env == "prod" {
        return 1024
    }
    return 512
}

func getLogLevel(env string) string {
    if env == "prod" {
        return "INFO"
    }
    return "DEBUG"
}
```

#### Deploy with CDK

```bash
# First deployment
cd cdk
cdk bootstrap  # Only once per account/region
cdk deploy

# Deploy specific environment
ENVIRONMENT=prod cdk deploy

# View changes before deploy
cdk diff

# Destroy stack
cdk destroy
```

### 3. CI/CD Pipeline Deployment

#### GitHub Actions

```yaml
# .github/workflows/deploy.yml
name: Deploy to AWS

on:
  push:
    branches:
      - main
      - develop
  pull_request:
    branches:
      - main

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.23.10'
      
      - name: Install dependencies
        run: go mod download
      
      - name: Run tests
        run: make test
      
      - name: Run linter
        run: |
          go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
          make lint
      
      - name: Generate coverage
        run: make test-coverage
      
      - name: Upload coverage
        uses: codecov/codecov-action@v3
        with:
          files: ./coverage.out
  
  build:
    needs: test
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.23.10'
      
      - name: Build
        run: make build
      
      - name: Upload artifact
        uses: actions/upload-artifact@v3
        with:
          name: lambda-function
          path: dist/bootstrap
  
  deploy-dev:
    needs: build
    if: github.ref == 'refs/heads/develop'
    runs-on: ubuntu-latest
    environment: development
    steps:
      - uses: actions/checkout@v3
      
      - name: Download artifact
        uses: actions/download-artifact@v3
        with:
          name: lambda-function
          path: dist/
      
      - name: Configure AWS credentials
        uses: aws-actions/configure-aws-credentials@v2
        with:
          aws-access-key-id: ${{ secrets.AWS_ACCESS_KEY_ID }}
          aws-secret-access-key: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
          aws-region: us-east-1
      
      - name: Deploy with SAM
        run: |
          pip install aws-sam-cli
          sam deploy \
            --stack-name my-app-dev \
            --parameter-overrides Environment=dev \
            --no-confirm-changeset \
            --no-fail-on-empty-changeset
  
  deploy-prod:
    needs: build
    if: github.ref == 'refs/heads/main'
    runs-on: ubuntu-latest
    environment: production
    steps:
      - uses: actions/checkout@v3
      
      - name: Download artifact
        uses: actions/download-artifact@v3
        with:
          name: lambda-function
          path: dist/
      
      - name: Configure AWS credentials
        uses: aws-actions/configure-aws-credentials@v2
        with:
          aws-access-key-id: ${{ secrets.AWS_ACCESS_KEY_ID_PROD }}
          aws-secret-access-key: ${{ secrets.AWS_SECRET_ACCESS_KEY_PROD }}
          aws-region: us-east-1
      
      - name: Deploy with SAM
        run: |
          pip install aws-sam-cli
          sam deploy \
            --stack-name my-app-prod \
            --parameter-overrides Environment=prod LogLevel=INFO \
            --no-confirm-changeset \
            --no-fail-on-empty-changeset
```

#### GitLab CI/CD

```yaml
# .gitlab-ci.yml
stages:
  - test
  - build
  - deploy

variables:
  GO_VERSION: "1.23.10"

test:
  stage: test
  image: golang:${GO_VERSION}
  script:
    - make deps
    - make test
    - make lint
  coverage: '/coverage: \d+\.\d+%/'

build:
  stage: build
  image: golang:${GO_VERSION}
  script:
    - make build
  artifacts:
    paths:
      - dist/bootstrap
    expire_in: 1 week

deploy:dev:
  stage: deploy
  image: public.ecr.aws/sam/build-go1.x:latest
  only:
    - develop
  script:
    - sam deploy --stack-name my-app-dev --parameter-overrides Environment=dev
  environment:
    name: development
    url: https://dev-api.example.com

deploy:prod:
  stage: deploy
  image: public.ecr.aws/sam/build-go1.x:latest
  only:
    - main
  script:
    - sam deploy --stack-name my-app-prod --parameter-overrides Environment=prod
  environment:
    name: production
    url: https://api.example.com
  when: manual  # Require manual approval for production
```

## Deployment Best Practices

### 1. Use Semantic Versioning

```bash
# Tag releases
git tag -a v1.0.0 -m "Release version 1.0.0"
git push origin v1.0.0

# Include version in build
make build VERSION=v1.0.0

# Or in code
-ldflags="-s -w -X main.version=$(VERSION)"
```

### 2. Blue/Green Deployments

```yaml
# template.yaml with deployment preference
Resources:
  ApiFunction:
    Type: AWS::Serverless::Function
    Properties:
      # ... other properties
      AutoPublishAlias: live
      DeploymentPreference:
        Type: AllAtOnce           # Or Canary10Percent5Minutes
        Alarms:
          - !Ref FunctionErrorAlarm
        Hooks:
          PreTraffic: !Ref PreTrafficHook
          PostTraffic: !Ref PostTrafficHook
```

### 3. Canary Deployments

```yaml
DeploymentPreference:
  Type: Canary10Percent5Minutes  # 10% traffic for 5 minutes
  Alarms:
    - !Ref FunctionErrorAlarm
    - !Ref HighLatencyAlarm
```

### 4. Rollback Strategy

```bash
# SAM rollback
sam deploy --no-execute-changeset  # Preview changes
# If issues, rollback via CloudFormation console

# CDK rollback
cdk deploy --rollback  # Automatic rollback on failure

# Manual rollback
aws lambda update-alias \
  --function-name my-function \
  --name live \
  --function-version <previous-version>
```

## Production Readiness Checklist

### Before Deployment

- [ ] **Tests pass** - All unit and integration tests
- [ ] **Code coverage** - Minimum 80% coverage
- [ ] **Linter passes** - No linter errors
- [ ] **Security scan** - No known vulnerabilities
- [ ] **Performance tested** - Load testing completed
- [ ] **Configuration validated** - All environment variables set
- [ ] **Infrastructure tested** - CDK/SAM templates validated
- [ ] **Documentation updated** - README and API docs current

### Configuration

- [ ] **Appropriate timeouts** - Less than Lambda timeout
- [ ] **Memory sized correctly** - Based on profiling
- [ ] **Log level set** - INFO or WARN for production
- [ ] **Metrics enabled** - CloudWatch metrics active
- [ ] **Tracing configured** - X-Ray with sampling
- [ ] **Error handling** - All errors handled gracefully
- [ ] **Rate limiting** - Protection against abuse
- [ ] **CORS configured** - Proper origin restrictions

### Security

- [ ] **Secrets in Secrets Manager** - No hardcoded secrets
- [ ] **IAM least privilege** - Minimal required permissions
- [ ] **Encryption enabled** - At rest and in transit
- [ ] **Input validation** - All inputs validated
- [ ] **Output sanitization** - No sensitive data leaked
- [ ] **Security headers** - SecurityHeaders middleware added
- [ ] **Authentication** - JWT or API key validation
- [ ] **Authorization** - Role-based access control

### Monitoring

- [ ] **CloudWatch dashboards** - Key metrics visible
- [ ] **Alarms configured** - Error rate, latency, throttles
- [ ] **Log aggregation** - Logs centralized and searchable
- [ ] **Distributed tracing** - X-Ray configured
- [ ] **APM integration** - Application performance monitoring
- [ ] **Cost monitoring** - Budget alerts configured

### Operations

- [ ] **Deployment process documented** - Clear deployment steps
- [ ] **Rollback plan** - Tested rollback procedure
- [ ] **Runbook created** - Operational procedures documented
- [ ] **On-call setup** - Team notified of alerts
- [ ] **Backup strategy** - Database backups configured
- [ ] **Disaster recovery** - DR plan documented and tested

## Summary

### Build Best Practices
1. Use ARM64 for better price/performance
2. Optimize binary size with build flags
3. Automate builds with Makefile
4. Use build cache for faster iteration
5. Verify builds before deployment

### Testing Best Practices
1. Test locally with SAM CLI
2. Write comprehensive unit tests
3. Include integration tests
4. Test with realistic data
5. Achieve 80%+ code coverage

### Deployment Best Practices
1. Use infrastructure as code (SAM/CDK)
2. Implement CI/CD pipelines
3. Use environment-specific configurations
4. Implement canary or blue/green deployments
5. Have rollback strategy ready

### Production Readiness
1. Complete pre-deployment checklist
2. Validate all configurations
3. Ensure security measures in place
4. Set up comprehensive monitoring
5. Document operational procedures

Following these best practices ensures reliable, secure, and efficient deployment of Lift applications to production.

