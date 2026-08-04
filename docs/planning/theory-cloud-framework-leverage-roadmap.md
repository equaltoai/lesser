# Theory Cloud Framework Leverage Roadmap

Status: Phase 4 docs closeout in progress; Phases 0 through 3b are merged to `main`; framework pins refreshed on 2026-05-18
Current maintenance focus: 2026-05-18 AppTheory / FaceTheory pin refresh.

## Reported need

Aron asked whether there are new Theory Cloud framework updates lesser should consider implementing after updating the
framework pins. He then authorized the full workflow: investigate deeply, enumerate changes, plan a roadmap, create a
GitHub Project, email `arch@lessersoul.ai`, and iterate milestone-by-milestone with Arch review and approval before
moving to the next milestone. Aron additionally authorized work requested by Arch on 2026-05-16.

## Implementation status

| Phase | Issue / PR | Status | Landed outcome |
| --- | --- | --- | --- |
| Phase 0 dependency/security baseline + 2026-05-18 pin refresh | #968 / #976 + maintenance refresh | Merged / in progress | Initial AppTheory `v1.6.0` and FaceTheory `v3.1.2` guidance refreshed to AppTheory `v1.7.0`, TableTheory remains `v1.8.3`, FaceTheory guidance refreshed to `v3.2.2`; auth UI dependency remediation and CDK asset-root canonicalization remain intact. |
| Phase 1a TableTheory Lambda client semantics | #969 / #977 | Merged | `NewLambdaOptimizedClient` now uses TableTheory Lambda-optimized timeout behavior with tests proving context/deadline handling. |
| Phase 1b AppTheory invocation-context threading | #970 / #978 | Merged | Import-processor SQS handling now scopes TableTheory repository access to the AppTheory event context deadline via `WithLambdaTimeout`; the cold-start client remains initialized once and is rebound per invocation. |
| Phase 2a strict route proof | #971 / #979 | Merged | WebFinger route registration uses AppTheory strict registration with route-parity tests. |
| Phase 2b strict route expansion | #972 / #980 | Merged | Objects/status object route registration uses AppTheory strict registration with route-parity tests. |
| Phase 3a CDK parity proof | #973 / #981 | Merged | Test-only proof compares a representative Lambda synthesized natively and through AppTheoryFunction. |
| Phase 3b CDK adoption | #974 / #982 | Merged | Triggerless inventory Lambdas use AppTheoryFunction with historical Lambda logical IDs preserved; triggered/scheduled/SSR functions remain native where parity is not yet proven. |
| Phase 4 release/docs closeout | #975 | In progress | Operator docs and release checklist are being updated to describe landed behavior, validation gates, rollback, and non-goals. |

## Investigation note

### Dimensions

- Surface: dependency management, Lambda runtime usage, TableTheory client wrappers, CDK infrastructure constructs,
  FaceTheory client install guidance.
- Lambda(s): broadly all Lambdas through shared runtime/CDK patterns; no new Lambda is proposed.
- Consumer class: operators running lesser instances; managed consumers (`lesser-host`) indirectly through release
  artifacts; FaceTheory client app authors through docs/guidance.
- Instance: repository-level work only; no deployment performed.
- Actor / activity context: none directly. This is not a federation payload/signature change.

### Upstream framework facts verified

- On 2026-05-18, the latest stable AppTheory release/tag verified for this investigation was `v1.7.0`.
  - Relevant capabilities: AppSync Lambda resolver support, strict route registration helpers, multi-language CDK/jsii
    constructs including `AppTheoryFunction`, `AppTheoryFunctionAlarms`, `AppTheoryQueueProcessor`,
    `AppTheorySsrSite`, `AppTheoryPathRoutedFrontend`, and existing Rest API / queue / table constructs.
- On 2026-05-18, the latest stable TableTheory release/tag verified for this investigation was `v1.8.3`.
  - Relevant capabilities: Lambda timeout configuration and hardening, encrypted batch retry fixes, audit/security guard
    hardening, pointer `omitempty` preservation.
- On 2026-05-18, the latest stable FaceTheory release verified for this investigation was `v3.2.2`.
  - Relevant capabilities: dependency pin alignment and Svelte vulnerable peer range exclusions.
