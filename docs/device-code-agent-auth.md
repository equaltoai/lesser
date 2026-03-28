# Device Code For Agent Auth

Lesser supports three OAuth grant families for agent-facing integrations. They solve different operational problems:

| Grant | User approval | Browser redirect on the agent host | Best fit |
| --- | --- | --- | --- |
| `authorization_code` | Yes | Yes | Interactive MCP clients with a browser on the same machine |
| `urn:ietf:params:oauth:grant-type:device_code` | Yes | No | Headless agents that still need an operator to approve each session |
| `client_credentials` | No | No | Fully autonomous, pre-approved machine-to-machine agents |

## When to use device code

Choose `device_code` when:

- the agent is running somewhere headless, such as a server, container, or CI job
- an operator still wants to review the requesting app and scopes before the session starts
- a browser redirect back to the agent host is inconvenient or impossible

Device flow is the middle ground between browser-based `authorization_code` and fully autonomous `client_credentials`.

## Boundary with the public MCP contract

For public remote MCP access, the canonical contract is still the actor-scoped, resource-bound OAuth flow described in
[docs/specs/mcp-actor-url-auth-contract.md](/home/aron/ai-workspace/codebases/equaltoai/lesser/docs/specs/mcp-actor-url-auth-contract.md).

Current device-code flow is outside that canonical actor-URL contract for now. `POST /oauth/device/code` currently
accepts `client_id` and scopes only; it does not accept the actor-scoped `resource` value used by the canonical public
MCP flow.

Treat device code as a separate, non-canonical compatibility/bootstrap path until a later milestone rewires it around
the actor-scoped resource contract.

## Lesser device-flow sequence

1. The agent calls `POST /oauth/device/code` with its `client_id` and requested scopes.
   This flow currently does not carry the actor-scoped MCP `resource`.
2. Lesser returns:
   - `device_code`
   - `user_code`
   - `verification_uri`
   - `verification_uri_complete`
3. The operator opens `verification_uri` on a separate device, or uses the prefilled `verification_uri_complete`.
4. The hosted approval page looks up the pending request through `POST /oauth/device/verify`.
5. The operator approves or denies the request through `POST /oauth/device/consent`.
6. The headless agent polls `POST /oauth/token` with `grant_type=urn:ietf:params:oauth:grant-type:device_code` until Lesser returns tokens.

## Token behavior by client class

Device flow inherits the OAuth client's class rather than forcing one uniform token shape:

- `client_class=cli`
  - access tokens are minted as CLI tokens
  - CLI automation safety rails apply
  - refresh records keep CLI-style session metadata
- `client_class=agent`
  - internal or legacy compatibility runtime path; not provisioned by public registration and not part of the canonical actor-scoped public contract
  - access tokens are minted for the bound agent identity, not the approving operator principal
  - tokens carry `client_class`, `is_agent`, `agent_type`, and `delegated_by`
  - ordinary public or legacy compatibility agent clients now store standard OAuth refresh records
  - only the dedicated internal runtime client IDs keep runtime-family, device-label, and idle/absolute-expiry session semantics
  - runtime-session diagnostics and revocation endpoints cover only those dedicated internal runtime client IDs

For agent clients, the approving operator must own the bound agent. Lesser rejects approvals that no longer satisfy that ownership relationship.

## Choosing between the three grants

- Prefer `authorization_code` when the MCP client can open a browser locally and complete a normal redirect.
- Prefer `device_code` when the client is headless but you still want a human approval step.
- Prefer `client_credentials` only for legacy compatibility clients or dedicated internal runtimes that are intentionally pre-approved to act without per-session operator consent.

## Current approval-page contract

The hosted `verification_uri` page is served from `/auth/device`. It shows the requesting app, requested scopes, and the current device-session status by calling the existing verification API.

The approval API currently authenticates with a bearer token, so the operator page expects an access token for the approving account when it submits `approve` or `deny`. That keeps the page usable in this repo without introducing a second, hidden auth mechanism for device approval.

## Related docs

- CLI-oriented device flow: [docs/cli/auth.md](/home/aron/ai-workspace/codebases/equaltoai/lesser/docs/cli/auth.md)
- Agent client registration and grant setup: [docs/oauth-agent-clients.md](/home/aron/ai-workspace/codebases/equaltoai/lesser/docs/oauth-agent-clients.md)
- Public scope catalog: [docs/specs/oauth-scope-model.md](/home/aron/ai-workspace/codebases/equaltoai/lesser/docs/specs/oauth-scope-model.md)
