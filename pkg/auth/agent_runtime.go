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

// Dedicated internal runtime client IDs. These are not part of the canonical public MCP client contract.
const (
	DelegatedAgentRuntimeClientID     = "lesser-agent-delegation"
	SelfSovereignAgentRuntimeClientID = "lesser-agent-self-sovereign"
)

// AgentRuntimeRefreshIdleTTL is the sliding inactivity window for agent runtime refresh sessions.
const AgentRuntimeRefreshIdleTTL = RefreshTokenDuration

// AgentRuntimeRefreshAbsoluteTTL caps the total lifetime of an agent runtime refresh session.
const AgentRuntimeRefreshAbsoluteTTL = RefreshTokenFamilyExpiry

// AgentRuntimeAuthStatus summarizes the latest persisted auth state for a runtime session.
type AgentRuntimeAuthStatus string

// Runtime session auth status values exposed through operator diagnostics.
const (
	AgentRuntimeAuthStatusHealthy AgentRuntimeAuthStatus = "HEALTHY"
	AgentRuntimeAuthStatusFailed  AgentRuntimeAuthStatus = "FAILED"
	AgentRuntimeAuthStatusExpired AgentRuntimeAuthStatus = "EXPIRED"
	AgentRuntimeAuthStatusRevoked AgentRuntimeAuthStatus = "REVOKED"
)

// AgentRuntimeAuthDiagnostic carries the latest persisted auth result for operator diagnostics.
type AgentRuntimeAuthDiagnostic struct {
	Status         AgentRuntimeAuthStatus
	FailureCode    string
	FailureMessage string
	FailureAt      time.Time
	LastSuccessAt  time.Time
}

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
	case DelegatedAgentRuntimeClientID, SelfSovereignAgentRuntimeClientID:
		return true
	default:
		return false
	}
}

