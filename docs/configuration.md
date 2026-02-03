# Configuration Reference

Lesser runtime configuration is primarily managed through environment variables (loaded via `pkg/config`).

- **Deployed stacks**: `lesser up` deploys CDK stacks that inject required environment variables into Lambda.
- **Local development**: run `./lesser dev init` to create `.env`, then `./lesser dev`.

## Deployment Inputs (Operators)

`./lesser up` requires:

- `--app <slug>`
- `--base-domain <example.com>` (must exist as a public Route53 hosted zone)
- `--aws-profile <profile>`

If a bootstrap mnemonic is generated on first deploy, `--out <path>` is required to persist it locally.

## Runtime Environment Variables

### Baseline (set by CDK in deployed stacks)

These are injected by infrastructure. You typically only set them manually for local development/testing.

- `ENVIRONMENT` (canonical: `development|staging|production`)
- `STAGE` (canonical: `dev|staging|live`)
- `APP_NAME`
- `DOMAIN_NAME` (alias: `DOMAIN`)
- `DYNAMODB_TABLE` (alias: `DYNAMO_TABLE_NAME`)
- `S3_BUCKET_NAME` (aliases: `S3_BUCKET`, `S3_MEDIA_BUCKET`, `MEDIA_BUCKET_NAME`)
- `PRIVATE_KEY_SECRET` (alias: `PRIVATE_KEY_SECRET_ARN`)
- `JWT_SECRET_ARN` (or `JWT_SECRET` for local dev)
- `WEBSOCKET_ENDPOINT` (alias: `WEBSOCKET_API_URL`)
- Queue URLs:
  - `IMPORT_QUEUE_URL` (alias: `IMPORT_PROCESSOR_QUEUE_URL`)
  - `EXPORT_QUEUE_URL` (alias: `EXPORT_PROCESSOR_QUEUE_URL`)
  - `MEDIA_QUEUE_URL` (alias: `MEDIA_PROCESSOR_QUEUE_URL`)
  - `SCHEDULED_QUEUE_URL`
  - `FEDERATION_DELIVERY_QUEUE_URL` (alias: `FEDERATION_QUEUE_URL`)
  - `PUSH_NOTIFICATION_QUEUE_URL` (alias: `PUSH_QUEUE_URL`)
- Optional:
  - `STREAM_EVENTS_TABLE_NAME` (SSE stream event log table, if deployed)

### Core (local development)

`./lesser dev init` writes a default `.env` with:

```bash
DOMAIN=localhost
INSTANCE_NAME="Lesser Dev"
AWS_REGION=us-east-1
DYNAMO_TABLE_NAME=lesser-dev
S3_BUCKET_NAME=lesser-dev-media
JWT_SECRET=... # random base64
```

Additional common local settings:

```bash
# Local DynamoDB (when using `./lesser dev dynamodb`)
DYNAMODB_ENDPOINT=http://localhost:8000
```

### Instance Metadata + Feature Flags

These control instance “about” metadata and basic product switches.

```bash
# Instance metadata
INSTANCE_NAME="Lesser ActivityPub Server"
INSTANCE_TITLE="Lesser Instance"
INSTANCE_SHORT_DESC="A personal ActivityPub server"
INSTANCE_DESCRIPTION="A lightweight, serverless ActivityPub implementation"
INSTANCE_ADMIN_EMAIL="admin@example.com"
INSTANCE_LANGUAGES="en" # comma-separated

# Instance mode
INSTANCE_MODE=hybrid # social|cms|hybrid

# Registration + federation switches
ALLOW_REGISTRATION=false
REGISTRATIONS_OPEN=false
APPROVAL_REQUIRED=true
INVITES_ENABLED=false
FEDERATION_ENABLED=true

# Limits
MAX_STATUS_CHARS=5000
MAX_MEDIA_SIZE=10485760  # bytes
MAX_VIDEO_SIZE=41943040  # bytes
MAX_UPLOAD_SIZE=10485760 # bytes
PAGE_SIZE=20
```

### Operational Toggles

```bash
# Logging / debugging
LOG_LEVEL=info # debug|info|warn|error
DEBUG=false

# Disable switches (true disables the feature)
DISABLE_METRICS=false
DISABLE_COST_TRACKING=false
DISABLE_RATE_LIMITING=false
DISABLE_FEDERATION_RATE_LIMITING=false
DISABLE_AI=false

# Observability
MONITORING_ENABLED=true
EMF_METRICS_ENABLED=true
XRAY_TRACING_ENABLED=false
ENABLE_PLAYGROUND=false
TRANSLATION_ENABLED=false

# GraphQL abuse-resilience (limits + gating)
# Introspection is disabled by default; it is enabled when DEBUG=true, ENABLE_PLAYGROUND=true,
# or GRAPHQL_ALLOW_INTROSPECTION=true.
GRAPHQL_ALLOW_INTROSPECTION=false
GRAPHQL_MAX_DEPTH=12
GRAPHQL_MAX_COMPLEXITY=500
GRAPHQL_PARSER_TOKEN_LIMIT=15000
GRAPHQL_REQUEST_TIMEOUT=25s
```

