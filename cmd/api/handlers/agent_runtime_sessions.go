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
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
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

func (h *Handler) noteAgentRuntimeRefreshFailure(ctx context.Context, refreshToken, clientID, failureCode, failureMessage string) {
	refreshToken = strings.TrimSpace(refreshToken)
	clientID = strings.TrimSpace(clientID)
	if refreshToken == "" || clientID == "" || !auth.IsAgentRuntimeClientID(clientID) {
		return
	}

	storedToken, err := h.repos.Account().GetRefreshToken(ctx, refreshToken)
	if err != nil || storedToken == nil {
		return
	}
	if storedToken.ClientID != clientID ||
		!auth.IsAgentRuntimeRefreshToken(storedToken) {
		return
	}
	if err := auth.RecordAgentRuntimeAuthFailure(ctx, h.repos, storedToken, failureCode, failureMessage, time.Now().UTC()); err != nil {
		h.logger.Warn("failed to persist runtime auth failure diagnostic", zap.Error(err))
	}
}

// exchangeAgentRuntimeRefreshTokenWithTelemetry is the bounded migration path
// for refresh credentials issued before stateless agent re-minting. Existing
// credentials are honored until their already-persisted expiry, but are never
// rotated and never produce another refresh token.
func (h *Handler) exchangeAgentRuntimeRefreshTokenWithTelemetry(ctx context.Context, oauthSvc *auth.OAuthService, refreshToken, clientID, _, _ string, telemetry *oauthGrantTelemetry) (string, string, []string, error) {
	storedToken, err := h.loadAgentRuntimeRefreshTokenForExchange(ctx, refreshToken, clientID, telemetry)
	if err != nil {
		return "", "", nil, err
	}

	now := time.Now().UTC()
	if storedToken.Revoked || !storedToken.RevokedAt.IsZero() {
		telemetry.setReason(oauthGrantReasonRefreshRuntimeReuse)
		return "", "", nil, auth.ErrInvalidToken
	}
	if !storedToken.ExpiresAt.After(now) || runtimeExpiryExceeded(now, storedToken.IdleExpiresAt) || runtimeExpiryExceeded(now, storedToken.AbsoluteExpiresAt) {
		telemetry.setReason(oauthGrantReasonRefreshTokenExpired)
		return "", "", nil, auth.ErrInvalidToken
	}

	accessToken, err := oauthSvc.GenerateAgentAccessTokenWithClientContextAndDelegation(
		ctx,
		storedToken.Username,
		clientID,
		"",
		storedToken.Scopes,
		runtimeRefreshAccessTTL(h.cfg, storedToken),
		storedToken.SessionID,
		auth.DelegationCredentialClaims{},
	)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidToken) {
			telemetry.setReason(oauthGrantReasonRefreshRuntimeInvalid)
			return "", "", nil, err
		}
		telemetry.setReason(oauthGrantReasonRefreshRotationInfrastructure)
		return "", "", nil, errors.Join(auth.ErrOAuthTemporarilyUnavailable, err)
	}

	return accessToken, "", storedToken.Scopes, nil
}

func (h *Handler) loadAgentRuntimeRefreshTokenForExchange(ctx context.Context, refreshToken, clientID string, telemetry *oauthGrantTelemetry) (*storage.RefreshToken, error) {
	storedToken, err := h.repos.Account().GetRefreshToken(ctx, refreshToken)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			telemetry.setReason(oauthGrantReasonRefreshTokenAbsent)
			return nil, auth.ErrInvalidToken
		}
		telemetry.setReason(oauthGrantReasonRefreshRotationInfrastructure)
		return nil, errors.Join(auth.ErrOAuthTemporarilyUnavailable, err)
	}
	if storedToken == nil {
		telemetry.setReason(oauthGrantReasonRefreshTokenAbsent)
		return nil, auth.ErrInvalidToken
	}
	if telemetry != nil {
		telemetry.FamilyID = strings.TrimSpace(storedToken.FamilyID)
		telemetry.setResourceIfEmpty(storedToken.Resource)
	}
	if storedToken.ClientID != clientID {
		telemetry.setReason(oauthGrantReasonRefreshRuntimeInvalid)
		return nil, auth.ErrInvalidToken
	}
	if !auth.IsAgentRuntimeRefreshToken(storedToken) {
		telemetry.setReason(oauthGrantReasonRefreshRuntimeInvalid)
		return nil, auth.ErrInvalidToken
	}
	return storedToken, nil
}

// HandleListAgentRuntimeSessionsLift lists dedicated internal runtime refresh sessions for an agent.
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

// HandleRevokeAgentRuntimeSessionLift revokes one dedicated internal runtime session family for an agent.
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
