# Lift Installation and Setup Guide

## Overview

This guide covers installing and setting up the Lift framework for building AWS Lambda functions in Go. Lift is a production-ready framework that provides automatic error handling, logging, observability, and multi-tenant support.

## Installation Instructions for Go Applications

### Prerequisites

Before installing Lift, ensure you have:
- **Go 1.23.10 or later** (Lift is tested with Go 1.23.10 as of 2025)
- AWS account with Lambda access
- Basic understanding of serverless concepts
- Git installed for dependency management

### Installing the Latest Release

#### For New Projects (Recommended Method)

When starting a new Lambda project, initialize a Go module first, then add Lift:

```bash
# Create your project directory
mkdir my-lambda-function
cd my-lambda-function

# Initialize Go module
go mod init github.com/yourusername/my-lambda-function

# Install Lift core package
go get github.com/pay-theory/lift/pkg/lift

# Install Lift middleware (recommended for production)
go get github.com/pay-theory/lift/pkg/middleware

# Install AWS Lambda Go SDK
go get github.com/aws/aws-lambda-go/lambda
```

This method ensures you get the latest stable release of Lift and its dependencies.

#### For Existing Projects

If you're adding Lift to an existing Go project:

```bash
# Navigate to your project directory
cd /path/to/your/project

# Add Lift to your project
go get github.com/pay-theory/lift/pkg/lift
go get github.com/pay-theory/lift/pkg/middleware

# Update all dependencies
go mod tidy
```

### Required Go Dependencies

When you install Lift, Go automatically installs these core dependencies:

**Essential Runtime Dependencies:**
- `github.com/pay-theory/lift/pkg/lift` - Core framework
- `github.com/pay-theory/lift/pkg/middleware` - Production middleware
- `github.com/aws/aws-lambda-go` v1.49.0 - AWS Lambda runtime
- `github.com/aws/aws-sdk-go-v2` v1.36.5 - AWS SDK for Go v2

**Key AWS Service Clients:**
- `github.com/aws/aws-sdk-go-v2/service/dynamodb` - DynamoDB operations
- `github.com/aws/aws-sdk-go-v2/service/s3` - S3 operations
- `github.com/aws/aws-sdk-go-v2/service/sns` - SNS notifications
- `github.com/aws/aws-sdk-go-v2/service/cloudwatch` - Metrics and logging

**Additional Production Dependencies:**
- `github.com/golang-jwt/jwt/v5` v5.2.2 - JWT authentication
- `go.uber.org/zap` v1.27.0 - Structured logging
- `github.com/pay-theory/dynamorm` v1.0.23 - DynamoDB ORM
- `github.com/pay-theory/limited` v1.0.0 - Rate limiting
- `github.com/google/uuid` v1.6.0 - UUID generation

All dependencies are managed automatically through `go mod`. After running `go get`, your `go.mod` file will include all necessary dependencies with their versions.

### Verifying Installation

Create a simple test file to verify Lift is installed correctly:

```go
// test.go
package main

import (
    "fmt"
    "github.com/pay-theory/lift/pkg/lift"
)

func main() {
    app := lift.New()
    fmt.Println("Lift installed successfully!")
    fmt.Printf("App created: %T\n", app)
}
```

Run the test:

```bash
go run test.go
```

If you see "Lift installed successfully!" your installation is complete.

### Installation Best Practices for 2025

Following Go best practices from 2025, when working with Lift:

#### Use Go Workspaces for Multi-Module Projects

If you're building multiple Lambda functions in one repository:

```bash
# Initialize a workspace
go work init

# Add multiple modules
go work use ./function1
go work use ./function2
go work use ./shared
```

#### Leverage Go 1.23 Features

Lift is compatible with Go 1.23's latest features:

```go
// Take advantage of Go 1.23 range over functions
for i, handler := range getHandlers() {
    app.POST(fmt.Sprintf("/endpoint%d", i), handler)
}

// Use enhanced type inference
type HandlerConfig struct {
    path    string
    method  string
    handler lift.Handler
}

configs := []HandlerConfig{
    {"/users", "GET", getUsersHandler},
    {"/users", "POST", createUserHandler},
}
```