// AgentRuntimeClientIDs returns the dedicated internal runtime client IDs that participate in
// runtime-session diagnostics and family rotation.
func AgentRuntimeClientIDs() []string {
	return []string{
		DelegatedAgentRuntimeClientID,
		SelfSovereignAgentRuntimeClientID,
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
		LastAuthSuccessAt: now,
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

// IsAgentRuntimeRefreshToken reports whether a stored refresh token belongs to the dedicated
// internal runtime session model rather than ordinary public or compatibility OAuth rotation.
func IsAgentRuntimeRefreshToken(token *storage.RefreshToken) bool {
	if token == nil || !IsAgentRuntimeClientID(token.ClientID) || strings.TrimSpace(token.SessionID) == "" {
		return false
	}

	return strings.TrimSpace(token.FamilyID) != "" ||
		token.Generation > 0 ||
		token.Current ||
		token.DeviceLabel != "" ||
		!token.LastUsedAt.IsZero() ||
		!token.IdleExpiresAt.IsZero() ||
		!token.AbsoluteExpiresAt.IsZero() ||
		!token.SessionCreatedAt.IsZero() ||
		token.AccessTTLSeconds > 0 ||
		!token.ReuseDetectedAt.IsZero() ||
		token.ReuseDetectedFromIP != "" ||
		token.ReuseDetectedFromUA != "" ||
		!token.LastAuthFailureAt.IsZero() ||
		token.LastAuthFailureCode != "" ||
		token.LastAuthFailureMsg != "" ||
		!token.LastAuthSuccessAt.IsZero()
}

// ListAgentRuntimeSessions returns the current runtime refresh session records for an agent.
func ListAgentRuntimeSessions(ctx context.Context, repos StorageProvider, username string) ([]storage.RefreshToken, error) {
	if repos == nil || repos.Account() == nil {
		return nil, ErrSessionStorage
	}

	currentBySessionID := map[string]storage.RefreshToken{}

	for _, clientID := range AgentRuntimeClientIDs() {
		if err := collectCurrentAgentSessionsForClient(ctx, repos, username, clientID, currentBySessionID); err != nil {
			return nil, err
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

func recordCurrentAgentSession(currentBySessionID map[string]storage.RefreshToken, token storage.RefreshToken) {
	if !token.Current || !IsAgentRuntimeRefreshToken(&token) {
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

// RuntimeSessionAuthDiagnostic builds the operator-facing auth diagnostic for a runtime session.
func RuntimeSessionAuthDiagnostic(now time.Time, token storage.RefreshToken) AgentRuntimeAuthDiagnostic {
	diagnostic := AgentRuntimeAuthDiagnostic{
		Status:         AgentRuntimeAuthStatusHealthy,
		FailureCode:    strings.TrimSpace(token.LastAuthFailureCode),
		FailureMessage: strings.TrimSpace(token.LastAuthFailureMsg),
		FailureAt:      token.LastAuthFailureAt,
		LastSuccessAt:  token.LastAuthSuccessAt,
	}

	switch {
	case token.Revoked && token.RevokedReason == "runtime_session_expired":
		diagnostic.Status = AgentRuntimeAuthStatusExpired
	case token.Revoked:
		diagnostic.Status = AgentRuntimeAuthStatusRevoked
	case runtimeSessionExpired(now, token):
		diagnostic.Status = AgentRuntimeAuthStatusExpired
	case !diagnostic.FailureAt.IsZero() && (diagnostic.LastSuccessAt.IsZero() || diagnostic.FailureAt.After(diagnostic.LastSuccessAt)):
		diagnostic.Status = AgentRuntimeAuthStatusFailed
	}

	return diagnostic
}

// RecordAgentRuntimeAuthFailure persists the latest auth failure across a runtime session family.
func RecordAgentRuntimeAuthFailure(ctx context.Context, repos StorageProvider, token *storage.RefreshToken, code, message string, eventAt time.Time) error {
	return recordAgentRuntimeAuthEvent(ctx, repos, token, false, eventAt, code, message)
}

// RecordAgentRuntimeAuthSuccess persists the latest successful token exchange/refresh for a runtime session family.
func RecordAgentRuntimeAuthSuccess(ctx context.Context, repos StorageProvider, token *storage.RefreshToken, eventAt time.Time) error {
	return recordAgentRuntimeAuthEvent(ctx, repos, token, true, eventAt, "", "")
}

func recordAgentRuntimeAuthEvent(ctx context.Context, repos StorageProvider, token *storage.RefreshToken, success bool, eventAt time.Time, code, message string) error {
	if repos == nil || repos.Account() == nil {
		return ErrSessionStorage
	}
	if token == nil {
		return nil
	}
	if eventAt.IsZero() {
		eventAt = time.Now().UTC()
	}
	if !IsAgentRuntimeRefreshToken(token) {
		if success {
			token.LastAuthSuccessAt = eventAt
			return nil
		}

		token.LastAuthFailureCode = strings.TrimSpace(code)
		token.LastAuthFailureMsg = strings.TrimSpace(message)
		token.LastAuthFailureAt = eventAt
		return nil
	}

	targets, err := listAgentRuntimeEventTargets(ctx, repos, token)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		targets = []storage.RefreshToken{*token}
	}

	for i := range targets {
		target := targets[i]
		if success {
			target.LastAuthSuccessAt = eventAt
		} else {
			target.LastAuthFailureCode = strings.TrimSpace(code)
			target.LastAuthFailureMsg = strings.TrimSpace(message)
			target.LastAuthFailureAt = eventAt
		}
		if err := repos.Account().UpdateRefreshToken(ctx, &target); err != nil {
			return err
		}
	}

	if success {
		token.LastAuthSuccessAt = eventAt
		return nil
	}

	token.LastAuthFailureCode = strings.TrimSpace(code)
	token.LastAuthFailureMsg = strings.TrimSpace(message)
	token.LastAuthFailureAt = eventAt
	return nil
}

func listAgentRuntimeEventTargets(ctx context.Context, repos StorageProvider, token *storage.RefreshToken) ([]storage.RefreshToken, error) {
	if !IsAgentRuntimeRefreshToken(token) {
		return nil, nil
	}
	switch {
	case token == nil:
		return nil, nil
	case strings.TrimSpace(token.FamilyID) != "":
		return repos.Account().ListRefreshTokensByFamily(ctx, token.FamilyID)
	case strings.TrimSpace(token.SessionID) != "":
		return repos.Account().ListRefreshTokensBySession(ctx, token.SessionID)
	default:
		return nil, nil
	}
}

func runtimeSessionExpired(now time.Time, token storage.RefreshToken) bool {
	return (!token.IdleExpiresAt.IsZero() && now.After(token.IdleExpiresAt)) ||
		(!token.AbsoluteExpiresAt.IsZero() && now.After(token.AbsoluteExpiresAt))
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
