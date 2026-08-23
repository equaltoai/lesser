# OAuth Refresh Reliability

Lesser's `/oauth/token` endpoint treats an OAuth error as a statement about
authority. Infrastructure failures must not be presented as evidence that a
grant, client, or refresh credential is invalid.

## Error decision table

| Evidence | Response | Client action |
| --- | --- | --- |
| Refresh token is authoritatively missing, expired, bound to another client/resource, or fails an authority revalidation | `400 invalid_grant` | Reauthorize |
| A consumed standard refresh token has a complete retained successor chain | `200`; a fresh access token plus the already-minted live refresh head | Continue with the returned head |
| A replay authority, encrypted successor artifact, budget pre-charge, or head integrity check cannot be completed | `503 temporarily_unavailable` with `Retry-After: 1` | Back off and retry the same request |
| Client is authoritatively missing or its presented authentication fails | `400 invalid_client` | Repair client configuration |
| Storage, throttling, transport, conditional-write, ambiguous-write, agent re-mint, or share-authorization read failure | `503 temporarily_unavailable` with `Retry-After: 1` | Back off and retry the same request |
| Three refresh CAS attempts are exhausted | `503 temporarily_unavailable` with `Retry-After: 1` | Back off and retry the same request |
| Share or owner revalidation authoritatively denies the principal | `400 invalid_grant` | Reauthorize |

Authorization-code and device-code issuance atomically consume the one-time
grant and create the refresh-token row. If that transaction fails, Lesser
returns no tokens and leaves no intentionally unbacked refresh credential.

## Stateless agent and machine re-mint

Dedicated agent-runtime clients receive a short-TTL signed access token only.
They re-prove their existing delegation, key challenge, or other mint-endpoint
authority and invoke that endpoint again. Re-minting is idempotent with respect
to refresh state: it performs no refresh create, update, rotation, or revocation
write. Infrastructure failures from every agent mint surface use the retryable
`503 temporarily_unavailable` taxonomy rather than a bare 500.

The migration is **honor until expiry**. Agent-runtime refresh credentials
issued before this contract remain usable until their persisted idle, absolute,
or token expiry. A valid legacy credential mints only a new access token and
returns no refresh token; the stored family is not mutated. Revoked, malformed,
or expired legacy credentials remain invalid. Reuse of a revoked legacy token
is rejected but intentionally does not revoke another stored family member:
the transitional honor path is read-only and does not retain the former
rotation-based theft-detection write. No new agent-runtime refresh credential
is issued, so the legacy population drains naturally by expiry while stateless
authority re-mint replaces the legacy family mechanism without a revocation
wave or a non-transactional revoke-then-create interval.

## Standard refresh authority and lineage

Standard dynamically registered clients use the existing refresh-token
`familyID`, `generation`, `current`, `revoked`, `revokedAt`, `revokedReason`,
and optimistic-lock `version` fields. The critical-path authority is a strongly
consistent singleton `OAUTH_REFRESH_AUTHORITY#<tuple-hash>` / `CURRENT` row per
`(username, client_id, resource)`. It contains a bounded eight-entry LRU family
slot list. Each slot binds the family ID, live-head hash, generation, expiry,
and update time. The authority row's TableTheory `revision` version field is the
CAS boundary. GSI2 remains populated only as reconciliation and operator
evidence; refresh authorization and replay never discover a head through it.

Rotation is one TableTheory transaction:

1. compare-and-swap the presented current generation;
2. retain it as revoked with reason `rotated`; and
3. create exactly one next generation;
4. create an immutable successor artifact whose raw successor is a fail-closed
   TableTheory `theorydb:"encrypted"` field and whose other lineage pointers are
   hashes; and
5. advance the tuple authority slot at its expected revision.

Each of at most three CAS attempts strongly re-reads the predecessor and
authority, then derives the family, generation, token version, and authority
revision inside that attempt. There is no out-of-transaction version seed.
Conditional contention uses injectable full-jitter backoff with a 25 ms base,
doubling delay, and 200 ms cap. A request that began on the active head and
loses contention returns retryable 503 after bounded exhaustion; contention
never destroys the grant or becomes `invalid_grant`.

Rows issued before lineage adoption are assigned a family and generation 1 in
their first rotation transaction. Primary keys and sort keys do not change.
The existing GSI2 family index is populated for standard lineage; the runtime
user and session indexes remain sparse to tokens carrying the pre-existing
runtime-session metadata. Ordinary web/CLI lineage does not populate them.

## Encrypted successor replay walk

A consumed standard token walks immutable direct-key successor artifacts until
its successor hash equals the consistently read authority slot's live-head
hash. Lesser then consistently reads and validates that head, revalidates the
resource and delegation authority, mints a fresh access token, and returns the
head's already-minted raw refresh token. Replay never rotates, revokes, or
creates another refresh credential. Missing/corrupt artifacts, absent authority,
decryption errors, head mismatches, budget charge/read errors, and walk
exhaustion are all retryable 503 responses.

Before walking, Lesser pre-charges eight steps against a per-family, per-minute
budget item capped at 64 steps. It refunds unused steps after the walk. Charge
and budget-read failures fail closed as retryable 503, preventing a replay
request from turning a long retained chain into unbounded read amplification.
A refund CAS failure is logged and retains the conservative full charge; it
does not downgrade an already-minted rescue response or replace the walk's
authoritative outcome.

## AppTheory OAuth primitive adoption

