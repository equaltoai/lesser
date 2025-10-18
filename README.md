# Lesser

A serverless, cost-optimized ActivityPub implementation built with Go, AWS Lambda, and the Lift framework.

## Overview

Lesser is a Mastodon-compatible federated social media platform that runs entirely on AWS serverless infrastructure. It provides full ActivityPub federation while maintaining costs at a fraction of traditional server-based implementations.

## Key Features

- **Full ActivityPub Support**: Complete federation with Mastodon and other ActivityPub servers
- **Serverless Architecture**: 23 Lambda functions handling all operations
- **Cost Optimization**: Built-in cost tracking and budget controls for sustainable operation
- **Multi-Tenant Support**: Run multiple instances from a single deployment
- **GraphQL API**: Modern API with 60+ operations alongside Mastodon REST compatibility
- **WebSocket Streaming**: Real-time updates for timelines and notifications
- **AI Integration**: Optional semantic search and content moderation via AWS Bedrock
- **Enterprise Monitoring**: CloudWatch dashboards, EMF metrics, and comprehensive alerting

## Architecture

Lesser uses AWS CDK with the Lift framework for infrastructure:

- **Lambda Functions**: Event-driven compute for all operations
- **DynamoDB**: Single-table design with 8 GSIs for efficient queries
- **S3 + CloudFront**: Global CDN for media delivery
- **API Gateway**: HTTP API with custom domain support
- **SQS**: Reliable message queuing for federation and async processing
- **EventBridge**: Scheduled tasks for aggregation and maintenance

## Quick Start

### Prerequisites

- AWS Account with credentials configured (`aws configure`)
- AWS CDK v2 installed (`npm install -g aws-cdk`)
- Go 1.24 or later
- Make installed for build automation

### Basic Deployment

```bash
# Clone the repository
git clone https://github.com/equaltoai/lesser.git
cd lesser

# Build Lambda functions
make build-lambdas

# Deploy infrastructure
cd infra/cdk
cdk bootstrap  # First time only
cdk deploy --all
```

### Production Deployment

For production, you'll need a domain and SSL certificate:

```bash
# Deploy with custom domain and required production settings
cdk deploy --all \
  --context environment=production \
  --context domain=yourdomain.com \
  --context certificateArn=arn:aws:acm:us-east-1:xxx:certificate/xxx \
  --context jwtSecret=your-secure-secret
```

## Project Structure

```
lesser/
├── cmd/                    # Lambda function entry points
│   ├── api/               # Main REST API handler
│   ├── graphql/           # GraphQL API handler
│   ├── federation-delivery/ # ActivityPub delivery
│   ├── inbox/             # ActivityPub inbox
│   ├── outbox/            # ActivityPub outbox
│   └── ...                # 18 more specialized functions
├── pkg/                    # Core packages
│   ├── activitypub/       # ActivityPub protocol implementation
│   ├── auth/              # Authentication (WebAuthn, OAuth, crypto wallets)
│   ├── federation/        # Federation routing and optimization
│   ├── lift/              # Lift framework extensions
│   ├── services/          # Domain services (accounts, lists, etc.)
│   ├── storage/           # DynamoDB repositories and models
│   └── streaming/         # WebSocket and real-time updates
├── infra/
│   └── cdk/               # AWS CDK infrastructure
│       ├── stacks/        # CDK stack definitions
│       ├── constructs/    # Reusable CDK constructs
│       └── config/        # Environment configurations
└── graph/                  # GraphQL schema and resolvers
```

## Configuration

Environment-specific settings are in `infra/cdk/config/`:

- **dev.yaml**: Development environment (512MB RAM, DEBUG logging)
- **staging.yaml**: Staging environment (1GB RAM, INFO logging)
- **prod.yaml**: Production environment (3GB RAM, full monitoring)

### Environment Variables

Key configuration options:

```bash
INSTANCE_TITLE="My Lesser Instance"
INSTANCE_ADMIN_EMAIL="admin@yourdomain.com"
FEDERATION_ENABLED=true
REGISTRATIONS_OPEN=false
MAX_STATUS_CHARS=5000
```

## Cost Management

Lesser includes comprehensive cost tracking:

- **Real-time cost calculation** for every operation
- **Per-instance budgets** with automatic enforcement
- **Cost aggregation** via scheduled Lambda functions
- **Budget alerts** through SNS and CloudWatch

Typical monthly costs:
- Development: < $5
- Small instance (100 users): $10-20
- Medium instance (1000 users): $50-100
- Large instance (10000 users): $200-500

## Monitoring

Built-in observability features:

- **CloudWatch Dashboards**: Comprehensive metrics for all components
- **EMF Metrics**: Structured metrics with dimensions
- **X-Ray Tracing**: Distributed tracing for debugging
- **Custom Alarms**: Automatic alerting for errors and performance issues

Access your dashboard at:
```
https://console.aws.amazon.com/cloudwatch/home?region=us-east-1#dashboards:name=lesser-{environment}
```

## API Documentation

Lesser provides three API interfaces:

### REST API (Mastodon-compatible)
- Full Mastodon v1 API compatibility
- Additional Lesser-specific endpoints
- OAuth 2.0 authentication

### GraphQL API
- 60+ operations for queries, mutations, and subscriptions
- DataLoader for N+1 query prevention
- Real-time subscriptions via WebSocket

### WebSocket Streaming
- Real-time timeline updates
- Notification streaming
- Presence and typing indicators

## Development

### Building Locally

```bash
# Install dependencies
go mod download

# Run tests
make test

# Build all Lambda functions
make build-lambdas

# Run specific function locally
cd cmd/api
go run main.go
```

### Testing

```bash
# Unit tests
make test

# Integration tests
make test-integration

# Load tests
make test-load
```

## Federation

Lesser implements the full ActivityPub protocol:

- **Inbox/Outbox**: Complete activity processing
- **WebFinger**: User discovery
- **HTTP Signatures**: Secure federation
- **Relay Support**: Optional relay configuration
- **Instance Blocks**: Moderation tools

## Security

- **Multi-factor Authentication**: WebAuthn, TOTP, backup codes
- **OAuth 2.0**: Secure third-party app access
- **Rate Limiting**: DDoS protection via AWS WAF
- **Encryption**: At-rest and in-transit encryption
- **Audit Logging**: Comprehensive security event tracking

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

Lesser is licensed under the GNU Affero General Public License v3.0. See [LICENSE](LICENSE) for details.

## Support

- **Documentation**: See the [docs](docs/) directory
- **Issues**: [GitHub Issues](https://github.com/equaltoai/lesser/issues)
- **Discussions**: [GitHub Discussions](https://github.com/equaltoai/lesser/discussions)

## Acknowledgments

Lesser is built on:
- [Lift Framework](https://github.com/pay-theory/lift) for Lambda patterns
- [DynamORM](https://github.com/pay-theory/dynamorm) for DynamoDB operations
- [gqlgen](https://github.com/99designs/gqlgen) for GraphQL
- The ActivityPub community for protocol specifications