#### Use Specific Module Versions

For production applications, pin to specific versions in `go.mod`:

```go
module github.com/yourcompany/your-app

go 1.23.10

require (
    github.com/pay-theory/lift
    github.com/aws/aws-lambda-go v1.49.0
)
```

Update versions regularly but test thoroughly:

```bash
# View available updates
go list -m -u all

# Update specific module
go get github.com/pay-theory/lift@latest

# Update all dependencies
go get -u ./...
```

### Common Installation Issues and Solutions

#### Issue: "module not found"

**Cause:** Go can't find the Lift module
**Solution:**

```bash
# Ensure Go modules are enabled (default in Go 1.16+)
go env -w GO111MODULE=on

# Clear module cache and retry
go clean -modcache
go get github.com/pay-theory/lift/pkg/lift
```

#### Issue: Version conflicts

**Cause:** Dependency version incompatibilities
**Solution:**

```bash
# View dependency graph
go mod graph

# Update indirect dependencies
go get -u ./...
go mod tidy
```

#### Issue: Build errors after installation

**Cause:** Missing or incompatible dependencies
**Solution:**

```bash
# Download all dependencies
go mod download

# Verify dependencies
go mod verify

# Clean and rebuild
go clean -cache
go build
```

### Development Environment Setup

#### Recommended IDE Setup

For the best development experience with Lift in 2025:

**VS Code with Go Extension:**
```json
{
    "go.useLanguageServer": true,
    "go.toolsManagement.autoUpdate": true,
    "go.lintTool": "golangci-lint",
    "go.lintOnSave": "package",
    "gopls": {
        "ui.semanticTokens": true,
        "analyses": {
            "unusedparams": true,
            "shadow": true
        }
    }
}
```

**GoLand/IntelliJ IDEA:**
- Enable Go Modules integration
- Configure GOROOT to Go 1.23.10
- Enable "Go Modules" in project settings

#### Install Development Tools

```bash
# Install golangci-lint for code quality
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Install AWS CDK for infrastructure
npm install -g aws-cdk
```

### Next Steps After Installation

1. **Create Your First Application:** See the "Creating a Basic Lift Application" guide
2. **Explore Middleware:** Review available middleware in `pkg/middleware`
3. **Set Up Local Testing:** Configure CDK for infrastructure deployment
4. **Review Examples:** Check `examples/hello-world` for a complete working example

## Quick Start Template

After installation, use this template to start building:

```go
package main

import (
    "github.com/aws/aws-lambda-go/lambda"
    "github.com/pay-theory/lift/pkg/lift"
    "github.com/pay-theory/lift/pkg/middleware"
)

func main() {
    // Create application
    app := lift.New()
    
    // Add essential middleware
    app.Use(middleware.RequestID())
    app.Use(middleware.Logger())
    app.Use(middleware.Recover())
    
    // Define routes
    app.GET("/health", func(ctx *lift.Context) error {
        return ctx.JSON(map[string]string{
            "status": "healthy",
        })
    })
    
    // Start Lambda handler
    lambda.Start(app.HandleRequest)
}
```

Build for Lambda:

```bash
# For ARM64 (recommended - better price/performance)
GOOS=linux GOARCH=arm64 go build -tags lambda.norpc -o bootstrap main.go

# For x86_64
GOOS=linux GOARCH=amd64 go build -tags lambda.norpc -o bootstrap main.go
```

## Summary

- **Use Go 1.23.10 or later** for the best compatibility and features
- **Install via `go get`** for automatic dependency management
- **Pin versions in production** to ensure consistent deployments
- **Follow Go 2025 best practices** including workspaces and enhanced type inference
- **Use ARM64 architecture** for Lambda deployments when possible
- **Set up proper tooling** (linters, IDE extensions) for development efficiency

The Lift framework follows modern Go conventions and integrates seamlessly with the Go ecosystem as of 2025.

