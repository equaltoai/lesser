# Lift CDK Getting Started Guide

This guide walks you through your first CDK integration with Lift, providing step-by-step instructions with specific code references from the current implementation.

## Prerequisites

- **AWS CDK v2.100.0+** installed and configured
- **Go 1.21+** for CDK Go bindings  
- **AWS CLI** configured with appropriate permissions
- **Lift application** built and ready for deployment

## Quick Start (5 minutes)

### Step 1: Initialize CDK Project

```bash
mkdir my-lift-cdk && cd my-lift-cdk
cdk init app --language go
```

### Step 2: Add Lift Dependencies

Add to your `go.mod`:

```go
require (
    github.com/aws/aws-cdk-go/awscdk/v2 v2.100.0
    github.com/aws/constructs-go/constructs/v10 v10.3.0
    github.com/aws/jsii-runtime-go v1.90.0
    github.com/pay-theory/lift/pkg/cdk v0.1.0
)
```

### Step 3: Create Basic Stack

Replace the contents of your main stack file with this basic implementation based on `pkg/cdk/patterns/lift_app.go:53-184`:

```go
package main

import (
    "github.com/aws/aws-cdk-go/awscdk/v2"
    "github.com/aws/constructs-go/constructs/v10"
    "github.com/aws/jsii-runtime-go"
    "github.com/pay-theory/lift/pkg/cdk/patterns"
)

type MyLiftStackProps struct {
    awscdk.StackProps
}

func NewMyLiftStack(scope constructs.Construct, id string, props *MyLiftStackProps) awscdk.Stack {
    var sprops awscdk.StackProps
    if props != nil {
        sprops = props.StackProps
    }
    stack := awscdk.NewStack(scope, &id, &sprops)

    // Create complete Lift application - Based on lift_app.go:53-99
    app := patterns.NewLiftApp(stack, jsii.String("MyApp"), &patterns.LiftAppProps{
        AppName:           jsii.String("my-lift-app"),
        CodeAssetPath:     jsii.String("./dist"),
        EnableDatabase:    jsii.Bool(true),
        EnableRateLimiting: jsii.Bool(true),
        EnableMultiTenant: jsii.Bool(false),
        Environment: &map[string]*string{
            "LOG_LEVEL": jsii.String("info"),
        },
    })

    // Add stack outputs - Based on lift_app.go:166-183
    awscdk.NewCfnOutput(stack, jsii.String("ApiUrl"), &awscdk.CfnOutputProps{
        Value:       app.API.GetUrl(),
        Description: jsii.String("API Gateway endpoint URL"),
        ExportName:  jsii.String("MyLiftApp-ApiUrl"),
    })

    awscdk.NewCfnOutput(stack, jsii.String("FunctionName"), &awscdk.CfnOutputProps{
        Value:       app.Function.Function.FunctionName(),
        Description: jsii.String("Lambda function name"),
        ExportName:  jsii.String("MyLiftApp-FunctionName"),
    })

    return stack
}

func main() {
    defer jsii.Close()

    app := awscdk.NewApp(nil)

    NewMyLiftStack(app, "MyLiftStack", &MyLiftStackProps{
        StackProps: awscdk.StackProps{
            Env: env(),
        },
    })

    app.Synth(nil)
}

// env determines the AWS environment (account+region) in which our stack is to be deployed.
func env() *awscdk.Environment {
    return &awscdk.Environment{
        Account: jsii.String("123456789012"), // Replace with your account
        Region:  jsii.String("us-east-1"),    // Replace with your region
    }
}
```

### Step 4: Build Your Lift Application

```bash
# In your Lift app directory
GOOS=linux GOARCH=arm64 go build -o dist/bootstrap main.go
```

### Step 5: Deploy

```bash
# In your CDK directory
go mod tidy
cdk synth
cdk deploy
```

**Result**: You now have a complete serverless application with API Gateway, Lambda, and DynamoDB deployed!

