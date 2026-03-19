package auth

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
)

// DefaultAgentRuntimeDeviceLabel is used when a runtime does not provide its own label.
const DefaultAgentRuntimeDeviceLabel = "local-agent"

// AgentRuntimeRefreshIdleTTL is the sliding inactivity window for agent runtime refresh sessions.
const AgentRuntimeRefreshIdleTTL = RefreshTokenDuration

// AgentRuntimeRefreshAbsoluteTTL caps the total lifetime of an agent runtime refresh session.
const AgentRuntimeRefreshAbsoluteTTL = RefreshTokenFamilyExpiry

// AgentRuntimeTokenIssueParams describes a first-class bearer + refresh runtime session.
type AgentRuntimeTokenIssueParams struct {
	Username    string
	ClientID    string
	Scopes      []string
	AccessTTL   time.Duration
	DeviceLabel string
}

// AgentRuntimeTokenBundle contains the issued OAuth tokens and stored refresh-session metadata.
type AgentRuntimeTokenBundle struct {
	AccessToken  string
	RefreshToken string
	Session      storage.RefreshToken
}

// IsAgentRuntimeClientID reports whether a client ID belongs to Lesser's long-lived agent runtime flows.
func IsAgentRuntimeClientID(clientID string) bool {
	switch strings.TrimSpace(clientID) {
	case "lesser-agent-delegation", "lesser-agent-self-sovereign":
		return true
	default:
		return false
	}
}

// CoalesceAgentRuntimeLabel returns a stable label for operator-visible runtime sessions.
func CoalesceAgentRuntimeLabel(primary, fallback string) string {
	label := strings.TrimSpace(primary)
	if label == "" {
		label = strings.TrimSpace(fallback)
	}
	if label == "" {
		label = DefaultAgentRuntimeDeviceLabel
	}
	return label
}

// IssueAgentRuntimeTokens mints an access + refresh pair backed by refresh-session metadata that can
// be safely refreshed by long-lived local runtimes without browser state.
func IssueAgentRuntimeTokens(ctx context.Context, cfg *config.Config, repos StorageProvider, params AgentRuntimeTokenIssueParams) (*AgentRuntimeTokenBundle, error) {
	if cfg == nil || repos == nil || repos.Account() == nil {
		return nil, ErrSessionStorage
	}

	accessTTL := params.AccessTTL
	if accessTTL <= 0 {
		accessTTL = AgentAccessTokenTTL(cfg)
	}

	now := time.Now().UTC()
	sessionID := common.GenerateSessionIDULID()
	familyID := common.GenerateSessionIDULID()
	deviceLabel := strings.TrimSpace(params.DeviceLabel)
	if deviceLabel == "" {
		deviceLabel = DefaultAgentRuntimeDeviceLabel
	}

	idleExpiry := now.Add(AgentRuntimeRefreshIdleTTL)
	absoluteExpiry := now.Add(AgentRuntimeRefreshAbsoluteTTL)
	if idleExpiry.After(absoluteExpiry) {
		idleExpiry = absoluteExpiry
	}

	oauthSvc := NewOAuthService(cfg.JWTSecret, cfg, repos, nil)
	accessToken, refreshToken, err := oauthSvc.GenerateTokensWithAccessTokenTTLAndClientContext(
		ctx,
		params.Username,
		params.ClientID,
		"",
		params.Scopes,
		accessTTL,
		ClientClassAgent,
		sessionID,
	)
	if err != nil {
		return nil, err
	}

	refreshRecord := storage.RefreshToken{
		Token:             refreshToken,
		Username:          params.Username,
		ClientID:          params.ClientID,
		Scopes:            params.Scopes,
		CreatedAt:         now,
		ExpiresAt:         idleExpiry,
		ClientClass:       ClientClassAgent,
		SessionID:         sessionID,
		FamilyID:          familyID,
		Generation:        1,
		Current:           true,
		DeviceLabel:       deviceLabel,
		LastUsedAt:        now,
		IdleExpiresAt:     idleExpiry,
		AbsoluteExpiresAt: absoluteExpiry,
		SessionCreatedAt:  now,
		AccessTTLSeconds:  int(accessTTL.Seconds()),
	}
	if err := repos.Account().CreateRefreshToken(ctx, &refreshRecord); err != nil {
		return nil, err
	}

	return &AgentRuntimeTokenBundle{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		Session:      refreshRecord,
	}, nil
}