- On 2026-05-18, no newer stable framework release had been identified beyond that dated AppTheory / TableTheory /
  FaceTheory snapshot. These are historical findings; lesser's current pins and client guidance have since advanced as
  listed below.

### What is definitely true in lesser today

- post-refresh pins:
  - root `go.mod`: AppTheory `v2.0.1`, TableTheory `v2.0.5`;
  - `infra/cdk/go.mod`: AppTheory `cdk-go` `v0.0.0-20260708194537-63e44cc6b4fc`, AWS CDK Go `v2.261.0`,
    jsii runtime `v1.133.0`;
  - FaceTheory client-install docs recommend the `v4.0.1` GitHub release asset; client-install validation/tests require
    dependency presence but do not enforce an exact version or URL;
  - auth UI: Astro `7.1.0`, Svelte `5.55.7`, devalue override `5.8.1`.
- The baseline already fixed one CDK/jsii surfaced issue by canonicalizing `LambdaAssetRoot` to an absolute path
  before `Code_FromAsset`.
- `pkg/storage/theorydb.GetLambdaClient(ctx)` uses `tabletheory.NewLambdaOptimized()` and applies
  `WithLambdaTimeoutConfig` / `WithLambdaTimeout(ctx)`.
- `pkg/storage/theorydb.NewLambdaOptimizedClient(ctx, region)` now uses TableTheory Lambda-optimized behavior and applies
  the supplied context's timeout semantics, with tests covering deadline and local-endpoint behavior.
- AppTheory `Context` and `EventContext` expose `.Context() context.Context`, and `EventContext` carries
  `RemainingMS`. The import-processor now scopes per-invocation TableTheory repository work to the event context
  deadline via `WithLambdaTimeout`; other Lambda families should continue to move one bounded slice at a time.
- lesser already uses several AppTheory CDK constructs: Dynamo tables, Rest API router, queues, queue consumers,
  EventBridge handlers, media CDN, Lambda roles, KMS keys, stream mappings. Lambda functions themselves are still
  created directly with AWS CDK `awslambda.NewFunction` where they have triggers or unproven downstream construct-path
  behavior. Triggerless inventory Lambdas with proven parity now use AppTheory `AppTheoryFunction`.
- The generated/verified Lambda inventory is still consistent after the baseline bump; `./lesser verify ci` passed.

### Specialist elevation check

- Federation trust: not directly touched. Framework leverage is reliability/deploy-maintenance work. Any later change to
  federation-delivery/inbox/outbox behavior remains subject to `protect-federation-trust` if it alters signing,
  verification, actor identity, delivery semantics, moderation gates, or blocking behavior. Current proposal does not.
- Mastodon/API contract: not touched. No REST/GraphQL/ActivityPub response shape changes are proposed. Strict route
  registration must be validated against the OpenAPI/static route contract before landing.
- Schema: not touched. No PK/SK/GSI/TableTheory tag/model shape change is proposed. TableTheory client wiring changes
  do not modify stored item shape.
- Framework consumption: touched. The current findings point to local idiomatic-consumption opportunities, not a
  confirmed framework gap. No framework patch or fork is proposed.
- Deploy/CDK: touched for CDK construct usage and synthesis validation; deploy itself remains out of scope until
  `deploy-instance` after merge.

### Fix-locus verdict

The right next work is in lesser: consume the already-released frameworks more idiomatically and verify the behavior. No
upstream framework change is required before starting. If AppTheory `AppTheoryFunction` cannot preserve lesser's current
Lambda alias/version/logging/DLQ/asset semantics without workaround, that becomes a framework-feedback signal before
local adoption proceeds.

### Ranked hypotheses

1. **TableTheory timeout support is under-leveraged in Lambda entrypoints.**
   - Evidence: `GetLambdaClient(ctx)` uses TableTheory Lambda timeout APIs; `NewLambdaOptimizedClient(ctx, region)` does
     not. Numerous Lambda init paths use the latter.
   - Verification: add targeted tests around `NewLambdaOptimizedClient` and update a bounded set of entrypoints to pass
     `ctx.Context()` where available; verify no behavior/schema change.
