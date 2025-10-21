# Lift CDK Constructs

AWS CDK constructs for deploying Lift applications with infrastructure as code.

## Overview

The Lift CDK package provides high-level constructs optimized for deploying serverless Go applications built with the Lift framework. It includes:

- **LiftFunction**: Lambda function construct with Lift-optimized defaults
- **LiftAPI**: API Gateway HTTP API construct with built-in CORS and custom domain support
- **LiftTable**: DynamoDB table construct with multi-tenant support
- **LiftApp**: Complete application pattern combining all components

## Installation

```bash
go get github.com/pay-theory/lift/pkg/cdk
```

## Quick Start

```go
package main

import (
    "github.com/aws/aws-cdk-go/awscdk/v2"
    "github.com/aws/constructs-go/constructs/v10"
    "github.com/aws/jsii-runtime-go"
    "github.com/pay-theory/lift/pkg/cdk/patterns"
)

func NewMyLiftStack(scope constructs.Construct, id string) awscdk.Stack {
    stack := awscdk.NewStack(scope, &id, nil)

    // Deploy a complete Lift application
    patterns.NewLiftApp(stack, jsii.String("MyApp"), &patterns.LiftAppProps{
        AppName:           jsii.String("my-lift-app"),
        CodeAssetPath:     jsii.String("./dist"),
        EnableMultiTenant: jsii.Bool(true),
        EnableDatabase:    jsii.Bool(true),
        EnableRateLimiting: jsii.Bool(true),
        Environment: &map[string]*string{
            "LOG_LEVEL": jsii.String("info"),
        },
    })

    return stack
}
```

## Constructs

### LiftFunction

Optimized Lambda function for Lift applications:

```go
fn := constructs.NewLiftFunction(stack, jsii.String("MyFunction"), &constructs.LiftFunctionProps{
    FunctionProps: awslambda.FunctionProps{
        Code:    awslambda.Code_FromAsset(jsii.String("./lambda"), nil),
        Handler: jsii.String("bootstrap"),
    },
    EnableTracing:     jsii.Bool(true),
    EnableMultiTenant: jsii.Bool(true),
})
```

Default optimizations:
- ARM64 architecture for better price/performance
- 512MB memory
- 30 second timeout
- PROVIDED_AL2023 runtime for Go
- X-Ray tracing support

### LiftAPI

API Gateway HTTP API with Lift integrations:

```go
api := constructs.NewLiftAPI(stack, jsii.String("MyAPI"), &constructs.LiftAPIProps{
    Name:        jsii.String("my-api"),
    EnableCORS:  jsii.Bool(true),
    DomainName:  jsii.String("api.example.com"),
    CertificateArn: jsii.String("arn:aws:acm:..."),
})

// Add routes
api.AddLambdaRoute(jsii.String("/users"), awsapigatewayv2.HttpMethod_GET, fn)
```

Features:
- CORS configuration for Lift headers
- Custom domain support
- Access logging
- Rate limiting configuration

### LiftTable

DynamoDB table optimized for Lift's single-table design:

```go
table := constructs.NewLiftTable(stack, jsii.String("MyTable"), &constructs.LiftTableProps{
    TableName:         jsii.String("my-table"),
    EnableMultiTenant: jsii.Bool(true),
    EnableStreams:     jsii.Bool(true),
    EnableAutoScaling: jsii.Bool(true),
})
```

Features:
- Single-table design with pk/sk keys
- Multi-tenant partitioning support
- Global secondary index (gsi1)
- Auto-scaling configuration
- Point-in-time recovery
- TTL support

### LiftApp Pattern

Complete application deployment:

```go
app := patterns.NewLiftApp(stack, jsii.String("MyApp"), &patterns.LiftAppProps{
    AppName:            jsii.String("my-app"),
    CodeAssetPath:      jsii.String("./dist"),
    EnableMultiTenant:  jsii.Bool(true),
    EnableDatabase:     jsii.Bool(true),
    EnableRateLimiting: jsii.Bool(true),
    MemorySize:         jsii.Number(1024),
    Timeout:            awscdk.Duration_Minutes(jsii.Number(5)),
})
```

Includes:
- Lambda function with Lift runtime
- API Gateway with catch-all routing
- DynamoDB table for application data
- Rate limiting table
- CloudWatch logs
- All necessary IAM permissions

## Building and Deploying

### Build your Lift application

```bash
GOOS=linux GOARCH=arm64 go build -o dist/bootstrap main.go
```

### Deploy with CDK

```bash
# Install CDK CLI
npm install -g aws-cdk

# Bootstrap CDK (first time only)
cdk bootstrap

# Deploy
cdk deploy
```

### Using Make

```bash
# Synthesize CloudFormation
make cdk-synth

# Deploy
make cdk-deploy

# Show differences
make cdk-diff

# Destroy stack
make cdk-destroy
```

## Examples

See `/pkg/cdk/examples/` for complete examples:

- `basic-api/` - Simple API with Lambda backend
- More examples coming soon

## Best Practices

1. **Use the LiftApp pattern** for standard applications
2. **Enable multi-tenant support** if building SaaS applications
3. **Configure auto-scaling** for production workloads
4. **Enable tracing** for observability
5. **Set appropriate memory** based on your workload (512MB-3008MB)
6. **Use custom domains** for production APIs

## Integration with Lift

The CDK constructs automatically configure:
- Environment variables expected by Lift
- IAM permissions for AWS services
- Optimized Lambda settings
- API Gateway routing compatible with Lift's router

## Contributing

When adding new constructs:
1. Follow the existing patterns
2. Include comprehensive documentation
3. Add examples showing usage
4. Ensure compatibility with Lift framework assumptions