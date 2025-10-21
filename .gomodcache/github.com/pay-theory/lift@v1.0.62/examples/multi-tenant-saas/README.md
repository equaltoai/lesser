# Multi-Tenant SaaS Example

This example demonstrates a comprehensive multi-tenant SaaS application built with the Lift framework. It showcases all major features including authentication, authorization, rate limiting, data isolation, and observability.

## Features

### 🏢 Multi-Tenancy
- **Tenant Isolation**: Complete data separation between tenants
- **Tenant-Specific Configuration**: Custom rate limits and settings per tenant
- **Tenant Context**: Automatic tenant context injection in all requests

### 🔐 Security
- **JWT Authentication**: Secure token-based authentication
- **Role-Based Access Control**: Admin, user, and viewer roles
- **API Key Support**: Alternative authentication method
- **Request Validation**: Comprehensive input validation

### 🚦 Rate Limiting
- **Tenant-Specific Limits**: Different rate limits based on subscription plans
- **Burst Protection**: Configurable burst limits
- **Fail-Open Design**: Graceful degradation when rate limiting fails
- **Multiple Strategies**: Fixed window, sliding window, and multi-window

### 📊 Observability
- **Structured Logging**: JSON-formatted logs with correlation IDs
- **Metrics Collection**: Request counts, latencies, and error rates
- **Health Checks**: Application health monitoring
- **Performance Tracking**: Response time monitoring

### 🗄️ Data Management
- **DynamORM Integration**: Type-safe DynamoDB operations
- **Single Table Design**: Efficient data modeling
- **Pagination**: Consistent pagination across all endpoints
- **CRUD Operations**: Complete Create, Read, Update, Delete functionality

## Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   API Gateway   │────│  Lambda (Lift)  │────│    DynamoDB     │
└─────────────────┘    └─────────────────┘    └─────────────────┘
                              │
                       ┌─────────────────┐
                       │   CloudWatch    │
                       └─────────────────┘
```

### Domain Model

```
Tenant (1) ──── (N) User
   │
   └── (N) Project (1) ──── (N) Task
```

- **Tenant**: Top-level organization with subscription plan and rate limits
- **User**: Individual users within a tenant with specific roles
- **Project**: Organizational unit for grouping tasks
- **Task**: Individual work items with status, priority, and assignments

## API Endpoints

### Public Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/tenants` | Create a new tenant |
| GET | `/api/health` | Health check |
| GET | `/metrics` | Application metrics |

### Protected Endpoints (Require Authentication)

#### Tenants
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/tenants/:id` | Get tenant details |

#### Users
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/users` | Create a new user |
| GET | `/api/users` | List users (paginated) |

#### Projects
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/projects` | Create a new project |
| GET | `/api/projects` | List projects (paginated) |

#### Tasks
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/tasks` | Create a new task |
| PUT | `/api/tasks/:id` | Update a task |
| GET | `/api/tasks` | List tasks by project (paginated) |

### Admin Endpoints (Require Admin Role)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/admin/*` | Administrative operations |

## Request/Response Examples

### Create Tenant

```bash
curl -X POST https://api.example.com/api/tenants \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Acme Corp",
    "email": "admin@acme.com",
    "plan": "pro"
  }'
```

Response:
```json
{
  "id": "tenant-123",
  "name": "Acme Corp",
  "email": "admin@acme.com",
  "plan": "pro",
  "status": "active",
  "rate_limit": 1000,
  "burst_limit": 50,
  "created_at": "2024-01-15T10:30:00Z",
  "updated_at": "2024-01-15T10:30:00Z"
}
```

### Create User

```bash
curl -X POST https://api.example.com/api/users \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <jwt-token>" \
  -H "X-Tenant-ID: tenant-123" \
  -d '{
    "name": "John Doe",
    "email": "john@acme.com",
    "role": "user"
  }'
```

### List Projects with Pagination

```bash
curl -X GET "https://api.example.com/api/projects?page=1&per_page=10" \
  -H "Authorization: Bearer <jwt-token>" \
  -H "X-Tenant-ID: tenant-123"
```

Response:
```json
{
  "data": [
    {
      "id": "project-456",
      "tenant_id": "tenant-123",
      "name": "Website Redesign",
      "description": "Complete redesign of company website",
      "status": "active",
      "owner_id": "user-789",
      "created_at": "2024-01-15T11:00:00Z",
      "updated_at": "2024-01-15T11:00:00Z"
    }
  ],
  "pagination": {
    "page": 1,
    "per_page": 10,
    "total": 1,
    "total_pages": 1,
    "next_page": null,
    "prev_page": null
  }
}
```

## Rate Limiting

### Subscription Plans

| Plan | Requests/Minute | Burst Limit |
|------|----------------|-------------|
| Free | 100 | 10 |
| Pro | 1,000 | 50 |
| Enterprise | 10,000 | 200 |

### Rate Limit Headers

All responses include rate limiting information:

```
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 999
X-RateLimit-Reset: 1642248600
```

When rate limit is exceeded (HTTP 429):

```
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1642248660
Retry-After: 60
```

## Authentication

### JWT Token Structure

```json
{
  "sub": "user-789",
  "tenant_id": "tenant-123",
  "role": "user",
  "scopes": ["read", "write"],
  "exp": 1642248600,
  "iat": 1642162200,
  "iss": "multi-tenant-saas",
  "aud": "api"
}
```

### Required Headers

For protected endpoints:

```
Authorization: Bearer <jwt-token>
X-Tenant-ID: <tenant-id>
```

