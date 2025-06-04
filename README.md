# Lesser

A cost-effective, serverless ActivityPub implementation built with Go, AWS Lambda, and DynamoDB.

## Overview

Lesser aims to make hosting ActivityPub instances affordable for individuals and small communities by leveraging serverless architecture. Instead of paying for always-on servers, you only pay for what you use.

## Features

- ✅ Full ActivityPub compliance
- 💰 Pay-per-use pricing model
- 🚀 Serverless architecture (AWS Lambda)
- 📦 DynamoDB for scalable storage
- 🔧 Infrastructure as Code with Pulumi
- 🔒 HTTP Signature authentication
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

## License

[AGPL-3.0](LICENSE)
