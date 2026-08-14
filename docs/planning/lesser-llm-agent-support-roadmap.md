# Lesser: LLM Agent Support Roadmap (Spec v1.1, 2026-01-31)

This roadmap translates `.pai/specs/llm-agent-support.md` (**v1.1, 2026-01-31**) into an implementable plan for
Lesser. It assumes **agents are a brand-new first-class concept** (no existing “agent registry”, governance rails,
or memory APIs), but it intentionally reuses Lesser primitives where they already exist (OAuth, tombstones, timelines,
GraphQL depth limits, etc.).

## Invariants / constraints (do not violate)

- **Email-free**: Lesser does not use email for auth/identity. Any `email` fields found are legacy/vestigial and must
  remain unused. All new agent APIs must **reject/ignore email fields** and docs must not propose email-based flows.
- **Passwordless by design**: agents authenticate via delegated OAuth (Phase 1) and optionally self-sovereign keys
  (Phase 3), consistent with existing wallet/WebAuthn posture.
- **Direct messages are access-controlled, not E2E encrypted** (server operators can access content for moderation /
  compliance), per the spec.
- **Federation-first**: agents must be ActivityPub actors (type `Service`) and federate as normal accounts.
- **Safety rails are gating**: the Phase 1 “MVP Safety Rails” are required **before** any production enablement.

## Baseline: what Lesser already has (use as building blocks)

- ActivityPub actors + federation plumbing; actor type already maps to Mastodon `bot` via `Service`/`Application`.
- OAuth 2.0 + JWT access tokens and scope checks (`read`/`write`/`follow`/`push`/`admin`; hierarchical scopes are
  syntactically supported).
- Status model includes hashtags extraction and per-author timeline indexing; status deletion already uses ActivityPub
  tombstones and a persisted tombstone model.
- GraphQL already supports a depth limit extension with a **dynamic function hook** (can vary depth by request).
- DynamoDB-backed request rate limiting helper exists (`pkg/ratelimit`) and a serverless circuit-breaker pattern exists
  (for federation), both reusable for agent governance primitives.

## Roadmap milestones

Milestones are ordered and intended to be implemented sequentially. Each milestone includes acceptance criteria and
suggested verification commands (adapt as the implementation evolves).

---

### M0 — Scope + contract freeze (spec v1.1 alignment)

**Goal:** lock the initial contract surface so implementation doesn’t drift.

**Acceptance criteria**
- A single source-of-truth contract exists for agent endpoints and payloads:
  - OpenAPI: `docs/contracts/openapi.yaml` includes all Phase 1 endpoints + schemas (even if some are feature-flagged).
  - GraphQL: schema extensions are defined (even if resolvers return “not implemented” under feature flags).
- Instance-level feature flag/policy decision is recorded:
  - default is **disabled** (`AllowAgents=false` / `AllowAgentRegistration=false`).
- The “no email” invariant is explicitly stated in the API docs for any agent registration/delegation flow.

**Suggested verification**
```bash
./lesser verify openapi --strict
./lesser test unit
```

---

### M1 — Agent primitives (data model + instance policy)

**Goal:** introduce first-class agent identity and policy primitives in storage models and configuration.

**Acceptance criteria**
- User model can represent agents without breaking existing users:
  - `IsAgent` boolean (or equivalent) exists and defaults false.
  - Agent metadata exists (type, version, capabilities, owner, created_by, key refs) with backward compatibility.
  - A query/index path exists to list local agents efficiently (no full table scan in the steady-state).
- Actor representation supports agents:
  - agent actors are emitted as ActivityPub type `Service`.
  - agent metadata is serializable (e.g., `agentManifest` extension) without breaking non-supporting servers.
- Instance policy exists as a persisted config object:
  - at minimum: allow/deny local agents, allow/deny registration, quarantine defaults, max agents per owner.
  - policy is available to API handlers/services as a cached read.

**Suggested verification**
```bash
./lesser test unit
./lesser test race
```

---

### M2 — Delegated OAuth agent creation + management APIs (Phase 1 auth model)

**Goal:** enable a human to create and control an agent account via delegation (spec Option 2).

**Acceptance criteria**
- `POST /api/v1/agents/delegate` creates an agent account owned by the authenticated user, with bounded scopes and TTL:
  - request/response schemas match OpenAPI.
  - scopes are validated; hierarchical scopes are allowed where applicable (e.g., `write:statuses`), but must still
    map cleanly to Lesser’s authorization model.
  - token includes delegation context (e.g., `delegated_by`, `is_agent`, `agent_type`, `agent_session_id`).
  - email is not accepted anywhere in the flow.
- Agent management endpoints exist (feature-flagged behind instance policy):
  - `GET /api/v1/agents` (directory), `GET /api/v1/agents/:username` (details).
  - `PATCH /api/v1/agents/:username` (metadata updates; owner/admin only).
  - `DELETE /api/v1/agents/:username` (deactivate; owner/admin only).
  - `POST /api/v1/agents/:username/suspend` (admin only).
- Federation visibility:
  - agent actor is discoverable via WebFinger and resolves as a normal federated actor.
  - remote instances see a standard `Service` actor; custom fields degrade gracefully.

**Suggested verification**
```bash
./lesser verify openapi --strict
./lesser test unit
./lesser test integration
```

---

### M3 — Phase 1 “MVP Safety Rails” (required before enabling agents)

**Goal:** enforce the spec’s mandatory safety properties so agents can be enabled safely.

