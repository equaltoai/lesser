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

The owner-bootstrap command provisions a confidential internal OAuth client and writes its `client_id` and plaintext
`client_secret` to the operator-controlled `lesser/<environment>/admin-oauth` Secrets Manager secret. The persisted
client is owner-bound (`OwnerID`), `client_class=operator`, and granted `read write admin`. Do not try to recreate it
through public `/oauth/register` or `/api/v1/apps`; public registration must remain generic and must not issue
`admin` or `operator` authority.

### Obtain an owner token

1. Read the bootstrap client ID, secret, owner username, and registered callback from the protected operator secret. Do
   not print the secret or token to logs, shell transcripts, tickets, or chat.
2. Open the authorization URL below in the authenticated instance-owner browser session. Replace the placeholders and
   URL-encode the query values:

   ```text
   https://{domain}/oauth/authorize?response_type=code&client_id={owner_client_id}&redirect_uri=https%3A%2F%2F{domain}%2Fauth%2Fcallback&resource=https%3A%2F%2F{canonical-api-host}%2Finstance%2Fptah%2Fmcp&scope=read%20write%20admin&state={fresh_random_state}
   ```

   The owner must be the local active admin bound to the client. Approve the requested scopes if the consent screen is
   shown. Repeat with the Ba resource when connecting Ba.
3. Exchange the callback code at `/oauth/token` with the same redirect URI, exact resource, client ID, and client
   secret using the authorization-code grant. Keep the returned bearer token in a protected environment variable or
   secret store; never put it in a URL.

The authorization server rejects the request for a non-owner/non-admin principal, rejects public clients requesting
`admin`, and rejects a token exchange whose resource or owner binding does not match. The resulting access token must
carry `client_class=operator`, `admin` in `scopes`, and the exact Ptah or Ba resource in `aud`.

### Connect Codex without exposing the client secret

The installed Codex MCP CLI supports `--bearer-token-env-var` but does not expose an OAuth client-secret option for
`codex mcp add`. Use the owner authorization-code exchange above outside Codex, then supply the resource-bound bearer
token through the environment:

```bash
export LESSER_PTAH_OWNER_TOKEN='(operator-issued token; do not echo it)'
export PTAH_RESOURCE='https://{canonical-api-host}/instance/ptah/mcp'

codex mcp add lesser-ptah-owner \
  --url "$PTAH_RESOURCE" \
  --oauth-resource "$PTAH_RESOURCE" \
  --bearer-token-env-var LESSER_PTAH_OWNER_TOKEN
```

Use a separate `LESSER_BA_OWNER_TOKEN` and the exact Ba resource for Ba. Do not substitute a public `read`/`write`
token, make `write` equivalent to owner authority, or bypass Body's x402 policy for ordinary callers. Once connected,
run the Ptah genesis conversation and verify that the owner/operator token is accepted without an `x402_grant_required`
challenge; Body's independent enforcement and the genesis flow remain Body-owned (see Body issue `#423`).

For a local claim check that does not print the token, set `LESSER_PTAH_OWNER_TOKEN` and `PTAH_RESOURCE`, then run:

```bash
python3 - <<'PY'
import base64, json, os

payload = os.environ["LESSER_PTAH_OWNER_TOKEN"].split(".")[1]
payload += "=" * (-len(payload) % 4)
claims = json.loads(base64.urlsafe_b64decode(payload))
assert claims["aud"] == [os.environ["PTAH_RESOURCE"]]
assert claims["client_class"] == "operator"
assert "admin" in claims["scopes"]
assert claims.get("is_agent") is not True
print("owner/operator claims and resource audience verified")
PY
```

This verifies Lesser's issuance contract locally; it is not a substitute for Body's integration verification.

## Related docs

- Soul boundary doc: [docs/soul.md](/home/aron/ai-workspace/codebases/equaltoai/lesser/docs/soul.md)
- Actor-URL auth contract: [docs/specs/mcp-actor-url-auth-contract.md](/home/aron/ai-workspace/codebases/equaltoai/lesser/docs/specs/mcp-actor-url-auth-contract.md)
- OAuth client guidance: [docs/oauth-agent-clients.md](/home/aron/ai-workspace/codebases/equaltoai/lesser/docs/oauth-agent-clients.md)
- Dynamic registration contract: [docs/specs/oauth-dynamic-client-registration.md](/home/aron/ai-workspace/codebases/equaltoai/lesser/docs/specs/oauth-dynamic-client-registration.md)
- Cutover verifier runbook: [docs/architecture/auth/mcp-auth-cutover-verification.md](/home/aron/ai-workspace/codebases/equaltoai/lesser/docs/architecture/auth/mcp-auth-cutover-verification.md)
