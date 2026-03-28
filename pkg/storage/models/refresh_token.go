package models

import (
	"fmt"
	"time"
)

// RefreshToken represents an OAuth 2.0 refresh token
type RefreshToken struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// DynamoDB keys - MUST match legacy exactly
	PK string `theorydb:"pk,attr:PK" json:"-"` // REFRESHTOKEN#token
	SK string `theorydb:"sk,attr:SK" json:"-"` // TOKEN

	// Runtime session lookup GSIs. These reuse the shared table GSIs and are only set for
	// refresh tokens that carry runtime session metadata.
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty" json:"-"`
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty" json:"-"`
	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK,omitempty" json:"-"`
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK,omitempty" json:"-"`
	GSI3PK string `theorydb:"index:gsi3,pk,attr:gsi3PK,omitempty" json:"-"`
	GSI3SK string `theorydb:"index:gsi3,sk,attr:gsi3SK,omitempty" json:"-"`

	// Core fields from legacy storage.RefreshToken
	Token       string    `theorydb:"attr:token" json:"Token"`
	ClientID    string    `theorydb:"attr:clientID" json:"ClientID"`
	Username    string    `theorydb:"attr:username" json:"Username"`
	Resource    string    `theorydb:"attr:resource,omitempty" json:"Resource,omitempty"`
	ExpiresAt   time.Time `theorydb:"attr:expiresAt" json:"ExpiresAt"`
	Scopes      []string  `theorydb:"attr:scopes" json:"Scopes"`
	ClientClass string    `theorydb:"attr:clientClass" json:"ClientClass,omitempty"`
	SessionID   string    `theorydb:"attr:sessionID" json:"SessionID,omitempty"`

	// Runtime session metadata. These fields are optional for regular OAuth clients and used
	// by long-lived agent runtimes for safe rotation and operator-visible session management.
	FamilyID            string    `theorydb:"attr:familyID,omitempty" json:"FamilyID,omitempty"`
	Generation          int       `theorydb:"attr:generation,omitempty" json:"Generation,omitempty"`
	Current             bool      `theorydb:"attr:current" json:"Current"`
	Revoked             bool      `theorydb:"attr:revoked" json:"Revoked"`
	RevokedAt           time.Time `theorydb:"attr:revokedAt,omitempty" json:"RevokedAt,omitempty"`
	RevokedReason       string    `theorydb:"attr:revokedReason,omitempty" json:"RevokedReason,omitempty"`
	DeviceLabel         string    `theorydb:"attr:deviceLabel,omitempty" json:"DeviceLabel,omitempty"`
	LastUsedAt          time.Time `theorydb:"attr:lastUsedAt,omitempty" json:"LastUsedAt,omitempty"`
	IdleExpiresAt       time.Time `theorydb:"attr:idleExpiresAt,omitempty" json:"IdleExpiresAt,omitempty"`
	AbsoluteExpiresAt   time.Time `theorydb:"attr:absoluteExpiresAt,omitempty" json:"AbsoluteExpiresAt,omitempty"`
	SessionCreatedAt    time.Time `theorydb:"attr:sessionCreatedAt,omitempty" json:"SessionCreatedAt,omitempty"`
	AccessTTLSeconds    int       `theorydb:"attr:accessTTLSeconds,omitempty" json:"AccessTTLSeconds,omitempty"`
	ReuseDetectedAt     time.Time `theorydb:"attr:reuseDetectedAt,omitempty" json:"ReuseDetectedAt,omitempty"`
	ReuseDetectedFromIP string    `theorydb:"attr:reuseDetectedFromIP,omitempty" json:"ReuseDetectedFromIP,omitempty"`
	ReuseDetectedFromUA string    `theorydb:"attr:reuseDetectedFromUA,omitempty" json:"ReuseDetectedFromUA,omitempty"`
	LastAuthFailureCode string    `theorydb:"attr:lastAuthFailureCode,omitempty" json:"LastAuthFailureCode,omitempty"`
	LastAuthFailureAt   time.Time `theorydb:"attr:lastAuthFailureAt,omitempty" json:"LastAuthFailureAt,omitempty"`
	LastAuthFailureMsg  string    `theorydb:"attr:lastAuthFailureMsg,omitempty" json:"LastAuthFailureMsg,omitempty"`
	LastAuthSuccessAt   time.Time `theorydb:"attr:lastAuthSuccessAt,omitempty" json:"LastAuthSuccessAt,omitempty"`

	// Tracking fields
	CreatedAt time.Time `theorydb:"attr:createdAt" json:"CreatedAt"`

	// TTL field for automatic expiration
	TTL int64 `theorydb:"ttl,attr:ttl" json:"-"`

	// Version enables optimistic locking so refresh rotation can use compare-and-swap semantics.
	Version int `theorydb:"version,attr:version" json:"Version,omitempty"`
}

