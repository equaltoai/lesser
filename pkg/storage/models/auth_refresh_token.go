package models

import (
	"fmt"
	"time"
)

// AuthRefreshToken represents a refresh token with advanced security features
// This implements the token family pattern with rotation and reuse detection
type AuthRefreshToken struct {
	// DynamoDB keys - using token as partition key
	PK string `dynamorm:"pk" json:"-"` // token (the actual token value)
	SK string `dynamorm:"sk" json:"-"` // TOKEN (constant sort key)

	// GSI keys for querying by user and family
	UserID      string `dynamorm:"index:user-index,pk" json:"user_id"`                                            // GSI PK for user-index
	Family      string `dynamorm:"index:family-index,pk" json:"family"`                                           // GSI PK for family-index
	UserFamily  string `dynamorm:"index:user-family-index,pk" json:"user_family"`                                 // GSI PK for user-family queries
	CreatedAtSK string `dynamorm:"index:user-index,sk;index:family-index,sk;index:user-family-index,sk" json:"-"` // SK for all GSIs

	// Core token data
	Token      string `json:"token"`        // The actual token value
	Generation int    `json:"generation"`   // Rotation generation number
	CreatedAt  int64  `json:"created_at"`   // Unix timestamp
	ExpiresAt  int64  `json:"expires_at"`   // Unix timestamp
	LastUsedAt int64  `json:"last_used_at"` // Unix timestamp for tracking

	// Security fields
	Revoked       bool   `json:"revoked"`        // Whether token is revoked
	RevokedReason string `json:"revoked_reason"` // Reason for revocation

	// Tracking fields
	DeviceName string `json:"device_name"` // Optional device identifier
	IPAddress  string `json:"ip_address"`  // IP address for security monitoring

	// TTL for automatic cleanup
	TTL int64 `dynamorm:"ttl" json:"-"`
}

// TableName returns the DynamoDB table name
func (AuthRefreshToken) TableName() string {
	return MainTableName
}

// UpdateKeys ensures all keys are properly set for DynamoDB operations
func (a *AuthRefreshToken) UpdateKeys() {
	// Primary key is the token value itself
	a.PK = a.Token
	a.SK = SKToken

	// Set GSI keys for efficient querying
	a.UserFamily = fmt.Sprintf("%s#%s", a.UserID, a.Family)
	a.CreatedAtSK = fmt.Sprintf("%d", a.CreatedAt)

	// Set TTL from ExpiresAt if not already set
	if a.TTL == 0 && a.ExpiresAt > 0 {
		a.TTL = a.ExpiresAt
	}
}

// BeforeCreate sets up keys before creating the record
func (a *AuthRefreshToken) BeforeCreate() error {
	a.UpdateKeys()
	return nil
}

// BeforeUpdate sets up keys before updating the record
func (a *AuthRefreshToken) BeforeUpdate() error {
	a.UpdateKeys()
	return nil
}

// IsExpired checks if the token has expired
func (a *AuthRefreshToken) IsExpired() bool {
	return time.Now().Unix() > a.ExpiresAt
}

// IsActive checks if the token is active (not revoked and not expired)
func (a *AuthRefreshToken) IsActive() bool {
	return !a.Revoked && !a.IsExpired()
}
