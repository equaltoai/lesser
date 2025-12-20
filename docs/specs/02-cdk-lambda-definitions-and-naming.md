# Spec 02: CDK Lambda Definitions and Naming (Inventory-Driven)

## Summary
CDK must define **every** product Lambda (Spec 01) with consistent naming, correct zip assets, correct roles, and consistent operational defaults. This spec covers the **function layer** only (not routes or triggers).

## Goals
- Eliminate drift between `Makefile LAMBDAS` and CDK-deployed Lambdas.
- Ensure CDK uses the correct `bin/<name>.zip` for each function.
- Standardize naming for functions and log groups to avoid collisions and stale dashboards.

## Non-Goals
- Adding/removing product Lambdas.
- Redesigning business behavior inside Lambdas.

## Requirements
### R1 — CDK creates the full product set
CDK creates all inventory Lambdas as `awslambda.Function` resources.

### R2 — Asset mapping is 1:1 with Makefile packaging
Each Lambda `<name>` must reference `../../bin/<name>.zip` from `infra/cdk`.

### R3 — Naming convention is consistent and env-qualified
Standard naming:
- Lambda function name: `lesser-<environment>-<lambda>`
- Log group name: `/aws/lambda/lesser-<environment>-<lambda>`

### R4 — Roles are explicit and consistent
Inventory specifies a role class for each Lambda (e.g., `basic`, `encryption`). CDK applies it consistently.

### R5 — Operational defaults are consistent
Inventory provides defaults (memory, timeout, log retention) with per-lambda overrides.

## Current Issues (What This Spec Fixes)
This spec originally addressed two recurring drift classes:
- **Partial coverage:** CDK defined fewer Lambdas than the `Makefile LAMBDAS`/inventory set.
- **Mis-wiring:** some constructs referenced the wrong `bin/<name>.zip` or used non-canonical names.

Both are prevented by inventory-driven generation and validated by the drift gate below.

## Implementation (Current)
- `infra/cdk/constructs/lambda_functions.go` generates **all** inventory Lambdas into `LambdaFunctions{Functions map[string]awslambda.Function}` keyed by inventory name.
- Downstream constructs reference functions via `functions.Must("<name>")` to avoid stale field mappings.
- Naming, log groups, roles, and operational defaults are applied uniformly during inventory iteration.

## Acceptance Criteria
- `cdk synth` produces one Lambda per inventory entry with correct asset and naming.
- No Lambda can accidentally point at another Lambda’s `bin/*.zip` artifact.
- Log group naming and retention are consistent across all Lambdas.

## Validation and Drift Prevention (Addendum)
- **Registry test:** `cd infra/cdk && go test ./constructs -run TestLambdaFunctionsGeneratedFromInventory -count=1` verifies every inventory lambda exists, follows `lesser-<env>-<lambda>` naming, uses `/aws/lambda/lesser-<env>-<lambda>` log groups, and stages the expected `bin/<name>.zip` asset (validated by comparing staged-asset file hash to `bin/<name>.zip` file hash).
- **Script hook:** `./scripts/verify_inventory.sh` now runs the registry test after checking Makefile vs inventory and doc freshness, failing on any mismatch.
- **Lift-aligned safeguards:** Validation enforces environment-qualified naming and explicit role assignment per inventory (mirrors Lift best practices for environment separation and least-privilege roles) and keeps infrastructure validation separate from application code.
- **How to run in CI/CD:** Invoke `./scripts/verify_inventory.sh` (or `make verify-inventory`) to gate changes; rerun `go test` locally to debug registry drift before deployment.
