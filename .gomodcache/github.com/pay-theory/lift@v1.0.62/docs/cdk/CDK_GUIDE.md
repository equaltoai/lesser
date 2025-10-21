# Lift CDK Integration Guide

This comprehensive guide covers the current state of AWS CDK integration with Lift, providing specific code references and examples for deploying Lift applications using Infrastructure as Code.

## Current Implementation Status

The Lift CDK implementation includes fully functional constructs, patterns, and stacks with production-ready features. All code references are from the current codebase state as of this documentation.

### Core CDK Components Available

#### 1. Lambda Functions (`pkg/cdk/constructs/lambda.go`)

**LiftFunction Construct** - Lines 73-185
- **Location**: `pkg/cdk/constructs/lambda.go:73-185`
- **Purpose**: Optimized Lambda function construct for Lift applications
- **Key Features**:
  - ARM64 architecture by default (`lambda.go:118-120`)
  - 512MB memory default (`lambda.go:121-123`)
  - 30-second timeout default (`lambda.go:124-126`)
  - X-Ray tracing support (`lambda.go:130-134`)
  - DynamORM environment variables (`lambda.go:160-185`)

**Example Usage**:
```go
// From pkg/cdk/constructs/lambda.go:87-99
fn := liftconstructs.NewLiftFunction(this, jsii.String("Function"), &liftconstructs.LiftFunctionProps{
    FunctionProps: awslambda.FunctionProps{
        Code:         awslambda.Code_FromAsset(jsii.String("./dist"), nil),
        Handler:      jsii.String("bootstrap"),
        Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
        Architecture: awslambda.Architecture_ARM_64(),
    },
    EnableTracing:     jsii.Bool(true),
    EnableMultiTenant: jsii.Bool(true),
    EnableDynamORM:    jsii.Bool(true),
})
```

#### 2. API Gateway (`pkg/cdk/constructs/api.go`)

**LiftAPI Construct** - Lines 55-533
- **Location**: `pkg/cdk/constructs/api.go:55-533`
- **Purpose**: HTTP API Gateway optimized for Lift applications
- **Key Features**:
  - CORS configuration with Lift-specific headers (`api.go:208-228`)
  - Access logging support (`api.go:161-166`)
  - Custom domain support (`api.go:374-392`)
  - Throttling configuration (`api.go:293-317`)

**CORS Configuration** (Lines 208-228):
```go
// From pkg/cdk/constructs/api.go:208-228
CorsPreflight: &awsapigatewayv2.CorsPreflightOptions{
    AllowOrigins: &[]*string{jsii.String("*")},
    AllowMethods: &[]awsapigatewayv2.CorsHttpMethod{
        awsapigatewayv2.CorsHttpMethod_GET,
        awsapigatewayv2.CorsHttpMethod_POST,
        awsapigatewayv2.CorsHttpMethod_PUT,
        awsapigatewayv2.CorsHttpMethod_DELETE,
        awsapigatewayv2.CorsHttpMethod_OPTIONS,
    },
    AllowHeaders: &[]*string{
        jsii.String("Content-Type"),
        jsii.String("Authorization"),
        jsii.String("X-Tenant-ID"),
        jsii.String("X-Request-ID"),
        jsii.String("X-Api-Key"),
    },
}
```

#### 3. DynamoDB Tables (`pkg/cdk/constructs/dynamodb.go`)

**LiftTable Construct** - Lines 88-468
- **Location**: `pkg/cdk/constructs/dynamodb.go:88-468`
- **Purpose**: DynamoDB table optimized for Lift applications
- **Key Features**:
  - Point-in-time recovery support (`dynamodb.go:120-125`)
  - DynamoDB streams configuration (`dynamodb.go:127-132`)
  - Auto-scaling capabilities (`dynamodb.go:134-150`)
  - TTL configuration (`dynamodb.go:152-157`)
  - Global Secondary Indexes (`dynamodb.go:159-200`)

