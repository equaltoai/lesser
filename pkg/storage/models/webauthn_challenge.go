package models

import (
	"time"
)

// WebAuthnChallenge represents a temporary challenge for WebAuthn registration/authentication
type WebAuthnChallenge struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// DynamoDB keys - MUST match legacy exactly
	PK string `dynamorm:"pk,attr:PK" json:"-"` // CHALLENGE#challenge
	SK string `dynamorm:"sk,attr:SK" json:"-"` // WEBAUTHN

	// Core fields from legacy storage.WebAuthnChallenge
	Challenge   string    `dynamorm:"attr:challenge" json:"challenge"`
	UserID      string    `dynamorm:"attr:userID" json:"user_id"`
	SessionData []byte    `dynamorm:"attr:sessionData" json:"session_data"` // Serialized session data
	ExpiresAt   time.Time `dynamorm:"attr:expiresAt" json:"expires_at"`
	Type        string    `dynamorm:"attr:type" json:"type"` // "registration" or "authentication"

	// Additional fields for DynamoDB
	ItemType string `dynamorm:"attr:ItemType" json:"ItemType"` // "WebAuthnChallenge"

	// TTL field for automatic expiration
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"-"`
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

// GetPK returns the partition key
func (w *WebAuthnChallenge) GetPK() string {
	return w.PK
}

// GetSK returns the sort key
func (w *WebAuthnChallenge) GetSK() string {
	return w.SK
}

// UpdateKeys updates the primary and sort keys based on the model data
func (w *WebAuthnChallenge) UpdateKeys() error {
	w.PK = "CHALLENGE#" + w.Challenge
	w.SK = "WEBAUTHN"
	w.ItemType = "WebAuthnChallenge"

	// Set TTL if not already set (5 minute expiry)
	if w.TTL == 0 && !w.ExpiresAt.IsZero() {
		w.TTL = w.ExpiresAt.Unix()
	}

	return nil
}
