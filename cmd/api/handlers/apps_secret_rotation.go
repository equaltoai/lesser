package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

// HandleAppRotateSecretLift rotates an OAuth client secret in place for the owning operator.
func (h *Handler) HandleAppRotateSecretLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	claims, err := h.authenticateWithAnyScope(ctx, auth.ScopeWrite, auth.ScopeAdmin)
	if err != nil {
		return h.handleAuthServiceError(ctx, err, "rotate oauth client secret")
	}

	clientID := strings.TrimSpace(ctx.Param("id"))
	if err := common.ValidateRequiredParam("id", clientID); err != nil {
		return h.respondBadRequest(ctx, "client id is required")
	}

	req, err := parseAppSecretRotationRequest(ctx)
	if err != nil {
		return h.respondBadRequest(ctx, err.Error())
	}
	gracePeriod, err := oauthClientSecretRotationGracePeriod(h.cfg, req)
	if err != nil {
		return h.respondBadRequest(ctx, err.Error())
	}

	client, err := h.repos.Account().GetOAuthClient(ctx.Context(), clientID)
	if err != nil || client == nil {
		return h.respondNotFound(ctx, "oauth client")
	}

	if strings.TrimSpace(client.OwnerID) == "" || !strings.EqualFold(strings.TrimSpace(client.OwnerID), claims.Username) {
		return h.respondForbidden(ctx, "not authorized to rotate this client")
	}

	if strings.EqualFold(strings.TrimSpace(client.ClientClass), auth.ClientClassAgent) {
		if _, agentErr := h.getAgentUserForOAuthClient(ctx.Context(), client, claims.Username); agentErr != nil {
			return h.respondForbidden(ctx, "not authorized to rotate this client")
		}
	}

	newSecret, err := generateOAuthClientSecret()
	if err != nil {
		h.logger.Error("failed to generate OAuth client secret", zap.Error(err))
		return h.respondInternalError(ctx, "failed to generate client secret")
	}
	newSecretHash, err := auth.HashOAuthClientSecret(newSecret)
	if err != nil {
		h.logger.Error("failed to hash rotated OAuth client secret", zap.Error(err))
		return h.respondInternalError(ctx, "failed to rotate client secret")
	}
	previousSecretHash, err := normalizeStoredOAuthClientSecretHash(client)
	if err != nil {
		h.logger.Error("failed to normalize prior OAuth client secret", zap.String("client_id", clientID), zap.Error(err))
		return h.respondInternalError(ctx, "failed to rotate client secret")
	}

	rotatedAt := time.Now().UTC()
	rotation := storage.OAuthClientSecretRotation{
		ActiveClientSecretHash: newSecretHash,
		RotatedAt:              rotatedAt,
		RotatedBy:              claims.Username,
	}
	if !req.ForceInvalidate {
		rotation.PreviousClientSecretHash = previousSecretHash
		rotation.PreviousClientSecretGraceExpiresAt = rotatedAt.Add(gracePeriod)
	}

	if err := h.repos.Account().RotateOAuthClientSecret(ctx.Context(), clientID, rotation); err != nil {
		h.logger.Error("failed to persist rotated OAuth client secret", zap.String("client_id", clientID), zap.Error(err))
		return h.respondInternalError(ctx, "failed to rotate client secret")
	}

	tokenEndpointAuthMethod := "none"
	if client.Confidential {
		tokenEndpointAuthMethod = "client_secret_post"
	}

	h.logger.Info("rotated OAuth client secret",
		zap.String("client_id", clientID),
		zap.String("owner_id", claims.Username),
		zap.String("client_class", client.ClientClass),
		zap.Bool("forced_invalidation", req.ForceInvalidate),
		zap.Duration("grace_period", gracePeriod))

	previousSecretValidUntil := ""
	if !rotation.PreviousClientSecretGraceExpiresAt.IsZero() {
		previousSecretValidUntil = rotation.PreviousClientSecretGraceExpiresAt.Format(time.RFC3339)
	}

	return apptheory.JSON(http.StatusOK, models.AppSecretRotationResponse{
		ClientID:                 clientID,
		ClientSecret:             newSecret,
		TokenEndpointAuthMethod:  tokenEndpointAuthMethod,
		GracePeriodSeconds:       int(gracePeriod.Seconds()),
		ForcedInvalidation:       req.ForceInvalidate,
		RotatedAt:                rotatedAt.Format(time.RFC3339),
		PreviousSecretValidUntil: previousSecretValidUntil,
	})
}

