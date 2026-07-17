package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestOAuthAuthorizeInstancePlaneResources(t *testing.T) {
	for _, surface := range []string{oauthInstanceSurfacePtah, oauthInstanceSurfaceBa} {
		t.Run(surface, func(t *testing.T) {
			cfg := round11TestConfig()
			resource := oauthInstanceTestResource(surface)
			var issuedAuthCode *storage.AuthorizationCode

			accountsSvc := &AccountsServiceStub{
				GetUserAppConsentFunc: func(context.Context, *accounts.GetUserAppConsentQuery) (*accounts.GetUserAppConsentResult, error) {
					return &accounts.GetUserAppConsentResult{
						Consent: &storage.UserAppConsent{Scopes: []string{auth.ScopeRead, auth.ScopeWrite}},
					}, nil
				},
				CreateAuthorizationCodeFunc: func(_ context.Context, cmd *accounts.CreateAuthorizationCodeCommand) (*accounts.CreateAuthorizationCodeResult, error) {
					issuedAuthCode = cmd.AuthCode
					return &accounts.CreateAuthorizationCodeResult{}, nil
				},
			}
			state := &round10QueryState{
				oauthClientsByID: map[string]storagemodels.OAuthClient{
					"instance-client": {
						ClientID:           "instance-client",
						Name:               "Instance Connector",
						RedirectURIs:       []string{"https://client.example/callback"},
						Scopes:             []string{auth.ScopeRead, auth.ScopeWrite},
						ClientClass:        auth.ClientClassCLI,
						RegistrationSource: oauthRegistrationSourceDynamic,
						CreatedAt:          time.Now().Add(-24 * time.Hour),
					},
				},
				usersByUsername: map[string]storagemodels.User{
					"alice": {Username: "alice", Approved: true},
				},
			}
			h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{AccountsSvc: accountsSvc})

			ctx, err := round10NewLiftContext(http.MethodGet, "/oauth/authorize", nil, map[string]string{
				"response_type":         "code",
				"client_id":             "instance-client",
				"redirect_uri":          "https://client.example/callback",
				"resource":              resource,
				"scope":                 "read write",
				"state":                 "instance-" + surface,
				"code_challenge":        "instance-code-challenge",
				"code_challenge_method": "S256",
			}, nil)
			require.NoError(t, err)
			ctx.Set("username", "alice")

			resp := requireStatus(t, http.StatusFound)(h.HandleOAuthAuthorizeLift(ctx))
			redirectURL, err := url.Parse(firstStringValue(resp.Headers, "location"))
			require.NoError(t, err)
			require.NotEmpty(t, redirectURL.Query().Get("code"))
			require.NotNil(t, issuedAuthCode)
			require.Equal(t, resource, issuedAuthCode.Resource)
			require.Equal(t, "alice", issuedAuthCode.Username)
			require.Equal(t, "alice", issuedAuthCode.PrincipalUsername)
		})
	}
}

