# ADR 0011: Server-side soul-binding ceremony API

**Status:** Accepted (2026-07-14)

## Context

Project 48 M7 introduces a cross-repo soul-binding ceremony for Ptah-created
agents. `lesser-body` and Ptah orchestrate the agent/capability side of the
ceremony, but Lesser remains the ActivityPub runtime and the owner of the local
soul/body binding rows.

The source-truth constraints in Lesser today are:

- `SOUL_BODY_BINDING#<agentId>` and `SOUL_BODY_BINDING_USERNAME#<username>` rows
  are modeled by `InstanceSoulBodyBinding` and
  `InstanceSoulBodyBindingUsername` under `pkg/storage/models/`.
- `InstanceRepository.BindSoulBody` is the canonical transactional writer. It
  creates both binding rows, treats the same soul/body pair as idempotent, and
  rejects soul-to-different-body or body-to-different-soul conflicts.
- `pkg/services/souls.Service.ResolveBoundAgent` and the existing GraphQL/REST
  identity surfaces derive the local souled/Ka state from those binding rows.
  That behavior must not regress.
- The hosted MicroVM/bootstrap path already binds hosted off-chain souls through
  `PublishHostedBootstrap` + `BindHostedBootstrap`. That path validates Host
  publication evidence and then uses the same local binding authority.
- `lesser-body` is the MCP/agent capability runtime. It may request a binding,
  but it must not write Lesser's DynamoDB table or bypass Lesser's validation.
- `lesser-host` is the hosted soul registry/control-plane source for hosted
  off-chain identity evidence. Lesser can accept body/Ptah's request only as an
  intent; Host registry evidence is the authority for hosted soul identity.

The ADR must give `lesser-body` a concrete API contract to build against while
preserving Lesser ownership of `SOUL_BODY_BINDING` records and preserving the
existing Ka/souled resolution contract.

## Decision

1. Add an additive, Lesser-owned REST contract for server-side soul binding:
   - `POST /api/v1/souls/bindings`
   - `GET /api/v1/souls/bindings/{agentId}`
2. Use a **single-call bind** with an authenticated status/read projection, not
   a durable initiate/poll ceremony.
3. Treat body/Ptah input as a binding request, never as storage authority.
   Lesser validates the local actor, validates hosted soul evidence against
   Host source truth, and writes `SOUL_BODY_BINDING` rows only through Lesser
   repository/service code.
4. Require an idempotency key for every bind request. Replays of the same caller,
   key, and payload return the current result; reusing the same key with a
   different payload is a conflict.
5. Keep `SOUL_BODY_BINDING` as the canonical binding truth. If implementation
   needs a request receipt to enforce replay semantics, that receipt is
   Lesser-owned, TTL-scoped control-plane evidence and not a replacement for the
   binding rows.
6. Require Host registry participation for Ptah-created hosted souls. Lesser
   must fail closed unless Host can prove that the requested soul is active,
   hosted/off-chain, belongs to this instance domain/local actor, and has the
   expected authority/binding fields.

## API contract

### Authentication and authorization

The binding API is server-to-server only.

- Caller: `lesser-body`/Ptah running for the same Lesser deployment.
- Credential: a deployment-scoped integration credential accepted by Lesser for
  the `soul-binding:write` and `soul-binding:read` capabilities. The concrete
  wiring is an implementation detail for lesser#1153 and lesser-host#696; it may
  reuse an existing instance-trust credential only if the implementation proves
  the scope is correct, otherwise it must use a dedicated Body-to-Lesser binding
  credential.
- The credential is never exposed to browsers, Mastodon clients, ActivityPub
  peers, or user OAuth clients.
- User OAuth bearer tokens are not sufficient for this endpoint. The existing
  wallet/user incorporation flow remains `/api/v1/souls/{agentId}/incorporate`.
- Lesser authorizes the request against the local instance and the target local
  actor. The body caller cannot choose a remote domain and cannot bind a local
  actor that Lesser would not treat as a local agent.

