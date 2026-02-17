# Lesser: Domain-first Reliability Roadmap (Trust + WebSockets + Translation + Search)

This roadmap is the execution plan to make the following surfaces **work reliably on every managed deploy/update**
(with no manual post-deploy steps):

- WebSockets (GraphQL subscriptions + streaming)
- Trust proxy + public attestations/JWKS (lesser.host integration)
- Translation
- Search (REST + GraphQL)

Assumption: `lesser-host` now provides **domain-only** endpoints (`https://<stage>.lesser.host`) for all trust APIs and
applies required configuration during provisioning and update jobs.

## Non-negotiables

1) **Domains only**
   - No Lambda Function URL hostnames or AWS-generated endpoints in any:
     - Lesser instance config shown to clients
     - receipts intended for automation
     - managed provisioning outputs
2) **Config is part of provisioning + updates**
   - If an integration requires env vars or IAM permissions, managed provisioning/update must apply them deterministically.
3) **No schema errors for user-visible features**
   - GraphQL must not error due to nulls in non-null list fields.

## Current failure modes (what we must eliminate)

### A) WebSockets “unreachable” from browser clients

Likely causes:
- `Sec-WebSocket-Protocol` negotiation mismatch for GraphQL clients (many libraries enforce `graphql-transport-ws` echo).
- auth token placement/format differences (handshake query param vs `connection_init`).

### B) Trust / attestations endpoints return 503 from instances

Root causes we must permanently prevent:
- missing `LESSER_HOST_URL` / `LESSER_HOST_INSTANCE_KEY_ARN` at deploy/update time
- stage mismatch (dev instance pointing at `https://lesser.host` when only `https://lab.lesser.host` exists)
- “silent fallback” defaults hiding misconfiguration

### C) Translation is “not enabled” (or enabled but unusable)

Root causes:
- missing `TRANSLATION_ENABLED=true`
- missing IAM permissions for AWS Translate (`translate:TranslateText`, `translate:ListLanguages`)

### D) Search fails with “requested element is null which the schema does not allow”

Root causes:
- GraphQL resolver returning `nil` elements inside `[Actor!]!` / `[Object!]!` lists.

## Roadmap

### M0 — Make managed integration config first-class (no “env var afterthought”) (1–2 weeks)

**Goal:** a managed deploy/update can always reapply the exact intended integration config (trust + translation) from a
single source of truth.

**Work**
- Extend the managed `--provisioning-input` schema to include optional integration config:
  - `lesser_host_url` (domain-only; stage-aware set by runner)
  - `lesser_host_attestations_url` (domain-only; usually same as above)
  - `lesser_host_instance_key_arn` (instance-account secret ARN)
  - `translation_enabled` (`true|false`)
- Update `./lesser up` to prefer provisioning-input values over ambient env vars, and pass them to CDK context.
- Persist the effective config into the deployment receipt (`state.json`) so update jobs can be reproducible.
- Add config validation (CLI-side and runtime-side):
  - reject Lambda Function URL hosts for `LESSER_HOST_URL`
  - emit clear “trust is misconfigured” warnings when trust proxy endpoints would 503

**Acceptance criteria**
- A runner can provision/update without exporting ad-hoc env vars, and the resulting instance has working trust + translation flags.
- Re-running `./lesser up` with the same provisioning input keeps trust integration working (no regressions).

**Suggested verification**
```bash
./lesser up --base-domain <domain> --provisioning-input provision.json
curl -sS https://<domain>/api/v2/instance | jq '.configuration.translation.enabled'
```

---

### M1 — Surface trust configuration in instance config for clients (2–4 days)

**Goal:** clients can discover the correct trust endpoints and whether the integration is enabled.

**Work**
- Add `configuration.trust` to `GET /api/v2/instance`:
  - `enabled` (boolean)
  - `base_url` (domain-only)
  - `jwks_url` and `attestations_url` (domain-only)
  - optional `proxy` paths used by the instance (`/api/v1/trust/...`)
- Ensure the value is only present/enabled when Lesser is configured to reach `lesser.host` successfully (not just “env var exists”).
- (Optional) add equivalent fields to the GraphQL `Instance` query response so GraphQL-only clients don’t need REST.

**Acceptance criteria**
- `curl https://<domain>/api/v2/instance | jq '.configuration.trust'` returns a stable object (not `null`) when configured.

---

### M2 — Trust proxy endpoint hardening + error semantics (1–2 weeks)

**Goal:** trust failures are diagnosable and never appear as a generic “503” when the root cause is configuration or auth.

**Work**
- Distinguish these cases in the proxy layer:
  - unconfigured (missing `LESSER_HOST_URL` / instance key) → explicit 422/409 with actionable message
  - upstream network failure → 503 “lesser-host unavailable”
  - upstream 401/403 → map to appropriate error (instance key invalid/revoked)