2. **AppTheory strict route registration can catch route contract drift earlier.**
   - Evidence: AppTheory exposes `GetStrict`, `PostStrict`, `HandleStrict`, etc.; lesser currently uses non-strict route
     registration across major HTTP surfaces.
   - Verification: introduce a local registration wrapper and targeted route-registration test for one surface before
     expanding.
3. **AppTheory CDK `AppTheoryFunction` / alarms may reduce custom Lambda construct drift.**
   - Evidence: v1.7.0 publishes Go jsii bindings for `AppTheoryFunction` and `AppTheoryFunctionAlarms`; lesser still
     creates functions directly with native AWS CDK.
   - Verification: synth a representative function behind tests and compare generated CloudFormation for function name,
     architecture, runtime, timeout, memory, role, env, asset path, DLQ, and permissions before broader migration.
4. **FaceTheory v4.0.1 is best consumed as guidance/provenance, not a hard client-app gate inside lesser.**
   - Evidence: `lesser client install` validates that a FaceTheory dependency exists; enforcing an exact dependency URL
     would break local workspace/client development.
   - Verification: docs/tests are enough in lesser; any client-app enforcement belongs in client repos or a non-blocking
     verifier.

### Verification step

Arch completed Tier 1 scope review on 2026-05-16 and found the Phase 0 -> Phase 1 -> Phase 2 -> Phase 3
sequence structurally sound, with the constraints below. Phase 0 PR review remains separate from scope approval; no
further implementation starts until Arch approves the current milestone and the next milestone sequence.

### Arch-required roadmap constraints

Arch's Tier 1 scope review approved the overall sequencing only with these constraints incorporated:

- **Split Phase 1 into separate PRs/milestones.** First prove `NewLambdaOptimizedClient` semantics and Lambda
  timeout-buffer application with tests. Later PRs thread invocation contexts by bounded Lambda family; do not convert
  every call site in one PR.
- **Prove timeout behavior at the operation boundary.** Repository methods already call `db.WithContext(ctx)`, so tests
  must show whether the configured TableTheory Lambda timeout buffer survives repository operations or whether lesser
  needs a small helper/wrapper that applies `WithLambdaTimeout(ctx)` where available.
- **Start Phase 2 on a low-risk route surface.** Do not begin strict-route adoption with all of `cmd/api/routes.go`;
  first prove AppTheory route dialect/canonicalization and route parity on a small surface.
- **Add a route inventory/parity artifact for Phase 2 validation.** OpenAPI/GraphQL checks alone are not enough because
  some ActivityPub and special endpoints are outside the Mastodon OpenAPI surface.
- **Keep CDK parity proof and adoption separate.** Phase 3a proves `AppTheoryFunction` / related construct parity; Phase
  3b adoption starts only after explicit Arch review of the synth/template diff. If parity requires workaround, stop and
  route `coordinate-framework-feedback` instead of migrating broadly.

## Scoped Need: Consume current Theory Cloud frameworks idiomatically

### Background

lesser is the flagship application example for Theory Cloud. After bumping AppTheory/TableTheory/FaceTheory pins, we
should not stop at version numbers: we should identify released framework capabilities that make lesser safer, more
reliable, or more canonical without weakening federation trust, breaking API contracts, or changing schema.

### Driver

Aron-direct framework-maintenance request, with Arch coordination explicitly authorized.

### Problem

lesser has historical compatibility layers and direct CDK/runtime patterns that predate the latest framework releases.
Some are now redundant or under-leverage released framework behavior, especially TableTheory Lambda timeout handling and
AppTheory strict runtime/CDK helpers.

### Surface affected

- Dependencies: `go.mod`, `go.sum`, `infra/cdk/go.mod`, `infra/cdk/go.sum`, `auth-ui/package.json`,
  `auth-ui/pnpm-lock.yaml`.
- TableTheory storage wrappers: `pkg/storage/theorydb/` and Lambda call sites that create DB clients.
- AppTheory HTTP/event registration: `cmd/api`, `cmd/graphql`, `cmd/sse`, ActivityPub endpoint Lambdas, and event
  processor entrypoints, bounded milestone-by-milestone.
