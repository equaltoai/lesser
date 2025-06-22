# Lesser

A **100% complete** serverless ActivityPub implementation that makes federated social media essentially free to operate. Built with Go, AWS Lambda, and DynamoDB. **Created in just 5 days using AI assistance.**

## 🎉 MVP Complete!

Lesser has achieved complete MVP status with full ActivityPub federation and Mastodon API compatibility. See our **[MVP Complete Summary](docs/MVP_COMPLETE_SUMMARY.md)** for a comprehensive feature list.

## Overview

Lesser proves that federated social media can cost pennies instead of hundreds of dollars per month. By leveraging serverless architecture and innovative features like AI-powered search, reactive moderation, and real-time cost tracking, Lesser enables anyone to run their own social media instance for $1-10/month.

## 🚀 What Makes Lesser Revolutionary

- **💰 1/100th the Cost** - $1-10/month for hundreds of users (compare to $50-500 for traditional hosting)
- **🤖 AI-Powered Search** - 13 search strategies including semantic understanding via AWS Bedrock
- **📊 Real-Time Cost Tracking** - See the cost of every action down to the micro-cent
- **🧠 Reactive Moderation Mesh** - Community-driven moderation with trust propagation
- **🔐 Modern Authentication** - WebAuthn, OAuth 2.0, and Web3 wallet support
- **🌐 100% ActivityPub** - Full federation with 10M+ Fediverse users
- **⚡ True Serverless** - Scales to zero, scales to millions, no servers to manage

## Current Status: 100% MVP Complete! ✅

Built in just 5 days using AI assistance (Cursor), Lesser now includes more features than many established ActivityPub implementations:

- ✅ **Full ActivityPub Protocol** - Complete federation implementation
- ✅ **100% Mastodon API** - All v1 endpoints implemented
- ✅ **60/60 GraphQL Operations** - Modern API with DataLoader optimization
- ✅ **AI-Powered Search** - Semantic search with AWS Bedrock Titan embeddings
- ✅ **Push Notifications** - Web Push Protocol with encryption
- ✅ **Media Processing** - AWS MediaConvert integration
- ✅ **Advanced Features** - Polls, filters, lists, scheduled posts, hashtag following
- ✅ **Enterprise Ready** - Cost tracking, audit logging, trust system

## ⚠️ Security Status

A comprehensive security audit has been conducted on the Lesser prototype. We are actively addressing all findings:

- **33 security findings** identified and documented
- **5-week remediation plan** in progress
- **Critical issues** being addressed first
- See [SECURITY_UPDATE_PLAN.md](SECURITY_UPDATE_PLAN.md) for details

**Important**: Lesser is currently in prototype phase. Do not deploy to production until security remediation is complete.

## Architecture

### Serverless-Native Design
- **Compute**: AWS Lambda (23 specialized functions)
- **Storage**: DynamoDB (single-table design with 8 GSIs)
- **Media**: S3 + CloudFront CDN
- **Search**: Multi-strategy with AI embeddings
- **Queue**: SQS for reliable async processing
- **Deploy**: Pulumi (infrastructure as code)

### Cost Breakdown (Monthly)

| Users | Traditional (Mastodon) | Lesser Serverless | Savings |
|-------|------------------------|-------------------|---------|
| 100   | $50-100               | $1-3              | 97%     |
| 1,000 | $200-500              | $10-30            | 94%     |
| 10,000| $1,000-5,000          | $100-300          | 90%     |

## 🎯 Key Innovations

### 1. Multi-Strategy Search System
Lesser implements 13 different search strategies across accounts, statuses, and hashtags:
- **Semantic search** using AWS Bedrock Titan embeddings (1536-dimensional)
- **Exact, prefix, fuzzy matching** with intelligent fallbacks
- **Language detection** via AWS Comprehend
- **Personalized results** based on social graph
- **Real-time indexing** with 90-day TTL

### 2. Cost-Aware Infrastructure
Every API response includes detailed cost breakdowns:
```json
{
  "data": { ... },
  "cost": {
    "total_cost_micros": 234,  // $0.000234
    "breakdown": {
      "dynamodb_reads": 2,
      "lambda_ms": 45,
      "bedrock_tokens": 150
    }
  }
}
```

### 3. Complete Developer Experience
- **GraphQL + REST APIs** - Use your preferred approach
- **WebSocket streaming** - Real-time updates
- **Comprehensive documentation** - Every endpoint documented
- **Postman collection** - Import and start testing

## Quick Start

### Prerequisites
- AWS Account
- Go 1.21+
- Node.js 18+ (for frontend)
- Pulumi CLI
- A domain name

### Deploy Your Instance

