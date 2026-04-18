# Lesser

A serverless, cost-optimized ActivityPub implementation built with Go, AWS Lambda, AppTheory, and TableTheory.

## Overview

Lesser is a Mastodon-compatible federated social media platform that runs entirely on AWS serverless infrastructure. It provides full ActivityPub federation while maintaining costs at a fraction of traditional server-based implementations.

For canonical operator and developer docs, start with `docs/README.md`. For soul and managed-agent boundaries, start with
`docs/soul.md`.

## Key Features

- **Full ActivityPub Support**: Complete federation with Mastodon and other ActivityPub servers
- **Serverless Architecture**: Dozens of Lambda functions for API + background processing
- **Cost Optimization**: Built-in cost tracking and budget controls for sustainable operation
- **Multi-Tenant Support**: Run multiple instances from a single deployment
- **GraphQL API**: Modern API with 60+ operations alongside Mastodon REST compatibility
- **WebSocket Streaming**: Real-time updates for timelines and notifications
- **AI Integration**: Optional semantic search and content moderation via AWS Bedrock
- **Enterprise Monitoring**: CloudWatch dashboards, EMF metrics, and comprehensive alerting

## Architecture

Lesser uses AWS CDK plus Theory Cloud framework building blocks:

- **AppTheory**: Runtime and CDK routing/middleware patterns
- **TableTheory**: Single-table DynamoDB models and repositories

- **Lambda Functions**: Event-driven compute for all operations
- **DynamoDB**: Single-table design with 9 generic GSIs for efficient queries
- **S3 + CloudFront**: Global CDN for media delivery
- **API Gateway**: HTTP API with custom domain support
- **SQS**: Reliable message queuing for federation and async processing
- **EventBridge**: Scheduled tasks for aggregation and maintenance

## Quick Start

### Prerequisites

- AWS credentials configured (for example: `aws sso login --profile ...` or `aws configure`)
- AWS CDK v2 installed (`npm install -g aws-cdk`)
- Go 1.25 or later
- A public Route53 hosted zone that exactly matches your base domain (for example: `example.com`)

### Basic Deployment

```bash
# Build the operator CLI
go build -o lesser ./cmd/lesser

# Deploy dev + live (and optionally staging)
./lesser up \
  --app my-lesser \
  --base-domain example.com \
  --aws-profile Penny \
  --out ~/.lesser/my-lesser/example.com/bootstrap.json
```

Notes:
- The **live** stage uses the apex domain (`example.com`), while **dev** uses `dev.example.com` (and **staging** uses `staging.example.com` if enabled).
- On first deploy, `--out <path>` is required so you don’t lose the 24-word Ethereum mnemonic (the file is created with `0600` permissions).
- A local deployment receipt is written to `~/.lesser/<app>/<base-domain>/state.json`.

## Project Structure

```
lesser/
├── cmd/                    # Lambda function entry points
│   ├── api/               # Main REST API handler
│   ├── graphql/           # GraphQL API handler
│   ├── federation-delivery/ # ActivityPub delivery
│   ├── inbox/             # ActivityPub inbox
│   ├── outbox/            # ActivityPub outbox
│   └── ...                # Additional specialized functions
├── pkg/                    # Core packages
│   ├── activitypub/       # ActivityPub protocol implementation
│   ├── apptheory/         # AppTheory wiring and migration helpers
│   ├── auth/              # Authentication (WebAuthn, OAuth, crypto wallets)
│   ├── federation/        # Federation routing and optimization
│   ├── services/          # Domain services (accounts, lists, etc.)
│   ├── storage/           # DynamoDB repositories and models
│   └── streaming/         # WebSocket and real-time updates
├── infra/
│   └── cdk/               # AWS CDK infrastructure
│       ├── stacks/        # CDK stack definitions
│       ├── constructs/    # Reusable CDK constructs
│       └── config/        # Reference templates (not loaded by CDK app)
└── graph/                  # GraphQL schema and resolvers
```

## Configuration

Infrastructure defaults (memory/timeouts, tables/buckets, CloudFront, etc.) live in `infra/cdk/stacks/` and `infra/cdk/inventory/`.

Soul and managed-agent integration boundaries are documented in `docs/soul.md`. Public actor-scoped MCP access lives in
`docs/mcp-remote-access.md`.

### Environment Variables

Runtime configuration is managed in AWS for deployed stacks; see `docs/configuration.md` for the canonical reference.

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

```bash
./lesser dashboard --app <app> --env live --region us-east-1
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
- **Published schema**: The canonical schema we ship to clients lives at `docs/contracts/graphql-schema.graphql`.
  It is generated from the modular source files (`graph/core.graphql`, `graph/phase1.graphql`, `graph/phase2.graphql`, `graph/phase3.graphql`)
  by running `./lesser schema` (or `./scripts/generate_schema.sh`). Always rerun it before sharing or checking in schema changes so frontend teams
  see every type in one place.

### WebSocket Streaming
- Real-time timeline updates
- Notification streaming
- Presence and typing indicators

## Development

### Building Locally

```bash
# Install dependencies
go mod download

# Build the Lesser CLI
go build -o lesser ./cmd/lesser

# Run tests
./lesser test

# Build all Lambda functions
./lesser build lambdas

# Run specific function locally
cd cmd/api
go run main.go
```

### Testing

```bash
# Unit tests
./lesser test

# Short unit sweep used by verify
./lesser test unit

# Unified verification (Spec 07 R6: lambda set, inventory, docs, unit tests)
./lesser verify

# Enable optional smoke suites inside verify (non-destructive HTTP only)
./lesser verify --smoke --smoke-base-url=https://lesser.host --smoke-token="Bearer xyz" --smoke-username=alice --smoke-object-id=123

# Run smoke suites directly
./lesser smoke core --base-url=https://lesser.host --token="Bearer xyz"
./lesser smoke federation --base-url=https://lesser.host --username=alice --object-id=123

# Enable optional CDK synth inside verify
./lesser verify --cdk --cdk-aws-profile=<profile> --cdk-region=us-east-1
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
