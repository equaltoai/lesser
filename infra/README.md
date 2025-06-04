# Lesser Infrastructure Deployment Guide

This directory contains the Pulumi infrastructure code for deploying Lesser to AWS.

## Prerequisites

1. **AWS Account** with appropriate permissions
2. **Pulumi CLI** installed ([installation guide](https://www.pulumi.com/docs/get-started/install/))
3. **Go 1.19+** installed
4. **AWS CLI** configured with credentials
5. **Domain** with Route 53 hosted zone

## Initial Setup

1. **Build Lambda functions** (from project root):
   ```bash
   make build-lambdas
   ```

2. **Configure Pulumi** (from infra directory):
   ```bash
   cd infra
   pulumi stack select production  # or create with: pulumi stack new production
   ```

3. **Set configuration values**:
   ```bash
   pulumi config set lesser:domain lesser.host
   pulumi config set lesser:hostedZoneId Z0051991INEKM5CN2M42
   pulumi config set lesser:environment production
   pulumi config set aws:region us-east-1
   
   # Generate and set JWT secret
   pulumi config set lesser:jwtSecret $(openssl rand -base64 32) --secret
   ```

## Deployment

1. **Preview changes**:
   ```bash
   pulumi preview
   ```

2. **Deploy infrastructure**:
   ```bash
   pulumi up
   ```

3. **Note the outputs**:
   ```
   Outputs:
   apiUrl: "https://lesser.host"
   mediaUrl: "https://media.lesser.host"
   tableName: "lesser-production"
   bucketName: "lesser-media-production"
   ```

## What Gets Deployed

### Core Infrastructure:
- **DynamoDB Table** - Single table with 5 GSIs for all data
- **S3 Bucket** - Media storage with lifecycle policies
- **CloudFront CDN** - Global media distribution
- **API Gateway** - HTTP API with custom domain

### Lambda Functions:
- `api` - Mastodon API endpoints
- `actor` - Actor profile endpoint
- `inbox` - Inbox handler
- `outbox` - Outbox handler
- `collections` - Followers/following
- `objects` - Object retrieval
- `webfinger` - Discovery endpoint
- `auth` - OAuth endpoints
- `media` - Media upload handler
- `activity-processor` - DynamoDB Streams processor

### Security:
- **ACM Certificate** - SSL/TLS for HTTPS
- **IAM Roles** - Least privilege access
- **JWT Secrets** - Encrypted in Pulumi state

## Post-Deployment Steps

1. **Verify DNS propagation**:
   ```bash
   dig lesser.host
   dig media.lesser.host
   ```

2. **Test WebFinger**:
   ```bash
   curl https://lesser.host/.well-known/webfinger?resource=acct:test@lesser.host
   ```

3. **Create first user** (via API):
   ```bash
   curl -X POST https://lesser.host/api/v1/accounts \
     -H "Content-Type: application/json" \
     -d '{
       "username": "admin",
       "email": "admin@example.com",
       "password": "secure-password",
       "agreement": true
     }'
   ```

## Updating

To update Lambda functions after code changes:

1. **Rebuild functions**:
   ```bash
   cd ..
   make build-lambdas
   ```

2. **Update infrastructure**:
   ```bash
   cd infra
   pulumi up
   ```

## Monitoring

- **CloudWatch Logs**: `/aws/lambda/lesser-*`
- **API Gateway Logs**: `/aws/apigateway/lesser`
- **DynamoDB Metrics**: AWS Console → DynamoDB → Tables → lesser-production

## Cost Estimates

For 100 active users:
- **DynamoDB**: ~$5-10/month (pay-per-request)
- **Lambda**: ~$5-10/month
- **S3**: ~$5/month (50GB storage)
- **CloudFront**: ~$5/month
- **Total**: ~$20-30/month

## Troubleshooting

### Certificate Validation Failed
- Ensure Route 53 DNS records are created
- Wait 5-10 minutes for DNS propagation
- Check ACM console for validation status

### Lambda Function Errors
```bash
# View logs
aws logs tail /aws/lambda/lesser-api --follow

# Test function
aws lambda invoke --function-name lesser-api --payload '{}' output.json
```

### DynamoDB Issues
- Check table exists: `aws dynamodb describe-table --table-name lesser-production`
- Verify GSIs are created
- Check Lambda has proper IAM permissions

## Destroy

To tear down all infrastructure:
```bash
pulumi destroy
```

**Warning**: This will delete all data! Export any important data first.

## Environment Variables

The Lambda functions use these environment variables (set automatically by Pulumi):
- `DYNAMODB_TABLE_NAME`: DynamoDB table name
- `S3_BUCKET_NAME`: Media bucket name
- `CDN_DOMAIN`: CloudFront domain for media
- `DOMAIN`: Main domain
- `JWT_SECRET`: Secret for JWT signing
- `AWS_REGION`: AWS region