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

## Framework-managed runtime behavior

These behaviors come from lesser's pinned Theory Cloud framework usage. Operators normally do not configure them
directly, but they are relevant during release validation and rollback planning.

- **TableTheory Lambda timeout safety**: Lambda-optimized DynamoDB clients use TableTheory's Lambda timeout handling when
  a request/event context has a deadline. The framework keeps a safety buffer before the Lambda hard timeout so storage
  operations can fail predictably instead of being cut off by the runtime. There is no operator-facing env var to disable
  this; validate timeout-sensitive changes with the full CI gate before rollout.
- **AppTheory strict route registration**: selected HTTP surfaces register already-existing routes with strict AppTheory
  helpers. Strict registration is a startup/test-time drift guard only; it does not introduce new route paths or response
  shapes.
- **AppTheory CDK functions**: selected triggerless inventory Lambdas are synthesized through AppTheory CDK constructs
  while preserving their historical CloudFormation logical IDs. Triggered/scheduled Lambdas remain on native CDK
  constructs where downstream event-source or permission parity has not yet been proven.
- **No schema toggle**: the framework baseline does not change DynamoDB PK/SK patterns, GSI usage, TableTheory model tags,
  or optimistic-concurrency versioning.

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
- `SOUL_BINDING_INTEGRATION_KEY_ARN` — ARN for the dedicated body/Ptah → Lesser server-to-server binding bearer. Required for instance-plane/body-enabled Ptah binding deploys; use the ARN-backed path and never commit or log the raw bearer value.

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

The soul-binding receiver credential is intentionally **not** an instance-owned feature flag. `lesser up` forwards
`SOUL_BINDING_INTEGRATION_KEY_ARN` into CDK context as `soulBindingIntegrationKeyArn`; CDK sets only the
`SOUL_BINDING_INTEGRATION_KEY_ARN` Lambda environment value and grants Lambda roles read access to that exact
Secrets Manager ARN. Runtime resolution remains fail-closed when neither the ARN nor a local/manual direct
`SOUL_BINDING_INTEGRATION_KEY` is present.

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

### Agent registration opt-in

Agent accounts and self-registration default to disabled. The public self-sovereign agent registration endpoints
(`POST /api/v1/agents/register/challenge` and `POST /api/v1/agents/register`) return `403` until an operator opts in.

To intentionally run an agent-enabled deployment, set both deployment inputs/env vars and redeploy:

```bash
ALLOW_AGENTS=true
ALLOW_AGENT_REGISTRATION=true
```

`lesser up` forwards those values into CDK context and seeds `SK="AGENT_CONFIG"` managed defaults when env-based
feature seeding is active. Operators can also enable agents persistently by setting `allow_agents=true` and
`allow_agent_registration=true` through the admin agent policy endpoint / GraphQL admin mutation. A persisted disabled
`AGENT_CONFIG` policy keeps self-registration disabled even if the built-in defaults would otherwise apply.

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

# Agent self-registration is secure-by-default. Intentional agent deployments
# must explicitly opt in with both env vars or persist an enabling
# AGENT_CONFIG policy through the admin agent policy endpoint.
ALLOW_AGENTS=false
ALLOW_AGENT_REGISTRATION=false

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

### Browser CORS + security headers

Browser CORS is restrictive by default for the API Lambda: only the instance origin derived from `DOMAIN_NAME` / `DOMAIN`
is allowed. Mastodon mobile clients and other non-browser clients are unaffected because CORS is enforced by browsers,
not by the server for ordinary HTTP clients.

Operators that serve a browser client from another trusted origin can opt in with a comma-separated allowlist:

```bash
# Preferred setting. Values must be origins only; paths, queries, fragments, and userinfo are ignored.
API_CORS_ALLOWED_ORIGINS="https://app.example.com,https://admin.example.com"

# Backward-compatible alias, only used when API_CORS_ALLOWED_ORIGINS is unset.
CORS_ALLOWED_ORIGINS="https://app.example.com"
```

For deployed API Gateway preflight responses, set the same value at deploy time with `lesser up
--api-cors-allowed-origins ...`, `API_CORS_ALLOWED_ORIGINS`, or managed provisioning input field
`api_cors_allowed_origins`. This keeps Lambda-handled responses and API Gateway `OPTIONS` responses on the same
allowlist.

Invalid entries are ignored fail-closed. `*` is accepted only as an explicit operator opt-in and should not be used for
production instances that expose authenticated browser flows. API responses also receive restrictive default browser
security headers (including CSP, HSTS, frame denial, MIME sniffing protection, referrer policy, permissions policy, and
cross-origin policies); handlers that need a narrower route-specific CSP set their own value and are not overwritten.

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
- GraphQL depth is capped at 4 for agent and `client_class=cli` tokens even if `GRAPHQL_MAX_DEPTH` is higher. The fixed
  complexity ceiling, parser-token limit, and pagination bounds continue to constrain broad or expensive automation
  queries.
- Recommended rollout: enable in `dev`, validate behavior, then `staging`, then `live`.

### Hardened auth + visibility rollout semantics

The generated REST and GraphQL contracts document the same hardened defaults that operators should validate during
rollout:

- OAuth bearer authentication is required for write APIs and for non-public GraphQL fields. The anonymous GraphQL
  public-read subset only exposes public/unlisted content.
- `direct` status creation remains 1:1 in v1: the request content must contain exactly one resolvable local or remote
  `@mention`. Lesser serializes the resolved actor into ActivityPub addressing (`to`/`cc`/`bto`/`bcc`) and those
  addressing fields are the authoritative source for repair/backfill tooling.
- Content mentions alone are not authorization. Operational tools that repair DM conversations must use stored
  recipient fields and must not infer participants from message text.
- New deployments remain locked-on-deploy until the operator unlocks the instance. Validate auth, public visibility,
  direct-message visibility, VAPID push signing, stream retry behavior, and cost/date-range reporting in `dev` before
  promoting to `staging` or `live`.
- Lambda-optimized TableTheory clients retain a timeout safety buffer before applying Lambda context deadlines. If you
  adjust timeout behavior, re-run the full local gate before deploying.

Detailed operator notes live in `docs/security/hardened-auth-visibility-rollout.md`.

### Crawler protection

```bash
# Mode toggle (used by multiple HTTP Lambdas)
CRAWLER_PROTECTION_MODE=off # off|observe|limit|block

# Emergency bypass (skip block + rate limiting for matching client IPs)
CRAWLER_PROTECTION_BYPASS_CIDRS="" # comma-separated CIDRs or IPs

# Forwarded client IP trust roots. X-Forwarded-For is ignored unless the
# trusted AWS request source IP is in one of these proxy CIDRs; leave empty to
# use the AWS source IP directly and fail closed on spoofed forwarding headers.
CRAWLER_TRUSTED_PROXY_CIDRS="" # comma-separated CIDRs or IPs

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