## Detailed Walkthrough

### Understanding the LiftApp Pattern

The `LiftApp` pattern (`pkg/cdk/patterns/lift_app.go`) creates a complete serverless application with these components:

#### Components Created

**Location**: `lift_app.go:87-163`

| Component | Type | Purpose | Code Reference |
|-----------|------|---------|----------------|
| Lambda Function | `LiftFunction` | Main application handler | `lift_app.go:87-100` |
| API Gateway | `LiftAPI` | HTTP API with CORS | `lift_app.go:139-149` |
| DynamoDB Table | `LiftTable` | Application data storage | `lift_app.go:102-120` |
| Rate Limit Table | `LiftTable` | Rate limiting storage | `lift_app.go:122-137` |

#### Automatic Route Configuration

**Location**: `lift_app.go:152-163`

```go
// Catch-all route for SPA support
app.API.AddLambdaRoute(
    jsii.String("/{proxy+}"),
    awsapigatewayv2.HttpMethod_ANY,
    app.Function.Function,
)

// Root route
app.API.AddLambdaRoute(
    jsii.String("/"),
    awsapigatewayv2.HttpMethod_ANY,
    app.Function.Function,
)
```

### Advanced Configuration Examples

#### Example 1: Production-Ready Application

Based on `pkg/cdk/constructs/monitoring_enhanced.go` and `pkg/cdk/constructs/security_enhanced.go`:

```go
func NewProductionLiftStack(scope constructs.Construct, id string, props *MyLiftStackProps) awscdk.Stack {
    stack := awscdk.NewStack(scope, &id, &props.StackProps)

    // Create VPC for security
    vpc := awsec2.NewVpc(stack, jsii.String("VPC"), &awsec2.VpcProps{
        MaxAzs: jsii.Number(2),
        NatGateways: jsii.Number(1),
    })

    // Create enhanced security - Based on security_enhanced.go:101-136
    security := liftconstructs.NewEnhancedSecurity(stack, jsii.String("Security"), &liftconstructs.EnhancedSecurityProps{
        Vpc:                vpc,
        EnableWAF:          jsii.Bool(true),
        EnableVPCFlowLogs:  jsii.Bool(true),
        Environment:        jsii.String("production"),
        ApplicationName:    jsii.String("my-lift-app"),
        WAFConfig: &liftconstructs.WAFRuleConfig{
            EnableRateLimit:      jsii.Bool(true),
            RateLimit:            jsii.Number(5000),
            EnableSQLiProtection: jsii.Bool(true),
            EnableXSSProtection:  jsii.Bool(true),
        },
    })

    // Create application with enhanced features
    app := patterns.NewLiftApp(stack, jsii.String("App"), &patterns.LiftAppProps{
        AppName:           jsii.String("my-production-app"),
        CodeAssetPath:     jsii.String("./dist"),
        EnableDatabase:    jsii.Bool(true),
        EnableRateLimiting: jsii.Bool(true),
        EnableMultiTenant: jsii.Bool(true),
        MemorySize:        jsii.Number(1024),
        Timeout:           jsii.Number(60),
        Environment: &map[string]*string{
            "LOG_LEVEL":     jsii.String("warn"),
            "ENVIRONMENT":   jsii.String("production"),
        },
    })

    // Add enhanced monitoring - Based on monitoring_enhanced.go:81-108
    monitoring := liftconstructs.NewEnhancedMonitoring(stack, jsii.String("Monitoring"), &liftconstructs.EnhancedMonitoringProps{
        Namespace:   jsii.String("MyApp/Production"),
        Environment: jsii.String("production"),
        MetricConfig: &liftconstructs.MetricConfiguration{
            DetailedMetrics:       jsii.Bool(true),
            EnableBusinessMetrics: jsii.Bool(true),
        },
        AlarmThresholds: &liftconstructs.AlarmThresholds{
            ErrorRate:            jsii.Number(1.0), // 1% error rate
            LatencyP99:           jsii.Number(2000), // 2 seconds
            ThrottleCount:        jsii.Number(5),
            ConcurrentExecutions: jsii.Number(100),
        },
        EnableRealTimeStreaming: jsii.Bool(true),
    })

    return stack
}
```

