package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func round12DecodeJWTClaims(t *testing.T, tokenString string) auth.Claims {
	t.Helper()

	parts := strings.Split(tokenString, ".")
	require.Len(t, parts, 3)

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)

	var claims auth.Claims
	require.NoError(t, json.Unmarshal(payloadBytes, &claims))
	return claims
}

func TestOAuthAuthorizeFlowRound12(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("unsupported response type without redirect_uri returns JSON error", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodGet, "/oauth/authorize", nil, map[string]string{
			"response_type": "token",
		}, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthAuthorizeLift(ctx))
		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "unsupported_response_type", body["error"])
	})

	t.Run("missing client_id redirects to auth UI error page in UI mode", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodGet, "/oauth/authorize", nil, map[string]string{
			"mode":          "ui",
			"response_type": "code",
			"redirect_uri":  "https://client.example/redirect",
			"scope":         "read write",
			"state":         "state-1",
		}, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusOK)(h.HandleOAuthAuthorizeLift(ctx))

		var body apimodels.OAuthAuthorizeResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Contains(t, body.NextURL, "https://example.com/auth/oauth/authorize?")
		require.Contains(t, body.NextURL, "error=")
		require.Contains(t, body.NextURL, "redirect_uri=")
		require.Contains(t, body.NextURL, "state=")
	})

	t.Run("invalid redirect_uri returns invalid_request JSON error", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodGet, "/oauth/authorize", nil, map[string]string{
			"response_type": "code",
			"client_id":     "client-1",
			"redirect_uri":  "https://evil.example/cb",
			"state":         "state-2",
		}, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthAuthorizeLift(ctx))
		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_request", body["error"])
	})

	t.Run("missing user session redirects to login in UI mode", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodGet, "/oauth/authorize", nil, map[string]string{
			"mode":          "ui",
			"response_type": "code",
			"client_id":     "client-1",
			"redirect_uri":  "https://example.com/callback",
			"scope":         "read write",
			"state":         "state-3",
		}, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusOK)(h.HandleOAuthAuthorizeLift(ctx))

		var body apimodels.OAuthAuthorizeResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Contains(t, body.NextURL, "https://example.com/auth/login")
		require.Contains(t, body.NextURL, "return_to=%2Foauth%2Fauthorize")
		require.Contains(t, body.NextURL, "auth_request=")
	})

	t.Run("invalid scopes returns invalid_scope redirect (handled)", func(t *testing.T) {
		accountsSvc := &AccountsServiceStub{
			GetUserAppConsentFunc: func(context.Context, *accounts.GetUserAppConsentQuery) (*accounts.GetUserAppConsentResult, error) {
				return &accounts.GetUserAppConsentResult{Consent: &storage.UserAppConsent{Scopes: []string{"read", "write"}}}, nil
			},
			CreateAuthorizationCodeFunc: func(context.Context, *accounts.CreateAuthorizationCodeCommand) (*accounts.CreateAuthorizationCodeResult, error) {
				return &accounts.CreateAuthorizationCodeResult{}, nil
			},
		}
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{AccountsSvc: accountsSvc})

		ctx, err := round10NewLiftContext(http.MethodGet, "/oauth/authorize", nil, map[string]string{
			"mode":          "ui",
			"response_type": "code",
			"client_id":     "client-1",
			"redirect_uri":  "https://example.com/callback",
			"scope":         "read badscope",
			"state":         "state-4",
		}, nil)
		require.NoError(t, err)
		ctx.Set("username", "alice")

		resp := requireStatus(t, http.StatusOK)(h.HandleOAuthAuthorizeLift(ctx))

		var body apimodels.OAuthAuthorizeResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Contains(t, body.NextURL, "error=invalid_scope")
	})

	t.Run("admin scopes are rejected from public oauth requests", func(t *testing.T) {
		accountsSvc := &AccountsServiceStub{
			GetUserAppConsentFunc: func(context.Context, *accounts.GetUserAppConsentQuery) (*accounts.GetUserAppConsentResult, error) {
				return &accounts.GetUserAppConsentResult{Consent: &storage.UserAppConsent{Scopes: []string{"read", "write", "follow", "admin", "admin:read", "admin:write"}}}, nil
			},
		}
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{AccountsSvc: accountsSvc})
		ctx, err := round10NewLiftContext(http.MethodGet, "/oauth/authorize", map[string]string{"Accept": "text/html"}, map[string]string{
			"response_type": "code",
			"client_id":     "client-1",
			"redirect_uri":  "https://example.com/callback",
			"scope":         "read write follow admin admin:read admin:write",
			"state":         "state-admin",
		}, nil)
		require.NoError(t, err)
		ctx.Set("username", "alice")

		resp := requireStatus(t, http.StatusFound)(h.HandleOAuthAuthorizeLift(ctx))
		redirectURL := firstStringValue(resp.Headers, "location")
		require.NotEmpty(t, redirectURL)

		parsed, parseErr := url.Parse(redirectURL)
		require.NoError(t, parseErr)
		require.Empty(t, parsed.Query().Get("code"))
		require.Equal(t, "invalid_scope", parsed.Query().Get("error"))
		require.Equal(t, "state-admin", parsed.Query().Get("state"))
	})

	t.Run("service registry missing returns server_error redirect (handled)", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodGet, "/oauth/authorize", nil, map[string]string{
			"mode":          "ui",
			"response_type": "code",
			"client_id":     "client-1",
			"redirect_uri":  "https://example.com/callback",
			"state":         "state-5",
		}, nil)
		require.NoError(t, err)
		ctx.Set("username", "alice")

		resp := requireStatus(t, http.StatusOK)(h.HandleOAuthAuthorizeLift(ctx))

		var body apimodels.OAuthAuthorizeResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Contains(t, body.NextURL, "error=server_error")
	})

	t.Run("missing consent stores state and redirects to consent UI", func(t *testing.T) {
		var storedState *storage.OAuthState
		accountsSvc := &AccountsServiceStub{
			GetUserAppConsentFunc: func(context.Context, *accounts.GetUserAppConsentQuery) (*accounts.GetUserAppConsentResult, error) {
				return &accounts.GetUserAppConsentResult{}, errors.New("no consent")
			},
			StoreOAuthStateFunc: func(_ context.Context, cmd *accounts.StoreOAuthStateCommand) (*accounts.StoreOAuthStateResult, error) {
				storedState = cmd.OAuthState
				return &accounts.StoreOAuthStateResult{}, nil
			},
			GetOAuthAppFunc: func(_ context.Context, query *accounts.GetOAuthAppQuery) (*accounts.GetOAuthAppResult, error) {
				return &accounts.GetOAuthAppResult{
					App: &storage.OAuthApp{
						ClientID: query.ClientID,
						Name:     "Test App",
						Website:  "https://app.example",
					},
				}, nil
			},
		}

		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{AccountsSvc: accountsSvc})
		ctx, err := round10NewLiftContext(http.MethodGet, "/oauth/authorize", map[string]string{"Accept": "text/html"}, map[string]string{
			"response_type": "code",
			"client_id":     "client-1",
			"redirect_uri":  "https://example.com/callback",
			"scope":         "read write",
			"state":         "state-6",
		}, nil)
		require.NoError(t, err)
		ctx.Set("username", "alice")

		resp := requireStatus(t, http.StatusFound)(h.HandleOAuthAuthorizeLift(ctx))
		require.Contains(t, firstStringValue(resp.Headers, "location"), "/auth/consent?")
		require.NotNil(t, storedState)
	})

	t.Run("agent connector consent stores principal and agent identities", func(t *testing.T) {
		var storedState *storage.OAuthState
		accountsSvc := &AccountsServiceStub{
			GetUserAppConsentFunc: func(context.Context, *accounts.GetUserAppConsentQuery) (*accounts.GetUserAppConsentResult, error) {
				return &accounts.GetUserAppConsentResult{}, errors.New("no consent")
			},
			StoreOAuthStateFunc: func(_ context.Context, cmd *accounts.StoreOAuthStateCommand) (*accounts.StoreOAuthStateResult, error) {
				storedState = cmd.OAuthState
				return &accounts.StoreOAuthStateResult{}, nil
			},
			GetOAuthAppFunc: func(_ context.Context, query *accounts.GetOAuthAppQuery) (*accounts.GetOAuthAppResult, error) {
				return &accounts.GetOAuthAppResult{
					App: &storage.OAuthApp{
						ClientID:      query.ClientID,
						Name:          "Agent Connector",
						Website:       "https://connector.example",
						ClientClass:   auth.ClientClassAgent,
						AgentUsername: "agent1",
					},
				}, nil
			},
		}
		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{
				"client-agent": {
					ClientID:      "client-agent",
					Name:          "Agent Connector",
					RedirectURIs:  []string{"https://example.com/callback"},
					Scopes:        []string{auth.ScopeRead, auth.ScopeWrite},
					ClientClass:   auth.ClientClassAgent,
					AgentUsername: "agent1",
					CreatedAt:     time.Now().Add(-24 * time.Hour),
				},
			},
			usersByUsername: map[string]storagemodels.User{
				"agent1": {Username: "agent1", IsAgent: true, AgentOwner: "@owner"},
			},
		}

		h, _, _ := round11NewHandler(t, cfg, state, &RegistryStub{AccountsSvc: accountsSvc})
		ctx, err := round10NewLiftContext(http.MethodGet, "/oauth/authorize", map[string]string{"Accept": "text/html"}, map[string]string{
			"response_type": "code",
			"client_id":     "client-agent",
			"redirect_uri":  "https://example.com/callback",
			"scope":         "read write",
			"state":         "state-agent-consent",
		}, nil)
		require.NoError(t, err)
		ctx.Set("username", "owner")

		resp := requireStatus(t, http.StatusFound)(h.HandleOAuthAuthorizeLift(ctx))
		require.Contains(t, firstStringValue(resp.Headers, "location"), "agent_username=agent1")
		require.Contains(t, firstStringValue(resp.Headers, "location"), "principal_username=owner")
		require.NotNil(t, storedState)
		require.Equal(t, "agent1", storedState.Username)
		require.Equal(t, "owner", storedState.PrincipalUsername)
		require.Equal(t, "agent1", storedState.AgentUsername)
	})

	t.Run("consent already granted issues authorization code and redirects", func(t *testing.T) {
		var issuedCode string
		accountsSvc := &AccountsServiceStub{
			GetUserAppConsentFunc: func(context.Context, *accounts.GetUserAppConsentQuery) (*accounts.GetUserAppConsentResult, error) {
				return &accounts.GetUserAppConsentResult{Consent: &storage.UserAppConsent{Scopes: []string{"read", "write"}}}, nil
			},
			CreateAuthorizationCodeFunc: func(_ context.Context, cmd *accounts.CreateAuthorizationCodeCommand) (*accounts.CreateAuthorizationCodeResult, error) {
				issuedCode = cmd.AuthCode.Code
				return &accounts.CreateAuthorizationCodeResult{}, nil
			},
		}
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{AccountsSvc: accountsSvc})
		ctx, err := round10NewLiftContext(http.MethodGet, "/oauth/authorize", map[string]string{"Accept": "text/html"}, map[string]string{
			"response_type": "code",
			"client_id":     "client-1",
			"redirect_uri":  "https://example.com/callback",
			"state":         "state-7",
		}, nil)
		require.NoError(t, err)
		ctx.Set("username", "alice")

		resp := requireStatus(t, http.StatusFound)(h.HandleOAuthAuthorizeLift(ctx))

		redirectURL := firstStringValue(resp.Headers, "location")
		require.NotEmpty(t, redirectURL)
		parsed, parseErr := url.Parse(redirectURL)
		require.NoError(t, parseErr)
		require.NotEmpty(t, parsed.Query().Get("code"))
		require.Equal(t, "state-7", parsed.Query().Get("state"))
		require.NotEmpty(t, issuedCode)
	})

	t.Run("store OAuth state failure returns server_error redirect (handled)", func(t *testing.T) {
		accountsSvc := &AccountsServiceStub{
			GetUserAppConsentFunc: func(context.Context, *accounts.GetUserAppConsentQuery) (*accounts.GetUserAppConsentResult, error) {
				return &accounts.GetUserAppConsentResult{}, nil
			},
			StoreOAuthStateFunc: func(context.Context, *accounts.StoreOAuthStateCommand) (*accounts.StoreOAuthStateResult, error) {
				return nil, errors.New("save failed")
			},
		}
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{AccountsSvc: accountsSvc})
		ctx, err := round10NewLiftContext(http.MethodGet, "/oauth/authorize", nil, map[string]string{
			"mode":          "ui",
			"response_type": "code",
			"client_id":     "client-1",
			"redirect_uri":  "https://example.com/callback",
			"state":         "state-8",
		}, nil)
		require.NoError(t, err)
		ctx.Set("username", "alice")

		resp := requireStatus(t, http.StatusOK)(h.HandleOAuthAuthorizeLift(ctx))

		var body apimodels.OAuthAuthorizeResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Contains(t, body.NextURL, "error=server_error")
	})
}

