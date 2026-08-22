package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
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

func TestOAuthInstancePlaneDynamicPublicClientOwnerGetsOperatorClaims(t *testing.T) {
	for _, clientClass := range []string{auth.ClientClassCLI, auth.ClientClassWeb} {
		for _, surface := range []string{oauthInstanceSurfacePtah, oauthInstanceSurfaceBa} {
			t.Run(clientClass+"/"+surface, func(t *testing.T) {
				cfg := round11TestConfig()
				resource := oauthInstanceTestResource(surface)
				redirectURI := "http://127.0.0.1:8787/callback"
				verifier := "lesser-owner-operator-verifier-rfc7636-" + clientClass + "-" + surface
				challengeBytes := sha256.Sum256([]byte(verifier))
				challenge := base64.RawURLEncoding.EncodeToString(challengeBytes[:])
				state := &round10QueryState{
					usersByUsername: map[string]storagemodels.User{
						"owner": {Username: "owner", Approved: true, Role: "admin"},
					},
				}

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
				h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{AccountsSvc: accountsSvc})

				registerCtx, err := round10NewLiftContext(http.MethodPost, "/oauth/register", map[string]string{
					"Content-Type": "application/json",
				}, nil, apimodels.OAuthDynamicClientRegistrationRequest{
					ClientName:              "Ptah public " + clientClass,
					ClientClass:             clientClass,
					RedirectURIs:            []string{redirectURI},
					Scope:                   "read write",
					TokenEndpointAuthMethod: oauthTokenEndpointAuthMethodNone,
				})
				require.NoError(t, err)
				registerResp := requireStatus(t, http.StatusCreated)(h.HandleOAuthDynamicClientRegistrationLift(registerCtx))
				var registration apimodels.OAuthDynamicClientRegistrationResponse
				require.NoError(t, json.Unmarshal(registerResp.Body, &registration))
				require.NotEmpty(t, registration.ClientID)
				require.Empty(t, registration.ClientSecret)
				require.Equal(t, clientClass, registration.ClientClass)
				require.NotContains(t, state.oauthClientsByID[registration.ClientID].Scopes, auth.ScopeAdmin)
				require.False(t, state.oauthClientsByID[registration.ClientID].Confidential)

				authorizeCtx, err := round10NewLiftContext(http.MethodGet, "/oauth/authorize", nil, map[string]string{
					"response_type":         "code",
					"client_id":             registration.ClientID,
					"redirect_uri":          redirectURI,
					"resource":              resource,
					"scope":                 "read write",
					"state":                 "owner-operator-" + surface,
					"code_challenge":        challenge,
					"code_challenge_method": "S256",
				}, nil)
				require.NoError(t, err)
				authorizeCtx.Set("username", "owner")

				authorizeResp := requireStatus(t, http.StatusFound)(h.HandleOAuthAuthorizeLift(authorizeCtx))
				authorizeURL, err := url.Parse(firstStringValue(authorizeResp.Headers, "location"))
				require.NoError(t, err)
				require.NotEmpty(t, authorizeURL.Query().Get("code"))
				require.NotNil(t, issuedAuthCode)
				require.Equal(t, resource, issuedAuthCode.Resource)
				require.Equal(t, "owner", issuedAuthCode.Username)
				require.Equal(t, "owner", issuedAuthCode.PrincipalUsername)
				require.ElementsMatch(t, []string{auth.ScopeRead, auth.ScopeWrite}, issuedAuthCode.Scopes)

				state.authorizationCodesByCode = map[string]storagemodels.AuthorizationCode{
					issuedAuthCode.Code: {
						Code:              issuedAuthCode.Code,
						ClientID:          issuedAuthCode.ClientID,
						RedirectURI:       issuedAuthCode.RedirectURI,
						Resource:          issuedAuthCode.Resource,
						Username:          issuedAuthCode.Username,
						PrincipalUsername: issuedAuthCode.PrincipalUsername,
						CodeChallenge:     issuedAuthCode.CodeChallenge,
						ExpiresAt:         issuedAuthCode.ExpiresAt,
						Scopes:            issuedAuthCode.Scopes,
					},
				}

				tokenParams := url.Values{
					"grant_type":    {auth.GrantTypeAuthorizationCode},
					"code":          {issuedAuthCode.Code},
					"client_id":     {registration.ClientID},
					"redirect_uri":  {redirectURI},
					"resource":      {resource},
					"code_verifier": {verifier},
				}
				tokenResp := requireStatus(t, http.StatusOK)(h.HandleOAuthTokenLift(round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte(tokenParams.Encode()))))
				var tokenBody apimodels.OAuthTokenResponse
				require.NoError(t, json.Unmarshal(tokenResp.Body, &tokenBody))
				require.NotEmpty(t, tokenBody.AccessToken)
				require.NotEmpty(t, tokenBody.RefreshToken)

				claims := round12DecodeJWTClaims(t, tokenBody.AccessToken)
				require.Equal(t, []string{resource}, []string(claims.Audience))
				require.Equal(t, auth.ClientClassOperator, claims.ClientClass)
				require.Equal(t, "owner", claims.Username)
				require.False(t, claims.IsAgent)
				require.ElementsMatch(t, []string{auth.ScopeRead, auth.ScopeWrite}, claims.Scopes)
				require.NotContains(t, claims.Scopes, auth.ScopeAdmin)

				storedRefresh := state.refreshTokensByToken[tokenBody.RefreshToken]
				require.Equal(t, auth.ClientClassOperator, storedRefresh.ClientClass)
				require.Equal(t, resource, storedRefresh.Resource)
				require.ElementsMatch(t, []string{auth.ScopeRead, auth.ScopeWrite}, storedRefresh.Scopes)

				refreshParams := url.Values{
					"grant_type":    {auth.GrantTypeRefreshToken},
					"refresh_token": {tokenBody.RefreshToken},
					"client_id":     {registration.ClientID},
					"resource":      {resource},
					"scope":         {"read write admin"},
				}
				refreshResp := requireStatus(t, http.StatusOK)(h.HandleOAuthTokenLift(round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte(refreshParams.Encode()))))
				var refreshedBody apimodels.OAuthTokenResponse
				require.NoError(t, json.Unmarshal(refreshResp.Body, &refreshedBody))
				refreshedClaims := round12DecodeJWTClaims(t, refreshedBody.AccessToken)
				require.Equal(t, []string{resource}, []string(refreshedClaims.Audience))
				require.Equal(t, auth.ClientClassOperator, refreshedClaims.ClientClass)
				require.ElementsMatch(t, []string{auth.ScopeRead, auth.ScopeWrite}, refreshedClaims.Scopes)
				require.NotContains(t, refreshedClaims.Scopes, auth.ScopeAdmin)
				require.Equal(t, auth.ClientClassOperator, state.refreshTokensByToken[refreshedBody.RefreshToken].ClientClass)
				require.Equal(t, resource, state.refreshTokensByToken[refreshedBody.RefreshToken].Resource)
				rotated := state.refreshTokensByToken[tokenBody.RefreshToken]
				require.True(t, rotated.Revoked)
				require.False(t, rotated.Current)
			})
		}
	}
}

