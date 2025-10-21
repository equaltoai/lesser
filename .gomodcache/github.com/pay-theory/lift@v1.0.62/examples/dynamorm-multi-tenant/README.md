# DynamORM Multi-Tenant Example

This example demonstrates a comprehensive multi-tenant SaaS application using Lift with DynamORM and enhanced CDK constructs. It showcases tenant isolation, monitoring, and production-ready patterns.

## Features

### Multi-Tenant Architecture
- **Tenant Isolation**: Complete data isolation using DynamORM patterns
- **Tenant-Scoped GSIs**: Optimized queries within tenant boundaries
- **IAM Boundary Enforcement**: Policies preventing cross-tenant access
- **Rate Limiting**: Per-tenant rate limiting based on subscription plans

### DynamORM Integration (New Standardized Approach)
- **Standard pk/sk Structure**: All tables use `pk` and `sk` attributes
- **Composite Keys**: Clear entity identification with `tenant#{id}`, `user#{id}` patterns
- **GSIs via Struct Tags**: DynamORM struct tags define which fields map to GSIs (GSIs must be created in infrastructure)
- **Single Table Design**: Efficient patterns for multi-tenant isolation
- **TTL Support**: Automatic data expiration using DynamORM tags

### Monitoring & Observability
- **CloudWatch Metrics**: Table-level and tenant-specific metrics
- **CloudWatch Alarms**: Automated alerts for throttling and errors
- **CloudWatch Dashboards**: Comprehensive monitoring views
- **X-Ray Tracing**: Distributed tracing with tenant context
- **SNS Notifications**: Alert notifications for operational issues

### Security
- **Tenant Boundary Policies**: IAM policies with strict tenant isolation
- **Read-Only Policies**: Granular permission models
- **Admin Policies**: Elevated permissions with tenant constraints
- **Cross-Tenant Access Prevention**: Explicit deny policies

## Architecture

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────────┐
│   API Gateway   │────│  Lambda Function │────│  DynamORM Table     │
│                 │    │                  │    │                     │
│ /api/tenants    │    │  Multi-tenant    │    │ PK: tenant#{id}     │
│ /api/users      │    │  Request Router  │    │ SK: user#{id}       │
│ /api/projects   │    │                  │    │      project#{id}   │
│ /api/metrics    │    │  Tenant Context  │    │                     │
└─────────────────┘    └──────────────────┘    │ GSIs:               │
                                                │ - tenant-entity     │
┌─────────────────┐    ┌──────────────────┐    │ - tenant-timeseries │
│  CloudWatch     │    │    SNS Topic     │    │ - tenant-status     │
│                 │    │                  │    └─────────────────────┘
│ - Metrics       │    │  Alert           │
│ - Alarms        │    │  Notifications   │
│ - Dashboards    │    │                  │
│ - X-Ray Traces  │    │                  │
└─────────────────┘    └──────────────────┘
```

## Data Model

### DynamORM Patterns (Standardized pk/sk)

```go
// Table structure (created by CDK)
Primary Key: pk (String)
Sort Key: sk (String)
TTL: ttl (optional)

// Composite key patterns
Tenant:  pk="tenant#{tenant_id}", sk="tenant#{tenant_id}"
User:    pk="tenant#{tenant_id}", sk="user#{user_id}"
Project: pk="tenant#{tenant_id}", sk="project#{project_id}"
```

### Model Definition with GSIs

```go
type User struct {
    // Standard keys - BOTH tags required
    PK string `dynamorm:"pk" `  // tenant#{tenant_id}
    SK string `dynamorm:"sk" `  // user#{user_id}
    
    // GSI definitions (replaces CDK GSI creation)
    TenantID   string `dynamorm:"index:tenant-entity,pk" `
    EntityType string `dynamorm:"index:tenant-entity,sk" `
    CreatedAt  string `dynamorm:"index:tenant-timeseries,sk" `
    Status     string `dynamorm:"index:status-tenant,pk" `
    
    // Business fields
    ID    string `json:"id" `
    Name  string `json:"name" `
    Email string `json:"email" `
    TTL   int64  `json:"ttl,omitempty" dynamorm:"ttl"`
}
```

**Note**: The struct tags tell DynamORM which fields to use for GSI queries. However, you must create the actual GSIs in your infrastructure (CDK, CloudFormation, or AWS Console). DynamORM does not create GSIs at runtime.

## Deployment

### Prerequisites

1. Go 1.21+
2. AWS CDK v2
3. AWS CLI configured

### Build and Deploy

```bash
# Build the Lambda function
cd examples/dynamorm-multi-tenant
GOOS=linux GOARCH=arm64 go build -o bootstrap main.go
zip function.zip bootstrap

