package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	storagerepos "github.com/equaltoai/lesser/pkg/storage/repositories"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

func agentRuntimeSessionFromToken(token storage.RefreshToken) apimodels.AgentRuntimeSession {
	var revokedAt *time.Time
	if !token.RevokedAt.IsZero() {
		value := token.RevokedAt
		revokedAt = &value
	}

	return apimodels.AgentRuntimeSession{
		SessionID:         token.SessionID,
		ClientID:          token.ClientID,
		DeviceLabel:       auth.CoalesceAgentRuntimeLabel(token.DeviceLabel, ""),
		Scope:             strings.Join(token.Scopes, " "),
		CreatedAt:         token.SessionCreatedAt,
		LastUsedAt:        token.LastUsedAt,
		IdleExpiresAt:     token.IdleExpiresAt,
		AbsoluteExpiresAt: token.AbsoluteExpiresAt,
		Revoked:           token.Revoked,
		RevokedAt:         revokedAt,
		RevokedReason:     token.RevokedReason,
	}
}

func runtimeRefreshAccessTTL(cfg *config.Config, token *storage.RefreshToken) time.Duration {
	ttlSeconds := token.AccessTTLSeconds
	if ttlSeconds <= 0 {
		ttlSeconds = int(auth.AgentAccessTokenTTL(cfg).Seconds())
	}
	ttl := time.Duration(ttlSeconds) * time.Second
	if ttl <= 0 {
		ttl = auth.AgentAccessTokenTTL(cfg)
	}
	now := time.Now().UTC()
	for _, bound := range []time.Duration{
		runtimePositiveRemaining(now, token.IdleExpiresAt),
		runtimePositiveRemaining(now, token.AbsoluteExpiresAt),
	} {
		if bound > 0 && bound < ttl {
			ttl = bound
		}
	}
	return ttl
}

func runtimeExpiryExceeded(now, expiry time.Time) bool {
	return !expiry.IsZero() && now.After(expiry)
}

func runtimePositiveRemaining(now, expiry time.Time) time.Duration {
	if expiry.IsZero() {
		return 0
	}
	remaining := expiry.Sub(now)
	if remaining <= 0 {
		return 0
	}
	return remaining
}

func nextRuntimeIdleExpiry(now, absoluteExpiry time.Time) time.Time {
	idleExpiry := now.Add(auth.AgentRuntimeRefreshIdleTTL)
	if !absoluteExpiry.IsZero() && idleExpiry.After(absoluteExpiry) {
		return absoluteExpiry
	}
	return idleExpiry
}

func (h *Handler) noteAgentRuntimeRefreshFailure(ctx context.Context, refreshToken, clientID, failureCode, failureMessage string) {
	refreshToken = strings.TrimSpace(refreshToken)
	clientID = strings.TrimSpace(clientID)
	if refreshToken == "" || clientID == "" {
		return
	}

	storedToken, err := h.repos.Account().GetRefreshToken(ctx, refreshToken)
	if err != nil || storedToken == nil {
		return
	}
	if storedToken.ClientID != clientID || !strings.EqualFold(strings.TrimSpace(storedToken.ClientClass), auth.ClientClassAgent) {
		return
	}
	if err := auth.RecordAgentRuntimeAuthFailure(ctx, h.repos, storedToken, failureCode, failureMessage, time.Now().UTC()); err != nil {
		h.logger.Warn("failed to persist runtime auth failure diagnostic", zap.Error(err))
	}
}

