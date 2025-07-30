# Lift CDK Implementation Plan for Lesser

This document provides a comprehensive task list for implementing Lesser's infrastructure using Lift CDK, based on the Lift framework documentation and Lesser's serverless architecture requirements.

## Overview

Lesser consists of 23 Lambda functions, DynamoDB tables with streams, S3 buckets, and API Gateway endpoints. This plan leverages Lift CDK's production-ready constructs while maintaining a VPC-less, cost-optimized architecture.

## Phase 6: Lift CDK Implementation Tasks

### Phase 6.1: Setup CDK Project Structure

#### Tasks:
1. **Initialize CDK Project**
   ```bash
   mkdir infra/cdk && cd infra/cdk
   cdk init app --language go
   ```

2. **Add Lift CDK Dependencies**
   - Add to `go.mod`:
   ```go
   require (
       github.com/aws/aws-cdk-go/awscdk/v2 v2.100.0
       github.com/aws/constructs-go/constructs/v10 v10.3.0
       github.com/aws/jsii-runtime-go v1.90.0
       github.com/pay-theory/lift/pkg/cdk v0.1.0
   )
   ```

3. **Create Directory Structure**
   ```
   infra/cdk/
   ├── main.go                 # CDK app entry point
   ├── stacks/
   │   ├── lesser_stack.go     # Main application stack
   │   ├── shared_stack.go     # Shared resources (DynamoDB, S3)
   │   └── monitoring_stack.go # Monitoring and observability
   ├── constructs/
   │   ├── lambda_functions.go # Lambda function definitions
   │   ├── api_routes.go       # API Gateway route configurations
   │   └── stream_processors.go # DynamoDB stream processors
   └── config/
       ├── dev.yaml            # Development environment config
       ├── staging.yaml        # Staging environment config
       └── prod.yaml           # Production environment config
   ```

4. **Create Environment Configuration**
   - Based on Lift CDK deployment config pattern (`cdk-deployment-config.yaml`):
   ```yaml
   environment: production
   appName: lesser
   domain: example.com
   memorySize: 1024
   timeout: 30
   logLevel: INFO
   features:
     enableMultiTenant: false
     enableRateLimiting: true
     enableMonitoring: true
   ```

### Phase 6.2: Create Core Lambda Functions

#### Lambda Function Groups:

1. **API Functions** (Using `LiftFunction` construct - `pkg/cdk/constructs/lambda.go:44-169`)
   ```go
   // Based on lambda.go:87-99
   apiLambda := constructs.NewLiftFunction(stack, jsii.String("API"), &constructs.LiftFunctionProps{
       FunctionProps: awslambda.FunctionProps{
           Code:         awslambda.Code_FromAsset(jsii.String("../../bin/api.zip")),
           Handler:      jsii.String("bootstrap"),
           Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
           Architecture: awslambda.Architecture_ARM_64(),
           MemorySize:   jsii.Number(3008),
           Timeout:      awscdk.Duration_Seconds(jsii.Number(60)),
       },
       EnableTracing:     jsii.Bool(true),
       EnableMultiTenant: jsii.Bool(false),
       EnableDynamORM:    jsii.Bool(true),
   })
   ```
   - Functions: api, graphql, auth, auth-api

2. **Federation Functions**
   - inbox, outbox, webfinger
   - Special requirements: Signature verification, HTTP client for remote delivery

3. **Stream Processors** (Using `DynamoStreamProcessor` - `dynamo-streams-patterns.md`)
   ```go
   // Based on dynamo-streams-patterns.md:152-169
   activityProcessor := constructs.NewDynamoStreamProcessor(stack, jsii.String("ActivityProcessor"), &constructs.DynamoStreamProcessorProps{
       FunctionProps: awslambda.FunctionProps{
           FunctionName: jsii.String("activity-stream-processor"),
           Code:         awslambda.Code_FromAsset(jsii.String("../../bin/activity-processor.zip")),
           Handler:      jsii.String("bootstrap"),
           Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
           MemorySize:   jsii.Number(1024),
           Timeout:      awscdk.Duration_Minutes(jsii.Number(5)),
       },
       ExistingTable: mainTable.Table,
       BatchSize: jsii.Number(25),
       ParallelizationFactor: jsii.Number(5),
       StreamViewType: awsdynamodb.StreamViewType_NEW_AND_OLD_IMAGES,
   })
   ```

