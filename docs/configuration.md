# Configuration Reference

Lesser configuration is managed through environment variables, CDK context, and YAML configuration files.

## Environment Variables

### Instance Configuration

```bash
# Basic Instance Settings
INSTANCE_TITLE="My Lesser Instance"
INSTANCE_SHORT_DESC="A federated social network"
INSTANCE_DESCRIPTION="Detailed description of your instance"
INSTANCE_ADMIN_EMAIL="admin@yourdomain.com"
INSTANCE_LANGUAGES="en,es,fr"  # Comma-separated language codes

# Feature Flags
FEDERATION_ENABLED=true
REGISTRATIONS_OPEN=false
APPROVAL_REQUIRED=true
INVITES_ENABLED=false

# Limits
MAX_STATUS_CHARS=5000
MAX_MEDIA_SIZE=10485760      # 10MB in bytes
MAX_VIDEO_SIZE=41943040      # 40MB in bytes
MAX_POLL_OPTIONS=4
MAX_POLL_OPTION_CHARS=50

# Security
REQUIRE_EMAIL_VERIFICATION=false  # Lesser is email-free by default
ENABLE_RATE_LIMITING=true
ENABLE_CONTENT_FILTERING=true
```

### AWS Configuration

```bash
# AWS Settings (usually auto-detected)
AWS_REGION=us-east-1
AWS_ACCOUNT_ID=123456789012

# Optional AWS Overrides
DYNAMODB_TABLE_NAME=lesser-main
S3_MEDIA_BUCKET=lesser-media
SQS_FEDERATION_QUEUE=lesser-federation
```

### Feature Configuration

```bash
# AI Features (Optional)
ENABLE_AI_SEARCH=false
BEDROCK_MODEL_ID=anthropic.claude-v2
AI_MODERATION_THRESHOLD=0.8

# Cost Management
ENABLE_COST_TRACKING=true
MONTHLY_BUDGET_USD=100
BUDGET_ALERT_THRESHOLD=0.8
COST_PER_USER_TARGET=0.10

# Monitoring
ENABLE_DETAILED_METRICS=true
ENABLE_XRAY_TRACING=false
LOG_LEVEL=INFO  # DEBUG, INFO, WARN, ERROR
```

## CDK Configuration Files

### Environment Configs

Located in `infra/cdk/config/`:

#### dev.yaml
```yaml
environment: development
appName: lesser-dev
domain: dev.lesser.local
memorySize: 512
timeout: 30
logLevel: DEBUG
features:
  enableMultiTenant: false
  enableRateLimiting: true
  enableMonitoring: false
  enableDeletionProtection: false
aws:
  region: us-east-1
  architecture: arm64
  runtime: provided.al2
```

#### staging.yaml
```yaml
environment: staging
appName: lesser-staging
domain: staging.lesser.app
memorySize: 1024
timeout: 30
logLevel: INFO
features:
  enableMultiTenant: false
  enableRateLimiting: true
  enableMonitoring: true
  enableDeletionProtection: true
monitoring:
  detailedMetrics: true
  errorRateThreshold: 0.05
cost:
  optimized: true
  reservedConcurrency: 10
```

#### prod.yaml
```yaml
environment: production
appName: lesser-prod
domain: lesser.app
memorySize: 3008
timeout: 30
logLevel: INFO
features:
  enableMultiTenant: true
  enableRateLimiting: true
  enableMonitoring: true
  enableDeletionProtection: true
  enablePointInTimeRecovery: true
monitoring:
  detailedMetrics: true
  businessMetrics: true
  realTimeStreaming: true
  errorRateThreshold: 0.01
  latencyP99Threshold: 2000
cost:
  optimized: true
  reservedConcurrency: 50
```

## CDK Context Variables

### Deployment Context

```bash
# Required for production
cdk deploy --context environment=production \
           --context domain=yourdomain.com \
           --context certificateArn=arn:aws:acm:... \
           --context jwtSecret=your-secret

# Optional context
--context region=us-west-2
--context alertEmail=ops@yourdomain.com
--context enableWAF=true
--context enableBackups=true
```

### Available Context Keys

| Key | Description | Required | Default |
|-----|-------------|----------|---------|
| environment | Deployment environment | Yes | development |
| domain | Instance domain | Prod: Yes | - |
| certificateArn | ACM certificate ARN | Prod: Yes | - |
| jwtSecret | JWT signing secret | Prod: Yes | - |
| region | AWS region | No | us-east-1 |
| alertEmail | CloudWatch alert email | No | - |
| enableWAF | Enable AWS WAF | No | false |
| enableBackups | Enable automated backups | No | false |

## Multi-Tenant Configuration

### Tenant Resolution

```bash
# Tenant resolution order (first match wins)
TENANT_RESOLUTION_HEADER=X-Tenant-ID     # HTTP header
TENANT_RESOLUTION_SUBDOMAIN=true         # Extract from subdomain
TENANT_RESOLUTION_PATH=true              # Extract from URL path
TENANT_RESOLUTION_JWT=true               # Extract from JWT claims
```

