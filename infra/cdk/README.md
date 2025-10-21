# Lesser CDK Infrastructure

This directory contains the AWS CDK (Cloud Development Kit) code for deploying the Lesser serverless application using the Lift framework patterns.

## Architecture

The CDK implementation creates the following AWS resources:

- **23 Lambda Functions**: Core application logic for API, federation, and processing
- **DynamoDB Table**: Single table design with 8 GSIs and DynamoDB Streams enabled
- **S3 Bucket**: Media storage with CloudFront CDN
- **API Gateway**: HTTP API with custom domain support
- **DynamoDB Stream Processors**: Event-driven processing for activities, federation, and notifications
- **CloudWatch**: Monitoring dashboards and alarms
- **WAF**: Web application firewall for rate limiting (without VPC)
- **KMS**: Encryption key for secrets

## Prerequisites

- AWS CDK v2 installed (`npm install -g aws-cdk`)
- Go 1.24 or later
- AWS credentials configured (`aws configure`)
- Built Lambda function binaries in `../../bin/` directory

## Directory Structure

```
infra/cdk/
├── main.go                    # CDK app entry point
├── stacks/
│   ├── lesser_stack.go        # Main application stack
│   ├── shared_stack.go        # Shared resources (KMS, secrets)
│   └── monitoring_stack.go    # Monitoring and observability
├── constructs/
│   ├── lambda_functions.go    # Lambda function definitions
│   ├── api_routes.go         # API Gateway route configurations
│   └── stream_processors.go  # DynamoDB stream processors
└── config/
    ├── development.yaml      # Development environment config
    ├── staging.yaml          # Staging environment config
    └── production.yaml       # Production environment config
```

## Deployment

### Bootstrap CDK (First Time Only)

Bootstrap CDK in your AWS account and region:

```bash
# Bootstrap CDK with proper permissions for cross-account deployments
cdk bootstrap aws://YOUR-ACCOUNT-ID/us-east-1

# For custom bucket name (optional)
cdk bootstrap aws://YOUR-ACCOUNT-ID/us-east-1 --toolkit-bucket-name my-cdk-bootstrap-bucket
```

### Build Lambda Functions

Before deploying, ensure all Lambda functions are built:

```bash
cd ../.. # Go to project root
make build-lambdas

# Verify all binaries exist
ls -la bin/
# Should see all 23 Lambda function binaries
```

### Environment-Specific Deployments

#### Development Environment

```bash
# Deploy all stacks to development
cdk deploy --all

# Deploy with custom domain
cdk deploy --all --context domain=my-dev.example.com

# Deploy with alert email
ALERT_EMAIL=alerts@example.com cdk deploy --all
```

#### Staging Environment

```bash
# Deploy to staging with proper context
cdk deploy --all \
  --context environment=staging \
  --context domain=staging.lesser.app \
  --require-approval broadening

# With custom settings
ALERT_EMAIL=staging-alerts@example.com cdk deploy --all \
  --context environment=staging \
  --context domain=staging.lesser.app
```

#### Production Environment

**CRITICAL**: Production requires JWT secret and certificate ARN:

```bash
# First, create/import SSL certificate in ACM
aws acm list-certificates --region us-east-1

# Deploy to production with all required context
cdk deploy --all \
  --context environment=production \
  --context domain=lesser.app \
  --context certificateArn=arn:aws:acm:us-east-1:ACCOUNT:certificate/CERT-ID \
  --context jwtSecret=your-super-secure-jwt-secret \
  --require-approval broadening

# With alert email and custom region
ALERT_EMAIL=prod-alerts@example.com cdk deploy --all \
  --context environment=production \
  --context domain=lesser.app \
  --context certificateArn=arn:aws:acm:us-east-1:ACCOUNT:certificate/CERT-ID \
  --context jwtSecret=your-super-secure-jwt-secret \
  --context region=us-west-2
```

### Stack-Specific Deployments

```bash
# Deploy only shared resources (KMS, secrets)
cdk deploy LesserSharedStack

# Deploy only monitoring stack for development
cdk deploy LesserMonitoringStack-development

# Deploy only application stack for staging
cdk deploy LesserApiStack-staging \
  --context environment=staging \
  --context domain=staging.lesser.app
```

