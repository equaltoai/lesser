package models

import (
	"fmt"
	"time"
)

// LoginAttempt represents a login attempt record for rate limiting
type LoginAttempt struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary keys
	PK string `theorydb:"pk,attr:PK" json:"pk"` // RATELIMIT#{identifier}
	SK string `theorydb:"sk,attr:SK" json:"sk"` // timestamp in RFC3339Nano format

	// Attributes
	Type      string    `theorydb:"attr:type" json:"type"`           // "LoginAttempt"
	Success   bool      `theorydb:"attr:success" json:"success"`     // whether the login was successful
	Timestamp time.Time `theorydb:"attr:timestamp" json:"timestamp"` // when the attempt occurred
	TTL       int64     `theorydb:"ttl,attr:ttl" json:"ttl"`         // automatic cleanup after 24 hours
}

// UpdateKeys updates the DynamoDB keys for the LoginAttempt model
func (la *LoginAttempt) UpdateKeys() error {
	// Set type
	la.Type = "LoginAttempt"

	// Set SK from Timestamp if available and SK not set
	if la.SK == "" && !la.Timestamp.IsZero() {
		la.SK = la.Timestamp.Format(time.RFC3339Nano)
	}

	// Note: PK must be set externally with the identifier (format: RATELIMIT#{identifier})
	// SK is generated from Timestamp if available
	return nil
}

// GetPK returns the partition key - required for BaseModel interface
func (la *LoginAttempt) GetPK() string {
	return la.PK
}

// GetSK returns the sort key - required for BaseModel interface
func (la *LoginAttempt) GetSK() string {
	return la.SK
}

// TableName returns the DynamoDB table name for login attempts
func (LoginAttempt) TableName() string {
	return MainTableName
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
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary keys
	PK string `theorydb:"pk,attr:PK" json:"pk"` // RATELIMIT#{identifier}
	SK string `theorydb:"sk,attr:SK" json:"sk"` // "LOCKOUT"

	// Attributes
	Type       string    `theorydb:"attr:type" json:"type"`              // "RateLimitLockout"
	UnlockTime time.Time `theorydb:"attr:unlockTime" json:"unlock_time"` // when the lockout expires
	TTL        int64     `theorydb:"ttl,attr:ttl" json:"ttl"`            // automatic cleanup
}

// UpdateKeys updates the DynamoDB keys for the RateLimitLockout model
func (rll *RateLimitLockout) UpdateKeys() error {
	// Set type
	rll.Type = "RateLimitLockout"

	// Set SK if not already set
	if rll.SK == "" {
		rll.SK = "LOCKOUT"
	}

	// Note: PK must be set externally with the identifier (format: RATELIMIT#{identifier})
	return nil
}

// GetPK returns the partition key - required for BaseModel interface
func (rll *RateLimitLockout) GetPK() string {
	return rll.PK
}

// GetSK returns the sort key - required for BaseModel interface
func (rll *RateLimitLockout) GetSK() string {
	return rll.SK
}

// TableName returns the DynamoDB table name for rate limit lockouts
func (RateLimitLockout) TableName() string {
	return MainTableName
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
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary keys
	PK string `theorydb:"pk,attr:PK" json:"pk"` // RATELIMIT#{userID|domain}#{endpoint}
	SK string `theorydb:"sk,attr:SK" json:"sk"` // WINDOW#{window_start}

	// GSI1 - per-user rate limit lookup (user limits only)
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty" json:"gsi1_pk,omitempty"` // USER_RATELIMIT#{userID}
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty" json:"gsi1_sk,omitempty"` // ENDPOINT#{endpoint}#WINDOW#{window_start}

	// Attributes
	Type         string    `theorydb:"attr:type" json:"type"`                  // "APIRateLimit"
	UserID       string    `theorydb:"attr:userID" json:"user_id"`             // User identifier
	Domain       string    `theorydb:"attr:domain" json:"domain,omitempty"`    // Domain for federation limits
	Endpoint     string    `theorydb:"attr:endpoint" json:"endpoint"`          // API endpoint pattern
	Count        int       `theorydb:"attr:count" json:"count"`                // Current request count
	Window       time.Time `theorydb:"attr:window" json:"window"`              // Window start time
	Blocked      bool      `theorydb:"attr:blocked" json:"blocked"`            // Whether user is blocked
	BlockedUntil time.Time `theorydb:"attr:blockedUntil" json:"blocked_until"` // When block expires
	UpdatedAt    time.Time `theorydb:"attr:updatedAt" json:"updated_at"`       // Last update time
	TTL          int64     `theorydb:"ttl,attr:ttl" json:"ttl"`                // Automatic cleanup

	// Escalating penalty tracking
	ViolationCount int       `theorydb:"attr:violationCount" json:"violation_count"` // Number of violations
	FirstViolation time.Time `theorydb:"attr:firstViolation" json:"first_violation"` // When first violation occurred
	LastViolation  time.Time `theorydb:"attr:lastViolation" json:"last_violation"`   // Most recent violation
}