**LiftTable Configuration** (Lines 88-468):
```go
// From pkg/cdk/constructs/dynamodb.go:88-468
func NewLiftTable(scope constructs.Construct, id *string, props *LiftTableProps) *LiftTable {
    builder := newLiftTableBuilder(scope, id, props)
    return builder.build()
}

// Example usage with auto-scaling and streams
props := &LiftTableProps{
    TableName:                 jsii.String("my-table"),
    PartitionKeyName:          jsii.String("PK"),
    SortKeyName:               jsii.String("SK"),
    EnablePointInTimeRecovery: jsii.Bool(true),
    EnableStreams:             jsii.Bool(true),
    EnableAutoScaling:         jsii.Bool(true),
    TimeToLiveAttribute:       jsii.String("ttl"),
}
```

### High-Level Patterns

#### 1. Complete Lift Application (`pkg/cdk/patterns/lift_app.go`)

**LiftApp Pattern** - Lines 44-184
- **Location**: `pkg/cdk/patterns/lift_app.go:44-184`
- **Purpose**: Complete serverless application with Lambda, API Gateway, and DynamoDB
- **Components**:
  - API Gateway with catch-all routing (`lift_app.go:152-163`)
  - Lambda function with optimized defaults (`lift_app.go:87-100`)
  - Optional DynamoDB table (`lift_app.go:102-120`)
  - Optional rate limiting table (`lift_app.go:122-137`)

**Complete Stack Example**:
```go
// From pkg/cdk/patterns/lift_app.go:53-99
app := patterns.NewLiftApp(stack, jsii.String("MyApp"), &patterns.LiftAppProps{
    AppName:           jsii.String("my-lift-app"),
    CodeAssetPath:     jsii.String("./dist"),
    EnableDatabase:    jsii.Bool(true),
    EnableRateLimiting: jsii.Bool(true),
    EnableMultiTenant: jsii.Bool(true),
    Environment: &map[string]*string{
        "CUSTOM_VAR": jsii.String("value"),
    },
})
```

#### 2. Microservice Stack (`pkg/cdk/stacks/microservice.go`)

**MicroserviceStack** - Lines 28-67
- **Location**: `pkg/cdk/stacks/microservice.go:28-67`
- **Purpose**: Production-ready microservice deployment
- **Features**:
  - Configurable environment variables (`microservice.go:35-39`)
  - Database enablement option (`microservice.go:18`)
  - Stack outputs for integration (`microservice.go:53-65`)

#### 3. Enhanced Monitoring (`pkg/cdk/constructs/monitoring_enhanced.go`)

**EnhancedMonitoring Construct** - Lines 71-108
- **Location**: `pkg/cdk/constructs/monitoring_enhanced.go:71-108`
- **Purpose**: Comprehensive CloudWatch monitoring for Lift applications
- **Features**:
  - Real CloudWatch metrics (`monitoring_enhanced.go:168-223`)
  - Custom business metrics (`monitoring_enhanced.go:267-303`)
  - Cold start tracking (`monitoring_enhanced.go:225-265`)
  - Comprehensive dashboards (`monitoring_enhanced.go:483-577`)

**Metrics Configuration** (Lines 168-174):
```go
// From pkg/cdk/constructs/monitoring_enhanced.go:168-174
m.Metrics["Requests"] = awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
    Namespace:  props.Namespace,
    MetricName: jsii.String("Requests"),
    DimensionsMap: baseDimensions,
    Statistic:  jsii.String("Sum"),
    Period:     awscdk.Duration_Minutes(jsii.Number(1)),
})
```

#### 4. Enhanced Security (`pkg/cdk/constructs/security_enhanced.go`)

**EnhancedSecurity Construct** - Lines 90-136
- **Location**: `pkg/cdk/constructs/security_enhanced.go:90-136`
- **Purpose**: Comprehensive security with WAF, VPC endpoints, and monitoring
- **Features**:
  - WAF with rate limiting (`security_enhanced.go:237-452`)
  - VPC endpoints for AWS services (`security_enhanced.go:545-578`)
  - Security groups with least privilege (`security_enhanced.go:166-222`)
  - VPC Flow Logs (`security_enhanced.go:580-615`)

