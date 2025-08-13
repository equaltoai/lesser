package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	
)

// Session errors
var (
	ErrSessionNotFound = errors.New("session not found")
	ErrSessionExpired  = errors.New("session expired")
	ErrDeviceNotFound  = errors.New("device not found")
)

// Session constants - enhanced security
const (
	SessionDuration               = 7 * 24 * time.Hour  // 7 days (reduced from 30)
	ShortAccessTokenDuration      = 15 * time.Minute     // 15 minutes
	RefreshTokenRotationWindow    = 1 * time.Hour        // Reduced grace period (from 24h)
	MaxSessionsPerUser           = 10                     // Limit concurrent sessions
	SessionInactivityTimeout     = 24 * time.Hour        // Auto-logout after inactivity
	DeviceTrustPromotionThreshold = 7 * 24 * time.Hour    // Days until device can be trusted
)

// Session is a type alias for storage.Session
type Session = storage.Session

// Device is a type alias for storage.Device
type Device = storage.Device

// SessionManager handles session operations with enhanced security
type SessionManager struct {
	repos         StorageProvider
	tokenVersions map[string]int // Track token versions per user
	mu            sync.RWMutex   // Protect token versions map
}

// SessionSecurityConfig holds security configuration for sessions
type SessionSecurityConfig struct {
	EnableIPBinding       bool
	EnableDeviceBinding   bool
	EnableGeoValidation   bool
	Require2FA            bool
	MaxConcurrentSessions int
}

// NewSessionManager creates a new session manager with enhanced security
func NewSessionManager(repos StorageProvider) *SessionManager {
	return &SessionManager{
		repos:         repos,
		tokenVersions: make(map[string]int),
	}
}

// CreateSession creates a new session for a user with enhanced security
func (sm *SessionManager) CreateSession(ctx context.Context, username, deviceName, userAgent, ipAddress, authMethod string) (*Session, error) {
	// Check session limits
	if err := sm.enforceSessionLimits(ctx, username); err != nil {
		return nil, err
	}

	// Generate session ID
	sessionID, err := generateSecureToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate session ID: %w", err)
	}

	// Generate refresh token
	refreshToken, err := generateSecureToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	// Generate device ID if new device
	deviceID, err := sm.getOrCreateDeviceID(ctx, username, userAgent, ipAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to get device ID: %w", err)
	}

	// Token version tracking (for future use)
	_ = sm.incrementTokenVersion(username)

	now := time.Now()
	session := &Session{
		SessionID:    sessionID,
		Username:     username,
		RefreshToken: refreshToken,
		DeviceID:     deviceID,
		DeviceName:   deviceName,
		UserAgent:    userAgent,
		IPAddress:    ipAddress,
		AuthMethod:   authMethod,
		CreatedAt:    now,
		LastActivity: now,
		ExpiresAt:    now.Add(SessionDuration),
	}

	// Store session
	_, err = sm.repos.Account().CreateSession(ctx, session.Username, session.IPAddress, session.UserAgent)
	if err != nil {
		return nil, fmt.Errorf("failed to store session: %w", err)
	}

	// Update device record
	if err := sm.updateDeviceRecord(ctx, deviceID, username, deviceName, userAgent, ipAddress); err != nil {
		// Non-fatal error, log but continue
		fmt.Printf("Failed to update device record: %v\n", err)
	}

	return session, nil
}

// ValidateRefreshToken validates a refresh token and returns the session
func (sm *SessionManager) ValidateRefreshToken(ctx context.Context, refreshToken string) (*Session, error) {
	// Get session by refresh token
	session, err := sm.repos.Account().GetSessionByRefreshToken(ctx, refreshToken)
	if err != nil {
		return nil, ErrInvalidRefreshToken
	}

	// Check if session is expired
	if time.Now().After(session.ExpiresAt) {
		return nil, ErrSessionExpired
	}

	// Check if this is a rotated token that's still in grace period
	if session.PreviousRefreshToken == refreshToken {
		if time.Since(session.TokenRotatedAt) > RefreshTokenRotationWindow {
			return nil, ErrInvalidRefreshToken
		}
		// Token is in grace period, allow it
	}

	return session, nil
}

