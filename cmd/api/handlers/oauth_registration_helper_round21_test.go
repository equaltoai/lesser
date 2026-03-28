package handlers

import (
	"net/http"
	"testing"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/stretchr/testify/require"
)

func TestParseOAuthDynamicClientRegistrationRequest_Round21(t *testing.T) {
	t.Parallel()

	handler, _, _ := round11NewHandler(t, round11TestConfig(), &round10QueryState{})

	t.Run("rejects non json content type", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/register", map[string]string{
			"Content-Type": "application/x-www-form-urlencoded",
		}, nil, []byte(`{}`))

		req, resp, err := handler.parseOAuthDynamicClientRegistrationRequest(ctx)
		require.Nil(t, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusUnsupportedMediaType, resp.Status)
	})

	t.Run("rejects empty request body", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/register", map[string]string{
			"content-type": "application/json",
		}, nil, nil)

		req, resp, err := handler.parseOAuthDynamicClientRegistrationRequest(ctx)
		require.Nil(t, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})

	t.Run("rejects malformed json", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/register", map[string]string{
			"Content-Type": "application/json",
		}, nil, []byte(`{"client_name"`))

		req, resp, err := handler.parseOAuthDynamicClientRegistrationRequest(ctx)
		require.Nil(t, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})

	t.Run("accepts lowercase content type header", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/oauth/register", map[string]string{
			"content-type": "application/json",
		}, nil, apimodels.OAuthDynamicClientRegistrationRequest{
			ClientName:   "Claude Code",
			RedirectURIs: []string{"http://127.0.0.1:8787/callback"},
		})
		require.NoError(t, err)

		req, resp, parseErr := handler.parseOAuthDynamicClientRegistrationRequest(ctx)
		require.NoError(t, parseErr)
		require.Nil(t, resp)
		require.Equal(t, "Claude Code", req.ClientName)
		require.Equal(t, []string{"http://127.0.0.1:8787/callback"}, req.RedirectURIs)
	})
}