#### Example 2: Multi-Tenant SaaS Application

Based on `pkg/cdk/stacks/multi_tenant_saas.go`:

```go
import (
    "github.com/pay-theory/lift/pkg/cdk/stacks"
)

func NewMultiTenantSaaSStack(scope constructs.Construct, id string, props *MyLiftStackProps) awscdk.Stack {
    // Use the pre-built multi-tenant SaaS stack
    return stacks.NewMultiTenantSaaSStack(scope, "SaaSPlatform", &stacks.MultiTenantSaaSStackProps{
        AppName:           "saas-platform",
        CodePath:          "./dist/bootstrap",
        EnableAuth:        true,
        EnableFileStorage: true,
        DomainName:        "api.saas-platform.com",
        CertificateArn:    "arn:aws:acm:us-east-1:123456789012:certificate/...",
    })
}
```

#### Example 2b: Custom Multi-Tenant Implementation

For custom multi-tenant implementations using Lift constructs:

```go
func NewCustomMultiTenantStack(scope constructs.Construct, id string, props *MyLiftStackProps) awscdk.Stack {
    stack := awscdk.NewStack(scope, &id, &props.StackProps)

    // Create multi-tenant DynamoDB table using LiftTable
    table := liftconstructs.NewLiftTable(stack, jsii.String("SaaSData"), &liftconstructs.LiftTableProps{
        TableName:                 jsii.String("saas-data"),
        PartitionKeyName:          jsii.String("PK"),
        SortKeyName:               jsii.String("SK"),
        EnableMultiTenant:         jsii.Bool(true),
        TenantAttribute:           jsii.String("TenantID"),
        EnableAutoScaling:         jsii.Bool(true),
        EnablePointInTimeRecovery: jsii.Bool(true),
        EnableStreams:             jsii.Bool(true),
        TimeToLiveAttribute:       jsii.String("ttl"),
    })

    // Create Lambda function with multi-tenant support
    fn := liftconstructs.NewLiftFunction(stack, jsii.String("SaaSFunction"), &liftconstructs.LiftFunctionProps{
        FunctionProps: awslambda.FunctionProps{
            Code:         awslambda.Code_FromAsset(jsii.String("./dist"), nil),
            Handler:      jsii.String("bootstrap"),
            MemorySize:   jsii.Number(1024),
            Timeout:      awscdk.Duration_Minutes(jsii.Number(5)),
        },
        EnableTracing:     jsii.Bool(true),
        EnableMultiTenant: jsii.Bool(true),
    })

    // Grant table access to function
    table.Table.GrantReadWriteData(fn.Function)

    // Create API with enhanced CORS for SaaS
    api := liftconstructs.NewLiftAPI(stack, jsii.String("SaaSAPI"), &liftconstructs.LiftAPIProps{
        Name:                jsii.String("saas-api"),
        EnableCORS:          jsii.Bool(true),
        EnableAccessLogging: jsii.Bool(true),
        DomainName:          jsii.String("api.saas-platform.com"),
        CertificateArn:      jsii.String("arn:aws:acm:us-east-1:123456789012:certificate/..."),
    })

    // Add catch-all route
    api.AddLambdaRoute(
        jsii.String("/{proxy+}"),
        awsapigatewayv2.HttpMethod_ANY,
        fn.Function,
    )

    return stack
}
```

#### Example 3: Event-Driven Microservices with DynamORM Event Store

Based on `pkg/cdk/patterns/microservice_complete.go` and `pkg/cdk/constructs/dynamorm_event_store.go`:

