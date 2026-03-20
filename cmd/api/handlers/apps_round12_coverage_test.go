package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	stdErrors "errors"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestApps_Round12_ParseAppRegistrationRequest_Coverage(t *testing.T) {
	handler, _, _ := round11NewHandlerSliceC(t, nil)

	t.Run("json_ok", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/apps", map[string]string{"Content-Type": "application/json"}, nil, apimodels.AppRegistrationRequest{
			ClientName:   "Test App",
			RedirectURIs: "https://example.com/callback",
			Scopes:       "read",
			Website:      "https://example.com",
		})
		require.NoError(t, err)

		req, err := handler.parseAppRegistrationRequest(ctx)
		require.NoError(t, err)
		require.Equal(t, "Test App", req.ClientName)
	})

	t.Run("json_parse_error", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/apps", map[string]string{"Content-Type": "application/json"}, nil, []byte(`{invalid}`))

		_, err := handler.parseAppRegistrationRequest(ctx)
		require.Error(t, err)
	})

	t.Run("form_urlencoded_ok", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/apps", map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, nil,
			[]byte("client_name=Test+App&redirect_uris=https%3A%2F%2Fexample.com%2Fcallback&scopes=read&website=https%3A%2F%2Fexample.com"))

		req, err := handler.parseAppRegistrationRequest(ctx)
		require.NoError(t, err)
		require.Equal(t, "Test App", req.ClientName)
	})

	t.Run("form_urlencoded_parse_error", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/apps", map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, nil, []byte("%"))

		_, err := handler.parseAppRegistrationRequest(ctx)
		require.Error(t, err)
	})

	t.Run("multipart_parse_error", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/apps",
			map[string]string{"Content-Type": "multipart/form-data; boundary=----WebKitFormBoundary7MA4YWxkTrZu0gW"},
			nil,
			[]byte("not-a-multipart-body"),
		)

		_, err := handler.parseAppRegistrationRequest(ctx)
		require.Error(t, err)
	})
}

func TestApps_Round12_FallbackAndValidationHelpers_Coverage(t *testing.T) {
	handler, _, _ := round11NewHandlerSliceC(t, nil)

	t.Run("parse_fallback_prefers_form", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/apps", map[string]string{"Content-Type": "text/plain"}, nil,
			[]byte("client_name=Test+App&redirect_uris=https%3A%2F%2Fexample.com%2Fcallback&scopes=read"))

		req, err := handler.parseFallbackRequest(ctx, string(ctx.Request.Body))
		require.NoError(t, err)
		require.Equal(t, "Test App", req.ClientName)
	})

	t.Run("parse_fallback_uses_json_last_resort", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/apps", map[string]string{"Content-Type": "application/json"}, nil,
			[]byte(`{"client_name":"Test App","redirect_uris":"https://example.com/callback","scopes":"%ZZ"}`))

		req, err := handler.parseFallbackRequest(ctx, string(ctx.Request.Body))
		require.NoError(t, err)
		require.Equal(t, "Test App", req.ClientName)
	})

	t.Run("parse_fallback_failure", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/apps", map[string]string{"Content-Type": "text/plain"}, nil, []byte(`{"client_name":%ZZ}`))

		_, err := handler.parseFallbackRequest(ctx, string(ctx.Request.Body))
		require.Error(t, err)
	})

	t.Run("build_request_from_params_validation_errors", func(t *testing.T) {
		_, err := handler.buildRequestFromParams(map[string]string{"client_name": "", "redirect_uris": "https://example.com/callback"})
		require.Error(t, err)

		_, err = handler.buildRequestFromParams(map[string]string{"client_name": "Test App", "redirect_uris": ""})
		require.Error(t, err)
	})

	t.Run("parse_and_validate_redirect_uris_errors", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/apps", nil, nil, nil)
		require.NoError(t, err)

		_, err = handler.parseAndValidateRedirectURIs(ctx, "")
		require.Error(t, err)

		ctx2, err := round10NewLiftContext(http.MethodPost, "/api/v1/apps", nil, nil, nil)
		require.NoError(t, err)

		_, err = handler.parseAndValidateRedirectURIs(ctx2, "example.com/callback")
		require.Error(t, err)
	})

	t.Run("parse_and_validate_redirect_uris_ok", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/apps", nil, nil, nil)
		require.NoError(t, err)

		uris, err := handler.parseAndValidateRedirectURIs(ctx, "urn:ietf:wg:oauth:2.0:oob myapp://callback https://example.com/callback")
		require.NoError(t, err)
		require.Len(t, uris, 3)
	})

	t.Run("validate_required_app_params_sets_422", func(t *testing.T) {
		ctxMissingName, err := round10NewLiftContext(http.MethodPost, "/api/v1/apps", nil, nil, nil)
		require.NoError(t, err)
		require.Error(t, handler.validateRequiredAppParams(ctxMissingName, &apimodels.AppRegistrationRequest{RedirectURIs: "https://example.com/callback"}))

		ctxMissingRedirect, err := round10NewLiftContext(http.MethodPost, "/api/v1/apps", nil, nil, nil)
		require.NoError(t, err)
		require.Error(t, handler.validateRequiredAppParams(ctxMissingRedirect, &apimodels.AppRegistrationRequest{ClientName: "Test App"}))
	})

	t.Run("validate_single_redirect_uri_variants", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/apps", nil, nil, nil)
		require.NoError(t, err)

		require.NoError(t, handler.validateSingleRedirectURI(ctx, ""))
		require.NoError(t, handler.validateSingleRedirectURI(ctx, "urn:ietf:wg:oauth:2.0:oob"))
		require.NoError(t, handler.validateSingleRedirectURI(ctx, "myapp://callback"))

		ctxBad, err := round10NewLiftContext(http.MethodPost, "/api/v1/apps", nil, nil, nil)
		require.NoError(t, err)
		require.Error(t, handler.validateSingleRedirectURI(ctxBad, "example.com/callback"))
	})

	t.Run("parse_scopes_default_and_explicit", func(t *testing.T) {
		require.Equal(t, auth.DefaultScopes(), handler.parseScopes(""))
		require.Equal(t, []string{"read", "write:accounts"}, handler.parseScopes("read write:accounts"))
		require.Equal(t, []string{"read", "write:accounts"}, handler.parseScopes("read,write:accounts"))
	})
}