func parseAppSecretRotationRequest(ctx *apptheory.Context) (models.AppSecretRotationRequest, error) {
	if ctx == nil || len(ctx.Request.Body) == 0 {
		return models.AppSecretRotationRequest{}, nil
	}

	contentType := strings.ToLower(strings.TrimSpace(headerValue(ctx, "Content-Type")))
	if contentType == "" {
		contentType = strings.ToLower(strings.TrimSpace(headerValue(ctx, "content-type")))
	}
	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		params, err := common.ParseFormURLEncoded(string(ctx.Request.Body))
		if err != nil {
			return models.AppSecretRotationRequest{}, errors.New("invalid request body")
		}
		return buildAppSecretRotationRequestFromParams(params)
	}

	var req models.AppSecretRotationRequest
	if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
		return models.AppSecretRotationRequest{}, errors.New("invalid request body")
	}
	return req, nil
}

func buildAppSecretRotationRequestFromParams(params map[string]string) (models.AppSecretRotationRequest, error) {
	req := models.AppSecretRotationRequest{}

	if raw := strings.TrimSpace(params["grace_period_seconds"]); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return models.AppSecretRotationRequest{}, errors.New("grace_period_seconds must be an integer")
		}
		req.GracePeriodSeconds = value
	}
	if raw := strings.TrimSpace(params["force_invalidate"]); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return models.AppSecretRotationRequest{}, errors.New("force_invalidate must be a boolean")
		}
		req.ForceInvalidate = value
	}

	return req, nil
}

func oauthClientSecretRotationGracePeriod(cfg *config.Config, req models.AppSecretRotationRequest) (time.Duration, error) {
	if req.GracePeriodSeconds < 0 {
		return 0, errors.New("grace_period_seconds must be zero or greater")
	}
	if req.ForceInvalidate {
		if req.GracePeriodSeconds > 0 {
			return 0, errors.New("grace_period_seconds cannot be combined with force_invalidate")
		}
		return 0, nil
	}
	if req.GracePeriodSeconds > 0 {
		return time.Duration(req.GracePeriodSeconds) * time.Second, nil
	}
	if cfg != nil && cfg.OAuthClientSecretRotationGracePeriod > 0 {
		return cfg.OAuthClientSecretRotationGracePeriod, nil
	}
	return config.DefaultOAuthClientSecretRotationGracePeriod, nil
}

func normalizeStoredOAuthClientSecretHash(client *storage.OAuthClient) (string, error) {
	if client == nil {
		return "", errors.New("oauth client is required")
	}

	storedSecret := strings.TrimSpace(client.ClientSecretHash)
	if storedSecret == "" {
		storedSecret = strings.TrimSpace(client.ClientSecret)
	}
	if storedSecret == "" {
		return "", errors.New("stored client secret missing")
	}
	if oauthClientSecretValueLooksHashed(storedSecret) {
		return storedSecret, nil
	}
	return auth.HashOAuthClientSecret(storedSecret)
}

func oauthClientSecretValueLooksHashed(storedSecret string) bool {
	return strings.HasPrefix(storedSecret, auth.OAuthClientSecretHashPrefix) ||
		strings.HasPrefix(storedSecret, "$2a$") ||
		strings.HasPrefix(storedSecret, "$2b$") ||
		strings.HasPrefix(storedSecret, "$2y$")
}

func generateOAuthClientSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", errors.New("failed to read random bytes")
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
