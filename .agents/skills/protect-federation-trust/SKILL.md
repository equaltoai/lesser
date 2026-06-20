---
name: protect-federation-trust
description: Use when a change touches the federation-trust surface — HTTP Signature signing or verification, actor identity resolution, inbox/outbox handling, delivery logic, governance state (AgentGovernanceState), moderation gates, relay behavior, or instance/domain blocking. Walks the trust impact across every federation code path before the change proceeds.
---

# Protect federation trust

Federation trust is lesser's most load-bearing non-negotiable surface. Every activity that crosses an instance boundary carries trust assumptions; if those assumptions break, lesser stops being a federated platform and becomes a broken one. Remote peers (Mastodon, Pleroma, Misskey, GoToSocial, and other ActivityPub-compliant servers) depend on lesser signing what it sends, verifying what it receives, and enforcing its own governance consistently.

This skill is the walk that makes federation-trust-adjacent changes safe to land.

## The federation-trust surface (memorize)

- **`pkg/federation/httpsig_enhanced.go`** — HTTP Signature signing and verification. RSA + Ed25519. The most security-critical file in the repo.
- **`pkg/federation/inbox.go`** — inbound activity parsing, signature verification, routing to processors. Every inbound activity flows through here.
- **`pkg/federation/enhanced_retry.go`** — outbound delivery retry with circuit breaker. Keeps federation working during remote outages without hammering them when genuinely down.
- **`pkg/federation/relay.go`** — optional ActivityPub relay integration for discovery/optimization.
- **`pkg/federation/cost/`** — per-domain cost tracking. Informs trust/budget decisions and lesser-host's broader trust model.
- **`pkg/agents/`** — AgentGovernanceState management (delegation, quarantine, verification, key rotation).
- **`pkg/auth/`** — authentication surfaces that intersect federation (actor keypair management, instance-key handling).
- **`cmd/inbox/`** — `POST /users/{username}/inbox` Lambda. The primary inbound federation surface.
- **`cmd/outbox/`** — `GET /users/{username}/outbox` Lambda.
- **`cmd/actor/`** — `GET /users/{username}` Lambda. Returns actor object with inbox, outbox, followers, following, and **public key**. The public key is how remote servers verify our signatures.
- **`cmd/federation-delivery/`** — SQS-triggered outbound delivery Lambda. Signs every outbound request.
- **`cmd/webfinger/`** — `/.well-known/webfinger` discovery.

## When this skill runs

Invoke this skill when:

- A change modifies signing or verification logic (algorithm selection, header inclusion, signature format)
- A change modifies actor object shape (inbox / outbox / followers / following URLs, publicKey block, endpoints, `delegated_by` field)
- A change modifies inbound activity parsing or routing (inbox handler, signature verification gate)
- A change modifies outbound delivery (retry logic, circuit breaker, signing, timeout, redirect handling)
- A change modifies AgentGovernanceState (delegation, quarantine, verification, key rotation, scope grants)
- A change modifies moderation gates (instance blocks, actor blocks, mute, suspend)
- A change modifies relay integration
- A change touches actor keypair generation, storage, or rotation
- An operator reports federation failure traced to a specific remote peer or activity type
- A CVE affects ActivityPub, HTTP Signatures, or federation-adjacent crypto libraries

## Preconditions

- **The change is described concretely.** "Improve signature verification" is too vague; "when verifying an inbound activity whose signature uses `hs2019` algorithm with Ed25519, accept the signature even when the `Digest` header uses SHA-512 instead of SHA-256" is concrete.
- **MCP tools healthy**, `memory_recent` first — federation work accrues per-remote-peer findings worth continuity.
- **Reproduction path is clear** — a specific remote peer, a specific activity shape, a specific operator deployment. Federation-trust work against abstract "what if" rarely lands cleanly.
- **For security-response work**, the CVE / vulnerability characterization is ready.

## The five-dimension walk

### Dimension 1: Signing impact