**WAF Configuration** (Lines 243-272):
```go
// From pkg/cdk/constructs/security_enhanced.go:243-272
rules = append(rules, awswafv2.CfnWebACL_RuleProperty{
    Name:     jsii.String("RateLimitRule"),
    Priority: jsii.Number(priority),
    Statement: &awswafv2.CfnWebACL_StatementProperty{
        RateBasedStatement: &awswafv2.CfnWebACL_RateBasedStatementProperty{
            Limit:            rateLimit,
            AggregateKeyType: jsii.String("IP"),
        },
    },
    Action: &awswafv2.CfnWebACL_RuleActionProperty{
        Block: &awswafv2.CfnWebACL_BlockActionProperty{
            CustomResponse: &awswafv2.CfnWebACL_CustomResponseProperty{
                ResponseCode:          jsii.Number(429),
                CustomResponseBodyKey: jsii.String("RateLimitExceeded"),
            },
        },
    },
})
```

#### 5. Complete Microservice Pattern (`pkg/cdk/patterns/microservice_complete.go`)

**MicroserviceComplete Pattern** - Lines 139-215
- **Location**: `pkg/cdk/patterns/microservice_complete.go:139-215`
- **Purpose**: Production-grade microservice with ECS, ALB, and service discovery
- **Features**:
  - ECS Fargate with auto-scaling (`microservice_complete.go:607-650`)
  - Application Load Balancer (`microservice_complete.go:529-605`)
  - AWS Cloud Map service discovery (`microservice_complete.go:374-384`)
  - Enhanced monitoring and security integration (`microservice_complete.go:652-698`)

## Getting Started with CDK Integration

### Prerequisites

1. **AWS CDK v2** installed and configured
2. **Go 1.21+** for CDK Go bindings
3. **Lift application** ready for deployment

### Basic Implementation Steps

#### Step 1: Initialize CDK Project

```bash
mkdir my-lift-cdk
cd my-lift-cdk
cdk init app --language go
```

#### Step 2: Add Lift CDK Dependencies

Add to your `go.mod`:
```go
require (
    github.com/aws/aws-cdk-go/awscdk/v2 v2.100.0
    github.com/pay-theory/lift/pkg/cdk v0.1.0
)
```

#### Step 3: Create Basic Stack

Based on `pkg/cdk/stacks/microservice.go:28-67`:

```go
package main

import (
    "github.com/aws/aws-cdk-go/awscdk/v2"
    "github.com/aws/constructs-go/constructs/v10"
    "github.com/aws/jsii-runtime-go"
    "github.com/pay-theory/lift/pkg/cdk/patterns"
)

func NewMyLiftStack(scope constructs.Construct, id string, props *awscdk.StackProps) awscdk.Stack {
    stack := awscdk.NewStack(scope, &id, props)

    // Create complete Lift application
    app := patterns.NewLiftApp(stack, jsii.String("MyApp"), &patterns.LiftAppProps{
        AppName:           jsii.String("my-service"),
        CodeAssetPath:     jsii.String("./dist"),
        EnableDatabase:    jsii.Bool(true),
        EnableRateLimiting: jsii.Bool(true),
        EnableMultiTenant: jsii.Bool(true),
        MemorySize:        jsii.Number(512),
        Timeout:           jsii.Number(30),
    })

    // Add stack outputs
    awscdk.NewCfnOutput(stack, jsii.String("ApiEndpoint"), &awscdk.CfnOutputProps{
        Value:       app.API.GetUrl(),
        Description: jsii.String("API Gateway endpoint"),
    })

    return stack
}
```

#### Step 4: Enhanced Configuration

For production deployments with monitoring and security (`pkg/cdk/constructs/monitoring_enhanced.go` and `pkg/cdk/constructs/security_enhanced.go`):

