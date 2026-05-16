# Theory Cloud Framework Leverage Roadmap

Status: proposed for Arch review (written 2026-05-16)
Branch with baseline bump: `chore/framework-deps-2026-05-16`

## Reported need

Aron asked whether there are new Theory Cloud framework updates lesser should consider implementing after updating the
framework pins. He then authorized the full workflow: investigate deeply, enumerate changes, plan a roadmap, create a
GitHub Project, email `arch@lessersoul.ai`, and iterate milestone-by-milestone with Arch review and approval before
moving to the next milestone. Aron additionally authorized work requested by Arch on 2026-05-16.

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

- AppTheory latest stable release/tag: `v1.6.0`.
  - Relevant capabilities: AppSync Lambda resolver support, strict route registration helpers, multi-language CDK/jsii
    constructs including `AppTheoryFunction`, `AppTheoryFunctionAlarms`, `AppTheoryQueueProcessor`,
    `AppTheorySsrSite`, `AppTheoryPathRoutedFrontend`, and existing Rest API / queue / table constructs.
- TableTheory latest stable release/tag: `v1.8.3`.
  - Relevant capabilities: Lambda timeout configuration and hardening, encrypted batch retry fixes, audit/security guard
    hardening, pointer `omitempty` preservation.
- FaceTheory latest stable release: `v3.1.2`.
  - Relevant capabilities: dependency pin alignment and Svelte vulnerable peer range exclusions.
- There is no newer stable framework release beyond the pins already applied on
  `chore/framework-deps-2026-05-16`.

### What is definitely true in lesser today

- The baseline branch pins:
  - root `go.mod`: AppTheory `v1.6.0`, TableTheory `v1.8.3`;
  - `infra/cdk/go.mod`: AppTheory `v1.6.0`, AWS CDK Go `v2.254.0`, jsii runtime `v1.129.0`;
  - FaceTheory client-install tests/docs: `v3.1.2` GitHub release asset;
  - auth UI: Astro `6.3.3`, Svelte `5.55.7`, devalue override `5.8.1`.
- The baseline branch already fixed one CDK/jsii surfaced issue by canonicalizing `LambdaAssetRoot` to an absolute path
  before `Code_FromAsset`.
- `pkg/storage/theorydb.GetLambdaClient(ctx)` uses `tabletheory.NewLambdaOptimized()` and applies
  `WithLambdaTimeoutConfig` / `WithLambdaTimeout(ctx)`.
- `pkg/storage/theorydb.NewLambdaOptimizedClient(ctx, region)` currently creates a standard `tabletheory.New` client and
  ignores the supplied context. Many Lambda entrypoints use this helper, often with `context.Background()`.
- AppTheory `Context` and `EventContext` expose `.Context() context.Context`, and `EventContext` carries
  `RemainingMS`, but many handler initialization paths and some handler paths do not consistently thread that context
  to TableTheory client creation.
- lesser already uses several AppTheory CDK constructs: Dynamo tables, Rest API router, queues, queue consumers,
  EventBridge handlers, media CDN, Lambda roles, KMS keys, stream mappings. Lambda functions themselves are still
  created directly with AWS CDK `awslambda.NewFunction` in `infra/cdk/constructs/lambda_functions.go`.
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
   - Evidence: v1.6.0 publishes Go jsii bindings for `AppTheoryFunction` and `AppTheoryFunctionAlarms`; lesser still
     creates functions directly with native AWS CDK.
   - Verification: synth a representative function behind tests and compare generated CloudFormation for function name,
     architecture, runtime, timeout, memory, role, env, asset path, DLQ, and permissions before broader migration.
4. **FaceTheory v3.1.2 is best consumed as guidance/provenance, not a hard client-app gate inside lesser.**
   - Evidence: `lesser client install` validates that a FaceTheory dependency exists; enforcing an exact dependency URL
     would break local workspace/client development.
   - Verification: docs/tests are enough in lesser; any client-app enforcement belongs in client repos or a non-blocking
     verifier.

### Verification step

Next concrete verification is Arch review of this scope, followed by Milestone 1 PR review for the already-completed
baseline bump branch. No further implementation should start until Arch approves the scope or requests revisions.

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
- FaceTheory v3.1.2 guidance remains backward-compatible for client app authors.

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
  endpoint behavior, converter registration, and timeout buffer application.
- **Validation**: `go test ./pkg/storage/theorydb`, `go test ./pkg/storage/...`, `go test ./...`, `go vet ./...`,
  `gofmt -l .`, `./lesser verify ci`.
