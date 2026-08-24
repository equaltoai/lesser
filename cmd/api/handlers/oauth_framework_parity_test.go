package handlers

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/stretchr/testify/require"
	frameworkoauth "github.com/theory-cloud/apptheory/v4/runtime/oauth"
)

func TestOAuthAuthorizationServerMetadataAdoptsAppTheoryPrimitive(t *testing.T) {
	cfg := round10TestConfig()
	cfg.AllowDeviceFlow = true
	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
	currentResponse := requireStatus(t, http.StatusOK)(h.HandleOAuthAuthorizationServerMetadataLift(
		round10NewLiftContextWithBodyBytes(http.MethodGet, "/.well-known/oauth-authorization-server", nil, nil, nil),
	))
	var current frameworkoauth.AuthorizationServerMetadata
	require.NoError(t, json.Unmarshal(currentResponse.Body, &current))

	framework, err := frameworkoauth.NewAuthorizationServerMetadata(
		cfg.BaseURL(),
		frameworkoauth.WithRevocationEndpoint(cfg.BaseURL()+"/oauth/revoke"),
		frameworkoauth.WithDeviceAuthorizationEndpoint(cfg.BaseURL()+"/oauth/device/code"),
	)
	require.NoError(t, err)
	framework.ScopesSupported = auth.CanonicalOAuthScopes()
	framework.GrantTypesSupported = []string{auth.GrantTypeAuthorizationCode, auth.GrantTypeRefreshToken, oauthDeviceCodeGrantType}
	framework.TokenEndpointAuthMethodsSupported = []string{"client_secret_basic", "client_secret_post", "none"}
	framework.JWKSURI = ""
	require.Equal(t, *framework, current)
}

func TestOAuthDCRAdoptsFrameworkValidationWithLesserRedirectPolicy(t *testing.T) {
	h, _, _ := round11NewHandler(t, round10TestConfig(), &round10QueryState{})
	ctx, err := round10NewLiftContext(http.MethodPost, "/oauth/register", map[string]string{
		"Content-Type": "application/json",
	}, nil, apimodels.OAuthDynamicClientRegistrationRequest{
		ClientName:              "unsafe",
		RedirectURIs:            []string{"http://evil.example/callback"},
		GrantTypes:              []string{"authorization_code", "refresh_token"},
		ResponseTypes:           []string{"code"},
		TokenEndpointAuthMethod: oauthTokenEndpointAuthMethodNone,
	})
	require.NoError(t, err)
	requireStatus(t, http.StatusBadRequest)(h.HandleOAuthDynamicClientRegistrationLift(ctx))

	req := &apimodels.OAuthDynamicClientRegistrationRequest{
		ClientName:              "unsafe",
		RedirectURIs:            []string{"http://evil.example/callback"},
		GrantTypes:              []string{"authorization_code", "refresh_token"},
		ResponseTypes:           []string{"code"},
		TokenEndpointAuthMethod: oauthTokenEndpointAuthMethodNone,
	}
	require.Error(t, validateFrameworkDynamicClientRegistrationRequest(req))
	require.Error(t, validateDynamicRegistrationRedirectURI(req.RedirectURIs[0]))

	req.RedirectURIs = []string{"http://127.0.0.1:8787/callback"}
	require.NoError(t, validateFrameworkDynamicClientRegistrationRequest(req))
}

func TestOAuthPKCEAdoptsFrameworkRFC7636VerifierBounds(t *testing.T) {
	verifier := "test"
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	oauthSvc := auth.NewOAuthService(round11StrongJWTSecret, round10TestConfig(), nil, nil)
	require.ErrorIs(t, oauthSvc.VerifyCodeChallenge(challenge, verifier, "S256"), auth.ErrInvalidCodeChallenge)
	matched, err := frameworkoauth.PKCEVerifyS256(verifier, challenge)
	require.Error(t, err)
	require.False(t, matched)

	rfcVerifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	rfcChallenge := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	require.NoError(t, oauthSvc.VerifyCodeChallenge(rfcChallenge, rfcVerifier, "S256"))
}

func TestOAuthProtectedResourceMetadataUsesAppTheoryPrimitive(t *testing.T) {
	h, _, _ := round11NewHandler(t, round10TestConfig(), &round10QueryState{})
	ctx := round10NewLiftContextWithBodyBytes(http.MethodGet, "/.well-known/oauth-protected-resource/mcp/agent1", nil, nil, nil)
	ctx.Params["username"] = "agent1"
	resp := requireStatus(t, http.StatusOK)(h.HandleOAuthProtectedResourceMetadataLift(ctx))
	var metadata frameworkoauth.ProtectedResourceMetadata
	require.NoError(t, json.Unmarshal(resp.Body, &metadata))
	require.Equal(t, auth.BuildPublicMCPAccessBundle(h.cfg.BaseURL(), "agent1").MCPURL, metadata.Resource)
	require.Equal(t, []string{h.cfg.BaseURL()}, metadata.AuthorizationServers)
	require.Equal(t, auth.CanonicalOAuthScopes(), metadata.ScopesSupported)
	require.Equal(t, []string{"header"}, metadata.BearerMethodsSupported)

}
