package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"mime/multipart"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	commonerrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/services/quotes"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestReleaseCoverageMargin_AppRegistrationRejectsInvalidRequests(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		headers map[string]string
		body    []byte
	}{
		{
			name:    "malformed JSON",
			headers: map[string]string{"Content-Type": "application/json"},
			body:    []byte(`{"client_name":`),
		},
		{
			name:    "missing required redirect URI",
			headers: map[string]string{"content-type": "application/json"},
			body:    []byte(`{"client_name":"release client"}`),
		},
		{
			name:    "unsupported form parameter",
			headers: map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
			body:    []byte("client_name=release+client&redirect_uris=https%3A%2F%2Fclient.example%2Fcb&surprise=true"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _, _ := round11NewHandlerSliceC(t, nil)
			ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/apps", tt.headers, nil, tt.body)
			resp := requireStatus(t, http.StatusUnprocessableEntity)(h.HandleAppRegistrationLift(ctx))
			require.NotEmpty(t, resp.Body)
		})
	}
}

func TestReleaseCoverageMargin_AppCredentialVerificationFailureModes(t *testing.T) {
	t.Parallel()

	encode := func(value string) string {
		return base64.StdEncoding.EncodeToString([]byte(value))
	}
	tests := []struct {
		name   string
		header string
	}{
		{name: "missing bearer header"},
		{name: "malformed base64", header: "Bearer not-base64"},
		{name: "missing secret separator", header: "Bearer " + encode("client-1")},
		{name: "wrong client secret", header: "Bearer " + encode("client-1:wrong")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &round10QueryState{oauthClientsByID: map[string]storagemodels.OAuthClient{
				"client-1": {
					ClientID:     "client-1",
					ClientSecret: "secret",
					RedirectURIs: []string{"https://client.example/cb"},
				},
			}}
			h, _, _ := round11NewHandlerSliceC(t, state)
			headers := map[string]string{}
			if tt.header != "" {
				headers["Authorization"] = tt.header
			}
			ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/apps/verify_credentials", headers, nil, nil)
			require.NoError(t, err)
			requireStatus(t, http.StatusUnauthorized)(h.HandleAppVerifyCredentialsLift(ctx))
		})
	}

	t.Run("valid credentials tolerate missing VAPID configuration", func(t *testing.T) {
		state := &round10QueryState{
			forceVapidNotFound: true,
			oauthClientsByID: map[string]storagemodels.OAuthClient{
				"client-1": {
					ClientID:     "client-1",
					ClientSecret: "secret",
					RedirectURIs: []string{"https://client.example/cb"},
				},
			},
		}
		h, _, _ := round11NewHandlerSliceC(t, state)
		headers := map[string]string{"authorization": "Bearer " + encode("client-1:secret")}
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/apps/verify_credentials", headers, nil, nil)
		require.NoError(t, err)
		resp := requireStatus(t, http.StatusOK)(h.HandleAppVerifyCredentialsLift(ctx))
		require.NotContains(t, string(resp.Body), `"vapid_key"`)
	})
}