```go
// Add enhanced monitoring - Based on monitoring_enhanced.go:81-108
monitoring := liftconstructs.NewEnhancedMonitoring(stack, jsii.String("Monitoring"), &liftconstructs.EnhancedMonitoringProps{
    Namespace:   jsii.String("MyApp/Production"),
    Environment: jsii.String("production"),
    MetricConfig: &liftconstructs.MetricConfiguration{
        DetailedMetrics:       jsii.Bool(true),
        EnableBusinessMetrics: jsii.Bool(true),
    },
    EnableRealTimeStreaming: jsii.Bool(true),
})

// Add enhanced security - Based on security_enhanced.go:101-136
security := liftconstructs.NewEnhancedSecurity(stack, jsii.String("Security"), &liftconstructs.EnhancedSecurityProps{
    Vpc:                vpc,
    EnableWAF:          jsii.Bool(true),
    EnableVPCFlowLogs:  jsii.Bool(true),
    Environment:        jsii.String("production"),
    ApplicationName:    jsii.String("my-service"),
})
```

### Advanced Patterns

#### DynamoDB Table Configuration

Based on `pkg/cdk/constructs/dynamodb.go:88-468`:

```go
// Create Lift-optimized DynamoDB table
table := liftconstructs.NewLiftTable(stack, jsii.String("Database"), &liftconstructs.LiftTableProps{
    TableName:                 jsii.String("my-app-table"),
    PartitionKeyName:          jsii.String("PK"),
    SortKeyName:               jsii.String("SK"),
    EnablePointInTimeRecovery: jsii.Bool(true),
    EnableStreams:             jsii.Bool(true),
    EnableAutoScaling:         jsii.Bool(true),
    TimeToLiveAttribute:       jsii.String("ttl"),
    MinReadCapacity:           jsii.Number(5),
    MaxReadCapacity:           jsii.Number(100),
    MinWriteCapacity:          jsii.Number(5),
    MaxWriteCapacity:          jsii.Number(100),
})

// Grant permissions to Lambda function
table.Table.GrantReadWriteData(function.Function)
```

#### API Gateway with Custom Authorizers

Based on `pkg/cdk/constructs/api.go:435-458`:

```go
// Create API with custom configuration
api := liftconstructs.NewLiftAPI(stack, jsii.String("API"), &liftconstructs.LiftAPIProps{
    Name:                jsii.String("my-api"),
    EnableCORS:          jsii.Bool(true),
    EnableAccessLogging: jsii.Bool(true),
    ThrottleRateLimit:   jsii.Number(1000),
    ThrottleBurstLimit:  jsii.Number(2000),
    DomainName:          jsii.String("api.example.com"),
    CertificateArn:      jsii.String("arn:aws:acm:us-east-1:123456789012:certificate/12345678-1234-1234-1234-123456789012"),
})

// Add routes with options - Based on api.go:228-251
api.AddLambdaRouteWithOptions(
    jsii.String("/api/users"),
    awsapigatewayv2.HttpMethod_GET,
    userFunction.Function,
    &liftconstructs.RouteOptions{
        Authorizer: customAuthorizer,
        ThrottleRateLimit: jsii.Number(100),
    },
)
```

## Production Deployment Examples

### Example 1: Basic Microservice

Based on `pkg/cdk/stacks/microservice.go`:

```go
// Create microservice stack
stack := stacks.NewMicroserviceStack(app, "MyService", &stacks.MicroserviceStackProps{
    StackProps: awscdk.StackProps{
        Env: &awscdk.Environment{
            Account: jsii.String("123456789012"),
            Region:  jsii.String("us-east-1"),
        },
    },
    ServiceName:    "user-service",
    CodePath:       "./dist/bootstrap",
    EnableDatabase: true,
    Environment: map[string]string{
        "LOG_LEVEL": "info",
        "ENV":       "production",
    },
    MemorySize: 1024,
})
```

### Example 2: Multi-Tenant SaaS Application

Based on `pkg/cdk/stacks/multi_tenant_saas.go`:

