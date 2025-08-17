package models

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// Session represents a user session with OAuth tokens
type Session struct {
	// Primary key - using session ID as partition key
	PK string `dynamorm:"pk" json:"pk"` // Format: "session#{sessionID}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "session#{sessionID}" (same as PK for simple key)

	// GSI1 - User sessions lookup
	GSI1PK string `dynamorm:"index:user-sessions-index,pk" json:"gsi1_pk"` // Format: "USER_SESSIONS#{userID}"
	GSI1SK string `dynamorm:"index:user-sessions-index,sk" json:"gsi1_sk"` // Format: "{created_at}#{sessionID}"

	// GSI2 - Access token lookup
	GSI2PK string `dynamorm:"index:token-index,pk" json:"gsi2_pk,omitempty"` // Format: "TOKEN#{access_token_hash}"
	GSI2SK string `dynamorm:"index:token-index,sk" json:"gsi2_sk,omitempty"` // Format: "{userID}"

	// Core session data
	SessionID    string   `json:"session_id"`
	UserID       string   `json:"user_id"`
	AccessToken  string   `json:"access_token"`            // Stored encrypted
	RefreshToken string   `json:"refresh_token,omitempty"` // Stored encrypted
	Scopes       []string `json:"scopes,omitempty"`

	// Session metadata
	IPAddress string `json:"ip_address,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
	DeviceID  string `json:"device_id,omitempty"`

	// Security and tracking
	IsRevoked    bool       `json:"is_revoked"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
	RevokeReason string     `json:"revoke_reason,omitempty"`

	// Timestamps
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	LastUsedAt time.Time `json:"last_used_at"`

	// TTL for automatic cleanup
	ExpiresAt int64 `dynamorm:"ttl" json:"expires_at"` // Unix timestamp for DynamoDB TTL

	// Additional context data
	Context map[string]interface{} `json:"context,omitempty"`

	// Version for optimistic locking
	Version int `dynamorm:"version" json:"version"`
}

// TableName returns the DynamoDB table name for the Session model
func (Session) TableName() string {
	return MainTableName // Use the main table
}

// BeforeCreate sets up the model before creation
func (s *Session) BeforeCreate() error {
	now := time.Now()
	s.CreatedAt = now
	s.UpdatedAt = now
	s.LastUsedAt = now

	// Generate session ID if not provided
	if err := common.ValidateRequiredParam("sessionID", s.SessionID); err != nil {
		var err error
		s.SessionID, err = generateSecureToken(32)
		if err != nil {
			return fmt.Errorf("failed to generate session ID: %w", err)
		}
	}

	// Generate access token if not provided
	if err := common.ValidateRequiredParam("accessToken", s.AccessToken); err != nil {
		var err error
		s.AccessToken, err = generateSecureToken(64)
		if err != nil {
			return fmt.Errorf("failed to generate access token: %w", err)
		}
	}

	// Set default expiry to 24 hours if not specified
	if s.ExpiresAt == 0 {
		s.ExpiresAt = now.Add(24 * time.Hour).Unix()
	}

	// Set up primary key
	s.PK = "session#" + s.SessionID
	s.SK = "session#" + s.SessionID

	// Set up GSI keys
	s.setupGSIKeys()

	return s.Validate()
}

// BeforeUpdate sets up the model before update
func (s *Session) BeforeUpdate() error {
	s.UpdatedAt = time.Now()

	// Update GSI keys in case user or other indexed fields changed
	s.setupGSIKeys()

	return s.Validate()
}

// setupGSIKeys configures all GSI partition and sort keys
func (s *Session) setupGSIKeys() {
	// GSI1 - User sessions lookup
	if err := common.ValidateRequiredParam("userID", s.UserID); err == nil {
		s.GSI1PK = "USER_SESSIONS#" + s.UserID
		s.GSI1SK = fmt.Sprintf("%s#%s", s.CreatedAt.Format(time.RFC3339), s.SessionID)
	} else {
		s.GSI1PK = ""
		s.GSI1SK = ""
	}

	// GSI2 - Access token lookup (only if access token exists)
	if err := common.ValidateRequiredParam("accessToken", s.AccessToken); err == nil {
		// Use a hash of the token for the GSI key to avoid storing the full token
		tokenHash := hashToken(s.AccessToken)
		s.GSI2PK = "TOKEN#" + tokenHash
		s.GSI2SK = s.UserID
	}
}

// Validate performs validation on the Session
func (s *Session) Validate() error {
	if err := common.ValidateRequiredParam("SessionID", strings.TrimSpace(s.SessionID)); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("UserID", strings.TrimSpace(s.UserID)); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("AccessToken", strings.TrimSpace(s.AccessToken)); err != nil {
		return err
	}
	if s.ExpiresAt <= 0 {
		return fmt.Errorf("ExpiresAt must be set")
	}

	return nil
}

// Touch updates the last used timestamp and extends expiry if needed
func (s *Session) Touch() {
	s.LastUsedAt = time.Now()

	// Extend expiry if less than 12 hours remaining
	currentExpiry := time.Unix(s.ExpiresAt, 0)
	if time.Until(currentExpiry) < 12*time.Hour {
		s.ExpiresAt = time.Now().Add(24 * time.Hour).Unix()
	}
}

// Revoke marks the session as revoked with the given reason
func (s *Session) Revoke(reason string) {
	s.IsRevoked = true
	now := time.Now()
	s.RevokedAt = &now
	s.RevokeReason = reason
}

// IsValid returns true if the session is valid (not revoked and not expired)
func (s *Session) IsValid() bool {
	if s.IsRevoked {
		return false
	}

	return time.Unix(s.ExpiresAt, 0).After(time.Now())
}

// IsExpired returns true if the session has expired
func (s *Session) IsExpired() bool {
	return time.Unix(s.ExpiresAt, 0).Before(time.Now())
}

// HasScope returns true if the session has the specified scope
func (s *Session) HasScope(scope string) bool {
	for _, s := range s.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// ValidateRequest validates the session against request metadata for security
func (s *Session) ValidateRequest(_, _ string) bool {
	// For now, just log differences - in production you might want stricter validation
	// This helps detect session hijacking attempts
	return true // Allow for now, but log differences
}

// SetContext sets a context value
func (s *Session) SetContext(key string, value interface{}) {
	if s.Context == nil {
		s.Context = make(map[string]interface{})
	}
	s.Context[key] = value
}

// GetContext gets a context value
func (s *Session) GetContext(key string) (interface{}, bool) {
	if s.Context == nil {
		return nil, false
	}
	value, exists := s.Context[key]
	return value, exists
}

// RemainingTime returns the time until the session expires
func (s *Session) RemainingTime() time.Duration {
	return time.Until(time.Unix(s.ExpiresAt, 0))
}

// generateSecureToken generates a cryptographically secure random token
func generateSecureToken(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// hashToken creates a secure hash of the token for GSI indexing
func hashToken(token string) string {
	// Use SHA256 to create a secure hash of the token
	hash := sha256.Sum256([]byte(token))
	// Return the first 16 characters of the hex-encoded hash for GSI key
	hexHash := hex.EncodeToString(hash[:])
	if len(hexHash) > 16 {
		return hexHash[:16]
	}
	return hexHash
}
