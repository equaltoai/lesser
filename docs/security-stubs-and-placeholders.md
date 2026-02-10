# Security-Sensitive Stubs & Placeholders (Living)

Date: 2026-02-07  
Repo: `lesser`  

This document tracks **security-sensitive** placeholder or stub implementations that must be either:

- implemented correctly, or
- explicitly gated (auth/role/feature flag) so they cannot be abused while incomplete.

It is intentionally narrower than “all stubs in the repo” and focuses on anything that impacts:
- authentication / authorization / roles
- privacy & visibility enforcement
- HTML rendering / content sanitization
- federation controls / budgets / instance operations
- streaming access controls

Related docs:
- Findings inventory: `docs/security-gaps.md`
- Remediation plan: `docs/security-remediation-roadmap.md`

---

## How to find new candidates (repeatable commands)

Start broad, then triage for “reachable from network” + “security impact”.

### 1) Search for obvious markers

```bash
rg -n "TODO|FIXME|stub|placeholder|For now, return|In production" cmd pkg graph
```

### 2) Find handlers/resolvers that ignore context/auth entirely

GraphQL mutations that ignore `ctx` (high risk by default):

```bash
rg -n "func \\(r \\*mutationResolver\\) \\w+\\(_ context\\.Context" graph
```

GraphQL queries that ignore `ctx`:

```bash
rg -n "func \\(r \\*queryResolver\\) \\w+\\(_ context\\.Context" graph
```

### 3) Find “return empty” / “hard-coded” responses in network surfaces

```bash
rg -n "return \\[\\]\\*|return \\[\\]\\w+\\{}|return &model\\.|return \\w+\\{[^}]*\\}" graph/query_resolvers_*.go graph/mutation_resolvers_*.go cmd
```

### 4) Triage rubric (when to add an entry)

Add an entry here if **any** are true:
- reachable from a public or authenticated API surface (REST/GraphQL/streaming/oEmbed/HTML)
- controls security posture (authz, visibility, moderation, budgets, federation limits)
- claims to enforce a security rule but currently doesn’t (or explicitly says it’s a placeholder)

---

## Tracked items

Columns:
- **Risk:** P0 (critical) → P3 (low)
- **Status:** open / gated / fixed

| Item | Location | Why it matters | Risk | Status | Next step |
| --- | --- | --- | ---: | --- | --- |
| Unauth + stub federation control mutations | `graph/mutation_resolvers_federation.go:16` `:42` `:59` `:71` `:83` | Sensitive “control plane” style operations (limits/budgets/pause) must never be callable without admin gating, even if currently stubbed. | P0 | gated | Keep `requireAdmin` gating + tests; implement real persistence or remove/disable placeholder behavior before expanding exposure. |
| Public placeholder instance metrics | `graph/query_resolvers_federation.go:23` | Returns internal-ish operational/cost info; currently callable without auth and uses placeholder values (“In production…”). | P1 | gated | Keep admin-only; implement real metrics or remove until implemented. |
| Public placeholder federation health/limits | `graph/query_resolvers_federation.go:128` `:137` | Placeholder “return empty” responses are easy to ship unintentionally; combined with missing auth they create an unaudited public surface. | P1 | gated | Keep admin-only; implement real health/limits or remove/disable until real. |
| List-stream membership validation missing | `cmd/sse/main.go:236` | Any authenticated user can subscribe to any list stream by list ID unless membership is checked; lists often imply private curation and can leak content. | P0 | fixed | — |
| Inbox interaction cleanup placeholder | `cmd/inbox/internal/routing/inbox.go:2605` | Inbox processing includes a placeholder for “comprehensive interaction cleanup”; incomplete cleanup can leave inconsistent moderation/privacy state across deletes/blocks/mutes. | P2 | open | Implement the cleanup or document why it is safe to omit; add tests for interaction cleanup invariants. |
| VPN detection placeholder (device fingerprinting) | `pkg/auth/device_fingerprinting.go:581` | Weakens anomaly detection; not an auth bypass by itself, but can reduce effectiveness of security controls built on device risk scoring. | P2 | open | Either implement real detection (or integrate a service) or remove the “signal” from enforcement decisions until reliable. |
| Audit file sink placeholder | `pkg/auth/audit.go:568` | `storeToFile` is a placeholder sink; if enabled/relied on, it can create a false sense of audit durability. | P3 | open | Decide whether file-based audit storage is supported; if so implement (rotate + permissions), otherwise remove/disable the sink and document expected audit persistence. |
| Session extension count + cleanup placeholders | `pkg/auth/session_lifecycle.go:267` `:274` | Session lifetime enforcement may drift from intended policy if extension count/cleanup is never implemented; can unintentionally increase session persistence. | P2 | open | Implement extension-count persistence (or remove the config knob) and ensure cleanup semantics are covered by tests. |

---

## Completion checklist (for closing an entry)

An item can be marked **fixed** only if:
- the placeholder/stub behavior is removed or made correct, and
- access is gated appropriately (auth/role/visibility), and
- there is a regression test that would fail if the stub returns or gating is removed, and
- any public surface changes are reflected in the public-surface policy (see roadmap Milestone 0).
