package auth

import (
	"context"
	"errors"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	"go.uber.org/zap"
)

// SessionLifecycleManager handles session creation, refresh, and cleanup
type SessionLifecycleManager struct {
	sessionManager  *SessionManager
	securityManager *SessionSecurityManager
	repos           StorageProvider
	logger          *zap.Logger
	config          *SessionLifecycleConfig
}

// SessionLifecycleConfig holds lifecycle management configuration
type SessionLifecycleConfig struct {
	// Session durations
	SessionDuration      time.Duration // Default session lifetime
	MaxSessionDuration   time.Duration // Maximum session lifetime (hard limit)
	InactivityTimeout    time.Duration // Auto-logout after inactivity
	RefreshTokenDuration time.Duration // Refresh token lifetime

	// Cleanup settings
	CleanupInterval           time.Duration // How often to run cleanup
	ExpiredSessionGracePeriod time.Duration // Grace period for expired sessions
	MaxInactiveSessions       int           // Max inactive sessions per user

	// Security settings
	RequireRefreshRotation    bool // Rotate refresh tokens on use
	SessionFixationPrevention bool // Regenerate session IDs on login
	ConcurrentSessionLimit    int  // Max concurrent sessions per user

	// Extension policies
	AllowSessionExtension bool          // Allow extending session lifetime
	ExtensionThreshold    time.Duration // When to extend sessions automatically
	MaxSessionExtensions  int           // Max extensions per session
}

// DefaultSessionLifecycleConfig provides secure defaults
func DefaultSessionLifecycleConfig() *SessionLifecycleConfig {
	return &SessionLifecycleConfig{
		SessionDuration:           7 * 24 * time.Hour,  // 7 days
		MaxSessionDuration:        30 * 24 * time.Hour, // 30 days max
		InactivityTimeout:         24 * time.Hour,      // 24 hours
		RefreshTokenDuration:      30 * 24 * time.Hour, // 30 days
		CleanupInterval:           1 * time.Hour,       // Hourly cleanup
		ExpiredSessionGracePeriod: 15 * time.Minute,    // 15 minute grace
		MaxInactiveSessions:       5,                   // 5 inactive sessions
		RequireRefreshRotation:    true,
		SessionFixationPrevention: true,
		ConcurrentSessionLimit:    10,
		AllowSessionExtension:     true,
		ExtensionThreshold:        6 * time.Hour, // Extend when < 6 hours left
		MaxSessionExtensions:      3,             // Max 3 extensions
	}
}

// SessionExtension represents a session extension event
type SessionExtension struct {
	SessionID      string        `json:"session_id"`
	ExtendedBy     time.Duration `json:"extended_by"`
	ExtendedAt     time.Time     `json:"extended_at"`
	Reason         string        `json:"reason"`
	ExtensionCount int           `json:"extension_count"`
}

// NewSessionLifecycleManager creates a new session lifecycle manager
func NewSessionLifecycleManager(sessionManager *SessionManager, securityManager *SessionSecurityManager, repos StorageProvider, logger *zap.Logger, config *SessionLifecycleConfig) *SessionLifecycleManager {
	if config == nil {
		config = DefaultSessionLifecycleConfig()
	}

	return &SessionLifecycleManager{
		sessionManager:  sessionManager,
		securityManager: securityManager,
		repos:           repos,
		logger:          logger,
		config:          config,
	}
}

