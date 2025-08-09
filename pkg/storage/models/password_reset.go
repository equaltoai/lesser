package models

import (
	"fmt"
	"time"
)

// PasswordReset represents a password reset token
type PasswordReset struct {
	PK string `dynamorm:"pk" json:"pk"` // USER#{username}
	SK string `dynamorm:"sk" json:"sk"` // RESET#{token}

	// GSI1 for token lookup
	GSI1PK string `dynamorm:"index:token-index,pk" json:"gsi1_pk"` // RESET_TOKEN#{token}
	GSI1SK string `dynamorm:"index:token-index,sk" json:"gsi1_sk"` // USERNAME#{username}

	Username  string    `json:"username"`
	Token     string    `json:"token"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Used      bool      `json:"used"`
	UsedAt    time.Time `json:"used_at,omitempty"`

	// TTL for automatic cleanup
	TTL int64 `dynamorm:"ttl" json:"ttl,omitempty"`
}

// TableName returns the DynamoDB table name
func (PasswordReset) TableName() string {
	return MainTableName
}

// BeforeCreate sets up the model before creation
func (r *PasswordReset) BeforeCreate() error {
	r.PK = fmt.Sprintf(KeyPatternUser, r.Username)
	r.SK = fmt.Sprintf("RESET#%s", r.Token)

	// Set up GSI for token lookup
	r.GSI1PK = fmt.Sprintf("RESET_TOKEN#%s", r.Token)
	r.GSI1SK = fmt.Sprintf("USERNAME#%s", r.Username)

	// Set TTL to expiry time + 1 day
	r.TTL = r.ExpiresAt.Add(24 * time.Hour).Unix()

	return nil
}

// GetPK returns the partition key
func (r *PasswordReset) GetPK() string {
	return r.PK
}

// GetSK returns the sort key
func (r *PasswordReset) GetSK() string {
	return r.SK
}

// UpdateKeys updates the keys
func (r *PasswordReset) UpdateKeys() error {
	return r.BeforeCreate()
}
