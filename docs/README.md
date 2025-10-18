# Lesser Documentation

## Quick Links

- [Deployment Guide](deployment.md) - Get Lesser running
- [Configuration Reference](configuration.md) - All configuration options
- [API Reference](api-reference.md) - REST, GraphQL, and WebSocket APIs
- [Architecture Overview](architecture.md) - System design and components
- [Federation Guide](federation.md) - ActivityPub implementation details
- [Cost Optimization](cost-optimization.md) - Managing and reducing costs
- [Troubleshooting](troubleshooting.md) - Common issues and solutions

## What is Lesser?

Lesser is a serverless ActivityPub implementation that provides Mastodon-compatible federated social media at a fraction of the traditional cost. Built on AWS Lambda and DynamoDB, it scales automatically and charges only for actual usage.

## Key Concepts

### Serverless Architecture
Lesser runs entirely on AWS managed services, eliminating server management and reducing costs by up to 90% compared to traditional hosting.

### ActivityPub Federation
Full compatibility with the ActivityPub protocol allows Lesser instances to communicate with Mastodon, Pleroma, and other federated platforms.

### Cost-Aware Design
Every component is optimized for cost, with built-in tracking, budgets, and automatic optimization strategies.

### Multi-Tenant Support
A single Lesser deployment can host multiple independent instances with complete data isolation.

## Getting Help

- **GitHub Issues**: Report bugs or request features
- **Discussions**: Ask questions and share experiences
- **CDK Documentation**: See `infra/cdk/README.md` for deployment details

## For Developers

- [Development Setup](development.md) - Local development environment
- [Testing Guide](testing.md) - Running and writing tests
- [Contributing](../CONTRIBUTING.md) - How to contribute

## For Operators

- [Monitoring Guide](monitoring.md) - CloudWatch dashboards and alerts
- [Security Guide](security.md) - Security best practices
- [Backup & Recovery](backup-recovery.md) - Data protection strategies
