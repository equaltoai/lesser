package models

import "time"

// RevokedAccessToken represents a revoked OAuth access token (JWT ID / JTI).
//
// Records are short-lived and should expire automatically via DynamoDB TTL when the token expires.
type RevokedAccessToken struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"-"` // REVOKEDTOKEN#<jti>
	SK string `theorydb:"sk,attr:SK" json:"-"` // TOKEN

	JTI       string    `theorydb:"attr:jti" json:"jti"`
	ExpiresAt time.Time `theorydb:"attr:expiresAt" json:"expires_at"`
	RevokedAt time.Time `theorydb:"attr:revokedAt" json:"revoked_at"`

	TTL int64 `theorydb:"ttl,attr:ttl" json:"-"`
}

// TableName returns the DynamoDB table name.
func (RevokedAccessToken) TableName() string {
	return MainTableName
}

// BeforeCreate sets up keys and TTL before creation.
func (r *RevokedAccessToken) BeforeCreate() error {
	r.PK = "REVOKEDTOKEN#" + r.JTI
	r.SK = SKToken

	if r.RevokedAt.IsZero() {
		r.RevokedAt = time.Now().UTC()
	}

	if r.TTL == 0 && !r.ExpiresAt.IsZero() {
		r.TTL = r.ExpiresAt.Unix()
	}

	return nil
}

// GetPK implements BaseModel interface.
func (r *RevokedAccessToken) GetPK() string {
	return r.PK
}

// GetSK implements BaseModel interface.
func (r *RevokedAccessToken) GetSK() string {
	return r.SK
}

// UpdateKeys implements BaseModel interface and updates DynamoDB keys.
func (r *RevokedAccessToken) UpdateKeys() error {
	r.PK = "REVOKEDTOKEN#" + r.JTI
	r.SK = SKToken
	return nil
}

