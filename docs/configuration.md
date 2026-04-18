# Configuration Reference

Lesser runtime configuration is managed through:

- **Baseline environment variables** (injected by CDK; loaded via `pkg/config`)
- **Instance-owned configuration** persisted in DynamoDB for feature switches that must survive redeploys

- **Deployed stacks**: `lesser up` deploys CDK stacks that inject required environment variables into Lambda.
- **Local development**: run `./lesser dev init` to create `.env`, then `./lesser dev`.

## Deployment Inputs (Operators)

`./lesser up` requires:

- `--app <slug>`
- `--base-domain <example.com>` (must exist as a public Route53 hosted zone)
- `--aws-profile <profile>`

If a bootstrap mnemonic is generated on first deploy, `--out <path>` is required to persist it locally.

## Deploy-time integration flags

Some integration inputs are consumed by `./lesser up` and CDK synthesis rather than by the normal runtime config path.
Treat these as deployment wiring, not as ordinary instance-owned feature flags.

- `BODY_ENABLED` → deploy-runner input used by `lesser up`
- `bodyEnabled` → current CDK context key for MCP/body route wiring
- `soulEnabled` → legacy CDK context alias still accepted for backward compatibility

In current Lesser code, `soulEnabled` means “use the historical soul name for the same Lesser ↔ lesser-body MCP
wiring now controlled by `bodyEnabled`.”

Related managed integration inputs:

- `LESSER_HOST_URL`
- `LESSER_HOST_ATTESTATIONS_URL`
- `LESSER_HOST_INSTANCE_KEY_ARN`

For the repo-boundary explanation of soul, see `docs/soul.md`.

## Instance-owned configuration (persistent)

Certain feature flags and integration URLs are stored in DynamoDB under `PK="INSTANCE#CONFIG"` so they do **not**
silently disappear when a deploy runner omits an env var.

Well-known config records:

- `SK="TRUST_CONFIG"`: replaces `LESSER_HOST_URL`, `LESSER_HOST_ATTESTATIONS_URL`, `LESSER_HOST_INSTANCE_KEY_ARN`
- `SK="TRANSLATION_CONFIG"`: replaces `TRANSLATION_ENABLED`
- `SK="TIPS_CONFIG"`: replaces `TIP_ENABLED`, `TIP_CHAIN_ID`, `TIP_CONTRACT_ADDRESS`
- `SK="AI_CONFIG"`: AI feature gating and defaults
- `SK="WELL_KNOWN_LESSER_SOUL_AGENT"`: stores the current HTTPS proof value served at `GET /.well-known/lesser-soul-agent`
- `SK="SOUL_ENS_CHANNEL#<agentId>"`: stores the ENS channel name, chain, and optional resolver override published for a soul agent

### Precedence model

Effective values resolve as:

1. `override` (set by instance operator/admin)
2. `managed` (set by provisioning/deploy tooling)
3. built-in defaults

Managed updates must never clobber overrides.

### Provisioning and updates

- `lesser up --provisioning-input` writes **managed defaults** into the instance table after deploy (merge-safe; missing
  inputs do not clear existing managed fields).
- If `--provisioning-input` is not provided, legacy env vars may seed managed defaults once (deprecated). The API also
  attempts a best-effort one-time migration from env vars on cold start when records are missing.

### Deprecated env vars (bootstrap/migration only)

These env vars are still read as a temporary bootstrap path, but should not be relied on in production:

- `LESSER_HOST_URL`, `LESSER_HOST_ATTESTATIONS_URL`, `LESSER_HOST_INSTANCE_KEY_ARN` (do not persist plaintext `LESSER_HOST_INSTANCE_KEY`)
- `TRANSLATION_ENABLED`
- `TIP_ENABLED`, `TIP_CHAIN_ID`, `TIP_CONTRACT_ADDRESS`

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

### Device flow + CLI automation safety rails

Device authorization is feature-gated and disabled by default:

```bash
ALLOW_DEVICE_FLOW=false
```

When device flow is enabled, Lesser serves the hosted verification flow and allows OAuth clients with the
`device_code` grant to bootstrap headless sessions.

For `client_class=cli` clients, access tokens minted via the device grant are classified as `client_class=cli` and are
governed server-side by stricter automation limits (regardless of username):

```bash
# Max concurrent in-flight requests per CLI session (JWT `sid`)
CLI_AUTOMATION_CONCURRENCY_LIMIT=2

# Per-session throttles (token-bucket style)
CLI_AUTOMATION_BURST_LIMIT=20
CLI_AUTOMATION_BURST_WINDOW=10s
CLI_AUTOMATION_SUSTAINED_LIMIT=60
CLI_AUTOMATION_SUSTAINED_WINDOW=1m

# Error-rate circuit breaker → lockout
CLI_AUTOMATION_ERROR_RATE_THRESHOLD=0.10
CLI_AUTOMATION_ERROR_RATE_MIN_REQUESTS=10
CLI_AUTOMATION_ERROR_RATE_WINDOW=1m
CLI_AUTOMATION_LOCKOUT_DURATION=1h
```

Notes:

- These rails apply based on the token’s classification (`client_class=cli`), not on account type.
- Agent-bound device-code clients inherit `client_class=agent` semantics instead of the CLI rails.
- GraphQL depth is capped for `client_class=cli` tokens even if `GRAPHQL_MAX_DEPTH` is higher.
- Recommended rollout: enable in `dev`, validate behavior, then `staging`, then `live`.

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