4. **WebSocket Functions**
   - streaming, stream-router
   - Requires WebSocket API Gateway configuration

### Phase 6.3: Setup API Gateway

#### HTTP API Configuration (Using `LiftAPI` - `pkg/cdk/constructs/api.go:56-209`)

```go
// Based on api.go:94-119 for CORS configuration
api := constructs.NewLiftAPI(stack, jsii.String("LesserAPI"), &constructs.LiftAPIProps{
    Name:                jsii.String("lesser-api"),
    EnableCORS:          jsii.Bool(true),
    EnableAccessLogging: jsii.Bool(true),
    ThrottleRateLimit:   jsii.Number(10000),
    ThrottleBurstLimit:  jsii.Number(5000),
    CorsConfiguration: &awsapigatewayv2.CorsPreflightOptions{
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
            jsii.String("X-Request-ID"),
            jsii.String("Accept"),
            jsii.String("Digest"),
            jsii.String("Signature"),
        },
    },
})
```

#### Route Configuration
1. **ActivityPub Routes**
   - `/.well-known/webfinger` → webfingerLambda
   - `/users/{username}` → actorLambda
   - `/users/{username}/inbox` → inboxLambda
   - `/users/{username}/outbox` → outboxLambda

2. **Mastodon API Routes**
   - `/api/v1/*` → apiLambda
   - `/api/v2/*` → apiLambda
   - `/api/graphql` → graphqlLambda

3. **OAuth Routes**
   - `/oauth/*` → authLambda
   - `/auth/*` → authApiLambda

### Phase 6.4: Configure DynamoDB Tables with Streams

#### Main Table Configuration (Using `DynamORMTable` - `pkg/cdk/constructs/dynamorm_table.go:78-167`)

```go
// Based on dynamorm_table.go:105-167
mainTable := constructs.NewDynamORMTable(stack, jsii.String("LesserTable"), &constructs.DynamORMTableProps{
    PartitionKey: &awsdynamodb.Attribute{
        Name: jsii.String("PK"),
        Type: awsdynamodb.AttributeType_STRING,
    },
    SortKey: &awsdynamodb.Attribute{
        Name: jsii.String("SK"),
        Type: awsdynamodb.AttributeType_STRING,
    },
    BillingMode:           awsdynamodb.BillingMode_PAY_PER_REQUEST,
    EnableStreams:         jsii.Bool(true),
    StreamViewType:        awsdynamodb.StreamViewType_NEW_AND_OLD_IMAGES,
    TimeToLiveAttribute:   jsii.String("TTL"),
    PointInTimeRecovery:   jsii.Bool(true),
    DeletionProtection:    jsii.Bool(isProd),
    EnableMultiTenant:     jsii.Bool(false),
    EnableAutoScaling:     jsii.Bool(false), // Using on-demand
})

// Add GSIs as per Lesser requirements
for i := 1; i <= 8; i++ {
    mainTable.AddDynamORMIndex(fmt.Sprintf("GSI%d", i), 
        &awsdynamodb.Attribute{
            Name: jsii.String(fmt.Sprintf("GSI%dPK", i)),
            Type: awsdynamodb.AttributeType_STRING,
        },
        &awsdynamodb.Attribute{
            Name: jsii.String(fmt.Sprintf("GSI%dSK", i)),
            Type: awsdynamodb.AttributeType_STRING,
        },
    )
}
```

### Phase 6.5: Setup DynamoDB Streams Processing

#### Stream Processor Patterns (Based on `dynamo-streams-patterns.md`)

