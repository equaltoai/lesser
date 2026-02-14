package handlers

import (
	"context"
	"net/http"
	"strings"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/common"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

const oauthTokenTypeHintAccessToken = "access_token"

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

	token, clientID, hint, resp := h.parseOAuthRevokeRequest(ctx)
	if resp != nil {
		return resp, nil
	}

	switch hint {
	case oauthTokenTypeHintAccessToken:
		h.revokeAccessTokenBestEffort(ctx.Context(), token)
	case oauthGrantTypeRefreshToken:
		h.revokeRefreshTokenBestEffort(ctx.Context(), token, clientID)
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

func (h *Handler) parseOAuthRevokeRequest(ctx *apptheory.Context) (token, clientID, hint string, resp *apptheory.Response) {
	if err := common.ValidateSliceNotEmpty("request_body", ctx.Request.Body); err != nil {
		r, _ := oauthRevokeError(http.StatusBadRequest, "invalid_request", "empty request body")
		return "", "", "", r
	}

	params, err := common.ParseFormURLEncoded(string(ctx.Request.Body))
	if err != nil {
		r, _ := oauthRevokeError(http.StatusBadRequest, "invalid_request", "unable to parse form data")
		return "", "", "", r
	}

	token = strings.TrimSpace(params["token"])
	if token == "" {
		r, _ := oauthRevokeError(http.StatusBadRequest, "invalid_request", "token is required")
		return "", "", "", r
	}

	clientID = strings.TrimSpace(params["client_id"])
	hint = strings.ToLower(strings.TrimSpace(params["token_type_hint"]))
	if hint == "" {
		hint = oauthGrantTypeRefreshToken
	}

	// Only access_token + refresh_token are supported. For other hints, return 200 (no-op).
	if hint != "" && hint != oauthGrantTypeRefreshToken && hint != oauthTokenTypeHintAccessToken {
		r, _ := okJSON(map[string]any{})
		return "", "", "", r
	}

	return token, clientID, hint, nil
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

func (h *Handler) revokeRefreshTokenBestEffort(ctx context.Context, token, clientID string) {
	stored, err := h.repos.Account().GetRefreshToken(ctx, token)
	if err != nil || stored == nil {
		return
	}

	// If client_id was provided, ensure it matches. Always respond 200 to avoid leaking token validity.
	if clientID != "" && stored.ClientID != clientID {
		return
	}

	if err := h.repos.Account().DeleteRefreshToken(ctx, token); err != nil {
		h.logger.Warn("failed to revoke refresh token", zap.Error(err), zap.String("client_id", stored.ClientID))
	}
}
