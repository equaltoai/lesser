# Spec 05: Environment Variable Contract Alignment

## Summary
Lesser currently has mismatches between:
- what code expects via env vars (`pkg/config/config.go`, `pkg/config/validator.go`, and any direct `os.Getenv` usage)
- what CDK sets on Lambdas (`infra/cdk/constructs/lambda_functions.go`)
- what the inventory claims is required (`infra/cdk/inventory/lambdas.go:LambdaSpec.RequiredEnvVars`)

This spec defines a canonical env var contract, how to apply it in CDK, and how to provide safe aliases during migration.

This spec is written to be directly actionable by PAI: it defines canonical inputs, explicit naming decisions, required file touch points, and acceptance gates.

## Canonical Inputs (Source of Truth)
PAI must treat these as canonical, in this priority order:
1. **Runtime config loader:** `pkg/config/config.go` (preferred names and fallback behavior).
2. **Runtime callers:** any direct `os.Getenv(...)` outside `pkg/config` (must be reconciled or refactored).
3. **Inventory declarations:** `infra/cdk/inventory/lambdas.go` (`RequiredEnvVars` is the deploy-time “must be present” list).

## Execution Constraints
PAI must not execute commands, run tests, or synthesize CDK. PAI may add/modify code and scripts, but all verification steps are run by the operator (Codex CLI) outside PAI.

## Goals
- Make env var names consistent across code and CDK.
- Ensure required env vars are present for every Lambda.
- Reduce “it works in one env but panics in another” failures.

## Non-Goals
- Moving all configuration to a new system; env vars remain the runtime contract.
- Introducing secrets into source control.

## Decisions (Lock These For Implementation)
### D1 — Always set both environment selectors
CDK must set both:
- `ENVIRONMENT` = the stack environment string (e.g., `development`, `staging`, `production`)
- `STAGE` = the same value as `ENVIRONMENT`

This is required because `pkg/config.GetMainTableName()` derives `lesser-<env>` from `ENVIRONMENT` or `STAGE`.

### D2 — Domain name: canonical is `DOMAIN_NAME`, keep `DOMAIN` as an alias
CDK must set:
- `DOMAIN_NAME` = instance domain (canonical)
- `DOMAIN` = same value (alias for older call sites)

### D3 — DynamoDB table name: canonical is `DYNAMODB_TABLE`, keep `DYNAMO_TABLE_NAME` as an alias
CDK must set:
- `DYNAMODB_TABLE` = `MainTable.TableName()` (canonical for validation tooling)
- `DYNAMO_TABLE_NAME` = same value (alias used by some Lambdas)

### D4 — Media bucket name: canonical is `S3_BUCKET_NAME`, keep aliases for existing call sites
CDK must set all of these to the same media bucket name:
- `S3_BUCKET_NAME` (canonical in `pkg/config/config.go`)
- `S3_BUCKET` (used by some validation and tooling)
- `S3_MEDIA_BUCKET` (used by some health checks)
- `MEDIA_BUCKET_NAME` (supported fallback in `pkg/config.GetS3Bucket()`)

### D5 — JWT secret: canonical is `JWT_SECRET_ARN` (do not require plaintext in CDK)
For deployed environments:
- Canonical is `JWT_SECRET_ARN` (Secrets Manager ARN or name).
- `JWT_SECRET` plaintext is allowed only for local dev/tests and must not be required for correctness in AWS.

PAI must update any production code paths that `panic`/`Fatal` when `JWT_SECRET` is unset to instead rely on `pkg/config` (which can resolve `JWT_SECRET_ARN`).

### D6 — Actor signing key: canonical is `PRIVATE_KEY_SECRET`
CDK must set:
- `PRIVATE_KEY_SECRET` = secret name used for the ActivityPub signing key (e.g., `lesser/actor-private-key`)

If CDK also exports an ARN for the same secret, it may be provided as an optional alias env var, but production code must not rely on a CDK-only name that the config loader/health checks don’t read.

### D7 — Queue URL env vars follow per-job queues (Spec 04 Q1)
Canonical queue URL env vars (from `pkg/config/config.go`) are:
- `IMPORT_QUEUE_URL`
- `EXPORT_QUEUE_URL`
- `MEDIA_QUEUE_URL`
- `SCHEDULED_QUEUE_URL`
- `FEDERATION_DELIVERY_QUEUE_URL`
- `PUSH_NOTIFICATION_QUEUE_URL`

