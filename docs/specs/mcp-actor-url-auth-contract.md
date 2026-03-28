# Actor-URL MCP Auth Contract

This document freezes the client-consumable remote MCP access form owned by Lesser for issue `#492`.

It is the single written contract for M0 and covers:

- the canonical actor-scoped MCP access form
- the binding rule between actor URL, OAuth client, and access decision
- the canonical public registration path
- the resource-bound token model for public MCP access
- the public MCP versus internal runtime auth boundary
- the deprecated connector-era surfaces scheduled for removal

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

## Canonical public client registration path

For normal remote MCP consumers, Lesser's canonical client-registration path is standard RFC 7591 dynamic client
registration at `POST /oauth/register`.

That means:

- OAuth discovery clients should register through `POST /oauth/register`
- published example client profiles, snippets, or starter metadata are convenience material only
- any published public-client profile is derived from the RFC 7591 contract and must not replace it as the source of
  truth

`POST /api/v1/apps` may continue to exist as an operator-controlled compatibility path, but it is not the canonical
remote MCP contract for new public clients.

## Token model for public MCP access

For public MCP authorization, `resource` is the canonical target identifier.

That contract applies across the normal OAuth flow:

- the client requests authorization for the actor-scoped MCP resource URL
- the authorization code is issued for that `resource`
- the token exchange must preserve the same `resource`
- refresh-token state remains attached to the same canonical resource target

The public MCP token model is therefore resource-bound, not connector-bound.

`agent_username` is not part of the canonical public token model. If it remains present in stored state during the
migration, it is legacy compatibility data only and must not replace `resource` as the target identifier for public MCP
sessions.

## Public MCP versus internal runtime auth boundary

The canonical public MCP contract is the actor-scoped, resource-bound flow described in this document:

- actor-scoped MCP resource URL
- actor-scoped protected-resource metadata
- standard OAuth metadata and RFC 7591 registration
- resource-bound authorization, token exchange, and refresh behavior

The following are not part of the canonical public remote MCP contract:

- internal runtime clients that keep long-lived agent-bound session semantics
- agent runtime refresh-token families and runtime-session diagnostics
- operator-only secret rotation and compatibility workflows for owned internal clients
- any browser-local connector inventory or connector-management UX

Those compatibility/runtime mechanisms may continue to exist temporarily for operational reasons, but public MCP
support must not depend on them. Public remote clients should be implementable from the actor-scoped resource contract
alone.

## Deprecated connector-era MCP surfaces

The following public-facing MCP-era surfaces are deprecated for removal:

- Simulacrum connector registration flows that create or manage public MCP connector records
- browser-local connector inventory, pasted-secret flows, and similar connector-management UX
- public `client_class=agent` and `agent_username` semantics on registration and token-related MCP guidance

They may continue to exist temporarily as compatibility aids, but new public MCP clients, examples, and docs must not
use them as the canonical integration path.
