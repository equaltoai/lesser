package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
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
	if err := h.repos.Account().UpdateOAuthClientSecretHash(ctx.Context(), clientID, newSecretHash); err != nil {
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
		zap.String("client_class", client.ClientClass))

	return apptheory.JSON(http.StatusOK, models.AppSecretRotationResponse{
		ClientID:                clientID,
		ClientSecret:            newSecret,
		TokenEndpointAuthMethod: tokenEndpointAuthMethod,
	})
}

func generateOAuthClientSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", errors.New("failed to read random bytes")
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
