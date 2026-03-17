package auth

import (
	"context"
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
