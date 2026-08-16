package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

const (
	// KeyPatternPasskeyRegistrationProof is the canonical PK pattern for passkey signup proofs.
	KeyPatternPasskeyRegistrationProof = "PASSKEY_REGISTRATION_PROOF#%s"
	// SKPasskeyRegistrationProof is the canonical SK for passkey signup proofs.
	SKPasskeyRegistrationProof = "PROOF"
	// DefaultPasskeyRegistrationProofTTL keeps proofs short-lived and self-cleaning.
	DefaultPasskeyRegistrationProofTTL = 10 * time.Minute
)

// PasskeyRegistrationProof is the single-use proof persisted between WebAuthn signup
// verification and account registration.
type PasskeyRegistrationProof struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"-"`
	SK string `theorydb:"sk,attr:SK" json:"-"`

	TTL int64 `theorydb:"ttl,attr:ttl" json:"-"`

	ID              string    `theorydb:"attr:id" json:"id"`
	Username        string    `theorydb:"attr:username" json:"username"`
	CeremonyID      string    `theorydb:"attr:ceremonyID" json:"ceremony_id"`
	CredentialID    string    `theorydb:"attr:credentialID" json:"credential_id"`
	PublicKey       []byte    `theorydb:"attr:publicKey" json:"public_key"`
	AttestationType string    `theorydb:"attr:attestationType" json:"attestation_type"`
	AAGUID          []byte    `theorydb:"attr:aaguid" json:"aaguid"`
	SignCount       uint32    `theorydb:"attr:signCount" json:"sign_count"`
	CloneWarning    bool      `theorydb:"attr:cloneWarning" json:"clone_warning"`
	BackupEligible  bool      `theorydb:"attr:backupEligible" json:"backup_eligible"`
	BackupState     bool      `theorydb:"attr:backupState" json:"backup_state"`
	CreatedAt       time.Time `theorydb:"attr:createdAt" json:"created_at"`
	ExpiresAt       time.Time `theorydb:"attr:expiresAt" json:"expires_at"`
	Consumed        bool      `theorydb:"attr:consumed" json:"consumed"`
	ConsumedAt      time.Time `theorydb:"attr:consumedAt" json:"consumed_at,omitempty"`
}

// TableName returns the DynamoDB table name.
func (PasskeyRegistrationProof) TableName() string {
	return MainTableName
}

// BeforeCreate sets keys and TTL defaults before creation.
func (p *PasskeyRegistrationProof) BeforeCreate() error {
	now := time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	if p.ExpiresAt.IsZero() {
		p.ExpiresAt = p.CreatedAt.Add(DefaultPasskeyRegistrationProofTTL)
	}
	if err := p.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}
	return p.Validate()
}

// BeforeUpdate keeps keys and TTL aligned on update.
func (p *PasskeyRegistrationProof) BeforeUpdate() error {
	if err := p.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}
	return p.Validate()
}

// GetPK returns the partition key.
func (p *PasskeyRegistrationProof) GetPK() string {
	return p.PK
}

// GetSK returns the sort key.
func (p *PasskeyRegistrationProof) GetSK() string {
	return p.SK
}

// UpdateKeys updates the keys and TTL from the current model state.
func (p *PasskeyRegistrationProof) UpdateKeys() error {
	p.ID = strings.TrimSpace(p.ID)
	p.Username = strings.TrimSpace(p.Username)
	p.CeremonyID = strings.TrimSpace(p.CeremonyID)
	p.CredentialID = strings.TrimSpace(p.CredentialID)

	if err := common.ValidateRequiredParam("id", p.ID); err != nil {
		return err
	}

	p.PK = fmt.Sprintf(KeyPatternPasskeyRegistrationProof, p.ID)
	p.SK = SKPasskeyRegistrationProof
	p.TTL = p.ExpiresAt.Unix()
	return nil
}

// Validate ensures the persisted proof is internally consistent.
func (p *PasskeyRegistrationProof) Validate() error {
	if err := common.ValidateRequiredParam("id", p.ID); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("username", p.Username); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("ceremonyID", p.CeremonyID); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("credentialID", p.CredentialID); err != nil {
		return err
	}
	if len(p.PublicKey) == 0 {
		return fmt.Errorf("public key is required")
	}
	if p.CreatedAt.IsZero() {
		return fmt.Errorf("created_at is required")
	}
	if p.ExpiresAt.IsZero() {
		return fmt.Errorf("expires_at is required")
	}
	if !p.ExpiresAt.After(p.CreatedAt) {
		return fmt.Errorf("expires_at must be after created_at")
	}
	return nil
}

// IsExpired reports whether the proof is expired at the provided time.
func (p *PasskeyRegistrationProof) IsExpired(now time.Time) bool {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return !now.Before(p.ExpiresAt)
}
