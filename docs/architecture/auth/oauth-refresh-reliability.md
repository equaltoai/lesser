# OAuth Refresh Reliability

Lesser's `/oauth/token` endpoint treats an OAuth error as a statement about
authority. Infrastructure failures must not be presented as evidence that a
grant, client, or refresh credential is invalid.

## Error decision table

| Evidence | Response | Client action |
| --- | --- | --- |
| Refresh token is authoritatively missing, expired, revoked, bound to another client, or bound to another resource | `400 invalid_grant` | Reauthorize |
| Client is authoritatively missing or its presented authentication fails | `400 invalid_client` | Repair client configuration |
| Storage, throttling, transport, conditional-write, ambiguous-write, or share-authorization read failure | `503 temporarily_unavailable` with `Retry-After` | Back off and retry the same request |
| Share or owner revalidation authoritatively denies the principal | `400 invalid_grant`; revoke the standard refresh family | Reauthorize |

Authorization-code and device-code issuance atomically consume the one-time
grant and create the refresh-token row. If that transaction fails, Lesser
returns no tokens and leaves no intentionally unbacked refresh credential.

## Standard refresh lineage

Standard dynamically registered clients use the existing refresh-token
`familyID`, `generation`, `current`, `revoked`, `revokedAt`, `revokedReason`,
and optimistic-lock `version` fields. Rotation is one TableTheory transaction:

1. compare-and-swap the presented current generation;
2. retain it as revoked with reason `rotated`; and
3. create exactly one next generation.

Rows issued before lineage adoption are assigned a family and generation 1 in
their first rotation transaction. Primary keys and sort keys do not change.
The existing GSI2 family index is populated for standard lineage; the runtime
user and session indexes remain sparse to tokens carrying the pre-existing
runtime-session metadata. Ordinary web/CLI lineage does not populate them.

## Bounded retry rescue

For roughly 30 seconds after rotation, the same client may present the replaced
generation once to recover from a lost token response. Lesser locates the sole
active generation, then atomically:

1. marks the stale generation's retry as redeemed;
2. revokes the active replacement; and
3. creates a new sole family head.

The redemption is compare-and-swap protected. A CAS-losing concurrent redemption
is treated as redeemed-reuse replay and revokes the family. Later same-client
stale presentations are terminal but do not start another revocation sweep.
Family-index absence inside the grace window is retryable because a DynamoDB GSI
is eventually consistent. Cross-client use and an authoritative actor share or
owner revalidation denial also revoke the family. Expired credentials,
already-revoked or stale generations, resource mismatch, and use outside the
grace window remain terminal without triggering family revocation.

Dedicated agent-runtime client paths keep their existing rotation and
concurrency behavior; this design applies to the standard DCR refresh path.

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
