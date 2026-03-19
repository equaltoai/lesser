# Lesser OAuth Scope Model

This document is the source of truth for Lesser issue `#259` and its child issues.

## Canonical externally advertised scope catalog

Lesser's public OAuth and MCP-facing surfaces advertise exactly these requestable scopes:

- `read`
- `write`
- `follow`
- `push`

`cmd/api/handlers/oauth_metadata.go` must keep `scopes_supported` aligned with this catalog.

## Scope semantics

- `read`
  Grants read-oriented OAuth API access and satisfies legacy `read:*` compatibility aliases.
- `write`
  Grants write-oriented OAuth API access and satisfies legacy `write:*` compatibility aliases.
- `follow`
  Canonical scope for relationship-management capability. Runtime authorization treats `follow`, `write:follows`, and broad `write` as equivalent ways to grant follow/unfollow behavior.
- `push`
  Canonical scope for push-subscription management. Runtime authorization accepts `push` explicitly and also allows broad `read` or `write` where the existing Mastodon-compatible handlers already do so.

## Non-canonical scope disposition

- `admin`
  Internal-only. Lesser may still issue or recognize `admin` and `admin:*` on internal/operator tokens, but public OAuth client registration and public authorization requests must reject them.
- `write:follows`
  Compatibility alias for the canonical `follow` capability. Accepted for backward compatibility and delegated-capability checks, but not advertised in discovery or metadata.
- `read:*` and `write:*`
  Compatibility aliases for legacy client records and legacy bearer tokens. They remain accepted on existing records and requests, but external UIs should request the canonical top-level scopes instead of advertising or prefilling the granular forms.
- `push:*`, `follow:*`
  Not part of the public contract.

## Registration and authorization contract

- `POST /api/v1/apps`
  Must validate requested scopes against the canonical catalog plus accepted compatibility aliases.
- Storage rule
  Lesser preserves accepted compatibility aliases verbatim on the client record for backward compatibility. Canonical scopes are stored canonically.
- Default registration scopes
  `read write`
- `/oauth/authorize`
  Must reject internal-only scopes such as `admin` even if an existing consent record contains them.
- Broad-scope implication
  `write` satisfies requests for `follow` and `write:*`.
  `read` satisfies requests for `read:*`.
  Narrow aliases do not imply their broad parent scope.

## Runtime enforcement contract

- Relationship operations
  `follow`, `write:follows`, and `write` are all valid grants for follow/unfollow behavior.
- Push subscription handlers
  `push` remains canonical for push-specific access.
  Existing Mastodon-compatible broad-scope behavior stays in place:
  `read` may read push subscription state.
  `write` may mutate push subscription state.
- Delegated agent scopes
  Delegation must continue to reject `admin` and `push`.
  Delegation subset checks must honor broad-scope implications and the `follow`/`write:follows` compatibility mapping.

## Migration policy for existing tokens and persisted records

- Already-issued bearer tokens remain valid until normal expiry.
- Legacy `read:*`, `write:*`, and `write:follows` claims continue to be honored through the compatibility layer.
- Existing OAuth client records and stored consents are grandfathered. Lesser does not retroactively rewrite or invalidate them as part of the scope-model cleanup.
- New public discovery and metadata surfaces advertise only the canonical catalog, not the compatibility aliases.

## RFC 9728 contract for lesser-body

The protected-resource metadata served by `lesser-body` must stay aligned with this document:

- advertise the same canonical scope catalog: `read`, `write`, `follow`, `push`
- do not advertise `admin`
- do not advertise compatibility aliases such as `write:follows`
- keep the scope semantics aligned with Lesser's auth server policy

The corresponding lesser-body implementation work is tracked separately, but this document is the Lesser-owned contract it should consume.

## MCP discovery contract for lesser-body

`/.well-known/mcp.json` must advertise only scopes that Lesser's OAuth server will actually honor for public clients:

- `read`
- `write`
- `follow`
- `push`

Discovery must not advertise `admin`, because public OAuth clients cannot obtain it from Lesser.

If a previously deployed discovery document exposed non-canonical scopes, existing clients should keep working through the compatibility layer until their stored registrations and live tokens roll over, but newly generated discovery responses should only expose the canonical set.