1. **Activity Processing Stream**
   ```go
   // Based on dynamo-streams-patterns.md:530-553 for filtering
   activityProcessor := constructs.NewDynamoStreamProcessor(stack, jsii.String("ActivityProcessor"), &constructs.DynamoStreamProcessorProps{
       FunctionProps: awslambda.FunctionProps{
           FunctionName: jsii.String("lesser-activity-processor"),
           Code:         awslambda.Code_FromAsset(jsii.String("../../bin/activity-processor.zip")),
           Handler:      jsii.String("bootstrap"),
           Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
       },
       ExistingTable: mainTable.Table,
       BatchSize: jsii.Number(25),
       ParallelizationFactor: jsii.Number(5),
       EventSourceProps: &awslambdaeventsources.DynamoEventSourceProps{
           Filters: &[]*map[string]interface{}{
               {
                   "eventName": []string{"INSERT", "MODIFY"},
                   "dynamodb": map[string]interface{}{
                       "NewImage": map[string]interface{}{
                           "PK": map[string]interface{}{
                               "S": []string{"ACTIVITY#*", "STATUS#*"},
                           },
                       },
                   },
               },
           },
       },
   })
   ```

2. **Federation Delivery Stream**
   ```go
   federationProcessor := constructs.NewDynamoStreamProcessor(stack, jsii.String("FederationProcessor"), &constructs.DynamoStreamProcessorProps{
       FunctionProps: awslambda.FunctionProps{
           FunctionName: jsii.String("lesser-federation-processor"),
           Code:         awslambda.Code_FromAsset(jsii.String("../../bin/outbox.zip")),
           Handler:      jsii.String("bootstrap"),
           Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
           Timeout:      awscdk.Duration_Minutes(jsii.Number(5)),
       },
       ExistingTable: mainTable.Table,
       BatchSize: jsii.Number(10),
       BisectBatchOnError: jsii.Bool(true),
       RetryAttempts: jsii.Number(3),
       EventSourceProps: &awslambdaeventsources.DynamoEventSourceProps{
           Filters: &[]*map[string]interface{}{
               {
                   "eventName": []string{"INSERT"},
                   "dynamodb": map[string]interface{}{
                       "NewImage": map[string]interface{}{
                           "PK": map[string]interface{}{
                               "S": []string{"OUTBOX#*"},
                           },
                       },
                   },
               },
           },
       },
   })
   ```

3. **Notification Stream**
   ```go
   // Low latency configuration for real-time notifications
   notificationProcessor := constructs.NewDynamoStreamProcessor(stack, jsii.String("NotificationProcessor"), &constructs.DynamoStreamProcessorProps{
       FunctionProps: awslambda.FunctionProps{
           FunctionName: jsii.String("lesser-notification-processor"),
           Code:         awslambda.Code_FromAsset(jsii.String("../../bin/notification-processor.zip")),
           Handler:      jsii.String("bootstrap"),
           Runtime:      awslambda.Runtime_PROVIDED_AL2023(),
       },
       ExistingTable: mainTable.Table,
       BatchSize: jsii.Number(10),
       MaxBatchingWindow: awscdk.Duration_Seconds(jsii.Number(1)),
       EventSourceProps: &awslambdaeventsources.DynamoEventSourceProps{
           Filters: &[]*map[string]interface{}{
               {
                   "eventName": []string{"INSERT"},
                   "dynamodb": map[string]interface{}{
                       "NewImage": map[string]interface{}{
                           "PK": map[string]interface{}{
                               "S": []string{"NOTIFICATION#*"},
                           },
                       },
                   },
               },
           },
       },
   })
   ```

### Phase 6.6: Configure S3 Buckets

#### Media Storage Bucket
```go
mediaBucket := awss3.NewBucket(stack, jsii.String("MediaBucket"), &awss3.BucketProps{
    BucketName: jsii.String(fmt.Sprintf("lesser-media-%s", environment)),
    Encryption: awss3.BucketEncryption_S3_MANAGED,
    BlockPublicAccess: awss3.BlockPublicAccess_BLOCK_ALL,
    Cors: &[]awss3.CorsRule{
        {
            AllowedMethods: &[]awss3.HttpMethods{
                awss3.HttpMethods_GET,
                awss3.HttpMethods_PUT,
                awss3.HttpMethods_POST,
            },
            AllowedOrigins: &[]*string{jsii.String("*")},
            AllowedHeaders: &[]*string{jsii.String("*")},
            MaxAge: jsii.Number(3000),
        },
    },
    LifecycleRules: &[]awss3.LifecycleRule{
        {
            Id: jsii.String("delete-incomplete-uploads"),
            AbortIncompleteMultipartUploadAfter: awscdk.Duration_Days(jsii.Number(7)),
            Enabled: jsii.Bool(true),
        },
    },
})
```

