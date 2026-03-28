# MCP Auth Cutover Verification

This runbook is the Lesser-owned verification path for the actor-URL MCP cutover.

It does not depend on a separate UI project. It verifies Lesser's published discovery and public-registration contract
directly.

## Repo-local verifier

Use the built-in verifier after deployment:

```bash
go build -o lesser ./cmd/lesser
./lesser verify mcp-auth-cutover \
  --base-url https://dev.example.com \
  --actors agent-a,agent-b
```

Read-only verification checks:

- authorization-server metadata is published
- actor-scoped protected-resource metadata resolves for multiple actors
- each protected-resource document resolves to a distinct actor-scoped MCP resource
- the published auth-server metadata does not advertise public `client_credentials`

## Write-path verification

`POST /oauth/register` and `POST /api/v1/apps` checks write temporary OAuth client rows. Run those only against a
disposable environment:

```bash
./lesser verify mcp-auth-cutover \
  --base-url https://dev.example.com \
  --actors agent-a,agent-b \
  --allow-write
```

Write-path verification checks:

- dynamic registration can mint a generic public client without a client secret
- public dynamic registration does not return connector-bound metadata such as `agent_username`
- dynamic registration rejects `client_credentials` on the public surface
- `POST /api/v1/apps` rejects the removed public `agent_username` input

## Manual client checks after deployment

Interactive auth still needs a real deployed environment and an operator account.

### Browser or desktop MCP client

1. Register a generic client through `POST /oauth/register`.
2. Start authorization with the actor-scoped MCP URL as `resource`.
3. Complete the normal authorization-code + PKCE flow.
4. Confirm the client connects to the actor-scoped MCP URL without any Simulacrum-era connector provisioning.

### CLI client

1. Register a generic client through `POST /oauth/register`.
2. If the instance enables device flow for CLI clients, complete the device authorization flow.
3. Confirm the session starts without any public `client_credentials` or agent-bound connector metadata.

## Ownership boundary

This verifier covers Lesser-owned public MCP discovery and registration behavior.

Cross-project UI verification belongs to the consuming product's own project and should not be treated as evidence that
Lesser's contract is or is not published correctly.
