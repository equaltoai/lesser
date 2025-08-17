package models

import (
	"time"
)

// OAuthClient represents an OAuth 2.0 client application
type OAuthClient struct {
	// DynamoDB keys - MUST match legacy exactly
	PK     string `dynamorm:"pk" json:"-"`      // CLIENT#clientID  
	SK     string `dynamorm:"sk" json:"-"`      // METADATA
	GSI1PK string `dynamorm:"gsi1pk" json:"-"`  // OWNER#ownerID (for owner index)
	GSI1SK string `dynamorm:"gsi1sk" json:"-"`  // CLIENT#clientID

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

// UpdateKeys updates the DynamoDB keys based on current field values
func (o *OAuthClient) UpdateKeys() {
	o.PK = "OAUTH_CLIENT#" + o.ClientID
	o.SK = "CLIENT"
	if o.OwnerID != "" {
		o.GSI1PK = "OWNER#" + o.OwnerID
		o.GSI1SK = "CLIENT#" + o.ClientID
	}
}

// BeforeCreate sets up the keys before creating
func (o *OAuthClient) BeforeCreate() error {
	o.UpdateKeys()
	if o.CreatedAt.IsZero() {
		o.CreatedAt = time.Now()
	}
	if o.UpdatedAt.IsZero() {
		o.UpdatedAt = o.CreatedAt
	}
	return nil
}

// BeforeUpdate sets the updated timestamp
func (o *OAuthClient) BeforeUpdate() error {
	o.UpdateKeys()
	o.UpdatedAt = time.Now()
	return nil
}