### Makefile Workflows

For day-to-day work, prefer the `make deploy-*` targets. Each deployment command:

```bash
make deploy-dev      # development
make deploy-test     # staging
make deploy-live     # production
```

- Rebuilds Lambda packages as needed
- Ensures the CDN private key secret and CloudFront public key/key group exist (via `scripts/ensure_cdn_credentials.sh`)
- Passes the resulting secret name and key pair ID into CDK context before deployment

This keeps the CloudFront credentials managed outside CloudFormation so stacks can be destroyed and redeployed reliably.

### Essential CDK Commands

```bash
# Show CloudFormation template without deploying
cdk synth

# Show deployment differences 
cdk diff

# Show differences for specific environment
cdk diff --context environment=production

# List all stacks
cdk list

# Show stack resources
cdk ls LesserApiStack-production

# Destroy development stack (CAREFUL!)
cdk destroy LesserApiStack-development --context environment=development

# Force destroy with dependency removal
cdk destroy --all --context environment=development --force
```

## Environment Configuration

Each environment has its own configuration in `config/*.yaml`:

### Development (`config/development.yaml`)
- **Memory**: 512MB RAM 
- **Timeout**: 30 seconds
- **Log Level**: DEBUG
- **Features**: No deletion protection, basic monitoring
- **Cost**: Optimized for minimal usage

### Staging (`config/staging.yaml`)
- **Memory**: 1024MB RAM
- **Timeout**: 30 seconds  
- **Log Level**: INFO
- **Features**: Deletion protection enabled, detailed monitoring
- **Cost**: Optimized with auto-scaling

### Production (`config/production.yaml`)
- **Memory**: 3008MB RAM (ARM64 optimized)
- **Timeout**: 30 seconds
- **Log Level**: INFO
- **Features**: Full deletion protection, comprehensive monitoring
- **Advanced Monitoring**: Error rate thresholds, latency monitoring

### Required Context Variables

| Variable | Development | Staging | Production | Description |
|----------|-------------|---------|------------|-------------|
| `environment` | development | staging | production | Environment name |
| `domain` | Optional | Required | Required | Custom domain |
| `certificateArn` | Not required | Optional | **Required** | ACM certificate ARN |
| `jwtSecret` | Not required | Optional | **Required** | JWT signing secret |
| `region` | Optional | Optional | Optional | AWS region (default: us-east-1) |

### Environment Variables

| Variable | Description | Required |
|----------|-------------|-----------|
| `ALERT_EMAIL` | Email for CloudWatch alarms | Optional |
| `CDK_DEFAULT_ACCOUNT` | AWS account ID | Optional |
| `CDK_DEFAULT_REGION` | AWS region | Optional |

## Cost Optimization

The infrastructure is optimized for cost:

- No VPC (saves ~$45/month)
- DynamoDB pay-per-request billing
- ARM64 Lambda functions (20% cheaper)
- S3 lifecycle policies for media
- CloudFront caching to reduce origin requests

## Monitoring

### Comprehensive CloudWatch Dashboard

Each environment creates a dedicated CloudWatch dashboard with:

#### Lambda Function Metrics (per function)
- **Invocations & Errors**: Request volume and error counts
- **Duration & Throttles**: Performance and capacity metrics
- **Concurrent Executions**: Real-time concurrency tracking
- **Iterator Age**: Stream processing lag (for stream processors)

#### API Gateway Metrics
- **4XX Errors**: Client-side errors (rate limits, bad requests)
- **5XX Errors**: Server-side errors (Lambda failures, timeouts)
- **Request Count**: API traffic volume
- **Latency**: Response time distribution

#### DynamoDB Metrics
- **Read/Write Capacity**: Consumed capacity units over time
- **Throttled Requests**: Read and write throttling events
- **GSI Metrics**: Capacity usage for all 8 Global Secondary Indexes

### Production-Ready Alarms

