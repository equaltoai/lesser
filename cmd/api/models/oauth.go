package models

// OAuthTokenRequest represents a token request
type OAuthTokenRequest struct {
	GrantType    string `json:"grant_type"`
	Code         string `json:"code,omitempty"`
	DeviceCode   string `json:"device_code,omitempty"`
	RedirectURI  string `json:"redirect_uri,omitempty"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"`
	CodeVerifier string `json:"code_verifier,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Resource     string `json:"resource,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// OAuthDeviceCodeRequest represents a device authorization request (RFC 8628-style).
//
// This is sent as application/x-www-form-urlencoded to POST /oauth/device/code.
type OAuthDeviceCodeRequest struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// OAuthDeviceCodeResponse represents the server response for device authorization initiation.
type OAuthDeviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete,omitempty"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// OAuthDeviceVerifyRequest represents a device verification lookup request.
//
// This is sent as application/x-www-form-urlencoded to POST /oauth/device/verify.
type OAuthDeviceVerifyRequest struct {
	UserCode string `json:"user_code"`
}

// OAuthDeviceVerifyResponse represents the device session metadata used by the auth UI to show consent.
type OAuthDeviceVerifyResponse struct {
	UserCode   string   `json:"user_code"`
	ClientID   string   `json:"client_id"`
	ClientName string   `json:"client_name,omitempty"`
	ClientURL  string   `json:"client_url,omitempty"`
	Scopes     []string `json:"scopes,omitempty"`
	Status     string   `json:"status"`
	ExpiresIn  int      `json:"expires_in"`
	Interval   int      `json:"interval"`
}

// OAuthDeviceConsentRequest represents a device session consent action.
//
// This is sent as application/x-www-form-urlencoded to POST /oauth/device/consent.
type OAuthDeviceConsentRequest struct {
	UserCode string `json:"user_code"`
	Action   string `json:"action"` // approve|deny
}

// OAuthDeviceConsentResponse represents the server response after approving/denying a device session.
type OAuthDeviceConsentResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
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