func TestOAuthAuthorizeHelpersRound12(t *testing.T) {
	cfg := round11TestConfig()
	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	t.Run("accept header lowercase triggers UI mode JSON response", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/oauth/authorize", map[string]string{"accept": "application/json"}, nil, nil)
		require.NoError(t, err)

		require.True(t, h.isOAuthAuthorizeUIMode(ctx))
		resp := requireStatus(t, http.StatusOK)(h.writeOAuthAuthorizeRedirect(ctx, "https://next.example/step"))
		var body apimodels.OAuthAuthorizeResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "https://next.example/step", body.NextURL)
	})

	t.Run("oauthErrorLift falls back to JSON on invalid redirect URI", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/oauth/authorize", nil, nil, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusBadRequest)(h.oauthErrorLift(ctx, "invalid_request", "Invalid redirect_uri", "https://example.com/\n", ""))
		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_request", body["error"])
	})

	t.Run("getUserFromSessionLift falls back to bearer token", func(t *testing.T) {
		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		ctx, err := round10NewLiftContext(http.MethodGet, "/oauth/authorize", map[string]string{
			"Authorization": "Bearer " + token,
		}, nil, nil)
		require.NoError(t, err)

		require.Equal(t, "alice", h.getUserFromSessionLift(ctx))
	})

	t.Run("getUserFromSessionLift returns empty on invalid token", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/oauth/authorize", map[string]string{
			"Authorization": "Bearer invalid",
		}, nil, nil)
		require.NoError(t, err)

		require.Empty(t, h.getUserFromSessionLift(ctx))
	})

	t.Run("redirectToConsentUI returns JSON error when registry is nil", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/oauth/authorize", nil, nil, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusInternalServerError)(h.redirectToConsentUI(ctx, &storage.OAuthState{
			State:       "state-1",
			ClientID:    "client-1",
			RedirectURI: "https://example.com/callback",
			Scopes:      []string{"read"},
		}))
		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "server_error", body["error"])
	})

	t.Run("redirectToConsentUI returns error when client lookup fails", func(t *testing.T) {
		accountsSvc := &AccountsServiceStub{
			GetOAuthAppFunc: func(context.Context, *accounts.GetOAuthAppQuery) (*accounts.GetOAuthAppResult, error) {
				return nil, errors.New("no client")
			},
		}
		hWithRegistry, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{AccountsSvc: accountsSvc})
		ctx, err := round10NewLiftContext(http.MethodGet, "/oauth/authorize", nil, nil, nil)
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusBadRequest)(hWithRegistry.redirectToConsentUI(ctx, &storage.OAuthState{
			State:       "state-1",
			ClientID:    "client-1",
			RedirectURI: "https://example.com/callback",
			Scopes:      []string{"read"},
		}))
		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_client", body["error"])
	})
}