# Deploy the CDK stack
cd cdk
go mod tidy
cdk deploy
```

### Environment Variables

The deployed Lambda will have these environment variables:
- `DYNAMODB_TABLE_NAME`: The DynamORM table name
- `AWS_REGION`: AWS region
- `_X_AMZN_TRACE_ID`: X-Ray trace ID (set by Lambda runtime)

## Usage Examples

### Create a Tenant

```bash
curl -X POST https://your-api-gateway-url/api/tenants \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Acme Corp",
    "email": "admin@acme.com",
    "plan": "enterprise"
  }'
```

### Create a User (Tenant-Scoped)

```bash
curl -X POST https://your-api-gateway-url/api/users \
  -H "Content-Type: application/json" \
  -H "X-Tenant-ID: tenant-123" \
  -d '{
    "name": "John Doe",
    "email": "john@acme.com",
    "role": "admin"
  }'
```

### Create a Project (Tenant-Scoped)

```bash
curl -X POST https://your-api-gateway-url/api/projects \
  -H "Content-Type: application/json" \
  -H "X-Tenant-ID: tenant-123" \
  -H "X-User-ID: user-456" \
  -d '{
    "name": "New Product Launch",
    "description": "Q2 product launch project"
  }'
```

### List Tenant Users

```bash
curl -X GET https://your-api-gateway-url/api/users \
  -H "X-Tenant-ID: tenant-123"
```

### Get Tenant Metrics

```bash
curl -X GET https://your-api-gateway-url/api/metrics \
  -H "X-Tenant-ID: tenant-123"
```

## Monitoring

### CloudWatch Dashboards

1. **DynamORMMultiTenantStack**: Overall stack metrics
2. **DynamORMMultiTenantDashboard**: Table-specific metrics

### Available Metrics

- **Table Metrics**: Consumed capacity, throttling, errors, latency
- **Tenant Metrics**: Operations per tenant, access violations
- **Lambda Metrics**: Invocations, errors, duration, throttles
- **API Metrics**: Request count, latency, client/server errors

### Alarms

- **ThrottledRequests**: Triggers on any throttling
- **SystemErrors**: Triggers on DynamoDB system errors
- **HighLatency**: Triggers when latency > 100ms
- **HighCapacity**: Triggers at 80% capacity utilization
- **TenantAccessViolations**: Triggers on cross-tenant access attempts

## Security Features

### Tenant Isolation

1. **IAM Policies**: Prevent cross-tenant data access
2. **DynamORM Patterns**: Logical isolation in single table
3. **Application Layer**: Tenant context validation

### Access Control

```go
// Example tenant boundary policy
{
  "Effect": "Allow",
  "Action": ["dynamodb:GetItem", "dynamodb:PutItem", "dynamodb:UpdateItem"],
  "Resource": "table-arn",
  "Condition": {
    "ForAllValues:StringEquals": {
      "dynamodb:LeadingKeys": ["${aws:PrincipalTag/TenantID}"]
    }
  }
}
```

### Monitoring

- **Access Violations**: Tracked and alerted
- **Cross-Tenant Attempts**: Logged and monitored
- **Rate Limiting**: Per-tenant limits enforced

## Development

### Running Locally

```bash
# Install dependencies
go mod tidy

# Run with SAM Local (requires sam-cli)
sam local start-api

# Or run directly for testing
go run main.go
```

### Testing

```bash
# Run tests
go test ./...

# Test with coverage
go test -cover ./...
```

## Production Considerations

### Performance
- **Connection Pooling**: Configure DynamORM client properly
- **Caching**: Consider adding ElastiCache for frequent reads
- **Auto-scaling**: Enable DynamoDB auto-scaling

### Security
- **JWT Authentication**: Implement proper JWT validation
- **API Keys**: Add API key authentication for different access levels
- **VPC**: Deploy Lambda in VPC for enhanced security

### Monitoring
- **Custom Metrics**: Add business-specific metrics
- **Log Aggregation**: Use CloudWatch Logs Insights
- **Alerting**: Configure PagerDuty or similar for critical alerts

### Compliance
- **Data Retention**: Implement TTL for GDPR compliance
- **Audit Logs**: Track all data access and modifications
- **Encryption**: Enable encryption at rest and in transit

## Troubleshooting

### Common Issues

1. **Throttling**: Check CloudWatch metrics and adjust capacity
2. **Access Denied**: Verify IAM policies and tenant context
3. **High Latency**: Review query patterns and GSI usage

### Debugging

- **X-Ray Traces**: View distributed traces in AWS X-Ray console
- **CloudWatch Logs**: Check Lambda logs for application errors
- **DynamORM Logs**: Enable debug logging for DynamORM operations

## Related Examples

- [`multi-tenant-saas`](../multi-tenant-saas/): Basic multi-tenant example
- [`dynamorm-integration`](../dynamorm-integration/): DynamORM basics
- [`rate-limiting`](../rate-limiting/): Rate limiting patterns
- [`observability-demo`](../observability-demo/): Monitoring examples