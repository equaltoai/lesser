# Actor-URL MCP Auth Contract

This document freezes the client-consumable remote MCP access form owned by Lesser for issue `#492`.

## Canonical remote MCP access form

For a remote MCP consumer targeting actor `{actor}` on Lesser domain `{domain}`, the canonical public contract is:

- actor-scoped MCP endpoint:
  `https://{domain}/mcp/{actor}`
- actor-scoped OAuth protected-resource metadata endpoint:
  `https://{domain}/.well-known/oauth-protected-resource/mcp/{actor}`
- OAuth authorization-server metadata endpoint:
  `https://{domain}/.well-known/oauth-authorization-server`

New remote clients should treat that actor-scoped form as canonical.

`/.well-known/mcp.json` remains part of public discovery, but it does not replace the actor-scoped protected-resource
metadata. The MCP server URL a client ultimately connects to is the actor-specific resource URL, not a separate
connector-specific endpoint.

## Discovery sequence

The intended discovery chain for remote MCP access is:

1. Start from the actor-specific MCP resource URL: `https://{domain}/mcp/{actor}`.
2. Resolve actor-specific protected-resource metadata from
   `https://{domain}/.well-known/oauth-protected-resource/mcp/{actor}`.
3. Resolve the OAuth authorization server from the protected-resource metadata.
4. Read OAuth authorization-server metadata from
   `https://{domain}/.well-known/oauth-authorization-server`.

The actor path segment is part of the resource identity. Clients must not expect a second public registration or
connector identifier to tell Lesser which actor is being accessed.

## Binding rule for actor URL, OAuth client, and access decision

Lesser's public MCP auth contract separates three concerns:

- actor identity:
  owned by the actor-scoped MCP resource URL, `https://{domain}/mcp/{actor}`
- OAuth client identity:
  owned by the registered OAuth client and the client authentication method used on OAuth endpoints
- access decision:
  owned by the requested resource and the consent/session state issued for that resource

That means:

- the actor being accessed comes from the MCP URL and its matching `resource` value
- the OAuth client proves which software is calling Lesser, but it does not choose the actor on behalf of the user
- authorization must be evaluated against the requested `resource`, not against connector-specific client metadata

For public remote MCP access, `client_class`, `agent_username`, or any similar client metadata must not be treated as
the canonical actor-binding source. If those fields continue to exist for compatibility during the migration, they are
secondary legacy signals and must not override the actor-scoped resource URL.
