# Lift CDK Constructs Library Plan

## Overview

A collection of AWS CDK constructs designed specifically for deploying Lift-based Lambda functions with security, compliance, and operational best practices built-in. These constructs help Pay Theory developers create consistent, reliable serverless applications.

## Core Objectives

1. **Simplified Deployment**: One-line CDK constructs for Lift Lambda functions
2. **Middleware Integration**: Built-in support for Lift middleware patterns
3. **Event Source Adapters**: Pre-configured constructs for all Lift adapters
4. **Security by Default**: Implement Pay Theory security standards automatically
5. **Operational Excellence**: Monitoring, logging, and observability built-in

## Construct Categories

### 1. Core Lambda Constructs

#### LiftFunction
Base construct for deploying Lift-based Lambda functions with:
- Go 1.21+ runtime configuration
- Optimized memory and timeout settings
- Environment variables for Lift configuration
- IAM roles with least privilege
- X-Ray tracing enabled
- CloudWatch Logs with retention policies
- Dead letter queue configuration

```typescript
const api = new LiftFunction(this, 'APIHandler', {
  handler: 'bootstrap',
  code: lambda.Code.fromAsset('./dist'),
  memorySize: 512,
  environment: {
    LIFT_LOG_LEVEL: 'info',
    LIFT_ENV: 'production'
  }
});
```

### 2. API Gateway Integration

#### LiftAPIGateway
REST API Gateway v2 optimized for Lift:
- Automatic route configuration from Lift app
- Built-in request validation
- CORS configuration
- API key management
- Custom domain support
- WAF integration options

```typescript
const api = new LiftAPIGateway(this, 'API', {
  liftFunction: apiFunction,
  domainName: 'api.example.com',
  corsOrigins: ['https://app.example.com'],
  wafEnabled: true
});
```

#### LiftWebSocketAPI
WebSocket API Gateway for Lift WebSocket handlers:
- Connection management table
- Route selection
- Authorizer integration

### 3. Middleware-Enabled Constructs

#### RateLimitedLiftFunction
Lift function with rate limiting middleware pre-configured:
- DynamoDB table for rate limit tracking
- Configurable windows and limits
- Support for IP, User, and Tenant-based limiting
- Integration with Limited library

```typescript
const rateLimitedAPI = new RateLimitedLiftFunction(this, 'RateLimitedAPI', {
  handler: 'bootstrap',
  code: lambda.Code.fromAsset('./dist'),
  rateLimiting: {
    type: 'IP', // or 'User', 'Tenant'
    limit: 1000,
    window: Duration.hours(1),
    tableName: 'api-rate-limits'
  }
});
```

#### IdempotentLiftFunction
Ensures idempotent processing with DynamoDB backing:
- Automatic idempotency key extraction
- Configurable TTL for idempotency records
- Response caching

```typescript
const idempotentProcessor = new IdempotentLiftFunction(this, 'Processor', {
  handler: 'bootstrap',
  code: lambda.Code.fromAsset('./dist'),
  idempotency: {
    keyPath: '$.requestId',
    ttl: Duration.days(7),
    tableName: 'idempotency-keys'
  }
});
```

### 4. Event Source Adapters

#### LiftSQSProcessor
SQS queue with Lift Lambda processor:
- Dead letter queue configuration
- Batch size optimization
- Error handling and retries
- Message visibility timeout

```typescript
const processor = new LiftSQSProcessor(this, 'QueueProcessor', {
  liftFunction: processorFunction,
  batchSize: 10,
  visibilityTimeout: Duration.minutes(5),
  deadLetterQueue: {
    maxReceiveCount: 3
  }
});
```

#### LiftEventBridgeHandler
EventBridge rule with Lift handler:
- Event pattern configuration
- Target retry policies
- Dead letter configuration

```typescript
const eventHandler = new LiftEventBridgeHandler(this, 'EventHandler', {
  liftFunction: handlerFunction,
  eventPattern: {
    source: ['payment.system'],
    detailType: ['Payment Processed']
  },
  retryAttempts: 2
});
```

#### LiftS3Processor
S3 bucket notifications to Lift function:
- Event type filtering
- Prefix/suffix filtering
- Error handling

#### LiftDynamoStreamProcessor
DynamoDB stream processing with Lift:
- Stream configuration
- Batch processing
- Error handling and bisecting

### 5. Security & Compliance Constructs

#### SecureLiftFunction
Enhanced security for sensitive operations:
- VPC deployment
- Secrets Manager integration
- KMS encryption for environment variables
- Security group configuration

```typescript
const secureApi = new SecureLiftFunction(this, 'SecureAPI', {
  handler: 'bootstrap',
  code: lambda.Code.fromAsset('./dist'),
  vpc: vpc,
  securityGroups: [apiSecurityGroup],
  secrets: {
    'DB_PASSWORD': secretsManager.Secret.fromSecretNameV2(this, 'DBPassword', 'db-password')
  }
});
```

#### ComplianceLiftStack
Pre-configured stack with compliance requirements:
- CloudTrail logging
- Config rules
- Log retention policies
- Encryption everywhere

### 6. Observability Constructs

#### MonitoredLiftFunction
Enhanced monitoring and alerting:
- Custom CloudWatch dashboards
- Alarm configuration
- Distributed tracing
- Log insights queries