- AppTheory CDK: `infra/cdk/constructs/`, `infra/cdk/stacks/`.
- FaceTheory docs/tests: `docs/guides/CLIENT_APP_GUIDE.md`, `cmd/lesser/client_install_test.go`.

### Classification

Dependency-maintenance, operational-reliability, framework-consumption, docs. Potential CDK/deploy reliability work.

### Narrowest-scope proposal

1. Land the already-validated dependency/security baseline.
2. Make TableTheory Lambda timeout/context usage consistent where lesser already uses TableTheory Lambda helpers.
3. Adopt AppTheory strict route registration wrappers gradually, with contract/static verification at each surface.
4. Prototype AppTheory CDK function/alarms construct parity before migrating any production Lambda construct.
5. Keep FaceTheory exact release pin guidance in docs/tests, not as a breaking runtime/client-install requirement.

### What this need explicitly does not cover

- No schema changes: no PK/SK/GSI/TableTheory tag edits, no migrations.
- No Mastodon REST, GraphQL, ActivityPub actor/object shape changes.
- No federation signing/verification behavior changes.
- No AppSync migration for lesser GraphQL.
- No replacement of the Lambda-per-concern architecture.
- No framework patches, vendoring, or unreleased commit pins.
- No deployment to any stage.

### Success criteria

- Framework pins and Dependabot remediation are merged with full `./lesser verify ci` evidence.
- TableTheory Lambda client wrappers have tests proving timeout-aware behavior and context propagation.
- A bounded set of Lambda entrypoints pass AppTheory `Context` / `EventContext` context into DB work where available.
- Strict AppTheory route registration is adopted for selected surfaces without OpenAPI/GraphQL/federation route drift.
- CDK function construct adoption either lands with template-parity proof or produces a framework-feedback signal instead
  of a local workaround.
- FaceTheory client guidance remains backward-compatible as release pins advance (the current recommendation is
  `v4.0.1`); lesser continues to require dependency presence without enforcing an exact version or URL.

### Specialist routing

- Federation trust: not touched initially; run `protect-federation-trust` if a later milestone changes federation
  behavior, delivery semantics, signature verification, actor identity, or moderation gates.
- API / contract: route strict-registration milestones through `preserve-mastodon-api-compat` if route shapes or error
  behavior change; otherwise verify with static OpenAPI/schema checks.
- Schema: not touched; run `validate-schema` if any TableTheory model tags, keys, or GSIs are proposed.
- Framework consumption: idiomatic; route through `coordinate-framework-feedback` only if AppTheory CDK/function parity
  requires a workaround.
- Deploy / CLI: deploy not included; `deploy-instance` after merge only.
- Advisor brief: n/a; Aron-direct authorization.

### Consumer impact

- Operators: safer dependency baseline and improved Lambda timeout/deploy reliability.
- Mastodon clients: no contract changes intended.
- Remote ActivityPub peers: no behavior changes intended.
- Sibling repos: `host` should be aware of any eventual release artifact/deploy behavior change; none in baseline.
- Framework stewards: Arch review requested; framework-feedback only if CDK/function parity gap is proven.

### AGPL posture

No proprietary blobs and no AGPL-incompatible dependencies proposed. Dependency changes remain open-source framework and
security-maintenance updates.

## Enumerated changes

### 1. Land framework and security dependency baseline

- **Paths**: `go.mod`, `go.sum`, `infra/cdk/go.mod`, `infra/cdk/go.sum`, `auth-ui/package.json`,
  `auth-ui/pnpm-lock.yaml`, `cmd/lesser/client_install_test.go`, `docs/guides/CLIENT_APP_GUIDE.md`,
  `app-theory/migrate-lift-roadmap.md`, `infra/cdk/constructs/lambda_functions.go`.
- **Surface**: deps, infra/cdk, docs, tests.
- **Classification**: dependency-maintenance, security, framework-consumption, operational-reliability.
- **Federation-trust impact**: none.
- **Contract impact**: none.
- **Schema impact**: none.
- **Framework consumption**: idiomatic; pins released versions and fixes CDK asset-root canonicalization surfaced by the
  new jsii/CDK stack.