Lesser must reject requests when:

- the integration credential is missing, invalid, or not scoped for binding;
- the target actor does not exist, is not an agent actor, or is outside the local
  instance;
- the requested soul identity is not proven by Host as an active hosted/off-chain
  identity for this instance and actor;
- the existing local binding rows already bind either side to a different peer.

### `POST /api/v1/souls/bindings`

Creates or confirms a local binding between a Host soul agent ID and a Lesser
local agent username.

Required headers:

```http
Authorization: Bearer <body-to-lesser integration credential>
Content-Type: application/json
Idempotency-Key: 01JZPTAHBIND6V6V2X8KX5H6J63
```

Optional headers:

```http
X-Correlation-Key: ptah-ceremony-2026-07-14T16:20:00Z-ada
```

Request body:

```json
{
  "actor_username": "drone-ada",
  "soul_agent_id": "0x8c4f8f05b4f1e71d8a0e06d4c6b26f3142a0cb28a1d98a8f5d36e0e5a4f2bb61",
  "body_actor_id": "body://ptah/drone-ada",
  "host_registration_id": "hreg_01JZPTHOSTREG",
  "host_conversation_id": "hconv_01JZPTHOSTCONV",
  "authority_model": "instance_trust",
  "anchor_state": "hosted_offchain",
  "operational_binding": "hosted_bound_soul",
  "principal_address": "0x1111111111111111111111111111111111111111",
  "evidence": {
    "source": "ptah",
    "host_request_id": "hreq_01JZPTHOSTREQ",
    "declaration_hash": "sha256:4c5835f5c2c84bcaadc17af3c5a5700fdd7f39fb7f61305b02d1a02a0e6c7c56",
    "issued_at": "2026-07-14T16:20:00Z"
  }
}
```

Field rules:

- `actor_username` is the local Lesser username to bind. Lesser normalizes and
  validates it through the same local account/agent lookup used by existing soul
  services.
- `soul_agent_id` is the Host soul agent ID. It must be canonical lowercase
  `0x` plus 64 hex characters, matching the validation already used by the soul
  service.
- `body_actor_id` is an optional body/Ptah correlation identifier. It is never a
  storage key and never overrides Lesser's local actor identity.
- `host_registration_id`, `host_conversation_id`, and `evidence` are correlation
  and audit hints. They are useful for Host registry lookup and logs, but they
  are not authority by themselves.
- `authority_model`, `anchor_state`, and `operational_binding` must match the
  hosted/off-chain values Lesser already accepts for hosted bootstrap:
  `instance_trust`, `hosted_offchain`, and `hosted_bound_soul`.
- `principal_address` may be supplied as a hint, but Lesser records only the
  canonical principal address returned or confirmed by Host registry evidence.

Successful response (`200 OK`):

```json
{
  "version": "1",
  "status": "bound",
  "binding_state": "bound",
  "agent": {
    "agent_id": "0x8c4f8f05b4f1e71d8a0e06d4c6b26f3142a0cb28a1d98a8f5d36e0e5a4f2bb61",
    "domain": "example.com",
    "local_id": "drone-ada",
    "authority_model": "instance_trust",
    "anchor_state": "hosted_offchain",
    "operational_binding": "hosted_bound_soul",
    "lifecycle_status": "active"
  },
  "binding": {
    "agent_username": "drone-ada",
    "principal_address": "0x1111111111111111111111111111111111111111",
    "bound_at": "2026-07-14T16:20:02Z",
    "updated_at": "2026-07-14T16:20:02Z"
  },
  "idempotency": {
    "key": "01JZPTAHBIND6V6V2X8KX5H6J63",
    "replayed": false,
    "payload_hash": "sha256:83f60fdf32b7483a2a13f2fa1cf02f0dbed32dd4ef405cfd8b51b17ce3d7f00c"
  },
  "links": {
    "status": "/api/v1/souls/bindings/0x8c4f8f05b4f1e71d8a0e06d4c6b26f3142a0cb28a1d98a8f5d36e0e5a4f2bb61"
  }
}
```