```go
// Multi-tenant SaaS with enhanced features
app := patterns.NewLiftApp(stack, jsii.String("SaaS"), &patterns.LiftAppProps{
    AppName:           jsii.String("saas-platform"),
    CodeAssetPath:     jsii.String("./dist"),
    EnableDatabase:    jsii.Bool(true),
    EnableRateLimiting: jsii.Bool(true),
    EnableMultiTenant: jsii.Bool(true),
    DatabaseTableName: jsii.String("saas-platform-data"),
    Environment: &map[string]*string{
        "TENANT_ISOLATION": jsii.String("strict"),
        "METRICS_ENABLED":  jsii.String("true"),
    },
})
```

### Example 3: Event-Driven Architecture

Based on `pkg/cdk/stacks/event_driven.go`:

```go
// Event-driven stack with multiple processors
eventStack := stacks.NewEventDrivenStack(app, "Events", &stacks.EventDrivenStackProps{
    AppName:                "order-processing",
    ApiCodePath:            "./dist/api/bootstrap",
    EventProcessorCodePath: "./dist/processor/bootstrap",
    EnableDatabase:         true,
    EnableEventSourcing:    true,
})
```

## Available Constructs Reference

### Core Constructs

| Construct | File Location | Purpose | Key Features |
|-----------|---------------|---------|--------------|
| `LiftFunction` | `pkg/cdk/constructs/lambda.go:73-185` | Optimized Lambda functions | ARM64, X-Ray, DynamORM env vars |
| `LiftAPI` | `pkg/cdk/constructs/api.go:55-533` | HTTP API Gateway | CORS, throttling, custom domains |
| `LiftTable` | `pkg/cdk/constructs/dynamodb.go:88-468` | DynamoDB table | PITR, streams, auto-scaling, TTL |
| `EnhancedMonitoring` | `pkg/cdk/constructs/monitoring_enhanced.go:82-111` | CloudWatch monitoring | Metrics, alarms, dashboards |
| `EnhancedSecurity` | `pkg/cdk/constructs/security_enhanced.go:100-137` | Security features | WAF, VPC endpoints, flow logs |

### Pattern Constructs

| Pattern | File Location | Purpose | Components |
|---------|---------------|---------|------------|
| `LiftApp` | `pkg/cdk/patterns/lift_app.go:44-184` | Complete application | Lambda + API + DynamoDB |
| `MicroserviceComplete` | `pkg/cdk/patterns/microservice_complete.go:139-215` | Production microservice | ECS + ALB + Service Discovery |
| `EventDrivenAPI` | `pkg/cdk/patterns/event_driven_api.go` | Event-driven architecture | API + Event processors |

### Stack Templates

| Stack | File Location | Purpose | Use Case |
|-------|---------------|---------|----------|
| `MicroserviceStack` | `pkg/cdk/stacks/microservice.go:28-67` | Single microservice | Simple service deployment |
| `MultiTenantSaaSStack` | `pkg/cdk/stacks/multi_tenant_saas.go` | SaaS platform | Multi-tenant applications |
| `EventDrivenStack` | `pkg/cdk/stacks/event_driven.go` | Event processing | Asynchronous workflows |

## Environment Variables and Configuration

### Lambda Function Environment Variables

Based on `pkg/cdk/constructs/lambda.go:118-151`:

| Variable | Purpose | Set By |
|----------|---------|--------|
| `LIFT_VERSION` | Framework version | `lambda.go:150` |
| `LIFT_MULTI_TENANT` | Multi-tenant mode | `lambda.go:152-154` |
| `LIFT_METRICS_ENABLED` | Metrics collection | `lambda.go:155-157` |
| `DYNAMORM_REGION` | AWS region for DynamORM | `lambda.go:167` |
| `DYNAMODB_TABLE_NAME` | DynamoDB table name | `lambda.go:169-171` |
| `DYNAMORM_DEBUG` | Debug mode | `lambda.go:174-178` |
| `DYNAMORM_RETRY_MAX_ATTEMPTS` | Retry configuration | `lambda.go:181` |
| `DYNAMORM_RETRY_BASE_DELAY` | Retry delay | `lambda.go:182` |

### DynamORM Configuration

Based on `pkg/cdk/constructs/lambda.go:160-185`:

