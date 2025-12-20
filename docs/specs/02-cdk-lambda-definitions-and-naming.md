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
The current CDK implementation in `infra/cdk/constructs/lambda_functions.go`:
- defines **22** Lambda functions, while the product set is **36** (`Makefile LAMBDAS`)
- contains mis-mapped fields where the semantic variable name does not match the deployed function asset/name, e.g.:
  - `NotificationProcessor` is created as `"push-delivery"` using `../../bin/push-delivery.zip`
  - `SearchIndexerFunction` is created as `"status-indexer"` using `../../bin/status-indexer.zip`
  - `HealthFunction` is created as `"federation-tracker"` using `../../bin/federation-tracker.zip`
- omits several product Lambdas entirely (examples: `actor`, `collections`, `objects`, `export-generator`, `metrics-processor`, `trend-aggregator`-adjacent scheduled processors, etc.)

This class of drift is prevented by inventory-driven generation.

## Proposed Implementation
1. Replace the bespoke `LambdaFunctions` struct “one field per function” pattern with:
   - `map[string]awslambda.Function` keyed by inventory name
2. Provide small typed helpers (optional):
   - `func (f *Functions) Must(name string) awslambda.Function`
3. Centralize function creation:
   - `createFunction(name, roleClass, propsOverrides, envOverrides)`
4. Generate all functions from inventory and return the `Functions` map.

## Acceptance Criteria
- `cdk synth` produces one Lambda per inventory entry with correct asset and naming.
- No Lambda can accidentally point at another Lambda’s `bin/*.zip` artifact.
- Log group naming and retention are consistent across all Lambdas.
