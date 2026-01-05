package models

// OAuthTokenRequest represents a token request
type OAuthTokenRequest struct {
	GrantType    string `json:"grant_type"`
	Code         string `json:"code,omitempty"`
	RedirectURI  string `json:"redirect_uri,omitempty"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"`
	CodeVerifier string `json:"code_verifier,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// OAuthTokenResponse represents a token response
type OAuthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
	CreatedAt    int64  `json:"created_at"`
}

// OAuthRevokeRequest represents a token revocation request
type OAuthRevokeRequest struct {
	Token         string `json:"token"`
	TokenTypeHint string `json:"token_type_hint,omitempty"`
	ClientID      string `json:"client_id"`
	ClientSecret  string `json:"client_secret"`
}

// OAuthErrorResponse represents an OAuth error
type OAuthErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
	ErrorURI         string `json:"error_uri,omitempty"`
}

// OAuthAuthorizeResponse represents the response when returning the next_url redirect as JSON.
type OAuthAuthorizeResponse struct {
	NextURL string `json:"next_url"`
}

// OAuthConsentRequest represents the form body posted to /oauth/consent.
type OAuthConsentRequest struct {
	State  string `json:"state"`
	Action string `json:"action"`
}

// OAuthConsentResponse represents the JSON response from /oauth/consent.
type OAuthConsentResponse struct {
	RedirectURI string `json:"redirect_uri"`
}