// ListAgentRuntimeSessions returns the current runtime refresh session records for an agent.
func ListAgentRuntimeSessions(ctx context.Context, repos StorageProvider, username string) ([]storage.RefreshToken, error) {
	if repos == nil || repos.Account() == nil {
		return nil, ErrSessionStorage
	}

	currentBySessionID := map[string]storage.RefreshToken{}

	for _, clientID := range []string{"lesser-agent-delegation", "lesser-agent-self-sovereign"} {
		if err := collectCurrentAgentSessionsForClient(ctx, repos, username, clientID, currentBySessionID); err != nil {
			return nil, err
		}
	}

	if err := collectCurrentAgentConnectorSessions(ctx, repos, username, currentBySessionID); err != nil {
		return nil, err
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

func recordCurrentAgentSession(currentBySessionID map[string]storage.RefreshToken, token storage.RefreshToken) {
	if strings.TrimSpace(token.SessionID) == "" || !token.Current {
		return
	}
	existing, ok := currentBySessionID[token.SessionID]
	if !ok || token.Generation >= existing.Generation {
		currentBySessionID[token.SessionID] = token
	}
}

func collectCurrentAgentSessionsForClient(ctx context.Context, repos StorageProvider, username, clientID string, currentBySessionID map[string]storage.RefreshToken) error {
	tokens, err := repos.Account().ListRefreshTokensByUserClient(ctx, username, clientID)
	if err != nil {
		return err
	}
	for _, token := range tokens {
		recordCurrentAgentSession(currentBySessionID, token)
	}
	return nil
}

func collectCurrentAgentConnectorSessions(ctx context.Context, repos StorageProvider, username string, currentBySessionID map[string]storage.RefreshToken) error {
	cursor := ""
	for {
		clients, nextCursor, err := repos.Account().ListOAuthClients(ctx, 100, cursor)
		if err != nil {
			return err
		}
		for _, client := range clients {
			if !isAgentConnectorClientForUsername(client, username) {
				continue
			}
			if err := collectCurrentAgentSessionsForClient(ctx, repos, username, client.ClientID, currentBySessionID); err != nil {
				return err
			}
		}
		if strings.TrimSpace(nextCursor) == "" {
			return nil
		}
		cursor = nextCursor
	}
}

func isAgentConnectorClientForUsername(client *storage.OAuthClient, username string) bool {
	if client == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(client.ClientClass), ClientClassAgent) {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(client.AgentUsername), username)
}

// RevokeAgentRuntimeFamily revokes all refresh tokens in a runtime session family.
func RevokeAgentRuntimeFamily(ctx context.Context, repos StorageProvider, token *storage.RefreshToken, reason, ipAddress, userAgent string) error {
	if repos == nil || repos.Account() == nil {
		return ErrSessionStorage
	}
	if token == nil || strings.TrimSpace(token.FamilyID) == "" {
		return nil
	}

	familyTokens, err := repos.Account().ListRefreshTokensByFamily(ctx, token.FamilyID)
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
				if err := repos.Account().UpdateRefreshToken(ctx, &familyToken); err != nil {
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
		if err := repos.Account().UpdateRefreshToken(ctx, &familyToken); err != nil {
			return err
		}
	}

	return nil
}

// RevokeAgentRuntimeSession revokes one runtime session without affecting unrelated sessions.
func RevokeAgentRuntimeSession(ctx context.Context, repos StorageProvider, username, sessionID, reason, ipAddress, userAgent string) error {
	sessions, err := ListAgentRuntimeSessions(ctx, repos, username)
	if err != nil {
		return err
	}
	for i := range sessions {
		if sessions[i].SessionID == sessionID {
			return RevokeAgentRuntimeFamily(ctx, repos, &sessions[i], reason, ipAddress, userAgent)
		}
	}
	return storage.ErrNotFound
}

// RevokeAllAgentRuntimeSessions revokes all current runtime sessions for an agent.
func RevokeAllAgentRuntimeSessions(ctx context.Context, repos StorageProvider, username, reason, ipAddress, userAgent string) error {
	sessions, err := ListAgentRuntimeSessions(ctx, repos, username)
	if err != nil {
		return err
	}
	for i := range sessions {
		if err := RevokeAgentRuntimeFamily(ctx, repos, &sessions[i], reason, ipAddress, userAgent); err != nil {
			return err
		}
	}
	return nil
}
