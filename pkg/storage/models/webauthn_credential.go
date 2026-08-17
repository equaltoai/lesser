package models

import (
	"time"
)

// WebAuthnCredential represents a WebAuthn credential for passwordless authentication
type WebAuthnCredential struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Canonical DynamoDB keys for the account-scoped WebAuthn credential row.
	PK string `theorydb:"pk,attr:PK" json:"-"` // USER#username
	SK string `theorydb:"sk,attr:SK" json:"-"` // WEBAUTHN_CRED#credentialID
	// GSI for credential lookup by ID
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty" json:"-"`
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty" json:"-"`

	// Persisted WebAuthn credential fields.
	ID              string    `theorydb:"attr:id" json:"id"`
	UserID          string    `theorydb:"attr:userID" json:"user_id"`
	PublicKey       []byte    `theorydb:"attr:publicKey" json:"public_key"`
	AttestationType string    `theorydb:"attr:attestationType" json:"attestation_type"`
	AAGUID          []byte    `theorydb:"attr:aaguid" json:"aaguid"`
	SignCount       uint32    `theorydb:"attr:signCount" json:"sign_count"`
	CloneWarning    bool      `theorydb:"attr:cloneWarning" json:"clone_warning"`
	BackupEligible  bool      `theorydb:"attr:backupEligible" json:"backup_eligible"`
	BackupState     bool      `theorydb:"attr:backupState" json:"backup_state"`
	CreatedAt       time.Time `theorydb:"attr:createdAt" json:"created_at"`
	LastUsedAt      time.Time `theorydb:"attr:lastUsedAt" json:"last_used_at"`
	Name            string    `theorydb:"attr:name" json:"name"` // User-friendly name

	// Additional fields for DynamoDB queries
	Type string `theorydb:"attr:type" json:"Type"` // "WebAuthnCredential"
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
