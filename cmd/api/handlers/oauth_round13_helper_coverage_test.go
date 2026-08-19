package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestOAuthAuthorizationCodeExchangeErrorResponsePinsExistingMapperArms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		err             error
		wantStatus      int
		wantCode        string
		wantDescription string
	}{
		{
			name:            "temporarily unavailable",
			err:             auth.ErrOAuthTemporarilyUnavailable,
			wantStatus:      http.StatusServiceUnavailable,
			wantCode:        "temporarily_unavailable",
			wantDescription: "Authorization code exchange is temporarily unavailable",
		},
		{
			name:            "invalid target",
			err:             errOAuthInvalidTarget,
			wantStatus:      http.StatusBadRequest,
			wantCode:        "invalid_target",
			wantDescription: "resource must match the original authorization request",
		},
		{
			name:            "invalid grant",
			err:             auth.ErrInvalidGrant,
			wantStatus:      http.StatusBadRequest,
			wantCode:        "invalid_grant",
			wantDescription: "Invalid authorization code or expired",
		},
		{
			name:            "invalid client",
			err:             auth.ErrInvalidClient,
			wantStatus:      http.StatusBadRequest,
			wantCode:        "invalid_client",
			wantDescription: "Invalid client credentials",
		},
		{
			name:            "unauthorized client",
			err:             auth.ErrUnauthorizedClient,
			wantStatus:      http.StatusBadRequest,
			wantCode:        "unauthorized_client",
			wantDescription: "This client is not allowed to use authorization_code",
		},
		{
			name:            "invalid code challenge",
			err:             auth.ErrInvalidCodeChallenge,
			wantStatus:      http.StatusBadRequest,
			wantCode:        "invalid_grant",
			wantDescription: "PKCE verification failed",
		},
		{
			name:            "unexpected failure",
			err:             errors.New("unexpected exchange failure"),
			wantStatus:      http.StatusInternalServerError,
			wantCode:        "server_error",
			wantDescription: "Authorization code exchange failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resp := requireStatus(t, tt.wantStatus)(oauthAuthorizationCodeExchangeErrorResponse(tt.err))
			var body map[string]string
			require.NoError(t, json.Unmarshal(resp.Body, &body))
			require.Equal(t, tt.wantCode, body["error"])
			require.Equal(t, tt.wantDescription, body["error_description"])
		})
	}
}