func TestOAuthInstancePlaneTokenAudienceBinding(t *testing.T) {
	for _, surface := range []string{oauthInstanceSurfacePtah, oauthInstanceSurfaceBa} {
		t.Run(surface+" success", func(t *testing.T) {
			cfg := round11TestConfig()
			resource := oauthInstanceTestResource(surface)
			state := &round10QueryState{
				oauthClientsByID: map[string]storagemodels.OAuthClient{
					"instance-client": {
						ClientID:     "instance-client",
						ClientSecret: "secret",
						RedirectURIs: []string{"https://client.example/callback"},
						Scopes:       []string{auth.ScopeRead, auth.ScopeWrite},
						ClientClass:  auth.ClientClassCLI,
						Confidential: true,
						CreatedAt:    time.Now().Add(-24 * time.Hour),
					},
				},
				authorizationCodesByCode: map[string]storagemodels.AuthorizationCode{
					"instance-code-" + surface: {
						Code:              "instance-code-" + surface,
						ClientID:          "instance-client",
						RedirectURI:       "https://client.example/callback",
						Resource:          resource,
						Username:          "alice",
						PrincipalUsername: "alice",
						ExpiresAt:         time.Now().Add(10 * time.Minute),
						Scopes:            []string{auth.ScopeRead, auth.ScopeWrite},
					},
				},
				usersByUsername: map[string]storagemodels.User{
					"alice": {Username: "alice", Approved: true},
				},
			}
			h, _, _ := round11NewHandler(t, cfg, state)

			params := url.Values{
				"grant_type":    {oauthGrantTypeAuthorizationCode},
				"code":          {"instance-code-" + surface},
				"client_id":     {"instance-client"},
				"client_secret": {"secret"},
				"redirect_uri":  {"https://client.example/callback"},
				"resource":      {resource},
			}
			ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte(params.Encode()))

			resp := requireStatus(t, http.StatusOK)(h.HandleOAuthTokenLift(ctx))
			var body apimodels.OAuthTokenResponse
			require.NoError(t, json.Unmarshal(resp.Body, &body))
			require.NotEmpty(t, body.AccessToken)
			require.NotEmpty(t, body.RefreshToken)

			claims := round12DecodeJWTClaims(t, body.AccessToken)
			require.Equal(t, []string{resource}, []string(claims.Audience))
			require.Equal(t, "alice", claims.Username)
			require.False(t, claims.IsAgent)
			require.Equal(t, resource, state.refreshTokensByToken[body.RefreshToken].Resource)

			refreshParams := url.Values{
				"grant_type":    {oauthGrantTypeRefreshToken},
				"refresh_token": {body.RefreshToken},
				"client_id":     {"instance-client"},
				"client_secret": {"secret"},
			}
			refreshResp := requireStatus(t, http.StatusOK)(h.HandleOAuthTokenLift(round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte(refreshParams.Encode()))))
			var refreshBody apimodels.OAuthTokenResponse
			require.NoError(t, json.Unmarshal(refreshResp.Body, &refreshBody))
			refreshClaims := round12DecodeJWTClaims(t, refreshBody.AccessToken)
			require.Equal(t, []string{resource}, []string(refreshClaims.Audience))
			require.Equal(t, resource, state.refreshTokensByToken[refreshBody.RefreshToken].Resource)
		})

		t.Run(surface+" mismatch is rejected", func(t *testing.T) {
			cfg := round11TestConfig()
			resource := oauthInstanceTestResource(surface)
			state := &round10QueryState{
				oauthClientsByID: map[string]storagemodels.OAuthClient{
					"instance-client": {
						ClientID:     "instance-client",
						ClientSecret: "secret",
						RedirectURIs: []string{"https://client.example/callback"},
						ClientClass:  auth.ClientClassCLI,
						Confidential: true,
						CreatedAt:    time.Now().Add(-24 * time.Hour),
					},
				},
				authorizationCodesByCode: map[string]storagemodels.AuthorizationCode{
					"instance-mismatch-" + surface: {
						Code:        "instance-mismatch-" + surface,
						ClientID:    "instance-client",
						RedirectURI: "https://client.example/callback",
						Resource:    resource,
						Username:    "alice",
						ExpiresAt:   time.Now().Add(10 * time.Minute),
						Scopes:      []string{auth.ScopeRead},
					},
				},
			}
			h, _, _ := round11NewHandler(t, cfg, state)
			params := url.Values{
				"grant_type":    {oauthGrantTypeAuthorizationCode},
				"code":          {"instance-mismatch-" + surface},
				"client_id":     {"instance-client"},
				"client_secret": {"secret"},
				"redirect_uri":  {"https://client.example/callback"},
				"resource":      {oauthInstanceOtherSurfaceResource(surface)},
			}

			resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte(params.Encode()))))
			var body map[string]string
			require.NoError(t, json.Unmarshal(resp.Body, &body))
			require.Equal(t, "invalid_target", body["error"])
		})
	}
}

