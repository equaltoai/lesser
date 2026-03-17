package handlers

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

func isAgentRuntimeClientID(clientID string) bool {
	switch strings.TrimSpace(clientID) {
	case delegatedAgentClientID, selfSovereignAgentClientID:
		return true
	default:
		return false
	}
}

func coalesceAgentRuntimeLabel(primary, fallback string) string {
	label := strings.TrimSpace(primary)
	if label == "" {
		label = strings.TrimSpace(fallback)
	}
	if label == "" {
		label = auth.DefaultAgentRuntimeDeviceLabel
	}
	return label
}

func agentRuntimeSessionFromToken(token storage.RefreshToken) apimodels.AgentRuntimeSession {
	var revokedAt *time.Time
	if !token.RevokedAt.IsZero() {
		value := token.RevokedAt
		revokedAt = &value
	}

	return apimodels.AgentRuntimeSession{
		SessionID:         token.SessionID,
		ClientID:          token.ClientID,
		DeviceLabel:       coalesceAgentRuntimeLabel(token.DeviceLabel, ""),
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

func (h *Handler) listAgentRuntimeSessions(ctx context.Context, username string) ([]storage.RefreshToken, error) {
	if h == nil || h.repos == nil || h.repos.Account() == nil {
		return nil, auth.ErrSessionStorage
	}

	currentBySessionID := map[string]storage.RefreshToken{}
	for _, clientID := range []string{delegatedAgentClientID, selfSovereignAgentClientID} {
		tokens, err := h.repos.Account().ListRefreshTokensByUserClient(ctx, username, clientID)
		if err != nil {
			return nil, err
		}
		for _, token := range tokens {
			if strings.TrimSpace(token.SessionID) == "" || !token.Current {
				continue
			}
			existing, ok := currentBySessionID[token.SessionID]
			if !ok || token.Generation >= existing.Generation {
				currentBySessionID[token.SessionID] = token
			}
		}
	}

	out := make([]storage.RefreshToken, 0, len(currentBySessionID))
	for _, token := range currentBySessionID {
		out = append(out, token)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].LastUsedAt.After(out[j].LastUsedAt)
	})
	return out, nil
}

func (h *Handler) revokeAgentRuntimeFamily(ctx context.Context, token *storage.RefreshToken, reason, ipAddress, userAgent string) error {
	if token == nil || strings.TrimSpace(token.FamilyID) == "" {
		return nil
	}

	familyTokens, err := h.repos.Account().ListRefreshTokensByFamily(ctx, token.FamilyID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	for i := range familyTokens {
		familyToken := familyTokens[i]
		if familyToken.Revoked {
			if familyToken.ReuseDetectedAt.IsZero() {
				familyToken.ReuseDetectedAt = now
				familyToken.ReuseDetectedFromIP = ipAddress
				familyToken.ReuseDetectedFromUA = userAgent
				if err := h.repos.Account().UpdateRefreshToken(ctx, &familyToken); err != nil {
					return err
				}
			}
			continue
		}
		familyToken.Revoked = true
		familyToken.RevokedAt = now
		familyToken.RevokedReason = reason
		familyToken.ReuseDetectedAt = now
		familyToken.ReuseDetectedFromIP = ipAddress
		familyToken.ReuseDetectedFromUA = userAgent
		if err := h.repos.Account().UpdateRefreshToken(ctx, &familyToken); err != nil {
			return err
		}
	}

	return nil
}

func (h *Handler) revokeAgentRuntimeSession(ctx context.Context, username, sessionID, reason, ipAddress, userAgent string) error {
	sessions, err := h.listAgentRuntimeSessions(ctx, username)
	if err != nil {
		return err
	}
	for i := range sessions {
		if sessions[i].SessionID == sessionID {
			return h.revokeAgentRuntimeFamily(ctx, &sessions[i], reason, ipAddress, userAgent)
		}
	}
	return storage.ErrNotFound
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
	for _, bound := range []time.Duration{
		time.Until(token.IdleExpiresAt),
		time.Until(token.AbsoluteExpiresAt),
	} {
		if bound > 0 && bound < ttl {
			ttl = bound
		}
	}
	return ttl
}

func nextRuntimeIdleExpiry(now, absoluteExpiry time.Time) time.Time {
	idleExpiry := now.Add(auth.AgentRuntimeRefreshIdleTTL)
	if !absoluteExpiry.IsZero() && idleExpiry.After(absoluteExpiry) {
		return absoluteExpiry
	}
	return idleExpiry
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
		if err := h.revokeAgentRuntimeFamily(ctx, storedToken, "refresh_token_reuse_detected", ipAddress, userAgent); err != nil {
			h.logger.Warn("failed to revoke runtime session family after refresh reuse", zap.Error(err))
		}
		return "", "", nil, auth.ErrInvalidToken
	}
	if now.After(storedToken.IdleExpiresAt) || now.After(storedToken.AbsoluteExpiresAt) {
		_ = h.revokeAgentRuntimeFamily(ctx, storedToken, "runtime_session_expired", ipAddress, userAgent)
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
		return "", "", nil, err
	}

	newToken := &storage.RefreshToken{
		Token:             newRefreshToken,
		Username:          storedToken.Username,
		ClientID:          clientID,
		Scopes:            storedToken.Scopes,
		CreatedAt:         now,
		ExpiresAt:         nextRuntimeIdleExpiry(now, storedToken.AbsoluteExpiresAt),
		ClientClass:       auth.ClientClassAgent,
		SessionID:         storedToken.SessionID,
		FamilyID:          storedToken.FamilyID,
		Generation:        storedToken.Generation + 1,
		Current:           true,
		DeviceLabel:       storedToken.DeviceLabel,
		LastUsedAt:        now,
		IdleExpiresAt:     nextRuntimeIdleExpiry(now, storedToken.AbsoluteExpiresAt),
		AbsoluteExpiresAt: storedToken.AbsoluteExpiresAt,
		SessionCreatedAt:  storedToken.SessionCreatedAt,
		AccessTTLSeconds:  storedToken.AccessTTLSeconds,
	}
	if err := h.repos.Account().CreateRefreshToken(ctx, newToken); err != nil {
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

	sessions, err := h.listAgentRuntimeSessions(ctx.Context(), username)
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
	if err := h.revokeAgentRuntimeSession(ctx.Context(), username, sessionID, reason, ipAddress, userAgent); err != nil {
		if err == storage.ErrNotFound {
			return common.RespondNotFound(ctx, "runtime session")
		}
		return common.RespondInternalServerError(ctx)
	}

	sessions, err := h.listAgentRuntimeSessions(ctx.Context(), username)
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
