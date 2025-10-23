package models

import (
	"fmt"
	"math"
	"time"
)

// OAuthClient represents an OAuth 2.0 client application
type OAuthClient struct {
	// DynamoDB keys - MUST match legacy exactly
	PK             string `dynamorm:"pk" json:"-"`                           // OAUTH_CLIENT#clientID
	SK             string `dynamorm:"sk" json:"-"`                           // METADATA
	GSI1PK         string `dynamorm:"index:gsi1,pk" json:"-"`                // OWNER#ownerID (for owner index)
	GSI1SK         string `dynamorm:"index:gsi1,sk" json:"-"`                // CLIENT#clientID
	OAuthClientsPK string `dynamorm:"index:oauth-clients-index,pk" json:"-"` // OAUTH_CLIENTS
	OAuthClientsSK string `dynamorm:"index:oauth-clients-index,sk" json:"-"` // CREATED_AT#{ts_desc}#CLIENT#{clientID}

	// Core fields from legacy storage.OAuthClient
	ID           string    `json:"id,omitempty"`
	ClientID     string    `json:"client_id"`
	ClientSecret string    `json:"client_secret"`
	Name         string    `json:"name"`
	Description  string    `json:"description,omitempty"`
	Website      string    `json:"website,omitempty"`
	RedirectURIs []string  `json:"redirect_uris"`
	GrantTypes   []string  `json:"grant_types,omitempty"`
	Scopes       []string  `json:"scopes,omitempty"`
	OwnerID      string    `json:"owner_id,omitempty"`
	Confidential bool      `json:"confidential"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at,omitempty"`
}

// TableName returns the DynamoDB table name
func (OAuthClient) TableName() string {
	return MainTableName
}

// BeforeCreate sets up the keys before creating
func (o *OAuthClient) BeforeCreate() error {
	if o.CreatedAt.IsZero() {
		o.CreatedAt = time.Now().UTC()
	}
	if o.UpdatedAt.IsZero() {
		o.UpdatedAt = o.CreatedAt
	}
	if err := o.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}
	return nil
}

// BeforeUpdate sets the updated timestamp
func (o *OAuthClient) BeforeUpdate() error {
	o.UpdatedAt = time.Now().UTC()
	if err := o.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}
	return nil
}

// GetPK returns the partition key for BaseModel interface
func (o *OAuthClient) GetPK() string {
	return o.PK
}

// GetSK returns the sort key for BaseModel interface
func (o *OAuthClient) GetSK() string {
	return o.SK
}

// UpdateKeys implements BaseModel interface and updates DynamoDB keys
func (o *OAuthClient) UpdateKeys() error {
	o.PK = "OAUTH_CLIENT#" + o.ClientID
	o.SK = SKMetadata
	if o.OwnerID != "" {
		o.GSI1PK = "OWNER#" + o.OwnerID
		o.GSI1SK = "CLIENT#" + o.ClientID
	}
	desc := encodeDescendingTimestamp(o.CreatedAt)
	o.OAuthClientsPK = "OAUTH_CLIENTS"
	o.OAuthClientsSK = fmt.Sprintf("CREATED_AT#%019d#CLIENT#%s", desc, o.ClientID)
	return nil
}

func encodeDescendingTimestamp(timestamp time.Time) int64 {
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	return math.MaxInt64 - timestamp.UTC().UnixNano()
}