// CreateSessionWithLifecycle creates a new session with full lifecycle management
func (slm *SessionLifecycleManager) CreateSessionWithLifecycle(ctx context.Context, username, deviceName, userAgent, ipAddress, authMethod string) (*Session, error) {
	// Check concurrent session limits
	if err := slm.enforceConcurrentSessionLimits(ctx, username); err != nil {
		slm.logger.Error("concurrent session limit exceeded", zap.String("username", username), zap.Error(err))
		return nil, errors.Join(ErrConcurrentSessionLimitExceeded, err)
	}

	// Create device fingerprint for security tracking
	fingerprint := slm.securityManager.GenerateDeviceFingerprint(userAgent, ipAddress, "")

	// Check for high-risk user agents
	if slm.securityManager.IsHighRiskUserAgent(userAgent) {
		slm.securityManager.LogSecurityEvent("high_risk_user_agent", "", username, "High-risk user agent detected", map[string]interface{}{
			"user_agent": userAgent,
			"ip_address": ipAddress,
		})
	}

	// Create the session using the session manager
	session, err := slm.sessionManager.CreateSession(ctx, username, deviceName, userAgent, ipAddress, authMethod)
	if err != nil {
		slm.logger.Error("failed to create session", zap.String("username", username), zap.String("authMethod", authMethod), zap.Error(err))
		return nil, errors.Join(ErrSessionCreationFailed, err)
	}

	// Apply session fixation prevention if enabled
	if slm.config.SessionFixationPrevention {
		newSessionID, err := slm.securityManager.PreventSessionFixation(session.SessionID)
		if err != nil {
			slm.logger.Warn("failed to prevent session fixation", zap.Error(err))
		} else if newSessionID != session.SessionID {
			session.SessionID = newSessionID
			// Update session in storage with new ID
			if err := slm.updateSessionInStorage(ctx, session); err != nil {
				slm.logger.Error("failed to update session with new ID", zap.Error(err))
			}
		}
	}

	// Log session creation
	slm.logger.Info("session created with lifecycle management",
		zap.String("sessionID", session.SessionID),
		zap.String("username", username),
		zap.String("authMethod", authMethod),
		zap.String("deviceFingerprint", fingerprint.Fingerprint))

	return session, nil
}

// RefreshSessionWithRotation refreshes a session with optional token rotation
func (slm *SessionLifecycleManager) RefreshSessionWithRotation(ctx context.Context, refreshToken string, ipAddress, userAgent string) (*Session, string, error) {
	// Validate the refresh token
	session, err := slm.sessionManager.ValidateRefreshToken(ctx, refreshToken)
	if err != nil {
		slm.logger.Error("invalid refresh token provided", zap.Error(err))
		return nil, "", errors.Join(ErrInvalidRefreshTokenProvided, err)
	}

	// Generate device fingerprint for security validation
	currentFingerprint := slm.securityManager.GenerateDeviceFingerprint(userAgent, ipAddress, "")

	// Perform security validation
	securityResult, err := slm.securityManager.ValidateSessionSecurity(ctx, session, currentFingerprint)
	if err != nil {
		slm.logger.Error("session security validation failed", zap.String("sessionID", session.SessionID), zap.Error(err))
		return nil, "", errors.Join(ErrSessionSecurityCheckFailed, err)
	}

	if !securityResult.Valid {
		slm.securityManager.LogSecurityEvent("session_security_failure", session.SessionID, session.Username, "Session failed security validation", map[string]interface{}{
			"risk_factors": securityResult.RiskFactors,
			"trust_score":  securityResult.TrustScore,
		})
		return nil, "", ErrSessionSecurityValidationFailed
	}

	// Check if session can be extended
	if !slm.canExtendSession(session) {
		return nil, "", ErrSessionMaxLifetimeReached
	}

	// Rotate refresh token if required
	var newRefreshToken string
	if slm.config.RequireRefreshRotation {
		newRefreshToken, err = slm.sessionManager.RotateRefreshToken(ctx, session)
		if err != nil {
			slm.logger.Error("failed to rotate refresh token", zap.String("sessionID", session.SessionID), zap.Error(err))
			return nil, "", errors.Join(ErrRefreshTokenRotationFailed, err)
		}
	} else {
		newRefreshToken = refreshToken
	}

	// Update session activity
	if err := slm.sessionManager.UpdateSessionActivity(ctx, session.SessionID, ipAddress); err != nil {
		slm.logger.Warn("failed to update session activity", zap.Error(err))
	}

	// Extend session if needed
	if slm.shouldExtendSession(session) {
		if err := slm.extendSession(ctx, session); err != nil {
			slm.logger.Warn("failed to extend session", zap.Error(err))
		}
	}

	slm.logger.Debug("session refreshed successfully",
		zap.String("sessionID", session.SessionID),
		zap.String("username", session.Username),
		zap.Bool("tokenRotated", slm.config.RequireRefreshRotation))

	return session, newRefreshToken, nil
}