func TestOAuthInstancePlaneDoesNotReuseActorScopedAuthorization(t *testing.T) {
	cfg := round11TestConfig()
	instanceResource := oauthInstanceTestResource(oauthInstanceSurfacePtah)
	actorResource := auth.BuildPublicMCPAccessBundle(cfg.BaseURL(), "agent1").MCPURL
	state := &round10QueryState{
		oauthClientsByID: map[string]storagemodels.OAuthClient{
			"actor-client": {
				ClientID:     "actor-client",
				ClientSecret: "secret",
				RedirectURIs: []string{"https://client.example/callback"},
				ClientClass:  auth.ClientClassAgent,
				Confidential: true,
				CreatedAt:    time.Now().Add(-24 * time.Hour),
			},
		},
		authorizationCodesByCode: map[string]storagemodels.AuthorizationCode{
			"actor-code": {
				Code:        "actor-code",
				ClientID:    "actor-client",
				RedirectURI: "https://client.example/callback",
				Resource:    actorResource,
				Username:    "agent1",
				ExpiresAt:   time.Now().Add(10 * time.Minute),
				Scopes:      []string{auth.ScopeRead},
			},
		},
	}
	h, _, _ := round11NewHandler(t, cfg, state)
	params := url.Values{
		"grant_type":    {oauthGrantTypeAuthorizationCode},
		"code":          {"actor-code"},
		"client_id":     {"actor-client"},
		"client_secret": {"secret"},
		"redirect_uri":  {"https://client.example/callback"},
		"resource":      {instanceResource},
	}

	resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte(params.Encode()))))
	var body map[string]string
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.Equal(t, "invalid_target", body["error"])
}

func TestOAuthInstancePlaneTokenRejectsAgentAuthorization(t *testing.T) {
	resource := oauthInstanceTestResource(oauthInstanceSurfacePtah)

	tests := []struct {
		name             string
		clientClass      string
		username         string
		principal        string
		principalIsAgent bool
	}{
		{
			name:             "agent client",
			clientClass:      auth.ClientClassAgent,
			username:         "alice",
			principal:        "alice",
			principalIsAgent: false,
		},
		{
			name:             "agent principal",
			clientClass:      auth.ClientClassCLI,
			username:         "agent1",
			principal:        "agent1",
			principalIsAgent: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &round10QueryState{
				oauthClientsByID: map[string]storagemodels.OAuthClient{
					"instance-client": {
						ClientID:     "instance-client",
						ClientSecret: "secret",
						RedirectURIs: []string{"https://client.example/callback"},
						ClientClass:  tt.clientClass,
						Confidential: true,
						CreatedAt:    time.Now().Add(-24 * time.Hour),
					},
				},
				authorizationCodesByCode: map[string]storagemodels.AuthorizationCode{
					"instance-agent-code": {
						Code:              "instance-agent-code",
						ClientID:          "instance-client",
						RedirectURI:       "https://client.example/callback",
						Resource:          resource,
						Username:          tt.username,
						PrincipalUsername: tt.principal,
						ExpiresAt:         time.Now().Add(10 * time.Minute),
						Scopes:            []string{auth.ScopeRead},
					},
				},
				usersByUsername: map[string]storagemodels.User{
					"alice":  {Username: "alice", Approved: true},
					"agent1": {Username: "agent1", IsAgent: tt.principalIsAgent, AgentOwner: "@alice"},
				},
			}
			h, _, _ := round11NewHandler(t, round11TestConfig(), state)

			params := url.Values{
				"grant_type":    {oauthGrantTypeAuthorizationCode},
				"code":          {"instance-agent-code"},
				"client_id":     {"instance-client"},
				"client_secret": {"secret"},
				"redirect_uri":  {"https://client.example/callback"},
				"resource":      {resource},
			}
			resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte(params.Encode()))))
			var body map[string]string
			require.NoError(t, json.Unmarshal(resp.Body, &body))
			require.Equal(t, "invalid_target", body["error"])
		})
	}
}