### Crawler protection

```bash
# Mode toggle (used by multiple HTTP Lambdas)
CRAWLER_PROTECTION_MODE=off # off|observe|limit|block

# Emergency bypass (skip block + rate limiting for matching client IPs)
CRAWLER_PROTECTION_BYPASS_CIDRS="" # comma-separated CIDRs or IPs

# Hard-block toggle (only relevant when CRAWLER_PROTECTION_MODE=block)
CRAWLER_BLOCK_AI_CRAWLERS=true

# Per-hour limits (used when mode=limit or mode=block)
CRAWLER_LIMIT_SEARCH_ENGINE_PER_HOUR=100
CRAWLER_LIMIT_GENERIC_BOT_PER_HOUR=30
CRAWLER_LIMIT_SUSPICIOUS_PER_HOUR=10

# Optional: extend the known AI crawler UA substrings list (comma-separated)
CRAWLER_AI_UA_PATTERNS_EXTRA=""

# Optional: emit crawler enforcement EMF metrics
CRAWLER_METRICS_ENABLED=true
```

### Moderation + ML

```bash
# AWS moderation
DISABLE_AWS_MODERATION=false
DISABLE_COMPREHEND=false
DISABLE_REKOGNITION=false
MODERATION_MODE="" # optional

# Bedrock + ML moderation
BEDROCK_MODEL_ID="anthropic.claude-3-haiku-20240307-v1:0"
BEDROCK_TRAINING_REGION=us-east-1
BEDROCK_INFERENCE_MODEL_ID=""
BEDROCK_GUARDRAIL_ID=""
BEDROCK_GUARDRAIL_VERSION="DRAFT"
BEDROCK_CUSTOMIZATION_ROLE_ARN=""
MODERATION_ML_ENABLED=false
MODERATION_ML_TENANTS="tenant-a,tenant-b" # comma-separated allow list
MODERATION_TRAINING_BUCKET_NAME=""
MODERATION_MODEL_METADATA_TABLE=""
```

### Media Streaming + CloudFront

```bash
MEDIA_SOURCE_BUCKET_NAME=""
MEDIA_STREAMING_BUCKET_NAME=""
MEDIA_CONVERT_ENDPOINT=""
MEDIA_CONVERT_ROLE_ARN=""
CLOUDFRONT_DOMAIN=""
CLOUDFRONT_KEY_PAIR_ID=""
CLOUDFRONT_PRIVATE_KEY_PATH=""
MANIFEST_TTL_HOURS=24
```

### Notifications + DLQ

```bash
PUSH_NOTIFICATION_TOPIC_ARN=""
NOTIFICATION_RETRY_QUEUE_URL=""
NOTIFICATION_DLQ_URL=""

DLQ_ENABLED=false
DLQ_MAX_RETRIES=3
DLQ_RETRY_DELAY=60
DLQ_FAIL_FAST=false
DLQ_PERMANENT_ERRORS=""
DLQ_TRANSIENT_ERRORS=""
```

### Privacy Hashing

```bash
ENABLE_PRIVACY_HASHING=false
PRIVACY_MASTER_KEY="" # required when privacy hashing is enabled

IP_PRIVACY_LEVEL=partial       # none|partial|full
EMAIL_PRIVACY_LEVEL=partial    # none|partial|full
USERNAME_PRIVACY_LEVEL=full    # none|partial|full
PII_PRIVACY_LEVEL=full         # none|partial|full
GENERIC_PRIVACY_LEVEL=full     # none|partial|full

KEY_ROTATION_ENABLED=false
KEY_ROTATION_INTERVAL=24h

ARGON2_MEMORY=65536
ARGON2_TIME=3
ARGON2_THREADS=4
ARGON2_KEY_LENGTH=32
```

## Secrets Manager Payloads

`JWT_SECRET_ARN` is expected to point at a Secrets Manager secret with JSON payload:

```json
{ "secret": "..." }
```

## CDK Context Variables (Infra Contributors)

If you run CDK directly (not recommended for operators), the CDK app reads:

- `app` (default: `lesser`)
- `baseDomain` (required for stage stacks)
- `hostedZoneId` (recommended; otherwise CDK will do a hosted zone lookup)
- `stage` (optional): `shared|dev|staging|live|all`
- `withStaging` (optional): `true` to include staging when deploying all stages

Example:

```bash
cd infra/cdk
AWS_PROFILE=Penny cdk deploy --all \
  --require-approval never \
  --context app=my-lesser \
  --context baseDomain=example.com \
  --context hostedZoneId=Z1234567890
```
