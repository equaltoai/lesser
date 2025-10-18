# Lesser - Project Structure

## Directory Organization

```
lesser/
├── cmd/                    # Lambda functions (23 specialized functions)
│   ├── activity-processor/ # Process ActivityPub activities
│   ├── actor/              # Actor endpoint handler
│   ├── ai-processor/       # AI processing for content
│   ├── api/                # REST API handlers (Mastodon compatible)
│   ├── auth/               # Authentication service
│   ├── auth-api/           # Authentication API endpoints
│   ├── collections/        # ActivityPub collections handler
│   ├── configure-instance/ # Instance configuration tool
│   ├── cost-aggregator/    # Cost tracking and aggregation
│   ├── export-generator/   # Data export functionality
│   ├── federation-*/       # Federation-related services
│   ├── graphql/            # GraphQL API server
│   ├── import-processor/   # Data import functionality
│   ├── inbox/              # ActivityPub inbox handler
│   ├── init-deploy/        # Initial deployment setup
│   ├── media-processor/    # Media processing service
│   ├── moderation-*/       # Content moderation services
│   ├── note-processor/     # Note/post processing
│   ├── objects/            # ActivityPub objects handler
│   ├── outbox/             # ActivityPub outbox handler
│   ├── push-delivery/      # Push notification delivery
│   ├── search-indexer/     # Search indexing service
│   ├── status-indexer/     # Status indexing service
│   ├── stream-*/           # Streaming API services
│   ├── trend-aggregator/   # Trending content aggregation
│   └── webfinger/          # WebFinger discovery service
│
├── docs/                   # Documentation
│   ├── api/                # API documentation
│   ├── architecture/       # Architecture documentation
│   ├── deployment/         # Deployment guides
│   ├── development/        # Development guides
│   ├── implementation/     # Implementation details
│   ├── legal/              # Legal documentation
│   ├── security/           # Security documentation
│   └── use-cases/          # Use case documentation
│
├── graph/                  # GraphQL implementation
│   ├── model/              # GraphQL data models
│   ├── *.graphql           # GraphQL schema files
│   └── *_resolvers.go      # GraphQL resolvers
│
├── infra/                  # Infrastructure as Code (Pulumi)
│
├── pkg/                    # Core packages (shared code)
│   ├── activitypub/        # ActivityPub protocol implementation
│   ├── ai/                 # AI service integrations
│   ├── api/                # API utilities
│   ├── auth/               # Authentication utilities
│   ├── common/             # Common utilities
│   ├── config/             # Configuration management
│   ├── cost/               # Cost tracking
│   ├── federation/         # Federation utilities
│   ├── httpclient/         # HTTP client utilities
│   ├── mastodon/           # Mastodon API compatibility
│   ├── media/              # Media handling
│   ├── middleware/         # HTTP middleware
│   ├── moderation/         # Content moderation
│   ├── monitoring/         # Monitoring and metrics
│   ├── notes/              # Note/post handling
│   ├── notifications/      # Notification system
│   ├── ratelimit/          # Rate limiting
│   ├── reports/            # Reporting system
│   ├── reputation/         # Reputation system
│   ├── storage/            # Database access layer
│   ├── translation/        # Translation services
│   ├── trends/             # Trending content analysis
│   ├── trust/              # Trust system
│   └── websocket/          # WebSocket handling
│
├── scripts/                # Utility scripts
│
└── tests/                  # Test suites
    ├── api/                # API tests
    ├── auth/               # Authentication tests
    ├── federation/         # Federation tests
    ├── load/               # Load tests
    ├── moderation/         # Moderation tests
    ├── search/             # Search tests
    ├── system/             # System tests
    └── utilities/          # Test utilities
```

## Key Architecture Components

### Lambda Functions (cmd/)
Each Lambda function is designed for a specific purpose, following the microservices pattern. They are deployed as separate AWS Lambda functions, each with its own responsibility.

### Shared Code (pkg/)
Common functionality is organized into packages under `pkg/`. These packages are imported by the Lambda functions as needed. This promotes code reuse and separation of concerns.

### GraphQL API (graph/)
The GraphQL API is implemented using gqlgen. Schema files define the API structure, and resolvers implement the functionality.

### Infrastructure (infra/)
Infrastructure is defined as code using Pulumi. This includes all AWS resources needed for the application.

### Documentation (docs/)
Comprehensive documentation is organized by topic, covering all aspects of the system.

### Tests (tests/)
Tests are organized by functionality, with separate directories for different types of tests.

## Data Flow

1. **API Requests**: Handled by API Gateway, routed to appropriate Lambda functions
2. **ActivityPub Federation**: Inbox/outbox pattern for federation with other servers
3. **Asynchronous Processing**: SQS queues for reliable async processing
4. **Storage**: DynamoDB for structured data, S3 for media files
5. **Search**: Multi-strategy search system with indexing
6. **Notifications**: Push delivery for real-time updates

## Single Table Design

Lesser uses a DynamoDB single-table design with Global Secondary Indexes (GSIs) for different access patterns. This approach optimizes for cost and performance in a serverless environment.

## Code Organization Principles

1. **Separation of Concerns**: Each Lambda function and package has a clear responsibility
2. **Dependency Injection**: Dependencies are injected for better testability
3. **Interface-Based Design**: Interfaces define contracts between components
4. **Error Handling**: Structured error responses with appropriate status codes
5. **Configuration Management**: Environment-based configuration
6. **Logging**: Structured logging with contextual information