# Lift CDK API Reference

This document provides a comprehensive API reference for all Lift CDK constructs, patterns, and stacks with specific code locations and usage examples.

## Table of Contents

- [Core Constructs](#core-constructs)
  - [LiftFunction](#liftfunction) - `pkg/cdk/constructs/lambda.go`
  - [LiftAPI](#liftapi) - `pkg/cdk/constructs/api.go`
  - [LiftTable](#lifttable) - `pkg/cdk/constructs/dynamodb.go`
- [Middleware Constructs](#middleware-constructs)
  - [RateLimitedFunction](#ratelimitedfunction) - `pkg/cdk/constructs/ratelimited.go`
  - [IdempotentFunction](#idempotentfunction) - `pkg/cdk/constructs/idempotent.go`
  - [SecureFunction](#securefunction) - `pkg/cdk/constructs/secure.go`
  - [MonitoredFunction](#monitoredfunction) - `pkg/cdk/constructs/monitored.go`
- [Enhanced Constructs](#enhanced-constructs)
  - [EnhancedMonitoring](#enhancedmonitoring) - `pkg/cdk/constructs/monitoring_enhanced.go`
  - [EnhancedSecurity](#enhancedsecurity) - `pkg/cdk/constructs/security_enhanced.go`
- [Pattern Constructs](#pattern-constructs)
  - [BasicAPI](#basicapi) - `pkg/cdk/patterns/basic_api.go`
  - [SecureAPI](#secureapi) - `pkg/cdk/patterns/secure_api.go`
  - [LiftApp](#liftapp) - `pkg/cdk/patterns/lift_app.go`
  - [MicroserviceComplete](#microservicecomplete) - `pkg/cdk/patterns/microservice_complete.go`
- [Stack Templates](#stack-templates)
  - [MicroserviceStack](#microservicestack) - `pkg/cdk/stacks/microservice.go`
  - [MultiTenantSaaSStack](#multitenantssaasstack) - `pkg/cdk/stacks/multi_tenant_saas.go`
  - [EventDrivenStack](#eventdrivenstack) - `pkg/cdk/stacks/event_driven.go`

## Core Constructs

### LiftFunction

**File**: `pkg/cdk/constructs/lambda.go`  
**Type**: Lambda Function Construct  
**Lines**: 44-277

Base Lambda function construct optimized for Lift applications with ARM64 support, Dead Letter Queues, and DynamORM integration.

#### Constructor

```go
func NewLiftFunction(scope constructs.Construct, id *string, props *LiftFunctionProps) *LiftFunction
```

**Location**: `lambda.go:57-170`

#### Properties (LiftFunctionProps)

**Struct Definition**: `lambda.go:17-41`

| Property | Type | Default | Description | Code Reference |
|----------|------|---------|-------------|----------------|
| `Runtime` | `awslambda.Runtime` | `PROVIDED_AL2023` | Lambda runtime | `lambda.go:61-63` |
| `Architecture` | `awslambda.Architecture` | `ARM_64` | CPU architecture | `lambda.go:64-66` |
| `MemorySize` | `*float64` | `512` | Memory in MB | `lambda.go:67-69` |
| `Timeout` | `awscdk.Duration` | `30s` | Function timeout | `lambda.go:70-72` |
| `EnableTracing` | `*bool` | `false` | X-Ray tracing | `lambda.go:73-75` |
| `EnableMetrics` | `*bool` | `false` | CloudWatch metrics | `lambda.go:76-78` |
| `EnableMultiTenant` | `*bool` | `false` | Multi-tenant support | `lambda.go:79-81` |
| `ReservedConcurrentExecutions` | `*float64` | `nil` | Concurrent execution limit | `lambda.go:82-84` |
| `EnableDynamORM` | `*bool` | `false` | DynamORM integration | `lambda.go:132-149` |
| `DynamORMTableName` | `*string` | `nil` | DynamORM table name | `lambda.go:132-149` |
| `DynamORMDebug` | `*bool` | `false` | DynamORM debug mode | `lambda.go:132-149` |

#### Methods

| Method | Return Type | Description | Code Reference |
|--------|-------------|-------------|----------------|
| `GetFunction()` | `awslambda.Function` | Returns underlying Lambda | `lambda.go:173-175` |
| `GetLogGroup()` | `awslogs.LogGroup` | Returns log group | `lambda.go:177-179` |
| `AddEnvironment(key, value)` | `void` | Adds environment variable | `lambda.go:188-190` |
| `GrantInvoke(grantee)` | `awsiam.Grant` | Grants invoke permission | `lambda.go:193-195` |
| `ConfigureDynamORM(table, debug)` | `void` | Configures DynamORM | `lambda.go:213-224` |

#### DynamORM Environment Variables

**Location**: `lambda.go:132-149`

```go
// Automatic DynamORM configuration
env["DYNAMORM_REGION"] = awscdk.Stack_Of(this).Region()
env["DYNAMODB_TABLE_NAME"] = props.DynamORMTableName
env["DYNAMORM_DEBUG"] = jsii.String(debugMode)
env["DYNAMORM_RETRY_MAX_ATTEMPTS"] = jsii.String("3")
env["DYNAMORM_RETRY_BASE_DELAY"] = jsii.String("100")
```

#### Example

```go
fn := liftconstructs.NewLiftFunction(this, jsii.String("MyFunction"), &liftconstructs.LiftFunctionProps{
    FunctionProps: awslambda.FunctionProps{
        Code:    awslambda.Code_FromAsset(jsii.String("./dist"), nil),
        Handler: jsii.String("bootstrap"),
    },
    EnableTracing:  jsii.Bool(true),
    EnableMetrics:  jsii.Bool(true),
    EnableDynamORM: jsii.Bool(true),
    DynamORMTableName: jsii.String("my-table"),
    MemorySize:     jsii.Number(1024),
    ReservedConcurrentExecutions: jsii.Number(100),
})

// Configure DynamORM after creation
fn.ConfigureDynamORM(jsii.String("my-table"), jsii.Bool(true))
```

### LiftAPI

API Gateway v2 construct integrated with Lift.

```go
api := constructs.NewLiftAPI(stack, id, props)
```

#### Properties

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `ApiName` | `*string` | Required | API name |
| `LiftHandler` | `LiftFunction` | Required | Lift function to integrate |
| `Description` | `*string` | `nil` | API description |
| `CorsOrigins` | `[]*string` | `["*"]` | CORS allowed origins |
| `CorsHeaders` | `[]*string` | `["*"]` | CORS allowed headers |
| `CorsMethods` | `[]*string` | All methods | CORS allowed methods |
| `CorsExposeHeaders` | `[]*string` | `nil` | CORS expose headers |
| `CustomDomainName` | `*string` | `nil` | Custom domain |
| `CertificateArn` | `*string` | `nil` | ACM certificate ARN |
| `StageName` | `*string` | `"$default"` | API stage name |
| `ThrottleRateLimit` | `*float64` | `10000` | Requests per second |
| `ThrottleBurstLimit` | `*float64` | `5000` | Burst capacity |
| `DisableExecuteApiEndpoint` | `*bool` | `false` | Disable default endpoint |
| `RouteSelectionExpression` | `*string` | `"$request.method $request.path"` | Route selection |
| `ValidateRequestBody` | `*bool` | `false` | Enable request validation |
| `ValidateRequestParameters` | `*bool` | `false` | Enable parameter validation |
| `EnableAccessLogging` | `*bool` | `false` | Enable access logs |
| `AccessLogFormat` | `*string` | JSON format | Access log format |
| `EnableDetailedMetrics` | `*bool` | `false` | Enable detailed CloudWatch metrics |

#### Methods

| Method | Description |
|--------|-------------|
| `Api() awsapigatewayv2.HttpApi` | Get underlying API Gateway |
| `DefaultStage() awsapigatewayv2.HttpStage` | Get default stage |
| `Url() *string` | Get API endpoint URL |
| `CustomDomain() awsapigatewayv2.DomainName` | Get custom domain (if configured) |

#### Example

```go
api := constructs.NewLiftAPI(stack, jsii.String("API"), &constructs.LiftAPIProps{
    ApiName: jsii.String("my-api"),
    LiftHandler: myFunction,
    
    // CORS configuration
    CorsOrigins: jsii.Strings("https://example.com", "https://app.example.com"),
    CorsHeaders: jsii.Strings("Content-Type", "Authorization"),
    
    // Custom domain
    CustomDomainName: jsii.String("api.example.com"),
    CertificateArn: jsii.String("arn:aws:acm:us-east-1:123456789012:certificate/..."),
    
    // Throttling
    ThrottleRateLimit: jsii.Number(1000),
    ThrottleBurstLimit: jsii.Number(2000),
    
    // Logging
    EnableAccessLogging: jsii.Bool(true),
    EnableDetailedMetrics: jsii.Bool(true),
})
```

### LiftTable

DynamoDB table construct with single-table design support.

```go
table := constructs.NewLiftTable(stack, id, props)
```

#### Properties

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `TableName` | `*string` | Auto-generated | Table name |
| `PartitionKeyName` | `*string` | Required | Partition key field name |
| `SortKeyName` | `*string` | `nil` | Sort key field name |
| `EnablePointInTimeRecovery` | `*bool` | `true` | Enable PITR |
| `EnableStreams` | `*bool` | `false` | Enable DynamoDB Streams |
| `StreamViewType` | `awsdynamodb.StreamViewType` | `NEW_AND_OLD_IMAGES` | Stream view type |
| `TimeToLiveAttribute` | `*string` | `nil` | TTL attribute |
| `ReadCapacity` | `*float64` | `nil` | Read capacity (provisioned mode) |
| `WriteCapacity` | `*float64` | `nil` | Write capacity (provisioned mode) |
| `EnableAutoScaling` | `*bool` | `true` | Enable auto-scaling |
| `MinReadCapacity` | `*float64` | `5` | Min read capacity |
| `MaxReadCapacity` | `*float64` | `40000` | Max read capacity |
| `MinWriteCapacity` | `*float64` | `5` | Min write capacity |
| `MaxWriteCapacity` | `*float64` | `40000` | Max write capacity |
| `TargetUtilization` | `*float64` | `70` | Target utilization % |
| `GlobalSecondaryIndexes` | `*[]*awsdynamodb.GlobalSecondaryIndexProps` | `nil` | GSIs |
| `Encryption` | `awsdynamodb.TableEncryption` | `AWS_MANAGED` | Encryption type |
| `RemovalPolicy` | `awscdk.RemovalPolicy` | `RETAIN` | Deletion policy |
| `DeletionProtection` | `*bool` | `false` | Deletion protection |
| `ReplicationRegions` | `*[]*string` | `nil` | Global Tables regions |
| `Tags` | `*map[string]*string` | `nil` | Custom tags |

#### Example

```go
table := constructs.NewLiftTable(stack, jsii.String("DataTable"), &constructs.LiftTableProps{
    PartitionKeyName: jsii.String("PK"),
    SortKeyName: jsii.String("SK"),
    EnablePointInTimeRecovery: jsii.Bool(true),
    EnableStreams: jsii.Bool(true),
    StreamViewType: awsdynamodb.StreamViewType_NEW_AND_OLD_IMAGES,
    TimeToLiveAttribute: jsii.String("ttl"),
    EnableAutoScaling: jsii.Bool(true),
    MinReadCapacity: jsii.Number(5),
    MaxReadCapacity: jsii.Number(1000),
    MinWriteCapacity: jsii.Number(5),
    MaxWriteCapacity: jsii.Number(1000),
    GlobalSecondaryIndexes: &[]*awsdynamodb.GlobalSecondaryIndexProps{
        {
            IndexName: jsii.String("GSI1"),
            PartitionKey: &awsdynamodb.Attribute{
                Name: jsii.String("GSI1PK"),
                Type: awsdynamodb.AttributeType_STRING,
            },
            SortKey: &awsdynamodb.Attribute{
                Name: jsii.String("GSI1SK"),
                Type: awsdynamodb.AttributeType_STRING,
            },
        },
    },
})
```

## Middleware Constructs

### RateLimitedFunction

Lambda function with built-in rate limiting.

```go
rateLimited := constructs.NewRateLimitedFunction(stack, id, props)
```

#### Properties

Extends [LiftFunctionProps](#liftfunction) with:

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `RateLimitType` | `RateLimitType` | `RateLimitTypeIP` | Type: `RateLimitTypeIP`, `RateLimitTypeUser`, or `RateLimitTypeTenant` |
| `Limit` | `*float64` | `1000` | Requests per window |
| `WindowSeconds` | `*float64` | `3600` | Time window in seconds |
| `TableName` | `*string` | Auto-generated | DynamoDB table name |
| `EnableMetrics` | `*bool` | `true` | Enable CloudWatch metrics |

#### Methods

Inherits all [LiftFunction](#liftfunction) methods plus:

| Method | Description |
|--------|-------------|
| `RateLimitTable() awsdynamodb.Table` | Get rate limit table |

#### Example

```go
rateLimited := constructs.NewRateLimitedFunction(stack, jsii.String("API"), &constructs.RateLimitedFunctionProps{
    LiftFunctionProps: constructs.LiftFunctionProps{
        CodeAssetPath: jsii.String("./dist/bootstrap"),
        MemorySize: jsii.Number(1024),
    },
    RateLimitType: constructs.RateLimitTypeUser,  // Rate limit by user ID
    Limit: jsii.Number(100),                      // 100 requests
    WindowSeconds: jsii.Number(900),             // per 15 minutes
    EnableMetrics: jsii.Bool(true),
})
```

### IdempotentFunction

Lambda function with automatic idempotency.

```go
idempotent := constructs.NewIdempotentFunction(stack, id, props)
```

#### Properties

Extends [LiftFunctionProps](#liftfunction) with:

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `KeyExtractor` | `IdempotentKeyExtractor` | `IdempotentKeyHeader` | Source: `IdempotentKeyHeader`, `IdempotentKeyBody`, `IdempotentKeyPath`, or `IdempotentKeyCustom` |
| `KeyField` | `*string` | `"X-Idempotency-Key"` | Path to extract key |
| `TTLSeconds` | `*float64` | `86400` | Record TTL (24 hours) |
| `TableName` | `*string` | Auto-generated | DynamoDB table name |
| `EnableResponseCaching` | `*bool` | `true` | Enable response caching |
| `MaxResponseSizeKB` | `*float64` | `400` | Max response size (KB) |

#### Methods

Inherits all [LiftFunction](#liftfunction) methods plus:

| Method | Description |
|--------|-------------|
| `IdempotencyTable() awsdynamodb.Table` | Get idempotency table |

#### Example

```go
idempotent := constructs.NewIdempotentFunction(stack, jsii.String("Payment"), &constructs.IdempotentFunctionProps{
    LiftFunctionProps: constructs.LiftFunctionProps{
        CodeAssetPath: jsii.String("./dist/bootstrap"),
        Timeout: jsii.Number(60),
    },
    KeyExtractor: constructs.IdempotentKeyBody,
    KeyField: jsii.String("paymentId"),
    TTLSeconds: jsii.Number(172800), // 48 hours
    EnableResponseCaching: jsii.Bool(true),
    MaxResponseSizeKB: jsii.Number(400),
})
```

### SecureFunction

Lambda function with enhanced security features.

```go
secure := constructs.NewSecureFunction(stack, id, props)
```

#### Properties

Extends [LiftFunctionProps](#liftfunction) with:

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `Vpc` | `awsec2.IVpc` | `nil` | Existing VPC |
| `CreateVpc` | `*bool` | `false` | Create new VPC |
| `VpcCidr` | `*string` | `"10.0.0.0/16"` | VPC CIDR (if creating) |
| `UsePrivateSubnets` | `*bool` | `false` | Use private subnets only |
| `SecurityGroupIds` | `[]*string` | `nil` | Security group IDs |
| `SecretArns` | `[]*string` | `nil` | Secrets Manager ARNs |
| `EncryptEnvironmentVariables` | `*bool` | `false` | KMS encryption for env vars |
| `KmsKeyArn` | `*string` | `nil` | Custom KMS key ARN |

#### Methods

Inherits all [LiftFunction](#liftfunction) methods plus:

| Method | Description |
|--------|-------------|
| `Vpc() awsec2.IVpc` | Get VPC |
| `SecurityGroups() []awsec2.ISecurityGroup` | Get security groups |
| `KmsKey() awskms.IKey` | Get KMS key |

#### Example

```go
secure := constructs.NewSecureFunction(stack, jsii.String("Secure"), &constructs.SecureFunctionProps{
    LiftFunctionProps: constructs.LiftFunctionProps{
        CodeAssetPath: jsii.String("./dist/bootstrap"),
        Environment: &map[string]*string{
            "DATABASE_URL": jsii.String("{{resolve:secretsmanager:db-secret:SecretString:url}}"),
        },
    },
    CreateVpc: jsii.Bool(true),
    UsePrivateSubnets: jsii.Bool(true),
    SecretArns: &[]*string{
        jsii.String("arn:aws:secretsmanager:us-east-1:123456789012:secret:db-secret"),
    },
    EncryptEnvironmentVariables: jsii.Bool(true),
})
```

### MonitoredFunction

Lambda function with comprehensive monitoring.

```go
monitored := constructs.NewMonitoredFunction(stack, id, props)
```

#### Properties

Extends [LiftFunctionProps](#liftfunction) with:

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `CreateDashboard` | `*bool` | `true` | Create CloudWatch dashboard |
| `DashboardName` | `*string` | Auto-generated | Dashboard name |
| `EnableAlarms` | `*bool` | `true` | Enable CloudWatch alarms |
| `AlarmEmail` | `*string` | `nil` | SNS topic email for alarms |
| `ErrorRateThreshold` | `*float64` | `0.01` | Error rate threshold (1%) |
| `LatencyThreshold` | `*float64` | `1000` | Latency threshold (ms) |
| `ThrottleThreshold` | `*float64` | `10` | Throttle threshold |
| `ConcurrentExecutionsThreshold` | `*float64` | `900` | Concurrent executions |
| `LambdaInsights` | `*bool` | `true` | Enable Lambda Insights |
| `EnableLogInsightsQueries` | `*bool` | `false` | Add Log Insights queries |

#### Methods

Inherits all [LiftFunction](#liftfunction) methods plus:

| Method | Description |
|--------|-------------|
| `Dashboard() awscloudwatch.Dashboard` | Get dashboard |
| `AlarmTopic() awssns.Topic` | Get alarm SNS topic |
| `ErrorAlarm() awscloudwatch.Alarm` | Get error rate alarm |
| `LatencyAlarm() awscloudwatch.Alarm` | Get latency alarm |
| `AddMetric(name, unit, value)` | Add custom metric |
| `AddAlarm(name, metric, threshold)` | Add custom alarm |
| `AddLogInsightsQuery(name, query)` | Add Log Insights query |
| `AddCommonLogInsightsQueries()` | Add pre-built queries |

#### Example

```go
monitored := constructs.NewMonitoredFunction(stack, jsii.String("API"), &constructs.MonitoredFunctionProps{
    LiftFunctionProps: constructs.LiftFunctionProps{
        CodeAssetPath: jsii.String("./dist/bootstrap"),
    },
    EnableAlarms: jsii.Bool(true),
    AlarmEmail: jsii.String("ops@example.com"),
    ErrorRateThreshold: jsii.Number(0.05),  // 5% error rate
    LatencyThreshold: jsii.Number(2000),    // 2 seconds
    EnableLogInsightsQueries: jsii.Bool(true),
})

// Add custom metric
monitored.AddMetric("PaymentProcessed", awscloudwatch.Unit_COUNT, jsii.Number(1))

```

## Enhanced Constructs

### EnhancedMonitoring

**File**: `pkg/cdk/constructs/monitoring_enhanced.go`  
**Type**: Comprehensive Monitoring Construct  
**Lines**: 1-655

Comprehensive monitoring construct with real CloudWatch metrics, alarms, dashboards, and log insights.

#### Constructor

```go
func NewEnhancedMonitoring(scope constructs.Construct, id *string, props *EnhancedMonitoringProps) *EnhancedMonitoring
```

#### Properties (EnhancedMonitoringProps)

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `Resource` | `MonitorableResource` | Required | Resource to monitor |
| `Namespace` | `*string` | Auto-generated | Custom namespace for metrics |
| `AlertTopic` | `awssns.ITopic` | `nil` | SNS topic for alerts |
| `DashboardName` | `*string` | Auto-generated | Dashboard name |
| `MetricConfig` | `*MetricConfiguration` | `nil` | Advanced metric configuration |
| `AlarmThresholds` | `*AlarmThresholds` | `nil` | Alarm threshold configuration |
| `EnableRealTimeStreaming` | `*bool` | `false` | Enable real-time streaming |
| `Environment` | `*string` | `nil` | Environment tag |

#### Methods

| Method | Return Type | Description |
|--------|-------------|-------------|
| `AddCustomMetric(name, namespace, dimensions)` | `awscloudwatch.IMetric` | Add custom metric |
| `AddAlarm(name, metric, threshold)` | `awscloudwatch.IAlarm` | Add custom alarm |
| `AddLogInsightsQuery(name, query)` | `awslogs.MetricFilter` | Add Log Insights query |
| `GetDashboard()` | `awscloudwatch.Dashboard` | Get CloudWatch dashboard |
| `GetMetrics()` | `map[string]awscloudwatch.IMetric` | Get all metrics |
| `GetAlarms()` | `map[string]awscloudwatch.IAlarm` | Get all alarms |

#### Example

```go
monitoring := constructs.NewEnhancedMonitoring(this, jsii.String("Monitoring"), &constructs.EnhancedMonitoringProps{
    Resource: myFunction,
    Namespace: jsii.String("MyApp/Metrics"),
    AlertTopic: alertTopic,
    DashboardName: jsii.String("MyApp-Dashboard"),
    MetricConfig: &constructs.MetricConfiguration{
        DetailedMetrics: jsii.Bool(true),
        EnableBusinessMetrics: jsii.Bool(true),
        Percentiles: &[]*float64{jsii.Number(50), jsii.Number(95), jsii.Number(99)},
    },
    AlarmThresholds: &constructs.AlarmThresholds{
        ErrorRate: jsii.Number(0.05),
        LatencyP99: jsii.Number(2000),
        ThrottleCount: jsii.Number(10),
    },
    EnableRealTimeStreaming: jsii.Bool(true),
    Environment: jsii.String("production"),
})

// Add custom metric
monitoring.AddCustomMetric(
    jsii.String("CustomMetric"),
    jsii.String("MyApp/Custom"),
    &map[string]*string{
        "Service": jsii.String("API"),
    },
)
```

### EnhancedSecurity

**File**: `pkg/cdk/constructs/security_enhanced.go`  
**Type**: Comprehensive Security Construct  
**Lines**: 1-757

Comprehensive security construct with WAF, VPC security groups, secrets management, and security monitoring.

#### Constructor

```go
func NewEnhancedSecurity(scope constructs.Construct, id *string, props *EnhancedSecurityProps) *EnhancedSecurity
```

#### Properties (EnhancedSecurityProps)

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `Vpc` | `awsec2.IVpc` | Required | VPC for security resources |
| `EnableWAF` | `*bool` | `true` | Enable AWS WAF |
| `WAFConfig` | `*WAFRuleConfig` | `nil` | WAF rule configuration |
| `EnableVPCFlowLogs` | `*bool` | `true` | Enable VPC Flow Logs |
| `EnableGuardDuty` | `*bool` | `false` | Enable GuardDuty |
| `EnableSecurityHub` | `*bool` | `false` | Enable Security Hub |
| `EnableConfigRules` | `*bool` | `false` | Enable Config rules |
| `Environment` | `*string` | `nil` | Environment tag |
| `ApplicationName` | `*string` | `nil` | Application name |
| `IngressRules` | `[]SecurityRule` | `nil` | Ingress security rules |
| `EgressRules` | `[]SecurityRule` | `nil` | Egress security rules |
| `Secrets` | `[]SecretConfig` | `nil` | Secrets configuration |
| `VPCEndpointConfig` | `*VPCEndpointConfig` | `nil` | VPC endpoint configuration |

#### Methods

| Method | Return Type | Description |
|--------|-------------|-------------|
| `GetSecurityGroup()` | `awsec2.SecurityGroup` | Get security group |
| `GetWAF()` | `awswafv2.CfnWebACL` | Get WAF Web ACL |
| `GetSecrets()` | `map[string]awssecretsmanager.Secret` | Get secrets |
| `GetVPCEndpoints()` | `map[string]awsec2.InterfaceVpcEndpoint` | Get VPC endpoints |
| `AddSecurityRule(rule)` | `void` | Add security rule |
| `CreateSecret(config)` | `awssecretsmanager.Secret` | Create secret |

#### Example

```go
security := constructs.NewEnhancedSecurity(this, jsii.String("Security"), &constructs.EnhancedSecurityProps{
    Vpc: myVpc,
    EnableWAF: jsii.Bool(true),
    WAFConfig: &constructs.WAFRuleConfig{
        EnableRateLimit: jsii.Bool(true),
        RateLimit: jsii.Number(2000),
        EnableSQLiProtection: jsii.Bool(true),
        EnableXSSProtection: jsii.Bool(true),
        IPWhitelist: &[]*string{
            jsii.String("203.0.113.0/24"),
        },
    },
    EnableVPCFlowLogs: jsii.Bool(true),
    Environment: jsii.String("production"),
    ApplicationName: jsii.String("MyApp"),
    Secrets: []constructs.SecretConfig{
        {
            Name: "database-password",
            Description: "Database password",
            Length: 32,
            EnableRotation: true,
        },
    },
    VPCEndpointConfig: &constructs.VPCEndpointConfig{
        EnableSecretsManager: jsii.Bool(true),
        EnableCloudWatchLogs: jsii.Bool(true),
        EnableKMS: jsii.Bool(true),
    },
})
```

## Patterns

### BasicAPI

Complete API with Lambda backend.

```go
api := patterns.NewBasicAPI(stack, id, props)
```

#### Properties

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `ApiName` | `*string` | Required | API name |
| `CodeAssetPath` | `*string` | Required | Lambda code path |
| `EnableCors` | `*bool` | `true` | Enable CORS |
| `EnableLogging` | `*bool` | `true` | Enable CloudWatch logs |
| `MemorySize` | `*float64` | `512` | Lambda memory |
| `Timeout` | `*float64` | `30` | Lambda timeout |
| `Environment` | `*map[string]*string` | `nil` | Environment variables |
| `CustomDomainName` | `*string` | `nil` | Custom domain |
| `CertificateArn` | `*string` | `nil` | ACM certificate |

#### Methods

| Method | Description |
|--------|-------------|
| `Function() *constructs.LiftFunction` | Get Lambda function |
| `Api() *constructs.LiftAPI` | Get API Gateway |

### SecureAPI

API with security features.

```go
secure := patterns.NewSecureAPI(stack, id, props)
```

#### Properties

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `ApiName` | `*string` | Required | API name |
| `CodeAssetPath` | `*string` | Required | Lambda code path |
| `EnableWAF` | `*bool` | `true` | Enable AWS WAF |
| `EnableRateLimiting` | `*bool` | `true` | Enable rate limiting |
| `RateLimitPerHour` | `*float64` | `10000` | Rate limit |
| `EnableVPC` | `*bool` | `false` | Use VPC |
| `UsePrivateSubnets` | `*bool` | `false` | Private subnets only |
| `EnableMonitoring` | `*bool` | `true` | Enable monitoring |
| `AlarmEmail` | `*string` | `nil` | Alarm notifications |

### LiftApp

Complete Lift application pattern.

```go
app := patterns.NewLiftApp(stack, id, props)
```

#### Properties

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `AppName` | `*string` | Required | Application name |
| `CodeAssetPath` | `*string` | Required | Lambda code path |
| `EnableDatabase` | `*bool` | `false` | Add DynamoDB table |
| `EnableRateLimiting` | `*bool` | `false` | Enable rate limiting |
| `EnableIdempotency` | `*bool` | `false` | Enable idempotency |
| `EnableMultiTenant` | `*bool` | `false` | Multi-tenant support |
| `EnableCache` | `*bool` | `false` | Add ElastiCache |
| `EnableQueue` | `*bool` | `false` | Add SQS queue |
| `EnableNotifications` | `*bool` | `false` | Add SNS topic |

### MicroserviceComplete

**File**: `pkg/cdk/patterns/microservice_complete.go`  
**Type**: Complete ECS-based Microservice Pattern  
**Lines**: 1-903

Complete ECS-based microservice pattern with load balancer, auto-scaling, service discovery, and comprehensive monitoring.

#### Constructor

```go
func NewMicroserviceComplete(scope constructs.Construct, id *string, props *MicroserviceCompleteProps) *MicroserviceComplete
```

#### Properties (MicroserviceCompleteProps)

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `ServiceName` | `*string` | Required | Service name |
| `ContainerConfig` | `*ContainerConfig` | Required | Container configuration |
| `NetworkConfig` | `*NetworkConfig` | Required | Network configuration |
| `LoadBalancerConfig` | `*LoadBalancerConfig` | `nil` | Load balancer configuration |
| `AutoScalingConfig` | `*AutoScalingConfig` | `nil` | Auto-scaling configuration |
| `ServiceDiscoveryConfig` | `*ServiceDiscoveryConfig` | `nil` | Service discovery configuration |
| `HealthCheckConfig` | `*HealthCheckConfig` | `nil` | Health check configuration |
| `Environment` | `*string` | `nil` | Environment tag |
| `EnableLogging` | `*bool` | `true` | Enable CloudWatch logging |
| `EnableMonitoring` | `*bool` | `true` | Enable monitoring |
| `EnableSecurity` | `*bool` | `true` | Enable security features |

#### Methods

| Method | Return Type | Description |
|--------|-------------|-------------|
| `GetCluster()` | `awsecs.Cluster` | Get ECS cluster |
| `GetService()` | `awsecs.FargateService` | Get Fargate service |
| `GetTaskDefinition()` | `awsecs.FargateTaskDefinition` | Get task definition |
| `GetLoadBalancer()` | `awselasticloadbalancingv2.ApplicationLoadBalancer` | Get load balancer |
| `GetTargetGroup()` | `awselasticloadbalancingv2.ApplicationTargetGroup` | Get target group |
| `GetAutoScalingGroup()` | `awsapplicationautoscaling.ScalableTarget` | Get auto-scaling group |
| `GetServiceDiscovery()` | `awsservicediscovery.Service` | Get service discovery |
| `AddEnvironmentVariable(key, value)` | `void` | Add environment variable |
| `AddSecret(name, secret)` | `void` | Add secret |
| `AddVolume(name, volume)` | `void` | Add volume |

#### Example

```go
microservice := patterns.NewMicroserviceComplete(this, jsii.String("Microservice"), &patterns.MicroserviceCompleteProps{
    ServiceName: jsii.String("my-service"),
    ContainerConfig: &patterns.ContainerConfig{
        Platform: awsecs.CpuArchitecture_ARM64,
        CodeAssetPath: jsii.String("./dist"),
        MemoryLimitMiB: jsii.Number(512),
        CpuLimitMiB: jsii.Number(256),
        Environment: &map[string]*string{
            "NODE_ENV": jsii.String("production"),
        },
    },
    NetworkConfig: &patterns.NetworkConfig{
        VPC: myVpc,
        AssignPublicIP: jsii.Bool(false),
        EnableVPCLogs: jsii.Bool(true),
        EnableContainerInsights: jsii.Bool(true),
    },
    LoadBalancerConfig: &patterns.LoadBalancerConfig{
        Enabled: jsii.Bool(true),
        DomainName: jsii.String("api.example.com"),
        Certificate: certificate,
        HealthCheckPath: jsii.String("/health"),
        HealthCheckInterval: awscdk.Duration_Seconds(jsii.Number(30)),
        HealthCheckTimeout: awscdk.Duration_Seconds(jsii.Number(5)),
        HealthyThresholdCount: jsii.Number(2),
        UnhealthyThresholdCount: jsii.Number(3),
    },
    AutoScalingConfig: &patterns.AutoScalingConfig{
        MinCapacity: jsii.Number(1),
        MaxCapacity: jsii.Number(10),
        TargetCPUUtilization: jsii.Number(70),
        TargetMemoryUtilization: jsii.Number(80),
        ScaleInCooldown: awscdk.Duration_Minutes(jsii.Number(5)),
        ScaleOutCooldown: awscdk.Duration_Minutes(jsii.Number(3)),
    },
    ServiceDiscoveryConfig: &patterns.ServiceDiscoveryConfig{
        Namespace: jsii.String("myapp.local"),
        ServiceName: jsii.String("api"),
        HealthCheckPath: jsii.String("/health"),
        HealthCheckInterval: awscdk.Duration_Seconds(jsii.Number(30)),
        TTL: awscdk.Duration_Seconds(jsii.Number(60)),
    },
    Environment: jsii.String("production"),
    EnableLogging: jsii.Bool(true),
    EnableMonitoring: jsii.Bool(true),
    EnableSecurity: jsii.Bool(true),
})
```

## Stacks

### MicroserviceStack

Stack for individual microservices.

```go
stack := stacks.NewMicroserviceStack(app, id, props)
```

#### Properties

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `ServiceName` | `string` | Required | Service name |
| `CodePath` | `string` | Required | Lambda code path |
| `EnableDatabase` | `bool` | `false` | Add DynamoDB |
| `EnableCache` | `bool` | `false` | Add ElastiCache |
| `EnableQueue` | `bool` | `false` | Add SQS queue |
| `MemorySize` | `int` | `512` | Lambda memory |
| `Environment` | `map[string]string` | `nil` | Environment vars |

### MultiTenantSaaSStack

Complete SaaS application stack.

```go
stack := stacks.NewMultiTenantSaaSStack(app, id, props)
```

#### Properties

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `AppName` | `string` | Required | Application name |
| `CodePath` | `string` | Required | Lambda code path |
| `EnableAuth` | `bool` | `true` | Add Cognito |
| `EnableFileStorage` | `bool` | `false` | Add S3 bucket |
| `CustomDomainName` | `string` | `nil` | Custom domain |
| `CertificateArn` | `string` | `nil` | ACM certificate |
| `EnableAnalytics` | `bool` | `false` | Add analytics |

### EventDrivenStack

Event-driven architecture stack.

```go
stack := stacks.NewEventDrivenStack(app, id, props)
```

#### Properties

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `AppName` | `string` | Required | Application name |
| `ApiCodePath` | `string` | Required | API Lambda path |
| `EventProcessorCodePath` | `string` | Required | Processor path |
| `EnableDLQ` | `bool` | `true` | Add dead letter queue |
| `EventBusName` | `string` | Auto-generated | EventBridge bus |
| `EventRules` | `[]EventRule` | `nil` | Event routing rules |