func TestValidateAuthorizationCodeExchangeRedirect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                      string
		client                    *storage.OAuthClient
		clientID                  string
		redirectURI               string
		wantErr                   error
		wantValidationDescription string
	}{
		{
			name:                      "missing client id",
			client:                    &storage.OAuthClient{RedirectURIs: []string{"https://example.com/callback"}},
			redirectURI:               "https://example.com/callback",
			wantErr:                   auth.ErrInvalidRequest,
			wantValidationDescription: "validation failed for parameters: missing required parameters: client_id",
		},
		{
			name:        "exact match",
			client:      &storage.OAuthClient{RedirectURIs: []string{"https://example.com/callback"}},
			clientID:    "client-1",
			redirectURI: "https://example.com/callback",
		},
		{
			name: "public cli loopback port mismatch",
			client: &storage.OAuthClient{
				RedirectURIs: []string{"http://127.0.0.1:37371/callback"},
				ClientClass:  auth.ClientClassCLI,
			},
			clientID:    "client-1",
			redirectURI: "http://127.0.0.1:3118/callback",
		},
		{
			name: "public web loopback port mismatch",
			client: &storage.OAuthClient{
				RedirectURIs: []string{"http://127.0.0.1:37371/callback"},
				ClientClass:  auth.ClientClassWeb,
			},
			clientID:    "client-1",
			redirectURI: "http://127.0.0.1:3118/callback",
			wantErr:     auth.ErrInvalidRequest,
		},
		{
			name:        "different path",
			client:      &storage.OAuthClient{RedirectURIs: []string{"https://example.com/callback"}},
			clientID:    "client-1",
			redirectURI: "https://example.com/other",
			wantErr:     auth.ErrInvalidRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateAuthorizationCodeExchangeRedirect(tt.client, tt.clientID, tt.redirectURI, "code")
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				if tt.wantValidationDescription != "" {
					var requestValidationErr *oauthAuthorizationCodeRequestValidationError
					require.ErrorAs(t, err, &requestValidationErr)
					require.Equal(t, tt.wantValidationDescription, requestValidationErr.Error())
					var validationErr common.ValidationError
					require.ErrorAs(t, err, &validationErr)
				}
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestLoadAndValidateAuthorizationCodeRequiresStoredPKCE(t *testing.T) {
	cfg := round11TestConfig()
	state := &round10QueryState{
		authorizationCodesByCode: map[string]storagemodels.AuthorizationCode{
			"legacy-code": {
				Code:        "legacy-code",
				ClientID:    "client-public",
				RedirectURI: "https://example.com/callback",
				Username:    "alice",
				ExpiresAt:   time.Now().Add(5 * time.Minute),
				Scopes:      []string{auth.ScopeRead},
			},
		},
	}
	handler, _, _ := round11NewHandler(t, cfg, state)
	oauthSvc := auth.NewOAuthService(cfg.JWTSecret, cfg, handler.repos, nil)

	_, err := handler.loadAndValidateAuthorizationCodeForExchangeWithTelemetry(
		context.Background(),
		oauthSvc,
		"legacy-code",
		"client-public",
		"https://example.com/callback",
		"verifier",
		nil,
	)
	require.ErrorIs(t, err, auth.ErrInvalidGrant)
}

func TestAuthorizationCodeExchangeTokenContext(t *testing.T) {
	t.Parallel()

	clientClass, sessionID, accessTTL, err := authorizationCodeExchangeTokenContext(&config.Config{}, &storage.OAuthClient{
		ClientClass: auth.ClientClassCLI,
	}, &storage.AuthorizationCode{
		Username: "agent-1",
	})
	require.NoError(t, err)
	require.Equal(t, auth.ClientClassCLI, clientClass)
	require.NotEmpty(t, sessionID)
	require.Equal(t, auth.AccessTokenDuration, accessTTL)

	clientClass, sessionID, accessTTL, err = authorizationCodeExchangeTokenContext(&config.Config{}, &storage.OAuthClient{
		ClientClass: auth.ClientClassAgent,
	}, &storage.AuthorizationCode{
		Username: "agent-1",
		Resource: "https://example.com/mcp/agent-1",
	})
	require.NoError(t, err)
	require.Equal(t, auth.ClientClassAgent, clientClass)
	require.NotEmpty(t, sessionID)
	require.Equal(t, auth.AgentAccessTokenTTL(&config.Config{}), accessTTL)

	_, _, _, err = authorizationCodeExchangeTokenContext(&config.Config{}, &storage.OAuthClient{
		ClientClass: auth.ClientClassAgent,
	}, &storage.AuthorizationCode{
		Username: "agent-1",
	})
	require.ErrorIs(t, err, auth.ErrInvalidGrant)
}

func TestBuildAuthorizationCodeRefreshToken(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	base := buildAuthorizationCodeRefreshToken(now, "refresh-1", "client-1", &storage.OAuthClient{
		Name: "web-app",
	}, &storage.AuthorizationCode{
		Username: "alice",
		Scopes:   []string{"read"},
	}, "web", "", auth.AccessTokenDuration)
	require.Equal(t, "refresh-1", base.Token)
	require.Equal(t, "alice", base.Username)
	require.Equal(t, "client-1", base.ClientID)
	require.Equal(t, "web", base.ClientClass)
	require.NotEmpty(t, base.FamilyID)
	require.True(t, base.Current)
	require.Equal(t, 1, base.Generation)

	agentToken := buildAuthorizationCodeRefreshToken(now, "refresh-2", "client-agent", &storage.OAuthClient{
		Name: "agent-app",
	}, &storage.AuthorizationCode{
		Username: "agent-1",
		Scopes:   []string{"read", "write"},
	}, auth.ClientClassAgent, "sess-1", 45*time.Minute)
	require.Equal(t, auth.ClientClassAgent, agentToken.ClientClass)
	require.Equal(t, "sess-1", agentToken.SessionID)
	require.NotEmpty(t, agentToken.FamilyID)
	require.True(t, agentToken.Current)
	require.Equal(t, 1, agentToken.Generation)
	require.Empty(t, agentToken.DeviceLabel)
	require.Zero(t, agentToken.AccessTTLSeconds)
	require.True(t, agentToken.SessionCreatedAt.IsZero())
	require.True(t, agentToken.AbsoluteExpiresAt.IsZero())
	require.True(t, agentToken.IdleExpiresAt.IsZero())

	runtimeToken := buildAuthorizationCodeRefreshToken(now, "refresh-3", delegatedAgentClientID, &storage.OAuthClient{
		Name: "runtime-app",
	}, &storage.AuthorizationCode{
		Username: "agent-1",
		Scopes:   []string{"read"},
	}, auth.ClientClassAgent, "sess-runtime", 45*time.Minute)
	require.True(t, runtimeToken.Current)
	require.Equal(t, 1, runtimeToken.Generation)
	require.NotEmpty(t, runtimeToken.FamilyID)
	require.Equal(t, "runtime-app", runtimeToken.DeviceLabel)
	require.Equal(t, int((45 * time.Minute).Seconds()), runtimeToken.AccessTTLSeconds)
	require.WithinDuration(t, now, runtimeToken.SessionCreatedAt, time.Second)
	require.WithinDuration(t, now.Add(45*time.Minute), runtimeToken.AbsoluteExpiresAt, time.Second)
	require.WithinDuration(t, now.Add(45*time.Minute), runtimeToken.IdleExpiresAt, time.Second)
	require.WithinDuration(t, runtimeToken.AbsoluteExpiresAt, runtimeToken.ExpiresAt, time.Second)
}

func TestOAuthDeviceApprovedTokenContext(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"agent-1": {
				Username:   "agent-1",
				IsAgent:    true,
				AgentOwner: "@owner",
			},
		},
	})

	username, clientClass, sessionID, accessTTL, err := handler.oauthDeviceApprovedTokenContext(context.Background(), &storage.OAuthClient{
		ClientClass: auth.ClientClassCLI,
	}, "owner")
	require.NoError(t, err)
	require.Equal(t, "owner", username)
	require.Equal(t, auth.ClientClassCLI, clientClass)
	require.NotEmpty(t, sessionID)
	require.Equal(t, auth.AccessTokenDuration, accessTTL)

	_, _, _, _, err = handler.oauthDeviceApprovedTokenContext(context.Background(), &storage.OAuthClient{
		ClientClass:   auth.ClientClassAgent,
		AgentUsername: "agent-1",
	}, "owner")
	require.ErrorIs(t, err, auth.ErrInvalidGrant)

	_, _, _, _, err = handler.oauthDeviceApprovedTokenContext(context.Background(), &storage.OAuthClient{
		ClientClass:   auth.ClientClassAgent,
		AgentUsername: "agent-1",
	}, "intruder")
	require.ErrorIs(t, err, auth.ErrInvalidGrant)
}