- **Does the change affect how lesser signs outbound activities?** Algorithm selection (rsa-sha256, hs2019 with Ed25519), header set included in the signature (Request-Line, Host, Date, Digest, minimum recommended), key identifier format.
- **Does the change affect signing for a specific activity type or for all outbound traffic?**
- **Does the change maintain compatibility with Mastodon's expectations?** Mastodon's signature verification is effectively the de-facto reference; changes that break Mastodon-compat are federation-breaking.
- **Is the instance keypair or actor keypair affected?** Instance-level signing (rare) vs per-actor signing (typical).

### Dimension 2: Verification impact

- **Does the change affect how lesser verifies inbound signatures?** Algorithm acceptance, header-set expectations, clock skew tolerance, key-fetching behavior.
- **Does the change add a verification bypass?** Refuse. Unsigned or invalid-signature activities are always rejected.
- **Does the change tighten verification** (e.g. reject activities missing the `Digest` header)? Preserves trust but may break peers that don't include it; evaluate federation-peer impact.
- **Does the change relax verification for interoperability with a specific peer?** Usually refuse; if genuinely warranted (the peer is following a valid alternative FEP), document the exception explicitly and alarm on its use.

### Dimension 3: Actor-object / keypair impact

- **Does the change affect actor object shape?** `inbox`, `outbox`, `followers`, `following`, `publicKey` (the load-bearing one), `endpoints`, `preferredUsername`, `type`, `attributedTo`, `delegated_by`.
- **Does the change affect `publicKey` serialization?** The `publicKey.publicKeyPem` field must be ASCII PEM, with the actor URL as `publicKey.id`. Formatting changes cascade into verification failures on remote servers.
- **Does the change affect keypair generation, storage, or rotation?** Keypairs live encrypted in `AccountKeys` rows. Rotation requires coordinated update of actor-object publication + signature transition.
- **Does the change affect WebFinger discovery?** `acct:username@domain` → actor URL resolution.

### Dimension 4: Delivery / inbox impact

- **Does the change affect outbound delivery?** Retry semantics, circuit breaker, DLQ handling, timeout, redirect following, cost tracking, rate limiting per domain.
- **Does the change affect inbound handling?** Activity parsing, signature verification gate, routing to processors, rejection semantics, rate limiting.
- **Does the change affect relay integration?** Outbound relay, inbound relay, opt-in policy.
- **Does the change affect how domain / instance blocks are enforced?** Block policy must be enforced on both inbound (reject activities from blocked instances) and outbound (never deliver to blocked instances).

### Dimension 5: Governance-state impact

- **Does the change touch AgentGovernanceState?** Delegation scopes, quarantine status, verification state, key rotation tracking. This row is separate from Account (PK = `USER#{username}`, SK = `AGENT_GOVERNANCE`); changes here gate future managed-agent workflows.
- **Does the change modify how governance interacts with federation?** A quarantined actor's outbound activities behave differently from an unquarantined one; changes to that behavior affect what remote peers see.
- **Does the change affect scope-based capability gates?** Managed agents operate under delegation scopes; scope changes propagate into what activities they can author.
- **Does the change affect moderation tooling?** Block / mute / suspend at account level; instance-level blocks and silences. Moderation actions must be immediately enforced.

## The audit output