func TestNormalizeDynamicClientRegistration_Round21(t *testing.T) {
	t.Parallel()

	cfg := round11TestConfig()

	t.Run("rejects nil request", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		normalized, resp, err := handler.normalizeDynamicClientRegistration(round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/register", nil, nil, nil), nil)
		require.Nil(t, normalized)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})

	t.Run("rejects missing client name", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/register", nil, nil, nil)

		normalized, resp, err := handler.normalizeDynamicClientRegistration(ctx, &apimodels.OAuthDynamicClientRegistrationRequest{
			RedirectURIs: []string{"http://127.0.0.1:8787/callback"},
		})
		require.Nil(t, normalized)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})

	t.Run("rejects public agent client class", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/register", nil, nil, nil)

		normalized, resp, err := handler.normalizeDynamicClientRegistration(ctx, &apimodels.OAuthDynamicClientRegistrationRequest{
			ClientName:   "CLI Client",
			RedirectURIs: []string{"http://127.0.0.1:8787/callback"},
			ClientClass:  auth.ClientClassAgent,
		})
		require.Nil(t, normalized)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})

	t.Run("rejects invalid client uri", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/register", nil, nil, nil)

		normalized, resp, err := handler.normalizeDynamicClientRegistration(ctx, &apimodels.OAuthDynamicClientRegistrationRequest{
			ClientName:   "CLI Client",
			RedirectURIs: []string{"http://127.0.0.1:8787/callback"},
			ClientURI:    "mailto:client@example.com",
		})
		require.Nil(t, normalized)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})

	t.Run("rejects invalid metadata uri", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/register", nil, nil, nil)

		normalized, resp, err := handler.normalizeDynamicClientRegistration(ctx, &apimodels.OAuthDynamicClientRegistrationRequest{
			ClientName:   "CLI Client",
			RedirectURIs: []string{"http://127.0.0.1:8787/callback"},
			LogoURI:      "mailto:logo@example.com",
		})
		require.Nil(t, normalized)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})

	t.Run("rejects invalid contacts", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/register", nil, nil, nil)

		normalized, resp, err := handler.normalizeDynamicClientRegistration(ctx, &apimodels.OAuthDynamicClientRegistrationRequest{
			ClientName:   "CLI Client",
			RedirectURIs: []string{"http://127.0.0.1:8787/callback"},
			Contacts:     []string{"not-an-email"},
		})
		require.Nil(t, normalized)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})

	t.Run("rejects invalid public scopes", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/register", nil, nil, nil)

		normalized, resp, err := handler.normalizeDynamicClientRegistration(ctx, &apimodels.OAuthDynamicClientRegistrationRequest{
			ClientName:   "CLI Client",
			RedirectURIs: []string{"http://127.0.0.1:8787/callback"},
			Scope:        "admin",
		})
		require.Nil(t, normalized)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})

	t.Run("captures optional owner for generic confidential registration", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ownerToken := round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{auth.ScopeRead})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/register", map[string]string{
			"Authorization": "Bearer " + ownerToken,
		}, nil, nil)

		normalized, resp, err := handler.normalizeDynamicClientRegistration(ctx, &apimodels.OAuthDynamicClientRegistrationRequest{
			ClientName:              "Web Client",
			RedirectURIs:            []string{"https://example.com/callback"},
			ClientClass:             auth.ClientClassWeb,
			TokenEndpointAuthMethod: oauthTokenEndpointAuthMethodClientSecretPost,
		})
		require.NoError(t, err)
		require.Nil(t, resp)
		require.Equal(t, auth.ClientClassWeb, normalized.clientClass)
		require.Equal(t, oauthTokenEndpointAuthMethodClientSecretPost, normalized.tokenEndpointAuthMethod)
		require.True(t, normalized.confidential)
		require.Equal(t, "owner", normalized.ownerID)
	})

	t.Run("normalizes public cli defaults", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/register", nil, nil, nil)

		normalized, resp, err := handler.normalizeDynamicClientRegistration(ctx, &apimodels.OAuthDynamicClientRegistrationRequest{
			ClientName:              "CLI Client",
			RedirectURIs:            []string{"customscheme://callback", "customscheme://callback"},
			Scope:                   "read write",
			TokenEndpointAuthMethod: oauthTokenEndpointAuthMethodNone,
			ClientURI:               "https://example.com/client",
			LogoURI:                 "https://example.com/logo.png",
			Contacts:                []string{"Team@example.com", "team@example.com"},
			TosURI:                  "https://example.com/terms",
			PolicyURI:               "https://example.com/privacy",
			SoftwareID:              "cli-software",
			SoftwareVersion:         "1.2.3",
		})
		require.NoError(t, err)
		require.Nil(t, resp)
		require.Equal(t, "CLI Client", normalized.clientName)
		require.Equal(t, []string{"customscheme://callback"}, normalized.redirectURIs)
		require.Equal(t, []string{auth.ScopeRead, auth.ScopeWrite}, normalized.scopes)
		require.Equal(t, []string{auth.GrantTypeAuthorizationCode, auth.GrantTypeRefreshToken}, normalized.grantTypes)
		require.Equal(t, []string{"code"}, normalized.responseTypes)
		require.Equal(t, oauthTokenEndpointAuthMethodNone, normalized.tokenEndpointAuthMethod)
		require.Equal(t, auth.ClientClassCLI, normalized.clientClass)
		require.False(t, normalized.confidential)
		require.Equal(t, "https://example.com/client", normalized.clientURI)
		require.Equal(t, "https://example.com/logo.png", normalized.logoURI)
		require.Equal(t, []string{"team@example.com"}, normalized.contacts)
		require.Equal(t, "https://example.com/terms", normalized.tosURI)
		require.Equal(t, "https://example.com/privacy", normalized.policyURI)
		require.Equal(t, "cli-software", normalized.softwareID)
		require.Equal(t, "1.2.3", normalized.softwareVersion)
	})
}

