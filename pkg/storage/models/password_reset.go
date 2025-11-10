package models

import (
	"fmt"
	"time"
)

// PasswordReset represents a password reset token
type PasswordReset struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	PK string `dynamorm:"pk,attr:PK" json:"pk"` // USER#{username}
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // RESET#{token}

	// GSI1 for token lookup
	GSI1PK string `dynamorm:"index:token-index,pk,attr:gsi1PK" json:"gsi1_pk"` // RESET_TOKEN#{token}
	GSI1SK string `dynamorm:"index:token-index,sk,attr:gsi1SK" json:"gsi1_sk"` // USERNAME#{username}

	Username  string    `dynamorm:"attr:username" json:"username"`
	Token     string    `dynamorm:"attr:token" json:"token"`
	Email     string    `dynamorm:"attr:email" json:"email"`
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	ExpiresAt time.Time `dynamorm:"attr:expiresAt" json:"expires_at"`
	Used      bool      `dynamorm:"attr:used" json:"used"`
	UsedAt    time.Time `dynamorm:"attr:usedAt" json:"used_at,omitempty"`

	// TTL for automatic cleanup
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
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
	// Validate required fields
	if r.Username == "" {
		return fmt.Errorf("username is required")
	}
	if r.Token == "" {
		return fmt.Errorf("token is required")
	}

	// Set primary keys
	r.PK = fmt.Sprintf(KeyPatternUser, r.Username)
	r.SK = fmt.Sprintf("RESET#%s", r.Token)

	// Set up GSI for token lookup
	r.GSI1PK = fmt.Sprintf("RESET_TOKEN#%s", r.Token)
	r.GSI1SK = fmt.Sprintf("USERNAME#%s", r.Username)

	return nil
}
