package models

import (
	"time"
)

// WebAuthnCredential represents a WebAuthn credential for passwordless authentication
type WebAuthnCredential struct {
	// DynamoDB keys - MUST match legacy exactly
	PK string `dynamorm:"pk" json:"-"` // USER#username
	SK string `dynamorm:"sk" json:"-"` // WEBAUTHN_CRED#credentialID
	// GSI for credential lookup by ID
	GSI1PK string `dynamorm:"index:gsi1,pk" json:"-"`
	GSI1SK string `dynamorm:"index:gsi1,sk" json:"-"`

	// Core fields from legacy storage.WebAuthnCredential
	ID              string    `json:"id"`
	UserID          string    `json:"user_id"`
	PublicKey       []byte    `json:"public_key"`
	AttestationType string    `json:"attestation_type"`
	AAGUID          []byte    `json:"aaguid"`
	SignCount       uint32    `json:"sign_count"`
	CloneWarning    bool      `json:"clone_warning"`
	BackupEligible  bool      `json:"backup_eligible"`
	BackupState     bool      `json:"backup_state"`
	CreatedAt       time.Time `json:"created_at"`
	LastUsedAt      time.Time `json:"last_used_at"`
	Name            string    `json:"name"` // User-friendly name

	// Additional fields for DynamoDB queries
	Type string `json:"Type"` // "WebAuthnCredential"
}

// TableName returns the DynamoDB table name
func (WebAuthnCredential) TableName() string {
	return MainTableName
}

// BeforeCreate sets up the keys before creating
func (w *WebAuthnCredential) BeforeCreate() error {
	w.PK = "USER#" + w.UserID
	w.SK = "WEBAUTHN_CRED#" + w.ID
	w.GSI1PK = "WEBAUTHN_CREDENTIAL#" + w.ID
	w.GSI1SK = "USER#" + w.UserID
	w.Type = "WebAuthnCredential"

	if w.CreatedAt.IsZero() {
		w.CreatedAt = time.Now()
	}
	if w.LastUsedAt.IsZero() {
		w.LastUsedAt = w.CreatedAt
	}

	return nil
}

// BeforeUpdate updates the last used timestamp
func (w *WebAuthnCredential) BeforeUpdate() error {
	w.LastUsedAt = time.Now()
	w.GSI1PK = "WEBAUTHN_CREDENTIAL#" + w.ID
	w.GSI1SK = "USER#" + w.UserID
	return nil
}

// GetPK returns the partition key
func (w *WebAuthnCredential) GetPK() string {
	return w.PK
}

// GetSK returns the sort key
func (w *WebAuthnCredential) GetSK() string {
	return w.SK
}

// UpdateKeys updates the primary and sort keys based on the model data
func (w *WebAuthnCredential) UpdateKeys() error {
	w.PK = "USER#" + w.UserID
	w.SK = "WEBAUTHN_CRED#" + w.ID
	w.GSI1PK = "WEBAUTHN_CREDENTIAL#" + w.ID
	w.GSI1SK = "USER#" + w.UserID
	w.Type = "WebAuthnCredential"
	return nil
}