```go
func NewEventDrivenStack(scope constructs.Construct, id string, props *MyLiftStackProps) awscdk.Stack {
    stack := awscdk.NewStack(scope, &id, &props.StackProps)

    // Create complete microservice with ECS - Based on microservice_complete.go:166-215
    microservice := patterns.NewMicroserviceComplete(stack, jsii.String("OrderService"), &patterns.MicroserviceCompleteProps{
        ServiceName: jsii.String("order-service"),
        Environment: jsii.String("production"),
        
        // Container configuration - Based on microservice_complete.go:95-111
        ContainerConfig: &patterns.ContainerConfig{
            CodeAssetPath:     jsii.String("./dist"),
            Platform:         awsecs.CpuArchitecture_ARM64(),
            CPU:              jsii.Number(512),
            Memory:           jsii.Number(1024),
            EnableXRayTracing: jsii.Bool(true),
        },
        
        // Service discovery - Based on microservice_complete.go:18-29
        ServiceDiscovery: &patterns.ServiceDiscoveryConfig{
            Namespace:       jsii.String("order-system.local"),
            ServiceName:     jsii.String("order-service"),
            HealthCheckPath: jsii.String("/health"),
            TTL:             awscdk.Duration_Seconds(jsii.Number(10)),
        },
        
        // Auto-scaling - Based on microservice_complete.go:49-61
        AutoScaling: &patterns.AutoScalingConfig{
            MinCapacity:             jsii.Number(2),
            MaxCapacity:             jsii.Number(20),
            TargetCPUUtilization:    jsii.Number(70),
            TargetMemoryUtilization: jsii.Number(80),
        },
        
        // Enhanced monitoring and security
        EnableEnhancedMonitoring: jsii.Bool(true),
        EnableEnhancedSecurity:   jsii.Bool(true),
    })

    // Create DynamORM Event Store for event sourcing
    eventStore := liftconstructs.NewDynamORMEventStore(stack, jsii.String("EventStore"), &liftconstructs.DynamORMEventStoreProps{
        EventTableName:         jsii.String("order-events"),
        SnapshotTableName:      jsii.String("order-snapshots"),
        Pattern:                liftconstructs.EventStorePattern_SINGLE_TABLE,
        SnapshotStrategy:       liftconstructs.SnapshotStrategy_FREQUENCY,
        EnableMultiTenant:      jsii.Bool(true),
        TenantAttribute:        jsii.String("TenantID"),
        EnableEventVersioning:  jsii.Bool(true),
        EnableEventEncryption:  jsii.Bool(true),
        EnableAutoScaling:      jsii.Bool(true),
        EnableMetrics:          jsii.Bool(true),
        EnableDetailedMetrics:  jsii.Bool(true),
    })

    // Create EventBridge for service communication
    eventBus := awsevents.NewEventBus(stack, jsii.String("OrderEventBus"), &awsevents.EventBusProps{
        EventBusName: jsii.String("order-events"),
    })

    // Create event processor Lambda with event store access
    processor := liftconstructs.NewLiftFunction(stack, jsii.String("EventProcessor"), &liftconstructs.LiftFunctionProps{
        FunctionProps: awslambda.FunctionProps{
            Code:       awslambda.Code_FromAsset(jsii.String("./dist/processor"), nil),
            Handler:    jsii.String("bootstrap"),
            MemorySize: jsii.Number(512),
            Environment: eventStore.GetEnvironmentVariables(),
        },
        EnableTracing: jsii.Bool(true),
    })

    // Grant event store access to processor
    eventStore.GrantEventReaderAccess(processor.Function)

    // Add event rule
    awsevents.NewRule(stack, jsii.String("OrderCreatedRule"), &awsevents.RuleProps{
        EventBus: eventBus,
        EventPattern: &awsevents.EventPattern{
            Source:     &[]*string{jsii.String("order.service")},
            DetailType: &[]*string{jsii.String("Order Created")},
        },
        Targets: &[]awsevents.IRuleTarget{
            awseventstargets.NewLambdaFunction(processor.Function, nil),
        },
    })

    return stack
}
```