func TestOAuthDynamicRegistrationHelpers_Round21(t *testing.T) {
	t.Parallel()

	t.Run("redirect uri validation covers policy branches", func(t *testing.T) {
		require.ErrorContains(t, validateDynamicRegistrationRedirectURI(""), "empty value")
		require.ErrorContains(t, validateDynamicRegistrationRedirectURI("urn:ietf:wg:oauth:2.0:oob"), "not allowed")
		require.ErrorContains(t, validateDynamicRegistrationRedirectURI("not-a-uri"), "invalid absolute URI")
		require.ErrorContains(t, validateDynamicRegistrationRedirectURI("https:///callback"), "must include a host")
		require.ErrorContains(t, validateDynamicRegistrationRedirectURI("http://example.com/callback"), "loopback host")
		require.NoError(t, validateDynamicRegistrationRedirectURI("https://example.com/callback"))
		require.NoError(t, validateDynamicRegistrationRedirectURI("http://localhost:8787/callback"))
		require.NoError(t, validateDynamicRegistrationRedirectURI("myapp://oauth/callback"))
	})

	t.Run("redirect uri normalization deduplicates", func(t *testing.T) {
		normalized, err := normalizeDynamicRegistrationRedirectURIs([]string{
			"http://127.0.0.1:8787/callback",
			"http://127.0.0.1:8787/callback",
			"https://example.com/callback",
		})
		require.NoError(t, err)
		require.Equal(t, []string{
			"http://127.0.0.1:8787/callback",
			"https://example.com/callback",
		}, normalized)

		_, err = normalizeDynamicRegistrationRedirectURIs(nil)
		require.ErrorContains(t, err, "at least one redirect URI")
	})

	t.Run("loopback host helper distinguishes valid hosts", func(t *testing.T) {
		require.False(t, isLoopbackRedirectHost(""))
		require.True(t, isLoopbackRedirectHost("localhost"))
		require.True(t, isLoopbackRedirectHost("127.0.0.1"))
		require.True(t, isLoopbackRedirectHost("::1"))
		require.False(t, isLoopbackRedirectHost("example.com"))
	})

	t.Run("client uri validation enforces http and https", func(t *testing.T) {
		require.ErrorContains(t, validateDynamicRegistrationClientURI("relative/path"), "absolute http or https URL")
		require.ErrorContains(t, validateDynamicRegistrationClientURI("customscheme://example"), "must use http or https")
		require.NoError(t, validateDynamicRegistrationClientURI("http://example.com/client"))
		require.NoError(t, validateDynamicRegistrationClientURI("https://example.com/client"))
	})

	t.Run("metadata uri validation enforces http and https", func(t *testing.T) {
		require.ErrorContains(t, validateDynamicRegistrationMetadataURI("logo_uri", "relative/path"), "absolute http or https URL")
		require.ErrorContains(t, validateDynamicRegistrationMetadataURI("logo_uri", "customscheme://example"), "must use http or https")
		require.NoError(t, validateDynamicRegistrationMetadataURI("logo_uri", "http://example.com/logo.png"))
		require.NoError(t, validateDynamicRegistrationMetadataURI("logo_uri", "https://example.com/logo.png"))
	})

	t.Run("client class normalization defaults by auth method", func(t *testing.T) {
		clientClass, err := normalizeDynamicRegistrationClientClass("", oauthTokenEndpointAuthMethodNone)
		require.NoError(t, err)
		require.Equal(t, auth.ClientClassCLI, clientClass)

		clientClass, err = normalizeDynamicRegistrationClientClass("", oauthTokenEndpointAuthMethodClientSecretPost)
		require.NoError(t, err)
		require.Equal(t, auth.ClientClassWeb, clientClass)

		_, err = normalizeDynamicRegistrationClientClass(auth.ClientClassAgent, oauthTokenEndpointAuthMethodClientSecretPost)
		require.ErrorContains(t, err, "client_class=agent is not supported for public registration")
	})

	t.Run("grant type normalization enforces dynamic policy", func(t *testing.T) {
		grants, err := normalizeDynamicRegistrationGrantTypes(nil, auth.ClientClassCLI, oauthTokenEndpointAuthMethodNone, true)
		require.NoError(t, err)
		require.Equal(t, []string{auth.GrantTypeAuthorizationCode, auth.GrantTypeRefreshToken, oauthDeviceCodeGrantType}, grants)

		grants, err = normalizeDynamicRegistrationGrantTypes([]string{auth.GrantTypeAuthorizationCode}, auth.ClientClassCLI, oauthTokenEndpointAuthMethodNone, false)
		require.NoError(t, err)
		require.Equal(t, []string{auth.GrantTypeAuthorizationCode}, grants)

		_, err = normalizeDynamicRegistrationGrantTypes([]string{auth.GrantTypeClientCredentials}, auth.ClientClassCLI, oauthTokenEndpointAuthMethodNone, false)
		require.ErrorContains(t, err, "invalid grant_types")

		_, err = normalizeDynamicRegistrationGrantTypes([]string{oauthDeviceCodeGrantType}, auth.ClientClassCLI, oauthTokenEndpointAuthMethodNone, false)
		require.ErrorContains(t, err, "device_code grant is disabled")

		_, err = normalizeDynamicRegistrationGrantTypes([]string{auth.GrantTypeClientCredentials}, auth.ClientClassWeb, oauthTokenEndpointAuthMethodClientSecretPost, false)
		require.ErrorContains(t, err, "invalid grant_types")
	})

	t.Run("response type normalization defaults to code and rejects unsupported values", func(t *testing.T) {
		responseTypes, err := normalizeDynamicRegistrationResponseTypes(nil)
		require.NoError(t, err)
		require.Equal(t, []string{"code"}, responseTypes)

		responseTypes, err = normalizeDynamicRegistrationResponseTypes([]string{"code", "code"})
		require.NoError(t, err)
		require.Equal(t, []string{"code"}, responseTypes)

		_, err = normalizeDynamicRegistrationResponseTypes([]string{"token"})
		require.ErrorContains(t, err, "response_types must only contain code")
	})

	t.Run("contact normalization lowercases and rejects invalid values", func(t *testing.T) {
		contacts, err := normalizeDynamicRegistrationContacts([]string{"Team@example.com", "team@example.com"})
		require.NoError(t, err)
		require.Equal(t, []string{"team@example.com"}, contacts)

		_, err = normalizeDynamicRegistrationContacts([]string{"not-an-email"})
		require.ErrorContains(t, err, "contacts must contain valid email addresses")
	})

	t.Run("grant subset helper accepts blanks and rejects extras", func(t *testing.T) {
		require.True(t, grantTypeSubsetAllowed([]string{auth.GrantTypeAuthorizationCode}, nil))
		require.True(t, grantTypeSubsetAllowed(
			[]string{"", auth.GrantTypeAuthorizationCode},
			[]string{auth.GrantTypeAuthorizationCode},
		))
		require.True(t, grantTypeSubsetAllowed(
			[]string{auth.GrantTypeAuthorizationCode, auth.GrantTypeRefreshToken},
			[]string{"", auth.GrantTypeAuthorizationCode},
		))
		require.False(t, grantTypeSubsetAllowed(
			[]string{auth.GrantTypeAuthorizationCode},
			[]string{auth.GrantTypeAuthorizationCode, auth.GrantTypeRefreshToken},
		))
	})

	t.Run("dynamic registration defaults vary by client class", func(t *testing.T) {
		require.Equal(t,
			[]string{auth.GrantTypeAuthorizationCode, auth.GrantTypeRefreshToken},
			dynamicRegistrationDefaultGrantTypes(auth.ClientClassCLI, oauthTokenEndpointAuthMethodNone, false),
		)
		require.Equal(t,
			[]string{auth.GrantTypeAuthorizationCode, auth.GrantTypeRefreshToken, oauthDeviceCodeGrantType},
			dynamicRegistrationDefaultGrantTypes(auth.ClientClassCLI, oauthTokenEndpointAuthMethodNone, true),
		)
		require.Equal(t,
			[]string{auth.GrantTypeAuthorizationCode, auth.GrantTypeRefreshToken},
			dynamicRegistrationDefaultGrantTypes(auth.ClientClassWeb, oauthTokenEndpointAuthMethodClientSecretPost, false),
		)
	})
}