func TestValidateAuthorizationCodeExchangeClientSecret(t *testing.T) {
	t.Parallel()

	handler, _, repos := round11NewHandlerSliceC(t, nil)
	ctx := context.Background()

	confidentialClient := &storage.OAuthClient{
		Name:         "Confidential App",
		RedirectURIs: []string{"https://example.com/callback"},
		Confidential: true,
	}
	require.NoError(t, repos.account.CreateOAuthClient(ctx, confidentialClient))

	oauthSvc := auth.NewOAuthService(handler.cfg.JWTSecret, handler.cfg, repos, nil)

	require.ErrorIs(t, validateAuthorizationCodeExchangeClientSecret(ctx, oauthSvc, confidentialClient, ""), auth.ErrInvalidClient)
	require.NoError(t, validateAuthorizationCodeExchangeClientSecret(ctx, oauthSvc, confidentialClient, confidentialClient.ClientSecret))
	require.ErrorIs(t, validateAuthorizationCodeExchangeClientSecret(ctx, oauthSvc, confidentialClient, "wrong-secret"), auth.ErrInvalidClient)

	publicClient := &storage.OAuthClient{
		Name:         "Public App",
		RedirectURIs: []string{"https://example.com/public"},
		Confidential: false,
	}
	require.NoError(t, repos.account.CreateOAuthClient(ctx, publicClient))
	require.NoError(t, validateAuthorizationCodeExchangeClientSecret(ctx, oauthSvc, publicClient, ""))
}