```markdown
## Federation-trust audit: <change name>

### Proposed change
<concrete description>

### Reproduction / driver
<specific remote peer, activity type, operator-reported symptom, CVE, or FEP adoption>

### Signing impact
- Outbound signing affected: <yes / no>
- Algorithms / headers changed: <...>
- Mastodon-compat preserved: <yes — verified / no — breaking explanation>
- Instance vs actor signing scope: <...>

### Verification impact
- Inbound verification affected: <yes / no>
- Verification tightened / relaxed: <...>
- Bypass introduced: <no — default; if yes, refuse unless explicitly authorized with audit>
- Peer compatibility impact: <...>

### Actor-object / keypair impact
- Actor object shape changed: <no / additive field / breaking>
- publicKey serialization changed: <no / yes — impact>
- Keypair generation / storage / rotation affected: <no / yes — plan>
- WebFinger / discovery affected: <no / yes — impact>

### Delivery / inbox impact
- Outbound delivery affected: <no / retry / circuit breaker / rate limit / DLQ>
- Inbound handling affected: <no / parsing / verification / routing / rejection>
- Relay integration affected: <no / yes — impact>
- Domain / instance block enforcement: <preserved / changed>

### Governance-state impact
- AgentGovernanceState touched: <no / yes — delegation / quarantine / verification / key-rotation>
- Scope-based capability gates affected: <no / yes>
- Moderation tooling affected: <no / yes>
- Enforcement immediacy preserved: <yes — required>

### Test coverage
- Signing tests: <added / existing>
- Verification tests: <added / existing — include known-good and known-bad signature fixtures>
- Delivery retry / circuit breaker tests: <added / existing>
- Inbound rejection tests: <added / existing>
- Moderation enforcement tests: <added / existing>

### Peer-compatibility verification
- Mastodon compatibility: <verified against test fixture / verified against test server / pending>
- Other AP implementations (Pleroma, Misskey, GoToSocial): <verified / not in scope / pending>

### Audit-log implications
<what audit events are emitted; retention; format>

### Rollout stance
- Standard dev → staging → live cadence: <yes>
- Compressed (for security urgency): <no / yes with authorization>
- Post-deploy monitoring additions: <signature verification failure rate, DLQ depth, per-domain delivery success rate>

### Proposed next skill
<enumerate-changes if audit clean; scope-need if audit surfaces scope growth; investigate-issue if an existing bug is revealed; coordinate-framework-feedback if the friction is in a crypto library or framework>
```

## Refusal cases

- **"Skip HTTP Signature verification for a specific peer."** Refuse. Verification is not optional.
- **"Accept unsigned activities from our own domain."** Refuse. Self-signed activities still carry the same trust assumptions.
- **"Cache verification results for 24 hours to reduce public-key fetches."** Acceptable with short TTLs that respect key rotation; a 24-hour cache risks accepting activities after a key has been rotated due to compromise. Evaluate the TTL against federation-peer rotation norms.
- **"Log the raw Authorization / Signature header so we can debug."** Refuse. Signature material can be sensitive; use redacted or hashed forms.
- **"Log the full actor private key for one-time debugging."** Never. Private keys never appear in logs under any circumstance.
- **"Skip the circuit breaker to deliver faster."** Refuse. Circuit breaker protects remote peers from runaway retry storms.
- **"Deliver to blocked instances once 'to close the follow loop.'"** Refuse. Block enforcement is absolute.
- **"Relax verification algorithm acceptance to work with this one old Pleroma server."** Generally refuse. If the algorithm is genuinely FEP-compliant and the Pleroma version is widely deployed, evaluate. Otherwise the old server can upgrade.
- **"Skip emitting audit events for inbound rejections."** Refuse. Audit events for rejection are part of operator observability.
- **"Make the keypair storage not encrypted; it's only in DynamoDB."** Refuse. Keypairs at rest are encrypted.

## Persist

Append when the audit surfaces a recurring pattern — a specific remote-peer quirk, a FEP adoption decision, a verification edge case that matters for future changes, a governance-state subtlety worth remembering. Routine audits that resolve cleanly aren't memory material. Five meaningful entries beat fifty log-shaped ones.

## Handoff

- **Audit clean, additive change** — invoke `enumerate-changes`.
- **Audit clean, with specific peer-compat coordination** — document the coordination, then `enumerate-changes`.
- **Audit surfaces scope growth** — revisit `scope-need`.
- **Audit reveals an existing federation-trust bug** — route through `investigate-issue`, then back here.
- **Audit reveals a crypto-library or framework friction** — invoke `coordinate-framework-feedback`.
- **Audit reveals a sibling-repo concern** (e.g. body / host expectations affected) — report cross-repo.
- **Audit identifies a security regression in the proposed change** — stop and refuse; surface to the user.