func TestReleaseCoverageMargin_AppRegistrationPolicyBranches(t *testing.T) {
	t.Parallel()

	h, _, _ := round11NewHandler(t, round11TestConfig(), &round10QueryState{})

	t.Run("fallback content is parsed as a public form registration", func(t *testing.T) {
		body := []byte("client_name=release+client&redirect_uris=https%3A%2F%2Fclient.example%2Fcb&scopes=read")
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/apps", map[string]string{"Content-Type": "text/plain"}, nil, body)
		req, err := h.parseAppRegistrationRequest(ctx)
		require.NoError(t, err)
		require.Equal(t, "release client", req.ClientName)
	})

	t.Run("multipart registration preserves public fields", func(t *testing.T) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		require.NoError(t, writer.WriteField("client_name", "release multipart client"))
		require.NoError(t, writer.WriteField("redirect_uris", "https://client.example/cb"))
		require.NoError(t, writer.WriteField("scopes", "read"))
		require.NoError(t, writer.Close())

		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/apps", map[string]string{
			"Content-Type": writer.FormDataContentType(),
		}, nil, body.Bytes())
		req, err := h.parseAppRegistrationRequest(ctx)
		require.NoError(t, err)
		require.Equal(t, "release multipart client", req.ClientName)
	})

	t.Run("blank and unknown form keys follow the public allowlist", func(t *testing.T) {
		require.NoError(t, validatePublicAppRegistrationParams(map[string]string{" ": "ignored"}))
		require.ErrorContains(t, validatePublicAppRegistrationParams(map[string]string{"private_flag": "true"}), "unsupported")
	})

	t.Run("disabled CLI flow is denied before client persistence", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/apps", nil, nil, nil)
		require.NoError(t, err)
		resp := requireStatus(t, http.StatusForbidden)(h.createOAuthClientAndRespond(ctx, &apimodels.AppRegistrationRequest{
			ClientName:   "release CLI",
			RedirectURIs: "http://127.0.0.1/callback",
			Scopes:       "read",
			ClientClass:  "cli",
		}, []string{"http://127.0.0.1/callback"}))
		require.Contains(t, string(resp.Body), "cli automation is disabled")
	})

	t.Run("unknown token endpoint authentication is rejected", func(t *testing.T) {
		_, _, err := normalizeOAuthTokenEndpointAuthMethod("private_key_jwt", "web")
		require.ErrorContains(t, err, "invalid token_endpoint_auth_method")

		ctx, ctxErr := round10NewLiftContext(http.MethodPost, "/api/v1/apps", nil, nil, nil)
		require.NoError(t, ctxErr)
		requireStatus(t, http.StatusUnprocessableEntity)(h.createOAuthClientAndRespond(ctx, &apimodels.AppRegistrationRequest{
			ClientName:              "unsupported auth client",
			RedirectURIs:            "https://client.example/cb",
			Scopes:                  "read",
			TokenEndpointAuthMethod: "private_key_jwt",
		}, []string{"https://client.example/cb"}))
	})

	t.Run("validation logs optional policy fields and still enforces required inputs", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/apps", nil, nil, nil)
		require.NoError(t, err)
		_, err = h.validateAppRegistrationParams(ctx, &apimodels.AppRegistrationRequest{
			ClientName:              "release policy client",
			RedirectURIs:            "https://client.example/cb",
			ClientClass:             "web",
			GrantTypes:              "authorization_code",
			TokenEndpointAuthMethod: "none",
		})
		require.NoError(t, err)

		_, err = h.validateAppRegistrationParams(ctx, &apimodels.AppRegistrationRequest{ClientName: "missing redirect"})
		require.Error(t, err)
	})
}