- **Acceptance**: all pins are updated, open auth-ui Dependabot alerts are remediated, and full local CI passes.
- **Validation**: `go test ./...`, `cd infra/cdk && go test ./...`, `go build -o lesser ./cmd/lesser`,
  `./lesser build lambdas`, `cd auth-ui && corepack pnpm install --frozen-lockfile`,
  `cd auth-ui && corepack pnpm audit --prod`, `cd auth-ui && corepack pnpm build`, `./lesser verify ci`.
- **Conventional Commit subject**: `chore(deps): update theory cloud framework pins`.

### 2. Make TableTheory Lambda client construction timeout-aware everywhere

- **Paths**: `pkg/storage/theorydb/base.go`, `pkg/storage/theorydb/client.go`, tests under
  `pkg/storage/theorydb/*_test.go`.
- **Surface**: storage, operational-reliability.
- **Classification**: framework-consumption, operational-reliability, test-coverage.
- **Federation-trust impact**: none.
- **Contract impact**: none.
- **Schema impact**: none.
- **Framework consumption**: idiomatic use of TableTheory `LambdaDB` timeout APIs.
- **Acceptance**: `NewLambdaOptimizedClient(ctx, region)` no longer ignores context and no longer creates a standard DB
  when Lambda-optimized behavior is requested; tests cover nil context, context with deadline, env region fallback, local
  endpoint behavior, converter registration, and timeout buffer application. Tests must prove timeout behavior at the
  repository/operation boundary, not just construction time.
- **Validation**: `go test ./pkg/storage/theorydb`, `go test ./pkg/storage/...`, `go test ./...`, `go vet ./...`,
  `gofmt -l .`, `./lesser verify ci`.
- **Conventional Commit subject**: `fix(storage): make lambda table clients timeout aware`.

### 3. Thread AppTheory request/event contexts into DB initialization call sites

- **Paths**: bounded Lambda-family slices that currently call `NewLambdaOptimizedClient(context.Background(), ...)` or
  equivalent inside request/event handling. Candidate slices include HTTP/discovery surfaces (`cmd/graphql`,
  `cmd/graphql-ws`, `cmd/sse`, `cmd/streaming`, `cmd/actor`, `cmd/objects`, `cmd/collections`, `cmd/webfinger`) and
  async/federation processors (`cmd/search-indexer`, `cmd/federation-*`, `cmd/cms-scheduler`,
  `cmd/trend-aggregator`, `cmd/enhanced-federation-processor`, `cmd/import-processor`).
- **Surface**: cmd, storage.
- **Classification**: operational-reliability, framework-consumption.
- **Federation-trust impact**: none if behavior is limited to context/deadline propagation. If federation-delivery or
  inbox behavior changes, run `protect-federation-trust` before implementation.
- **Contract impact**: none.
- **Schema impact**: none.
- **Framework consumption**: idiomatic AppTheory `Context.Context()` / `EventContext.Context()` use.
- **Acceptance**: each PR targets one bounded Lambda family, passes the real AppTheory context where available, tests cover
  deadline propagation, and no `context.Background()` remains in that targeted runtime path. Broad all-call-site conversion
  is explicitly out of scope for a single PR.
- **Validation**: targeted `go test ./cmd/<lambda>`, `go test ./pkg/storage/theorydb`, `go test ./...`, `go vet ./...`,
  `gofmt -l .`, `./lesser verify ci`.
- **Conventional Commit subject**: `fix(runtime): thread app contexts into table clients`.

### 4. Add AppTheory strict route registration wrapper and adopt on one low-risk surface

- **Paths**: a small helper package/function near each route registration surface, first applied to one low-risk Lambda
  such as `cmd/webfinger` or `cmd/objects`; associated tests.
- **Surface**: cmd, tests.
- **Classification**: contract-stability, framework-consumption, operational-reliability.
- **Federation-trust impact**: none if route shapes are unchanged; ActivityPub endpoint surfaces must be route-parity
  tested.
- **Contract impact**: backward-compatible only; route shapes must not change.
- **Schema impact**: none.
- **Framework consumption**: idiomatic AppTheory `GetStrict` / `PostStrict` / `HandleStrict` use.
- **Acceptance**: route registration fails fast in tests if AppTheory rejects a pattern, while generated/static contracts
  remain unchanged.