// RotateRefreshToken rotates a refresh token for enhanced security
func (sm *SessionManager) RotateRefreshToken(ctx context.Context, session *Session) (string, error) {
	// Generate new refresh token
	newRefreshToken, err := generateSecureToken()
	if err != nil {
		return "", fmt.Errorf("failed to generate new refresh token: %w", err)
	}

	// Update session with new token
	session.PreviousRefreshToken = session.RefreshToken
	session.RefreshToken = newRefreshToken
	session.TokenRotatedAt = time.Now()
	session.LastActivity = time.Now()

	// Extend expiration on activity (sliding expiration)
	if time.Until(session.ExpiresAt) < SessionDuration/2 {
		session.ExpiresAt = time.Now().Add(SessionDuration)
	}

	// Update in storage
	if err := sm.repos.Account().UpdateSession(ctx, session.SessionID, session.RefreshToken, session.IPAddress, session.LastActivity, session.ExpiresAt); err != nil {
		return "", fmt.Errorf("failed to update session: %w", err)
	}

	return newRefreshToken, nil
}

// UpdateSessionActivity updates the last activity timestamp
func (sm *SessionManager) UpdateSessionActivity(ctx context.Context, sessionID, ipAddress string) error {
	session, err := sm.repos.Account().GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	session.LastActivity = time.Now()
	session.IPAddress = ipAddress

	// Sliding expiration
	if time.Until(session.ExpiresAt) < SessionDuration/2 {
		session.ExpiresAt = time.Now().Add(SessionDuration)
	}

	return sm.repos.Account().UpdateSession(ctx, session.SessionID, session.RefreshToken, session.IPAddress, session.LastActivity, session.ExpiresAt)
}

// RevokeSession revokes a specific session
func (sm *SessionManager) RevokeSession(ctx context.Context, sessionID string) error {
	return sm.repos.Account().DeleteSession(ctx, sessionID)
}

// RevokeAllUserSessions revokes all sessions for a user
func (sm *SessionManager) RevokeAllUserSessions(ctx context.Context, username string) error {
	sessions, err := sm.repos.Account().GetUserSessions(ctx, username)
	if err != nil {
		return err
	}

	for _, session := range sessions {
		if err := sm.repos.Account().DeleteSession(ctx, session.SessionID); err != nil {
			// Log error but continue
			fmt.Printf("Failed to delete session %s: %v\n", session.SessionID, err)
		}
	}

	return nil
}

// GetUserDevices returns all devices for a user
func (sm *SessionManager) GetUserDevices(ctx context.Context, username string) ([]*Device, error) {
	return sm.repos.Account().GetUserDevices(ctx, username)
}

// TrustDevice marks a device as trusted
func (sm *SessionManager) TrustDevice(ctx context.Context, deviceID string) error {
	device, err := sm.repos.Account().GetDevice(ctx, deviceID)
	if err != nil {
		return err
	}

	device.TrustLevel = "trusted"
	return sm.repos.Account().UpdateDevice(ctx, device)
}

// Helper functions

// generateSecureToken generates a cryptographically secure random token
func generateSecureToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// detectDeviceType attempts to detect device type from user agent
func detectDeviceType(userAgent string) string {
	// Simple detection logic - can be enhanced
	switch {
	case contains(userAgent, "Mobile") || contains(userAgent, "Android") || contains(userAgent, "iPhone"):
		return "mobile"
	case contains(userAgent, "Tablet") || contains(userAgent, "iPad"):
		return "tablet"
	default:
		return "desktop"
	}
}

// contains is a simple case-insensitive string contains
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			len(s) > 0 && len(substr) > 0 &&
				strings.Contains(strings.ToLower(s), strings.ToLower(substr)))
}

// Enhanced security helper methods

// enforceSessionLimits checks and enforces session limits per user
func (sm *SessionManager) enforceSessionLimits(ctx context.Context, username string) error {
	sessions, err := sm.repos.Account().GetUserSessions(ctx, username)
	if err != nil {
		return fmt.Errorf("failed to get user sessions: %w", err)
	}

	// Count active sessions
	activeCount := 0
	now := time.Now()
	for _, session := range sessions {
		if now.Before(session.ExpiresAt) {
			activeCount++
		}
	}

	// If at limit, remove oldest session
	if activeCount >= MaxSessionsPerUser {
		if err := sm.removeOldestSession(ctx, username, sessions); err != nil {
			return fmt.Errorf("failed to remove oldest session: %w", err)
		}
	}

	return nil
}

// removeOldestSession removes the oldest session for a user
func (sm *SessionManager) removeOldestSession(ctx context.Context, username string, sessions []*Session) error {
	if len(sessions) == 0 {
		return nil
	}

	// Find oldest session
	oldest := sessions[0]
	for _, session := range sessions {
		if session.CreatedAt.Before(oldest.CreatedAt) {
			oldest = session
		}
	}

	return sm.repos.Account().DeleteSession(ctx, oldest.SessionID)
}

