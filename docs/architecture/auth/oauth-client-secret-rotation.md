# OAuth Client Secret Rotation

Lesser supports in-place OAuth client secret rotation for owned compatibility/runtime clients without changing the
`client_id`.

This is not part of the canonical public actor-scoped MCP contract. Public remote MCP access must not depend on
connector-style secret rotation or other owned-runtime management workflows.

## Continuity contract

Routine rotation is intentionally no-downtime:

- existing bearer access tokens remain valid until their normal expiry
- refresh-token exchanges remain anchored to `client_id` and continue to accept the previous secret only while the grace window is still active
- `client_credentials` token acquisition also continues to accept the previous secret only while the grace window is still active
- once the grace window expires, new client-authenticated exchanges must use the replacement secret

Forced invalidation is narrower than token revocation:

- Lesser immediately rejects the previous secret for refresh-token and `client_credentials` requests
- Lesser does not retroactively revoke already-issued bearer access tokens as part of secret rotation alone
- operators should treat refresh-token or session revocation as a separate incident-response action when they need to cut off already-issued tokens

## Audit events

Secret rotation emits explicit auth-audit events:

- `auth.oauth.client_secret_rotated`
- `auth.oauth.client_secret_rotation_failed`

Both events are intentionally secret-free. The stored metadata contract is:

- `client_id`
- `client_class`
- `client_auth_method`
- `agent_username` when the client is agent-bound
- `forced_invalidation`
- `grace_period_seconds`
- `previous_secret_valid_until` when a grace window exists

The audit trail answers:

- who initiated the rotation
- which connector changed
- whether the rotation kept a grace window or used immediate invalidation
- when the previous secret stops being accepted

Plaintext `client_secret` values must never appear in audit metadata or logs.

## Management handoff

Connector-management surfaces such as Simulacrum should treat the following as the canonical rotation-status fields:

- `rotated_at`
- `forced_invalidation`
- `grace_period_seconds`
- `previous_secret_valid_until`

Those fields are available from Lesser's rotation response and persisted connector state without exposing secret material.

Downstream UI tracking:

- Simulacrum `#54`: broader connector-management UX
- Simulacrum `#55`: rotation-specific status surfacing