Replay response for the same caller, idempotency key, and canonical payload is
also `200 OK`; the response sets `idempotency.replayed` to `true` and returns the
current binding projection. If the binding was completed by the first request but
the request receipt has expired, Lesser still treats the same soul/body pair as
idempotently bound by reading `SOUL_BODY_BINDING` source truth.

### `GET /api/v1/souls/bindings/{agentId}`

Returns the Lesser-local binding projection for a soul agent ID. This is a
read/status endpoint for retry recovery and confirmation; it does not initiate a
ceremony.

Required header:

```http
Authorization: Bearer <body-to-lesser integration credential>
```

Optional query parameter:

- `actor_username`: when supplied, Lesser verifies that the binding is for that
  local actor and returns a conflict-shaped response if the soul is bound to a
  different local actor.

Successful bound response (`200 OK`):

```json
{
  "version": "1",
  "status": "bound",
  "binding_state": "bound",
  "agent": {
    "agent_id": "0x8c4f8f05b4f1e71d8a0e06d4c6b26f3142a0cb28a1d98a8f5d36e0e5a4f2bb61",
    "domain": "example.com",
    "local_id": "drone-ada",
    "authority_model": "instance_trust",
    "anchor_state": "hosted_offchain",
    "operational_binding": "hosted_bound_soul",
    "lifecycle_status": "active"
  },
  "binding": {
    "agent_username": "drone-ada",
    "principal_address": "0x1111111111111111111111111111111111111111",
    "bound_at": "2026-07-14T16:20:02Z",
    "updated_at": "2026-07-14T16:20:02Z"
  }
}
```

Unbound response (`404 Not Found`):

```json
{
  "error": "Soul binding was not found",
  "error_description": "No Lesser-local soul/body binding exists for the requested soul agent ID.",
  "error_code": "SOUL_BINDING_NOT_FOUND"
}
```

## Single-call versus initiate/poll

The binding write is local, bounded, and already represented as a transaction in
`InstanceRepository.BindSoulBody`. The expensive/interactive ceremony work lives
outside this API: Ptah/body conducts the agent capability ceremony and Host owns
the hosted soul registry evidence. Lesser's responsibility at this boundary is to
validate those facts and atomically create the local binding rows.

For that reason, the API is single-call:

1. authenticate the body/Ptah caller;
2. validate request grammar and local actor identity;
3. verify Host registry evidence for the requested hosted soul;
4. write or confirm the local `SOUL_BODY_BINDING` rows;
5. return the binding projection.

A separate initiate/poll workflow would create another durable ceremony state in
Lesser, duplicate the hosted bootstrap state machine, and increase the chance of
stranding an agent between Host publication and local `complete.bound`. The
status endpoint is enough for retry recovery because the terminal source truth is
either the binding rows or an error response that caused no write.

If #1153 discovers a genuinely long-running Lesser-owned step, the extension
point is to add an explicit `202 Accepted` receipt state for this same endpoint,
not to let body write storage. Such a change would require a follow-up ADR or ADR
amendment because it adds a new Lesser-owned state machine.

## Mid-ceremony states and replay semantics

The API exposes one terminal success state, `bound`, and several failure states.
The following states may be recorded in a Lesser-owned idempotency receipt and
may appear in structured logs/metrics, but only `bound` is persisted as binding
truth:

| State | Durability | Meaning | Replay behavior |
| --- | --- | --- | --- |
| `received` | receipt/log | Lesser accepted a syntactically valid request and reserved the idempotency key. | Same key/payload continues or returns latest known state. |
| `validating_local_actor` | receipt/log | Lesser is verifying the local actor exists and is an agent. | Same key/payload continues or returns latest known state. |
| `checking_host_registry` | receipt/log | Lesser is checking Host source truth for the hosted soul. | Same key/payload continues or returns latest known state; Host outage is retryable. |
| `binding_write` | receipt/log | Lesser is executing the local binding transaction. | Same key/payload may race; repository idempotency resolves the same pair to one binding. |
| `bound` | binding rows + optional receipt | `SOUL_BODY_BINDING` and username index rows exist for the same soul/body pair. | Same key/payload returns `200 OK` with `replayed: true`; same pair without a receipt is also success. |
| `rejected` | receipt/log | Request failed validation or authority checks and no binding was written. | Same key/payload returns the same class of error until inputs/source truth change. |
| `conflict` | receipt/log | Existing binding rows bind either side to a different peer. | Same key/payload returns conflict; changing the key does not bypass it. |
| `host_unavailable` | receipt/log | Host registry evidence could not be verified. No binding was written. | Retry with the same key is allowed. |

Replay rules:

- `Idempotency-Key` is required for `POST`.
- The idempotency scope is `(authenticated body caller, Idempotency-Key)`.
- Lesser canonicalizes the request payload before hashing. Whitespace and JSON
  object ordering do not change the payload hash; semantic changes do.
- Same scope + same payload hash returns the latest known outcome.
- Same scope + different payload hash returns `409 Conflict` with
  `SOUL_BINDING_IDEMPOTENCY_MISMATCH`.
- A different idempotency key does not bypass existing binding conflicts. The
  repository source truth still rejects soul/body conflicts.
- A completed same-pair binding is idempotent even after any request receipt TTL.
  Lesser reads the binding rows and returns `bound`.
- A retryable Host outage must not create a partial binding row.

## Error grammar

Errors use Lesser's standard REST error envelope:

```json
{
  "error": "Human readable summary",
  "error_description": "Actionable detail without secrets or raw Host credentials.",
  "error_code": "SOUL_BINDING_EXAMPLE_CODE"
}
```

Binding-specific codes:

| HTTP | `error_code` | Meaning |
| --- | --- | --- |
| 400 | `SOUL_BINDING_IDEMPOTENCY_KEY_REQUIRED` | `POST` omitted `Idempotency-Key`. |
| 400 | `SOUL_BINDING_INVALID_REQUEST` | JSON or required fields are malformed. |
| 400 | `SOUL_BINDING_AGENT_ID_INVALID` | `soul_agent_id` is not canonical `0x` + 64 hex. |
| 401 | `SOUL_BINDING_AUTH_REQUIRED` | Missing or invalid integration credential. |
| 403 | `SOUL_BINDING_FORBIDDEN` | Credential is valid but not authorized for this instance or capability. |
| 404 | `SOUL_BINDING_ACTOR_NOT_FOUND` | Target local actor does not exist. |
| 404 | `SOUL_BINDING_NOT_FOUND` | Status lookup found no local binding. |
| 409 | `SOUL_BINDING_IDEMPOTENCY_MISMATCH` | Same idempotency key was reused for a different payload. |
| 409 | `SOUL_BINDING_SOUL_CONFLICT` | Soul agent ID is already bound to a different local actor. |
| 409 | `SOUL_BINDING_BODY_CONFLICT` | Local actor is already bound to a different soul agent ID. |
| 422 | `SOUL_BINDING_ACTOR_INVALID` | Target local actor exists but is not eligible for binding. |
| 422 | `SOUL_BINDING_HOST_REGISTRY_REJECTED` | Host source truth does not prove the requested identity/binding. |
| 503 | `SOUL_BINDING_HOST_REGISTRY_UNAVAILABLE` | Host registry could not be reached or returned a retryable failure. |
| 500 | `SOUL_BINDING_INTERNAL` | Unexpected Lesser-side failure. |

Example idempotency mismatch:

