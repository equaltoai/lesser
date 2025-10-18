# Lesser - Technical Stack

## Core Technologies

- **Language**: Go 1.23+
- **Cloud Provider**: AWS (Serverless Architecture)
- **Infrastructure as Code**: Pulumi
- **Database**: DynamoDB (single-table design with GSIs)
- **API**: GraphQL + REST (Mastodon API compatible)
- **Authentication**: OAuth 2.0, WebAuthn, Web3 wallets
- **Media Processing**: AWS MediaConvert
- **Search**: Multi-strategy with AWS Bedrock Titan embeddings
- **Messaging**: SQS for async processing
- **Storage**: S3 + CloudFront CDN
- **AI Services**: AWS Bedrock, AWS Comprehend

## Key Libraries & Dependencies

- **GraphQL**: 99designs/gqlgen
- **AWS SDK**: aws-sdk-go-v2
- **HTTP Router**: go-chi/chi
- **Authentication**: go-webauthn/webauthn, golang-jwt/jwt
- **JSON Processing**: bytedance/sonic
- **WebSockets**: gorilla/websocket
- **Logging**: go.uber.org/zap
- **Testing**: stretchr/testify

## Common Commands

### Build Commands

```bash
# Build all Lambda functions
make build

# Build a specific Lambda function
make build-[function-name]

# Build and package all Lambda functions
make build-lambdas

# Generate GraphQL code
make gqlgen
```

### Deployment Commands

```bash
# Deploy infrastructure with Pulumi
make deploy-infra

# Deploy Lambda functions
make deploy-functions

# Full deployment (build + Pulumi)
make deploy

# Preview deployment changes
make deploy-preview

# Initialize deployment (VAPID keys + admin account)
make init-deploy DOMAIN=your-domain.com
```

### Development Commands

```bash
# Run tests
make test

# Run tests with coverage
make test-coverage

# Run integration tests
make integration-test

# Format code
make fmt

# Run linter
make lint

# Initialize development environment
make dev-init

# Start local DynamoDB
make local-dynamodb
```

### Load Testing

```bash
# Run k6 load test locally
make k6-local

# Run k6 test on Grafana Cloud
make k6-cloud

# Setup k6 environment
make k6-setup
```

## Development Conventions

- **Lambda Function Structure**: Each Lambda function is in its own directory under `cmd/`
- **Shared Code**: Common functionality is in packages under `pkg/`
- **Single Table Design**: DynamoDB uses a single table with GSIs for different access patterns
- **Error Handling**: Structured error responses with appropriate HTTP status codes
- **Logging**: Structured logging with zap logger
- **Testing**: Unit tests for core functionality, integration tests for API endpoints

## Environment Configuration

Key environment variables are stored in `.env` file:
- `DOMAIN`: Instance domain name
- `INSTANCE_NAME`: Name of the Lesser instance
- `AWS_REGION`: AWS region for deployment
- `DYNAMO_TABLE_NAME`: DynamoDB table name
- `S3_BUCKET_NAME`: S3 bucket for media storage
- `JWT_SECRET`: Secret for JWT token signing