#### Lambda Alarms (per function)
- **Error Rate**: > 5% error rate over 2 evaluation periods
- **Duration**: Average > 10 seconds over 3 periods
- **Throttles**: Any throttling events (immediate alert)
- **Iterator Age**: > 1 minute for stream processors (data lag)

#### API Gateway Alarms
- **5XX Errors**: > 10 errors over 2 periods (critical system issues)

#### DynamoDB Alarms
- **Read Throttles**: Any read throttling (capacity issue)
- **Write Throttles**: Any write throttling (capacity issue)

### EventBridge Scheduled Rules

#### Cost Aggregation (Production Pattern)
- **Schedule**: Every 1 hour (`rate(1 hour)`)
- **Target**: Cost aggregator Lambda function
- **Retry Policy**: 2 attempts, 1-hour max age
- **Purpose**: Track and aggregate DynamoDB costs

#### Trend Aggregation (Production Pattern)  
- **Schedule**: Every 15 minutes (`rate(15 minutes)`)
- **Target**: Trend aggregator Lambda function
- **Retry Policy**: 2 attempts, 30-minute max age
- **Purpose**: Calculate trending hashtags and content

### Log Groups & Retention

#### API Gateway Logs
- **Path**: `/aws/apigateway/lesser-{environment}`
- **Retention**: 7 days (dev), 30 days (prod)

#### Lambda Function Logs (per function)
- **Path**: `/aws/lambda/lesser-{environment}-{function-name}`
- **Retention**: 7 days (dev), 30 days (prod)
- **Functions**: All 23 Lambda functions have dedicated log groups

### Alert Configuration

Set the `ALERT_EMAIL` environment variable to receive notifications:

```bash
# Deploy with email alerts
ALERT_EMAIL=alerts@yourcompany.com cdk deploy --all
```

Alerts are sent via SNS topic for:
- Lambda function errors and performance issues
- API Gateway server errors
- DynamoDB throttling events
- Stream processing delays

### Dashboard Access

After deployment, access your dashboard at:
```
https://console.aws.amazon.com/cloudwatch/home?region=us-east-1#dashboards:name=lesser-{environment}
```

Replace `{environment}` with your deployed environment (development, staging, production).

## Security

- WAF for rate limiting without VPC
- KMS encryption for secrets
- S3 bucket encryption
- No public S3 access
- IAM least privilege policies
- TLS in transit

## Customization

To customize the deployment:

1. Edit environment configs in `config/*.yaml`
2. Modify context values in `cdk.json`
3. Override with CLI: `cdk deploy --context key=value`

## Troubleshooting

### CDK Bootstrap Issues

#### Not Bootstrapped
```bash
# Error: Need to perform AWS CDK bootstrap
cdk bootstrap aws://YOUR-ACCOUNT-ID/us-east-1

# For specific regions
cdk bootstrap aws://YOUR-ACCOUNT-ID/us-west-2
```

#### Bootstrap Permissions
```bash
# If bootstrap fails with permissions
aws sts get-caller-identity  # Verify correct credentials

# Required permissions: 
# - CloudFormation full access
# - S3 bucket creation
# - IAM role creation
```

### Build and Compilation Issues

#### Go Module Issues
```bash
go mod tidy
go mod download
go mod verify

# Clear module cache if needed
go clean -modcache
```

#### Lambda Build Issues
```bash
# Ensure building for Linux ARM64 (matching config)
cd cmd/api
GOOS=linux GOARCH=arm64 go build -o ../../bin/api main.go

# Build all functions at once
cd ../.. # Project root
make build-lambdas

# Check binary architecture
file bin/api  # Should show: ARM aarch64
```

#### Missing Dependencies
```bash
# Install CDK dependencies
npm install -g aws-cdk@latest

# Verify Go version (1.24+ required)
go version

# Install required Go modules
go get gopkg.in/yaml.v2
```

### Deployment Issues

#### Context Variable Errors
```bash
# Production deployment missing required context
cdk deploy --all \
  --context environment=production \
  --context domain=your-domain.com \
  --context certificateArn=arn:aws:acm:region:account:certificate/id \
  --context jwtSecret=your-jwt-secret
```