// getOrCreateDeviceID gets existing device ID or creates new one
func (sm *SessionManager) getOrCreateDeviceID(ctx context.Context, username, userAgent, ipAddress string) (string, error) {
	// Try to find existing device by fingerprint
	devices, err := sm.repos.Account().GetUserDevices(ctx, username)
	if err != nil {
		return "", err
	}

	// Simple device fingerprinting (can be enhanced)
	for _, device := range devices {
		if device.LastUserAgent == userAgent {
			return device.DeviceID, nil
		}
	}

	// Create new device ID
	return generateSecureToken()
}

// incrementTokenVersion increments and returns the token version for a user
func (sm *SessionManager) incrementTokenVersion(username string) int {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.tokenVersions[username]++
	return sm.tokenVersions[username]
}

// GetTokenVersion returns the current token version for a user
func (sm *SessionManager) GetTokenVersion(username string) int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return sm.tokenVersions[username]
}

// InvalidateAllUserTokens increments token version to invalidate all tokens
func (sm *SessionManager) InvalidateAllUserTokens(username string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.tokenVersions[username] += 100 // Large increment to invalidate all tokens
}

// calculateSecurityFlags determines security flags based on auth method and context
func (sm *SessionManager) calculateSecurityFlags(userAgent, ipAddress, authMethod string) map[string]bool {
	flags := make(map[string]bool)

	// High security auth methods
	flags["high_security"] = authMethod == "webauthn" || authMethod == "wallet"
	flags["password_auth"] = authMethod == "password"
	flags["mobile_device"] = contains(userAgent, "Mobile")
	flags["trusted_network"] = sm.isTrustedNetwork(ipAddress)

	return flags
}

// isTrustedNetwork checks if an IP address is from a trusted network
func (sm *SessionManager) isTrustedNetwork(ipAddress string) bool {
	// Simple implementation - can be enhanced with actual network ranges
	trustedRanges := []string{
		"192.168.",   // Private networks
		"10.",        // Private networks
		"172.16.",    // Private networks
		"127.0.0.1",  // Localhost
	}

	for _, trusted := range trustedRanges {
		if strings.HasPrefix(ipAddress, trusted) {
			return true
		}
	}

	return false
}

// updateDeviceRecord updates or creates a device record
func (sm *SessionManager) updateDeviceRecord(ctx context.Context, deviceID, username, deviceName, userAgent, ipAddress string) error {
	now := time.Now()

	// Try to get existing device
	existingDevice, err := sm.repos.Account().GetDevice(ctx, deviceID)
	if err != nil {
		// Create new device
		device := &Device{
			DeviceID:      deviceID,
			Username:      username,
			DeviceName:    deviceName,
			DeviceType:    detectDeviceType(userAgent),
			LastIPAddress: ipAddress,
			LastUserAgent: userAgent,
			CreatedAt:     now,
			LastSeenAt:    now,
			TrustLevel:    "untrusted",
		}
		return sm.repos.Account().CreateDevice(ctx, device)
	}

	// Update existing device
	existingDevice.LastIPAddress = ipAddress
	existingDevice.LastUserAgent = userAgent
	existingDevice.LastSeenAt = now

	// Promote to trusted if device has been used long enough
	if existingDevice.TrustLevel == "untrusted" && time.Since(existingDevice.CreatedAt) > DeviceTrustPromotionThreshold {
		existingDevice.TrustLevel = "trusted"
	}

	return sm.repos.Account().UpdateDevice(ctx, existingDevice)
}

// CleanupInactiveSessions removes sessions that have been inactive too long
func (sm *SessionManager) CleanupInactiveSessions(ctx context.Context) error {
	// This would typically be called by a background job
	// Implementation depends on storage capabilities
	return nil
}

// DetectAnomalousSession checks for suspicious session activity
func (sm *SessionManager) DetectAnomalousSession(ctx context.Context, session *Session, currentIP string) (bool, string) {
	// Check for IP address changes
	if session.IPAddress != currentIP && !sm.isTrustedNetwork(currentIP) {
		return true, "IP address changed from untrusted network"
	}

	// Check for session age
	if time.Since(session.LastActivity) > SessionInactivityTimeout {
		return true, "Session inactive for too long"
	}

	// Additional anomaly detection can be added here
	return false, ""
}
