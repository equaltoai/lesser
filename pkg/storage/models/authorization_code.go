package models

import (
	"time"
)

// AuthorizationCode represents an OAuth 2.0 authorization code
type AuthorizationCode struct {
	// DynamoDB keys - MUST match legacy exactly
	PK string `dynamorm:"pk" json:"-"` // AUTHCODE#code
	SK string `dynamorm:"sk" json:"-"` // CODE
	
	// Core fields from legacy storage.AuthorizationCode
	Code          string    `json:"Code"`
	ClientID      string    `json:"ClientID"`
	Username      string    `json:"Username"`
	CodeChallenge string    `json:"CodeChallenge"`
	ExpiresAt     time.Time `json:"ExpiresAt"`
	Scopes        []string  `json:"Scopes"`
	
	// Tracking fields
	CreatedAt time.Time `json:"CreatedAt"`
	
	// TTL field for automatic expiration
	TTL int64 `dynamorm:"ttl" json:"-"`
}

// TableName returns the DynamoDB table name
func (AuthorizationCode) TableName() string {
	return "lesser-main"
}

// BeforeCreate sets up the keys and TTL before creating
func (a *AuthorizationCode) BeforeCreate() error {
	a.PK = "AUTHCODE#" + a.Code
	a.SK = "CODE"
	
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now()
	}
	
	// Set TTL if not already set
	if a.TTL == 0 && !a.ExpiresAt.IsZero() {
		a.TTL = a.ExpiresAt.Unix()
	}
	
	return nil
}