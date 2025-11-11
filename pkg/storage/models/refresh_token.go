package models

import (
	"time"
)

// RefreshToken represents an OAuth 2.0 refresh token
type RefreshToken struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// DynamoDB keys - MUST match legacy exactly
	PK string `dynamorm:"pk,attr:PK" json:"-"` // REFRESHTOKEN#token
	SK string `dynamorm:"sk,attr:SK" json:"-"` // TOKEN

	// Core fields from legacy storage.RefreshToken
	Token     string    `dynamorm:"attr:token" json:"Token"`
	ClientID  string    `dynamorm:"attr:clientID" json:"ClientID"`
	Username  string    `dynamorm:"attr:username" json:"Username"`
	ExpiresAt time.Time `dynamorm:"attr:expiresAt" json:"ExpiresAt"`
	Scopes    []string  `dynamorm:"attr:scopes" json:"Scopes"`

	// Tracking fields
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"CreatedAt"`

	// TTL field for automatic expiration
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"-"`
}

// TableName returns the DynamoDB table name
func (RefreshToken) TableName() string {
	return MainTableName
}

// BeforeCreate sets up the keys and TTL before creating
func (r *RefreshToken) BeforeCreate() error {
	r.PK = "REFRESHTOKEN#" + r.Token
	r.SK = SKToken

	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now()
	}

	// Set TTL if not already set
	if r.TTL == 0 && !r.ExpiresAt.IsZero() {
		r.TTL = r.ExpiresAt.Unix()
	}

	return nil
}

// GetPK returns the partition key for BaseModel interface
func (r *RefreshToken) GetPK() string {
	return r.PK
}

// GetSK returns the sort key for BaseModel interface
func (r *RefreshToken) GetSK() string {
	return r.SK
}

// UpdateKeys implements BaseModel interface and updates DynamoDB keys
func (r *RefreshToken) UpdateKeys() error {
	r.PK = "REFRESHTOKEN#" + r.Token
	r.SK = "TOKEN"
	return nil
}
