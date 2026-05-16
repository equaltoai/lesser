# Hosted Bound Souls + x402 Boundary Contract

Date: 2026-05-16  
Repo: `lesser`  
Tracking issue: `equaltoai/lesser#985`  
Parent milestone: `equaltoai/lesser-host#308`

This document freezes the Lesser-owned boundary constraints for Project 32 M1.
It is intentionally a contract document, not an implementation plan for public x402 invocation.

## M1 scope and non-goals

M1 defines what Lesser must preserve while `lesser-host` and `lesser-body` prepare hosted-bound-soul and x402 policy
enforcement.

M1 does **not**:

- implement public x402 invocation in Lesser
- change OAuth token issuance, scope semantics, or federation authorization
- make hosted, offchain, immutable, or onchain status a capability gate inside Lesser
- expose private communications reachability, payment evidence, tenant data, wallet material, or unresolved security
  details in public responses or logs

## Terms

- **Hosted/offchain status**: registry, tenancy, deployment, or policy state owned by `lesser-host` that describes how a
  soul or hosted actor is managed.
- **x402/payment evidence**: payment challenge, receipt, entitlement, or paid-caller signal evaluated by `lesser-host` or
  a future payment-aware capability layer.
- **Principal authority**: the identity Lesser accepts for a request. In Lesser today that means a local authenticated
  account, an OAuth client/token subject, an operator/admin role, or a verified ActivityPub actor for federation input.
- **Tool/capability authority**: permission to call MCP tools or agent capabilities. The runtime and filtering layer is
  owned by `lesser-body`, even when the routes are fronted through a Lesser deployment.
- **Explicit scoped grant**: a future M2 grant that binds caller, resource, scopes/capabilities, issuer, expiry, and
  revocation semantics. Payment evidence alone is not an explicit scoped grant.

## Normative boundary rules

1. Hosted/offchain status is descriptive state, not Lesser principal authority.
2. x402/payment evidence is not a substitute for OAuth bearer authentication, OAuth consent, local admin/operator role,
   ActivityPub HTTP Signature verification, or AgentGovernanceState scope state.
3. A remote federated actor does not become a paid tool caller by sending an ActivityPub activity, including an activity
   with hosted/offchain, payment, wallet, or soul-shaped extension fields.
4. Public paid callers cannot call tools or agent capabilities before a later M2 explicit scoped grant exists and is
   enforced by the appropriate runtime boundary.
5. Lesser must continue to apply its existing default-deny public-surface policy: only explicitly public reads are
   unauthenticated, and public reads never imply write, admin, operator, or tool authority.
6. Lesser-owned logs and public responses must not expose private communications reachability, raw payment evidence,
   tenant data, wallet material, signing keys, bearer tokens, or unresolved security details.

## Lesser-owned entrypoint constraints

| Surface | Entrypoints | Authority accepted by Lesser | M1 constraint |
| --- | --- | --- | --- |
| ActivityPub discovery and public reads | `GET /.well-known/webfinger`, `GET /.well-known/nodeinfo`, `GET /nodeinfo/2.0`, `GET /users/{username}`, public `GET /objects/{id}`, `GET /users/{username}/outbox`, `GET /users/{username}/followers`, `GET /users/{username}/following`, `GET /users/{username}/liked` | Public read plus ActivityPub visibility rules; authorized fetch remains ActivityPub-level authentication | Hosted/offchain or payment fields may be rendered only as safe descriptive metadata where already contracted. They must not grant OAuth, admin, operator, or tool authority. Actor URL, inbox/outbox URLs, WebFinger identifiers, and public-key serialization remain stable. |
| Inbound federation | `POST /users/{username}/inbox` and shared inbox `POST /inbox` | Verified remote ActivityPub actor via HTTP Signatures, plus local moderation and instance/domain policy | Inbound federation remains social-protocol input. A verified remote actor can deliver valid ActivityPub activities, but cannot become a Lesser OAuth principal or paid tool caller without a later explicit scoped grant. No HTTP Signature bypass is introduced for hosted/offchain or payment claims. |
| Outbound federation | actor outbox publication, `POST /users/{username}/outbox` where local actor authorization applies, and `federation-delivery` signed delivery | Local actor account and actor signing key material | Outbound ActivityPub identity continues to be the Lesser actor and its signing key. Hosted/offchain status does not change signer identity, key ownership, delivery retry policy, or domain-block enforcement. |
| Mastodon-compatible REST | Public `/api/v1/*` reads, authenticated `/api/v1/*` writes/interactions, moderation/admin routes | Public-read rules, OAuth bearer token subject, scopes, and local role checks | Payment evidence or hosted status must not upgrade a public caller into a user, moderator, admin, operator, or tool caller. REST error and response shapes remain Mastodon-compatible unless a future contract change is separately reviewed. |
| OAuth registration and token flow | `POST /oauth/register`, `POST /api/v1/apps`, `GET /oauth/authorize`, `POST /oauth/token`, `POST /oauth/consent`, metadata under `/.well-known/oauth-*` | Registered OAuth client, local account consent, resource binding, token scopes | Public registration continues to create generic OAuth clients, not hosted-agent or paid-caller principals. `admin` remains internal-only. Tokens are not issued or upgraded from hosted/offchain status or x402/payment evidence in M1. |
| Actor-scoped MCP frontage | `GET/POST/DELETE /mcp/{actor}`, legacy `/mcp`, `GET /.well-known/mcp.json`, `GET /.well-known/oauth-protected-resource/mcp/{actor}` when `bodyEnabled` wiring is enabled | Resource-bound OAuth token to the actor-scoped MCP URL; MCP runtime policy in `lesser-body` | Lesser may front these routes, but tool/capability decisions belong to `lesser-body`. Public paid callers and federated actors must not gain MCP capability authority unless M2 defines and enforces explicit scoped grants. |
| GraphQL | Public-read subset of `POST/GET /graphql` and `POST/GET /api/graphql`; authenticated, mod/admin, and operator operations | Public-read allowlist, OAuth bearer token, and resolver role checks | Hosted/offchain status and payment evidence do not expand the anonymous GraphQL allowlist or satisfy authenticated, moderator, admin, or operator resolvers. |
| Streaming | `GET /api/v1/streaming/*`, streaming WebSocket channels | Existing public-stream allowance plus authenticated user-scoped stream rules | Payment evidence does not grant access to user, notification, direct, list, or other private streams. |
| Trust proxy public reads | `GET /api/v1/trust/jwks.json`, `GET /api/v1/trust/attestations`, `GET /api/v1/trust/attestations/{id}` | Public verification material only | Public trust reads remain verification/discovery material. They are not a grant surface and must not leak private hosted-tenant, payment, or wallet material. |
| Bootstrap, config, moderation, and admin operations | `/setup/*`, `/api/v1/admin/*`, `/api/v1/moderation/*`, operator configuration endpoints | Local setup state, authenticated admin/moderator/operator authority | Hosted/offchain status and payment evidence are never sufficient for operator, admin, moderator, bootstrap, or configuration authority. |