// ExtendSession extends a session's lifetime
func (slm *SessionLifecycleManager) extendSession(ctx context.Context, session *Session) error {
	if !slm.config.AllowSessionExtension {
		return ErrSessionExtensionDisabled
	}

	if !slm.canExtendSession(session) {
		return ErrSessionCannotBeExtended
	}

	// Calculate extension duration
	extensionDuration := slm.config.SessionDuration / 2 // Extend by half the session duration
	newExpiry := session.ExpiresAt.Add(extensionDuration)

	// Enforce maximum session duration
	maxExpiry := session.CreatedAt.Add(slm.config.MaxSessionDuration)
	if newExpiry.After(maxExpiry) {
		newExpiry = maxExpiry
	}

	// Update session expiry
	session.ExpiresAt = newExpiry

	// Log extension
	extension := &SessionExtension{
		SessionID:      session.SessionID,
		ExtendedBy:     extensionDuration,
		ExtendedAt:     time.Now(),
		Reason:         "automatic_extension",
		ExtensionCount: slm.getSessionExtensionCount(session) + 1,
	}

	slm.logger.Info("session extended",
		zap.String("sessionID", session.SessionID),
		zap.Duration("extendedBy", extension.ExtendedBy),
		zap.Time("newExpiry", newExpiry))

	return slm.updateSessionInStorage(ctx, session)
}

// shouldExtendSession determines if a session should be automatically extended
func (slm *SessionLifecycleManager) shouldExtendSession(session *Session) bool {
	if !slm.config.AllowSessionExtension {
		return false
	}

	// Check if session is close to expiry
	timeUntilExpiry := time.Until(session.ExpiresAt)
	return timeUntilExpiry < slm.config.ExtensionThreshold
}

// canExtendSession checks if a session can be extended
func (slm *SessionLifecycleManager) canExtendSession(session *Session) bool {
	// Check if we've reached the maximum session duration
	maxExpiry := session.CreatedAt.Add(slm.config.MaxSessionDuration)
	if time.Now().After(maxExpiry) {
		return false
	}

	// Check extension count (would need to be tracked in session)
	extensionCount := slm.getSessionExtensionCount(session)
	return extensionCount < slm.config.MaxSessionExtensions
}

// getSessionExtensionCount gets the number of times a session has been extended
func (slm *SessionLifecycleManager) getSessionExtensionCount(_ *Session) int {
	// This would typically be stored in the session context or a separate field
	// For now, return 0 - implement based on your session storage schema
	return 0
}

// CleanupExpiredSessions removes expired and inactive sessions
func (slm *SessionLifecycleManager) CleanupExpiredSessions(_ context.Context) error {
	slm.logger.Info("starting session cleanup")

	// This would typically scan for expired sessions and remove them
	// Implementation depends on your storage backend capabilities

	// For DynamoDB, TTL handles most cleanup automatically
	// But we can also manually clean up sessions that should be removed immediately

	// Placeholder implementation - in production, this would:
	// 1. Query for expired sessions
	// 2. Query for inactive sessions beyond threshold
	// 3. Remove sessions exceeding user limits
	// 4. Clean up orphaned session data

	slm.logger.Info("session cleanup completed")
	return nil
}

// enforceConcurrentSessionLimits ensures a user doesn't exceed session limits
func (slm *SessionLifecycleManager) enforceConcurrentSessionLimits(ctx context.Context, username string) error {
	sessions, err := slm.sessionManager.repos.Account().GetUserSessions(ctx, username)
	if err != nil {
		slm.logger.Error("failed to get user sessions", zap.String("username", username), zap.Error(err))
		return errors.Join(ErrUserSessionsRetrieval, err)
	}

	// Count active sessions
	activeCount := 0
	now := time.Now()
	for _, session := range sessions {
		if now.Before(session.ExpiresAt) {
			activeCount++
		}
	}

	if activeCount >= slm.config.ConcurrentSessionLimit {
		// Remove oldest session to make room
		if err := slm.removeOldestSession(ctx, username, sessions); err != nil {
			slm.logger.Error("failed to remove oldest session", zap.String("username", username), zap.Error(err))
			return errors.Join(ErrOldestSessionRemoval, err)
		}
	}

	return nil
}