func TestReleaseCoverageMargin_OAuthResourceValidation(t *testing.T) {
	t.Run("resource indicators reject ambiguous URLs", func(t *testing.T) {
		tests := []struct {
			name     string
			resource string
			contains string
		}{
			{name: "invalid escape", resource: "https://example.com/%", contains: "absolute https URI"},
			{name: "fragment", resource: "https://example.com/mcp/alice#fragment", contains: "fragment"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := normalizeOAuthResourceIndicator(tt.resource)
				require.ErrorContains(t, err, tt.contains)
			})
		}
	})

	h, _, _ := round11NewHandler(t, round11TestConfig(), &round10QueryState{})

	t.Run("flow binding rejects structurally incomplete requests", func(t *testing.T) {
		_, err := h.normalizeAuthorizeScopesForFlow(context.Background(), nil)
		require.Error(t, err)

		resp, err := h.bindAuthorizeTarget(nil, nil)
		require.NoError(t, err)
		require.Nil(t, resp)

		flow := &authorizeFlow{
			request: &authorizeRequest{},
			client: &storage.OAuthClient{
				RegistrationSource: oauthRegistrationSourceDynamic,
			},
		}
		resp, err = h.bindAuthorizeTarget(nil, flow)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)
	})

	t.Run("actor resources fail closed before account ownership checks", func(t *testing.T) {
		tests := []struct {
			name     string
			resource string
		}{
			{name: "invalid URL escape", resource: "https://example.com/%"},
			{name: "query string", resource: "https://example.com/mcp/alice?x=1"},
			{name: "wrong path", resource: "https://example.com/users/alice"},
			{name: "invalid actor name", resource: "https://example.com/mcp/bad%20name"},
			{name: "missing local actor", resource: "https://example.com/mcp/alice"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, _, err := h.resolveAuthorizeTargetActorFromResource(context.Background(), tt.resource, "owner")
				require.Error(t, err)
				var targetErr *oauthAuthorizeTargetError
				require.ErrorAs(t, err, &targetErr)
			})
		}
		require.False(t, oauthResourceTargetsInstancePlane("https://example.com/%"))
	})

	t.Run("instance resources preserve the account-holder boundary", func(t *testing.T) {
		tests := []string{
			"https://example.com/%",
			"https://example.com/instance/ptah/mcp?x=1",
			"https://example.com/instance/ptah",
		}
		for _, resource := range tests {
			_, _, err := h.resolveAuthorizeTargetInstanceFromResource(context.Background(), resource, "alice")
			require.Error(t, err)
		}

		_, err := (*Handler)(nil).canonicalOAuthInstanceResource(oauthInstanceSurfacePtah)
		require.Error(t, err)
		require.ErrorIs(t, h.validateOAuthInstanceResourceOwner(context.Background(), "", "alice", "bob"), errOAuthInvalidTarget)
		require.Empty(t, oauthAuthorizationCodePrincipalUsername(nil))
		require.False(t, h.oauthInstanceOperatorPrincipal(context.Background(), "missing"))

		require.Error(t, h.validateOAuthInstanceRefreshTokenTarget(context.Background(), &storage.OAuthClient{
			ClientClass: "operator",
			OwnerID:     "somebody-else",
		}, &storage.RefreshToken{Username: "alice"}))

		var nilHandler *Handler
		_, _, err = nilHandler.resolveAuthorizeTargetInstanceFromResource(
			context.Background(),
			"https://example.com/instance/ptah/mcp",
			"alice",
		)
		require.ErrorContains(t, err, "unavailable")
	})

	t.Run("authorization initialization fails closed when the signing secret is unavailable", func(t *testing.T) {
		withFailingOAuthSecretResolver(t)
		secretFailure, _, _ := round11NewHandler(t, round11TestConfig(), &round10QueryState{})
		secretFailure.cfg.JWTSecret = ""
		ctx, err := round10NewLiftContext(http.MethodGet, "/oauth/authorize", nil, map[string]string{
			"response_type": "code",
			"client_id":     "client-1",
			"redirect_uri":  "https://client.example/cb",
		}, nil)
		require.NoError(t, err)
		flow, resp, err := secretFailure.initializeAuthorizeFlow(ctx)
		require.NoError(t, err)
		require.Nil(t, flow)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusFound, resp.Status)
		require.Contains(t, firstStringValue(resp.Headers, "location"), "server_error")
	})

	t.Run("instance binding maps internal canonicalization failures to server_error", func(t *testing.T) {
		badConfig := &Handler{logger: zap.NewNop()}
		ctx, err := round10NewLiftContext(http.MethodGet, "/oauth/authorize", nil, nil, nil)
		require.NoError(t, err)
		resp, err := badConfig.bindAuthorizeTarget(ctx, &authorizeFlow{
			request: &authorizeRequest{resource: "https://example.com/instance/ptah/mcp"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusBadRequest, resp.Status)
		require.Contains(t, string(resp.Body), "server_error")
	})
}

func TestReleaseCoverageMargin_OAuthTokenParsingAndAgentLifetimes(t *testing.T) {
	t.Parallel()

	t.Run("token request honors lowercase content type and ignores non-Basic authorization", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(
			http.MethodPost,
			"/oauth/token",
			map[string]string{
				"content-type":  "application/x-www-form-urlencoded",
				"Authorization": "Bearer unrelated",
			},
			nil,
			[]byte("grant_type=refresh_token&client_id=client-1&refresh_token=rt-1"),
		)
		req, resp, err := parseOAuthTokenRequest(ctx)
		require.NoError(t, err)
		require.Nil(t, resp)
		require.Equal(t, "client-1", req.clientID)
	})

	t.Run("Basic authorization overrides form client credentials", func(t *testing.T) {
		ctx := round10NewLiftContextWithBodyBytes(
			http.MethodPost,
			"/oauth/token",
			map[string]string{
				"Content-Type":  "application/x-www-form-urlencoded",
				"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte("basic-client:basic-secret")),
			},
			nil,
			[]byte("grant_type=refresh_token&client_id=form-client&client_secret=form-secret&refresh_token=rt-1"),
		)
		req, resp, err := parseOAuthTokenRequest(ctx)
		require.NoError(t, err)
		require.Nil(t, resp)
		require.Equal(t, "basic-client", req.clientID)
		require.Equal(t, "basic-secret", req.clientSecret)
	})

	t.Run("token request rejects malformed Basic credentials and resources", func(t *testing.T) {
		tests := []struct {
			name    string
			header  string
			body    string
			wantErr string
		}{
			{
				name:    "Basic credential without separator",
				header:  "Basic " + base64.StdEncoding.EncodeToString([]byte("client-only")),
				body:    "grant_type=authorization_code",
				wantErr: "invalid_client",
			},
			{
				name:    "resource fragment",
				body:    "grant_type=authorization_code&resource=https%3A%2F%2Fexample.com%2Fmcp%2Falice%23fragment",
				wantErr: "invalid_target",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
				if tt.header != "" {
					headers["Authorization"] = tt.header
				}
				ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/token", headers, nil, []byte(tt.body))
				req, resp, err := parseOAuthTokenRequest(ctx)
				require.NoError(t, err)
				require.Nil(t, req)
				require.NotNil(t, resp)
				require.Contains(t, string(resp.Body), tt.wantErr)
			})
		}
	})

	t.Run("agent refresh lifetime is bounded by every stored expiry", func(t *testing.T) {
		now := time.Date(2026, time.August, 5, 12, 0, 0, 0, time.UTC)
		tests := []struct {
			name  string
			token *storage.RefreshToken
			ok    bool
			ttl   time.Duration
		}{
			{name: "missing token"},
			{name: "expired refresh", token: &storage.RefreshToken{Current: true, ExpiresAt: now.Add(-time.Second)}},
			{
				name: "expired idle bound",
				token: &storage.RefreshToken{
					Current:       true,
					ExpiresAt:     now.Add(time.Hour),
					IdleExpiresAt: now.Add(-time.Second),
				},
			},
			{
				name: "short absolute bound caps access token",
				token: &storage.RefreshToken{
					Current:           true,
					ExpiresAt:         now.Add(time.Hour),
					AbsoluteExpiresAt: now.Add(2 * time.Minute),
				},
				ok:  true,
				ttl: 2 * time.Minute,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				ttl, expiry, err := agentRefreshGrantLifetimes(now, round11TestConfig(), tt.token)
				if !tt.ok {
					require.Error(t, err)
					return
				}
				require.NoError(t, err)
				require.Equal(t, tt.ttl, ttl)
				require.Equal(t, now.Add(tt.ttl), expiry)
			})
		}
		require.False(t, refreshTokenCarriesRuntimeDecisionState(nil))
		require.False(t, oauthClientSupportsGrantType(&storage.OAuthClient{}, "unknown_grant"))
		require.Empty(t, refreshGrantClientClass(nil, nil))
	})
}

