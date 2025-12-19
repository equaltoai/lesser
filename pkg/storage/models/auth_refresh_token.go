package models

import (
	"fmt"
	"time"
)

// AuthRefreshToken represents a refresh token with advanced security features
// This implements the token family pattern with rotation and reuse detection
type AuthRefreshToken struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// DynamoDB keys - using token as partition key
	PK string `dynamorm:"pk,attr:PK" json:"-"` // token (the actual token value)
	SK string `dynamorm:"sk,attr:SK" json:"-"` // TOKEN (constant sort key)

	// GSI keys for querying by user, family, and user-family
	UserID     string `dynamorm:"attr:userID" json:"user_id"`
	Family     string `dynamorm:"attr:family" json:"family"`
	UserFamily string `dynamorm:"attr:userFamily" json:"user_family"`

	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsi1PK" json:"-"` // USER#{userID}
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsi1SK" json:"-"` // {createdAt}
	GSI2PK string `dynamorm:"index:gsi2,pk,attr:gsi2PK" json:"-"` // FAMILY#{family}
	GSI2SK string `dynamorm:"index:gsi2,sk,attr:gsi2SK" json:"-"` // {createdAt}
	GSI3PK string `dynamorm:"index:gsi3,pk,attr:gsi3PK" json:"-"` // USER_FAMILY#{userID}#{family}
	GSI3SK string `dynamorm:"index:gsi3,sk,attr:gsi3SK" json:"-"` // {createdAt}

	// Core token data
	Token      string `dynamorm:"attr:token" json:"token"`             // The actual token value
	Generation int    `dynamorm:"attr:generation" json:"generation"`   // Rotation generation number
	CreatedAt  int64  `dynamorm:"attr:createdAt" json:"created_at"`    // Unix timestamp
	ExpiresAt  int64  `dynamorm:"attr:expiresAt" json:"expires_at"`    // Unix timestamp
	LastUsedAt int64  `dynamorm:"attr:lastUsedAt" json:"last_used_at"` // Unix timestamp for tracking

	// Security fields
	Revoked       bool   `dynamorm:"attr:revoked" json:"revoked"`              // Whether token is revoked
	RevokedReason string `dynamorm:"attr:revokedReason" json:"revoked_reason"` // Reason for revocation

	// Tracking fields
	DeviceName string `dynamorm:"attr:deviceName" json:"device_name"` // Optional device identifier
	IPAddress  string `dynamorm:"attr:ipAddress" json:"ip_address"`   // IP address for security monitoring

	// TTL for automatic cleanup
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"-"`
}

// TableName returns the DynamoDB table name
func (AuthRefreshToken) TableName() string {
	return MainTableName
}

// UpdateKeys ensures all keys are properly set for DynamoDB operations
func (a *AuthRefreshToken) UpdateKeys() error {
	// Primary key is the token value itself
	a.PK = a.Token
	a.SK = SKToken

	createdAtStr := fmt.Sprintf("%d", a.CreatedAt)
	a.UserFamily = fmt.Sprintf("%s#%s", a.UserID, a.Family)

	a.GSI1PK = fmt.Sprintf("USER#%s", a.UserID)
	a.GSI1SK = createdAtStr
	a.GSI2PK = fmt.Sprintf("FAMILY#%s", a.Family)
	a.GSI2SK = createdAtStr
	a.GSI3PK = fmt.Sprintf("USER_FAMILY#%s#%s", a.UserID, a.Family)
	a.GSI3SK = createdAtStr

	// Set TTL from ExpiresAt if not already set
	if a.TTL == 0 && a.ExpiresAt > 0 {
		a.TTL = a.ExpiresAt
	}

	return nil
}

// GetPK returns the partition key for this model
func (a *AuthRefreshToken) GetPK() string {
	return a.PK
}

// GetSK returns the sort key for this model
func (a *AuthRefreshToken) GetSK() string {
	return a.SK
}

// BeforeCreate sets up keys before creating the record
func (a *AuthRefreshToken) BeforeCreate() error {
	return a.UpdateKeys()
}

// BeforeUpdate sets up keys before updating the record
func (a *AuthRefreshToken) BeforeUpdate() error {
	return a.UpdateKeys()
}

// IsExpired checks if the token has expired
func (a *AuthRefreshToken) IsExpired() bool {
	return time.Now().Unix() > a.ExpiresAt
}

// IsActive checks if the token is active (not revoked and not expired)
func (a *AuthRefreshToken) IsActive() bool {
	return !a.Revoked && !a.IsExpired()
}
