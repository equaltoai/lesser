# Lesser: Instance-Owned Configuration Roadmap (Eliminate Env-Var Drift)

Status: proposal (written 2026-02-17)

## Problem

Today, critical instance behavior is effectively controlled by **Lambda environment variables** (set via CDK/CloudFormation):

- Trust / attestations / AI verification proxy (`LESSER_HOST_*`)
- Translation (`TRANSLATION_ENABLED`)
- Tipping (`TIP_*`)

This is brittle in AWS:

- CloudFormation treats the function `Environment.Variables` map as **authoritative desired state**.
- Any deploy path that does not include a key **deletes it**, which silently disables features.
- Managed instances can become “misconfigured” simply by re-running a deploy from a different runner or local machine.

We need instance configuration to be:

- **persisted** (survives redeploys),
- **discoverable** (clients can query it),
- **admin-configurable** (instance operators can change it),
- **managed-provisionable** (lesser.host can set sane defaults during provisioning),
- **merge-safe on update** (never cleared because an input is missing),
- and based on **well-known names**.

## Target design (non-negotiable)

### 1) Persist config in DynamoDB under well-known keys

Use the existing instance config namespace:

- `PK = "INSTANCE#CONFIG"`
- `SK = <WELL_KNOWN_CONFIG_KEY>`

Define constants for SK names (no stringly-typed keys scattered around the codebase).

### 2) Two-layer config: managed defaults + operator overrides

To satisfy:

- “accept whatever lesser-host defines when provisioned”
- “use what’s already there on updates”
- “must be configurable on the instance (not host-blocked)”

Store *both*:

- `managed` (set/updated by provisioning tooling)
- `override` (set by instance admin/operator)

Runtime resolves `effective = override if set else managed else builtin default`.

Managed updates MUST NOT clobber overrides.

### 3) Never store plaintext secrets in DynamoDB

For trust integration:

- Store **only** `instance_key_secret_arn` (Secrets Manager ARN) in config.
- Runtime loads plaintext key via Secrets Manager (with in-process caching).

### 4) Env vars become bootstrap inputs only

Env vars may still exist as:

- a **temporary bootstrap** path (first deploy / migration),
- a **local dev** override,

…but production behavior must not depend on env vars being present in the CloudFormation template.

## Well-known config records (v1)

### `SK="TRUST_CONFIG"`

Fields:

- `managed.base_url` (e.g. `https://lab.lesser.host`)
- `managed.attestations_url` (usually same as base)
- `managed.instance_key_secret_arn` (Secrets Manager ARN in instance account)
- `override.*` (same shape, optional)

### `SK="TRANSLATION_CONFIG"`

Fields:

- `managed.enabled` (bool)
- `override.enabled` (optional bool)

### `SK="TIPS_CONFIG"`

Fields:

- `managed.enabled` (bool)
- `managed.chain_id` (int)
- `managed.contract_address` (string)
- `override.*` (optional)

### `SK="AI_CONFIG"` (already exists in `models.AIInstanceConfig`)

Wire this record into runtime feature gating and admin configuration instead of leaving it unused.

## Roadmap (implementation milestones)

### M1 — Storage models + repository API (well-known keys)

**Work**

- Add models under `pkg/storage/models/`:
  - `InstanceTrustConfig`
  - `InstanceTranslationConfig`
  - `InstanceTipsConfig`
  - (use existing `AIInstanceConfig`)
- Add repository helpers under `pkg/storage/repositories/`:
  - `EnsureTrustConfig()` / `GetTrustConfig()` / `SetTrustManagedDefaults()` / `SetTrustOverride()` / `ClearTrustOverride()`
  - same shape for translation + tips + AI
- Add a single resolver function that returns the **effective** config for runtime usage:
  - `EffectiveTrustConfig()`, `EffectiveTranslationEnabled()`, `EffectiveTipsConfig()`, `EffectiveAIConfig()`

**Acceptance criteria**

- Missing records are created with built-in defaults (idempotently).
- Repository calls never return “not found” for config reads; they return defaults.

---

### M2 — Runtime consumption: `/api/v1/instance` + trust proxy

**Work**

- Update `GET /api/v1/instance` (and/or `/api/v2/instance`) to source:
  - `configuration.trust.*` from `EffectiveTrustConfig()`
  - `configuration.translation.enabled` from `EffectiveTranslationEnabled()`
  - `configuration.tips.*` from `EffectiveTipsConfig()`
- Update trust proxy handlers (`/api/v1/trust/*`) to:
  - use `EffectiveTrustConfig()` for base URLs and secret ARN
  - load the instance key via Secrets Manager on-demand (cached)
  - return explicit 422/409 with actionable messages when disabled/missing

**Acceptance criteria**

- A redeploy that drops env vars does not change `/api/v1/instance` flags.
- Trust endpoints no longer depend on `LESSER_HOST_*` env vars to be present.

---

### M3 — Admin API surface (instance-configurable)

**Work**

- Add GraphQL query/mutations (admin-only) for config:
  - Query: `instanceConfig` (returns managed + override + effective)
  - Mutations:
    - `setManagedDefaults` (reserved for provisioning tooling; still admin-authenticated)
    - `setOverride` / `clearOverride` for instance operators
- Add a small “viewer role” signal in GraphQL so clients can determine admin status without OAuth-scope hacks.

**Acceptance criteria**

- Admin users can enable/disable translation/tips/AI and set trust base URL from the instance itself.
- Non-admin users cannot mutate config.

---

### M4 — `lesser up` applies provisioning config into DynamoDB (merge-safe)

**Work**

- Extend the `--provisioning-input` schema (vNext) to include:
  - trust managed defaults (base URLs + instance key secret ARN)
  - translation enabled
  - tips config
  - AI config defaults
- After CDK deploy completes, `lesser up` writes these values into the instance table as **managed defaults**:
  - only sets managed fields that are explicitly provided
  - never clears existing managed fields when an input is missing
  - never touches overrides
- Keep env vars as optional bootstrap inputs only:
  - if provisioning input is absent, env vars can seed managed defaults once
  - log a warning when env vars are used to seed config

**Acceptance criteria**

- A managed runner can re-run `lesser up` repeatedly without losing config.
- A local deploy cannot wipe trust/translation/tips by omission.

---

### M5 — Migration + guardrails

**Work**

- One-time migration behavior:
  - If `TRUST_CONFIG` is missing but `LESSER_HOST_*` env vars exist, create the config record (managed defaults).
  - Same for translation/tips.
- Add runtime warnings/metrics when falling back to env vars.
- Document the new precedence model and deprecate env-var-only config in `docs/configuration.md`.

**Acceptance criteria**

- Existing instances migrate forward without manual steps.
- Operators can see when they’re relying on deprecated env var config.

## Notes / explicit non-goals

- This roadmap does **not** fix AI processing correctness (stream wiring, queue semantics) beyond configuration gating.
- This roadmap does **not** change lesser-host’s CloudFront routing; see `lesser-host/docs/roadmap-domain-first.md`.