#### CloudFront Distribution
```go
distribution := awscloudfront.NewDistribution(stack, jsii.String("MediaCDN"), &awscloudfront.DistributionProps{
    DefaultBehavior: &awscloudfront.BehaviorOptions{
        Origin: awscloudfrontorigins.NewS3Origin(mediaBucket, &awscloudfrontorigins.S3OriginProps{
            OriginAccessIdentity: oai,
        }),
        ViewerProtocolPolicy: awscloudfront.ViewerProtocolPolicy_REDIRECT_TO_HTTPS,
        CachePolicy: awscloudfront.CachePolicy_CACHING_OPTIMIZED,
    },
    DomainNames: &[]*string{jsii.String(fmt.Sprintf("media.%s", domain))},
    Certificate: certificate,
    PriceClass: awscloudfront.PriceClass_PRICE_CLASS_100,
})
```

### Phase 6.7: Setup Enhanced Security (Without VPC)

#### Security Configuration (Based on `pkg/cdk/constructs/security_enhanced.go`)

1. **WAF Configuration** (security_enhanced.go:243-272)
   ```go
   waf := awswafv2.NewCfnWebACL(stack, jsii.String("LesserWAF"), &awswafv2.CfnWebACLProps{
       Scope: jsii.String("REGIONAL"),
       DefaultAction: &awswafv2.CfnWebACL_DefaultActionProperty{
           Allow: &awswafv2.CfnWebACL_AllowActionProperty{},
       },
       Rules: &[]awswafv2.CfnWebACL_RuleProperty{
           {
               Name:     jsii.String("RateLimitRule"),
               Priority: jsii.Number(1),
               Statement: &awswafv2.CfnWebACL_StatementProperty{
                   RateBasedStatement: &awswafv2.CfnWebACL_RateBasedStatementProperty{
                       Limit:            jsii.Number(5000),
                       AggregateKeyType: jsii.String("IP"),
                   },
               },
               Action: &awswafv2.CfnWebACL_RuleActionProperty{
                   Block: &awswafv2.CfnWebACL_BlockActionProperty{
                       CustomResponse: &awswafv2.CfnWebACL_CustomResponseProperty{
                           ResponseCode: jsii.Number(429),
                       },
                   },
               },
           },
       },
   })
   ```

2. **IAM Policies**
   - Least privilege for each Lambda function
   - DynamoDB access limited to specific tables
   - S3 access limited to media bucket
   - No cross-service access unless required

3. **KMS Encryption**
   ```go
   kmsKey := awskms.NewKey(stack, jsii.String("LesserKey"), &awskms.KeyProps{
       Description: jsii.String("Lesser encryption key for actor private keys"),
       EnableKeyRotation: jsii.Bool(true),
   })
   ```

### Phase 6.8: Configure Monitoring & Observability

#### Enhanced Monitoring (Based on `pkg/cdk/constructs/monitoring_enhanced.go:81-108`)

```go
monitoring := constructs.NewEnhancedMonitoring(stack, jsii.String("Monitoring"), &constructs.EnhancedMonitoringProps{
    Namespace:   jsii.String("Lesser/Production"),
    Environment: jsii.String("production"),
    MetricConfig: &constructs.MetricConfiguration{
        DetailedMetrics:       jsii.Bool(true),
        EnableBusinessMetrics: jsii.Bool(true),
        Resolution:            jsii.Number(60), // 1-minute resolution
    },
    AlarmThresholds: &constructs.AlarmThresholds{
        ErrorRate:            jsii.Number(1.0),  // 1% error rate
        LatencyP99:           jsii.Number(3000), // 3 seconds
        ThrottleCount:        jsii.Number(10),
        ConcurrentExecutions: jsii.Number(100),
    },
    EnableRealTimeStreaming: jsii.Bool(false), // Cost optimization
})
```