```typescript
const monitoredApi = new MonitoredLiftFunction(this, 'MonitoredAPI', {
  handler: 'bootstrap',
  code: lambda.Code.fromAsset('./dist'),
  monitoring: {
    alarms: {
      errorRate: 0.01, // 1%
      latencyP99: Duration.seconds(1),
      throttleRate: 0.05 // 5%
    },
    dashboard: true,
    tracing: lambda.Tracing.ACTIVE
  }
});
```

### 7. Multi-Tenant Constructs

#### MultiTenantLiftAPI
API Gateway with tenant isolation:
- Tenant ID extraction from JWT/headers
- Per-tenant rate limiting
- Tenant-specific metrics

```typescript
const multiTenantApi = new MultiTenantLiftAPI(this, 'MultiTenantAPI', {
  liftFunction: apiFunction,
  tenantIdSource: 'jwt', // or 'header', 'path'
  tenantIdPath: '$.custom.tenantId',
  perTenantRateLimit: {
    limit: 10000,
    window: Duration.hours(1)
  }
});
```

## Integration Examples

### Complete API Service
```typescript
export class PaymentAPIStack extends Stack {
  constructor(scope: Construct, id: string, props?: StackProps) {
    super(scope, id, props);

    // Create rate-limited, idempotent API function
    const apiFunction = new RateLimitedLiftFunction(this, 'PaymentAPI', {
      handler: 'bootstrap',
      code: lambda.Code.fromAsset('./dist/api'),
      memorySize: 1024,
      timeout: Duration.seconds(30),
      rateLimiting: {
        type: 'Tenant',
        limit: 10000,
        window: Duration.hours(1)
      }
    });

    // Add idempotency
    apiFunction.addMiddleware(new IdempotencyMiddleware({
      tableName: 'payment-idempotency'
    }));

    // Create API Gateway
    const api = new LiftAPIGateway(this, 'PaymentAPIGateway', {
      liftFunction: apiFunction,
      domainName: 'api.payments.example.com',
      wafEnabled: true,
      apiKeyRequired: true
    });

    // Add monitoring
    new MonitoringConstruct(this, 'APIMonitoring', {
      function: apiFunction,
      api: api,
      alarms: {
        errorRate: 0.01,
        latencyP99: Duration.seconds(2)
      }
    });
  }
}
```

### Event Processing Pipeline
```typescript
export class PaymentProcessingStack extends Stack {
  constructor(scope: Construct, id: string, props?: StackProps) {
    super(scope, id, props);

    // SQS processor with idempotency
    const processor = new IdempotentLiftFunction(this, 'PaymentProcessor', {
      handler: 'bootstrap',
      code: lambda.Code.fromAsset('./dist/processor'),
      idempotency: {
        keyPath: '$.paymentId',
        ttl: Duration.days(30)
      }
    });

    // Create SQS queue with processor
    const queue = new LiftSQSProcessor(this, 'PaymentQueue', {
      liftFunction: processor,
      batchSize: 25,
      deadLetterQueue: {
        maxReceiveCount: 3
      }
    });

    // EventBridge for payment events
    new LiftEventBridgeHandler(this, 'PaymentEvents', {
      liftFunction: processor,
      eventPattern: {
        source: ['payment.service'],
        detailType: ['Payment Initiated', 'Payment Completed']
      }
    });
  }
}
```

## Development Patterns

### Construct Principles
1. **Convention over Configuration**: Sensible defaults for Pay Theory use cases
2. **Composability**: Constructs can be combined for complex scenarios
3. **Type Safety**: Full TypeScript support with proper types
4. **Testability**: Built-in support for CDK testing

### Testing Strategy
```typescript
// Unit tests for constructs
test('RateLimitedLiftFunction creates DynamoDB table', () => {
  const stack = new Stack();
  new RateLimitedLiftFunction(stack, 'Function', {
    handler: 'bootstrap',
    code: lambda.Code.fromAsset('./dist'),
    rateLimiting: {
      type: 'IP',
      limit: 1000,
      window: Duration.hours(1)
    }
  });
  
  Template.fromStack(stack).hasResourceProperties('AWS::DynamoDB::Table', {
    BillingMode: 'PAY_PER_REQUEST'
  });
});
```

## Roadmap

### Phase 1: Core Constructs (Month 1)
- LiftFunction base construct
- LiftAPIGateway
- Basic middleware constructs

### Phase 2: Event Sources (Month 2)
- All Lift adapter constructs
- Event source configurations
- Error handling patterns

### Phase 3: Advanced Features (Month 3)
- Multi-tenant constructs
- Advanced monitoring
- Compliance templates

## Usage Guide

### Installation
```bash
npm install @pay-theory/lift-cdk
```

### Basic Usage
```typescript
import { LiftFunction, LiftAPIGateway } from '@pay-theory/lift-cdk';

const apiFunction = new LiftFunction(this, 'API', {
  handler: 'bootstrap',
  code: lambda.Code.fromAsset('./dist')
});

const api = new LiftAPIGateway(this, 'Gateway', {
  liftFunction: apiFunction
});
```

### CLI Integration
```bash
# Generate CDK app from Lift project
lift-cdk init

# Deploy with optimizations
lift-cdk deploy --optimize
```