## Error Handling

### Standard Error Response

```json
{
  "error": "Validation failed",
  "message": "Request validation failed",
  "validation_errors": {
    "name": ["Name is required", "Name must be at least 2 characters"],
    "email": ["Email format is invalid"]
  },
  "request_id": "req-123456",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

### HTTP Status Codes

| Code | Description |
|------|-------------|
| 200 | Success |
| 201 | Created |
| 204 | No Content |
| 400 | Bad Request |
| 401 | Unauthorized |
| 403 | Forbidden |
| 404 | Not Found |
| 409 | Conflict |
| 422 | Validation Error |
| 429 | Rate Limit Exceeded |
| 500 | Internal Server Error |

## Deployment

### Prerequisites

1. AWS Account with appropriate permissions
2. DynamoDB table configured
3. Lambda function deployed
4. API Gateway configured

### Environment Variables

```bash
# Database
DYNAMODB_TABLE_NAME=multi-tenant-saas
AWS_REGION=us-east-1

# Authentication
JWT_SECRET=your-secret-key
JWT_ISSUER=multi-tenant-saas
JWT_AUDIENCE=api

# Logging
LOG_LEVEL=info
LOG_FORMAT=json

# Rate Limiting
RATE_LIMIT_ENABLED=true
RATE_LIMIT_REDIS_URL=redis://localhost:6379
```

### DynamoDB Table Schema

```json
{
  "TableName": "multi-tenant-saas",
  "KeySchema": [
    {
      "AttributeName": "pk",
      "KeyType": "HASH"
    },
    {
      "AttributeName": "sk",
      "KeyType": "RANGE"
    }
  ],
  "AttributeDefinitions": [
    {
      "AttributeName": "pk",
      "AttributeType": "S"
    },
    {
      "AttributeName": "sk",
      "AttributeType": "S"
    },
    {
      "AttributeName": "gsi1pk",
      "AttributeType": "S"
    },
    {
      "AttributeName": "gsi1sk",
      "AttributeType": "S"
    }
  ],
  "GlobalSecondaryIndexes": [
    {
      "IndexName": "GSI1",
      "KeySchema": [
        {
          "AttributeName": "gsi1pk",
          "KeyType": "HASH"
        },
        {
          "AttributeName": "gsi1sk",
          "KeyType": "RANGE"
        }
      ],
      "Projection": {
        "ProjectionType": "ALL"
      }
    }
  ],
  "BillingMode": "PAY_PER_REQUEST"
}
```

### Single Table Design

| Entity | PK | SK | GSI1PK | GSI1SK |
|--------|----|----|--------|--------|
| Tenant | `TENANT#<id>` | `TENANT#<id>` | - | - |
| User | `TENANT#<tenant_id>` | `USER#<user_id>` | `USER#<user_id>` | `TENANT#<tenant_id>` |
| Project | `TENANT#<tenant_id>` | `PROJECT#<project_id>` | `PROJECT#<project_id>` | `TENANT#<tenant_id>` |
| Task | `PROJECT#<project_id>` | `TASK#<task_id>` | `TENANT#<tenant_id>` | `TASK#<task_id>` |

## Testing

### Unit Tests

```bash
go test ./...
```

### Integration Tests

```bash
go test -tags=integration ./...
```

### Load Testing

```bash
# Using the built-in load testing framework
go run examples/load-test/main.go
```

## Monitoring

### CloudWatch Metrics

- Request count by endpoint
- Response time percentiles
- Error rates by status code
- Rate limit violations
- Tenant activity

### CloudWatch Logs

Structured JSON logs with:
- Request ID for correlation
- Tenant ID for filtering
- User ID for auditing
- Performance metrics
- Error details

### Health Checks

```bash
curl https://api.example.com/api/health
```

Response:
```json
{
  "status": "healthy",
  "timestamp": "2024-01-15T10:30:00Z",
  "version": "1.0.0"
}
```

## Security Considerations

### Data Isolation

- All queries include tenant ID filters
- Cross-tenant access is prevented at the application level
- Database-level row security (future enhancement)

### Authentication Security

- JWT tokens with short expiration times
- Secure secret management using AWS Secrets Manager
- Token refresh mechanism (future enhancement)

### Rate Limiting Security

- Prevents abuse and ensures fair usage
- Tenant-specific limits based on subscription
- Burst protection for traffic spikes

### Input Validation

- Comprehensive validation using struct tags
- SQL injection prevention (N/A for DynamoDB)
- XSS prevention through proper encoding

## Performance Optimization

### Cold Start Optimization

- Minimal dependencies
- Connection pooling
- Lazy initialization

### Database Optimization

- Single table design for efficient queries
- Proper indexing strategy
- Connection reuse

### Caching Strategy

- In-memory caching for frequently accessed data
- Redis for distributed caching (future enhancement)
- CDN for static assets (future enhancement)

## Future Enhancements

### Planned Features

1. **Real-time Notifications**: WebSocket support for live updates
2. **File Uploads**: S3 integration for file storage
3. **Audit Logging**: Comprehensive audit trail
4. **Advanced Analytics**: Usage analytics and reporting
5. **Multi-region Support**: Global deployment strategy
6. **API Versioning**: Backward compatibility support

### Scalability Improvements

1. **Auto-scaling**: Dynamic scaling based on load
2. **Database Sharding**: Horizontal scaling strategy
3. **Microservices**: Service decomposition
4. **Event-driven Architecture**: Asynchronous processing

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

This example is part of the Lift framework and follows the same license terms. 