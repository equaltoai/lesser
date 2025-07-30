package models

import (
	"fmt"
	"time"
)

// OAuthState represents OAuth state data in DynamoDB
type OAuthState struct {
	// Key fields
	PK string `dynamorm:"pk" json:"-"`
	SK string `dynamorm:"sk" json:"-"`

	// State data fields
	State               string    `json:"state"`
	Provider            string    `json:"provider"`
	RedirectURI         string    `json:"redirect_uri"`
	Username            string    `json:"username,omitempty"`
	ClientID            string    `json:"client_id,omitempty"`
	Scopes              []string  `json:"scopes,omitempty"`
	CodeChallenge       string    `json:"code_challenge,omitempty"`
	CodeChallengeMethod string    `json:"code_challenge_method,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	ExpiresAt           time.Time `json:"expires_at"`

	// DynamoDB TTL field
	TTL int64 `dynamorm:"ttl" json:"ttl,omitempty"`
}

// UpdateKeys updates the composite keys based on the state
func (o *OAuthState) UpdateKeys() {
	if o.State != "" {
		o.PK = fmt.Sprintf("OAUTH_STATE#%s", o.State)
		o.SK = "STATE"
	}

	// Set TTL for DynamoDB
	if !o.ExpiresAt.IsZero() {
		o.TTL = o.ExpiresAt.Unix()
	}
}