// TableName returns the DynamoDB table name
func (RefreshToken) TableName() string {
	return MainTableName
}

// BeforeCreate sets up the keys and TTL before creating
func (r *RefreshToken) BeforeCreate() error {
	r.PK = "REFRESHTOKEN#" + r.Token
	r.SK = SKToken

	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now()
	}
	if r.carriesRuntimeSessionState() {
		if r.SessionCreatedAt.IsZero() {
			r.SessionCreatedAt = r.CreatedAt
		}
		if r.LastUsedAt.IsZero() {
			r.LastUsedAt = r.CreatedAt
		}
		if r.IdleExpiresAt.IsZero() {
			r.IdleExpiresAt = r.ExpiresAt
		}
		if r.AbsoluteExpiresAt.IsZero() {
			r.AbsoluteExpiresAt = r.ExpiresAt
		}
		if r.Generation == 0 {
			r.Generation = 1
		}
		if !r.Revoked && !r.Current {
			r.Current = true
		}
	}

	// Set TTL if not already set
	if r.TTL == 0 && !r.ExpiresAt.IsZero() {
		r.TTL = r.ExpiresAt.Unix()
	}

	r.updateRuntimeIndexes()

	return nil
}

// GetPK returns the partition key for BaseModel interface
func (r *RefreshToken) GetPK() string {
	return r.PK
}

// GetSK returns the sort key for BaseModel interface
func (r *RefreshToken) GetSK() string {
	return r.SK
}

// UpdateKeys implements BaseModel interface and updates DynamoDB keys
func (r *RefreshToken) UpdateKeys() error {
	r.PK = "REFRESHTOKEN#" + r.Token
	r.SK = "TOKEN"
	r.updateRuntimeIndexes()
	return nil
}

func (r *RefreshToken) updateRuntimeIndexes() {
	r.GSI1PK = ""
	r.GSI1SK = ""
	r.GSI2PK = ""
	r.GSI2SK = ""
	r.GSI3PK = ""
	r.GSI3SK = ""

	if !r.carriesRuntimeSessionState() {
		return
	}

	if r.Username != "" && r.ClientID != "" {
		r.GSI1PK = fmt.Sprintf("RUNTIME_USER#%s#%s", r.Username, r.ClientID)
		r.GSI1SK = fmt.Sprintf("%d#%s#%08d", r.SessionCreatedAt.Unix(), r.SessionID, r.Generation)
	}
	if r.FamilyID != "" {
		r.GSI2PK = "RUNTIME_FAMILY#" + r.FamilyID
		r.GSI2SK = fmt.Sprintf("%08d", r.Generation)
	}
	if r.SessionID != "" {
		r.GSI3PK = "RUNTIME_SESSION#" + r.SessionID
		r.GSI3SK = fmt.Sprintf("%08d", r.Generation)
	}
}

func (r *RefreshToken) carriesRuntimeSessionState() bool {
	return r.FamilyID != "" ||
		r.Generation > 0 ||
		r.Current ||
		r.DeviceLabel != "" ||
		!r.LastUsedAt.IsZero() ||
		!r.IdleExpiresAt.IsZero() ||
		!r.AbsoluteExpiresAt.IsZero() ||
		!r.SessionCreatedAt.IsZero() ||
		r.AccessTTLSeconds > 0 ||
		!r.ReuseDetectedAt.IsZero() ||
		r.ReuseDetectedFromIP != "" ||
		r.ReuseDetectedFromUA != ""
}