- **Validation**: targeted route tests, `./lesser verify openapi`, `./lesser verify ci`, `go test ./...`, `go vet ./...`,
  `gofmt -l .`.
- **Conventional Commit subject**: `test(routes): enforce strict app route registration`.

### 5. Expand strict route registration by bounded surface

- **Paths**: bounded route registration surfaces selected after the low-risk proof. `cmd/api/routes.go` is not the first
  target and should itself be split if route inventory shows meaningful sub-surfaces; tests, generated/static contract
  artifacts, and a route inventory/parity artifact are required.
- **Surface**: bounded REST, GraphQL endpoint Lambda routing, SSE/streaming, or ActivityPub endpoint slices.
- **Classification**: contract-stability, framework-consumption, operational-reliability.
- **Federation-trust impact**: none.
- **Contract impact**: backward-compatible only; no route path/method/error shape changes.
- **Schema impact**: none.
- **Framework consumption**: idiomatic AppTheory strict helpers.
- **Acceptance**: each PR targets one bounded route surface and migrates with no route inventory, OpenAPI, or GraphQL
  contract drift unless explicitly coordinated.
- **Validation**: targeted route tests, route inventory/parity artifact, `./lesser verify openapi` or full
  `./lesser verify ci`, `go test ./...`, `go vet ./...`, `gofmt -l .`.
- **Conventional Commit subject**: `fix(api): fail fast on route registration drift`.

### 6. Prototype AppTheoryFunction CDK parity for one representative Lambda

- **Paths**: `infra/cdk/constructs/lambda_functions.go`, `infra/cdk/constructs/*_test.go`, possibly a prototype helper
  under `infra/cdk/constructs/`.
- **Surface**: infra/cdk.
- **Classification**: framework-consumption, operational-reliability.
- **Federation-trust impact**: none.
- **Contract impact**: none.
- **Schema impact**: none.
- **Framework consumption**: conditional idiomatic AppTheory CDK adoption; if parity requires workaround, stop and send
  `coordinate-framework-feedback`.
- **Acceptance**: synthesized template parity is proven for one representative Lambda: function name, runtime,
  architecture, memory, timeout, role, environment variables, code asset root, log retention, DLQ/schedule behavior, and
  permissions match the native-CDK baseline or intentional differences are documented and approved.
- **Validation**: `cd infra/cdk && go test ./constructs ./stacks`, representative `cdk synth` without timeouts,
  `./lesser verify ci`.
- **Conventional Commit subject**: `chore(cdk): prove apptheory function parity`.

### 7. Adopt AppTheoryFunction / AppTheoryFunctionAlarms where parity is proven

- **Paths**: `infra/cdk/constructs/lambda_functions.go`, alarm/monitoring construct tests, inventory docs if needed.
- **Surface**: infra/cdk.
- **Classification**: framework-consumption, operational-reliability.
- **Federation-trust impact**: none.
- **Contract impact**: none.
- **Schema impact**: none.
- **Framework consumption**: idiomatic only if item 6 proves parity without framework workaround.
- **Acceptance**: a bounded set of Lambda functions uses AppTheory constructs without changing deployment semantics, and
  CDK/inventory verifiers pass.
- **Validation**: `cd infra/cdk && go test ./...`, representative `cdk synth`, `./lesser verify ci`.
- **Conventional Commit subject**: `chore(cdk): adopt apptheory lambda constructs`.

### 8. Document framework leverage and release guidance

- **Paths**: `docs/planning/theory-cloud-framework-leverage-roadmap.md`, `docs/release-checklist.md`,
  `docs/deployment.md` or `docs/guides/CLIENT_APP_GUIDE.md` as needed.
- **Surface**: docs.
- **Classification**: docs, framework-consumption, operator-reliability.
- **Federation-trust impact**: none.
- **Contract impact**: none.
- **Schema impact**: none.
- **Framework consumption**: documents canonical usage and constraints.
- **Acceptance**: operator/release docs name the framework pins, validation gates, and the non-goals around schema/API and
  no local framework patches.