func TestValidateAuthorizationCodeExchangeClient(t *testing.T) {
	t.Parallel()

	handler, _, repos := round11NewHandlerSliceC(t, nil)
	ctx := context.Background()
	oauthSvc := auth.NewOAuthService(handler.cfg.JWTSecret, handler.cfg, repos, nil)

	client := &storage.OAuthClient{
		Name:         "Confidential App",
		RedirectURIs: []string{"https://example.com/callback"},
		Confidential: true,
	}
	require.NoError(t, repos.account.CreateOAuthClient(ctx, client))

	_, err := handler.validateAuthorizationCodeExchangeClient(ctx, oauthSvc, client.ClientID, "https://example.com/other", client.ClientSecret, "code")
	require.ErrorIs(t, err, auth.ErrInvalidRequest)

	got, err := handler.validateAuthorizationCodeExchangeClient(ctx, oauthSvc, client.ClientID, "https://example.com/callback", client.ClientSecret, "code")
	require.NoError(t, err)
	require.Equal(t, client.ClientID, got.ClientID)
}

func TestLoadAndValidateAuthorizationCodeForExchange(t *testing.T) {
	t.Parallel()

	handler, _, repos := round11NewHandlerSliceC(t, nil)
	ctx := context.Background()

	oauthSvc := auth.NewOAuthService(handler.cfg.JWTSecret, handler.cfg, repos, nil)

	code := &storage.AuthorizationCode{
		Code:          "code-1",
		ClientID:      "client-1",
		Username:      "alice",
		RedirectURI:   "https://example.com/callback",
		CodeChallenge: "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM",
		ExpiresAt:     time.Now().Add(5 * time.Minute),
	}
	require.NoError(t, repos.account.CreateAuthorizationCode(ctx, code))

	got, err := handler.loadAndValidateAuthorizationCodeForExchangeWithTelemetry(ctx, oauthSvc, code.Code, code.ClientID, code.RedirectURI, "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk", nil)
	require.NoError(t, err)
	require.Equal(t, code.Code, got.Code)

	_, err = handler.loadAndValidateAuthorizationCodeForExchangeWithTelemetry(ctx, oauthSvc, code.Code, "other-client", code.RedirectURI, "", nil)
	require.ErrorIs(t, err, auth.ErrInvalidGrant)

	_, err = handler.loadAndValidateAuthorizationCodeForExchangeWithTelemetry(ctx, oauthSvc, code.Code, code.ClientID, "https://example.com/other", "", nil)
	require.ErrorIs(t, err, auth.ErrInvalidGrant)

	_, err = handler.loadAndValidateAuthorizationCodeForExchangeWithTelemetry(ctx, oauthSvc, code.Code, code.ClientID, code.RedirectURI, "wrong-verifier", nil)
	require.Error(t, err)
}