## Best Practices Implementation

### 1. Environment-Specific Configuration

**Create environment configs based on the YAML template**:

```go
type EnvironmentConfig struct {
    Environment   string
    MemorySize    float64
    LogRetention  float64
    EnableDebug   bool
    RateLimit     float64
}

func getEnvironmentConfig(env string) *EnvironmentConfig {
    configs := map[string]*EnvironmentConfig{
        "development": {
            Environment:  "development",
            MemorySize:   512,
            LogRetention: 7,
            EnableDebug:  true,
            RateLimit:    100,
        },
        "staging": {
            Environment:  "staging", 
            MemorySize:   1024,
            LogRetention: 14,
            EnableDebug:  false,
            RateLimit:    1000,
        },
        "production": {
            Environment:  "production",
            MemorySize:   2048,
            LogRetention: 90,
            EnableDebug:  false,
            RateLimit:    10000,
        },
    }
    return configs[env]
}
```

### 2. Security by Default

**Always enable security features in production**:

```go
// Security configuration based on security_enhanced.go
securityConfig := &liftconstructs.EnhancedSecurityProps{
    Vpc:                vpc,
    EnableWAF:          jsii.Bool(true),
    EnableVPCFlowLogs:  jsii.Bool(true),
    Environment:        jsii.String("production"),
    ApplicationName:    jsii.String("my-app"),
    // WAF rules based on security_enhanced.go:155-163
    WAFConfig: &liftconstructs.WAFRuleConfig{
        EnableRateLimit:      jsii.Bool(true),
        RateLimit:            jsii.Number(5000),
        EnableSQLiProtection: jsii.Bool(true),
        EnableXSSProtection:  jsii.Bool(true),
        EnableKnownBadInputs: jsii.Bool(true),
    },
}
```

### 3. Comprehensive Monitoring

**Set up monitoring for all environments**:

```go
// Monitoring configuration based on monitoring_enhanced.go
monitoringConfig := &liftconstructs.EnhancedMonitoringProps{
    Namespace:   jsii.String("MyApp/" + environment),
    Environment: jsii.String(environment),
    MetricConfig: &liftconstructs.MetricConfiguration{
        DetailedMetrics:       jsii.Bool(true),
        EnableBusinessMetrics: jsii.Bool(true),
        Resolution:            jsii.Number(1), // 1-second resolution
    },
    // Alarm thresholds based on monitoring_enhanced.go:131-138
    AlarmThresholds: &liftconstructs.AlarmThresholds{
        ErrorRate:            jsii.Number(5.0),   // 5% error rate
        LatencyP99:           jsii.Number(3000),  // 3 seconds
        ThrottleCount:        jsii.Number(5),
        ConcurrentExecutions: jsii.Number(100),
    },
    EnableRealTimeStreaming: jsii.Bool(isProd),
}
```

### 4. Database Best Practices

**Configure DynamoDB with proper isolation**:

```go
// DynamoDB configuration using LiftTable
tableConfig := &liftconstructs.LiftTableProps{
    TableName:                 jsii.String("app-data"),
    PartitionKeyName:          jsii.String("PK"),
    SortKeyName:               jsii.String("SK"),
    EnablePointInTimeRecovery: jsii.Bool(true),
    EnableStreams:             jsii.Bool(true),
    TimeToLiveAttribute:       jsii.String("ttl"),
    EnableAutoScaling:         jsii.Bool(isProd),
    // Multi-tenant configuration
    EnableMultiTenant:         jsii.Bool(true),
    TenantAttribute:           jsii.String("TenantID"),
}
```

## Common Deployment Scenarios

### Scenario 1: API-Only Application

```go
// Simple REST API without database
app := patterns.NewLiftApp(stack, jsii.String("APIApp"), &patterns.LiftAppProps{
    AppName:           jsii.String("rest-api"),
    CodeAssetPath:     jsii.String("./dist"),
    EnableDatabase:    jsii.Bool(false),
    EnableRateLimiting: jsii.Bool(true),
    MemorySize:        jsii.Number(512),
    Environment: &map[string]*string{
        "API_VERSION": jsii.String("v1"),
    },
})
```