func (h *Handler) exchangeAgentRuntimeRefreshToken(ctx context.Context, oauthSvc *auth.OAuthService, refreshToken, clientID, ipAddress, userAgent string) (string, string, []string, error) {
	storedToken, err := h.repos.Account().GetRefreshToken(ctx, refreshToken)
	if err != nil {
		return "", "", nil, auth.ErrInvalidToken
	}
	if storedToken.ClientID != clientID {
		return "", "", nil, auth.ErrInvalidToken
	}

	now := time.Now().UTC()
	if storedToken.Revoked {
		if err := auth.RevokeAgentRuntimeFamily(ctx, h.repos, storedToken, "refresh_token_reuse_detected", ipAddress, userAgent); err != nil {
			h.logger.Warn("failed to revoke runtime session family after refresh reuse", zap.Error(err))
		}
		if err := auth.RecordAgentRuntimeAuthFailure(ctx, h.repos, storedToken, "invalid_grant", "Refresh token reuse detected; runtime session revoked", now); err != nil {
			h.logger.Warn("failed to persist runtime reuse diagnostic", zap.Error(err))
		}
		return "", "", nil, auth.ErrInvalidToken
	}
	if runtimeExpiryExceeded(now, storedToken.IdleExpiresAt) || runtimeExpiryExceeded(now, storedToken.AbsoluteExpiresAt) {
		_ = auth.RevokeAgentRuntimeFamily(ctx, h.repos, storedToken, "runtime_session_expired", ipAddress, userAgent)
		if err := auth.RecordAgentRuntimeAuthFailure(ctx, h.repos, storedToken, "invalid_grant", "Runtime session expired", now); err != nil {
			h.logger.Warn("failed to persist runtime expiry diagnostic", zap.Error(err))
		}
		return "", "", nil, auth.ErrInvalidToken
	}

	accessTTL := runtimeRefreshAccessTTL(h.cfg, storedToken)
	accessToken, newRefreshToken, err := oauthSvc.GenerateTokensWithAccessTokenTTLAndClientContext(
		ctx,
		storedToken.Username,
		clientID,
		"",
		storedToken.Scopes,
		accessTTL,
		auth.ClientClassAgent,
		storedToken.SessionID,
	)
	if err != nil {
		return "", "", nil, err
	}

	storedToken.Current = false
	storedToken.Revoked = true
	storedToken.RevokedAt = now
	storedToken.RevokedReason = "rotated"
	storedToken.LastUsedAt = now
	if err := h.repos.Account().UpdateRefreshToken(ctx, storedToken); err != nil {
		if storagerepos.ErrorHandler.IsConditionalCheckFailed(err) || strings.Contains(strings.ToLower(err.Error()), "condition") {
			if revokeErr := auth.RevokeAgentRuntimeFamily(ctx, h.repos, storedToken, "refresh_token_reuse_detected", ipAddress, userAgent); revokeErr != nil {
				h.logger.Warn("failed to revoke runtime session family after concurrent refresh", zap.Error(revokeErr))
			}
			if diagErr := auth.RecordAgentRuntimeAuthFailure(ctx, h.repos, storedToken, "invalid_grant", "Concurrent refresh detected; runtime session revoked", now); diagErr != nil {
				h.logger.Warn("failed to persist runtime concurrent refresh diagnostic", zap.Error(diagErr))
			}
			return "", "", nil, auth.ErrInvalidToken
		}
		return "", "", nil, err
	}

	newToken := &storage.RefreshToken{
		Token:               newRefreshToken,
		Username:            storedToken.Username,
		ClientID:            clientID,
		Scopes:              storedToken.Scopes,
		CreatedAt:           now,
		ExpiresAt:           nextRuntimeIdleExpiry(now, storedToken.AbsoluteExpiresAt),
		ClientClass:         auth.ClientClassAgent,
		SessionID:           storedToken.SessionID,
		FamilyID:            storedToken.FamilyID,
		Generation:          storedToken.Generation + 1,
		Current:             true,
		DeviceLabel:         storedToken.DeviceLabel,
		LastUsedAt:          now,
		IdleExpiresAt:       nextRuntimeIdleExpiry(now, storedToken.AbsoluteExpiresAt),
		AbsoluteExpiresAt:   storedToken.AbsoluteExpiresAt,
		SessionCreatedAt:    storedToken.SessionCreatedAt,
		AccessTTLSeconds:    storedToken.AccessTTLSeconds,
		LastAuthFailureCode: storedToken.LastAuthFailureCode,
		LastAuthFailureAt:   storedToken.LastAuthFailureAt,
		LastAuthFailureMsg:  storedToken.LastAuthFailureMsg,
		LastAuthSuccessAt:   now,
	}
	if err := h.repos.Account().CreateRefreshToken(ctx, newToken); err != nil {
		_ = auth.RevokeAgentRuntimeFamily(ctx, h.repos, storedToken, "runtime_session_rotation_failed", ipAddress, userAgent)
		if diagErr := auth.RecordAgentRuntimeAuthFailure(ctx, h.repos, storedToken, "server_error", "Runtime session rotation failed", now); diagErr != nil {
			h.logger.Warn("failed to persist runtime rotation diagnostic", zap.Error(diagErr))
		}
		return "", "", nil, err
	}

	return accessToken, newRefreshToken, storedToken.Scopes, nil
}

