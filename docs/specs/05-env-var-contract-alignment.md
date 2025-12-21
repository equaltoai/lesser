# Spec 05: Environment Variable Contract Alignment

## Summary
This document locks the canonical environment-variable contract for Lesser and captures traceability between runtime expectations, CDK provisioning, and inventory declarations. It centers on Decisions D1–D8 to align env-var names, aliases, and rationales for domain, environment selectors, DynamoDB table naming, S3 buckets, queue URLs, WebSocket endpoints, JWT secrets, and the actor private key secret.

## Canonical Inputs (Source of Truth)
Priority order for the contract:
1. Runtime config loader: `pkg/config/config.go` (canonical names and allowed aliases).
2. Runtime callers: direct `os.Getenv` outside `pkg/config` (must be reconciled to canonical names or removed).
3. Inventory declarations: `infra/cdk/inventory/lambdas.go` (`RequiredEnvVars` must list only non-baseline requirements).

## Execution Constraints
PAI does not execute commands, tests, or synth; operators run verification outside PAI.

## Canonical Env-Var Contract (Decisions D1–D8)
- **D1 Environment selectors**  
  Canonical: `ENVIRONMENT`; Alias: `STAGE` (same value).  
  Rationale: `pkg/config.GetMainTableName()` derives `lesser-<env>` from these; both must be present on every Lambda.
- **D2 Domain**  
  Canonical: `DOMAIN_NAME`; Alias: `DOMAIN` (same value).  
  Rationale: Health/config use `DOMAIN_NAME`; legacy call sites still read `DOMAIN`.
- **D3 DynamoDB table naming**  
  Canonical: `DYNAMODB_TABLE`; Alias: `DYNAMO_TABLE_NAME` (same value).  
  Rationale: Validators and inventory expect `DYNAMODB_TABLE`; some Lambdas still read the alias.
- **D4 Media bucket naming**  
  Canonical: `S3_BUCKET_NAME`; Aliases: `S3_BUCKET`, `S3_MEDIA_BUCKET`, `MEDIA_BUCKET_NAME` (all set to the same media bucket).  
  Rationale: Config loader supports these fallbacks; CDK must materialize the same value everywhere.
- **D5 JWT secret handling**  
  Canonical: `JWT_SECRET_ARN`; Alias (dev/test only): `JWT_SECRET` plaintext.  
  Rationale: Production must rely on the secret ARN; plaintext may be accepted locally but not required in AWS.
- **D6 Actor signing key**  
  Canonical: `PRIVATE_KEY_SECRET`; Optional alias: `PRIVATE_KEY_SECRET_ARN` (same value).  
  Rationale: ActivityPub signing key is required; avoid CDK-only names not read by config/health checks.
- **D7 Queue URL mapping (per-job queues, Spec 04 Q1)**  
  Canonical queue URLs: `IMPORT_QUEUE_URL`, `EXPORT_QUEUE_URL`, `MEDIA_QUEUE_URL`, `SCHEDULED_QUEUE_URL`, `FEDERATION_DELIVERY_QUEUE_URL`, `PUSH_NOTIFICATION_QUEUE_URL`.  
  Aliases (set to same URL during migration):  
  - `IMPORT_PROCESSOR_QUEUE_URL` → `IMPORT_QUEUE_URL`  
  - `EXPORT_PROCESSOR_QUEUE_URL` → `EXPORT_QUEUE_URL`  
  - `MEDIA_PROCESSOR_QUEUE_URL` → `MEDIA_QUEUE_URL`  
  - `PUSH_QUEUE_URL` → `PUSH_NOTIFICATION_QUEUE_URL` (deprecated)  
  - `FEDERATION_QUEUE_URL` → `FEDERATION_DELIVERY_QUEUE_URL` (deprecated)  
  Deprecated and not to be provisioned: `IMPORT_EXPORT_QUEUE_URL`.
  Notes:
  - `SCHEDULED_QUEUE_URL` is provisioned in CDK as a standalone queue (no event source mapping yet).
- **D8 WebSocket endpoints**  
  Canonical: `WEBSOCKET_ENDPOINT`; Alias: `WEBSOCKET_API_URL`.  
  Rationale: Only these names carry the WebSocket endpoint; do not reuse unrelated vars (e.g., `SQS_QUEUE_URL`).

## Canonical Variables and Aliases (Quick Reference)
- Environment selectors: `ENVIRONMENT` (canonical), `STAGE` (alias, same value).
- Domain: `DOMAIN_NAME` (canonical), `DOMAIN` (alias).
- DynamoDB main table: `DYNAMODB_TABLE` (canonical), `DYNAMO_TABLE_NAME` (alias).
- Media bucket: `S3_BUCKET_NAME` (canonical), aliases `S3_BUCKET`, `S3_MEDIA_BUCKET`, `MEDIA_BUCKET_NAME`.
- Queue URLs: canonical set in D7; aliases map 1:1 as listed in D7.
- WebSocket endpoints: `WEBSOCKET_ENDPOINT` (canonical), `WEBSOCKET_API_URL` (alias).
- JWT secret: `JWT_SECRET_ARN` (canonical), `JWT_SECRET` (dev/test-only alias).
- Actor signing key: `PRIVATE_KEY_SECRET` (canonical), optional alias `PRIVATE_KEY_SECRET_ARN` (same value).

## Traceability and Refactor Guidance
- **Config loader alignment:** Ensure `pkg/config/config.go` exposes and prioritizes the canonical names above; aliases are migration-only and must resolve to identical values where supported.
- **Direct `os.Getenv` usage:** Any direct reads outside config must be reconciled to the canonical names; if legacy aliases remain, document and schedule removal.
- **Inventory (`infra/cdk/inventory/lambdas.go`):** `RequiredEnvVars` should list only non-baseline vars; baseline canonical vars from D1–D8 are injected by CDK and omitted from `RequiredEnvVars`.
- **CDK baseline env injection (`infra/cdk/constructs/lambda_functions.go`):** Every Lambda receives the baseline canonical set (D1–D8). Aliases are populated only where needed for backward compatibility and must mirror canonical values.
- **Guardrail tests:** Add/maintain tests that synth the Lambda registry and assert (a) baseline canonical vars exist on every function, and (b) each `RequiredEnvVars` entry is present in the synthesized env map.
- **Deprecated alias handling:** Treat `PUSH_QUEUE_URL`, `FEDERATION_QUEUE_URL`, and any unified `IMPORT_EXPORT_QUEUE_URL` usage as deprecated; trace remaining call sites and plan removal once CDK and config consumers are migrated.
  - Runtime note: the deprecated unified import/export queue path has been removed; queueing is per-job.

## Change Log (Planned Implementation Touchpoints)
- Runtime config loader: reinforce canonical names and alias resolution for D1–D8.
- Validators: accept `JWT_SECRET_ARN` as fulfilling JWT requirements; avoid panics on missing plaintext in AWS.
- Lambda inventory: ensure `RequiredEnvVars` only lists non-baseline needs and uses canonical naming.
- CDK baseline env injection: inject canonical envs (and necessary aliases) for all Lambdas; ensure queue URL provisioning matches D7.
- Guardrail tests: enforce presence of baseline canonical vars and inventory-declared vars in synthesized environments.

## Acceptance Scope
- Canonical env vars and aliases are fully documented with rationales (D1–D8).
- Traceability from canonical names to deprecated aliases is explicit for downstream refactors.
- Touchpoints (config loader, validators, inventory, CDK env injection, guardrail tests) are identified for consuming the canonical contract.