### Scenario 2: Full-Stack Application with Database

```go
// Complete application with database and rate limiting
app := patterns.NewLiftApp(stack, jsii.String("FullStackApp"), &patterns.LiftAppProps{
    AppName:           jsii.String("full-stack-app"),
    CodeAssetPath:     jsii.String("./dist"),
    EnableDatabase:    jsii.Bool(true),
    EnableRateLimiting: jsii.Bool(true),
    EnableMultiTenant: jsii.Bool(false),
    DatabaseTableName: jsii.String("app-data"),
    RateLimitTableName: jsii.String("app-rate-limits"),
})
```

### Scenario 3: Multi-Tenant SaaS Platform

```go
// Multi-tenant SaaS with enhanced security and monitoring
import "github.com/pay-theory/lift/pkg/cdk/stacks"

saasStack := stacks.NewMultiTenantSaaSStack(app, "SaaSPlatform", &stacks.MultiTenantSaaSStackProps{
    AppName:           "saas-platform",
    CodePath:          "./dist/bootstrap",
    EnableAuth:        true,
    EnableFileStorage: true,
    DomainName:        "api.platform.com",
    CertificateArn:    "arn:aws:acm:...",
})
```

## Troubleshooting Common Issues

### 1. Build Errors

**Go Module Issues**:
```bash
go mod init my-lift-cdk
go mod tidy
go get github.com/pay-theory/lift/pkg/cdk
```

**CDK Version Conflicts**:
```bash
npm install -g aws-cdk@latest
cdk --version  # Should be 2.100.0+
```

### 2. Deployment Failures

**Permission Issues**:
```bash
# Ensure your AWS credentials have sufficient permissions
aws sts get-caller-identity
aws iam get-user
```

**Bootstrap CDK Environment**:
```bash
cdk bootstrap aws://123456789012/us-east-1
```

### 3. Function Errors

**Memory/Timeout Issues**:
```go
// Increase resources for complex applications
FunctionProps: awslambda.FunctionProps{
    MemorySize: jsii.Number(2048),
    Timeout:    awscdk.Duration_Minutes(jsii.Number(15)),
}
```

**Environment Variable Issues**:
```go
// Debug environment variables
Environment: &map[string]*string{
    "LOG_LEVEL":    jsii.String("debug"),
    "AWS_REGION":   jsii.String("us-east-1"),
    "ENVIRONMENT":  jsii.String("development"),
}
```

## Next Steps

1. **Explore Advanced Patterns**: Check `pkg/cdk/patterns/` for more complex architectures
2. **Security Enhancement**: Implement `pkg/cdk/constructs/security_enhanced.go` features
3. **Monitoring Setup**: Use `pkg/cdk/constructs/monitoring_enhanced.go` for observability
4. **Multi-Region Deployment**: Extend stacks for multiple AWS regions
5. **CI/CD Integration**: Set up automated deployments with GitHub Actions or CodePipeline

## Additional Resources

- **Examples Directory**: `/examples/` contains 27+ working examples
- **CDK Patterns**: `/pkg/cdk/patterns/` for reusable architectures  
- **CDK Stacks**: `/pkg/cdk/stacks/` for complete stack implementations
- **Security Features**: `/pkg/cdk/constructs/security_enhanced.go`
- **Monitoring Tools**: `/pkg/cdk/constructs/monitoring_enhanced.go`
- **DynamORM Integration**: 
  - `/pkg/cdk/constructs/dynamorm_event_store.go` for event sourcing
  - `/pkg/cdk/constructs/dynamorm_crud_handlers.go` for CRUD operations

This guide provides a solid foundation for deploying Lift applications with AWS CDK. All code examples reference the actual implementation in the codebase and represent production-ready patterns.