- Decide whether instance-proxied JWKS/attestations should be **public** (recommended) or remain auth-gated:
  - If public, remove user-auth requirement for `GET /api/v1/trust/jwks.json` and `GET /api/v1/trust/attestations*`.
  - If auth-gated, ensure clients use `configuration.trust.jwks_url` / `attestations_url` directly (domain-only).

**Acceptance criteria**
- A misconfigured instance does not return a generic 503 for trust endpoints; it returns an explicit “trust not configured” error.
- A configured instance can call lesser.host trust endpoints successfully using the instance key.

---

### M3 — Translation end-to-end (flag + IAM + smoke) (1–2 weeks)

**Goal:** translation works when enabled and fails gracefully when disabled.

**Work**
- Ensure translation enablement is applied by managed deploy/update (`TRANSLATION_ENABLED=true`).
- Update CDK/IAM so Lambdas have:
  - `translate:TranslateText`
  - `translate:ListLanguages`
  - (scope: `*` is acceptable initially; optionally restrict by region/account later)
- Add a non-destructive smoke check that validates:
  - translation is enabled in `/api/v2/instance`
  - translate endpoint returns a structured result (or a structured “disabled” response)

**Acceptance criteria**
- With translation enabled, a request to translate returns 200 (not IAM AccessDenied).
- With translation disabled, the API returns a clear 422 and instance config shows `enabled=false`.

---

### M4 — Search: eliminate GraphQL schema null errors permanently (2–4 days)

**Goal:** GraphQL search never errors due to nulls in non-null list fields.

**Work**
- Update GraphQL search resolver to:
  - build `accounts/statuses/hashtags` using `append` (skip nils)
  - ensure `statuses` always contains valid `Object` values (load status by id when needed)
- Add regression tests that assert:
  - searching for a non-self user returns results (or empty arrays) without schema errors
  - searching for self continues to work

**Acceptance criteria**
- Search queries never return `the requested element is null which the schema does not allow`.

---

### M5 — WebSockets: converge on a compatible contract (1–3 weeks)

**Goal:** browser clients can connect reliably to both:
- streaming WS (`wss://ws.<domain>/stream`)
- GraphQL subscriptions WS (`wss://ws.<domain>/`)

**Work**
1) **Protocol negotiation decision (GraphQL WS)**
   - Try returning `Sec-WebSocket-Protocol: graphql-transport-ws` from the `$connect` handler and verify whether API
     Gateway forwards it in the handshake response.
   - If API Gateway cannot echo the subprotocol, choose one:
     - implement a client transport that does not require subprotocol echo (custom WS client), or
     - move GraphQL subscriptions to a different WS origin that supports subprotocol negotiation.

2) **Auth contract**
   - Document and enforce a single supported method:
     - token in query string (`access_token`) **or**
     - token via `connection_init` payload
   - Ensure Lesser accepts the chosen method consistently and logs actionable errors.

3) **Smoke verification**
   - Add a smoke test that:
     - connects
     - authenticates
     - starts a subscription
     - validates at least one message path (ping/pong or event delivery)

**Acceptance criteria**
- A standard browser client can establish a stable connection and receive subscription events on a managed instance.

---

### M6 — “Once and for all” verification gates (ongoing; add within 1–2 weeks after M0–M5)

**Goal:** prevent regressions by making the integration surfaces verifiable in automation.

**Work**
- Add a `./lesser verify` mode (or extend existing smoke suites) that can run against a base URL and check:
  - trust proxy configured (or explicitly disabled)
  - translation enabled implies AWS Translate calls succeed
  - GraphQL search returns without schema errors
  - WS endpoints are reachable and stable
- Integrate this into the managed update job in `lesser-host` as a post-deploy validation step.

**Acceptance criteria**
- A managed update that breaks trust/translation/search/ws is detected automatically and marked failed with a clear reason.

## Sim / managed instance verification checklist (operator-friendly)

After provisioning or update:

1) Instance config
```bash
curl -sS https://<domain>/api/v2/instance | jq '.configuration | {translation:.translation, trust:.trust}'
```

2) Trust public endpoints (domain-only; preferred path)
```bash
curl -sS https://<stage>.lesser.host/.well-known/jwks.json | jq .
```

3) Translation (when enabled)
```bash
curl -sS -H "Authorization: Bearer <token>" \\
  -X POST "https://<domain>/api/v1/statuses/<id>/translate" | jq .
```

4) GraphQL search (non-self user)
```bash
curl -sS -H "Authorization: Bearer <token>" \\
  -H "Content-Type: application/json" \\
  -d '{"query":"query($q:String!){search(query:$q){accounts{preferredUsername}}}","variables":{"q":"test"}}' \\
  https://<domain>/api/graphql | jq .
```

## Dependencies (cross-repo)

- `simulacrum` (client):
  - may need adjustments depending on the chosen WebSocket protocol negotiation strategy (M5).
- `lesser-host` (control plane):
  - should consume the provisioning-input config changes from M0 and ensure update jobs reapply config deterministically.