func TestOAuthInstancePlaneDynamicPublicClientNonAdminDoesNotGetOperatorClaims(t *testing.T) {
	cfg := round11TestConfig()
	resource := oauthInstanceTestResource(oauthInstanceSurfacePtah)
	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"member": {Username: "member", Approved: true, Role: "user"},
		},
		authorizationCodesByCode: map[string]storagemodels.AuthorizationCode{
			"member-instance-code": {
				Code:              "member-instance-code",
				ClientID:          "public-client",
				RedirectURI:       "https://client.example/callback",
				Resource:          resource,
				Username:          "member",
				PrincipalUsername: "member",
				ExpiresAt:         time.Now().Add(10 * time.Minute),
				Scopes:            []string{auth.ScopeRead, auth.ScopeWrite},
			},
		},
		oauthClientsByID: map[string]storagemodels.OAuthClient{
			"public-client": {
				ClientID:           "public-client",
				RedirectURIs:       []string{"https://client.example/callback"},
				Scopes:             []string{auth.ScopeRead, auth.ScopeWrite},
				ClientClass:        auth.ClientClassCLI,
				RegistrationSource: oauthRegistrationSourceDynamic,
				CreatedAt:          time.Now().Add(-time.Hour),
			},
		},
	}
	h, _, _ := round11NewHandler(t, cfg, state)

	params := url.Values{
		"grant_type":   {auth.GrantTypeAuthorizationCode},
		"code":         {"member-instance-code"},
		"client_id":    {"public-client"},
		"redirect_uri": {"https://client.example/callback"},
		"resource":     {resource},
	}
	resp := requireStatus(t, http.StatusOK)(h.HandleOAuthTokenLift(round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte(params.Encode()))))
	var body apimodels.OAuthTokenResponse
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	claims := round12DecodeJWTClaims(t, body.AccessToken)
	require.Equal(t, auth.ClientClassCLI, claims.ClientClass)
	require.False(t, claims.IsAgent)
	require.ElementsMatch(t, []string{auth.ScopeRead, auth.ScopeWrite}, claims.Scopes)
	require.NotContains(t, claims.Scopes, auth.ScopeAdmin)
}

