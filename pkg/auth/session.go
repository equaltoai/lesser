package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/storage"
)

// Session errors
var (
	ErrSessionNotFound = errors.New("session not found")
	ErrSessionExpired  = errors.New("session expired")
	ErrDeviceNotFound  = errors.New("device not found")
)

// Session constants
const (
	SessionDuration            = 30 * 24 * time.Hour // 30 days
	ShortAccessTokenDuration   = 15 * time.Minute    // 15 minutes (from modern auth plan)
	RefreshTokenRotationWindow = 24 * time.Hour      // Allow old refresh token for 24h after rotation
)

// Type aliases for convenience
type Session = storage.Session
type Device = storage.Device

// SessionManager handles session operations
type SessionManager struct {
	storage storage.Storage
}

// NewSessionManager creates a new session manager
func NewSessionManager(storage storage.Storage) *SessionManager {
	return &SessionManager{
		storage: storage,
	}
}

// CreateSession creates a new session for a user
func (sm *SessionManager) CreateSession(ctx context.Context, username, deviceName, userAgent, ipAddress, authMethod string) (*Session, error) {
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

	// Generate device ID
	deviceID, err := generateSecureToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate device ID: %w", err)
	}

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
	if err := sm.storage.CreateSession(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to store session: %w", err)
	}

	// Create device record
	device := &Device{
		DeviceID:      deviceID,
		Username:      username,
		DeviceName:    deviceName,
		DeviceType:    detectDeviceType(userAgent),
		LastIPAddress: ipAddress,
		LastUserAgent: userAgent,
		CreatedAt:     now,
		LastSeenAt:    now,
		TrustLevel:    "untrusted", // New devices start as untrusted
	}

	if err := sm.storage.CreateDevice(ctx, device); err != nil {
		// Non-fatal error, log but continue
		fmt.Printf("Failed to create device record: %v\n", err)
	}

	return session, nil
}

// ValidateRefreshToken validates a refresh token and returns the session
func (sm *SessionManager) ValidateRefreshToken(ctx context.Context, refreshToken string) (*Session, error) {
	// Get session by refresh token
	session, err := sm.storage.GetSessionByRefreshToken(ctx, refreshToken)
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
	if err := sm.storage.UpdateSession(ctx, session); err != nil {
		return "", fmt.Errorf("failed to update session: %w", err)
	}

	return newRefreshToken, nil
}

// UpdateSessionActivity updates the last activity timestamp
func (sm *SessionManager) UpdateSessionActivity(ctx context.Context, sessionID, ipAddress string) error {
	session, err := sm.storage.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	session.LastActivity = time.Now()
	session.IPAddress = ipAddress

	// Sliding expiration
	if time.Until(session.ExpiresAt) < SessionDuration/2 {
		session.ExpiresAt = time.Now().Add(SessionDuration)
	}

	return sm.storage.UpdateSession(ctx, session)
}

// RevokeSession revokes a specific session
func (sm *SessionManager) RevokeSession(ctx context.Context, sessionID string) error {
	return sm.storage.DeleteSession(ctx, sessionID)
}

// RevokeAllUserSessions revokes all sessions for a user
func (sm *SessionManager) RevokeAllUserSessions(ctx context.Context, username string) error {
	sessions, err := sm.storage.GetUserSessions(ctx, username)
	if err != nil {
		return err
	}

	for _, session := range sessions {
		if err := sm.storage.DeleteSession(ctx, session.SessionID); err != nil {
			// Log error but continue
			fmt.Printf("Failed to delete session %s: %v\n", session.SessionID, err)
		}
	}

	return nil
}

// GetUserDevices returns all devices for a user
func (sm *SessionManager) GetUserDevices(ctx context.Context, username string) ([]*Device, error) {
	return sm.storage.GetUserDevices(ctx, username)
}

// TrustDevice marks a device as trusted
func (sm *SessionManager) TrustDevice(ctx context.Context, deviceID string) error {
	device, err := sm.storage.GetDevice(ctx, deviceID)
	if err != nil {
		return err
	}

	device.TrustLevel = "trusted"
	return sm.storage.UpdateDevice(ctx, device)
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