- **Validation**: `./lesser verify ci`, `scripts/verify_docs.sh` if needed.
- **Conventional Commit subject**: `docs: record theory cloud framework leverage plan`.

## Roadmap

### Goal

Bring lesser's framework consumption up to the latest released Theory Cloud baseline and then incrementally consume the
newly available reliability/strictness/CDK capabilities in a way that preserves federation trust, Mastodon API
compatibility, schema-as-contract, AGPL posture, and operator deploy safety.

### Classification

Dependency-maintenance, operational-reliability, framework-consumption, contract-stability, docs.

### Surfaces affected

Dependencies, TableTheory storage wrappers, AppTheory runtime route/event handling, CDK infrastructure constructs,
FaceTheory client-install guidance, planning/release docs.

### Sibling-repo coordination

- body: not required initially.
- soul: not required.
- host: awareness required before release if CDK/deploy behavior or release artifact shape changes. Current baseline does
  not change artifact shape.
- greater: not required.
- sim: not required; may benefit from FaceTheory docs but no client contract change.

### Framework coordination

- AppTheory: Arch/framework-steward awareness required for CDK function parity. Send `coordinate-framework-feedback` if
  `AppTheoryFunction` cannot preserve lesser semantics without workaround.
- TableTheory: no upstream change required; consume Lambda timeout APIs. Send feedback only if timeout/context API cannot
  support lesser's Lambda patterns idiomatically.
- FaceTheory: no upstream change required; lesser documents current release asset.

### Federation / ecosystem coordination

- Mastodon clients: no impact intended.
- Remote AP peers: no impact intended.
- Operators: release notes should call out framework pin updates and deployment validation, especially if CDK function
  construct adoption changes synthesized resources.

## Phases

### Phase 0: Dependency/security baseline

- Items: 1, 8 (baseline portions already in branch).
- Dependencies: none.
- Risks: dependency bump can affect every Lambda; CDK/jsii can alter asset resolution; auth-ui security updates must keep
  static build output valid.
- Current evidence: full `./lesser verify ci`, Go tests, CDK tests, auth-ui install/audit/build all passed on the branch.

### Phase 1a: TableTheory Lambda client semantics

- Items: 2 only.
- Dependencies: Phase 0 merged or at least based on the same pins.
- Risks: applying timeout buffers at the wrong lifecycle point could double-apply deadlines, break local DynamoDB/dev
  endpoint behavior, or create unnecessary per-request clients.
- Mitigation: prove `NewLambdaOptimizedClient` semantics with targeted tests, keep existing client reuse where possible,
  use TableTheory's LambdaDB APIs, and prove timeout behavior at the repository/operation boundary.

### Phase 1b+: AppTheory invocation-context threading by Lambda family

- Items: bounded slices of 3.
- Dependencies: Phase 1a completed and Arch-approved.
- Risks: converting all entrypoints at once would mix unrelated Lambda behavior changes and make deadline/cancellation
  regressions hard to localize.
- Mitigation: one Lambda family per PR, with targeted tests and no schema/model/tag changes. If federation-delivery,
  inbox, outbox, or moderation behavior changes beyond context propagation, run `protect-federation-trust` before landing.

### Phase 2a: AppTheory strict route registration proof

- Items: 4 only.
- Dependencies: Arch approval of Phase 1 completion or explicit approval to run independently.
- Risks: route pattern dialect differences (`:username` vs `{username}`) could silently change routing if migrated
  carelessly; API contract drift would strand clients.
- Mitigation: start with one low-risk surface, test exact route parity, and produce a route inventory/parity artifact in
  addition to OpenAPI/GraphQL/static contract checks. `cmd/api/routes.go` is not the proof target.

### Phase 2b+: Strict route expansion by bounded surface

- Items: bounded slices of 5.
- Dependencies: Phase 2a completed and Arch-approved.
- Risks: broader API/streaming/federation route surfaces include endpoints that are not fully represented in Mastodon
  OpenAPI, so generated contracts alone are insufficient evidence.
- Mitigation: migrate one bounded route surface per PR, keep path strings unchanged unless contract review approves a
  mapping, include route inventory/parity artifacts, and run appropriate static contract verification.

### Phase 3a: AppTheory CDK function construct parity proof