func TestApps_Round12_CreateOAuthClientAndVapidHelpers_Coverage(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("create_oauth_client_ok_includes_vapid_key", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/apps", nil, nil, nil)
		require.NoError(t, err)

		req := &apimodels.AppRegistrationRequest{
			ClientName:   "Test App",
			RedirectURIs: "https://example.com/callback",
			Scopes:       "",
			Website:      "https://example.com",
		}
		requireStatus(t, http.StatusOK)(handler.createOAuthClientAndRespond(ctx, req, []string{"https://example.com/callback"}))
	})

	t.Run("agent_client_requires_authenticated_owner", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/apps", nil, nil, nil)
		require.NoError(t, err)

		req := &apimodels.AppRegistrationRequest{
			ClientName:    "Agent Connector",
			RedirectURIs:  "https://example.com/callback",
			Scopes:        "read write",
			ClientClass:   auth.ClientClassAgent,
			AgentUsername: "agent1",
		}
		requireStatus(t, http.StatusUnauthorized)(handler.createOAuthClientAndRespond(ctx, req, []string{"https://example.com/callback"}))
	})

	t.Run("agent_client_rejects_unowned_agent", func(t *testing.T) {
		state := &round10QueryState{
			usersByUsername: map[string]storagemodels.User{
				"agent1": {Username: "agent1", IsAgent: true, AgentOwner: "@someone-else"},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ownerToken := round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{auth.ScopeRead})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/apps", map[string]string{
			"Authorization": "Bearer " + ownerToken,
		}, nil, nil)
		require.NoError(t, err)

		req := &apimodels.AppRegistrationRequest{
			ClientName:    "Agent Connector",
			RedirectURIs:  "https://example.com/callback",
			Scopes:        "read write",
			ClientClass:   auth.ClientClassAgent,
			AgentUsername: "agent1",
		}
		requireStatus(t, http.StatusForbidden)(handler.createOAuthClientAndRespond(ctx, req, []string{"https://example.com/callback"}))
	})

	t.Run("agent_client_stores_bound_agent_username", func(t *testing.T) {
		state := &round10QueryState{
			usersByUsername: map[string]storagemodels.User{
				"agent1": {Username: "agent1", IsAgent: true, AgentOwner: "@owner"},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ownerToken := round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{auth.ScopeRead})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/apps", map[string]string{
			"Authorization": "Bearer " + ownerToken,
		}, nil, nil)
		require.NoError(t, err)

		req := &apimodels.AppRegistrationRequest{
			ClientName:              "Agent Connector",
			RedirectURIs:            "https://example.com/callback",
			Scopes:                  "read write",
			ClientClass:             auth.ClientClassAgent,
			AgentUsername:           "agent1",
			GrantTypes:              auth.GrantTypeClientCredentials + " " + auth.GrantTypeAuthorizationCode,
			TokenEndpointAuthMethod: "client_secret_post",
		}
		requireStatus(t, http.StatusOK)(handler.createOAuthClientAndRespond(ctx, req, []string{"https://example.com/callback"}))

		require.Len(t, state.oauthClientsByID, 1)
		for _, client := range state.oauthClientsByID {
			require.Equal(t, auth.ClientClassAgent, client.ClientClass)
			require.Equal(t, "agent1", client.AgentUsername)
			require.Equal(t, []string{auth.GrantTypeClientCredentials, auth.GrantTypeAuthorizationCode}, client.GrantTypes)
			require.True(t, client.Confidential)
		}
	})

	t.Run("client_credentials_requires_confidential_auth_method", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/apps", nil, nil, nil)
		require.NoError(t, err)

		req := &apimodels.AppRegistrationRequest{
			ClientName:              "Public Agent Connector",
			RedirectURIs:            "https://example.com/callback",
			Scopes:                  "read write",
			ClientClass:             auth.ClientClassAgent,
			AgentUsername:           "agent1",
			GrantTypes:              auth.GrantTypeClientCredentials,
			TokenEndpointAuthMethod: "none",
		}
		requireStatus(t, http.StatusUnprocessableEntity)(handler.createOAuthClientAndRespond(ctx, req, []string{"https://example.com/callback"}))
	})

	t.Run("registration_rejects_internal_admin_scope", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/apps", nil, nil, nil)
		require.NoError(t, err)

		req := &apimodels.AppRegistrationRequest{
			ClientName:   "Admin App",
			RedirectURIs: "https://example.com/callback",
			Scopes:       "admin",
		}
		requireStatus(t, http.StatusUnprocessableEntity)(handler.createOAuthClientAndRespond(ctx, req, []string{"https://example.com/callback"}))
	})

	t.Run("registration_preserves_compatibility_aliases", func(t *testing.T) {
		state := &round10QueryState{}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/apps", nil, nil, nil)
		require.NoError(t, err)

		req := &apimodels.AppRegistrationRequest{
			ClientName:   "Compat App",
			RedirectURIs: "https://example.com/callback",
			Scopes:       "write:follows",
		}
		requireStatus(t, http.StatusOK)(handler.createOAuthClientAndRespond(ctx, req, []string{"https://example.com/callback"}))

		require.Len(t, state.oauthClientsByID, 1)
		for _, client := range state.oauthClientsByID {
			require.Equal(t, []string{"write:follows"}, client.Scopes)
		}
	})

	t.Run("create_oauth_client_repo_error", func(t *testing.T) {
		state := &round10QueryState{createErrorOnce: stdErrors.New("create failed")}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/apps", nil, nil, nil)
		require.NoError(t, err)

		req := &apimodels.AppRegistrationRequest{
			ClientName:   "Test App",
			RedirectURIs: "https://example.com/callback",
			Scopes:       "read",
		}
		requireStatus(t, http.StatusInternalServerError)(handler.createOAuthClientAndRespond(ctx, req, []string{"https://example.com/callback"}))
	})

	t.Run("get_vapid_key_error_returns_empty", func(t *testing.T) {
		state := &round10QueryState{forceVapidNotFound: true}
		handler, _, _ := round11NewHandler(t, cfg, state)

		require.Equal(t, "", handler.getVAPIDKey())
	})

	t.Run("truncate_string", func(t *testing.T) {
		require.Equal(t, "short", truncateStringLift("short", 10))
		require.Equal(t, "abc", truncateStringLift("abcdef", 3))
	})
}