func TestOAuthInstancePlaneOperatorClaimIsBoundToExactResource(t *testing.T) {
	h, _, _ := round11NewHandler(t, round11TestConfig(), &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"admin": {Username: "admin", Approved: true, Role: "admin"},
		},
	})
	operatorClient := &storage.OAuthClient{ClientID: "operator-client", ClientClass: auth.ClientClassOperator, OwnerID: "admin"}
	tests := []struct {
		name        string
		clientClass string
		resource    string
		wantClass   string
		wantErr     bool
	}{
		{
			name:        "operator client actor resource is downgraded",
			clientClass: auth.ClientClassOperator,
			resource:    "https://api.example.com/mcp/agent1",
			wantClass:   auth.ClientClassWeb,
		},
		{
			name:        "operator client non instance resource is downgraded",
			clientClass: auth.ClientClassOperator,
			resource:    "https://mcp.example/resource",
			wantClass:   auth.ClientClassWeb,
		},
		{
			name:        "public actor resource stays public",
			clientClass: auth.ClientClassCLI,
			resource:    "https://api.example.com/mcp/agent1",
			wantClass:   auth.ClientClassCLI,
		},
		{
			name:        "unclassified client stays unclassified",
			clientClass: "",
			resource:    oauthInstanceTestResource(oauthInstanceSurfacePtah),
			wantClass:   "",
		},
		{
			name:        "wrong instance host is rejected",
			clientClass: auth.ClientClassCLI,
			resource:    "https://other.example.com/instance/ptah/mcp",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := operatorClient
			if tt.clientClass != auth.ClientClassOperator {
				client = &storage.OAuthClient{ClientID: "public-client", ClientClass: tt.clientClass}
			}
			code := &storage.AuthorizationCode{
				ClientID:          client.ClientID,
				Resource:          tt.resource,
				Username:          "admin",
				PrincipalUsername: "admin",
				Scopes:            []string{auth.ScopeRead, auth.ScopeWrite},
			}
			got, err := h.oauthAuthorizationCodeClientClass(context.Background(), client, code)
			if tt.wantErr {
				require.ErrorIs(t, err, errOAuthInvalidTarget)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantClass, got)
			require.NotEqual(t, auth.ClientClassOperator, got)
		})
	}
}

func TestOAuthInstancePlaneRefreshRejectsOperatorTokenOutsideInstanceResource(t *testing.T) {
	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"admin": {Username: "admin", Approved: true, Role: "admin"},
		},
		oauthClientsByID: map[string]storagemodels.OAuthClient{
			"operator-client": {
				ClientID:     "operator-client",
				ClientSecret: "secret",
				ClientClass:  auth.ClientClassOperator,
				OwnerID:      "admin",
				Confidential: true,
				GrantTypes:   []string{auth.GrantTypeRefreshToken},
			},
		},
		refreshTokensByToken: map[string]storagemodels.RefreshToken{
			"operator-refresh": {
				Token:       "operator-refresh",
				ClientID:    "operator-client",
				Username:    "admin",
				Resource:    "https://mcp.example/resource",
				ClientClass: auth.ClientClassOperator,
				Scopes:      []string{auth.ScopeRead, auth.ScopeWrite},
				ExpiresAt:   time.Now().Add(time.Hour),
			},
		},
	}
	h, _, _ := round11NewHandler(t, round11TestConfig(), state)
	params := url.Values{
		"grant_type":    {auth.GrantTypeRefreshToken},
		"refresh_token": {"operator-refresh"},
		"client_id":     {"operator-client"},
		"client_secret": {"secret"},
	}
	resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte(params.Encode()))))
	var body map[string]string
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.Equal(t, "invalid_grant", body["error"])
}

