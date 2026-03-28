package auth

import "strings"

// PublicMCPAccessBundle describes the client-neutral actor-scoped MCP access
// surface that can be shown by agent UIs without provisioning connector state.
type PublicMCPAccessBundle struct {
	MCPURL                 string
	ProtectedResourceURL   string
	AuthorizationServerURL string
	RegistrationURL        string
	SupportedScopes        []string
	Guidance               []string
}

// BuildPublicMCPAccessBundle returns the canonical actor-scoped MCP discovery
// bundle for a given local actor username.
func BuildPublicMCPAccessBundle(baseURL, actorUsername string) PublicMCPAccessBundle {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	actorUsername = strings.TrimSpace(actorUsername)

	bundle := PublicMCPAccessBundle{
		SupportedScopes: []string{ScopeRead, ScopeWrite, ScopeFollow, ScopePush},
		Guidance: []string{
			"Start from the actor-scoped MCP URL.",
			"Use that same MCP URL as the OAuth resource value.",
			"Discover OAuth metadata through the actor-scoped protected-resource document.",
			"Register a generic OAuth client with /oauth/register when your client needs registration.",
			"Do not rely on client_credentials or agent-bound connector metadata on the public surface.",
		},
	}
	if baseURL == "" || actorUsername == "" {
		return bundle
	}

	bundle.MCPURL = baseURL + "/mcp/" + actorUsername
	bundle.ProtectedResourceURL = baseURL + "/.well-known/oauth-protected-resource/mcp/" + actorUsername
	bundle.AuthorizationServerURL = baseURL + "/.well-known/oauth-authorization-server"
	bundle.RegistrationURL = baseURL + "/oauth/register"

	return bundle
}
