# OAuth Agent Client Compatibility

The canonical scope contract for that flow lives in [docs/specs/oauth-scope-model.md](/home/aron/ai-workspace/codebases/equaltoai/lesser/docs/specs/oauth-scope-model.md).
The grant-selection guidance for browser, headless-approved, and fully autonomous agents lives in [docs/device-code-agent-auth.md](/home/aron/ai-workspace/codebases/equaltoai/lesser/docs/device-code-agent-auth.md).

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
- `POST /oauth/token` also supports `urn:ietf:params:oauth:grant-type:device_code` for agent clients that request operator approval without a redirect-capable browser.
- `client_credentials` responses are access-token-only.
- Access tokens inherit Lesser's configured agent access-token TTL.
- Device-code and client-credentials tokens for agent clients are minted for the bound agent identity and carry `client_class`, `is_agent`, `agent_type`, and `delegated_by` claims.

## Secret recovery

- Operators can rotate an owned client secret in place with `POST /api/v1/apps/{id}/rotate_secret`.
- Existing bearer access tokens remain valid until their normal expiry.
- The old client secret stops working immediately after rotation.
- The replacement secret is returned once in the rotation response and must be redistributed to the connector.

## RFC 7591 dynamic registration

Lesser now exposes `POST /oauth/register` and advertises it through RFC 8414 metadata.

The current compatibility story is:

- native or CLI discovery clients can dynamically register a public client with `token_endpoint_auth_method=none`
- dynamically registered public clients use PKCE for the authorization-code flow
- if device flow is enabled, dynamic `cli` clients also receive `device_code`
- agent-bound dynamic clients still require the existing ownership check before Lesser will bind them to an agent identity

Manual `POST /api/v1/apps` registration remains available as an operator-controlled alternative.