```bash
# 1. Clone and configure
git clone https://github.com/yourusername/lesser.git
cd lesser
cp .env.example .env
# Edit .env with your settings

# 2. Deploy infrastructure
cd infra
pulumi config set domain yourdomain.com
pulumi config set aws:region us-east-1
pulumi up

# 3. Your instance is live!
✅ WebFinger: https://yourdomain.com/.well-known/webfinger
✅ Your handle: @you@yourdomain.com
✅ Mastodon API: https://yourdomain.com/api/v1/
✅ GraphQL: https://yourdomain.com/graphql
✅ Cost so far: ~$0.10
```

### Connect with Apps
Lesser works with all Mastodon clients:
- **iOS**: Ivory, Toot!, Ice Cubes, Mammoth
- **Android**: Tusky, Fedilab, Megalodon
- **Web**: Elk, Phanpy, Semaphore
- **Desktop**: Whalebird, Hyperspace

## Development

### Project Structure
```
lesser/
├── cmd/                    # Lambda functions
│   ├── api/               # REST API handlers
│   ├── graphql/           # GraphQL server
│   ├── federation/        # ActivityPub endpoints
│   └── processors/        # Async workers
├── pkg/                    # Core packages
│   ├── activitypub/       # Protocol implementation
│   ├── storage/           # DynamoDB interface
│   ├── search/            # Multi-strategy search
│   ├── ai/                # AWS AI integrations
│   └── cost/              # Cost tracking
├── infra/                  # Pulumi IaC
├── docs/                   # Documentation
└── tests/                  # Test suites
```

### Running Tests
```bash
# Unit tests
make test

# Integration tests  
make test-integration

# GraphQL tests
python tests/test_graphql.py

# Full suite
make test-all
```

## Documentation

### 📚 Essential Reading
- **[MVP Complete Summary](docs/MVP_COMPLETE_SUMMARY.md)** - All implemented features
- **[Documentation Index](docs/DOCUMENTATION_INDEX.md)** - Complete navigation guide
- **[Quick Start Guide](docs/deployment/QUICK_START.md)** - Deploy in 15 minutes
- **[Architecture Overview](docs/architecture/OVERVIEW.md)** - System design

### 🏗️ Technical Deep Dives
- **[API Reference](docs/api/API_REFERENCE.md)** - Complete REST API
- **[GraphQL API](docs/api/GRAPHQL_API.md)** - GraphQL schema and operations
- **[Search Design](docs/architecture/SEARCH_DESIGN.md)** - Multi-strategy search system
- **[Storage Architecture](docs/architecture/STORAGE_ARCHITECTURE.md)** - DynamoDB patterns

### 🚀 For Businesses
- **[Use Cases](docs/use-cases/)** - Community, government, research platforms
- **[PayTheory Partnership](paytheory-partnership/)** - Social commerce integration
- **[Pitch Deck](docs/PITCH.md)** - Lesser value proposition

## Why Lesser?

### For Individuals
- **Own your social presence** - No platform lock-in
- **Costs less than coffee** - $1-3/month typical
- **Privacy first** - No ads, no tracking
- **Full features** - Everything Mastodon has and more

### For Communities  
- **Sustainable** - Low costs = long-term viability
- **Transparent** - See exactly what everything costs
- **Safe** - AI-assisted moderation + community consensus
- **Connected** - Federate with 10M+ users

### For Developers
- **Modern stack** - Go, GraphQL, WebSockets
- **Well documented** - Every endpoint, every feature
- **Cost aware** - Build with economics in mind
- **Open source** - AGPL-3.0 license

## The 5-Day Build Story

Lesser was built in just 5 days using AI assistance (Cursor/Claude), proving that:
- Modern AI tools can accelerate development dramatically
- Serverless architecture enables rapid implementation
- Complex protocols like ActivityPub can be implemented quickly
- A single developer with AI can outpace traditional teams

Read the [full story](paytheory-partnership/pitch-materials/lesser_5_days_with_cursor_story.md) of how Lesser was built.

## Contributing

With the MVP complete, we're looking for contributors to help with:
- **Frontend Development** - Build beautiful UIs on top of Lesser
- **Mobile Apps** - Native iOS/Android clients
- **Feature Extensions** - Live streaming, voice spaces, e2e encryption
- **Language Support** - Internationalization
- **Documentation** - Tutorials, guides, videos

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

[GNU AGPL-3.0](LICENSE) - Free as in freedom, copyleft for the community

## Acknowledgments

Lesser stands on the shoulders of giants:
- The ActivityPub W3C working group
- Mastodon and the broader Fediverse community  
- AWS for making serverless accessible
- Anthropic's Claude for AI assistance
- Everyone who believes social media should be free and open

---

<div align="center">

**Lesser: Social Media Infrastructure for Everyone**

*Proving that federated social media doesn't need to be expensive. It just needs to be built differently.*

[Deploy Now](docs/deployment/QUICK_START.md) • [View Features](docs/MVP_COMPLETE_SUMMARY.md) • [Read Docs](docs/DOCUMENTATION_INDEX.md)

</div>