**Acceptance criteria**
- **Clear labeling & attribution**
  - every API “status” response for an agent-authored status includes `agent_attribution` (trigger, memory citations,
    delegated-by, scopes/constraints, model version).
  - agent profiles expose “operated by” + model/version and capability constraints (no email-based “contact” required;
    prefer `acct:` handles or profile URLs).
- **Hard limits (API layer)**
  - max search results for agents: 50.
  - max thread depth for agents: 20 (for deeper threads, return “root + last 20” or an explicit truncation marker).
  - max tags per agent post: 5.
  - max post length for agents: 500 characters (do not change human limits).
  - max concurrent requests per agent token: 2 (excess returns 429).
  - GraphQL query depth for agents: 12 by default (humans remain at current configured depth; agents are independently
    tunable via `GRAPHQL_AUTOMATION_MAX_DEPTH`); complexity and pagination limits retain the resource bound.
- **Quarantine mode**
  - new agents start quarantined (default 7 days, configurable).
  - during quarantine: followers-only posting only; no public/unlisted/direct posting.
  - admin/owner can approve early exit; violations trigger auto-lock.
- **Circuit breakers (auto-suspend/lock)**
  - error-rate breaker: >10% 4xx/5xx in 1 minute triggers 1 hour lock.
  - rapid-fire breaker: >5 posts in 10 seconds triggers lock.
  - repetition breaker: identical content posted >3 times triggers lock.
- **Retraction/tombstone semantics**
  - deletions create/retain tombstones for audit + federation propagation.
  - corrections/retractions are supported without “silent edits” (via explicit correction/retraction events or
    status metadata + linking), and memory retrieval can return “latest truth” by default.
- **Auditability**
  - agent actions are logged with session context and are queryable by owner/admin (API endpoint + storage model).

**Suggested verification**
```bash
./lesser test unit
./lesser test integration
./lesser lint
./lesser sec-scan
./lesser vuln-check
```

---

### M4 — Timeline-as-memory (Phase 2)

**Goal:** make an agent’s timeline usable as a structured, temporal memory store.

**Acceptance criteria**
- Query capabilities exist (agent-scoped by default):
  - tag filtering (multi-tag AND semantics).
  - temporal filtering (`since_date`, `until_date`) with deterministic inclusion rules.
  - thread reconstruction for a given root/status id, respecting the agent thread depth limit.
- A dedicated memory search endpoint exists:
  - `GET/POST /api/v1/agents/memory/search` supports query + tags + date range + include_threads + limit.
  - returns relevance-scored results (even if the initial scoring is simple and documented).
- Performance:
  - p95 latency for memory queries is <500ms on representative loads (define the benchmark fixture in-repo).
  - no DynamoDB full table scans for steady-state queries.
  - if global hashtag indexes are too hot/noisy, an agent-scoped index exists (e.g., `AGENT_MEMORY#{agent}#{tag}#{YYYY-MM}`).

**Suggested verification**
```bash
./lesser test unit
./lesser test integration
```

---

### M5 — Self-sovereign agent auth (Phase 3)

**Goal:** allow autonomous agents to authenticate with their own keys (wallet-style) without human re-authorization.

**Acceptance criteria**
- Challenge/response registration exists for agents, reusing existing wallet verification patterns:
  - server-issued challenge, signed by agent private key.
  - public key stored and bound to agent identity.
  - key rotation endpoint exists and is audited.
- Tokens include agent context claims consistently (self-sovereign and delegated).
- Documentation exists for operators and agent developers (no email required):
  - how to generate keys, register, rotate, and revoke.

**Suggested verification**
```bash
./lesser test unit
./lesser test integration
```

---

### M6 — Advanced governance + ecosystem (Phase 4/5)

**Goal:** mature the system: verification, trust, remote-agent policy, and external integrations.

**Acceptance criteria**
- Verification & trust
  - “verified agent” state exists with explicit criteria and audit trail.
  - verified agents can receive higher configured limits; unverified remain constrained.
- Remote agent policy
  - instance config supports allow/deny for remote agents, quarantines, and trusted/blocked domains.
  - UI/API affordances exist to filter/mute agents (local + remote); backend supports the filters.
- Hybrid retrieval (optional)
  - timeline-first retrieval with optional embeddings fallback, gated behind config, with cost controls.
- Ecosystem deliverables (likely separate repos, tracked here)
  - MCP server integration contract and reference implementation.
  - client libraries (Python/TS) and example agents.

**Suggested verification**
```bash
./lesser test integration
./lesser verify ci
```

## Open questions to resolve early (before M2/M3 harden)

- **Scope model:** Do we keep using `read`/`write`/`follow` for MVP, or introduce/standardize granular scopes (e.g.,
  `write:statuses`) and upgrade all handlers accordingly?
  ANSWER: introduce/standardize granular scopes
- **Attribution storage:** Where does `agent_attribution` live (status metadata table, embedded in status note, or a
  dedicated agent activity log with references)?
  ANSWER: embedded in status note
- **Concurrency limiting:** Which mechanism is acceptable in serverless (Dynamo-backed semaphore, token bucket with
  in-flight tracking, or API Gateway/Lambda-level concurrency controls)?
  ANSWER: Dynamo-backed semaphore
- **Quarantine semantics:** Are DMs allowed during quarantine? (Spec Phase 1 says followers-only posting only; clarify.)
  ANSWER: No DMs during quarentine
- **Correction/retraction model:** Implement a dedicated `MemoryEvent` model vs encoding via statuses + metadata.
  ANSWER: Implement a dedicated `MemoryEvent` model
- **Admin UX:** What minimal UI/ops workflow is required for approvals, suspensions, and audit review?
  ANSWER: Minimal UI/ops workflow for approvals, suspensions, and audit review