func TestApps_Round12_HandleAppVerifyCredentialsLift_Coverage(t *testing.T) {
	cfg := round11TestConfig()

	state := &round10QueryState{
		oauthClientsByID: map[string]storagemodels.OAuthClient{
			"client-1": {ClientID: "client-1", ClientSecret: "secret", Name: "Test App", RedirectURIs: []string{"https://example.com/callback"}, Website: "https://example.com"},
		},
	}

	handler, _, _ := round11NewHandler(t, cfg, state)

	t.Run("missing_auth_header", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/apps/verify_credentials", nil, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusUnauthorized)(handler.HandleAppVerifyCredentialsLift(ctx))
	})

	t.Run("invalid_bearer_token_not_base64", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/apps/verify_credentials", map[string]string{"Authorization": "Bearer not_base64!!"}, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusUnauthorized)(handler.HandleAppVerifyCredentialsLift(ctx))
	})

	t.Run("base64_decodes_without_colon", func(t *testing.T) {
		token := base64.StdEncoding.EncodeToString([]byte("client-1"))
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/apps/verify_credentials", map[string]string{"Authorization": "Bearer " + token}, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusUnauthorized)(handler.HandleAppVerifyCredentialsLift(ctx))
	})

	t.Run("base64_secret_mismatch", func(t *testing.T) {
		token := base64.StdEncoding.EncodeToString([]byte("client-1:wrong"))
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/apps/verify_credentials", map[string]string{"Authorization": "Bearer " + token}, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusUnauthorized)(handler.HandleAppVerifyCredentialsLift(ctx))
	})

	t.Run("base64_client_not_found", func(t *testing.T) {
		state := &round10QueryState{
			notFoundPKs: map[string]bool{"OAUTH_CLIENT#missing": true},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		token := base64.StdEncoding.EncodeToString([]byte("missing:secret"))
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/apps/verify_credentials", map[string]string{"Authorization": "Bearer " + token}, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusUnauthorized)(handler.HandleAppVerifyCredentialsLift(ctx))
	})

	t.Run("access_token_ok", func(t *testing.T) {
		oauthToken := round11SignTokenWithClientID(t, cfg.JWTSecret, "alice", "client-1", []string{auth.ScopeRead}, "sess-1")
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/apps/verify_credentials", map[string]string{"authorization": "Bearer " + oauthToken}, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusOK)(handler.HandleAppVerifyCredentialsLift(ctx))
	})

	t.Run("access_token_client_lookup_fails", func(t *testing.T) {
		state := &round10QueryState{notFoundPKs: map[string]bool{"OAUTH_CLIENT#client-1": true}}
		handler, _, _ := round11NewHandler(t, cfg, state)

		oauthToken := round11SignTokenWithClientID(t, cfg.JWTSecret, "alice", "client-1", []string{auth.ScopeRead}, "sess-1")
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/apps/verify_credentials", map[string]string{"Authorization": "Bearer " + oauthToken}, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusUnauthorized)(handler.HandleAppVerifyCredentialsLift(ctx))
	})

	t.Run("vapid_keys_not_found_returns_empty", func(t *testing.T) {
		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{
				"client-1": {ClientID: "client-1", ClientSecret: "secret", RedirectURIs: []string{"https://example.com/callback"}},
			},
			forceVapidNotFound: true,
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		oauthToken := round11SignTokenWithClientID(t, cfg.JWTSecret, "alice", "client-1", []string{auth.ScopeRead}, "sess-1")
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/apps/verify_credentials", map[string]string{"Authorization": "Bearer " + oauthToken}, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusOK)(handler.HandleAppVerifyCredentialsLift(ctx))
	})
}

