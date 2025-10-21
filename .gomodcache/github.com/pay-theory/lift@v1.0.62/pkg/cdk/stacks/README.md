# Lift CDK Stack Templates

Pre-built CDK stack templates for common Lift application patterns.

## Available Stacks

### 1. Microservice Stack
Single microservice with API Gateway, Lambda, and optional DynamoDB.

```go
stack := stacks.NewMicroserviceStack(app, "MyService", &stacks.MicroserviceStackProps{
    ServiceName:    "user-service",
    CodePath:       "./dist/bootstrap",
    EnableDatabase: true,
    MemorySize:     1024,
    Environment: map[string]string{
        "LOG_LEVEL": "info",
    },
})
```

### 2. Multi-Tenant SaaS Stack  
Complete SaaS application with authentication, file storage, and multi-tenancy.

```go
stack := stacks.NewMultiTenantSaaSStack(app, "MySaaS", &stacks.MultiTenantSaaSStackProps{
    AppName:           "my-saas",
    CodePath:          "./dist/bootstrap",
    DomainName:        "api.example.com",
    CertificateArn:    "arn:aws:acm:...",
    EnableAuth:        true,
    EnableFileStorage: true,
})
```

Features:
- Cognito user authentication
- S3 file storage with versioning
- Multi-tenant DynamoDB
- Custom domain support
- Rate limiting

### 3. Event-Driven Stack
Event-driven architecture with EventBridge, SQS, and event sourcing.

```go
stack := stacks.NewEventDrivenStack(app, "MyEventApp", &stacks.EventDrivenStackProps{
    AppName:                "order-system",
    ApiCodePath:            "./dist/api/bootstrap",
    EventProcessorCodePath: "./dist/processor/bootstrap",
    EnableDLQ:              true,
    EventBusName:           "orders",
})
```

Features:
- EventBridge for event routing
- SQS for reliable processing
- Dead letter queue support
- Event store with DynamoDB
- Separate API and processor functions

## Usage

1. Import the stacks package:
```go
import "github.com/pay-theory/lift/pkg/cdk/stacks"
```

2. Create your CDK app:
```go
app := awscdk.NewApp(nil)
```

3. Use a pre-built stack:
```go
stacks.NewMicroserviceStack(app, "MyStack", &stacks.MicroserviceStackProps{
    // ... configuration
})
```

4. Deploy:
```bash
cdk deploy
```

## Customization

All stacks are designed to be extended. You can:
- Override default values
- Add additional resources
- Modify IAM permissions
- Integrate with existing infrastructure

## Best Practices

1. **Use environment-specific props**: Pass different configurations for dev/staging/prod
2. **Enable monitoring**: All stacks include CloudWatch integration
3. **Set appropriate limits**: Configure memory, timeout, and concurrency
4. **Use custom domains**: For production deployments
5. **Enable backups**: DynamoDB point-in-time recovery is enabled by default