func TestOAuthInstancePlaneRefreshRejectsAgentAuthorization(t *testing.T) {
	resource := oauthInstanceTestResource(oauthInstanceSurfaceBa)
	tests := []struct {
		name        string
		clientClass string
		storedClass string
	}{
		{name: "agent client", clientClass: auth.ClientClassAgent},
		{name: "agent token class", clientClass: auth.ClientClassCLI, storedClass: auth.ClientClassAgent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &round10QueryState{
				oauthClientsByID: map[string]storagemodels.OAuthClient{
					"instance-client": {
						ClientID:     "instance-client",
						ClientSecret: "secret",
						RedirectURIs: []string{"https://client.example/callback"},
						ClientClass:  tt.clientClass,
						Confidential: true,
						CreatedAt:    time.Now().Add(-24 * time.Hour),
					},
				},
				refreshTokensByToken: map[string]storagemodels.RefreshToken{
					"instance-refresh": {
						Token:       "instance-refresh",
						ClientID:    "instance-client",
						Username:    "alice",
						Resource:    resource,
						ClientClass: tt.storedClass,
						Scopes:      []string{auth.ScopeRead},
						CreatedAt:   time.Now().Add(-time.Hour),
						ExpiresAt:   time.Now().Add(time.Hour),
					},
				},
				usersByUsername: map[string]storagemodels.User{
					"alice": {Username: "alice", Approved: true},
				},
			}
			h, _, _ := round11NewHandler(t, round11TestConfig(), state)

			params := url.Values{
				"grant_type":    {oauthGrantTypeRefreshToken},
				"refresh_token": {"instance-refresh"},
				"client_id":     {"instance-client"},
				"client_secret": {"secret"},
			}
			resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte(params.Encode()))))
			var body map[string]string
			require.NoError(t, json.Unmarshal(resp.Body, &body))
			require.Equal(t, "invalid_grant", body["error"])
		})
	}
}

func TestOAuthActorScopedResourceRegression(t *testing.T) {
	cfg := round11TestConfig()
	actorResource := auth.BuildPublicMCPAccessBundle(cfg.BaseURL(), "agent1").MCPURL
	var issuedAuthCode *storage.AuthorizationCode
	accountsSvc := &AccountsServiceStub{
		GetUserAppConsentFunc: func(context.Context, *accounts.GetUserAppConsentQuery) (*accounts.GetUserAppConsentResult, error) {
			return &accounts.GetUserAppConsentResult{Consent: &storage.UserAppConsent{Scopes: []string{auth.ScopeRead}}}, nil
		},
		CreateAuthorizationCodeFunc: func(_ context.Context, cmd *accounts.CreateAuthorizationCodeCommand) (*accounts.CreateAuthorizationCodeResult, error) {
			issuedAuthCode = cmd.AuthCode
			return &accounts.CreateAuthorizationCodeResult{}, nil
		},
	}
	state := &round10QueryState{
		oauthClientsByID: map[string]storagemodels.OAuthClient{
			"actor-client": {
				ClientID:     "actor-client",
				Name:         "Actor Connector",
				RedirectURIs: []string{"https://client.example/callback"},
				Scopes:       []string{auth.ScopeRead},
				ClientClass:  auth.ClientClassAgent,
				Confidential: true,
				CreatedAt:    time.Now().Add(-24 * time.Hour),
			},
		},
		usersByUsername: map[string]storagemodels.User{
			"agent1": {Username: "agent1", IsAgent: true, AgentOwner: "@alice"},
		},
	}
	h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{AccountsSvc: accountsSvc})
	ctx, err := round10NewLiftContext(http.MethodGet, "/oauth/authorize", nil, map[string]string{
		"response_type": "code",
		"client_id":     "actor-client",
		"redirect_uri":  "https://client.example/callback",
		"resource":      actorResource,
		"scope":         "read",
		"state":         "actor-success",
	}, nil)
	require.NoError(t, err)
	ctx.Set("username", "alice")

	resp := requireStatus(t, http.StatusFound)(h.HandleOAuthAuthorizeLift(ctx))
	parsed, err := url.Parse(firstStringValue(resp.Headers, "location"))
	require.NoError(t, err)
	require.NotEmpty(t, parsed.Query().Get("code"))
	require.NotNil(t, issuedAuthCode)
	require.Equal(t, actorResource, issuedAuthCode.Resource)
	require.Equal(t, "agent1", issuedAuthCode.Username)
}