Lesser adapts its OAuth wire surface to AppTheory's `runtime/oauth` primitives:

- RFC 8414 authorization-server metadata is constructed and served by
  `NewAuthorizationServerMetadata` and
  `AuthorizationServerMetadataHandler`. The advertised `/authorize`, `/token`,
  and `/register` paths are real routes; the historical `/oauth/*` forms remain
  additive compatibility aliases. Lesser fills the framework metadata's scope
  and supported token-authentication lists. Because Lesser currently signs
  OAuth access tokens symmetrically, the optional `jwks_uri` is omitted rather
  than advertising public key material that does not exist.
- RFC 7591 request parsing and response serialization embed AppTheory's
  `DynamicClientRegistrationRequest` and
  `DynamicClientRegistrationResponse`. Every request passes through
  `ValidateDynamicClientRegistrationRequest`. Lesser uses the framework
  `AllowedRedirectURIs` policy seam to supply only redirects allowed by its
  stricter HTTPS, native custom-scheme, and loopback-HTTP policy, then layers
  its additive metadata, client-class, scope, and grant policy on the validated
  core.
- RFC 9728 actor-scoped MCP protected-resource metadata is constructed and
  served by `NewProtectedResourceMetadata` and
  `ProtectedResourceMetadataHandler`.
- PKCE S256 verification delegates to AppTheory `PKCEVerifyS256`, including
  RFC 7636's 43-to-128-character verifier bounds.

### PKCE short-verifier sunset

The prior acceptance of verifiers shorter than 43 characters was a non-RFC
compatibility deviation and ends with this release. No stored token or database
migration is required: code verifiers are client-held and authorization codes
are short-lived. An affected client fails loudly with OAuth `invalid_grant`
during code redemption in its next authorization flow; Lesser neither accepts
the short proof nor silently falls back. Clients must generate a verifier of
43–128 characters from the RFC 7636 unreserved character set, derive an S256
challenge, and restart authorization. Authorization codes issued before the
cutover remain redeemable only when their client presents an RFC-conformant
verifier matching the stored challenge.

## Token-endpoint outcome telemetry

Each recognized `authorization_code` or `refresh_token` attempt emits exactly
one best-effort CloudWatch Embedded Metric Format (EMF) JSON line on stdout.
Emission is controlled by `LESSER_OAUTH_GRANT_EMF_ENABLED` and defaults to
enabled. A disabled flag, a handler without a writer, JSON encoding failure, or
stdout write failure never changes the OAuth response. Write failures produce a
warning only.

The EMF contract is:

- namespace: `Lesser/OAuth`;
- metrics: `oauth_grant_outcomes_total` and
  `oauth_grant_exceptions_total`, both `Count`;
- fixed drill-down dimensions:
  `{outcome, reason_code, retry_path, http_status}`;
- an additional empty `{}` dimension set on the same directive, producing the
  dimensionless rollups required for alarm math;
- retry paths: `none`, `direct_rotate`, and `retry_rescue`;
- hashed correlation properties: `client_id_hash`, `resource_hash`, and
  `family_id_hash`, plus `request_id` and `grant_type`. These are properties,
  never dimensions.

CloudWatch creates a distinct metric stream per dimension-value combination.
Dimensioned streams are therefore drill-down signals and must not be summed as
totals. The exception-rate alarm contract is:

```text
SUM(dimensionless oauth_grant_exceptions_total)
/
SUM(dimensionless oauth_grant_outcomes_total)
```

over the same period. `refresh_retry_rescue_served` is the sole exception
reason: it records the rescue mechanism doing work. Ordinary success and all
failure classes increment the outcome counter but not the exception counter.

### Closed reason-code vocabulary

Only the following values may become the `reason_code` EMF dimension. Any new
or unmapped internal detail is clamped to `other`; the unsanitized value remains
in the `detail_reason` property for diagnosis.

- common: `success`, `invalid_request`, `invalid_client`, `invalid_target`,
  `unauthorized_client`, `temporarily_unavailable`, `server_error`,
  `token_generation_failed`, `other`;
- authorization code: `authorization_code_absent`,
  `authorization_code_client_mismatch`,
  `authorization_code_redirect_mismatch`,
  `authorization_code_pkce_mismatch`, `authorization_code_scope_invalid`,
  `authorization_code_resource_mismatch`,
  `authorization_code_authority_revoked`,
  `authorization_code_invalid_context`,
  `authorization_code_already_consumed`;
- standard refresh terminal/containment:
  `refresh_token_absent`, `refresh_cross_client_replay`,
  `refresh_share_grant_revoked`, `refresh_retry_replayed`,
  `refresh_token_expired`, `refresh_stale_generation`,
  `refresh_resource_mismatch`, `refresh_resource_authority_revoked`,
  `refresh_outside_retry_grace`, `refresh_successor_absent`, and
  `refresh_authority_absent`;
- refresh rescue/specialized runtime: `refresh_retry_rescue_served`,
  `refresh_runtime_invalid`, `refresh_runtime_reuse`, and
  `refresh_rotation_infrastructure`.

The three standard-family containment triggers retain their exact reason names:
`refresh_cross_client_replay`, `refresh_share_grant_revoked`, and
`refresh_retry_replayed`. Expiry, staleness, resource mismatch, an elapsed grace
window, and authoritative absence remain distinguishable terminal outcomes
without implying that a family revocation occurred. All retryable 503 responses
use `temporarily_unavailable`.
