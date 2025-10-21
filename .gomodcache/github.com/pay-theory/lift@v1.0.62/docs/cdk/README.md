# Lift CDK Documentation

AWS CDK integration for the Lift serverless framework, providing infrastructure as code for Go-based Lambda applications.

## Overview

The Lift CDK package (`pkg/cdk`) provides high-level constructs and patterns for deploying Lift applications to AWS. It includes optimized defaults for Lambda, API Gateway, DynamoDB, and other AWS services commonly used with serverless applications.

## Quick Links

- [Best Practices Guide](./best-practices.md) - Comprehensive guide for production deployments
- [API Documentation](/pkg/cdk/README.md) - Detailed construct and pattern reference
- [Examples](/pkg/cdk/examples/) - Working examples of CDK deployments

## Getting Started

### 1. Initialize CDK in Your Project

```bash
# Create a new Lift project
lift new my-app
cd my-app

# Initialize CDK deployment
lift cdk-init

# For specific stack types:
lift cdk-init microservice
lift cdk-init saas
lift cdk-init event-driven
```

### 2. Deploy Your Application

```bash
# Build and deploy
lift cdk-deploy

# Or use make commands
make deploy
```

### 3. View Changes Before Deploying

```bash
lift cdk-diff
```

## Available Constructs

### Core Constructs

- **LiftFunction** - Optimized Lambda function with ARM64, tracing, and multi-tenant support
- **LiftAPI** - API Gateway HTTP API with CORS, custom domains, and rate limiting
- **LiftTable** - DynamoDB table with single-table design, GSI, and auto-scaling
- **SecureFunction** - Enhanced Lambda function with security best practices
- **RateLimitedFunction** - Lambda function with built-in rate limiting
- **ComplianceStack** - Complete compliance framework implementation
- **AuditingConstruct** - Comprehensive audit logging and monitoring

### High-Level Patterns

- **LiftApp** - Complete application with Lambda, API Gateway, and DynamoDB
- **MicroserviceStack** - Single microservice deployment
- **MultiTenantSaaSStack** - SaaS application with auth and file storage
- **EventDrivenStack** - Event-driven architecture with EventBridge and SQS

## CLI Commands

The Lift CLI includes integrated CDK commands:

| Command | Description |
|---------|-------------|
| `lift build` | Build Lambda function for deployment |
| `lift cdk-init [type]` | Initialize CDK for your project |
| `lift cdk-deploy [stack]` | Deploy stack to AWS |
| `lift cdk-synth [stack]` | Synthesize CloudFormation template |
| `lift cdk-diff [stack]` | Show deployment changes |
| `lift cdk-destroy [stack]` | Destroy deployed stack |

## Project Structure

```
my-lift-app/
├── main.go              # Lambda handler (or cmd/main.go)
├── pkg/
│   └── handlers/        # Business logic
├── cdk/
│   ├── main.go         # CDK app
│   ├── cdk.json        # CDK config
│   └── stacks/         # Custom stacks
├── dist/               # Build output
└── Makefile           # Build commands
```

## Testing

The CDK package includes comprehensive testing utilities:

```go
import "github.com/pay-theory/lift/pkg/cdk/test"

func TestMyStack(t *testing.T) {
    // Create test stack
    testStack := test.NewTestStack()
    
    // Create your stack
    NewMyStack(testStack.Stack(), "TestStack", props)
    
    // Use event helpers for testing
    eventHelpers := test.NewEventHelpers()
    sqsEvent := eventHelpers.GenerateSQSEvent([]test.SQSMessage{
        {ID: "test-1", Body: "test message", Timestamp: "1234567890"},
    })
    
    // Validate events
    validator := test.NewEventValidator()
    err := validator.ValidateSQSMessage(sqsEvent.Records[0])
    assert.NoError(t, err)
}
```

## Integration with CI/CD

### GitHub Actions Example

```yaml
name: Deploy
on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      
      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: '20'
      
      - name: Install CDK
        run: npm install -g aws-cdk
      
      - name: Configure AWS
        uses: aws-actions/configure-aws-credentials@v4
        with:
          aws-access-key-id: ${{ secrets.AWS_ACCESS_KEY_ID }}
          aws-secret-access-key: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
          aws-region: us-east-1
      
      - name: Deploy
        run: |
          make build
          make deploy
```

## Cost Optimization

The Lift CDK constructs include cost-optimized defaults:

- ARM64 Lambda architecture (better price/performance)
- On-demand DynamoDB billing
- Appropriate log retention periods
- Auto-scaling with reasonable limits

## Security Features

Built-in security best practices:

- Encryption at rest for all data stores
- Least-privilege IAM policies
- X-Ray tracing enabled
- CloudWatch logs with encryption
- Multi-tenant data isolation

## Monitoring

All Lift CDK deployments include:

- CloudWatch Logs for Lambda functions
- X-Ray tracing for distributed tracing
- CloudWatch metrics for all services
- Optional alarms for production deployments

## Migration Guide

### From Manual Deployment

1. Run `lift cdk-init` in your existing project
2. Update the CDK app with your configuration
3. Run `lift cdk-diff` to see what will be created
4. Deploy with `lift cdk-deploy`

### From CloudFormation/SAM

1. Identify your existing resources
2. Map them to Lift CDK constructs
3. Gradually migrate to CDK management
4. Use CDK's import functionality for existing resources

## Troubleshooting

### Common Issues

**CDK command not found**
```bash
npm install -g aws-cdk
```

**Bootstrap required**
```bash
cdk bootstrap aws://ACCOUNT-NUMBER/REGION
```

**Build failures**
```bash
# Ensure you're building for Linux
GOOS=linux GOARCH=arm64 go build -o dist/bootstrap cmd/main.go
```

## Next Steps

1. Read the [Best Practices Guide](./best-practices.md)
2. Explore [example deployments](/pkg/cdk/examples/)
3. Check out [pre-built stacks](/pkg/cdk/stacks/)
4. Join the Lift community for support

## Contributing

When contributing CDK constructs:

1. Follow existing patterns
2. Include comprehensive tests
3. Update documentation
4. Add examples
5. Run linting: `golangci-lint run ./pkg/cdk/...`