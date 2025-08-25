# Lesser Deployment Guide

This comprehensive guide covers the deployment of Lesser, a serverless ActivityPub implementation, from initial setup to production deployment.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Infrastructure Setup](#infrastructure-setup)
- [Configuration](#configuration)
- [Deployment Process](#deployment-process)
- [Post-Deployment Validation](#post-deployment-validation)
- [Monitoring & Maintenance](#monitoring--maintenance)
- [Troubleshooting](#troubleshooting)
- [Scaling Considerations](#scaling-considerations)

## Prerequisites

### Required Software

- **AWS CLI v2.0+**: Configure with appropriate permissions
- **Pulumi CLI v3.0+**: Infrastructure as Code tool
- **Go 1.21+**: For building Lambda functions
- **Node.js 18+**: For Pulumi TypeScript programs
- **Docker**: For local development and testing
- **k6**: For load testing (optional but recommended)

### AWS Account Requirements

- **IAM Permissions**: Full access to Lambda, DynamoDB, S3, Secrets Manager, CloudFormation
- **Service Limits**: Ensure adequate limits for Lambda concurrent executions
- **Region Selection**: Choose region based on latency and compliance requirements

### Domain and DNS

- **Custom Domain**: Registered domain name for your instance
- **DNS Control**: Ability to modify DNS records (A, CNAME, TXT)
- **SSL Certificate**: AWS Certificate Manager or external SSL certificate

## Infrastructure Setup

### Step 1: Clone and Prepare Repository

```bash
git clone https://github.com/your-org/lesser.git
cd lesser

# Install dependencies
npm install
go mod download

# Verify setup
make verify-setup
```

### Step 2: Configure AWS Environment

```bash
# Configure AWS CLI
aws configure

# Verify access
aws sts get-caller-identity

# Set AWS region
export AWS_DEFAULT_REGION=us-east-1  # Choose your preferred region
```

### Step 3: Create Pulumi Stack

```bash
# Initialize Pulumi stack
cd infra
pulumi stack init production  # or staging, development

# Set configuration
pulumi config set aws:region us-east-1
pulumi config set lesser:domain your-domain.com
pulumi config set lesser:environment production
```

## Configuration

### Environment Variables

Create a `.env.production` file with the following variables:

```bash
# === Core Configuration ===
DOMAIN_NAME=your-domain.com
ENVIRONMENT=production
SERVICE_NAME=lesser

# === AWS Configuration ===
AWS_REGION=us-east-1
DYNAMODB_TABLE=lesser-production-main
S3_BUCKET=lesser-production-media
PRIVATE_KEY_SECRET=lesser-production-activitypub-key

# === Security Configuration ===
JWT_SECRET=your-jwt-secret-here  # Generate: openssl rand -base64 32
ENCRYPTION_KEY=your-encryption-key-here  # Generate: openssl rand -base64 32

# === OAuth Configuration (Optional) ===
OAUTH_CLIENT_ID=your-oauth-client-id
OAUTH_CLIENT_SECRET=your-oauth-client-secret

# === Instance Configuration ===
INSTANCE_TITLE="Your Lesser Instance"
INSTANCE_SHORT_DESC="A personal ActivityPub server"
INSTANCE_ADMIN_EMAIL=admin@your-domain.com
REGISTRATIONS_OPEN=false
APPROVAL_REQUIRED=true
FEDERATION_ENABLED=true

# === Performance Configuration ===
MAX_STATUS_CHARS=5000
MAX_MEDIA_SIZE=10485760  # 10MB
MAX_VIDEO_SIZE=41943040  # 40MB

# === Monitoring Configuration ===
LOG_LEVEL=info
MONITORING_ENABLED=true
EMF_METRICS_ENABLED=true
XRAY_TRACING_ENABLED=false  # Enable for detailed tracing
```

### Validate Configuration

```bash
# Run configuration validator
go run cmd/validate-config/main.go

# Check for security issues
go run pkg/security/input_validation_audit.go
```

### Generate Secrets

```bash
# Generate ActivityPub keys
cd scripts
./generate-activitypub-keys.sh

# Store in AWS Secrets Manager
aws secretsmanager create-secret \
  --name lesser-production-activitypub-key \
  --description "ActivityPub signing keys for Lesser instance" \
  --secret-string file://activitypub-keys.json

# Generate other secrets
export JWT_SECRET=$(openssl rand -base64 32)
export ENCRYPTION_KEY=$(openssl rand -base64 32)
```

## Deployment Process

### Phase 1: Infrastructure Deployment

```bash
# Deploy infrastructure
cd infra
pulumi up --yes

# Verify infrastructure
pulumi stack output --json > ../deployment-outputs.json
```

### Phase 2: Build and Deploy Lambda Functions

```bash
# Build all Lambda functions
make build-lambdas

# Deploy functions
make deploy-functions

# Verify deployments
aws lambda list-functions --query 'Functions[?starts_with(FunctionName, `lesser-`)].FunctionName'
```

### Phase 3: Database Setup

```bash
# Initialize DynamoDB tables
go run cmd/db-init/main.go --environment=production

# Create initial user (optional)
go run cmd/create-admin/main.go --username=admin --email=admin@your-domain.com
```

### Phase 4: DNS Configuration

```bash
# Get API Gateway URL from deployment
API_GATEWAY_URL=$(pulumi stack output apiGatewayUrl)

# Configure DNS records
# A/CNAME record: your-domain.com -> API_GATEWAY_URL
# Example using Route53:
aws route53 change-resource-record-sets --hosted-zone-id Z1234567890 --change-batch file://dns-change.json
```

### Phase 5: SSL Certificate

```bash
# Request certificate (if using ACM)
aws acm request-certificate \
  --domain-name your-domain.com \
  --validation-method DNS \
  --region us-east-1

# Validate certificate
# Follow DNS validation instructions
```

## Post-Deployment Validation

### Health Checks

```bash
# Basic health check
curl https://your-domain.com/health

# Detailed health check
curl https://your-domain.com/health/detailed

# API functionality
curl https://your-domain.com/api/v1/instance
```

### Federation Validation

```bash
# WebFinger endpoint
curl https://your-domain.com/.well-known/webfinger?resource=acct:admin@your-domain.com

# NodeInfo endpoint
curl https://your-domain.com/.well-known/nodeinfo

# Actor endpoint
curl -H "Accept: application/activity+json" https://your-domain.com/users/admin
```

### Load Testing

```bash
# Run realistic load tests
cd tests/k6
k6 run --env BASE_URL=https://your-domain.com realistic-load.js

# Monitor during load test
aws logs tail /aws/lambda/lesser-api --follow --since 1h
```

### Security Validation

```bash
# Run security headers test
curl -I https://your-domain.com/api/v1/instance

# SSL/TLS validation
nmap --script ssl-enum-ciphers -p 443 your-domain.com

# OWASP ZAP scan (if available)
zap-baseline.py -t https://your-domain.com
```

## Monitoring & Maintenance

### CloudWatch Setup

```bash
# Create custom dashboard
aws cloudwatch put-dashboard --dashboard-name "Lesser-Production" --dashboard-body file://dashboard.json

# Set up alarms
aws cloudwatch put-metric-alarm --alarm-name "Lesser-API-Errors" --alarm-description "API error rate too high" --metric-name "Errors" --namespace "AWS/Lambda" --statistic "Sum" --period 300 --evaluation-periods 2 --threshold 10 --comparison-operator "GreaterThanThreshold"
```

### Log Management

```bash
# Configure log retention
aws logs put-retention-policy --log-group-name /aws/lambda/lesser-api --retention-in-days 30

# Set up log insights queries
aws logs start-query --log-group-name /aws/lambda/lesser-api --start-time 1634567890 --end-time 1634654290 --query-string "fields @timestamp, @message | filter @message like /ERROR/"
```

### Cost Monitoring

```bash
# Enable detailed billing
aws ce get-cost-and-usage --time-period Start=2024-01-01,End=2024-01-31 --granularity MONTHLY --metrics BlendedCost --group-by Type=DIMENSION,Key=SERVICE

# Set up cost alerts
aws budgets create-budget --account-id 123456789012 --budget file://budget.json
```

### Automated Backups

```bash
# Enable DynamoDB backups
aws dynamodb put-backup-policy --table-name lesser-production-main --backup-policy BackupEnabled=true

# Set up S3 cross-region replication
aws s3api put-bucket-replication --bucket lesser-production-media --replication-configuration file://replication.json
```

## Troubleshooting

### Common Deployment Issues

#### 1. Lambda Function Timeout

**Symptoms**: HTTP 504 errors, incomplete operations
**Diagnosis**: 
```bash
aws logs filter-log-events --log-group-name /aws/lambda/lesser-api --filter-pattern "Task timed out"
```
**Solution**: Increase Lambda timeout in Pulumi configuration

#### 2. DynamoDB Throttling

**Symptoms**: HTTP 500 errors, slow response times
**Diagnosis**:
```bash
aws dynamodb describe-table --table-name lesser-production-main --query 'Table.ProvisionedThroughput'
```
**Solution**: Enable auto-scaling or increase provisioned capacity

#### 3. Cold Start Issues

**Symptoms**: Intermittent slow responses
**Diagnosis**: Monitor Lambda initialization duration
**Solution**: Implement Lambda warming or increase memory allocation

#### 4. Certificate Issues

**Symptoms**: SSL/TLS errors, browser warnings
**Diagnosis**:
```bash
openssl s_client -connect your-domain.com:443 -servername your-domain.com
```
**Solution**: Verify DNS validation, check certificate status

### Debugging Commands

```bash
# View Lambda logs
aws logs tail /aws/lambda/lesser-api --follow --since 10m

# Check DynamoDB metrics
aws cloudwatch get-metric-statistics --namespace AWS/DynamoDB --metric-name ConsumedReadCapacityUnits --dimensions Name=TableName,Value=lesser-production-main --start-time 2024-01-01T00:00:00Z --end-time 2024-01-01T01:00:00Z --period 300 --statistics Average

# Test federation connectivity
curl -v -H "Accept: application/activity+json" https://your-domain.com/users/admin

# Validate API Gateway configuration
aws apigatewayv2 get-api --api-id YOUR_API_ID
```

### Performance Optimization

#### Lambda Optimization

```bash
# Optimize Lambda package size
cd cmd/api
go build -ldflags="-s -w" -o bootstrap main.go
upx --best bootstrap  # Optional compression

# Enable ARM64 for better price/performance
# Update Pulumi configuration to use ARM64 architecture
```

#### Database Optimization

```bash
# Analyze DynamoDB access patterns
aws dynamodb query --table-name lesser-production-main --key-condition-expression "PK = :pk" --expression-attribute-values '{":pk":{"S":"USER#admin"}}'

# Enable DynamoDB Accelerator (DAX) if needed
aws dax create-cluster --cluster-name lesser-cache --node-type dax.r4.large --replication-factor 3
```

## Scaling Considerations

### Horizontal Scaling

- **Lambda Concurrency**: Configure reserved concurrency per function
- **API Gateway**: Implement throttling and request limits
- **DynamoDB**: Use Global Secondary Indexes for efficient queries

### Vertical Scaling

- **Lambda Memory**: Optimize memory allocation based on usage patterns
- **DynamoDB Capacity**: Use auto-scaling or on-demand billing

### Multi-Region Deployment

```bash
# Deploy to additional regions
pulumi stack init production-eu-west-1
pulumi config set aws:region eu-west-1
pulumi up

# Set up Route 53 health checks and failover
aws route53 create-health-check --caller-reference $(date +%s) --health-check-config Type=HTTPS,ResourcePath=/health,FullyQualifiedDomainName=your-domain.com
```

### CDN Setup

```bash
# Configure CloudFront distribution
aws cloudfront create-distribution --distribution-config file://cloudfront-config.json

# Update DNS to point to CloudFront
aws route53 change-resource-record-sets --hosted-zone-id Z1234567890 --change-batch file://cdn-dns-change.json
```

## Security Hardening

### Network Security

```bash
# Configure WAF rules
aws wafv2 create-web-acl --name lesser-protection --scope CLOUDFRONT --default-action Allow={} --rules file://waf-rules.json

# Set up VPC endpoints for AWS services
aws ec2 create-vpc-endpoint --vpc-id vpc-12345678 --service-name com.amazonaws.us-east-1.dynamodb
```

### Access Control

```bash
# Implement API rate limiting
aws apigatewayv2 create-route --api-id YOUR_API_ID --route-key "POST /api/v1/statuses" --throttle RateLimit=100,BurstLimit=200

# Set up CloudTrail for audit logging
aws cloudtrail create-trail --name lesser-audit --s3-bucket-name lesser-audit-logs
```

### Regular Security Updates

```bash
# Schedule regular security scans
aws events put-rule --name "lesser-security-scan" --schedule-expression "rate(7 days)"

# Automated dependency updates
go get -u all
go mod tidy
```

## Maintenance Procedures

### Regular Tasks

1. **Weekly**: Review CloudWatch logs and metrics
2. **Monthly**: Analyze cost reports and optimize resources  
3. **Quarterly**: Update dependencies and security patches
4. **Annually**: Review and update disaster recovery procedures

### Update Process

```bash
# Backup current state
pulumi stack export --file backup-$(date +%Y%m%d).json

# Deploy updates
git pull origin main
make build
pulumi up

# Verify deployment
make verify-deployment

# Rollback if needed
pulumi rollback
```

### Disaster Recovery

```bash
# Create snapshots
aws dynamodb create-backup --table-name lesser-production-main --backup-name manual-backup-$(date +%Y%m%d)

# Test restore procedure
aws dynamodb restore-table-from-backup --target-table-name lesser-test --backup-arn arn:aws:dynamodb:us-east-1:123456789012:table/lesser-production-main/backup/01234567890123-12345678

# Verify restored data
go run cmd/verify-backup/main.go --table-name lesser-test
```

## Support and Resources

### Documentation Links

- [ActivityPub Specification](https://www.w3.org/TR/activitypub/)
- [AWS Lambda Documentation](https://docs.aws.amazon.com/lambda/)
- [Pulumi AWS Provider](https://www.pulumi.com/registry/packages/aws/)

### Monitoring Dashboard

Access your monitoring dashboard at:
- CloudWatch: `https://console.aws.amazon.com/cloudwatch/home?region=us-east-1#dashboards:name=Lesser-Production`
- Cost Explorer: `https://console.aws.amazon.com/cost-management/home#/dashboard`

### Community Support

- [Lesser GitHub Issues](https://github.com/your-org/lesser/issues)
- [ActivityPub Community](https://socialhub.activitypub.rocks/)
- [AWS Community Forums](https://forums.aws.amazon.com/)

---

## Quick Reference Commands

```bash
# Health check
curl https://your-domain.com/health

# View logs
aws logs tail /aws/lambda/lesser-api --follow

# Check costs
aws ce get-cost-and-usage --time-period Start=$(date -d '30 days ago' +%Y-%m-%d),End=$(date +%Y-%m-%d) --granularity MONTHLY --metrics BlendedCost

# Update deployment
make build && pulumi up

# Rollback
pulumi rollback

# Emergency stop (disable API Gateway)
aws apigatewayv2 update-stage --api-id YOUR_API_ID --stage-name prod --throttle RateLimit=0,BurstLimit=0
```

This deployment guide provides a comprehensive foundation for deploying and maintaining Lesser in production. Always test procedures in a staging environment before applying to production systems.