func TestOAuthAuthorizationCodeScopesCannotRequestOperatorAuthority(t *testing.T) {
	publicClient := &storage.OAuthClient{
		ClientID:    "public-client",
		ClientClass: auth.ClientClassCLI,
		Scopes:      []string{auth.ScopeRead, auth.ScopeWrite},
	}
	operatorClient := &storage.OAuthClient{
		ClientID:    "operator-client",
		ClientClass: auth.ClientClassOperator,
		Scopes:      []string{auth.ScopeRead, auth.ScopeWrite, auth.ScopeAdmin},
	}
	validCode := &storage.AuthorizationCode{Scopes: []string{auth.ScopeRead}}
	require.ErrorIs(t, validateOAuthAuthorizationCodeScopes(nil, validCode), auth.ErrInvalidGrant)
	require.ErrorIs(t, validateOAuthAuthorizationCodeScopes(publicClient, &storage.AuthorizationCode{
		Scopes: []string{"not-a-public-scope"},
	}), auth.ErrInvalidScope)

	tests := []struct {
		name    string
		client  *storage.OAuthClient
		code    *storage.AuthorizationCode
		wantErr error
	}{
		{
			name:   "public client cannot carry admin scope",
			client: publicClient,
			code: &storage.AuthorizationCode{
				Resource: "https://api.example.com/instance/ptah/mcp",
				Scopes:   []string{auth.ScopeRead, auth.ScopeAdmin},
			},
			wantErr: auth.ErrInvalidScope,
		},
		{
			name:   "operator client cannot carry admin scope outside instance plane",
			client: operatorClient,
			code: &storage.AuthorizationCode{
				Resource: "https://mcp.example/resource",
				Scopes:   []string{auth.ScopeRead, "admin:read"},
			},
			wantErr: auth.ErrInvalidScope,
		},
		{
			name: "authorization code cannot exceed client grant",
			client: &storage.OAuthClient{
				ClientID:    "restricted-client",
				ClientClass: auth.ClientClassCLI,
				Scopes:      []string{auth.ScopeRead},
			},
			code: &storage.AuthorizationCode{
				Resource: "https://api.example.com/mcp/agent1",
				Scopes:   []string{auth.ScopeRead, auth.ScopeWrite},
			},
			wantErr: auth.ErrInvalidScope,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOAuthAuthorizationCodeScopes(tt.client, tt.code)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestOAuthInstanceOperatorPrincipalRequiresActiveLocalAdmin(t *testing.T) {
	h, _, _ := round11NewHandler(t, round11TestConfig(), &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"active-admin":     {Username: "active-admin", Approved: true, Role: "admin"},
			"ordinary-user":    {Username: "ordinary-user", Approved: true, Role: "user"},
			"unapproved-admin": {Username: "unapproved-admin", Approved: false, Role: "admin"},
			"suspended-admin":  {Username: "suspended-admin", Approved: true, Role: "admin", Suspended: true},
			"silenced-admin":   {Username: "silenced-admin", Approved: true, Role: "admin", Silenced: true},
			"agent-admin":      {Username: "agent-admin", Approved: true, Role: "admin", IsAgent: true},
		},
	})

	tests := []struct {
		username string
		want     bool
	}{
		{username: "active-admin", want: true},
		{username: "ordinary-user", want: false},
		{username: "unapproved-admin", want: false},
		{username: "suspended-admin", want: false},
		{username: "silenced-admin", want: false},
		{username: "agent-admin", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.username, func(t *testing.T) {
			got, err := h.oauthInstanceOperatorPrincipalStatus(context.Background(), tt.username)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestOAuthInstanceOperatorHelpersRejectMissingState(t *testing.T) {
	h, _, _ := round11NewHandler(t, round11TestConfig(), &round10QueryState{})
	authorized, err := h.oauthInstanceOperatorPrincipalStatus(context.Background(), "")
	require.NoError(t, err)
	require.False(t, authorized)
	authorized, err = h.oauthInstanceOperatorPrincipalStatus(context.Background(), "missing")
	require.NoError(t, err)
	require.False(t, authorized)
	var nilHandler *Handler
	authorized, err = nilHandler.oauthInstanceOperatorPrincipalStatus(context.Background(), "admin")
	require.NoError(t, err)
	require.False(t, authorized)

	require.Empty(t, oauthAuthorizationCodePrincipalUsername(nil))
	require.Equal(t, "legacy-user", oauthAuthorizationCodePrincipalUsername(&storage.AuthorizationCode{Username: "legacy-user"}))

	_, err = h.oauthAuthorizationCodeClientClass(context.Background(), nil, nil)
	require.ErrorIs(t, err, auth.ErrInvalidGrant)
}

func TestOAuthInstancePlaneRefreshRejectsStaleOperatorPrincipal(t *testing.T) {
	resource := oauthInstanceTestResource(oauthInstanceSurfacePtah)
	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"admin": {
				Username:  "admin",
				Approved:  true,
				Role:      "admin",
				Suspended: true,
			},
		},
		oauthClientsByID: map[string]storagemodels.OAuthClient{
			"public-client": {
				ClientID:     "public-client",
				RedirectURIs: []string{"https://client.example/callback"},
				Scopes:       []string{auth.ScopeRead, auth.ScopeWrite},
				ClientClass:  auth.ClientClassCLI,
			},
		},
		refreshTokensByToken: map[string]storagemodels.RefreshToken{
			"stale-operator-refresh": {
				Token:       "stale-operator-refresh",
				ClientID:    "public-client",
				Username:    "admin",
				Resource:    resource,
				ClientClass: auth.ClientClassOperator,
				Scopes:      []string{auth.ScopeRead, auth.ScopeWrite},
				ExpiresAt:   time.Now().Add(time.Hour),
			},
		},
	}
	h, _, _ := round11NewHandler(t, round11TestConfig(), state)

	params := url.Values{
		"grant_type":    {auth.GrantTypeRefreshToken},
		"refresh_token": {"stale-operator-refresh"},
		"client_id":     {"public-client"},
	}
	resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte(params.Encode()))))
	var body map[string]string
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.Equal(t, "invalid_grant", body["error"])
	require.Contains(t, state.refreshTokensByToken, "stale-operator-refresh")
}