// UpdateKeys updates the DynamoDB keys for the APIRateLimit model
func (arl *APIRateLimit) UpdateKeys() error {
	// Set type
	if arl.Type == "" {
		arl.Type = "APIRateLimit"
	}

	// Set SK from Window if available and SK not set
	if arl.SK == "" && !arl.Window.IsZero() {
		arl.SK = fmt.Sprintf("WINDOW#%s", arl.Window.Format(time.RFC3339))
	}

	// Set GSI1 for user-scoped limits only (federation rows carry no UserID).
	// Legacy rows written before this GSI existed carry no gsi1 keys; they are
	// TTL-transient and are not cleared by ClearAPIRateLimitsForUser.
	if arl.UserID != "" {
		arl.GSI1PK = fmt.Sprintf("USER_RATELIMIT#%s", arl.UserID)
		arl.GSI1SK = fmt.Sprintf("ENDPOINT#%s#WINDOW#%s", arl.Endpoint, arl.Window.Format(time.RFC3339))
	} else {
		arl.GSI1PK = ""
		arl.GSI1SK = ""
	}

	// Note: PK must be set externally with the identifier (format: RATELIMIT#{key})
	return nil
}

// GetPK returns the partition key - required for BaseModel interface
func (arl *APIRateLimit) GetPK() string {
	return arl.PK
}

// GetSK returns the sort key - required for BaseModel interface
func (arl *APIRateLimit) GetSK() string {
	return arl.SK
}

// TableName returns the DynamoDB table name for API rate limits
func (APIRateLimit) TableName() string {
	return MainTableName
}

// NewAPIRateLimit creates a new APIRateLimit record
func NewAPIRateLimit(userID, endpoint string, windowStart time.Time) *APIRateLimit {
	now := time.Now()
	key := fmt.Sprintf("%s:%s", userID, endpoint)

	arl := &APIRateLimit{
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
	_ = arl.UpdateKeys() // Set GSI1 keys for user-scoped lookups
	return arl
}

// NewFederationRateLimit creates a new rate limit record for federation domains
func NewFederationRateLimit(domain, endpoint string, windowStart time.Time) *APIRateLimit {
	now := time.Now()
	key := fmt.Sprintf("DOMAIN#%s:%s", domain, endpoint)

	return &APIRateLimit{
		PK:        fmt.Sprintf("RATELIMIT#%s", key),
		SK:        fmt.Sprintf("WINDOW#%s", windowStart.Format(time.RFC3339)),
		Type:      "FederationRateLimit",
		Domain:    domain,
		Endpoint:  endpoint,
		Count:     0,
		Window:    windowStart,
		Blocked:   false,
		UpdatedAt: now,
		TTL:       windowStart.Add(25 * time.Hour).Unix(), // TTL after window + 1 day
	}
}

// RateLimitViolation represents a rate limit violation for escalating penalties
type RateLimitViolation struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary keys
	PK string `theorydb:"pk,attr:PK" json:"pk"` // RATELIMIT_VIOLATION#{userID|domain}
	SK string `theorydb:"sk,attr:SK" json:"sk"` // timestamp of violation

	// Attributes
	Type           string    `theorydb:"attr:type" json:"type"`                      // "RateLimitViolation"
	UserID         string    `theorydb:"attr:userID" json:"user_id"`                 // User identifier
	Domain         string    `theorydb:"attr:domain" json:"domain,omitempty"`        // Domain for federation violations
	Endpoint       string    `theorydb:"attr:endpoint" json:"endpoint"`              // Endpoint that was rate limited
	ViolationType  string    `theorydb:"attr:violationType" json:"violation_type"`   // "api" or "federation"
	Timestamp      time.Time `theorydb:"attr:timestamp" json:"timestamp"`            // When violation occurred
	PenaltyMinutes int       `theorydb:"attr:penaltyMinutes" json:"penalty_minutes"` // Minutes of penalty applied
	TTL            int64     `theorydb:"ttl,attr:ttl" json:"ttl"`                    // Cleanup after 7 days
}

// UpdateKeys updates the DynamoDB keys for the RateLimitViolation model
func (rlv *RateLimitViolation) UpdateKeys() error {
	// Set type
	rlv.Type = "RateLimitViolation"

	// Set SK from Timestamp if available and SK not set
	if rlv.SK == "" && !rlv.Timestamp.IsZero() {
		rlv.SK = rlv.Timestamp.Format(time.RFC3339Nano)
	}

	// Note: PK must be set externally with the identifier (format: RATELIMIT_VIOLATION#{identifier})
	return nil
}

// GetPK returns the partition key - required for BaseModel interface
func (rlv *RateLimitViolation) GetPK() string {
	return rlv.PK
}

// GetSK returns the sort key - required for BaseModel interface
func (rlv *RateLimitViolation) GetSK() string {
	return rlv.SK
}

// TableName returns the DynamoDB table name for rate limit violations
func (RateLimitViolation) TableName() string {
	return MainTableName
}

// NewRateLimitViolation creates a new rate limit violation record
func NewRateLimitViolation(userID, domain, endpoint, violationType string, penaltyMinutes int) *RateLimitViolation {
	now := time.Now()
	identifier := userID
	if domain != "" {
		identifier = fmt.Sprintf("DOMAIN#%s", domain)
	}

	return &RateLimitViolation{
		PK:             fmt.Sprintf("RATELIMIT_VIOLATION#%s", identifier),
		SK:             now.Format(time.RFC3339Nano),
		Type:           "RateLimitViolation",
		UserID:         userID,
		Domain:         domain,
		Endpoint:       endpoint,
		ViolationType:  violationType,
		Timestamp:      now,
		PenaltyMinutes: penaltyMinutes,
		TTL:            now.Add(7 * 24 * time.Hour).Unix(), // TTL for 7 days
	}
}
