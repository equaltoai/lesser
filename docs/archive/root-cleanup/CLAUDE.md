# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Lesser is a complete serverless ActivityPub implementation built in just 5 days using AI assistance. It provides:
- Full Mastodon API compatibility (100% of v1 endpoints)
- Complete ActivityPub federation protocol implementation
- GraphQL API with 60 operations
- WebSocket streaming support
- 1/100th the operational cost of traditional hosting solutions

## Key Development Commands

### Building
```bash
make build          # Build all Lambda functions
make build-lambdas  # Build Lambda deployment packages
make generate       # Generate GraphQL code
```

### Testing
```bash
make test           # Run all Go unit tests
make test-api       # Run Python API tests
make test-federation # Run federation tests
make test-search    # Run search tests
make test-ai        # Run AI integration tests
make test-auth      # Run authentication tests
make test-load      # Run k6 load tests
```

### Development
```bash
make dev            # Run local development server
make fmt            # Format Go code
make lint           # Run linters
make clean          # Clean build artifacts
```

### Deployment
```bash
make deploy         # Deploy with Pulumi
make logs           # Tail Lambda logs
make pulumi-up      # Deploy infrastructure
make pulumi-destroy # Destroy infrastructure
```

### Load Testing
```bash
make k6-auth        # Test auth endpoints
make k6-timeline    # Test timeline performance
make k6-posting     # Test post creation
make k6-federation  # Test federation
```

## Architecture Overview

### Serverless Design
- **23 Lambda Functions**: Each function handles specific responsibilities
- **DynamoDB**: Single-table design with 8 GSIs for efficient queries
- **S3**: Media storage with CloudFront CDN
- **SQS**: Async job processing

### Key Lambda Functions
- `api`: Main REST API handler (Mastodon-compatible)
- `graphql`: GraphQL API server
- `auth` & `auth-api`: Authentication services
- `inbox`/`outbox`: ActivityPub federation endpoints
- `processor-*`: Async processors for various tasks

### Directory Structure
```
/cmd/          # Lambda function entry points (23 services)
/pkg/          # Core business logic and packages
  /activitypub/  # Protocol implementation
  /storage/      # DynamoDB data layer
  /auth/         # Authentication (OAuth, WebAuthn, wallet)
  /federation/   # Federation routing and delivery
  /ai/           # AWS Bedrock integration
/infra/        # Pulumi infrastructure as code
/tests/        # Python integration tests
/graph/        # GraphQL schema and resolvers
/docs/         # Documentation
```

## Important Design Patterns

### DynamoDB Single-Table Design
- All data in one table with composite keys
- 8 GSIs for different access patterns
- Careful attention to hot partition avoidance
- Cost tracking on every DB operation

### Cost Tracking
- Every DynamoDB operation tracks consumed capacity
- Real-time cost monitoring via context
- Aggregated cost reporting
- Target: < $0.01 per user per month

### Lambda Considerations
- **Stateless**: No shared memory between invocations
- **Cold Starts**: Keep Lambda packages small
- **Timeouts**: 30s API Gateway limit
- **Memory**: Cost vs performance optimization

## API Information

### REST API (Mastodon v1)
- Base path: `/api/v1/`
- Full compatibility with Mastodon clients
- OAuth 2.0 authentication
- WebSocket streaming at `/api/v1/streaming`

### GraphQL API
- Endpoint: `/graphql`
- 60 operations (queries, mutations, subscriptions)
- DataLoader for N+1 query prevention
- Real-time subscriptions

### Key Endpoints
- `/inbox` - ActivityPub inbox (federation)
- `/outbox` - ActivityPub outbox
- `/.well-known/webfinger` - WebFinger discovery
- `/nodeinfo` - Instance information

## Development Guidelines

### When Making Changes
1. **Lambda Functions**: Keep them focused and small
2. **DynamoDB**: Always consider cost and hot partitions
3. **Federation**: Test with real ActivityPub instances
4. **Authentication**: Support OAuth, WebAuthn, and wallet
5. **Cost Tracking**: Add tracking to new DB operations

### Common Pitfalls
- Don't assume Lambda persistence between invocations
- Always handle DynamoDB throttling gracefully
- Test federation with multiple server implementations
- Consider API Gateway 30s timeout limit
- Remember S3 eventual consistency

### Security Considerations
- Never log sensitive data (tokens, keys)
- Always validate ActivityPub signatures
- Use CSRF protection on state-changing operations
- Sanitize all user input
- Rate limit by user, not IP (Lambda shares IPs)

## Environment Configuration

Key environment variables needed:
- `DOMAIN_NAME`: Your instance domain
- `AWS_REGION`: AWS region for resources
- `DYNAMODB_TABLE`: Main DynamoDB table name
- `PRIVATE_KEY_SECRET`: ActivityPub signing key

## AI Development Methodology

Lesser was built using a "chunking" methodology:
1. **Deep-First**: Complete one feature entirely before moving to the next
2. **Three-Tab Model**: README (vision), STATE (progress), ARCHITECTURE (design)
3. **No Placeholders**: Every function works or doesn't exist
4. **Continuous Testing**: API tests run after each chunk

## Current Git Status

Branch: main
Modified files:
- pkg/auth/webauthn.go
- pkg/storage/dynamodb/auth.go
- docs/greater/ (untracked)

## Project References

- **docs/greater**: The client application developed independent of greater, this is available for reference only

## Testing Notes

- Python tests use pytest and require `pip install -r requirements.txt`
- Load tests use k6 and test real performance characteristics
- Federation tests require ngrok or public endpoint
- All tests can run against local or deployed instances