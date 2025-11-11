package models

import (
	"fmt"
	"time"
)

// OAuthState represents OAuth state data in DynamoDB
type OAuthState struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Key fields
	PK string `dynamorm:"pk,attr:PK" json:"-"`
	SK string `dynamorm:"sk,attr:SK" json:"-"`

	// State data fields
	State               string    `dynamorm:"attr:state" json:"state"`
	Provider            string    `dynamorm:"attr:provider" json:"provider"`
	RedirectURI         string    `dynamorm:"attr:redirectURI" json:"redirect_uri"`
	Username            string    `dynamorm:"attr:username" json:"username,omitempty"`
	ClientID            string    `dynamorm:"attr:clientID" json:"client_id,omitempty"`
	Scopes              []string  `dynamorm:"attr:scopes" json:"scopes,omitempty"`
	CodeChallenge       string    `dynamorm:"attr:codeChallenge" json:"code_challenge,omitempty"`
	CodeChallengeMethod string    `dynamorm:"attr:codeChallengeMethod" json:"code_challenge_method,omitempty"`
	CreatedAt           time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	ExpiresAt           time.Time `dynamorm:"attr:expiresAt" json:"expires_at"`

	// DynamoDB TTL field
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// GetPK returns the partition key for BaseModel interface
func (o *OAuthState) GetPK() string {
	return o.PK
}

// GetSK returns the sort key for BaseModel interface
func (o *OAuthState) GetSK() string {
	return o.SK
}

// UpdateKeys implements BaseModel interface and updates the composite keys based on the state
func (o *OAuthState) UpdateKeys() error {
	if o.State != "" {
		o.PK = fmt.Sprintf("OAUTH_STATE#%s", o.State)
		o.SK = "STATE"
	}

	// Set TTL for DynamoDB
	if !o.ExpiresAt.IsZero() {
		o.TTL = o.ExpiresAt.Unix()
	}
	return nil
}

// TableName returns the DynamoDB table backing OAuthState.
func (OAuthState) TableName() string {
	return MainTableName
}
