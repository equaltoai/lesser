package models

import (
	"time"
)

// WebAuthnChallenge represents a temporary challenge for WebAuthn registration/authentication
type WebAuthnChallenge struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// DynamoDB keys - MUST match legacy exactly
	PK string `theorydb:"pk,attr:PK" json:"-"` // CHALLENGE#challenge
	SK string `theorydb:"sk,attr:SK" json:"-"` // WEBAUTHN

	// Core fields from legacy storage.WebAuthnChallenge
	Challenge   string    `theorydb:"attr:challenge" json:"challenge"`
	UserID      string    `theorydb:"attr:userID" json:"user_id"`
	SessionData []byte    `theorydb:"attr:sessionData" json:"session_data"` // Serialized session data
	ExpiresAt   time.Time `theorydb:"attr:expiresAt" json:"expires_at"`
	Type        string    `theorydb:"attr:type" json:"type"` // "registration" or "authentication"

	// Additional fields for DynamoDB
	ItemType string `theorydb:"attr:itemType" json:"ItemType"` // "WebAuthnChallenge"

	// TTL field for automatic expiration
	TTL int64 `theorydb:"ttl,attr:ttl" json:"-"`
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