// HandleListAgentRuntimeSessionsLift lists first-class runtime refresh sessions for an agent.
func (h *Handler) HandleListAgentRuntimeSessionsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentsEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}

	username := strings.TrimSpace(ctx.Param("username"))
	if err := common.ValidateUsernameParamID(username); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	claims, resp, err := h.authenticateAgentOwner(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	account, err := h.repos.Account().GetAccount(ctx.Context(), username)
	if err != nil || account == nil || account.User == nil || !account.User.IsAgent {
		return common.RespondNotFound(ctx, "agent")
	}
	if !h.isAgentOwnerOrAdmin(claims, account.User) {
		return common.RespondForbidden(ctx, "not authorized to manage this agent")
	}

	sessions, err := auth.ListAgentRuntimeSessions(ctx.Context(), h.repos, username)
	if err != nil {
		return common.RespondInternalServerError(ctx)
	}

	out := make([]apimodels.AgentRuntimeSession, 0, len(sessions))
	for _, session := range sessions {
		out = append(out, agentRuntimeSessionFromToken(session))
	}
	return okJSON(out)
}

// HandleRevokeAgentRuntimeSessionLift revokes one runtime refresh session family for an agent.
func (h *Handler) HandleRevokeAgentRuntimeSessionLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	if resp, err := h.ensureAgentsEnabled(ctx); resp != nil || err != nil {
		return resp, err
	}

	username := strings.TrimSpace(ctx.Param("username"))
	sessionID := strings.TrimSpace(ctx.Param("sessionID"))
	if err := common.ValidateUsernameParamID(username); err != nil {
		return common.RespondValidationError(ctx, err)
	}
	if err := common.ValidateRequiredParam("sessionID", sessionID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	claims, resp, err := h.authenticateAgentOwner(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	account, err := h.repos.Account().GetAccount(ctx.Context(), username)
	if err != nil || account == nil || account.User == nil || !account.User.IsAgent {
		return common.RespondNotFound(ctx, "agent")
	}
	if !h.isAgentOwnerOrAdmin(claims, account.User) {
		return common.RespondForbidden(ctx, "not authorized to manage this agent")
	}

	var req apimodels.RevokeAgentRuntimeSessionRequest
	if len(ctx.Request.Body) > 0 {
		if err := common.ParseRequestWithFallback(ctx, &req); err != nil {
			return common.RespondBadRequest(ctx, "invalid request body")
		}
	}
	userAgent, ipAddress := h.getDeviceInfo(ctx)
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "manual_runtime_session_revocation"
	}
	if err := auth.RevokeAgentRuntimeSession(ctx.Context(), h.repos, username, sessionID, reason, ipAddress, userAgent); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return common.RespondNotFound(ctx, "runtime session")
		}
		return common.RespondInternalServerError(ctx)
	}

	sessions, err := auth.ListAgentRuntimeSessions(ctx.Context(), h.repos, username)
	if err != nil {
		return common.RespondInternalServerError(ctx)
	}
	for _, session := range sessions {
		if session.SessionID == sessionID {
			return okJSON(agentRuntimeSessionFromToken(session))
		}
	}

	return apptheory.JSON(http.StatusOK, map[string]any{"session_id": sessionID, "revoked": true})
}