#### Certificate Issues
```bash
# List available certificates
aws acm list-certificates --region us-east-1

# Import certificate if needed
aws acm import-certificate \
  --certificate fileb://certificate.pem \
  --private-key fileb://private-key.pem \
  --certificate-chain fileb://certificate-chain.pem
```

#### Domain Configuration
```bash
# Verify domain ownership for ACM
aws acm describe-certificate --certificate-arn YOUR-CERT-ARN

# Check DNS settings
nslookup your-domain.com
dig your-domain.com
```

### Runtime Issues

#### Lambda Function Errors
```bash
# Check CloudWatch logs
aws logs describe-log-groups --log-group-name-prefix "/aws/lambda/lesser"

# Tail logs for specific function
aws logs tail /aws/lambda/lesser-production-api --follow

# Check function configuration
aws lambda get-function --function-name lesser-production-api
```

#### DynamoDB Issues
```bash
# Check table status
aws dynamodb describe-table --table-name lesser-production

# Monitor capacity metrics
aws cloudwatch get-metric-statistics \
  --namespace AWS/DynamoDB \
  --metric-name ConsumedReadCapacityUnits \
  --dimensions Name=TableName,Value=lesser-production \
  --start-time 2024-01-01T00:00:00Z \
  --end-time 2024-01-01T23:59:59Z \
  --period 3600 \
  --statistics Sum
```

#### API Gateway Issues
```bash
# Test API endpoints
curl -v https://api.your-domain.com/health

# Check API Gateway logs
aws logs tail /aws/apigateway/lesser-production --follow

# Verify custom domain mapping
aws apigatewayv2 get-domain-names
```

### Stack Dependencies

#### Dependency Conflicts
```bash
# Deploy stacks in correct order
cdk deploy LesserSharedStack
cdk deploy LesserMonitoringStack-production
cdk deploy LesserApiStack-production

# Force dependency resolution
cdk deploy --all --context environment=production --force
```

#### Stack Deletion Issues
```bash
# Check stack dependencies before deletion
cdk list --context environment=development

# Delete in reverse order
cdk destroy LesserApiStack-development
cdk destroy LesserMonitoringStack-development  
cdk destroy LesserSharedStack  # Only if no other environments
```

### Performance Issues

#### Cold Start Optimization
- ARM64 functions reduce cold start by 20%
- Memory optimization in config files
- Provisioned concurrency for critical functions

#### DynamoDB Throttling
```bash
# Check throttle metrics
aws cloudwatch get-metric-statistics \
  --namespace AWS/DynamoDB \
  --metric-name UserReadThrottledRequests \
  --dimensions Name=TableName,Value=lesser-production
  
# Consider switching to provisioned capacity for high traffic
```

### Cost Monitoring Issues

#### Cost Tracking Verification
```bash
# Check if cost aggregation is running
aws events list-rules --name-prefix lesser-production-cost

# Monitor cost aggregator function
aws logs tail /aws/lambda/lesser-production-cost-aggregator --follow
```

### Security Issues

#### KMS Key Issues
```bash
# Check KMS key permissions
aws kms describe-key --key-id alias/lesser-shared-key

# Verify key usage permissions
aws kms list-grants --key-id alias/lesser-shared-key
```

#### Secrets Manager Issues
```bash
# Check secret creation
aws secretsmanager list-secrets --filters Key=name,Values=lesser

# Test secret retrieval
aws secretsmanager get-secret-value --secret-id lesser-production-private-key
```

### Common Error Solutions

| Error | Solution |
|-------|----------|
| `ModuleNotFoundError` | Run `go mod tidy && go mod download` |
| `Bootstrap required` | Run `cdk bootstrap aws://account/region` |  
| `Certificate not found` | Import certificate to ACM in correct region |
| `Domain validation failed` | Verify DNS records for domain validation |
| `JWT secret required` | Add `--context jwtSecret=xxx` for production |
| `Lambda timeout` | Increase timeout in environment config |
| `DynamoDB throttling` | Monitor capacity and consider provisioned mode |
| `S3 access denied` | Check bucket policies and IAM permissions |
