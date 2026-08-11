# Agent share-grant storage contract

This contract is the Lesser-owned authorization input for actor-scoped MCP share-list checks. It is additive to agent ownership and delegation: creating a share grant does **not** mint a token, create a session, alter OAuth claims, or change `DelegatedBy` / `DelegationPrincipal` behavior.

## Direct active-grant lookup

For agent username `agent` and authenticated caller username `grantee`, read one item from the deployment's existing `lesser-{stage}` table:

| DynamoDB field | Exact value |
| --- | --- |
| `PK` | `USER#{lowercase(agent)}` |
| `SK` | `AGENT_SHARE#GRANTEE#{lowercase(grantee)}` |

The lookup is a single `GetItem` against the base table. An authorization reader such as lesser-body **must** set `ConsistentRead=true` and perform the lookup for every request. It must not cache a positive result. The GSI described below is discovery-only and must never be used for an authorization decision because GSI reads are eventually consistent.

An item is active only when all of the following are true:

1. the item exists at the exact `(PK, SK)` above;
2. `agentUsername` and `granteeUsername` exactly match the normalized usernames used to build the keys;
3. `grantedBy` is non-empty and `grantedAt` is a valid timestamp; and
4. `revokedAt` is absent.

A missing item means **not shared**, with no error. A present `revokedAt` means **not shared**, even if sparse-index attributes are unexpectedly present. A malformed item, table/read error, timeout, or permission error must fail closed: do not authorize the caller and surface the operational error according to the consuming service's error contract.

Revocation updates the base-table row by setting `revokedAt` and `revokedBy` and removing `gsi2PK` and `gsi2SK` in the same version-conditioned write. Therefore the next strongly consistent base-table read observes the grant as inactive; there is no application cache or revocation grace window.

## Persisted attributes

`AgentShareGrant` rows use TableTheory's camelCase attribute convention:

| Attribute | Type | Semantics |
| --- | --- | --- |
| `PK` | string | Direct-read partition key above. |
| `SK` | string | Direct-read sort key above. |
| `agentUsername` | string | Normalized local agent username. |
| `granteeUsername` | string | Normalized existing local Lesser account username. |
| `grantedBy` | string | Normalized owner/admin username that last granted or refreshed access. |
| `grantedAt` | timestamp | UTC time of the latest grant or re-grant. |
| `revokedAt` | timestamp, optional | UTC revocation time. Presence makes the grant inactive. |
| `revokedBy` | string, optional | Normalized owner/admin username that revoked access. |
| `version` | number | TableTheory optimistic-concurrency version. |
| `gsi2PK` | string, active only | `AGENT_SHARE#GRANTEE#{lowercase(grantee)}`. |
| `gsi2SK` | string, active only | `AGENT#{lowercase(agent)}`. |

An idempotent re-grant refreshes `grantedBy` and `grantedAt`, removes `revokedAt` / `revokedBy`, restores the sparse GSI2 keys, and advances `version`.

## Discovery and management views

- Owner/admin list: query base-table `PK=USER#{agent}` with `SK begins_with AGENT_SHARE#GRANTEE#`. This returns active and revoked records so the audit state remains visible.
- "Shared with me" discovery: query GSI2 with `gsi2PK=AGENT_SHARE#GRANTEE#{grantee}`. Only active grants populate this sparse index. Callers must still treat discovery as non-authoritative and re-check the base-table row when authorizing a request.
- Grant and revoke writes are accepted only from the local agent owner or an administrator. Grantees must resolve to an existing local Lesser account. Remote/federated identifiers, unknown accounts, owner/admin self-grants, and grants naming the agent itself are rejected.
- Grant/re-grant/revoke writes use optimistic concurrency. A losing conditional-write race returns `409 Conflict`; callers may re-read the grant and retry from the current state.

Grant and revoke mutations emit `agent.share.grant` and `agent.share.revoke` activity-log events respectively, with the agent, grantee, and acting owner/admin attribution.
