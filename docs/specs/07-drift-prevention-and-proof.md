# Spec 07: Drift Prevention and Proof (Checks, Tests, Docs)

## Summary
To maintain “all 9s,” Lesser needs guardrails that prevent recurring drift between:
- `cmd/*` handlers
- `Makefile LAMBDAS` (packaging set)
- CDK deployed functions and wiring
- monitoring coverage
- docs (counts, endpoint tables, removed tooling references)

This spec defines the automated checks and minimal proof suite required to keep the product at 9/10 after Specs 01–06 are implemented.

This spec depends on:
- Specs 01–06 (inventory exists; CDK wiring and monitoring are aligned)

This spec is written to be directly actionable by PAI: it defines canonical inputs, required artifacts, and operator-run verification commands.

## Canonical Inputs (Source of Truth)
PAI must treat these as canonical, in this priority order:
1. **Product set:** `Makefile` `LAMBDAS`
2. **Inventory:** `infra/cdk/inventory/lambdas.go`
3. **CDK naming and routing:** Specs 02–04
4. **Monitoring coverage:** Spec 06 implementation in `infra/cdk/stacks/monitoring_stack.go`

## Execution Constraints
PAI must not execute commands, run tests, or deploy/synth infrastructure. PAI may add/modify code, tests, and scripts, but all verification steps are run by the operator (Codex CLI) outside PAI.

## Goals
- Prevent regressions where a Lambda exists in one place but not another.
- Provide a lightweight, repeatable “proof” that:
  - federation endpoints are reachable and correct
  - core APIs (Mastodon, GraphQL, WebSockets) are reachable
  - background processors are at least invocable and not obviously miswired
- Make it easy to run locally and in CI.

## Non-Goals
- Full production load testing as a gate.
- Comprehensive behavior tests for every Mastodon endpoint.

## Requirements
### R1 — Lambda inventory drift check (sets must match)
Add an automated check that compares **sets** (not counts) across:
1. `Makefile LAMBDAS`
2. Inventory (Spec 01)
3. CDK-created function set
4. Monitoring-covered function set

The check must fail if:
- any Lambda is missing from any set
- any extra Lambda appears in CDK/monitoring that isn’t in the product inventory

### R2 — Handler presence check
Ensure every product Lambda:
- has `cmd/<name>/main.go`
- contains a `func main()` entrypoint

Extend `scripts/verify_lambdas.sh` rather than creating redundant scripts.

### R3 — Routing proof (federation)
Add a minimal federation routing smoke suite that verifies:
- `GET /.well-known/webfinger?resource=acct:<user>@<domain>` returns a valid response
- `GET /users/<username>` returns ActivityPub JSON when `Accept: application/activity+json`
- `GET /users/<username>/followers` returns ActivityPub collection JSON
- `GET /users/<username>/following` returns ActivityPub collection JSON
- `GET /users/<username>/liked` returns ActivityPub collection JSON
- `GET /objects/<id>` returns ActivityPub JSON for a known object

### R4 — Core API proof
Add a minimal API smoke suite that verifies:
- `GET /api/v1/instance` returns a sane instance payload
- `POST /api/graphql` returns a valid response (route + auth behavior as expected)
- `GET /health` returns 200 and includes expected keys

### R5 — Background wiring proof (non-destructive)
Add non-destructive checks that prove background wiring is not obviously broken:
- SQS queues exist and event source mappings exist for SQS consumer Lambdas.
- DynamoDB stream event source mappings exist for stream processors.
- EventBridge rules exist for scheduled jobs.

Where possible, verify via:
- `cdk synth` snapshot checks (resource existence)
- optional post-deploy checks (CloudWatch metrics show invocations)

### R6 — Single command for local verification
Add a `make verify` target that runs:
- `scripts/verify_lambdas.sh`
- inventory drift check
- unit tests (`make test` or `go test -short ./...`)
- (optional) `cdk synth` in `infra/cdk` for a configured environment

### R7 — Doc drift checks (no stale tooling)
Add a lightweight doc drift check that fails on:
- stale Lambda count claims
- references to Pulumi (CDK is the only IaC path)

## Acceptance Criteria
- A single command (`make verify`) provides high-signal confidence that:
  - product Lambdas are complete and aligned
  - federation routing works end-to-end
  - core API endpoints are reachable
  - CDK can synth without drift
- Drift check failures are actionable (print missing/extra items).

## Operator Verification (Run Outside PAI)
Recommended local/CI sequence (adjust for environment/tooling availability):
- `make verify-lambda-set`
- `make verify-inventory`
- `make test` (or `go test -short ./...`)
- `make verify` (once implemented)
- `cd infra/cdk && cdk synth` (if toolchain is available)

## Open Questions
1. Should `cdk synth` be a required check in CI? (Toolchain availability may vary.)
2. What is the minimal “known object” fixture for `/objects/<id>` smoke tests in non-prod?