// removeOldestSession removes the oldest session for a user
func (slm *SessionLifecycleManager) removeOldestSession(ctx context.Context, username string, sessions []*storage.Session) error {
	if err := common.ValidateSliceNotEmpty("sessions", sessions); err != nil {
		return nil
	}

	// Find oldest session
	oldest := sessions[0]
	for _, session := range sessions {
		if session.CreatedAt.Before(oldest.CreatedAt) {
			oldest = session
		}
	}

	slm.logger.Info("removing oldest session to enforce limits",
		zap.String("username", username),
		zap.String("sessionID", oldest.SessionID),
		zap.Time("createdAt", oldest.CreatedAt))

	return slm.sessionManager.RevokeSession(ctx, oldest.SessionID)
}

// updateSessionInStorage updates a session in storage
func (slm *SessionLifecycleManager) updateSessionInStorage(ctx context.Context, session *Session) error {
	// Update session using the session manager
	return slm.sessionManager.repos.Account().UpdateSession(
		ctx,
		session.SessionID,
		session.RefreshToken,
		session.IPAddress,
		session.LastActivity,
		session.ExpiresAt,
	)
}

// GetSessionHealth returns health information about a session
func (slm *SessionLifecycleManager) GetSessionHealth(ctx context.Context, sessionID string) (*SessionHealth, error) {
	session, err := slm.sessionManager.repos.Account().GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	health := &SessionHealth{
		SessionID:        session.SessionID,
		Username:         session.Username,
		CreatedAt:        session.CreatedAt,
		LastActivity:     session.LastActivity,
		ExpiresAt:        session.ExpiresAt,
		IsActive:         time.Now().Before(session.ExpiresAt),
		TimeUntilExpiry:  time.Until(session.ExpiresAt),
		InactivityTime:   time.Since(session.LastActivity),
		CanBeExtended:    slm.canExtendSession(session),
		ShouldBeExtended: slm.shouldExtendSession(session),
		ExtensionCount:   slm.getSessionExtensionCount(session),
	}

	return health, nil
}

// SessionHealth represents the health status of a session
type SessionHealth struct {
	SessionID        string        `json:"session_id"`
	Username         string        `json:"username"`
	CreatedAt        time.Time     `json:"created_at"`
	LastActivity     time.Time     `json:"last_activity"`
	ExpiresAt        time.Time     `json:"expires_at"`
	IsActive         bool          `json:"is_active"`
	TimeUntilExpiry  time.Duration `json:"time_until_expiry"`
	InactivityTime   time.Duration `json:"inactivity_time"`
	CanBeExtended    bool          `json:"can_be_extended"`
	ShouldBeExtended bool          `json:"should_be_extended"`
	ExtensionCount   int           `json:"extension_count"`
}

// ScheduleCleanup schedules periodic session cleanup
func (slm *SessionLifecycleManager) ScheduleCleanup(ctx context.Context) {
	ticker := time.NewTicker(slm.config.CleanupInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := slm.CleanupExpiredSessions(ctx); err != nil {
					slm.logger.Error("session cleanup failed", zap.Error(err))
				}
			}
		}
	}()
}

// RevokeAllUserSessionsWithReason revokes all sessions for a user with a specific reason
func (slm *SessionLifecycleManager) RevokeAllUserSessionsWithReason(ctx context.Context, username, reason string) error {
	sessions, err := slm.sessionManager.repos.Account().GetUserSessions(ctx, username)
	if err != nil {
		return err
	}

	for _, session := range sessions {
		if err := slm.sessionManager.RevokeSession(ctx, session.SessionID); err != nil {
			slm.logger.Error("failed to revoke session",
				zap.String("sessionID", session.SessionID),
				zap.String("reason", reason),
				zap.Error(err))
		}
	}

	slm.securityManager.LogSecurityEvent("mass_session_revocation", "", username, "All user sessions revoked", map[string]interface{}{
		"reason":        reason,
		"session_count": len(sessions),
	})

	return nil
}
