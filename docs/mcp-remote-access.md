# Remote MCP Access

Lesser's canonical remote MCP contract is actor-scoped and resource-bound.

This page describes the **public MCP access contract** only. For what soul means in Lesser, what `soulEnabled` means,
and how `lesser-body`, `lesser-host`, and `lesser-soul` relate, start with `docs/soul.md`.

When Lesser is wired to `lesser-body`, the actor-scoped MCP routes documented here are fronted on the Lesser domain but
served by the imported `lesser-body` MCP Lambda.

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

## Instance-owner Ptah/Ba access

Ptah and Ba are instance-plane MCP resources. Their owner path is intentionally separate from public actor access:

- Ptah resource: `https://{canonical-api-host}/instance/ptah/mcp`
- Ba resource: `https://{canonical-api-host}/instance/ba/mcp`
- Ptah protected-resource metadata: `https://{canonical-api-host}/.well-known/oauth-protected-resource/instance/ptah/mcp`
- Ba protected-resource metadata: `https://{canonical-api-host}/.well-known/oauth-protected-resource/instance/ba/mcp`

Use the exact resource URL, including its `api.` host and path, as the OAuth `resource` value. A Ptah token cannot be
reused for Ba. The authorization-server URL remains `https://{domain}/oauth/authorize` and the token endpoint remains
`https://{domain}/oauth/token`.

The supported owner path is the normal MCP OAuth flow. A separate owner-bootstrap secret, manual authorization-code
exchange, or pasted bearer-token workaround is not required.

### Standard owner/admin OAuth flow

1. Dynamically register a public MCP client with `POST /oauth/register`. Use `token_endpoint_auth_method=none`, PKCE,
   and the ordinary `read write` scopes. The stored client remains generic (`client_class=cli` or `web`); it does not
   request or receive an operator marker during registration.
2. Open `/oauth/authorize` in the authenticated browser session of the active local instance owner/admin. Use the
   exact resource for the surface being connected and request only `read write`:

   ```text
   https://{domain}/oauth/authorize?response_type=code&client_id={public_client_id}&redirect_uri={callback}&resource=https%3A%2F%2F{canonical-api-host}%2Finstance%2Fptah%2Fmcp&scope=read%20write&code_challenge={pkce_challenge}&code_challenge_method=S256&state={fresh_random_state}
   ```

   Use `/instance/ba/mcp` for Ba. The resource must retain the canonical `api.` host and exact path.
3. Exchange the authorization code at `/oauth/token` with the same client ID, redirect URI, exact resource, and PKCE
   verifier. A public client does not send a client secret. Standard MCP clients, including Codex, can perform this
   flow without the operator manually handling a bootstrap credential.

For an active local admin, Lesser derives operator authority only at token issuance when the resource is exactly the
Ptah or Ba instance resource. The access and refresh token claims are:

- `aud`: exactly the requested instance resource;
- `client_class`: `operator`;
- `scopes`: the requested `read write` set, without silently adding `admin`; and
- `is_agent`: absent or false.

Refresh rotation preserves that same operator marker, exact audience, and scope set. Body recognizes the explicit
operator marker, so Ptah/Ba genesis does not require an x402 payment challenge. Ordinary users, non-admins, agent
clients or tokens, actor-scoped resources, non-instance resources, and wrong hosts do not receive operator authority.
Body's independent enforcement and the genesis flow remain Body-owned (see Body issue `#423`).

## Related docs

- Soul boundary doc: [docs/soul.md](/home/aron/ai-workspace/codebases/equaltoai/lesser/docs/soul.md)
- Actor-URL auth contract: [docs/specs/mcp-actor-url-auth-contract.md](/home/aron/ai-workspace/codebases/equaltoai/lesser/docs/specs/mcp-actor-url-auth-contract.md)
- OAuth client guidance: [docs/oauth-agent-clients.md](/home/aron/ai-workspace/codebases/equaltoai/lesser/docs/oauth-agent-clients.md)
- Dynamic registration contract: [docs/specs/oauth-dynamic-client-registration.md](/home/aron/ai-workspace/codebases/equaltoai/lesser/docs/specs/oauth-dynamic-client-registration.md)
- Cutover verifier runbook: [docs/architecture/auth/mcp-auth-cutover-verification.md](/home/aron/ai-workspace/codebases/equaltoai/lesser/docs/architecture/auth/mcp-auth-cutover-verification.md)