- **Conventional Commit subject**: `fix(storage): make lambda table clients timeout aware`.

### 3. Thread AppTheory request/event contexts into DB initialization call sites

- **Paths**: bounded Lambda entrypoints that currently call `NewLambdaOptimizedClient(context.Background(), ...)` or
  equivalent inside request/event handling; likely `cmd/graphql`, `cmd/graphql-ws`, `cmd/sse`, `cmd/streaming`,
  `cmd/search-indexer`, `cmd/federation-*`, `cmd/cms-scheduler`, `cmd/trend-aggregator`, `cmd/actor`, `cmd/objects`,
  `cmd/collections`, `cmd/webfinger`, `cmd/enhanced-federation-processor`, `cmd/import-processor`.
- **Surface**: cmd, storage.
- **Classification**: operational-reliability, framework-consumption.
- **Federation-trust impact**: none if behavior is limited to context/deadline propagation. If federation-delivery or
  inbox behavior changes, run `protect-federation-trust` before implementation.
- **Contract impact**: none.
- **Schema impact**: none.
- **Framework consumption**: idiomatic AppTheory `Context.Context()` / `EventContext.Context()` use.
- **Acceptance**: selected call sites pass the real AppTheory context where available, tests cover deadline propagation
  and no `context.Background()` remains in the targeted runtime path.
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

### 5. Expand strict route registration to Mastodon/API and streaming surfaces

- **Paths**: `cmd/api/routes.go`, `cmd/api/main.go`, `cmd/graphql/main.go`, `cmd/sse/main.go`, relevant route tests.
- **Surface**: REST, GraphQL endpoint Lambda routing, SSE/streaming.
- **Classification**: contract-stability, framework-consumption, operational-reliability.
- **Federation-trust impact**: none.
- **Contract impact**: backward-compatible only; no route path/method/error shape changes.
- **Schema impact**: none.
- **Framework consumption**: idiomatic AppTheory strict helpers.
- **Acceptance**: strict registration covers high-traffic HTTP surfaces and OpenAPI/GraphQL static verification remains
  clean.
- **Validation**: `go test ./cmd/api ./cmd/api/handlers ./cmd/graphql ./cmd/sse`, `./lesser verify openapi`,
  `./lesser verify graphql-coverage`, `./lesser verify ci`, `go vet ./...`, `gofmt -l .`.
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

### Phase 1: TableTheory Lambda timeout/context hardening

- Items: 2, then bounded slices of 3.
- Dependencies: Phase 0 merged or at least based on the same pins.
- Risks: creating per-request clients could hurt cold/warm performance if implemented incorrectly; timeout buffers must
  be applied once; local DynamoDB/dev endpoint behavior must remain intact.
- Mitigation: keep singleton/client reuse semantics where possible, use TableTheory's LambdaDB APIs, add targeted tests,
  and do not alter model tags or item shapes.

### Phase 2: AppTheory strict route registration

- Items: 4, then 5.
- Dependencies: Phase 0; Arch approval of Phase 1 completion.
- Risks: route pattern dialect differences (`:username` vs `{username}`) could silently change routing if migrated
  carelessly; API contract drift would strand clients.
- Mitigation: start with one low-risk surface, test exact route parity, run OpenAPI/GraphQL/static contract checks, and
  keep path strings unchanged unless AppTheory rejects them and contract review approves a mapping.

### Phase 3: AppTheory CDK function construct parity

- Items: 6, then 7 only if 6 proves parity.
- Dependencies: Phase 0; Arch approval of Phase 2 completion.
- Risks: synthesized CloudFormation resource replacement, Lambda alias/version rollback semantics, DLQ/schedule wiring,
  IAM permission drift, log-retention drift, asset-root drift.
- Mitigation: treat item 6 as a parity proof first. If AppTheory construct support is not exact enough, stop and send a
  framework-feedback signal rather than local workaround.

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

## Open questions for Arch

1. Should Phase 1 prioritize all `NewLambdaOptimizedClient(context.Background(), ...)` call sites in one PR, or should it
   slice by Lambda family to reduce review risk?
2. Is strict route registration worthwhile on `cmd/api/routes.go` in one PR, or should it remain limited to smaller
   ActivityPub/GraphQL/SSE surfaces until route dialect parity is proven?
3. For CDK, should lesser target `AppTheoryFunction` adoption only for new Lambdas first, or is template-parity proof
   strong enough to migrate existing functions once one representative Lambda passes?
4. Should any of these phases be split into separate GitHub Project milestones beyond the four phases above?
