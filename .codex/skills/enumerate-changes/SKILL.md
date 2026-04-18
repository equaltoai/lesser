---
name: enumerate-changes
description: Use after scope-need and relevant specialist skills approve work. Takes the scoped-need document and produces a flat, ordered list of discrete changes required. Each change is scoped to be a single commit.
---

# Enumerate changes

A scoped need describes *what* is being delivered. An enumerated change list describes *what must move in the repo*. This skill is the transformation.

lesser change lists vary in scale: a narrow bug fix might be two commits; a Mastodon-compat-preserving feature addition might be eight; a federation-surface expansion with coordinated sibling-repo work might be larger. The single-commit rule holds regardless of total count.

## Input required

An approved scoped-need document from `scope-need`. If the scope touches federation trust, API contract, schema, framework consumption, deploy, or advisor briefs, the relevant specialist skill's findings also apply. Load prior context with `memory_recent`.

## The walk

Walk the scoped need against every surface of lesser:

1. **`cmd/<lambda>/main.go`** — Lambda entrypoints. 43 in total; see `docs/specs/01-lambda-inventory-matrix.md`.
2. **`pkg/activitypub/`** — ActivityPub primitives (activity types, object shapes, JSON-LD context handling).
3. **`pkg/federation/`** — federation logic (HTTP Signatures, delivery retry, inbox handling, relay, cost tracking).
4. **`pkg/services/`** — domain services (accounts, notes, lists, follow-graph, moderation, etc.).
5. **`pkg/storage/`** — repository layer and model definitions.
6. **`pkg/storage/models/*.go`** — DynamoDB models with TableTheory struct tags. Schema changes land here.
7. **`pkg/auth/`** — authentication (WebAuthn, OAuth 2.0, wallet signature, TOTP).
8. **`pkg/agents/`** — agent governance state and delegation logic.
9. **`graph/*.graphql`** — modular GraphQL schema (`core.graphql`, `phase1.graphql`, `phase2.graphql`, `phase3.graphql`). Regenerate `docs/contracts/graphql-schema.graphql` after changes.
10. **`infra/cdk/`** — AWS CDK TypeScript stacks + Go pinning exports. Changes here affect every deployment.
11. **`docs/contracts/openapi.yaml`** — Mastodon-compat REST contract. Rides with handler changes that affect it.
12. **`docs/specs/01-lambda-inventory-matrix.md`** — Lambda inventory. Updates ride with Lambda additions/removals.
13. **`docs/architecture/dynamodb/gsi_usage_guide.md`** — GSI access-pattern reference. Updates ride with GSI changes.
14. **`docs/federation.md` / `docs/configuration.md` / `docs/deployment.md`** — operator documentation.
15. **`docs/architecture/{auth,dynamodb,cms,moderation}/`** — subsystem deep-dives.
16. **`tests/`** — integration tests, system tests, local e2e harness.
17. **`go.mod` / `go.sum`** — dependency changes.
18. **`AGENTS.md`** — repository guidelines. Rarely touched; changes are governance-level.
19. **`README.md`** — top-level overview. Rides with feature-list changes.
20. **`CODEOWNERS`** — ownership. Rarely touched.

A change that touches none of these isn't really a change. A change that touches several is fine when they share intent.

## The ordering rules

1. **Test-first for bug fixes.** Add the regression test first (fails against current code), then land the fix. Especially important for federation-trust, schema-consistency, and Mastodon-compat fixes.
2. **Models before services, services before handlers, handlers before Lambda main.** Dependency direction is inward-to-outward.
3. **Schema changes (TableTheory tag edits) land before the handlers that read/write the new shape**, with the exception of test-first bug fixes where the order inverts.
4. **CDK infrastructure changes land separately from Lambda code changes.** CDK changes affect every deployment; isolation matters for bisect and rollback.
5. **GraphQL schema file changes land alongside `docs/contracts/graphql-schema.graphql` regeneration in the same commit.** Never land schema changes without the generated contract.
6. **OpenAPI spec updates land alongside the handler change that affects them.** Same commit when possible.
7. **Dependency bumps land in isolated commits** for bisect clarity.
8. **Framework-consumption changes** (AppTheory / TableTheory / CDK) — if a change reflects idiomatic consumption of a new framework version, it lands alongside the framework bump. If it reflects framework awkwardness, refer to `coordinate-framework-feedback` — the change may not belong here.
9. **Documentation rides with the behavior it describes** for federation / schema / API / architecture changes.
10. **AGPL-affecting items land in isolated commits** — license-header additions, dependency-license vetting, AGPL-compliance fixes.

## The mission-scope rule

Every enumerated item must answer: **is this lesser-mission work (federation, API contract, schema, reliability, AGPL, framework-feedback), or is it scope growth outside the mission?**

- **In-mission**: federation feature / fix, Mastodon-compat preservation / refinement, GraphQL schema evolution, ActivityPub actor shape work, schema correctness, reliability / security / observability, operator-facing deploy / CLI improvements, moderation tooling, AGPL compliance, idiomatic framework-consumption
- **Scope growth (refuse)**: transaction routing, payments processing, identity issuance, general-purpose SaaS capability, proprietary extensions, framework patches, non-AGPL dependencies