func TestReleaseCoverageMargin_OAuthConsentRequiresEveryScope(t *testing.T) {
	t.Parallel()

	h, _, _ := round11NewHandler(t, round11TestConfig(), &RegistryStub{
		AccountsSvc: &AccountsServiceStub{
			GetUserAppConsentFunc: func(context.Context, *accounts.GetUserAppConsentQuery) (*accounts.GetUserAppConsentResult, error) {
				return &accounts.GetUserAppConsentResult{Consent: &storage.UserAppConsent{Scopes: []string{"read"}}}, nil
			},
		},
	})
	require.False(t, h.hasUserConsentedToApp(context.Background(), "alice", "client-1", "", []string{"read", "write"}))
}

func TestReleaseCoverageMargin_OAuthRevocationFailsClosed(t *testing.T) {
	ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/revoke", nil, nil, []byte("token=rt-1"))

	t.Run("nil handler", func(t *testing.T) {
		var h *Handler
		requireStatus(t, http.StatusInternalServerError)(h.HandleOAuthRevokeLift(ctx))
	})

	t.Run("missing storage", func(t *testing.T) {
		h := &Handler{logger: zap.NewNop()}
		requireStatus(t, http.StatusServiceUnavailable)(h.HandleOAuthRevokeLift(ctx))
	})

	t.Run("default and unsupported hints do not expose token existence", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, round11TestConfig(), &round10QueryState{})
		parsed, resp := h.parseOAuthRevokeRequest(ctx)
		require.Nil(t, resp)
		require.Equal(t, oauthGrantTypeRefreshToken, parsed.hint)

		unknown := round10NewLiftContextWithBodyBytes(http.MethodPost, "/oauth/revoke", nil, nil, []byte("token=rt-1&token_type_hint=id_token"))
		requireStatus(t, http.StatusOK)(h.HandleOAuthRevokeLift(unknown))
	})

	t.Run("OAuth secret resolution failure preserves a successful best-effort response", func(t *testing.T) {
		withFailingOAuthSecretResolver(t)
		h, _, _ := round11NewHandler(t, round11TestConfig(), &round10QueryState{})
		h.cfg.JWTSecret = ""
		require.Nil(t, h.revokeRefreshTokenBestEffort(context.Background(), nil))
	})

	t.Run("refresh client load and OAuth initialization failures do not revoke", func(t *testing.T) {
		stored := &storage.RefreshToken{ClientID: "client-1"}

		loadFailure, _, _ := round11NewHandler(t, round11TestConfig(), &round10QueryState{
			firstErrorPK: map[string]error{"OAUTH_CLIENT#client-1": errors.New("read failed")},
		})
		client, resp := loadFailure.validateOAuthRevokeRefreshClient(context.Background(), stored, "secret")
		require.Nil(t, client)
		require.Nil(t, resp)

		state := &round10QueryState{oauthClientsByID: map[string]storagemodels.OAuthClient{
			"client-1": {ClientID: "client-1", ClientSecret: "secret", Confidential: true},
		}}
		h, _, _ := round11NewHandler(t, round11TestConfig(), state)
		withFailingOAuthSecretResolver(t)
		h.cfg.JWTSecret = ""
		client, resp = h.validateOAuthRevokeRefreshClient(context.Background(), stored, "secret")
		require.Nil(t, client)
		require.NotNil(t, resp)
		require.Equal(t, http.StatusInternalServerError, resp.Status)
	})
}