## Repository responsibility split

### Lesser owns

- ActivityPub actor identity, actor object shape, inbox/outbox handling, WebFinger, NodeInfo, objects, collections, and
  HTTP Signature signing/verification.
- Mastodon-compatible REST, GraphQL, streaming, and OAuth authorization-server behavior.
- OAuth scope validation and token issuance for Lesser-owned resources, including the public scope catalog documented in
  `docs/specs/oauth-scope-model.md`.
- The actor-scoped public MCP resource contract documented in `docs/specs/mcp-actor-url-auth-contract.md` and
  `docs/mcp-remote-access.md`, while the runtime Lambda is imported from `lesser-body` when enabled.
- Local account identity and `AgentGovernanceState` rows for delegation, quarantine, verification, and self-scope state.
- Public-surface default-deny, ActivityPub visibility filtering, moderation/domain blocking, and redacted audit logging.

### Host owns

- Hosted/offchain bound-soul registry state, tenant/deployment metadata, managed-update orchestration, and managed-host
  policy vocabulary.
- x402/payment verification, paid-caller entitlement evaluation, wallet/payment evidence handling, and public caller
  policy material before it crosses into Lesser or Body contracts.
- Trust material publication that Lesser exposes as public verification reads, such as managed trust JWKS and
  attestations.
- Deciding whether a future M2 scoped grant should be issued. Host issuance alone is not sufficient unless Lesser/Body
  also recognize the grant shape that M2 defines.

### Body owns

- MCP runtime execution, tool inventory, capability filtering, and scope/profile enforcement.
- Rejecting MCP tool calls that rely only on payment evidence, hosted/offchain status, or ActivityPub actor identity.
- Enforcing any future M2 explicit scoped grant for public paid callers before invoking tools.
- Private-data filtering for tool responses, including `drone` versus `souled` profiles or successor policy profiles.

## M2 grant preconditions

Before any M2 work lets a public paid caller reach tools or capabilities, the grant must be explicit and bounded.
At minimum, the cross-repo contract must define:

- grant issuer and verification material
- caller identity and how it is proven outside ActivityPub social delivery
- target actor/resource binding
- allowed scopes/capabilities and whether they map to OAuth scopes, Body capability scopes, or both
- expiry, revocation, and replay protections
- audit events for grant issuance, acceptance, use, denial, and revocation
- failure behavior when Host, Body, or Lesser cannot validate the grant
- redaction requirements for logs and public responses

Until that contract exists, Lesser treats federated actors, unauthenticated public callers, hosted/offchain state, and
payment evidence as insufficient for tool/capability authority.

## Contract and federation audit

- Mastodon REST compatibility: no endpoint, request, response, status-code, or error-shape changes in M1.
- GraphQL compatibility: no schema changes in M1.
- ActivityPub compatibility: no actor object, inbox/outbox URL, WebFinger, object, collection, signing, or verification
  changes in M1.
- Federation trust: HTTP Signature verification remains mandatory for inbound activities, and outbound delivery remains
  signed by the appropriate Lesser actor key.
- Schema: no DynamoDB PK/SK/GSI/TableTheory-tag changes in M1.
- Deployment: docs-only contract; standard release and deploy discipline applies if this rides in a Lesser release.

## Related Lesser contracts

- `docs/soul.md`
- `docs/security-public-surface.md`
- `docs/specs/oauth-scope-model.md`
- `docs/specs/mcp-actor-url-auth-contract.md`
- `docs/mcp-remote-access.md`
- `docs/architecture/adr/0007-agent-governance-state-contract.md`