func TestOAuthTokenLiftRound12(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("empty request body", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, nil)
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(ctx))
		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_request", body["error"])
	})

	t.Run("invalid form encoding", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("a=%ZZ"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(ctx))
		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_request", body["error"])
	})

	t.Run("unsupported grant type", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=password&client_id=client-1"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(ctx))
		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "unsupported_grant_type", body["error"])
	})

	t.Run("authorization_code missing params", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=authorization_code&client_id=client-1"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(ctx))
		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_request", body["error"])
	})

	t.Run("authorization_code invalid_client", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=authorization_code&code=code-1&client_id=client-1&redirect_uri=https%3A%2F%2Fexample.com%2Fcallback&client_secret=wrong"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(ctx))
		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_client", body["error"])
	})

	t.Run("authorization_code confidential client requires client_secret", func(t *testing.T) {
		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{
				"client-1": {
					ClientID:     "client-1",
					ClientSecret: "secret",
					Name:         "Test App",
					RedirectURIs: []string{"https://example.com/callback"},
					Scopes:       []string{auth.ScopeRead, auth.ScopeWrite},
					Confidential: true,
					CreatedAt:    time.Now().Add(-24 * time.Hour),
				},
			},
		}

		h, _, _ := round11NewHandler(t, cfg, state)
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=authorization_code&code=code-1&client_id=client-1&redirect_uri=https%3A%2F%2Fexample.com%2Fcallback"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(ctx))
		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_client", body["error"])
	})

	t.Run("authorization_code invalid_grant mismatched client", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=authorization_code&code=code-1&client_id=client-2&redirect_uri=https%3A%2F%2Fexample.com%2Fcallback"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(ctx))
		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_grant", body["error"])
	})

	t.Run("authorization_code invalid_grant when redirect_uri does not match authorization request", func(t *testing.T) {
		redirectA := "https://example.com/callback-a"
		redirectB := "https://example.com/callback-b"

		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{
				"client-1": {
					ClientID:     "client-1",
					ClientSecret: "secret",
					Name:         "Test App",
					RedirectURIs: []string{redirectA, redirectB},
					Scopes:       []string{auth.ScopeRead, auth.ScopeWrite},
					CreatedAt:    time.Now().Add(-24 * time.Hour),
				},
			},
			authorizationCodesByCode: map[string]storagemodels.AuthorizationCode{
				"swap": {
					Code:        "swap",
					ClientID:    "client-1",
					RedirectURI: redirectA,
					Username:    "alice",
					ExpiresAt:   time.Now().Add(5 * time.Minute),
					Scopes:      []string{auth.ScopeRead, auth.ScopeWrite},
				},
			},
		}

		h, _, _ := round11NewHandler(t, cfg, state)
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=authorization_code&code=swap&client_id=client-1&redirect_uri="+url.QueryEscape(redirectB)))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(ctx))
		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_grant", body["error"])
	})

	t.Run("authorization_code PKCE verification failed", func(t *testing.T) {
		verifier := "good-verifier"
		sum := sha256.Sum256([]byte(verifier))
		challenge := base64.RawURLEncoding.EncodeToString(sum[:])

		state := &round10QueryState{
			authorizationCodesByCode: map[string]storagemodels.AuthorizationCode{
				"pkce": {
					Code:          "pkce",
					ClientID:      "client-1",
					RedirectURI:   "https://example.com/callback",
					Username:      "alice",
					CodeChallenge: challenge,
					ExpiresAt:     time.Now().Add(5 * time.Minute),
					Scopes:        []string{auth.ScopeRead, auth.ScopeWrite},
				},
			},
		}

		h, _, _ := round11NewHandler(t, cfg, state)
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=authorization_code&code=pkce&client_id=client-1&redirect_uri=https%3A%2F%2Fexample.com%2Fcallback&code_verifier=wrong"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(ctx))
		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_grant", body["error"])
		require.Contains(t, body["error_description"], "PKCE")
	})

	t.Run("authorization_code success even when refresh token storage fails", func(t *testing.T) {
		state := &round10QueryState{
			createErrorOnce: errors.New("create failed"),
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=authorization_code&code=code-1&client_id=client-1&redirect_uri=https%3A%2F%2Fexample.com%2Fcallback"))
		resp := requireStatus(t, http.StatusOK)(h.HandleOAuthTokenLift(ctx))
		var body apimodels.OAuthTokenResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.NotEmpty(t, body.AccessToken)
		require.NotEmpty(t, body.RefreshToken)
		require.Equal(t, "read write", body.Scope)
	})

	t.Run("authorization_code cli client issues cli tokens", func(t *testing.T) {
		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{
				"client-1": {
					ClientID:     "client-1",
					ClientSecret: "secret",
					Name:         "lesser cli",
					RedirectURIs: []string{"https://example.com/callback"},
					Scopes:       []string{auth.ScopeRead, auth.ScopeWrite},
					ClientClass:  auth.ClientClassCLI,
					CreatedAt:    time.Now().Add(-24 * time.Hour),
				},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=authorization_code&code=code-1&client_id=client-1&redirect_uri=https%3A%2F%2Fexample.com%2Fcallback"))
		resp := requireStatus(t, http.StatusOK)(h.HandleOAuthTokenLift(ctx))
		var body apimodels.OAuthTokenResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.NotEmpty(t, body.AccessToken)
		require.NotEmpty(t, body.RefreshToken)
		require.Equal(t, "read write", body.Scope)

		claims := round12DecodeJWTClaims(t, body.AccessToken)
		require.Equal(t, auth.ClientClassCLI, claims.ClientClass)
		require.NotEmpty(t, claims.SessionID)
	})

	t.Run("authorization_code agent client issues agent runtime-style session", func(t *testing.T) {
		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{
				"client-agent": {
					ClientID:      "client-agent",
					ClientSecret:  "secret",
					Name:          "Connector App",
					RedirectURIs:  []string{"https://example.com/callback"},
					Scopes:        []string{auth.ScopeRead, auth.ScopeWrite},
					ClientClass:   auth.ClientClassAgent,
					AgentUsername: "agent1",
					CreatedAt:     time.Now().Add(-24 * time.Hour),
				},
			},
			authorizationCodesByCode: map[string]storagemodels.AuthorizationCode{
				"agent-code": {
					Code:              "agent-code",
					ClientID:          "client-agent",
					RedirectURI:       "https://example.com/callback",
					Username:          "agent1",
					PrincipalUsername: "owner",
					AgentUsername:     "agent1",
					ExpiresAt:         time.Now().Add(5 * time.Minute),
					Scopes:            []string{auth.ScopeRead, auth.ScopeWrite},
				},
			},
			usersByUsername: map[string]storagemodels.User{
				"agent1": {Username: "agent1", IsAgent: true, AgentType: "counsel", AgentOwner: "@owner"},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=authorization_code&code=agent-code&client_id=client-agent&redirect_uri=https%3A%2F%2Fexample.com%2Fcallback"))
		resp := requireStatus(t, http.StatusOK)(h.HandleOAuthTokenLift(ctx))
		var body apimodels.OAuthTokenResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.NotEmpty(t, body.AccessToken)
		require.NotEmpty(t, body.RefreshToken)

		claims := round12DecodeJWTClaims(t, body.AccessToken)
		require.Equal(t, "agent1", claims.Username)
		require.True(t, claims.IsAgent)
		require.Equal(t, auth.ClientClassAgent, claims.ClientClass)
		require.Equal(t, "@owner", claims.DelegatedBy)
		require.NotEmpty(t, claims.SessionID)

		storedRefresh, ok := state.refreshTokensByToken[body.RefreshToken]
		require.True(t, ok)
		require.Equal(t, auth.ClientClassAgent, storedRefresh.ClientClass)
		require.Equal(t, "agent1", storedRefresh.Username)
		require.NotEmpty(t, storedRefresh.SessionID)
		require.NotEmpty(t, storedRefresh.FamilyID)
		require.True(t, storedRefresh.Current)
		require.Equal(t, 1, storedRefresh.Generation)
		require.Equal(t, "Connector App", storedRefresh.DeviceLabel)
		require.Equal(t, int(auth.AgentAccessTokenTTL(cfg).Seconds()), storedRefresh.AccessTTLSeconds)
	})

	t.Run("refresh_token agent client uses runtime rotation", func(t *testing.T) {
		now := time.Now().UTC()
		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{
				"client-agent": {
					ClientID:      "client-agent",
					ClientSecret:  "secret",
					Name:          "Connector App",
					RedirectURIs:  []string{"https://example.com/callback"},
					Scopes:        []string{auth.ScopeRead},
					ClientClass:   auth.ClientClassAgent,
					AgentUsername: "agent1",
					CreatedAt:     now.Add(-24 * time.Hour),
				},
			},
			refreshTokensByToken: map[string]storagemodels.RefreshToken{
				"rt-agent-connector": buildRuntimeRefreshToken(t, "rt-agent-connector", "agent1", "client-agent", "sid-agent-connector", "family-agent-connector", "Connector App", 1, true, false, now),
			},
			usersByUsername: map[string]storagemodels.User{
				"agent1": {Username: "agent1", IsAgent: true, AgentOwner: "@owner"},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=refresh_token&refresh_token=rt-agent-connector&client_id=client-agent"))
		resp := requireStatus(t, http.StatusOK)(h.HandleOAuthTokenLift(ctx))
		var body apimodels.OAuthTokenResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.NotEmpty(t, body.RefreshToken)
		require.NotEqual(t, "rt-agent-connector", body.RefreshToken)

		oldToken := state.refreshTokensByToken["rt-agent-connector"]
		require.True(t, oldToken.Revoked)
		require.False(t, oldToken.Current)

		newToken, ok := state.refreshTokensByToken[body.RefreshToken]
		require.True(t, ok)
		require.Equal(t, auth.ClientClassAgent, newToken.ClientClass)
		require.Equal(t, oldToken.SessionID, newToken.SessionID)
		require.Equal(t, oldToken.FamilyID, newToken.FamilyID)
		require.Equal(t, oldToken.Generation+1, newToken.Generation)
		require.True(t, newToken.Current)
	})

	t.Run("refresh_token agent client survives long-lived session age", func(t *testing.T) {
		now := time.Now().UTC()
		token := buildRuntimeRefreshToken(t, "rt-agent-aged", "agent1", "client-agent", "sid-agent-aged", "family-agent-aged", "Connector App", 1, true, false, now)
		token.SessionCreatedAt = now.Add(-72 * time.Hour)
		token.LastUsedAt = now.Add(-2 * time.Hour)
		token.IdleExpiresAt = now.Add(36 * time.Hour)
		token.AbsoluteExpiresAt = now.Add(72 * time.Hour)
		token.ExpiresAt = token.IdleExpiresAt
		token.AccessTTLSeconds = int((30 * time.Hour).Seconds())

		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{
				"client-agent": {
					ClientID:      "client-agent",
					ClientSecret:  "secret",
					Name:          "Connector App",
					RedirectURIs:  []string{"https://example.com/callback"},
					Scopes:        []string{auth.ScopeRead},
					ClientClass:   auth.ClientClassAgent,
					AgentUsername: "agent1",
					CreatedAt:     now.Add(-96 * time.Hour),
				},
			},
			refreshTokensByToken: map[string]storagemodels.RefreshToken{
				token.Token: token,
			},
			usersByUsername: map[string]storagemodels.User{
				"agent1": {Username: "agent1", IsAgent: true, AgentOwner: "@owner"},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=refresh_token&refresh_token=rt-agent-aged&client_id=client-agent"))
		resp := requireStatus(t, http.StatusOK)(h.HandleOAuthTokenLift(ctx))
		var body apimodels.OAuthTokenResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.NotEmpty(t, body.AccessToken)
		require.NotEmpty(t, body.RefreshToken)

		claims := round12DecodeJWTClaims(t, body.AccessToken)
		require.Equal(t, auth.ClientClassAgent, claims.ClientClass)
		require.Equal(t, "sid-agent-aged", claims.SessionID)
		require.NotNil(t, claims.IssuedAt)
		require.NotNil(t, claims.ExpiresAt)
		require.Greater(t, claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time), 24*time.Hour)

		oldToken := state.refreshTokensByToken["rt-agent-aged"]
		require.True(t, oldToken.Revoked)
		require.Equal(t, "rotated", oldToken.RevokedReason)

		newToken, ok := state.refreshTokensByToken[body.RefreshToken]
		require.True(t, ok)
		require.Equal(t, token.FamilyID, newToken.FamilyID)
		require.Equal(t, token.Generation+1, newToken.Generation)
		require.Equal(t, token.SessionID, newToken.SessionID)
	})

	t.Run("authorization_code invalid_grant when code consumption fails", func(t *testing.T) {
		state := &round10QueryState{
			deleteErrorOnce: errors.New("delete failed"),
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=authorization_code&code=code-1&client_id=client-1&redirect_uri=https%3A%2F%2Fexample.com%2Fcallback"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(ctx))
		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_grant", body["error"])
	})

	t.Run("refresh_token missing params", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=refresh_token&client_id=client-1"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(ctx))
		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_request", body["error"])
	})

	t.Run("refresh_token invalid_client", func(t *testing.T) {
		state := &round10QueryState{
			refreshTokensByToken: map[string]storagemodels.RefreshToken{
				"rt-1": {Token: "rt-1", ClientID: "client-1", Username: "alice", ExpiresAt: time.Now().Add(1 * time.Hour), Scopes: []string{auth.ScopeRead}},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=refresh_token&refresh_token=rt-1&client_id=client-1&client_secret=wrong"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(ctx))
		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_client", body["error"])
	})

	t.Run("refresh_token confidential client requires client_secret", func(t *testing.T) {
		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{
				"client-1": {
					ClientID:     "client-1",
					ClientSecret: "secret",
					Name:         "Test App",
					RedirectURIs: []string{"https://example.com/callback"},
					Scopes:       []string{auth.ScopeRead, auth.ScopeWrite},
					Confidential: true,
					CreatedAt:    time.Now().Add(-24 * time.Hour),
				},
			},
			refreshTokensByToken: map[string]storagemodels.RefreshToken{
				"rt-1": {Token: "rt-1", ClientID: "client-1", Username: "alice", ExpiresAt: time.Now().Add(1 * time.Hour), Scopes: []string{auth.ScopeRead}},
			},
		}

		h, _, _ := round11NewHandler(t, cfg, state)
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=refresh_token&refresh_token=rt-1&client_id=client-1"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(ctx))
		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_client", body["error"])
	})

	t.Run("refresh_token confidential client accepts previous secret during grace window", func(t *testing.T) {
		activeHash, err := auth.HashOAuthClientSecret("secret-new")
		require.NoError(t, err)
		previousHash, err := auth.HashOAuthClientSecret("secret-old")
		require.NoError(t, err)

		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{
				"client-1": {
					ClientID:                           "client-1",
					ClientSecret:                       activeHash,
					PreviousClientSecret:               previousHash,
					PreviousClientSecretGraceExpiresAt: time.Now().Add(2 * time.Hour),
					Name:                               "Test App",
					RedirectURIs:                       []string{"https://example.com/callback"},
					Scopes:                             []string{auth.ScopeRead, auth.ScopeWrite},
					Confidential:                       true,
					CreatedAt:                          time.Now().Add(-24 * time.Hour),
				},
			},
			refreshTokensByToken: map[string]storagemodels.RefreshToken{
				"rt-1": {Token: "rt-1", ClientID: "client-1", Username: "alice", ExpiresAt: time.Now().Add(1 * time.Hour), Scopes: []string{auth.ScopeRead}},
			},
		}

		h, _, _ := round11NewHandler(t, cfg, state)
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=refresh_token&refresh_token=rt-1&client_id=client-1&client_secret=secret-old"))
		resp := requireStatus(t, http.StatusOK)(h.HandleOAuthTokenLift(ctx))
		var body apimodels.OAuthTokenResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.NotEmpty(t, body.AccessToken)
		require.NotEmpty(t, body.RefreshToken)
		require.Equal(t, "read", body.Scope)
	})

	t.Run("refresh_token invalid_grant when token missing", func(t *testing.T) {
		state := &round10QueryState{
			notFoundPKs: map[string]bool{
				"REFRESHTOKEN#missing": true,
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=refresh_token&refresh_token=missing&client_id=client-1"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(ctx))
		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_grant", body["error"])
	})

	t.Run("refresh_token invalid_grant when token belongs to different client", func(t *testing.T) {
		state := &round10QueryState{
			refreshTokensByToken: map[string]storagemodels.RefreshToken{
				"rt-2": {Token: "rt-2", ClientID: "client-1", Username: "alice", ExpiresAt: time.Now().Add(1 * time.Hour), Scopes: []string{auth.ScopeRead}},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=refresh_token&refresh_token=rt-2&client_id=client-2"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(ctx))
		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_grant", body["error"])
	})

	t.Run("refresh_token returns invalid_grant when storing new token fails", func(t *testing.T) {
		state := &round10QueryState{
			refreshTokensByToken: map[string]storagemodels.RefreshToken{
				"rt-3": {Token: "rt-3", ClientID: "client-1", Username: "alice", ExpiresAt: time.Now().Add(1 * time.Hour), Scopes: []string{auth.ScopeRead}},
			},
			createErrorOnce:  errors.New("create failed"),
			disableAuditRepo: true,
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=refresh_token&refresh_token=rt-3&client_id=client-1"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(ctx))
		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_grant", body["error"])
	})

	t.Run("refresh_token success even when deleting old token fails", func(t *testing.T) {
		state := &round10QueryState{
			refreshTokensByToken: map[string]storagemodels.RefreshToken{
				"rt-4": {Token: "rt-4", ClientID: "client-1", Username: "alice", ExpiresAt: time.Now().Add(1 * time.Hour), Scopes: []string{auth.ScopeRead}},
			},
			deleteErrorOnce: errors.New("delete failed"),
		}
		h, _, _ := round11NewHandler(t, cfg, state)
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=refresh_token&refresh_token=rt-4&client_id=client-1"))
		resp := requireStatus(t, http.StatusOK)(h.HandleOAuthTokenLift(ctx))
		var body apimodels.OAuthTokenResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.NotEmpty(t, body.AccessToken)
		require.NotEmpty(t, body.RefreshToken)
		require.Equal(t, "read", body.Scope)
	})

	t.Run("client_credentials issues access-only agent token with delegated claims", func(t *testing.T) {
		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{
				"client-agent": {
					ClientID:      "client-agent",
					ClientSecret:  "secret",
					Name:          "Agent Connector",
					RedirectURIs:  []string{"https://example.com/callback"},
					GrantTypes:    []string{auth.GrantTypeClientCredentials},
					Scopes:        []string{auth.ScopeRead, auth.ScopeWrite},
					ClientClass:   auth.ClientClassAgent,
					AgentUsername: "agent1",
					OwnerID:       "owner",
					Confidential:  true,
					CreatedAt:     time.Now().Add(-24 * time.Hour),
				},
			},
			usersByUsername: map[string]storagemodels.User{
				"agent1": {
					Username:   "agent1",
					IsAgent:    true,
					AgentType:  "assistant",
					AgentOwner: "@owner",
				},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=client_credentials&client_id=client-agent&client_secret=secret"))
		resp := requireStatus(t, http.StatusOK)(h.HandleOAuthTokenLift(ctx))
		var body apimodels.OAuthTokenResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.NotEmpty(t, body.AccessToken)
		require.Empty(t, body.RefreshToken)
		require.Equal(t, "read write", body.Scope)
		require.Equal(t, int(auth.AgentAccessTokenTTL(cfg).Seconds()), body.ExpiresIn)

		claims := round12DecodeJWTClaims(t, body.AccessToken)
		require.Equal(t, "agent1", claims.Username)
		require.Equal(t, "client-agent", claims.ClientID)
		require.Equal(t, auth.ClientClassAgent, claims.ClientClass)
		require.True(t, claims.IsAgent)
		require.Equal(t, "assistant", claims.AgentType)
		require.Equal(t, "@owner", claims.DelegatedBy)
		require.NotEmpty(t, claims.SessionID)
		require.Equal(t, claims.SessionID, claims.AgentSessionID)
	})

	t.Run("client_credentials rejects non-agent clients", func(t *testing.T) {
		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{
				"client-web": {
					ClientID:     "client-web",
					ClientSecret: "secret",
					Name:         "Web App",
					RedirectURIs: []string{"https://example.com/callback"},
					GrantTypes:   []string{auth.GrantTypeAuthorizationCode, auth.GrantTypeRefreshToken},
					Scopes:       []string{auth.ScopeRead, auth.ScopeWrite},
					ClientClass:  auth.ClientClassWeb,
					Confidential: true,
					CreatedAt:    time.Now().Add(-24 * time.Hour),
				},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=client_credentials&client_id=client-web&client_secret=secret"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(ctx))
		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "unauthorized_client", body["error"])
	})

	t.Run("client_credentials rejects requested scopes outside the client scope set", func(t *testing.T) {
		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{
				"client-agent": {
					ClientID:      "client-agent",
					ClientSecret:  "secret",
					Name:          "Agent Connector",
					RedirectURIs:  []string{"https://example.com/callback"},
					GrantTypes:    []string{auth.GrantTypeClientCredentials},
					Scopes:        []string{auth.ScopeRead},
					ClientClass:   auth.ClientClassAgent,
					AgentUsername: "agent1",
					OwnerID:       "owner",
					Confidential:  true,
					CreatedAt:     time.Now().Add(-24 * time.Hour),
				},
			},
			usersByUsername: map[string]storagemodels.User{
				"agent1": {
					Username:   "agent1",
					IsAgent:    true,
					AgentOwner: "@owner",
				},
			},
		}
		h, _, _ := round11NewHandler(t, cfg, state)

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=client_credentials&client_id=client-agent&client_secret=secret&scope=write"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(ctx))
		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_scope", body["error"])
	})

	t.Run("device_code grant disabled returns access_denied", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type="+oauthDeviceCodeGrantType+"&device_code=dc-1&client_id=client-1"))
		resp := requireStatus(t, http.StatusForbidden)(h.HandleOAuthTokenLift(ctx))
		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "access_denied", body["error"])
	})

	t.Run("device_code missing params", func(t *testing.T) {
		cfgDevice := round11TestConfig()
		cfgDevice.AllowDeviceFlow = true
		h, _, _ := round11NewHandler(t, cfgDevice, &round10QueryState{})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type="+oauthDeviceCodeGrantType+"&client_id=client-1"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(ctx))
		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_request", body["error"])
	})

	t.Run("device_code invalid_grant when device_code missing", func(t *testing.T) {
		cfgDevice := round11TestConfig()
		cfgDevice.AllowDeviceFlow = true
		deviceCode := "missing"
		deviceHash := oauthDeviceCodeHash(deviceCode)

		state := &round10QueryState{
			notFoundPKs: map[string]bool{
				"OAUTH_DEVICE#" + deviceHash: true,
			},
		}
		h, _, _ := round11NewHandler(t, cfgDevice, state)
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type="+oauthDeviceCodeGrantType+"&device_code="+deviceCode+"&client_id=client-1"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(ctx))
		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_grant", body["error"])
	})

	t.Run("device_code expired_token when device session expired", func(t *testing.T) {
		cfgDevice := round11TestConfig()
		cfgDevice.AllowDeviceFlow = true
		deviceCode := "expired"
		deviceHash := oauthDeviceCodeHash(deviceCode)

		state := &round10QueryState{
			oauthDeviceSessionsByHash: map[string]storagemodels.OAuthDeviceSession{
				deviceHash: {
					DeviceCodeHash:  deviceHash,
					UserCode:        "ABCD-EFGH",
					ClientID:        "client-1",
					Scopes:          []string{auth.ScopeRead, auth.ScopeWrite},
					Status:          "pending",
					IntervalSeconds: oauthDevicePollIntervalSeconds,
					CreatedAt:       time.Now().Add(-2 * time.Minute),
					UpdatedAt:       time.Now().Add(-2 * time.Minute),
					ExpiresAt:       time.Now().Add(-1 * time.Minute),
				},
			},
		}
		h, _, _ := round11NewHandler(t, cfgDevice, state)
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type="+oauthDeviceCodeGrantType+"&device_code="+deviceCode+"&client_id=client-1"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(ctx))
		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "expired_token", body["error"])
	})

	t.Run("device_code authorization_pending when pending", func(t *testing.T) {
		cfgDevice := round11TestConfig()
		cfgDevice.AllowDeviceFlow = true
		deviceCode := "pending"
		deviceHash := oauthDeviceCodeHash(deviceCode)

		state := &round10QueryState{
			oauthDeviceSessionsByHash: map[string]storagemodels.OAuthDeviceSession{
				deviceHash: {
					DeviceCodeHash:  deviceHash,
					UserCode:        "ABCD-EFGH",
					ClientID:        "client-1",
					Scopes:          []string{auth.ScopeRead, auth.ScopeWrite},
					Status:          "pending",
					IntervalSeconds: oauthDevicePollIntervalSeconds,
					CreatedAt:       time.Now().Add(-2 * time.Minute),
					UpdatedAt:       time.Now().Add(-2 * time.Minute),
					ExpiresAt:       time.Now().Add(5 * time.Minute),
				},
			},
		}
		h, _, _ := round11NewHandler(t, cfgDevice, state)
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type="+oauthDeviceCodeGrantType+"&device_code="+deviceCode+"&client_id=client-1"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(ctx))
		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "authorization_pending", body["error"])
	})

	t.Run("device_code slow_down when polled too frequently", func(t *testing.T) {
		cfgDevice := round11TestConfig()
		cfgDevice.AllowDeviceFlow = true
		deviceCode := "slow"
		deviceHash := oauthDeviceCodeHash(deviceCode)
		last := time.Now().UTC()

		state := &round10QueryState{
			oauthDeviceSessionsByHash: map[string]storagemodels.OAuthDeviceSession{
				deviceHash: {
					DeviceCodeHash:  deviceHash,
					UserCode:        "ABCD-EFGH",
					ClientID:        "client-1",
					Scopes:          []string{auth.ScopeRead, auth.ScopeWrite},
					Status:          "pending",
					IntervalSeconds: oauthDevicePollIntervalSeconds,
					PollCount:       1,
					LastPolledAt:    &last,
					CreatedAt:       time.Now().Add(-2 * time.Minute),
					UpdatedAt:       time.Now().Add(-2 * time.Minute),
					ExpiresAt:       time.Now().Add(5 * time.Minute),
				},
			},
		}
		h, _, _ := round11NewHandler(t, cfgDevice, state)
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type="+oauthDeviceCodeGrantType+"&device_code="+deviceCode+"&client_id=client-1"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(ctx))
		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "slow_down", body["error"])
	})

	t.Run("device_code access_denied when denied", func(t *testing.T) {
		cfgDevice := round11TestConfig()
		cfgDevice.AllowDeviceFlow = true
		deviceCode := "denied"
		deviceHash := oauthDeviceCodeHash(deviceCode)

		state := &round10QueryState{
			oauthDeviceSessionsByHash: map[string]storagemodels.OAuthDeviceSession{
				deviceHash: {
					DeviceCodeHash:  deviceHash,
					UserCode:        "ABCD-EFGH",
					ClientID:        "client-1",
					Scopes:          []string{auth.ScopeRead, auth.ScopeWrite},
					Status:          "denied",
					IntervalSeconds: oauthDevicePollIntervalSeconds,
					CreatedAt:       time.Now().Add(-2 * time.Minute),
					UpdatedAt:       time.Now().Add(-2 * time.Minute),
					ExpiresAt:       time.Now().Add(5 * time.Minute),
				},
			},
		}
		h, _, _ := round11NewHandler(t, cfgDevice, state)
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type="+oauthDeviceCodeGrantType+"&device_code="+deviceCode+"&client_id=client-1"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(ctx))
		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "access_denied", body["error"])
	})

	t.Run("device_code invalid_grant when consumed", func(t *testing.T) {
		cfgDevice := round11TestConfig()
		cfgDevice.AllowDeviceFlow = true
		deviceCode := "consumed"
		deviceHash := oauthDeviceCodeHash(deviceCode)

		state := &round10QueryState{
			oauthDeviceSessionsByHash: map[string]storagemodels.OAuthDeviceSession{
				deviceHash: {
					DeviceCodeHash:  deviceHash,
					UserCode:        "ABCD-EFGH",
					ClientID:        "client-1",
					Scopes:          []string{auth.ScopeRead, auth.ScopeWrite},
					Status:          "consumed",
					IntervalSeconds: oauthDevicePollIntervalSeconds,
					CreatedAt:       time.Now().Add(-2 * time.Minute),
					UpdatedAt:       time.Now().Add(-2 * time.Minute),
					ExpiresAt:       time.Now().Add(5 * time.Minute),
				},
			},
		}
		h, _, _ := round11NewHandler(t, cfgDevice, state)
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type="+oauthDeviceCodeGrantType+"&device_code="+deviceCode+"&client_id=client-1"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(ctx))
		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_grant", body["error"])
	})

	t.Run("device_code approved issues tokens", func(t *testing.T) {
		cfgDevice := round11TestConfig()
		cfgDevice.AllowDeviceFlow = true
		deviceCode := "approved"
		deviceHash := oauthDeviceCodeHash(deviceCode)

		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{
				"client-1": {
					ClientID:     "client-1",
					Name:         "CLI App",
					RedirectURIs: []string{"https://example.com/callback"},
					ClientClass:  auth.ClientClassCLI,
					CreatedAt:    time.Now().Add(-24 * time.Hour),
				},
			},
			oauthDeviceSessionsByHash: map[string]storagemodels.OAuthDeviceSession{
				deviceHash: {
					DeviceCodeHash:   deviceHash,
					UserCode:         "ABCD-EFGH",
					ClientID:         "client-1",
					Scopes:           []string{auth.ScopeRead, auth.ScopeWrite},
					Status:           "approved",
					IntervalSeconds:  oauthDevicePollIntervalSeconds,
					ApprovedUsername: "alice",
					CreatedAt:        time.Now().Add(-2 * time.Minute),
					UpdatedAt:        time.Now().Add(-2 * time.Minute),
					ExpiresAt:        time.Now().Add(5 * time.Minute),
				},
			},
		}
		h, _, _ := round11NewHandler(t, cfgDevice, state)
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type="+oauthDeviceCodeGrantType+"&device_code="+deviceCode+"&client_id=client-1"))
		resp := requireStatus(t, http.StatusOK)(h.HandleOAuthTokenLift(ctx))
		var body apimodels.OAuthTokenResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.NotEmpty(t, body.AccessToken)
		require.NotEmpty(t, body.RefreshToken)
		require.Equal(t, "read write", body.Scope)

		claims1 := round12DecodeJWTClaims(t, body.AccessToken)
		require.Equal(t, auth.ClientClassCLI, claims1.ClientClass)
		require.NotEmpty(t, claims1.SessionID)

		// The test harness does not persist Create() mutations, so seed the refresh token lookup explicitly.
		if state.refreshTokensByToken == nil {
			state.refreshTokensByToken = map[string]storagemodels.RefreshToken{}
		}
		state.refreshTokensByToken[body.RefreshToken] = storagemodels.RefreshToken{
			Token:       body.RefreshToken,
			ClientID:    "client-1",
			Username:    "alice",
			ExpiresAt:   time.Now().Add(1 * time.Hour),
			Scopes:      []string{auth.ScopeRead, auth.ScopeWrite},
			CreatedAt:   time.Now().Add(-1 * time.Minute),
			ClientClass: auth.ClientClassCLI,
			SessionID:   claims1.SessionID,
		}

		ctxRefresh := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type=refresh_token&refresh_token="+url.QueryEscape(body.RefreshToken)+"&client_id=client-1"))
		respRefresh := requireStatus(t, http.StatusOK)(h.HandleOAuthTokenLift(ctxRefresh))
		var refreshBody apimodels.OAuthTokenResponse
		require.NoError(t, json.Unmarshal(respRefresh.Body, &refreshBody))
		require.NotEmpty(t, refreshBody.AccessToken)
		require.NotEmpty(t, refreshBody.RefreshToken)
		require.Equal(t, "read write", refreshBody.Scope)

		refreshClaims := round12DecodeJWTClaims(t, refreshBody.AccessToken)
		require.Equal(t, auth.ClientClassCLI, refreshClaims.ClientClass)
		require.Equal(t, claims1.SessionID, refreshClaims.SessionID)
	})

	t.Run("device_code approved agent client issues agent runtime-style session", func(t *testing.T) {
		cfgDevice := round11TestConfig()
		cfgDevice.AllowDeviceFlow = true
		cfgDevice.AgentAccessTokenDuration = 12 * time.Hour
		deviceCode := "agent-approved"
		deviceHash := oauthDeviceCodeHash(deviceCode)

		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{
				"client-agent": {
					ClientID:      "client-agent",
					Name:          "Agent Device Connector",
					RedirectURIs:  []string{"https://example.com/callback"},
					GrantTypes:    []string{oauthDeviceCodeGrantType, auth.GrantTypeRefreshToken},
					ClientClass:   auth.ClientClassAgent,
					AgentUsername: "agent1",
					CreatedAt:     time.Now().Add(-24 * time.Hour),
				},
			},
			oauthDeviceSessionsByHash: map[string]storagemodels.OAuthDeviceSession{
				deviceHash: {
					DeviceCodeHash:   deviceHash,
					UserCode:         "WXYZ-1234",
					ClientID:         "client-agent",
					Scopes:           []string{auth.ScopeRead, auth.ScopeFollow},
					Status:           oauthDeviceSessionStatusApproved,
					IntervalSeconds:  oauthDevicePollIntervalSeconds,
					ApprovedUsername: "owner",
					CreatedAt:        time.Now().Add(-2 * time.Minute),
					UpdatedAt:        time.Now().Add(-2 * time.Minute),
					ExpiresAt:        time.Now().Add(5 * time.Minute),
				},
			},
			usersByUsername: map[string]storagemodels.User{
				"agent1": {
					Username:   "agent1",
					IsAgent:    true,
					AgentType:  "assistant",
					AgentOwner: "@owner",
				},
			},
		}

		h, _, _ := round11NewHandler(t, cfgDevice, state)
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type="+oauthDeviceCodeGrantType+"&device_code="+deviceCode+"&client_id=client-agent"))
		resp := requireStatus(t, http.StatusOK)(h.HandleOAuthTokenLift(ctx))

		var body apimodels.OAuthTokenResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.NotEmpty(t, body.AccessToken)
		require.NotEmpty(t, body.RefreshToken)
		require.Equal(t, "read follow", body.Scope)
		require.Equal(t, int((12 * time.Hour).Seconds()), body.ExpiresIn)

		claims := round12DecodeJWTClaims(t, body.AccessToken)
		require.Equal(t, "agent1", claims.Username)
		require.True(t, claims.IsAgent)
		require.Equal(t, auth.ClientClassAgent, claims.ClientClass)
		require.Equal(t, "assistant", claims.AgentType)
		require.Equal(t, "@owner", claims.DelegatedBy)
		require.NotEmpty(t, claims.SessionID)
		require.Equal(t, claims.SessionID, claims.AgentSessionID)

		storedRefresh, ok := state.refreshTokensByToken[body.RefreshToken]
		require.True(t, ok)
		require.Equal(t, auth.ClientClassAgent, storedRefresh.ClientClass)
		require.Equal(t, "agent1", storedRefresh.Username)
		require.Equal(t, []string{auth.ScopeRead, auth.ScopeFollow}, storedRefresh.Scopes)
		require.NotEmpty(t, storedRefresh.SessionID)
		require.NotEmpty(t, storedRefresh.FamilyID)
		require.True(t, storedRefresh.Current)
		require.Equal(t, 1, storedRefresh.Generation)
		require.Equal(t, "Agent Device Connector", storedRefresh.DeviceLabel)
		require.Equal(t, int((12 * time.Hour).Seconds()), storedRefresh.AccessTTLSeconds)
	})

	t.Run("device_code approved agent client rejects non-owner approval", func(t *testing.T) {
		cfgDevice := round11TestConfig()
		cfgDevice.AllowDeviceFlow = true
		deviceCode := "agent-forbidden"
		deviceHash := oauthDeviceCodeHash(deviceCode)

		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{
				"client-agent": {
					ClientID:      "client-agent",
					Name:          "Agent Device Connector",
					RedirectURIs:  []string{"https://example.com/callback"},
					GrantTypes:    []string{oauthDeviceCodeGrantType, auth.GrantTypeRefreshToken},
					ClientClass:   auth.ClientClassAgent,
					AgentUsername: "agent1",
					CreatedAt:     time.Now().Add(-24 * time.Hour),
				},
			},
			oauthDeviceSessionsByHash: map[string]storagemodels.OAuthDeviceSession{
				deviceHash: {
					DeviceCodeHash:   deviceHash,
					UserCode:         "ZXCV-9876",
					ClientID:         "client-agent",
					Scopes:           []string{auth.ScopeRead},
					Status:           oauthDeviceSessionStatusApproved,
					IntervalSeconds:  oauthDevicePollIntervalSeconds,
					ApprovedUsername: "intruder",
					CreatedAt:        time.Now().Add(-2 * time.Minute),
					UpdatedAt:        time.Now().Add(-2 * time.Minute),
					ExpiresAt:        time.Now().Add(5 * time.Minute),
				},
			},
			usersByUsername: map[string]storagemodels.User{
				"agent1": {
					Username:   "agent1",
					IsAgent:    true,
					AgentOwner: "@owner",
				},
			},
		}

		h, _, _ := round11NewHandler(t, cfgDevice, state)
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", nil, nil, []byte("grant_type="+oauthDeviceCodeGrantType+"&device_code="+deviceCode+"&client_id=client-agent"))
		resp := requireStatus(t, http.StatusBadRequest)(h.HandleOAuthTokenLift(ctx))

		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "invalid_grant", body["error"])
		require.Empty(t, state.refreshTokensByToken)
	})
}
