package handlers

import (
	"context"
	"net/http"
	"strings"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

const oauthTokenTypeHintAccessToken = "access_token"

type oauthRevokeRequest struct {
	token        string
	clientID     string
	clientSecret string
	hint         string
}

// HandleOAuthRevokeLift handles POST /oauth/revoke (RFC 7009).
//
// Lesser currently supports revoking refresh tokens. Access tokens are short-lived and are not revoked server-side.
// Per RFC 7009, the endpoint returns a 200 response even for unknown tokens to avoid token fishing.
func (h *Handler) HandleOAuthRevokeLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if h == nil || ctx == nil {
		return common.RespondInternalServerError(ctx)
	}
	if h.repos == nil || h.repos.Account() == nil {
		return oauthRevokeError(http.StatusServiceUnavailable, "server_error", "storage is not initialized")
	}

	req, resp := h.parseOAuthRevokeRequest(ctx)
	if resp != nil {
		return resp, nil
	}

	switch req.hint {
	case oauthTokenTypeHintAccessToken:
		h.revokeAccessTokenBestEffort(ctx.Context(), req.token)
	case oauthGrantTypeRefreshToken:
		if resp := h.revokeRefreshTokenBestEffort(ctx.Context(), req); resp != nil {
			return resp, nil
		}
	default:
		// Only access_token + refresh_token are supported. For other hints, return 200 (no-op).
	}
	return okJSON(map[string]any{})
}

func oauthRevokeError(status int, code, description string) (*apptheory.Response, error) {
	return apptheory.JSON(status, apimodels.OAuthErrorResponse{
		Error:            code,
		ErrorDescription: description,
	})
}

func (h *Handler) parseOAuthRevokeRequest(ctx *apptheory.Context) (*oauthRevokeRequest, *apptheory.Response) {
	if err := common.ValidateSliceNotEmpty("request_body", ctx.Request.Body); err != nil {
		r, _ := oauthRevokeError(http.StatusBadRequest, "invalid_request", "empty request body")
		return nil, r
	}

	params, err := common.ParseFormURLEncoded(string(ctx.Request.Body))
	if err != nil {
		r, _ := oauthRevokeError(http.StatusBadRequest, "invalid_request", "unable to parse form data")
		return nil, r
	}

	token := strings.TrimSpace(params["token"])
	if token == "" {
		r, _ := oauthRevokeError(http.StatusBadRequest, "invalid_request", "token is required")
		return nil, r
	}

	clientID := strings.TrimSpace(params["client_id"])
	clientSecret := params["client_secret"]
	basicClientID, basicClientSecret, usedBasicAuth, basicErr := parseOAuthTokenBasicClientCredentials(ctx)
	if basicErr != nil {
		r, _ := oauthRevokeError(http.StatusBadRequest, "invalid_client", "invalid client credentials")
		return nil, r
	}
	if usedBasicAuth {
		clientID = basicClientID
		clientSecret = basicClientSecret
	}

	hint := strings.ToLower(strings.TrimSpace(params["token_type_hint"]))
	if hint == "" {
		hint = oauthGrantTypeRefreshToken
	}

	// Only access_token + refresh_token are supported. For other hints, return 200 (no-op).
	if hint != "" && hint != oauthGrantTypeRefreshToken && hint != oauthTokenTypeHintAccessToken {
		r, _ := okJSON(map[string]any{})
		return nil, r
	}

	return &oauthRevokeRequest{
		token:        token,
		clientID:     clientID,
		clientSecret: clientSecret,
		hint:         hint,
	}, nil
}

func (h *Handler) revokeAccessTokenBestEffort(ctx context.Context, token string) {
	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil || claims == nil {
		return
	}

	jti := strings.TrimSpace(claims.ID)
	if jti == "" || claims.ExpiresAt == nil {
		return
	}

	expiresAt := claims.ExpiresAt.Time
	if expiresAt.IsZero() {
		return
	}

	if err := h.repos.Account().RevokeAccessToken(ctx, jti, expiresAt); err != nil {
		h.logger.Warn("failed to revoke access token", zap.Error(err))
	}
}

func (h *Handler) revokeRefreshTokenBestEffort(ctx context.Context, req *oauthRevokeRequest) *apptheory.Response {
	if req == nil {
		return nil
	}

	stored, err := h.repos.Account().GetRefreshToken(ctx, req.token)
	if err != nil || stored == nil {
		return nil
	}

	// Refresh token revocation is authenticated to the token-owning client. Unknown
	// tokens still return 200 above, but a known token without a matching client_id
	// must not be revocable by an unauthenticated third party that merely learns the
	// token string.
	if req.clientID == "" || stored.ClientID != req.clientID {
		return nil
	}

	client, resp := h.validateOAuthRevokeRefreshClient(ctx, stored, req.clientSecret)
	if resp != nil {
		return resp
	}
	if client == nil {
		return nil
	}

	if auth.IsAgentRuntimeClientID(stored.ClientID) {
		if err := auth.RevokeAgentRuntimeFamily(ctx, h.repos, stored, "oauth_revoke", "", ""); err != nil {
			h.logger.Warn("failed to revoke runtime refresh session", zap.Error(err), zap.String("client_id", stored.ClientID))
		}
		return nil
	}

	if err := h.repos.Account().DeleteRefreshToken(ctx, req.token); err != nil {
		h.logger.Warn("failed to revoke refresh token", zap.Error(err), zap.String("client_id", stored.ClientID))
	}
	return nil
}

func (h *Handler) validateOAuthRevokeRefreshClient(ctx context.Context, stored *storage.RefreshToken, clientSecret string) (*storage.OAuthClient, *apptheory.Response) {
	client, err := h.repos.Account().GetOAuthClient(ctx, strings.TrimSpace(stored.ClientID))
	if err != nil || client == nil {
		h.logger.Warn("failed to load OAuth client for refresh token revocation", zap.Error(err), zap.String("client_id", stored.ClientID))
		return nil, nil
	}

	oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	if err := validateRefreshGrantClientSecret(ctx, oauthSvc, client, stored.ClientID, clientSecret); err != nil {
		resp, _ := oauthRevokeError(http.StatusBadRequest, "invalid_client", "invalid client credentials")
		return nil, resp
	}
	return client, nil
}
