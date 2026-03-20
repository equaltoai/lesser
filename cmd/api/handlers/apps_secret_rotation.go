package handlers

import (
	"context"
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
		h.logOAuthClientSecretRotation(ctx, claims.Username, oauthClientSecretRotationAuditMetadata(clientID, nil, req, 0, time.Time{}), false, err)
		return h.respondBadRequest(ctx, err.Error())
	}
	gracePeriod, err := oauthClientSecretRotationGracePeriod(h.cfg, req)
	if err != nil {
		h.logOAuthClientSecretRotation(ctx, claims.Username, oauthClientSecretRotationAuditMetadata(clientID, nil, req, 0, time.Time{}), false, err)
		return h.respondBadRequest(ctx, err.Error())
	}

	client, err := h.repos.Account().GetOAuthClient(ctx.Context(), clientID)
	if err != nil || client == nil {
		h.logOAuthClientSecretRotation(ctx, claims.Username, oauthClientSecretRotationAuditMetadata(clientID, nil, req, gracePeriod, time.Time{}), false, errors.New("oauth client not found"))
		return h.respondNotFound(ctx, "oauth client")
	}

	if strings.TrimSpace(client.OwnerID) == "" || !strings.EqualFold(strings.TrimSpace(client.OwnerID), claims.Username) {
		h.logOAuthClientSecretRotation(ctx, claims.Username, oauthClientSecretRotationAuditMetadata(clientID, client, req, gracePeriod, time.Time{}), false, errors.New("not authorized to rotate this client"))
		return h.respondForbidden(ctx, "not authorized to rotate this client")
	}

	if strings.EqualFold(strings.TrimSpace(client.ClientClass), auth.ClientClassAgent) {
		if _, agentErr := h.getAgentUserForOAuthClient(ctx.Context(), client, claims.Username); agentErr != nil {
			h.logOAuthClientSecretRotation(ctx, claims.Username, oauthClientSecretRotationAuditMetadata(clientID, client, req, gracePeriod, time.Time{}), false, agentErr)
			return h.respondForbidden(ctx, "not authorized to rotate this client")
		}
	}

	newSecret, err := generateOAuthClientSecret()
	if err != nil {
		h.logger.Error("failed to generate OAuth client secret", zap.Error(err))
		h.logOAuthClientSecretRotation(ctx, claims.Username, oauthClientSecretRotationAuditMetadata(clientID, client, req, gracePeriod, time.Time{}), false, err)
		return h.respondInternalError(ctx, "failed to generate client secret")
	}
	newSecretHash, err := auth.HashOAuthClientSecret(newSecret)
	if err != nil {
		h.logger.Error("failed to hash rotated OAuth client secret", zap.Error(err))
		h.logOAuthClientSecretRotation(ctx, claims.Username, oauthClientSecretRotationAuditMetadata(clientID, client, req, gracePeriod, time.Time{}), false, err)
		return h.respondInternalError(ctx, "failed to rotate client secret")
	}
	previousSecretHash, err := normalizeStoredOAuthClientSecretHash(client)
	if err != nil {
		h.logger.Error("failed to normalize prior OAuth client secret", zap.String("client_id", clientID), zap.Error(err))
		h.logOAuthClientSecretRotation(ctx, claims.Username, oauthClientSecretRotationAuditMetadata(clientID, client, req, gracePeriod, time.Time{}), false, err)
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
		h.logOAuthClientSecretRotation(ctx, claims.Username, oauthClientSecretRotationAuditMetadata(clientID, client, req, gracePeriod, rotation.PreviousClientSecretGraceExpiresAt), false, err)
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
	h.logOAuthClientSecretRotation(ctx, claims.Username, oauthClientSecretRotationAuditMetadata(clientID, client, req, gracePeriod, rotation.PreviousClientSecretGraceExpiresAt), true, nil)

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

func oauthClientSecretRotationAuditMetadata(clientID string, client *storage.OAuthClient, req models.AppSecretRotationRequest, gracePeriod time.Duration, previousSecretValidUntil time.Time) map[string]interface{} {
	metadata := map[string]interface{}{
		"client_id":            clientID,
		"forced_invalidation":  req.ForceInvalidate,
		"grace_period_seconds": int(gracePeriod.Seconds()),
	}
	if client != nil {
		metadata["client_class"] = strings.TrimSpace(client.ClientClass)
		metadata["client_auth_method"] = oauthClientTokenEndpointAuthMethod(client)
		if agentUsername := strings.TrimSpace(client.AgentUsername); agentUsername != "" {
			metadata["agent_username"] = agentUsername
		}
	}
	if !previousSecretValidUntil.IsZero() {
		metadata["previous_secret_valid_until"] = previousSecretValidUntil.Format(time.RFC3339)
	}
	return metadata
}

func (h *Handler) logOAuthClientSecretRotation(ctx *apptheory.Context, username string, metadata map[string]interface{}, success bool, err error) {
	if h == nil {
		return
	}

	auditLogger := auth.NewAuditLogger(h.repos, h.logger, auth.DefaultAuditConfig())
	if auditLogger == nil {
		return
	}

	userAgent, ipAddress := h.getDeviceInfo(ctx)
	requestID := ""
	auditCtx := context.Background()
	if ctx != nil {
		auditCtx = ctx.Context()
		requestID = ctx.RequestID
	}
	auditLogger.LogOAuthClientSecretRotation(auditCtx, username, ipAddress, userAgent, requestID, metadata, success, err)
}

func generateOAuthClientSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", errors.New("failed to read random bytes")
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