### Per-Tenant Settings

```json
{
  "tenant1": {
    "domain": "tenant1.lesser.app",
    "features": {
      "federationEnabled": true,
      "registrationsOpen": false
    },
    "limits": {
      "maxUsers": 1000,
      "maxStorageGB": 100
    }
  }
}
```

## Federation Configuration

### Instance Policies

```bash
# Federation Modes
FEDERATION_MODE=open          # open, allowlist, blocklist, closed

# Federation Limits
MAX_FEDERATION_RETRY_COUNT=5
FEDERATION_TIMEOUT_SECONDS=30
FEDERATION_USER_AGENT="Lesser/1.0"

# Instance Blocks (comma-separated)
BLOCKED_INSTANCES="spam.instance,bad.actor"
ALLOWED_INSTANCES="trusted.friend,good.neighbor"
```

### Relay Configuration

```bash
# Optional Relay Support
RELAY_ENABLED=false
RELAY_INBOX_URL="https://relay.example.com/inbox"
RELAY_FOLLOW_BACK=true
```

## Rate Limiting

### API Rate Limits

```bash
# Requests per minute by endpoint
RATE_LIMIT_STATUSES_CREATE=30
RATE_LIMIT_MEDIA_UPLOAD=10
RATE_LIMIT_SEARCH=60
RATE_LIMIT_DEFAULT=300

# Federation rate limits
RATE_LIMIT_FEDERATION_INCOMING=100
RATE_LIMIT_FEDERATION_OUTGOING=50
```

### WAF Configuration

```bash
# AWS WAF settings
WAF_ENABLED=true
WAF_BLOCK_THRESHOLD=2000  # Requests per 5 minutes
WAF_RATE_BASED_RULE=true
WAF_GEO_BLOCKING=""        # Comma-separated country codes
```

## Storage Configuration

### DynamoDB Settings

```bash
# Billing mode
DYNAMODB_BILLING_MODE=PAY_PER_REQUEST  # or PROVISIONED

# If PROVISIONED:
DYNAMODB_READ_CAPACITY=5
DYNAMODB_WRITE_CAPACITY=5
DYNAMODB_AUTOSCALING_ENABLED=true
DYNAMODB_AUTOSCALING_TARGET=70  # Target utilization %
```

### S3 Configuration

```bash
# Media storage
S3_MEDIA_PREFIX="media/"
S3_MEDIA_MAX_SIZE_MB=40
S3_MEDIA_ALLOWED_TYPES="image/jpeg,image/png,image/gif,video/mp4"

# Lifecycle policies
S3_LIFECYCLE_ENABLED=true
S3_LIFECYCLE_ARCHIVE_DAYS=90
S3_LIFECYCLE_DELETE_DAYS=365
```

## Monitoring Configuration

### CloudWatch Settings

```bash
# Metrics
METRICS_NAMESPACE="Lesser"
METRICS_DETAILED=true
METRICS_CUSTOM_DIMENSIONS="Environment,Tenant"

# Alarms
ALARM_ERROR_THRESHOLD=10        # Errors per 5 minutes
ALARM_LATENCY_THRESHOLD=2000    # Milliseconds
ALARM_THROTTLE_THRESHOLD=5      # Throttles per minute
```

### Logging

```bash
# Log retention
LOG_RETENTION_DAYS=7   # Development
LOG_RETENTION_DAYS=30  # Production

# Log sampling
LOG_SAMPLING_RATE=0.1  # Sample 10% of requests
```

## Security Configuration

### Authentication

```bash
# JWT Settings
JWT_ALGORITHM=RS256
JWT_EXPIRY_HOURS=24
JWT_REFRESH_EXPIRY_DAYS=30

# OAuth Settings
OAUTH_ENABLED=true
OAUTH_PROVIDERS="github,google"
OAUTH_CALLBACK_URL="https://yourdomain.com/oauth/callback"

# WebAuthn
WEBAUTHN_ENABLED=true
WEBAUTHN_RP_NAME="Lesser Instance"
WEBAUTHN_RP_ID="yourdomain.com"
```

### Encryption

```bash
# KMS Settings
KMS_KEY_ALIAS="alias/lesser"
ENCRYPT_SENSITIVE_DATA=true
ENCRYPT_BACKUPS=true
```

## Performance Tuning

### Lambda Configuration

```bash
# Memory allocation (MB)
LAMBDA_MEMORY_API=512
LAMBDA_MEMORY_FEDERATION=256
LAMBDA_MEMORY_MEDIA=1024
LAMBDA_MEMORY_AI=2048

# Provisioned concurrency
LAMBDA_PROVISIONED_API=5
LAMBDA_PROVISIONED_FEDERATION=2
```

### Caching

```bash
# CloudFront caching
CDN_CACHE_DEFAULT_TTL=3600      # 1 hour
CDN_CACHE_MAX_TTL=86400         # 1 day
CDN_CACHE_MEDIA_TTL=31536000    # 1 year

# Application caching
CACHE_USER_PROFILES=true
CACHE_TTL_SECONDS=300
```
