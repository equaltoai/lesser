# Acceptance Rubric: “All 9’s” (Quality, Consistency, Completeness)

This rubric defines what “9/10” means for Lesser across **quality**, **consistency**, and **completeness**. It is designed to be objective and verifiable via repository checks, `cdk synth`, and post-deploy smoke signals.

## Scoring Model
- **9/10**: all **Hard Gates** pass, and there are no open **P0/P1** gaps; remaining issues are documented and bounded (P2+), with no systemic drift.
- **10/10**: 9/10 plus all **Soft Gates** pass and remaining work is polish only (no architectural ambiguity).

## Evidence Artifacts (Required for 9/10)
- `docs/specs/01-lambda-inventory-matrix.md` is the canonical product inventory.
- `scripts/verify_lambdas.sh` (and any drift checks added in Spec 07) pass.
- `cdk synth` succeeds for at least one environment config with no missing references.
- Minimal smoke suite exists (Spec 07) and is runnable against non-prod.

## Hard Gates (Must Pass for 9/10)

### Completeness — Hard Gates
1. **Full product deployed**: every Lambda in `Makefile` `LAMBDAS` exists as a CDK function and is packaged to `bin/<name>.zip`.
2. **Trigger coverage**: each Lambda has the correct trigger wiring (API Gateway route, SQS event source mapping, DynamoDB stream mapping, EventBridge schedule, etc.) as defined in the inventory.
3. **Routing correctness**:
   - Every API Gateway route in CDK maps to an implemented handler and matches how the handler registers routes (Lift route patterns vs API Gateway templates).
   - Federation endpoints (`/users/...`, `/objects/...`, WebFinger) are reachable.
4. **Environment contracts**: all required env vars are set by CDK using canonical names (or explicitly documented aliases), and the runtime config loader does not panic in non-prod defaults.

### Consistency — Hard Gates
1. **Single source of truth**: Lambda names and existence are consistent across:
   - `cmd/<lambda>/main.go`
   - `Makefile LAMBDAS`
   - `docs/specs/01-lambda-inventory-matrix.md`
   - CDK function definitions
2. **No legacy IaC**: non-CDK IaC tooling is not referenced anywhere in code/docs/scripts; CDK is the only deploy path.
3. **Naming conventions**: CDK logical IDs, function names, log group names, alarms, and dashboards follow a consistent scheme and do not include stale/phantom functions.
4. **Documentation is not misleading**: docs do not claim outdated Lambda counts, endpoints, or tooling.

### Quality — Hard Gates
1. **Build + unit tests**: `make build-lambdas` and `make test` succeed.
2. **Static checks**: `make fmt` produces no diff and `make lint` succeeds (or lint exceptions are narrowly scoped and justified).
3. **Operational safety**:
   - SQS consumers have DLQs and sane retry/backoff behavior.
   - Stream processors are not scheduled-invoked (and vice versa).
   - IAM policies are least-privilege by default (broad `*` access is documented and tracked).

## Soft Gates (Strongly Preferred for 9/10; Required for 10/10)

### Completeness — Soft Gates
- Post-deploy “proof” suite covers at least one representative path per trigger type:
  - API (REST + federation)
  - Stream processors
  - SQS processors
  - Schedules
- Backfill / bootstrap tools are documented and runnable in non-prod.

### Consistency — Soft Gates
- Inventory-driven generation: CDK uses a single inventory list to generate functions, routes, schedules, stream mappings, monitoring targets, and env var sets.
- Repo-wide doc drift check exists (Spec 07) to prevent reintroducing stale claims.

### Quality — Soft Gates
- Monitoring baseline:
  - CloudWatch dashboards exist and include key lambdas/queues/streams.
  - Alarms exist for DLQ depth, error rates, and throttles.
- Structured logs include request IDs and stable service names across all API lambdas.

## Severity Definitions
- **P0**: production outage / cannot deploy / data loss risk / security vulnerability.
- **P1**: major missing product component, repeated drift, or broken core pathway.
- **P2**: non-critical missing monitoring/alerts, minor wiring cleanup, or documentation gaps.
- **P3**: polish and nice-to-haves.
