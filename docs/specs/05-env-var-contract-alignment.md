# Spec 05: Environment Variable Contract Alignment

## Summary
Lesser currently has mismatches between:
- what code expects via env vars (e.g., `pkg/config/config.go`, per-service config)
- what CDK sets on Lambdas

This spec defines a canonical env var contract, how to apply it in CDK, and how to provide safe aliases during migration.

## Goals
- Make env var names consistent across code and CDK.
- Ensure required env vars are present for every Lambda.
- Reduce “it works in one env but panics in another” failures.

## Non-Goals
- Moving all configuration to a new system; env vars remain the runtime contract.
- Introducing secrets into source control.

## Requirements
### R1 — Canonical names come from centralized config
Where a value is represented in `pkg/config/config.go`, the env var name in that loader is canonical.

### R2 — CDK sets canonical vars everywhere
CDK must set a baseline env map (domain, environment, table names, bucket names, etc.) for all Lambdas.

### R3 — Queue URLs and other per-processor vars are explicit
Each processor Lambda receives the queue/stream/schedule-specific env vars it needs, named canonically.

### R4 — Aliases are supported only as a migration bridge
If legacy names exist (e.g., `FEDERATION_QUEUE_URL` vs `FEDERATION_DELIVERY_QUEUE_URL`), code may temporarily accept aliases, but:
- the alias list is documented
- there is a planned removal step after CDK alignment

## Current Mismatches (Examples)
- `pkg/config/config.go` reads federation delivery from `FEDERATION_DELIVERY_QUEUE_URL`, but CDK currently sets `FEDERATION_QUEUE_URL`.
- `pkg/config/config.go` reads per-job queues from:
  - `IMPORT_QUEUE_URL`, `EXPORT_QUEUE_URL`, `MEDIA_QUEUE_URL`, `SCHEDULED_QUEUE_URL`
  but CDK currently does not set these queue URLs on functions.
- `pkg/config/config.go` will `panic` unless **either** `ENVIRONMENT` or `STAGE` is set (it uses these to derive the canonical DynamoDB table name). CDK must set one of these for every Lambda.

## Proposed Implementation
1. Define a canonical env var map in the inventory (Spec 01) with:
   - baseline env vars (applies to all)
   - per-lambda overrides
2. Update CDK to apply baseline + per-lambda env consistently.
3. Add validation in CDK or a build-time checker:
   - fail if a lambda lacks required env vars (per inventory)
4. Add a small runtime helper (if needed) to read canonical-or-alias names during migration.

## Acceptance Criteria
- No Lambda relies on an env var name that CDK doesn’t set.
- No undocumented env var aliases remain.
- `make dev` / non-prod defaults do not panic due to missing stage/environment.