- Items: 6 only.
- Dependencies: Phase 0; Arch approval of Phase 2 completion or explicit approval to run independently.
- Risks: synthesized CloudFormation resource replacement, Lambda alias/version rollback semantics, DLQ/schedule wiring,
  IAM permission drift, log-retention drift, asset-root drift.
- Mitigation: compare synthesized template semantics for one representative existing Lambda. If AppTheory construct support
  is not exact enough, stop and send a framework-feedback signal rather than local workaround.

### Phase 3b: AppTheory CDK function construct adoption

- Items: 7 only if Phase 3a proves parity and Arch explicitly approves the synth/template diff.
- Dependencies: Phase 3a completed and Arch-approved.
- Risks: broad construct migration can change deployment artifacts even when Lambda code is untouched.
- Mitigation: adopt only where the Phase 3a parity criteria hold; leave non-parity functions on native CDK with a
  documented reason and/or framework-feedback signal.

### Phase 4: Release/docs closeout

- Items: 8 final updates.
- Dependencies: completed implementation phases.
- Risks: docs can overpromise deployment behavior.
- Mitigation: release notes and docs only describe landed behavior and validation evidence.

## Stage rollout plan

Deployments are not part of this roadmap implementation PR loop. After merge to `main`, operators should use
`deploy-instance` discipline.

### Dev

- Command: `./lesser up --app <slug> --base-domain <domain> --aws-profile <profile> --stage dev`
- Soak criteria:
  - CLI deploy completes with shared stack then per-stage stack.
  - Account creation/login and note creation work.
  - WebFinger/actor/object/inbox/outbox endpoints respond.
  - Federation delivery queue has no unexpected DLQ growth.
  - CloudWatch Lambda error rates do not increase.
  - For CDK construct phases, synthesized resource names/aliases/versions behave as rollback-compatible.

### Staging (where used)

- Command: `./lesser up --app <slug> --base-domain <domain> --aws-profile <profile> --stage staging`
- Soak criteria: same as dev plus any partner/client validation the operator normally uses.

### Live

- Command: `./lesser up --app <slug> --base-domain <domain> --aws-profile <profile> --stage live`
- Authorization: operator-run only; no live deploy by Codex.
- Post-deploy monitoring: CloudWatch error rate/latency, API Gateway 4xx/5xx, SQS DLQ depth, DynamoDB Streams iterator
  age, federation delivery success rate, HTTP Signature failure rate, SNS error-topic messages.

## Release artifact plan

- GitHub Release: next normal lesser release after merged milestones.
- Release notes: include framework pins, auth-ui vulnerability remediation, any TableTheory timeout behavior hardening,
  any CDK synthesized-resource behavior notes, and no schema/API/federation behavior changes unless a later milestone
  says otherwise.
- Managed-consumer (`lesser-host`) impact: none for baseline artifact shape. Coordinate with host if CDK/resource outputs
  or release artifact shape changes later.

## Rollback plan

- Lambda-version rollback: prior release commit/deploy via `./lesser up`; never delete prior Lambda function versions.
- CDK stack rollback: revert commit and deploy; CloudFormation rollback for failed deploys.
- Schema rollback: not applicable; no schema changes planned.
- Federation-state rollback: no federation state changes planned; delivered activities are never recallable.

## AGPL posture

- No proprietary blobs added.
- Dependency license posture remains open-source / AGPL-compatible; any new dependency introduced later must be vetted.

## Advisor-brief authorization

Not applicable. Aron directly authorized the initiative and Arch-requested follow-up work on 2026-05-16.

## Arch scope-review answers

1. Slice Phase 1. Start with helper semantics/tests, then move through Lambda-family slices.
2. Do not put all of `cmd/api/routes.go` in the first strict-route PR. Prove dialect/canonicalization and route parity
   on a small surface first.
3. Use template-parity proof before migrating existing functions. New-Lambdas-only is safe but less useful; representative
   existing-Lambda parity is the better proof, with adoption gated on separate review.
4. Split milestones/PRs beyond the original four phases: Phase 1a vs Phase 1b+ slices, Phase 2a vs Phase 2b+ slices,
   and Phase 3a vs Phase 3b.