Compatibility aliases that may be set to the same values during migration:
- `IMPORT_PROCESSOR_QUEUE_URL` → `IMPORT_QUEUE_URL`
- `EXPORT_PROCESSOR_QUEUE_URL` → `EXPORT_QUEUE_URL`
- `MEDIA_PROCESSOR_QUEUE_URL` → `MEDIA_QUEUE_URL`
- `PUSH_QUEUE_URL` → `PUSH_NOTIFICATION_QUEUE_URL` (CDK currently sets this; treat as deprecated)
- `FEDERATION_QUEUE_URL` → `FEDERATION_DELIVERY_QUEUE_URL` (treat as deprecated)

Deprecated:
- `IMPORT_EXPORT_QUEUE_URL` must not be provisioned or required (unified queue path is deprecated).

### D8 — WebSocket endpoint canonical vars
Canonical WebSocket endpoint vars are:
- `WEBSOCKET_ENDPOINT` (used by `pkg/config.Config.WebSocketEndpoint`)
- `WEBSOCKET_API_URL` (used by some Lambdas as a fallback)

PAI must remove any misuse of unrelated env vars as WebSocket endpoints (e.g., do not repurpose `SQS_QUEUE_URL`).

## Requirements
### R0 — Inventory-required vars are explicit and minimal
`infra/cdk/inventory/lambdas.go` must list only *non-baseline* required vars in `LambdaSpec.RequiredEnvVars` (baseline vars come from this spec’s Decisions).

### R1 — Canonical names come from centralized config
Where a value is represented in `pkg/config/config.go`, the env var name in that loader is canonical.

### R2 — CDK sets canonical vars everywhere
CDK must set a baseline env map (domain, environment, table names, bucket names, etc.) for all Lambdas, using the Decisions above.

### R3 — Queue URLs and other per-processor vars are explicit
Each processor Lambda receives the queue/stream/schedule-specific env vars it needs, named canonically.

### R4 — Aliases are supported only as a migration bridge
If legacy names exist (e.g., `FEDERATION_QUEUE_URL` vs `FEDERATION_DELIVERY_QUEUE_URL`), code may temporarily accept aliases, but:
- the alias list is documented
- there is a planned removal step after CDK alignment

## Current Mismatches (Examples)
- `cmd/federation-delivery/main.go` requires `FEDERATION_DELIVERY_QUEUE_URL`, but CDK often sets `FEDERATION_QUEUE_URL`.
- Health/validation expects `DOMAIN_NAME`, `DYNAMODB_TABLE`, `PRIVATE_KEY_SECRET`, but CDK frequently sets only partial/alternate names.
- Some code paths require `JWT_SECRET` even though deployment prefers `JWT_SECRET_ARN` (must be fixed).
- `cmd/note-processor/main.go` currently reuses `SQS_QUEUE_URL` as a WebSocket endpoint via `cfg.SQSQueueURL` (must be corrected to use `cfg.WebSocketEndpoint` / `WEBSOCKET_ENDPOINT`).

## Implementation (PAI Execution Checklist)
PAI should implement these steps in order.

### Step 1 — Reconcile code to the canonical contract
1. Replace direct `os.Getenv(...)` usage (outside AWS runtime metadata) with `pkg/config` where possible.
2. Where direct reads must remain, ensure the env var name matches the Decisions above.
3. Update `pkg/config/validator.go` to treat `JWT_SECRET_ARN` as satisfying JWT secret requirements in deployed environments (do not require plaintext `JWT_SECRET`).

### Step 2 — Update the inventory required-env list
Update `infra/cdk/inventory/lambdas.go`:
- Ensure `LambdaSpec.RequiredEnvVars` uses the canonical names from the Decisions.
- Remove any entries that are actually baseline (because CDK sets them for every Lambda).

### Step 3 — Update CDK to set baseline + per-lambda env vars
Update `infra/cdk/constructs/lambda_functions.go`:
- Set the baseline env vars from Decisions D1–D8 on every Lambda.
- For queue URLs, set only what is needed (or set all canonical queue URLs if that is simpler, but ensure correctness).
- Add aliases only where required for existing code paths (and document them).

### Step 4 — Add a guardrail test
Add a `go test` in `infra/cdk/constructs` that synthesizes the Lambda registry and asserts:
- every Lambda has the baseline vars (Decisions)
- every `RequiredEnvVars` entry for that Lambda exists in the synthesized env map

## Operator Verification (Run Outside PAI)
- `make verify-inventory`
- `cd infra/cdk && go test ./...` (or at minimum, the new env-contract guardrail test)

## Acceptance Criteria
- No Lambda relies on an env var name that CDK doesn’t set.
- No undocumented env var aliases remain.
- `make dev` / non-prod defaults do not panic due to missing stage/environment.
