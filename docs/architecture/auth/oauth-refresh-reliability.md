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
| Share or owner revalidation authoritatively denies the principal | `400 invalid_grant` | Reauthorize |

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

The redemption is compare-and-swap protected. A concurrent or subsequent
redemption is terminal and cannot fork the family. Family-index absence inside
the grace window is retryable because a DynamoDB GSI is eventually consistent.
Cross-client use, expired credentials, revoked families, resource mismatch, or
use outside the grace window remain terminal and trigger family revocation.

Dedicated agent-runtime client paths keep their existing rotation and
concurrency behavior; this design applies to the standard DCR refresh path.