func TestOAuthInstancePlaneAuthorizationCodeUsesLegacyUsernameWhenPrincipalMissing(t *testing.T) {
	resource := oauthInstanceTestResource(oauthInstanceSurfaceBa)
	h, _, _ := round11NewHandler(t, round11TestConfig(), &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"admin": {Username: "admin", Approved: true, Role: "admin"},
		},
	})

	client := &storage.OAuthClient{
		ClientID:    "owner-operator",
		ClientClass: auth.ClientClassOperator,
		OwnerID:     "admin",
	}
	clientClass, err := h.oauthAuthorizationCodeClientClass(context.Background(), client, &storage.AuthorizationCode{
		ClientID: client.ClientID,
		Resource: resource,
		Username: "admin",
		Scopes:   []string{auth.ScopeRead, auth.ScopeWrite},
	})
	require.NoError(t, err)
	require.Equal(t, auth.ClientClassOperator, clientClass)
}

func TestOAuthInstancePlaneResourceAndScopeGuardsCoverNegativeBranches(t *testing.T) {
	_, err := normalizeOAuthResourceIndicator("https://api.example.com/mcp/agent1?")
	require.Error(t, err)

	h, _, _ := round11NewHandler(t, round11TestConfig(), &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"agent1": {Username: "agent1", IsAgent: true, AgentOwner: "@owner"},
		},
	})
	_, _, err = h.resolveAuthorizeTargetActorFromResource(context.Background(), "https://api.example.com/mcp/agent1?", "owner")
	require.Error(t, err)

	require.True(t, oauthScopesContainAdmin([]string{"read", " admin:read "}))
	require.True(t, oauthScopesContainAdmin([]string{"ADMIN"}))
	require.False(t, oauthScopesContainAdmin([]string{auth.ScopeRead, auth.ScopeWrite}))
}
