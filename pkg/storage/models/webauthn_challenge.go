package models

import (
	"time"
)

// WebAuthnChallenge represents a temporary challenge for WebAuthn registration/authentication
type WebAuthnChallenge struct {
	// DynamoDB keys - MUST match legacy exactly
	PK string `dynamorm:"pk" json:"-"` // CHALLENGE#challenge
	SK string `dynamorm:"sk" json:"-"` // WEBAUTHN

	// Core fields from legacy storage.WebAuthnChallenge
	Challenge   string    `json:"challenge"`
	UserID      string    `json:"user_id"`
	SessionData []byte    `json:"session_data"` // Serialized session data
	ExpiresAt   time.Time `json:"expires_at"`
	Type        string    `json:"type"` // "registration" or "authentication"

	// Additional fields for DynamoDB
	ItemType string `json:"ItemType"` // "WebAuthnChallenge"

	// TTL field for automatic expiration
	TTL int64 `dynamorm:"ttl" json:"-"`
}

// TableName returns the DynamoDB table name
func (WebAuthnChallenge) TableName() string {
	return MainTableName
}

// BeforeCreate sets up the keys and TTL before creating
func (w *WebAuthnChallenge) BeforeCreate() error {
	w.PK = "CHALLENGE#" + w.Challenge
	w.SK = "WEBAUTHN"
	w.ItemType = "WebAuthnChallenge"

	// Set TTL if not already set (5 minute expiry)
	if w.TTL == 0 && !w.ExpiresAt.IsZero() {
		w.TTL = w.ExpiresAt.Unix()
	}

	return nil
}
