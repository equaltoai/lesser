# Deployment Guide

This guide covers deploying Lesser to AWS using CDK.

## Prerequisites

### Required Tools
- AWS CLI configured with credentials
- AWS CDK v2: `npm install -g aws-cdk`
- Go 1.24+
- Make

### AWS Account Setup
- Ensure you have appropriate IAM permissions
- Note your AWS account ID and preferred region

## Quick Deployment

### 1. Build Lambda Functions

```bash
# From project root
make build-lambdas

# Verify binaries exist
ls -la bin/
```

### 2. Bootstrap CDK

First-time CDK setup in your AWS account:

```bash
cd infra/cdk
cdk bootstrap aws://YOUR-ACCOUNT-ID/us-east-1
```

### 3. Deploy to Development

```bash
# Deploy all stacks with default settings
cdk deploy --all

# Or deploy with custom domain
cdk deploy --all --context domain=dev.yourdomain.com
```

## Production Deployment

### 1. Obtain SSL Certificate

For production, you need an ACM certificate:

```bash
# Request certificate via AWS Console or CLI
aws acm request-certificate \
  --domain-name yourdomain.com \
  --subject-alternative-names "*.yourdomain.com" \
  --validation-method DNS
```

### 2. Configure Production Settings

Review and adjust `infra/cdk/config/prod.yaml`:

```yaml
environment: production
appName: lesser-prod
domain: yourdomain.com
memorySize: 3008
timeout: 30
logLevel: INFO
features:
  enableMonitoring: true
  enableDeletionProtection: true
  enablePointInTimeRecovery: true
```

### 3. Deploy Production Stack

```bash
cdk deploy --all \
  --context environment=production \
  --context domain=yourdomain.com \
  --context certificateArn=arn:aws:acm:us-east-1:xxx:certificate/xxx \
  --context jwtSecret=your-very-secure-secret \
  --require-approval broadening
```

## Environment Configuration

### Development
- Memory: 512MB
- Logging: DEBUG
- Monitoring: Basic
- Cost: Minimal

### Staging
- Memory: 1024MB
- Logging: INFO
- Monitoring: Detailed
- Reserved Concurrency: 10

### Production
- Memory: 3008MB (ARM64)
- Logging: INFO
- Monitoring: Comprehensive
- Reserved Concurrency: 50
- Deletion Protection: Enabled

## Post-Deployment

### 1. Verify Deployment

```bash
# Check API health
curl https://yourdomain.com/health

# Verify federation endpoint
curl https://yourdomain.com/.well-known/webfinger?resource=acct:admin@yourdomain.com
```

### 2. Create Admin User

```bash
# Use the API to create first admin user
curl -X POST https://yourdomain.com/api/v1/accounts \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "email": "admin@yourdomain.com"}'
```

### 3. Configure Instance

Set instance metadata via environment variables or API:

```bash
INSTANCE_TITLE="My Lesser Instance"
INSTANCE_ADMIN_EMAIL="admin@yourdomain.com"
FEDERATION_ENABLED=true
REGISTRATIONS_OPEN=false
```

## Stack Management

### List Stacks
```bash
cdk list
```

### Show Differences
```bash
cdk diff --context environment=production
```

### Update Stack
```bash
cdk deploy LesserStack-production --context environment=production
```

### Delete Stack
```bash
# BE CAREFUL - This deletes all data
cdk destroy --all --context environment=development
```

## Monitoring Setup

After deployment, access CloudWatch dashboard:

```
https://console.aws.amazon.com/cloudwatch/home?region=us-east-1#dashboards:name=lesser-{environment}
```

Set up email alerts:
```bash
ALERT_EMAIL=ops@yourdomain.com cdk deploy --all
```

## Cost Controls

### Set Budget Alerts
```bash
aws budgets create-budget \
  --account-id YOUR-ACCOUNT-ID \
  --budget file://budget.json
```

### Monitor Costs
Check the cost aggregator Lambda logs:
```bash
aws logs tail /aws/lambda/lesser-production-cost-aggregator --follow
```

## Troubleshooting

### CDK Bootstrap Issues
```bash
# Clear CDK context and retry
rm -rf cdk.context.json
cdk bootstrap --force
```

### Build Failures
```bash
# Ensure correct architecture
GOOS=linux GOARCH=arm64 go build
```

### Certificate Issues
- Ensure certificate is in us-east-1 for CloudFront
- Verify DNS validation records are in place

### Domain Not Working
- Check Route53 hosted zone
- Verify CloudFront distribution status
- Allow up to 15 minutes for propagation

## Next Steps

- [Configuration Reference](configuration.md) - Customize your instance
- [Monitoring Guide](monitoring.md) - Set up comprehensive monitoring
- [Federation Guide](federation.md) - Connect to the Fediverse