func TestReleaseCoverageMargin_QuoteFailureAndPaginationContracts(t *testing.T) {
	t.Parallel()

	cfg := round11TestConfig()
	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:statuses"})
	target := &storagemodels.Status{
		StatusID:       "target-1",
		AuthorID:       cfg.ActorURL("bob"),
		AuthorUsername: "bob",
		Visibility:     storagemodels.VisibilityPublic,
	}
	created := &storagemodels.Status{
		StatusID:       "quote-1",
		AuthorID:       cfg.ActorURL("alice"),
		AuthorUsername: "alice",
		Visibility:     storagemodels.VisibilityPublic,
	}

	t.Run("create failures never return a partially attached quote", func(t *testing.T) {
		tests := []struct {
			name       string
			resolve    func(context.Context, string, string) (*storagemodels.Status, error)
			create     func(context.Context, *notes.CreateNoteCommand) (*notes.NoteResult, error)
			attach     func(context.Context, *storagemodels.Status, string) (*quotes.QuotePostResult, error)
			wantStatus int
		}{
			{
				name:       "missing target",
				resolve:    func(context.Context, string, string) (*storagemodels.Status, error) { return nil, nil },
				wantStatus: http.StatusNotFound,
			},
			{
				name:    "note creation error",
				resolve: func(context.Context, string, string) (*storagemodels.Status, error) { return target, nil },
				create: func(context.Context, *notes.CreateNoteCommand) (*notes.NoteResult, error) {
					return nil, errors.New("create failed")
				},
				wantStatus: http.StatusInternalServerError,
			},
			{
				name:       "empty note result",
				resolve:    func(context.Context, string, string) (*storagemodels.Status, error) { return target, nil },
				create:     func(context.Context, *notes.CreateNoteCommand) (*notes.NoteResult, error) { return nil, nil },
				wantStatus: http.StatusInternalServerError,
			},
			{
				name:    "attachment error",
				resolve: func(context.Context, string, string) (*storagemodels.Status, error) { return target, nil },
				create: func(context.Context, *notes.CreateNoteCommand) (*notes.NoteResult, error) {
					return &notes.NoteResult{Note: created}, nil
				},
				attach: func(context.Context, *storagemodels.Status, string) (*quotes.QuotePostResult, error) {
					return nil, errors.New("attach failed")
				},
				wantStatus: http.StatusInternalServerError,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				reg := &RegistryStub{
					NotesSvc: &NotesServiceStub{
						ResolveQuoteTargetFunc: tt.resolve,
						CreateNoteFunc:         tt.create,
					},
					QuotesSvc: &QuotesServiceStub{
						CheckQuotePermissionsFunc: func(context.Context, string, *storagemodels.Status) (bool, error) {
							return true, nil
						},
						AttachQuoteToStatusFunc: tt.attach,
					},
				}
				h, _, _ := round11NewHandler(t, cfg, reg)
				ctx, err := round10NewLiftContext(
					http.MethodPost,
					"/api/v1/statuses/target-1/quote",
					map[string]string{"Authorization": "Bearer " + token},
					nil,
					apimodels.CreateQuotePostRequest{Status: "release quote"},
				)
				require.NoError(t, err)
				ctx.Params["id"] = "target-1"
				requireStatus(t, tt.wantStatus)(h.HandleCreateQuotePostLift(ctx))
			})
		}
	})

	t.Run("child reach cannot exceed a private target", func(t *testing.T) {
		privateTarget := *target
		privateTarget.Visibility = storagemodels.VisibilityPrivate
		reg := &RegistryStub{
			NotesSvc: &NotesServiceStub{
				ResolveQuoteTargetFunc: func(context.Context, string, string) (*storagemodels.Status, error) {
					return &privateTarget, nil
				},
			},
			QuotesSvc: &QuotesServiceStub{},
		}
		h, _, _ := round11NewHandler(t, cfg, reg)
		ctx, err := round10NewLiftContext(
			http.MethodPost,
			"/api/v1/statuses/target-1/quote",
			map[string]string{"Authorization": "Bearer " + token},
			nil,
			apimodels.CreateQuotePostRequest{Status: "release quote", Visibility: storagemodels.VisibilityPublic},
		)
		require.NoError(t, err)
		ctx.Params["id"] = "target-1"
		requireStatus(t, http.StatusUnprocessableEntity)(h.HandleCreateQuotePostLift(ctx))
	})

	t.Run("missing visible target is indistinguishable from an absent status", func(t *testing.T) {
		reg := &RegistryStub{
			NotesSvc: &NotesServiceStub{
				GetNoteWithViewerFunc: func(context.Context, *notes.GetNoteQuery) (*storagemodels.Status, error) {
					return nil, nil
				},
			},
			QuotesSvc: &QuotesServiceStub{},
		}
		h, _, _ := round11NewHandler(t, cfg, reg)
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/target-1/quotes", nil, nil, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "target-1"
		requireStatus(t, http.StatusNotFound)(h.HandleGetQuotesOfStatusLift(ctx))
	})

	t.Run("first permission update materializes safe defaults", func(t *testing.T) {
		writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write:accounts"})
		var saved *storagemodels.QuotePermissions
		reg := &RegistryStub{QuotesSvc: &QuotesServiceStub{
			GetQuotePermissionsFunc: func(context.Context, string) (*storagemodels.QuotePermissions, error) {
				return nil, nil
			},
			UpdateQuotePermissionsFunc: func(_ context.Context, permissions *storagemodels.QuotePermissions) error {
				copy := *permissions
				saved = &copy
				return nil
			},
		}}
		h, _, _ := round11NewHandler(t, cfg, reg)
		ctx, err := round10NewLiftContext(
			http.MethodPut,
			"/api/v1/accounts/quote_permissions",
			map[string]string{"Authorization": "Bearer " + writeToken},
			nil,
			apimodels.UpdateQuotePermissionsRequest{},
		)
		require.NoError(t, err)
		requireStatus(t, http.StatusOK)(h.HandleUpdateQuotePermissionsLift(ctx))
		require.NotNil(t, saved)
		require.Equal(t, "alice", saved.Username)
		require.NotNil(t, saved.BlockList)
	})

	t.Run("pagination rejects unavailable and cyclic pages", func(t *testing.T) {
		h, _, _ := round11NewHandlerSliceC(t, nil)
		notesSvc := &NotesServiceStub{GetNoteWithViewerFunc: func(context.Context, *notes.GetNoteQuery) (*storagemodels.Status, error) {
			return nil, nil
		}}

		_, err := h.listVisibleQuoteSummaries(context.Background(), notesSvc, &QuotesServiceStub{
			GetQuoteRelationshipsForStatusFunc: func(context.Context, string, int, string) (*quotes.QuoteRelationshipPage, error) {
				return nil, nil
			},
		}, "target-1", "alice", 1, 0)
		require.ErrorContains(t, err, "page is unavailable")

		calls := 0
		_, err = h.listVisibleQuoteSummaries(context.Background(), notesSvc, &QuotesServiceStub{
			GetQuoteRelationshipsForStatusFunc: func(context.Context, string, int, string) (*quotes.QuoteRelationshipPage, error) {
				calls++
				return &quotes.QuoteRelationshipPage{
					Relationships: []*storagemodels.QuoteRelationship{nil},
					NextCursor:    "same-cursor",
				}, nil
			},
		}, "target-1", "alice", 21, 0)
		require.ErrorContains(t, err, "cursor repeated")
		require.Equal(t, 2, calls)
	})

	t.Run("visible summary propagates storage failures and hides missing rows", func(t *testing.T) {
		relationship := &storagemodels.QuoteRelationship{QuoterNoteID: "quote-1"}
		_, visible, err := visibleQuoteSummary(context.Background(), &NotesServiceStub{
			GetNoteWithViewerFunc: func(context.Context, *notes.GetNoteQuery) (*storagemodels.Status, error) {
				return nil, commonerrors.Internal("read failed")
			},
		}, relationship, "alice")
		require.Error(t, err)
		require.False(t, visible)

		_, visible, err = visibleQuoteSummary(context.Background(), &NotesServiceStub{
			GetNoteWithViewerFunc: func(context.Context, *notes.GetNoteQuery) (*storagemodels.Status, error) {
				return nil, nil
			},
		}, relationship, "alice")
		require.NoError(t, err)
		require.False(t, visible)
	})

	t.Run("summary and relationship bounds preserve compatibility fallbacks", func(t *testing.T) {
		require.Nil(t, boundedQuoteRelationships([]*storagemodels.QuoteRelationship{{}}, 0))
		require.Len(t, boundedQuoteRelationships([]*storagemodels.QuoteRelationship{{}, {}}, 1), 1)

		published := time.Date(2026, time.August, 5, 10, 0, 0, 0, time.UTC)
		summary := quoteStatusSummary(&storagemodels.Status{
			StatusID:       "quote-1",
			AuthorUsername: "alice",
			PublishedAt:    published,
		})
		require.Equal(t, "alice", summary.Account.ID)
		require.Equal(t, published.Format(time.RFC3339Nano), summary.CreatedAt)
		require.Empty(t, quotePermissionsResponse(nil).BlockList)
	})
}