If any enumerated item is scope growth, stop and revisit `scope-need`.

## The federation-trust rule

Every enumerated item must also answer: **does this touch federation trust?**

- **No** — the default for most changes. Proceed normally.
- **Yes — preserves or strengthens trust** (e.g. better signature verification, additional audit log, tighter delivery retry). Enumerate with `protect-federation-trust` findings referenced.
- **Yes — proposed to weaken trust** (e.g. accept unsigned activities from a specific peer, skip verification for performance). Refuse unless explicit authorization exists with documented justification.

## The contract rule

Every enumerated item must also answer: **does this change observable behavior for Mastodon clients, remote Fediverse peers, sibling repos, or direct API consumers?**

- **No** — the default.
- **Yes — backward-compatible** (additive). Proceed.
- **Yes — breaking** (removed field, changed semantics, removed endpoint, changed ActivityPub actor shape). The `preserve-mastodon-api-compat` walk must be complete; enumeration references its coordination plan.

## The schema rule

Every enumerated item must also answer: **does this touch PK / SK / GSI / TableTheory tags / optimistic-concurrency versioning?**

- **No** — the default.
- **Yes** — the `validate-schema` walk must be complete before this item is enumerable. The enumeration references the walk's output and any required rollout coordination.

## The framework-consumption rule

Every enumerated item must also answer: **does this consume AppTheory / TableTheory / FaceTheory / greater-components idiomatically, or does it work around them?**

- **Idiomatic** — proceed.
- **Workaround** — stop. Route through `coordinate-framework-feedback`. The change may not belong in lesser; it may belong in the framework.

## The single-commit rule

Each enumerated item fits in one commit:

- One logical intent
- `go build ./...` succeeds at the end of the commit
- `go test ./...` passes at the end of the commit
- `go vet ./...` passes
- `gofmt` / `goimports` leave the tree clean
- `./lesser verify --smoke` (or equivalent) passes for contract-adjacent changes
- For CDK changes: `cdk synth` succeeds for at least one representative `(<app>, <stage>)`
- No commit depends on a later item to compile or pass tests

## Output format

```markdown
### N. <imperative title>

- **Paths**: <files or directories touched>
- **Surface**: <cmd / activitypub / federation / services / storage / storage/models / auth / agents / graph / infra/cdk / docs / tests / deps>
- **Classification**: <security / federation-trust / contract-stability / schema / operational-reliability / AGPL / framework-feedback / bug-fix / test-coverage / dependency-maintenance / docs>
- **Federation-trust impact**: <none / preserves / strengthens — refuse if weakens>
- **Contract impact**: <none / backward-compatible / breaking — coordination referenced>
- **Schema impact**: <none / walk complete via validate-schema>
- **Framework consumption**: <idiomatic / reported upstream via coordinate-framework-feedback>
- **Acceptance**: <one sentence: what makes this commit done>
- **Validation**: <`go test ./<package>/...`, `go vet ./...`, `gofmt -l .`, `./lesser verify --smoke`, `cdk synth` for representative stage>
- **Conventional Commit subject**: `<type(scope): subject>` (lowercase present-tense also acceptable: "feat: milestone M2 federation delivery retry")
```

## Self-check before handing off

- [ ] Every item is in-mission — not scope growth
- [ ] No item weakens federation trust
- [ ] No item silently breaks Mastodon-compat, GraphQL, ActivityPub actor, or OpenAPI contracts
- [ ] Schema-touching items reference the `validate-schema` walk
- [ ] Contract-affecting items reference the `preserve-mastodon-api-compat` walk
- [ ] Framework-awkward items are routed to `coordinate-framework-feedback`, not patched locally
- [ ] No item patches AppTheory, TableTheory, or CDK in the lesser tree
- [ ] Bug fixes follow test-first ordering
- [ ] CDK changes isolated from Lambda code
- [ ] GraphQL schema changes include `docs/contracts/graphql-schema.graphql` regeneration
- [ ] OpenAPI changes ride with handler changes
- [ ] Dependency bumps isolated
- [ ] Every item has a test or verify-smoke validation
- [ ] No item requires a future item to compile
- [ ] No hardcoded secrets or signing keys
- [ ] No full-JWT / full-actor-private-key / raw-password / raw-credential logging
- [ ] No deletion of Lambda function versions or bootstrap mnemonic files
- [ ] No AGPL-incompatible dependencies introduced
- [ ] No proprietary blobs
- [ ] Full list satisfies the scoped need's success criteria

## Persist

Append only if the enumeration surfaces something unusual — a test-coverage gap, a Lambda inventory discipline subtlety, a GraphQL schema regeneration edge case, a TableTheory tag interaction that matters for future changes, a CDK-cross-stack dependency that required care. Routine enumerations aren't memory material. Five meaningful entries beat fifty log-shaped ones.

## Handoff

Invoke `plan-roadmap` to sequence the flat list into phases and identify the per-stage rollout plan.
