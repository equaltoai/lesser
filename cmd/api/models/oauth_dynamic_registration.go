package models

// OAuthDynamicClientRegistrationRequest represents the supported RFC 7591 client metadata subset.
type OAuthDynamicClientRegistrationRequest struct {
	ClientName              string   `json:"client_name,omitempty"`
	RedirectURIs            []string `json:"redirect_uris,omitempty"`
	Scope                   string   `json:"scope,omitempty"`
	GrantTypes              []string `json:"grant_types,omitempty"`
	ResponseTypes           []string `json:"response_types,omitempty"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method,omitempty"`
	ClientURI               string   `json:"client_uri,omitempty"`
	LogoURI                 string   `json:"logo_uri,omitempty"`
	Contacts                []string `json:"contacts,omitempty"`
	TosURI                  string   `json:"tos_uri,omitempty"`
	PolicyURI               string   `json:"policy_uri,omitempty"`
	SoftwareID              string   `json:"software_id,omitempty"`
	SoftwareVersion         string   `json:"software_version,omitempty"`

	// Lesser extensions used to keep dynamic registration aligned with existing connector policy.
	ClientClass   string `json:"client_class,omitempty"`
	AgentUsername string `json:"agent_username,omitempty"`
}

// OAuthDynamicClientRegistrationResponse represents RFC 7591 registration output.
type OAuthDynamicClientRegistrationResponse struct {
	ClientID                string   `json:"client_id"`
	ClientSecret            string   `json:"client_secret,omitempty"`
	ClientIDIssuedAt        int64    `json:"client_id_issued_at"`
	ClientSecretExpiresAt   int64    `json:"client_secret_expires_at"`
	ClientName              string   `json:"client_name,omitempty"`
	RedirectURIs            []string `json:"redirect_uris,omitempty"`
	GrantTypes              []string `json:"grant_types,omitempty"`
	ResponseTypes           []string `json:"response_types,omitempty"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method,omitempty"`
	Scope                   string   `json:"scope,omitempty"`
	ClientURI               string   `json:"client_uri,omitempty"`
	LogoURI                 string   `json:"logo_uri,omitempty"`
	Contacts                []string `json:"contacts,omitempty"`
	TosURI                  string   `json:"tos_uri,omitempty"`
	PolicyURI               string   `json:"policy_uri,omitempty"`
	SoftwareID              string   `json:"software_id,omitempty"`
	SoftwareVersion         string   `json:"software_version,omitempty"`

	// Lesser extensions that make dynamic clients manageable through the existing connector model.
	ClientClass        string `json:"client_class,omitempty"`
	AgentUsername      string `json:"agent_username,omitempty"`
	RegistrationSource string `json:"registration_source,omitempty"`
}
