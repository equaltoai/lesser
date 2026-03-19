# OAuth Agent Client Compatibility

Lesser's current autonomous agent bootstrap flow is based on pre-provisioned OAuth clients.

The canonical scope contract for that flow lives in [docs/specs/oauth-scope-model.md](/home/aron/ai-workspace/codebases/equaltoai/lesser/docs/specs/oauth-scope-model.md).

## Supported registration flow

- Register a client with `POST /api/v1/apps`.
- Use `client_class=agent` for agent-bound connectors.
- Use `grant_types=client_credentials authorization_code refresh_token` for dual browser + machine flows, or narrow the list to the grants you intend to allow.
- Use `token_endpoint_auth_method=client_secret_post` for confidential agent connectors.
- Request canonical scopes from the public catalog: `read`, `write`, `follow`, `push`.
- Do not request `admin`; it is internal-only and rejected on public OAuth surfaces.
- Store the returned `client_secret` because it is only shown once.

The app-registration response echoes the persisted `grant_types` and `token_endpoint_auth_method` so operators can confirm the connector shape that Lesser stored.
Legacy `read:*`, `write:*`, and `write:follows` aliases remain accepted for compatibility, but new clients should prefer the canonical catalog.

## Supported token flow

- `POST /oauth/token` supports `client_credentials` for confidential agent clients.
- `client_credentials` responses are access-token-only.
- Access tokens inherit Lesser's configured agent access-token TTL.
- Tokens are minted for the bound agent identity and carry `client_class`, `is_agent`, `agent_type`, and `delegated_by` claims.

## Secret recovery

- Operators can rotate an owned client secret in place with `POST /api/v1/apps/{id}/rotate_secret`.
- Existing bearer access tokens remain valid until their normal expiry.
- The old client secret stops working immediately after rotation.
- The replacement secret is returned once in the rotation response and must be redistributed to the connector.

## Current RFC 7591 position

Lesser does not yet expose a dedicated RFC 7591 dynamic client registration endpoint.

The current compatibility story for MCP clients is:

- pre-provision the OAuth client with `POST /api/v1/apps`
- bind the client to the intended agent identity
- use discovery metadata to learn the token endpoint and supported grant/auth-method set
- authenticate with `client_credentials` using the returned confidential secret

Dynamic registration remains a separate follow-up track.