#### Key Metrics to Monitor
1. **Lambda Metrics**
   - Duration, Errors, Throttles
   - Iterator Age for stream processors
   - Concurrent Executions

2. **DynamoDB Metrics**
   - ConsumedReadCapacityUnits/ConsumedWriteCapacityUnits
   - UserErrors, SystemErrors
   - Stream Records

3. **API Gateway Metrics**
   - 4XXError, 5XXError
   - Count, Latency
   - IntegrationLatency

### Phase 6.9: Create Environment-Specific Stacks

#### Stack Organization
```go
type LesserStackProps struct {
    awscdk.StackProps
    Environment string
    Domain      string
    Certificate awscertificatemanager.ICertificate
}

func NewLesserStack(scope constructs.Construct, id string, props *LesserStackProps) awscdk.Stack {
    stack := awscdk.NewStack(scope, &id, &props.StackProps)
    
    // Environment-specific configuration
    isProd := props.Environment == "production"
    config := loadEnvironmentConfig(props.Environment)
    
    // Create resources with environment-specific settings
    // ...
    
    return stack
}
```

#### Environment Configurations
1. **Development**
   - Lower memory/timeout settings
   - Debug logging enabled
   - No deletion protection
   - Minimal monitoring

2. **Staging**
   - Production-like settings
   - Standard monitoring
   - Some cost optimizations

3. **Production**
   - Maximum performance settings
   - Comprehensive monitoring
   - Deletion protection enabled
   - Point-in-time recovery

### Phase 6.10: Document Deployment Process

#### Deployment Commands
```bash
# Build Lambda functions
make build-lambdas

# Synthesize CDK
cd infra/cdk
cdk synth

# Deploy specific environment
cdk deploy LesserStack-Dev --context environment=development
cdk deploy LesserStack-Staging --context environment=staging
cdk deploy LesserStack-Prod --context environment=production --require-approval broadening

# Show differences
cdk diff LesserStack-Prod

# Destroy (non-prod only)
cdk destroy LesserStack-Dev
```

#### CI/CD Integration
```yaml
# GitHub Actions example
- name: Deploy to AWS
  run: |
    npm install -g aws-cdk
    cd infra/cdk
    cdk deploy --require-approval never
```

## Implementation Order

1. **Week 1**: Project setup, shared resources (DynamoDB, S3, KMS)
2. **Week 2**: Core Lambda functions and API Gateway
3. **Week 3**: Stream processors and event-driven components
4. **Week 4**: Security, monitoring, and production hardening
5. **Week 5**: Testing, documentation, and deployment automation

## Key Considerations

1. **Cost Optimization**
   - No VPC (saves ~$45/month)
   - Pay-per-request DynamoDB
   - ARM64 Lambdas (20% cheaper)
   - S3 lifecycle policies
   - CloudFront caching

2. **Performance**
   - Stream parallelization for high throughput
   - Batch processing where appropriate
   - Connection pooling in Lambda
   - Strategic caching

3. **Security**
   - IAM least privilege
   - KMS encryption at rest
   - TLS in transit
   - WAF rate limiting
   - No public S3 access

4. **Reliability**
   - Dead letter queues
   - Retry logic with backoff
   - Circuit breakers for federation
   - Health checks

## Success Metrics

- Infrastructure deployment time < 30 minutes
- Lambda cold start < 1 second
- API response time P99 < 500ms
- Stream processing lag < 5 seconds
- Monthly cost < $10 for small instance

## References

- Lift CDK Documentation: `/reference/lift/docs/cdk/`
- DynamoDB Streams Patterns: `/reference/lift/docs/cdk/dynamo-streams-patterns.md`
- CDK Getting Started: `/reference/lift/docs/cdk/CDK_GETTING_STARTED.md`
- CDK Guide: `/reference/lift/docs/cdk/CDK_GUIDE.md`