package handlers

import (
	"net/http"
	"strings"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/common"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

// HandleOAuthRevokeLift handles POST /oauth/revoke (RFC 7009).
//
// Lesser currently supports revoking refresh tokens. Access tokens are short-lived and are not revoked server-side.
// Per RFC 7009, the endpoint returns a 200 response even for unknown tokens to avoid token fishing.
func (h *Handler) HandleOAuthRevokeLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if h == nil || ctx == nil {
		return common.RespondInternalServerError(ctx)
	}
	if h.repos == nil || h.repos.Account() == nil {
		return apptheory.JSON(http.StatusServiceUnavailable, apimodels.OAuthErrorResponse{
			Error:            "server_error",
			ErrorDescription: "storage is not initialized",
		})
	}

	if err := common.ValidateSliceNotEmpty("request_body", ctx.Request.Body); err != nil {
		return apptheory.JSON(http.StatusBadRequest, apimodels.OAuthErrorResponse{
			Error:            "invalid_request",
			ErrorDescription: "empty request body",
		})
	}

	params, err := common.ParseFormURLEncoded(string(ctx.Request.Body))
	if err != nil {
		return apptheory.JSON(http.StatusBadRequest, apimodels.OAuthErrorResponse{
			Error:            "invalid_request",
			ErrorDescription: "unable to parse form data",
		})
	}

	token := strings.TrimSpace(params["token"])
	if token == "" {
		return apptheory.JSON(http.StatusBadRequest, apimodels.OAuthErrorResponse{
			Error:            "invalid_request",
			ErrorDescription: "token is required",
		})
	}

	clientID := strings.TrimSpace(params["client_id"])
	hint := strings.ToLower(strings.TrimSpace(params["token_type_hint"]))
	if hint == "" {
		hint = "refresh_token"
	}

	// Only access_token + refresh_token are supported. For other hints, return 200 (no-op).
	if hint != "" && hint != "refresh_token" && hint != "access_token" {
		return okJSON(map[string]any{})
	}

	if hint == "access_token" {
		oauthSvc := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err == nil && claims != nil {
			jti := strings.TrimSpace(claims.ID)
			var expiresAt time.Time
			if claims.ExpiresAt != nil {
				expiresAt = claims.ExpiresAt.Time
			}
			if jti != "" && !expiresAt.IsZero() {
				if err := h.repos.Account().RevokeAccessToken(ctx.Context(), jti, expiresAt); err != nil {
					h.logger.Warn("failed to revoke access token", zap.Error(err))
				}
			}
		}

		return okJSON(map[string]any{})
	}

	stored, err := h.repos.Account().GetRefreshToken(ctx.Context(), token)
	if err == nil && stored != nil {
		// If client_id was provided, ensure it matches. Always respond 200 to avoid leaking token validity.
		if clientID == "" || stored.ClientID == clientID {
			if err := h.repos.Account().DeleteRefreshToken(ctx.Context(), token); err != nil {
				h.logger.Warn("failed to revoke refresh token", zap.Error(err), zap.String("client_id", stored.ClientID))
			}
		}
	}

	return okJSON(map[string]any{})
}
