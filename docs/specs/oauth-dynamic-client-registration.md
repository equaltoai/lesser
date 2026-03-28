# OAuth Dynamic Client Registration

Lesser now exposes `POST /oauth/register` as its RFC 7591 dynamic client registration endpoint for OAuth discovery clients.

For public remote MCP clients, that RFC 7591 endpoint is the canonical registration contract. Published client examples
or prefilled public-client profiles are convenience material only; they are derived from `POST /oauth/register` rather
than replacing it.

## Registration authorization model

Dynamic registration is open by default. Lesser does not currently require an RFC 7591 initial access token.

Instead, Lesser constrains the public surface with:

- JSON-only request parsing with strict unknown-field rejection
- explicit redirect URI policy
- server-side grant policy assignment
- public-client vs confidential-client validation
- dedicated `/oauth/register` rate limiting

Agent-bound registration is more restrictive:

- `client_class=agent` requires an authenticated owner principal
- the owner must already satisfy the same ownership check used by manual connector creation in `cmd/api/handlers/oauth_agent_binding.go`
- a dynamic registration request cannot bind an agent it does not own

## Supported metadata subset

Lesser currently accepts this RFC 7591 subset:

- `client_name`
- `redirect_uris`
- `scope`
- `grant_types`
- `token_endpoint_auth_method`
- `client_uri`
- `software_id`
- `software_version`

Lesser extensions:

- `client_class`
- `agent_username`

Unsupported metadata is rejected with `invalid_client_metadata` rather than silently ignored.

## Redirect URI policy

Dynamic registration accepts:

- `https://...` redirect URIs with a host
- loopback `http://127.0.0.1/...`, `http://[::1]/...`, or `http://localhost/...`
- absolute custom-scheme native redirect URIs such as `claude://oauth/callback`

Dynamic registration rejects:

- `urn:ietf:wg:oauth:2.0:oob`
- non-loopback `http://...`
- malformed or relative redirect URIs

## Public and confidential clients

Lesser supports both:

- confidential clients with `token_endpoint_auth_method=client_secret_post`
- public clients with `token_endpoint_auth_method=none`

When `token_endpoint_auth_method` is omitted, Lesser infers:

- `client_class=agent` -> confidential `client_secret_post`
- otherwise -> `none` implies `cli`, and confidential defaults imply `web`

Dynamically registered public clients are expected to use PKCE for the authorization-code flow. Lesser enforces that requirement for dynamically registered public clients on `/oauth/authorize`.

## Grant policy

Lesser assigns or constrains grants server-side:

- dynamic `cli` clients default to `authorization_code refresh_token`
- if device flow is enabled, dynamic `cli` clients also receive `urn:ietf:params:oauth:grant-type:device_code`
- dynamic confidential `agent` clients default to `authorization_code refresh_token client_credentials`
- dynamic public `agent` clients default to `authorization_code refresh_token`
- requested `grant_types` must be a subset of Lesser's allowed default set for that client shape

This keeps dynamic registration aligned with the broader agent-auth grant policy and prevents arbitrary clients from self-assigning `client_credentials`.

## Provenance and management

Lesser persists dynamic-registration provenance on the shared OAuth client record:

- `registration_source`
- `client_uri`
- `software_id`
- `software_version`
- existing `confidential`, `client_class`, `agent_username`, and `owner_id`

That is the Lesser-side contract for storage and management views:

- manual and dynamic clients must be distinguishable by `registration_source`
- public and confidential clients must be distinguishable by `confidential`
- agent binding remains visible through `client_class`, `agent_username`, and `owner_id`

This keeps manual and dynamic connectors in one coherent storage model so downstream management can list, revoke, rotate, or delete them uniformly.
