# Lesser

A cost-effective, serverless ActivityPub implementation built with Go, AWS Lambda, and DynamoDB.

## Overview

Lesser aims to make hosting ActivityPub instances affordable for individuals and small communities by leveraging serverless architecture. Instead of paying for always-on servers, you only pay for what you use.

## Features

- 🚀 **Serverless Architecture** - No always-on servers to maintain
- 💰 **Cost-Effective** - ~$23/month for 100 users (pay only for what you use)
- 🔄 **Full ActivityPub Support** - Compatible with Mastodon, Pleroma, and other ActivityPub servers
- 🔐 **Secure** - HTTP signatures, OAuth 2.0, and encrypted storage
- 📈 **Scalable** - Automatically scales with demand
- 🛠️ **Easy Deployment** - One-command infrastructure deployment with Pulumi
- 📱 WebFinger discovery

## Architecture

- **Compute**: AWS Lambda functions in Go
- **Storage**: DynamoDB for activities and actors
- **API**: API Gateway for HTTP endpoints
- **Media**: S3 for images and videos
- **IaC**: Pulumi for deployment

## Cost Estimates

- **Small instance** (100 users): ~$23/month
- **Medium instance** (1K users): ~$150/month
- **Large instance** (10K users): ~$600/month

Compare this to traditional VPS hosting which starts at $20-50/month for even the smallest instances!

## Project Structure

```
lesser/
├── cmd/                    # Lambda function handlers
│   ├── webfinger/         # WebFinger discovery
│   ├── actor/             # Actor profiles
│   ├── inbox/             # Receive activities
│   ├── outbox/            # Send activities
│   └── ...
├── pkg/                    # Shared packages
│   ├── activitypub/       # ActivityPub types and logic
│   ├── storage/           # DynamoDB interface
│   └── federation/        # HTTP signatures and delivery
├── infra/                  # Pulumi infrastructure
└── DESIGN.md              # Detailed design document
```

## Getting Started

See [DESIGN.md](DESIGN.md) for detailed architecture and implementation plans.

## Quick Start

### Prerequisites

- AWS Account
- Go 1.19 or later
- Pulumi CLI
- A domain with Route 53 hosted zone

### Deployment

1. **Clone the repository**
   ```bash
   git clone https://github.com/yourusername/lesser.git
   cd lesser
   ```

2. **Configure your domain**
   ```bash
   cd infra
   pulumi config set lesser:domain yourdomain.com
   pulumi config set lesser:hostedZoneId YOUR_ROUTE53_ZONE_ID
   pulumi config set lesser:jwtSecret $(openssl rand -base64 32) --secret
   ```

3. **Deploy to AWS**
   ```bash
   make deploy
   ```

4. **Create your first user**
   ```bash
   curl -X POST https://yourdomain.com/api/v1/accounts \
     -H "Content-Type: application/json" \
     -d '{
       "username": "alice",
       "email": "alice@example.com",
       "password": "secure-password",
       "agreement": true
     }'
   ```

That's it! Your ActivityPub server is now running. You can connect with any Mastodon-compatible client.

## Development

### Project Structure

```
lesser/
├── cmd/                    # Lambda function handlers
│   ├── webfinger/         # WebFinger discovery
│   ├── actor/             # Actor profiles
│   ├── inbox/             # Receive activities
│   ├── outbox/            # Send activities
│   └── ...
├── pkg/                    # Shared packages
│   ├── activitypub/       # ActivityPub types and logic
│   ├── storage/           # DynamoDB interface
│   └── federation/        # HTTP signatures and delivery
├── infra/                  # Pulumi infrastructure
└── DESIGN.md              # Detailed design document
```

## Contributing

We welcome contributions! Please see [DEVELOPER_GUIDELINES.md](DEVELOPER_GUIDELINES.md) for development standards and practices.

## Cost Breakdown

For 100 active users (estimated monthly):
- **DynamoDB**: $5-10 (pay-per-request pricing)
- **Lambda**: $5-10 (first million requests free)
- **S3**: $5 (media storage)
- **CloudFront**: $5 (bandwidth)
- **Total**: ~$20-30/month

## Status

Lesser is **95% complete** and ready for production use! It implements:
- ✅ Full ActivityPub protocol
- ✅ All activity types
- ✅ Mastodon client compatibility
- ✅ Media uploads
- ✅ OAuth 2.0 authentication
- ✅ One-click deployment

See [PROGRESS.md](PROGRESS.md) for detailed implementation status.

## License

[AGPL-3.0](LICENSE)
