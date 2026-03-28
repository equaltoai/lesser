# Remote MCP Access

Lesser's canonical remote MCP contract is actor-scoped and resource-bound.

For actor `{actor}` on Lesser domain `{domain}`, the normal public connection shape is:

- MCP URL: `https://{domain}/mcp/{actor}`
- protected-resource metadata URL: `https://{domain}/.well-known/oauth-protected-resource/mcp/{actor}`
- authorization-server metadata URL: `https://{domain}/.well-known/oauth-authorization-server`
- dynamic registration URL: `https://{domain}/oauth/register`

This is the connection story client teams should build around. Simulacrum-era connector registration, pasted secrets,
and browser-local connector inventory are not part of the canonical public MCP path.

## Canonical sequence

1. Start from the actor-scoped MCP URL.
2. Resolve the actor-scoped protected-resource document.
3. Read the shared authorization-server metadata it points at.
4. Register a generic OAuth client with `POST /oauth/register` when the client needs registration.
5. Send the same actor-scoped MCP URL as the OAuth `resource` value during authorization.

## Read-only discovery examples

Fetch authorization-server metadata:

```bash
curl -sS "https://{domain}/.well-known/oauth-authorization-server" | jq .
```

Fetch actor-scoped protected-resource metadata:

```bash
curl -sS "https://{domain}/.well-known/oauth-protected-resource/mcp/{actor}" | jq .
```

## Canonical registration example

Register a generic public client:

```bash
curl -sS -X POST "https://{domain}/oauth/register" \
  -H "Content-Type: application/json" \
  -d '{
    "client_name": "Example MCP Client",
    "redirect_uris": ["http://127.0.0.1/callback"],
    "scope": "read write follow",
    "token_endpoint_auth_method": "none"
  }' | jq .
```

The canonical public result is a generic OAuth client, not an agent-bound connector record.

## Derived client examples

These are derived examples built on the canonical actor-scoped contract above. They are convenience profiles, not a
second source of truth.

### Browser or desktop client

- Register a generic OAuth client through `POST /oauth/register`.
- Open `/oauth/authorize` with the actor-scoped MCP URL as the `resource`.
- Complete the normal authorization-code + PKCE flow.
- Use the resulting bearer token against the actor-scoped MCP URL.

### CLI or headless client

- Prefer the same authorization-code flow when a local browser callback is available.
- If device flow is enabled for CLI clients on the target instance, treat it as a separate bootstrap path rather than as
  the canonical actor-URL contract.
- Even in that derived CLI flow, the canonical public registration path is still `POST /oauth/register`.

## Related docs

- Actor-URL auth contract: [docs/specs/mcp-actor-url-auth-contract.md](/home/aron/ai-workspace/codebases/equaltoai/lesser/docs/specs/mcp-actor-url-auth-contract.md)
- OAuth client guidance: [docs/oauth-agent-clients.md](/home/aron/ai-workspace/codebases/equaltoai/lesser/docs/oauth-agent-clients.md)
- Dynamic registration contract: [docs/specs/oauth-dynamic-client-registration.md](/home/aron/ai-workspace/codebases/equaltoai/lesser/docs/specs/oauth-dynamic-client-registration.md)
- Cutover verifier runbook: [docs/architecture/auth/mcp-auth-cutover-verification.md](/home/aron/ai-workspace/codebases/equaltoai/lesser/docs/architecture/auth/mcp-auth-cutover-verification.md)