func TestOAuthInstancePlaneRejectsUnknownResourceAndAgentPrincipal(t *testing.T) {
	for _, tt := range []struct {
		name     string
		resource string
	}{
		{name: "unknown surface", resource: "https://api.example.com/instance/unknown/mcp"},
		{name: "wrong host", resource: "https://api.other.example/instance/ptah/mcp"},
		{name: "trailing slash", resource: "https://api.example.com/instance/ptah/mcp/"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := round11TestConfig()
			state := &round10QueryState{
				oauthClientsByID: map[string]storagemodels.OAuthClient{
					"instance-client": {
						ClientID:     "instance-client",
						RedirectURIs: []string{"https://client.example/callback"},
						ClientClass:  auth.ClientClassCLI,
						Confidential: true,
						CreatedAt:    time.Now().Add(-24 * time.Hour),
					},
				},
			}
			h, _, _ := round11NewHandler(t, cfg, state)
			ctx, err := round10NewLiftContext(http.MethodGet, "/oauth/authorize", nil, map[string]string{
				"response_type": "code",
				"client_id":     "instance-client",
				"redirect_uri":  "https://client.example/callback",
				"resource":      tt.resource,
			}, nil)
			require.NoError(t, err)
			ctx.Set("username", "alice")

			resp := requireStatus(t, http.StatusFound)(h.HandleOAuthAuthorizeLift(ctx))
			require.Contains(t, firstStringValue(resp.Headers, "location"), "error=invalid_target")
		})
	}

	t.Run("agent principal", func(t *testing.T) {
		cfg := round11TestConfig()
		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{
				"instance-client": {
					ClientID:     "instance-client",
					RedirectURIs: []string{"https://client.example/callback"},
					ClientClass:  auth.ClientClassCLI,
					Confidential: true,
					CreatedAt:    time.Now().Add(-24 * time.Hour),
				},
			},
			usersByUsername: map[string]storagemodels.User{
				"agent1": {Username: "agent1", IsAgent: true, AgentOwner: "@alice"},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx, err := round10NewLiftContext(http.MethodGet, "/oauth/authorize", nil, map[string]string{
			"response_type": "code",
			"client_id":     "instance-client",
			"redirect_uri":  "https://client.example/callback",
			"resource":      oauthInstanceTestResource(oauthInstanceSurfacePtah),
		}, nil)
		require.NoError(t, err)
		ctx.Set("username", "agent1")

		resp := requireStatus(t, http.StatusFound)(h.HandleOAuthAuthorizeLift(ctx))
		require.Contains(t, firstStringValue(resp.Headers, "location"), "error=access_denied")
	})
}

func TestOAuthInstancePlaneRejectsAgentClient(t *testing.T) {
	cfg := round11TestConfig()
	state := &round10QueryState{
		oauthClientsByID: map[string]storagemodels.OAuthClient{
			"agent-client": {
				ClientID:      "agent-client",
				RedirectURIs:  []string{"https://client.example/callback"},
				ClientClass:   auth.ClientClassAgent,
				AgentUsername: "agent1",
				Confidential:  true,
				CreatedAt:     time.Now().Add(-24 * time.Hour),
			},
		},
	}
	h, _, _ := round11NewHandler(t, cfg, state)
	ctx, err := round10NewLiftContext(http.MethodGet, "/oauth/authorize", nil, map[string]string{
		"response_type": "code",
		"client_id":     "agent-client",
		"redirect_uri":  "https://client.example/callback",
		"resource":      oauthInstanceTestResource(oauthInstanceSurfaceBa),
	}, nil)
	require.NoError(t, err)
	ctx.Set("username", "alice")

	resp := requireStatus(t, http.StatusFound)(h.HandleOAuthAuthorizeLift(ctx))
	require.Contains(t, firstStringValue(resp.Headers, "location"), "error=access_denied")
}

func oauthInstanceTestResource(surface string) string {
	return "https://api.example.com/instance/" + surface + "/mcp"
}

func oauthInstanceOtherSurfaceResource(surface string) string {
	if surface == oauthInstanceSurfacePtah {
		return oauthInstanceTestResource(oauthInstanceSurfaceBa)
	}
	return oauthInstanceTestResource(oauthInstanceSurfacePtah)
}

func TestOAuthInstancePlaneResourceGuards(t *testing.T) {
	require.False(t, oauthResourceTargetsInstancePlane("%"))
	require.True(t, oauthResourceTargetsInstancePlane("https://api.example.com/instance/ptah/mcp"))

	require.False(t, oauthClientCanAuthorizeInstanceResource(nil))
	require.True(t, oauthClientCanAuthorizeInstanceResource(&storage.OAuthClient{ClientClass: auth.ClientClassCLI}))
	require.False(t, oauthClientCanAuthorizeInstanceResource(&storage.OAuthClient{ClientClass: auth.ClientClassAgent}))
	require.False(t, oauthClientCanAuthorizeInstanceResource(&storage.OAuthClient{AgentUsername: "agent1"}))
}