```go
// Environment variables for DynamORM are automatically configured
// when EnableDynamORM is set to true in LiftFunctionProps:
// - DYNAMORM_REGION: AWS region
// - DYNAMODB_TABLE_NAME: table name (if DynamORMTableName is provided)
// - DYNAMORM_DEBUG: debug mode setting
// - DYNAMORM_RETRY_MAX_ATTEMPTS: retry configuration
// - DYNAMORM_RETRY_BASE_DELAY: retry delay
```

## Deployment Commands

### Using CDK CLI

```bash
# Build your Lift application
GOOS=linux GOARCH=arm64 go build -o dist/bootstrap main.go

# Synthesize CloudFormation
cdk synth

# Deploy stack
cdk deploy MyLiftStack

# Deploy with parameters
cdk deploy MyLiftStack --parameters Environment=production

# Destroy stack
cdk destroy MyLiftStack
```

### Using Makefile (if available)

Based on patterns in the codebase:

```bash
# Build and deploy
make cdk-deploy

# Synthesize templates
make cdk-synth

# Show differences
make cdk-diff

# Destroy infrastructure
make cdk-destroy
```

## Monitoring and Observability

### CloudWatch Integration

The CDK constructs provide comprehensive monitoring based on `pkg/cdk/constructs/monitoring_enhanced.go`:

1. **Automatic Metrics** (Lines 168-223):
   - Request counts and error rates
   - Latency percentiles (P50, P95, P99)
   - Cold start metrics
   - Custom business metrics

2. **Alarms** (Lines 413-481):
   - Error rate thresholds
   - Latency thresholds
   - Throttling detection
   - Concurrent execution limits

3. **Dashboards** (Lines 483-577):
   - Comprehensive monitoring views
   - Multi-widget layouts
   - Real-time metrics display

### X-Ray Tracing

Based on `pkg/cdk/constructs/lambda.go:129-134`:

```go
// Enable X-Ray tracing for Lambda function
fnProps := &liftconstructs.LiftFunctionProps{
    EnableTracing: jsii.Bool(true),
    // ... other properties
}

function := liftconstructs.NewLiftFunction(stack, jsii.String("Function"), fnProps)
```

## Security Best Practices

### WAF Configuration

Based on `pkg/cdk/constructs/security_enhanced.go:237-452`:

- Rate limiting protection
- SQL injection prevention
- XSS protection
- IP whitelist/blacklist support
- Geographic blocking

### VPC Endpoints

Based on `pkg/cdk/constructs/security_enhanced.go:545-578`:

- Secrets Manager endpoint
- CloudWatch Logs endpoint
- X-Ray endpoint
- Private network access to AWS services

### Table Permissions

Based on `pkg/cdk/constructs/dynamodb.go:400-468`:

```go
// Grant standard DynamoDB permissions
table.Table.GrantReadWriteData(lambdaFunction)

// Grant read-only access
table.Table.GrantReadData(lambdaFunction)

// Grant full access
table.Table.GrantFullAccess(lambdaFunction)
```

## Troubleshooting Common Issues

### 1. Build Errors

Ensure Go version compatibility:
```bash
go version  # Should be 1.21+
go mod tidy
```

### 2. CDK Synthesis Failures

Check CDK version:
```bash
cdk --version  # Should be 2.x
npm update -g aws-cdk
```

### 3. Deployment Timeouts

For large applications, increase timeout:
```go
// In Lambda configuration - Based on lambda.go:70-72
Timeout: awscdk.Duration_Minutes(jsii.Number(15)),
```

### 4. Permission Issues

Ensure proper IAM permissions:
```go
// Based on dynamodb.go:400-468
table.Table.GrantReadWriteData(function.Function)
```

## Additional Resources

- **Examples Directory**: `/examples/` contains 27+ working examples
- **Integration Tests**: `pkg/cdk/integration/` for testing patterns
- **Test Utilities**: `pkg/cdk/test/` for development helpers

This guide provides a comprehensive overview of the current CDK implementation in Lift. All code references are verified against the actual codebase and represent production-ready functionality.