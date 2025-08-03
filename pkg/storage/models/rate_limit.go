package models

import (
	"fmt"
	"time"
)

// LoginAttempt represents a login attempt record for rate limiting
type LoginAttempt struct {
	// Primary keys
	PK string `dynamorm:"pk" json:"pk"` // RATELIMIT#{identifier}
	SK string `dynamorm:"sk" json:"sk"` // timestamp in RFC3339Nano format

	// Attributes
	Type      string    `json:"type"`       // "LoginAttempt"
	Success   bool      `json:"success"`    // whether the login was successful
	Timestamp time.Time `json:"timestamp"`  // when the attempt occurred
	TTL       int64     `json:"ttl" dynamorm:"ttl"` // automatic cleanup after 24 hours
}

// UpdateKeys updates the DynamoDB keys for the LoginAttempt model
func (la *LoginAttempt) UpdateKeys() {
	// PK is set when creating the record (RATELIMIT#{identifier})
	// SK is set when creating the record (timestamp in RFC3339Nano)
	if la.Type == "" {
		la.Type = "LoginAttempt"
	}
}

// NewLoginAttempt creates a new LoginAttempt record
func NewLoginAttempt(identifier string, success bool) *LoginAttempt {
	now := time.Now()
	return &LoginAttempt{
		PK:        fmt.Sprintf("RATELIMIT#%s", identifier),
		SK:        now.Format(time.RFC3339Nano),
		Type:      "LoginAttempt",
		Success:   success,
		Timestamp: now,
		TTL:       now.Add(24 * time.Hour).Unix(), // TTL for automatic cleanup
	}
}

// RateLimitLockout represents an active rate limit lockout
type RateLimitLockout struct {
	// Primary keys
	PK string `dynamorm:"pk" json:"pk"` // RATELIMIT#{identifier}
	SK string `dynamorm:"sk" json:"sk"` // "LOCKOUT"

	// Attributes
	Type       string    `json:"type"`        // "RateLimitLockout"
	UnlockTime time.Time `json:"unlock_time"` // when the lockout expires
	TTL        int64     `json:"ttl" dynamorm:"ttl"` // automatic cleanup
}

// UpdateKeys updates the DynamoDB keys for the RateLimitLockout model
func (rll *RateLimitLockout) UpdateKeys() {
	// PK and SK are set when creating the record
	if rll.Type == "" {
		rll.Type = "RateLimitLockout"
	}
}

// NewRateLimitLockout creates a new RateLimitLockout record
func NewRateLimitLockout(identifier string, unlockTime time.Time) *RateLimitLockout {
	return &RateLimitLockout{
		PK:         fmt.Sprintf("RATELIMIT#%s", identifier),
		SK:         "LOCKOUT",
		Type:       "RateLimitLockout",
		UnlockTime: unlockTime,
		TTL:        unlockTime.Unix(), // TTL matches unlock time
	}
}

// APIRateLimit represents a rate limit counter for API endpoints
type APIRateLimit struct {
	// Primary keys
	PK string `dynamorm:"pk" json:"pk"` // RATELIMIT#{userID}#{endpoint}
	SK string `dynamorm:"sk" json:"sk"` // WINDOW#{window_start}
	
	// Attributes
	Type         string    `json:"type"`          // "APIRateLimit"
	UserID       string    `json:"user_id"`       // User identifier
	Endpoint     string    `json:"endpoint"`      // API endpoint pattern
	Count        int       `json:"count"`         // Current request count
	Window       time.Time `json:"window"`        // Window start time
	Blocked      bool      `json:"blocked"`       // Whether user is blocked
	BlockedUntil time.Time `json:"blocked_until"` // When block expires
	UpdatedAt    time.Time `json:"updated_at"`    // Last update time
	TTL          int64     `json:"ttl" dynamorm:"ttl"` // Automatic cleanup
}

// UpdateKeys updates the DynamoDB keys for the APIRateLimit model
func (arl *APIRateLimit) UpdateKeys() {
	// PK and SK are set when creating/updating the record
	if arl.Type == "" {
		arl.Type = "APIRateLimit"
	}
}

// NewAPIRateLimit creates a new APIRateLimit record
func NewAPIRateLimit(userID, endpoint string, windowStart time.Time) *APIRateLimit {
	now := time.Now()
	key := fmt.Sprintf("%s:%s", userID, endpoint)
	
	return &APIRateLimit{
		PK:        fmt.Sprintf("RATELIMIT#%s", key),
		SK:        fmt.Sprintf("WINDOW#%s", windowStart.Format(time.RFC3339)),
		Type:      "APIRateLimit",
		UserID:    userID,
		Endpoint:  endpoint,
		Count:     0,
		Window:    windowStart,
		Blocked:   false,
		UpdatedAt: now,
		TTL:       windowStart.Add(25 * time.Hour).Unix(), // TTL after window + 1 day
	}
}