```json
{
  "error": "Idempotency key was reused with a different soul-binding request",
  "error_description": "Reuse the original request payload or send a new idempotency key for a new ceremony attempt.",
  "error_code": "SOUL_BINDING_IDEMPOTENCY_MISMATCH"
}
```

Example Host registry rejection:

```json
{
  "error": "Host registry did not confirm the requested soul binding",
  "error_description": "The requested soul was not active for this Lesser domain and local agent.",
  "error_code": "SOUL_BINDING_HOST_REGISTRY_REJECTED"
}
```

Retryable `503` responses may include `Retry-After` when Lesser can derive a
safe retry interval. Error responses must not include Host bearer tokens,
instance API keys, wallet secrets, raw provider transcripts, or private key
material.

## Host soul-registry participation

Host participation is required for the Ptah-created hosted soul path. It is not
nil.

For lesser#1153 to implement this ADR without trusting body/Ptah assertions,
lesser-host H3 (#696) must provide, or confirm an existing endpoint already
provides, a stable read-only registry projection addressable by soul agent ID
and scoped to the Lesser instance credential. The projection must include at
least:

- `agent_id`;
- `domain` and `local_id`;
- canonical owner/principal address for the hosted soul;
- `authority_model` = `instance_trust`;
- `anchor_state` = `hosted_offchain`;
- `operational_binding` = `hosted_bound_soul`;
- active `status` and `lifecycle_status` values;
- publication version or equivalent evidence that the hosted soul is registered;
- request/correlation ID suitable for redacted audit logs;
- optional registration/conversation IDs when Host can connect them to the
  Ptah/body ceremony.

Lesser will use this Host projection as validation source truth before writing
`SOUL_BODY_BINDING`. Body/Ptah-provided evidence can help find or correlate the
Host record, but it cannot replace Host source truth.

Later extension point: Host may add a callback, reservation, or richer registry
attestation for ceremonies. Such extensions must still preserve the same
boundary: Host proves hosted soul identity; Body orchestrates capability
ceremony; Lesser writes local binding rows.

## Compatibility with Ka/souled resolution

This ADR does not change Ka resolution or public actor semantics.

- `ResolveBoundAgent` remains the local read boundary for determining whether a
  local agent is souled.
- GraphQL drone workflow identity semantics continue to derive `souled` and
  `BOUND` from the existing binding rows.
- `GET /api/v1/souls/bound/me` and existing hosted mint-conversation reads keep
  their current behavior.
- `/api/v1/souls/{agentId}/incorporate` remains the wallet/user-owned
  incorporation flow and is not replaced by this server-to-server endpoint.
- Hosted MicroVM/bootstrap flows continue to use Host declaration/publication
  evidence and `BindHostedBootstrap`; the new API must share validation/write
  semantics rather than introducing a conflicting binding path.
- Public ActivityPub actor objects, inbox/outbox URLs, HTTP signatures, Mastodon
  client responses, and public note attribution do not change.
- `SoulAgentID` remains private unless an existing authorized surface already
  exposes it to the local principal or server-side integration caller.

## Consequences

Positive:

- `lesser-body` gets a stable, buildable contract for Ptah-created agent
  binding without touching Lesser's table directly.
- Lesser remains the sole writer of `SOUL_BODY_BINDING` records.
- Existing repository idempotency and conflict semantics remain load-bearing.
- Host registry source truth prevents body/Ptah from binding arbitrary local
  actors to unproven soul IDs.
- The design avoids duplicating the hosted bootstrap state machine in another
  Lesser-owned initiate/poll flow.

Costs and risks:

- lesser#1153 may need a small Lesser-owned, TTL-scoped idempotency receipt
  model. Any such model is a schema change and must run through schema review.
- lesser-host#696 is a real dependency. If Host cannot provide the registry
  projection, the implementation must fail closed or restrict the endpoint to
  cases Lesser can already verify from Host source truth.
- A dedicated integration credential may require deployment/SSM wiring. It must
  not be implemented by reusing broad operator credentials in request logs or
  browser-visible configuration.
- The API is Lesser-specific, not a Mastodon-compatible endpoint. Future
  implementation must update `docs/contracts/openapi.yaml` with this additive
  contract.

## Downstream implementation checklist

### lesser#1153

- Add routes for `POST /api/v1/souls/bindings` and
  `GET /api/v1/souls/bindings/{agentId}`.
- Add request/response models matching this ADR and regenerate/update the
  OpenAPI contract.
- Implement server-to-server authentication/authorization for the body/Ptah
  integration capability.
- Validate `actor_username` through local account/agent lookup and reject
  non-agent targets.
- Validate `soul_agent_id` with the existing canonical soul agent ID rules.
- Fetch/confirm Host registry source truth for the requested soul and local
  actor before writing any binding rows.
- Call the same service/repository path that writes `SOUL_BODY_BINDING` and
  username-index rows transactionally; do not add a body-owned write path.
- Implement idempotency receipts or an equivalent Lesser-owned mechanism for
  replay/mismatch handling. If this adds a new row shape, run schema validation.
- Keep logs redacted: no private keys, instance credentials, Host bearer tokens,
  wallet secrets, raw transcripts, or raw provider responses.
- Add tests for success, same-key replay, same-pair replay after receipt expiry,
  idempotency mismatch, Host rejection, Host outage, soul conflict, body conflict,
  actor-not-found, and target-not-agent.
- Confirm existing Ka/souled GraphQL and REST read paths observe the new binding
  through existing `SOUL_BODY_BINDING` source truth.

### lesser-body#358 and lesser-body#359

- Treat Lesser's API as the only way to create a local soul/body binding.
- Generate and persist a stable `Idempotency-Key` per Ptah binding attempt.
- Send `actor_username`, canonical `soul_agent_id`, and Host correlation evidence
  in the request body.
- Treat `200` + `binding_state: bound` as success, including replayed success.
- Use `GET /api/v1/souls/bindings/{agentId}` only for retry recovery/status
  confirmation, not as a ceremony initiator.
- Handle `409` conflicts as operator/user-visible binding conflicts, not as
  retryable transport errors.
- Handle `422 SOUL_BINDING_HOST_REGISTRY_REJECTED` as failed Host evidence.
- Retry `503 SOUL_BINDING_HOST_REGISTRY_UNAVAILABLE` with the same idempotency
  key and bounded backoff.
- Never write Lesser DynamoDB rows directly and never forward browser/user tokens
  as the server-side integration credential.

### lesser-host#696

- Provide or identify the stable Host registry read projection Lesser can call by
  `soul_agent_id` under instance trust.
- Include the authority, anchor, operational binding, lifecycle, publication, and
  principal fields listed above.
- Return request IDs and safe error codes that Lesser can map into redacted
  `SOUL_BINDING_HOST_REGISTRY_*` responses.
- Preserve Host as registry source truth without requiring Host to write Lesser's
  local binding rows.

## References

- [lesser#1152](https://github.com/equaltoai/lesser/issues/1152)
- [lesser#1153](https://github.com/equaltoai/lesser/issues/1153)
- [lesser-body#358](https://github.com/equaltoai/lesser-body/issues/358)
- [lesser-body#359](https://github.com/equaltoai/lesser-body/issues/359)
- [lesser-host#696](https://github.com/equaltoai/lesser-host/issues/696)
- [Soul/body architecture notes](../../soul.md)
- [Hosted soul genesis projection contract](../../contracts/hosted-soul-genesis-projection.md)
- [Hosted bound souls x402 boundary](../../contracts/hosted-bound-souls-x402-boundary.md)
- [Instance soul/body binding model](../../../pkg/storage/models/instance_soul_body_binding.go)
- [Instance repository binding writer](../../../pkg/storage/repositories/instance_repository_soul_body_binding.go)
- [Soul service](../../../pkg/services/souls/service.go)
