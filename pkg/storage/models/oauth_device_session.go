package models

import (
	"fmt"
	"time"
)

// OAuthDeviceSession represents an OAuth device authorization session (RFC 8628-style).
//
// The device_code is treated as a secret and is never stored directly; callers store only a hash.
// The user_code is short and human-entered, and is indexed for lookup by the web UI approval flow.
type OAuthDeviceSession struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary keys
	PK string `theorydb:"pk,attr:PK" json:"-"` // OAUTH_DEVICE#<deviceCodeHash>
	SK string `theorydb:"sk,attr:SK" json:"-"` // SESSION

	// GSI1 - user_code lookup (web approval flow)
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK" json:"-"` // OAUTH_DEVICE_USER_CODE#<userCode>
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK" json:"-"` // <createdAt>#<deviceCodeHash>

	// Core fields
	DeviceCodeHash string   `theorydb:"attr:deviceCodeHash" json:"device_code_hash"`
	UserCode       string   `theorydb:"attr:userCode" json:"user_code"`
	ClientID       string   `theorydb:"attr:clientID" json:"client_id"`
	Scopes         []string `theorydb:"attr:scopes" json:"scopes,omitempty"`

	// State machine
	Status string `theorydb:"attr:status" json:"status"` // pending|approved|denied|consumed

	// Polling governance (abuse control)
	IntervalSeconds int        `theorydb:"attr:intervalSeconds" json:"interval_seconds"`
	PollCount       int        `theorydb:"attr:pollCount" json:"poll_count,omitempty"`
	LastPolledAt    *time.Time `theorydb:"attr:lastPolledAt" json:"last_polled_at,omitempty"`

	// Approval outcome
	ApprovedUsername string     `theorydb:"attr:approvedUsername" json:"approved_username,omitempty"`
	ApprovedAt       *time.Time `theorydb:"attr:approvedAt" json:"approved_at,omitempty"`
	DeniedAt         *time.Time `theorydb:"attr:deniedAt" json:"denied_at,omitempty"`
	ConsumedAt       *time.Time `theorydb:"attr:consumedAt" json:"consumed_at,omitempty"`

	// Timestamps
	CreatedAt time.Time `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
	ExpiresAt time.Time `theorydb:"attr:expiresAt" json:"expires_at"`

	// DynamoDB TTL (derived from ExpiresAt)
	TTL int64 `theorydb:"ttl,attr:ttl" json:"-"`
}

// TableName returns the DynamoDB table name for OAuth device sessions.
func (OAuthDeviceSession) TableName() string {
	return MainTableName
}

// GetPK returns the DynamoDB partition key for the session.
func (o *OAuthDeviceSession) GetPK() string {
	return o.PK
}

// GetSK returns the DynamoDB sort key for the session.
func (o *OAuthDeviceSession) GetSK() string {
	return o.SK
}

// UpdateKeys derives primary/GSI keys and TTL fields from the session content.
func (o *OAuthDeviceSession) UpdateKeys() error {
	if o.DeviceCodeHash != "" {
		o.PK = "OAUTH_DEVICE#" + o.DeviceCodeHash
		o.SK = "SESSION"
	}

	if o.UserCode != "" {
		o.GSI1PK = "OAUTH_DEVICE_USER_CODE#" + o.UserCode
		created := o.CreatedAt
		if created.IsZero() {
			created = time.Now().UTC()
		}
		o.GSI1SK = fmt.Sprintf("%s#%s", created.Format(time.RFC3339Nano), o.DeviceCodeHash)
	}

	if !o.ExpiresAt.IsZero() {
		o.TTL = o.ExpiresAt.Unix()
	}

	return nil
}