func TestApps_Round12_RotateSecret_Coverage(t *testing.T) {
	cfg := round11TestConfig()

	t.Run("owner can rotate client secret with default grace window", func(t *testing.T) {
		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{
				"client-1": {
					ClientID:      "client-1",
					ClientSecret:  "secret",
					Name:          "Agent Connector",
					RedirectURIs:  []string{"https://example.com/callback"},
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
					AgentOwner: "@owner",
				},
			},
		}
		handler, _, repos := round11NewHandlerSliceC(t, state)

		token := round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{auth.ScopeWrite})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/apps/client-1/rotate_secret", map[string]string{
			"Authorization": "Bearer " + token,
		}, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "client-1"

		resp := requireStatus(t, http.StatusOK)(handler.HandleAppRotateSecretLift(ctx))
		var body apimodels.AppSecretRotationResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "client-1", body.ClientID)
		require.NotEmpty(t, body.ClientSecret)
		require.NotEqual(t, "secret", body.ClientSecret)
		require.Equal(t, "client_secret_post", body.TokenEndpointAuthMethod)
		require.Equal(t, 86400, body.GracePeriodSeconds)
		require.False(t, body.ForcedInvalidation)
		require.NotEmpty(t, body.RotatedAt)
		require.NotEmpty(t, body.PreviousSecretValidUntil)

		oauthSvc := auth.NewOAuthService(handler.cfg.JWTSecret, handler.cfg, repos, nil)
		require.NoError(t, oauthSvc.ValidateClient(context.Background(), "client-1", "secret"))
		require.NoError(t, oauthSvc.ValidateClient(context.Background(), "client-1", body.ClientSecret))
	})

	t.Run("non-owner cannot rotate client secret", func(t *testing.T) {
		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{
				"client-1": {
					ClientID:     "client-1",
					ClientSecret: "secret",
					Name:         "Agent Connector",
					RedirectURIs: []string{"https://example.com/callback"},
					Scopes:       []string{auth.ScopeRead, auth.ScopeWrite},
					OwnerID:      "owner",
					Confidential: true,
					CreatedAt:    time.Now().Add(-24 * time.Hour),
				},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		token := round11SignAccessToken(t, cfg.JWTSecret, "other-user", []string{auth.ScopeWrite})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/apps/client-1/rotate_secret", map[string]string{
			"Authorization": "Bearer " + token,
		}, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "client-1"

		requireStatus(t, http.StatusForbidden)(handler.HandleAppRotateSecretLift(ctx))
	})

	t.Run("missing auth returns unauthorized", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/apps/client-1/rotate_secret", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "client-1"

		requireStatus(t, http.StatusUnauthorized)(handler.HandleAppRotateSecretLift(ctx))
	})

	t.Run("missing client id returns bad request", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})
		token := round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{auth.ScopeWrite})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/apps//rotate_secret", map[string]string{
			"Authorization": "Bearer " + token,
		}, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusBadRequest)(handler.HandleAppRotateSecretLift(ctx))
	})

	t.Run("missing client returns not found", func(t *testing.T) {
		handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
			notFoundPKs: map[string]bool{"OAUTH_CLIENT#missing": true},
		})
		token := round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{auth.ScopeWrite})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/apps/missing/rotate_secret", map[string]string{
			"Authorization": "Bearer " + token,
		}, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "missing"

		requireStatus(t, http.StatusNotFound)(handler.HandleAppRotateSecretLift(ctx))
	})

	t.Run("forced invalidation cuts off the old secret immediately", func(t *testing.T) {
		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{
				"client-1": {
					ClientID:     "client-1",
					ClientSecret: "secret",
					Name:         "Agent Connector",
					RedirectURIs: []string{"https://example.com/callback"},
					Scopes:       []string{auth.ScopeRead, auth.ScopeWrite},
					OwnerID:      "owner",
					Confidential: true,
					CreatedAt:    time.Now().Add(-24 * time.Hour),
				},
			},
		}
		handler, _, repos := round11NewHandlerSliceC(t, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/apps/client-1/rotate_secret", map[string]string{
			"Authorization": "Bearer " + round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{auth.ScopeWrite}),
			"Content-Type":  "application/json",
		}, nil, map[string]any{"force_invalidate": true})
		require.NoError(t, err)
		ctx.Params["id"] = "client-1"

		resp := requireStatus(t, http.StatusOK)(handler.HandleAppRotateSecretLift(ctx))
		var body apimodels.AppSecretRotationResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.True(t, body.ForcedInvalidation)
		require.Zero(t, body.GracePeriodSeconds)
		require.Empty(t, body.PreviousSecretValidUntil)

		oauthSvc := auth.NewOAuthService(handler.cfg.JWTSecret, handler.cfg, repos, nil)
		require.Equal(t, auth.ErrInvalidClient, oauthSvc.ValidateClient(context.Background(), "client-1", "secret"))
		require.NoError(t, oauthSvc.ValidateClient(context.Background(), "client-1", body.ClientSecret))
	})

	t.Run("agent ownership drift returns forbidden", func(t *testing.T) {
		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{
				"client-1": {
					ClientID:      "client-1",
					ClientSecret:  "secret",
					Name:          "Agent Connector",
					RedirectURIs:  []string{"https://example.com/callback"},
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
					AgentOwner: "@someone-else",
				},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		token := round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{auth.ScopeWrite})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/apps/client-1/rotate_secret", map[string]string{
			"Authorization": "Bearer " + token,
		}, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "client-1"

		requireStatus(t, http.StatusForbidden)(handler.HandleAppRotateSecretLift(ctx))
	})

	t.Run("update failure returns internal server error", func(t *testing.T) {
		state := &round10QueryState{
			executeErrorOnce: stdErrors.New("update failed"),
			oauthClientsByID: map[string]storagemodels.OAuthClient{
				"client-1": {
					ClientID:     "client-1",
					ClientSecret: "secret",
					Name:         "App",
					RedirectURIs: []string{"https://example.com/callback"},
					Scopes:       []string{auth.ScopeRead, auth.ScopeWrite},
					OwnerID:      "owner",
					Confidential: true,
					CreatedAt:    time.Now().Add(-24 * time.Hour),
				},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		token := round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{auth.ScopeWrite})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/apps/client-1/rotate_secret", map[string]string{
			"Authorization": "Bearer " + token,
		}, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "client-1"

		requireStatus(t, http.StatusInternalServerError)(handler.HandleAppRotateSecretLift(ctx))
	})

	t.Run("public client rotation reports none auth method", func(t *testing.T) {
		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{
				"client-1": {
					ClientID:     "client-1",
					ClientSecret: "secret",
					Name:         "Public App",
					RedirectURIs: []string{"https://example.com/callback"},
					Scopes:       []string{auth.ScopeRead},
					OwnerID:      "owner",
					Confidential: false,
					CreatedAt:    time.Now().Add(-24 * time.Hour),
				},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		token := round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{auth.ScopeWrite})
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/apps/client-1/rotate_secret", map[string]string{
			"Authorization": "Bearer " + token,
		}, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "client-1"

		resp := requireStatus(t, http.StatusOK)(handler.HandleAppRotateSecretLift(ctx))
		var body apimodels.AppSecretRotationResponse
		require.NoError(t, json.Unmarshal(resp.Body, &body))
		require.Equal(t, "none", body.TokenEndpointAuthMethod)
	})

	t.Run("invalid rotation request returns bad request", func(t *testing.T) {
		state := &round10QueryState{
			oauthClientsByID: map[string]storagemodels.OAuthClient{
				"client-1": {
					ClientID:     "client-1",
					ClientSecret: "secret",
					Name:         "Agent Connector",
					RedirectURIs: []string{"https://example.com/callback"},
					Scopes:       []string{auth.ScopeRead, auth.ScopeWrite},
					OwnerID:      "owner",
					Confidential: true,
					CreatedAt:    time.Now().Add(-24 * time.Hour),
				},
			},
		}
		handler, _, _ := round11NewHandler(t, cfg, state)

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/apps/client-1/rotate_secret", map[string]string{
			"Authorization": "Bearer " + round11SignAccessToken(t, cfg.JWTSecret, "owner", []string{auth.ScopeWrite}),
			"Content-Type":  "application/json",
		}, nil, map[string]any{
			"force_invalidate":     true,
			"grace_period_seconds": 60,
		})
		require.NoError(t, err)
		ctx.Params["id"] = "client-1"

		requireStatus(t, http.StatusBadRequest)(handler.HandleAppRotateSecretLift(ctx))
	})

	t.Run("generate oauth client secret helper returns value", func(t *testing.T) {
		secret, err := generateOAuthClientSecret()
		require.NoError(t, err)
		require.NotEmpty(t, secret)
		require.Len(t, secret, 44)
	})
}
